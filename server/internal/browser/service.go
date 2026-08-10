package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// cdpPort is the default Chromium remote-debugging port when the gateway does
// not publish a cdp endpoint (legacy / unit-test fallback).
const cdpPort = 9222

// vncWSport is the default websockify port when the gateway does not publish
// a novnc endpoint.
const vncWSport = 6080

var (
	// ErrCapacity is returned when the global tab cap is reached and no tab can
	// be evicted to make room.
	ErrCapacity = errors.New("preview capacity reached")
	// ErrInvalidMaxTabs is returned when SetMaxTabs is called with n < 1.
	ErrInvalidMaxTabs = errors.New("max tabs must be at least 1")
	// ErrInternalEndpointMissing is returned when a gateway resolver is present
	// but named cdp/novnc are absent or still point at the external publish
	// surface. Callers must not dial fallbackIP:9222/6080.
	ErrInternalEndpointMissing = errors.New("sandbox internal cdp/novnc endpoint missing")
)

// StatsSnapshot is a point-in-time view of the remote preview tab pool.
type StatsSnapshot struct {
	TabCount       int `json:"tabCount"`
	MaxTabs        int `json:"maxTabs"`
	ContainerCount int `json:"containerCount"`
}

// containerState is a cached CDP attachment to one sandbox's in-container
// Chromium (the sandbox owns its lifecycle; we only connect/disconnect).
type containerState struct {
	name      string
	ip        string // host portion used for dials (may be LB IP)
	cdpAddr   string // "host:port" for CDP (empty → ip:cdpPort)
	novncAddr string // "host:port" for websockify (empty → ip:vncWSport)
	engine    Engine
}

// Service manages the per-viewer preview tabs opened inside app_preview
// sandboxes. It dials each sandbox's in-container CDP/websockify; it never
// creates or destroys containers (the sandbox-gateway owns that). Safe for
// concurrent use.
type Service struct {
	sbx  SandboxExecer
	cfg  Config
	dial engineDialer

	// readyProbe reports whether a sandbox IP already exposes CDP+websockify.
	// Overridable in tests; defaults to probeVNCReady.
	readyProbe func(ctx context.Context, ip string) bool

	mu         sync.Mutex
	reg        *tabRegistry
	containers map[string]*containerState
	sessions   map[string]*Session

	stop     chan struct{}
	stopOnce sync.Once
}

// New builds a Service. sbx is the sandbox manager (used only to start the VNC
// stack inside a sandbox over SSH via Exec).
func New(sbx SandboxExecer, cfg Config) *Service {
	s := &Service{
		sbx:        sbx,
		cfg:        cfg,
		dial:       dialRod,
		reg:        newTabRegistry(cfg.MaxTabs, cfg.MaxTabsPerContainer),
		containers: map[string]*containerState{},
		sessions:   map[string]*Session{},
		stop:       make(chan struct{}),
	}
	s.readyProbe = s.probeVNCReady
	return s
}

// Session is one viewer's isolated tab.
type Session struct {
	ID        string
	container string
	page      Page
	svc       *Service
	done      chan struct{}
	reason    string
}

// Page exposes the underlying tab controller for the WS handler.
func (se *Session) Page() Page { return se.page }

// Done is closed when the session is torn down (by the client, idle sweep, or
// LRU eviction). Reason explains why.
func (se *Session) Done() <-chan struct{} { return se.done }

// Reason returns why the session ended ("closed" | "idle" | "evicted").
func (se *Session) Reason() string { return se.reason }

// Touch marks the session active (defers idle reclamation).
func (se *Session) Touch() { se.svc.touch(se.ID) }

// Close tears the session down.
func (se *Session) Close() { se.svc.CloseSession(se.ID) }

// VNCWebSocketURL returns the container-local websockify endpoint for noVNC.
func (se *Session) VNCWebSocketURL() (string, error) {
	se.svc.mu.Lock()
	cs := se.svc.containers[se.container]
	se.svc.mu.Unlock()
	if cs == nil {
		return "", fmt.Errorf("vnc container %s missing", se.container)
	}
	host, port := cs.novncHostPort()
	if host == "" {
		return "", fmt.Errorf("vnc container %s missing", se.container)
	}
	return fmt.Sprintf("ws://%s:%d", host, port), nil
}

func (cs *containerState) novncHostPort() (string, int) {
	if host, port := splitHostPort(cs.novncAddr); host != "" && port > 0 {
		return host, port
	}
	return cs.ip, vncWSport
}

func splitHostPort(addr string) (string, int) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// bare host / IPv6 without port
		return addr, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

// resolvePreviewEndpoints prefers gateway-published cdp/novnc addresses.
// Without a resolver (unit tests / legacy), it falls back to sandboxIP:9222/6080.
// With a resolver, missing or publish-surface addresses fail closed — never
// dial LB_IP/bindIP:9222/6080.
func (s *Service) resolvePreviewEndpoints(ctx context.Context, sandboxName, fallbackIP string) (cdpAddr, novncAddr string, err error) {
	r, ok := s.sbx.(SandboxEndpointResolver)
	if !ok || r == nil {
		return fmt.Sprintf("%s:%d", fallbackIP, cdpPort), fmt.Sprintf("%s:%d", fallbackIP, vncWSport), nil
	}
	cdpRaw, cdpErr := r.EndpointAddr(ctx, sandboxName, "cdp")
	if cdpErr != nil || strings.TrimSpace(cdpRaw) == "" {
		return "", "", fmt.Errorf("%w: cdp: %v", ErrInternalEndpointMissing, cdpErr)
	}
	novncRaw, novncErr := r.EndpointAddr(ctx, sandboxName, "novnc")
	if novncErr != nil || strings.TrimSpace(novncRaw) == "" {
		return "", "", fmt.Errorf("%w: novnc: %v", ErrInternalEndpointMissing, novncErr)
	}
	cdpAddr = dialableEndpoint(strings.TrimSpace(cdpRaw))
	novncAddr = dialableEndpoint(strings.TrimSpace(novncRaw))
	if unsafeInternalEndpoint(cdpAddr, fallbackIP) || unsafeInternalEndpoint(novncAddr, fallbackIP) {
		log.Error().
			Str("sandbox", sandboxName).
			Str("fallback_ip", fallbackIP).
			Str("cdp", cdpAddr).
			Str("novnc", novncAddr).
			Msg("refusing to dial CDP/noVNC on publish surface")
		return "", "", fmt.Errorf("%w: still on publish surface (fallbackIP=%s cdp=%s novnc=%s)", ErrInternalEndpointMissing, fallbackIP, cdpAddr, novncAddr)
	}
	return cdpAddr, novncAddr, nil
}

func unsafeInternalEndpoint(addr, fallbackIP string) bool {
	host, _ := splitHostPort(addr)
	if host == "" {
		return true
	}
	if strings.Contains(host, ".svc.cluster.local") {
		return false
	}
	fb := strings.TrimSpace(fallbackIP)
	if fb == "" || host != fb {
		return false
	}
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return false
	}
	return true
}

// dialableEndpoint rewrites 0.0.0.0 / :: publish hosts to loopback so local
// probes and WebSocket dials reach docker-proxy on this machine.
func dialableEndpoint(addr string) string {
	host, port := splitHostPort(addr)
	if host == "" {
		return addr
	}
	if host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
		if port > 0 {
			return net.JoinHostPort(host, strconv.Itoa(port))
		}
		return host
	}
	return addr
}

// SetReadyProbe overrides the CDP/websockify readiness check (tests / custom
// health logic). Passing nil restores the default probeVNCReady.
func (s *Service) SetReadyProbe(fn func(ctx context.Context, ip string) bool) {
	if fn == nil {
		s.readyProbe = s.probeVNCReady
		return
	}
	s.readyProbe = fn
}

// Start launches the background sweeper (idle tabs). In-sandbox VNC needs no
// image pre-pull — the sandbox image already contains the stack.
func (s *Service) Start() {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				s.sweep()
			}
		}
	}()
}

// Stop halts the sweeper and tears sessions down. Sandbox engines are only
// disconnected; their containers are left to the sandbox lifecycle.
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	for _, id := range ids {
		s.closeLocked(id, "shutdown")
	}
	for _, cs := range s.containers {
		_ = cs.engine.Close()
	}
	s.containers = map[string]*containerState{}
}

// OpenInSandbox attaches to the VNC/CDP stack running inside an app_preview
// sandbox and opens an isolated tab navigated to targetURL (typically
// http://127.0.0.1:<port>/ so Chromium stays inside the sandbox network namespace).
// sandboxIP is only used when no SandboxEndpointResolver is present (legacy /
// unit tests). With a gateway resolver, named internal cdp/novnc are required.
func (s *Service) OpenInSandbox(ctx context.Context, sandboxName, sandboxIP, targetURL string) (*Session, error) {
	if sandboxName == "" || sandboxIP == "" {
		return nil, fmt.Errorf("sandbox name/ip required")
	}
	cdpAddr, novncAddr, err := s.resolvePreviewEndpoints(ctx, sandboxName, sandboxIP)
	if err != nil {
		return nil, err
	}
	if err := s.EnsureSandboxVNC(ctx, sandboxName, sandboxIP); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.supersedeContainerLocked(sandboxName)

	if s.reg.full() {
		if id, ok := s.reg.lru(); ok {
			s.closeLocked(id, "evicted")
		}
		if s.reg.full() {
			return nil, ErrCapacity
		}
	}

	return s.openTabInSandboxLocked(ctx, sandboxName, sandboxIP, cdpAddr, novncAddr, targetURL)
}

// supersedeContainerLocked closes existing sessions in sandboxName when the
// per-container cap is reached (later connection wins). Caller holds s.mu.
func (s *Service) supersedeContainerLocked(sandboxName string) {
	if s.reg.containerCount(sandboxName) < s.reg.maxPerContainer {
		return
	}
	for id, sess := range s.sessions {
		if sess.container != sandboxName {
			continue
		}
		log.Info().
			Str("reason", "superseded").
			Str("old_session", id).
			Str("sandbox", sandboxName).
			Str("container", sandboxName).
			Msg("sandbox preview session superseded")
		s.closeLocked(id, "superseded")
	}
}

// openTabInSandboxLocked dials CDP, opens a tab, and registers the session.
// On NewTab failure the cached engine is evicted and redialed once. Caller
// holds s.mu.
func (s *Service) openTabInSandboxLocked(ctx context.Context, sandboxName, sandboxIP, cdpAddr, novncAddr, targetURL string) (*Session, error) {
	cs, err := s.attachSandboxLocked(ctx, sandboxName, sandboxIP, cdpAddr, novncAddr)
	if err != nil {
		return nil, err
	}
	page, err := cs.engine.NewTab(ctx, targetURL)
	if err != nil {
		log.Warn().Str("sandbox", sandboxName).Err(err).Msg("cdp engine evicted, redialing")
		_ = cs.engine.Close()
		delete(s.containers, sandboxName)
		cs, err = s.attachSandboxLocked(ctx, sandboxName, sandboxIP, cdpAddr, novncAddr)
		if err != nil {
			return nil, err
		}
		page, err = cs.engine.NewTab(ctx, targetURL)
		if err != nil {
			return nil, fmt.Errorf("open tab: %w", err)
		}
	}
	sess := &Session{ID: uuid.NewString(), container: sandboxName, page: page, svc: s, done: make(chan struct{})}
	s.sessions[sess.ID] = sess
	s.reg.add(sess.ID, sandboxName)
	log.Info().Str("session", sess.ID).Str("sandbox", sandboxName).Str("url", targetURL).Int("tabs", s.reg.count()).Msg("sandbox preview tab opened")
	return sess, nil
}

// EnsureSandboxVNC starts /usr/local/bin/vnc-preview.sh inside the sandbox (over
// SSH) when CDP/websockify are not yet reachable (idempotent).
func (s *Service) EnsureSandboxVNC(ctx context.Context, sandboxName, sandboxIP string) error {
	cdpAddr, novncAddr, err := s.resolvePreviewEndpoints(ctx, sandboxName, sandboxIP)
	if err != nil {
		return err
	}
	_, hasResolver := s.sbx.(SandboxEndpointResolver)
	checkReady := func() bool {
		// Prefer gateway-published cdp/novnc when the manager can resolve them.
		if hasResolver {
			return s.probeVNCReadyAddrs(ctx, cdpAddr, novncAddr)
		}
		if s.readyProbe != nil {
			return s.readyProbe(ctx, sandboxIP)
		}
		return s.probeVNCReady(ctx, sandboxIP)
	}
	if checkReady() {
		return nil
	}
	if _, err := s.sbx.Exec(ctx, sandboxName, 90*time.Second, "/usr/local/bin/vnc-preview.sh"); err != nil {
		log.Warn().Str("sandbox", sandboxName).Err(err).Msg("vnc-preview.sh exec failed; probing anyway")
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if checkReady() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Warn().Str("sandbox", sandboxName).Msg("vnc-preview health check failed")
	return fmt.Errorf("未启动浏览器组件: sandbox %s VNC/CDP not ready (need VNC_PREVIEW=1)", sandboxName)
}

func (s *Service) probeVNCReady(ctx context.Context, ip string) bool {
	return s.probeVNCReadyAddrs(ctx, fmt.Sprintf("%s:%d", ip, cdpPort), fmt.Sprintf("%s:%d", ip, vncWSport))
}

func (s *Service) probeVNCReadyAddrs(ctx context.Context, cdpAddr, novncAddr string) bool {
	cdpHost, cdpP := splitHostPort(cdpAddr)
	if cdpHost == "" {
		return false
	}
	if cdpP <= 0 {
		cdpP = cdpPort
	}
	novncHost, novncP := splitHostPort(novncAddr)
	if novncHost == "" {
		novncHost = cdpHost
	}
	if novncP <= 0 {
		novncP = vncWSport
	}
	verCtx, verCancel := context.WithTimeout(ctx, 2*time.Second)
	defer verCancel()
	req, err := http.NewRequestWithContext(verCtx, http.MethodGet, fmt.Sprintf("http://%s:%d/json/version", cdpHost, cdpP), nil)
	if err != nil {
		return false
	}
	// Chrome DevTools HTTP rejects non-IP/non-localhost Host on some endpoints;
	// keep dialing the ClusterIP DNS while advertising a loopback Host.
	setCDPRequestHost(req, cdpP)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(novncHost, strconv.Itoa(novncP)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	tabCtx, tabCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tabCancel()
	if !s.probeTabCreate(tabCtx, cdpHost, cdpP) {
		log.Warn().Str("cdp", cdpAddr).Msg("vnc-preview tab probe failed")
		return false
	}
	return true
}

// setCDPRequestHost forces Host to 127.0.0.1:<port> so Chromium accepts
// /json/new (and related DevTools HTTP) when Approving dials via Service DNS
// (sbx-*.svc.cluster.local). TCP still targets req.URL.Host.
func setCDPRequestHost(req *http.Request, port int) {
	if req == nil || port <= 0 {
		return
	}
	req.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

// probeTabCreate verifies Chromium can still create a tab (catches zombie CDP).
func (s *Service) probeTabCreate(ctx context.Context, host string, port int) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("http://%s:%d/json/new?about:blank", host, port), nil)
	if err != nil {
		return false
	}
	setCDPRequestHost(req, port)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	var tab struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &tab); err != nil || tab.ID == "" {
		return false
	}
	closeReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("http://%s:%d/json/close/%s", host, port, tab.ID), nil)
	if err != nil {
		return false
	}
	setCDPRequestHost(closeReq, port)
	closeResp, err := http.DefaultClient.Do(closeReq)
	if err != nil {
		return false
	}
	_ = closeResp.Body.Close()
	return closeResp.StatusCode == http.StatusOK
}

// attachSandboxLocked dials CDP on an existing sandbox (or reuses a cached engine).
func (s *Service) attachSandboxLocked(ctx context.Context, name, ip, cdpAddr, novncAddr string) (*containerState, error) {
	if cs, ok := s.containers[name]; ok && cs.engine != nil {
		if cs.ip == "" {
			cs.ip = ip
		}
		if cs.cdpAddr == "" {
			cs.cdpAddr = cdpAddr
		}
		if cs.novncAddr == "" {
			cs.novncAddr = novncAddr
		}
		return cs, nil
	}
	host, port := splitHostPort(cdpAddr)
	if host == "" {
		host = ip
	}
	if port <= 0 {
		port = cdpPort
	}
	eng, err := s.dial(ctx, fmt.Sprintf("http://%s:%d", host, port))
	if err != nil {
		return nil, fmt.Errorf("connect sandbox cdp: %w", err)
	}
	cs := &containerState{name: name, ip: ip, cdpAddr: cdpAddr, novncAddr: novncAddr, engine: eng}
	s.containers[name] = cs
	return cs, nil
}

// Stats returns current tab-pool occupancy. Safe for concurrent use.
func (s *Service) Stats() StatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StatsSnapshot{
		TabCount:       s.reg.count(),
		MaxTabs:        s.reg.maxTabs,
		ContainerCount: len(s.containers),
	}
}

// SetMaxTabs updates the global tab cap at runtime. Shrinking does not evict
// existing sessions; LRU applies only when new sessions open while at capacity.
func (s *Service) SetMaxTabs(n int) error {
	if n < 1 {
		return ErrInvalidMaxTabs
	}
	s.mu.Lock()
	s.reg.maxTabs = n
	s.mu.Unlock()
	return nil
}

// CloseSession tears down one session.
func (s *Service) CloseSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked(id, "closed")
}

func (s *Service) touch(id string) {
	s.mu.Lock()
	s.reg.touch(id)
	s.mu.Unlock()
}

// closeLocked closes a session's page and clears accounting. Caller holds s.mu.
func (s *Service) closeLocked(id, reason string) {
	sess, ok := s.sessions[id]
	if !ok {
		return
	}
	sess.reason = reason
	_ = sess.page.Close()
	delete(s.sessions, id)
	s.reg.remove(id)
	close(sess.done)
}

// sweep frees idle tabs and disconnects cached CDP engines for sandboxes that
// have held zero tabs beyond the TTL. It never destroys containers — the
// sandbox-gateway owns their lifecycle.
func (s *Service) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.reg.idle(s.cfg.TabIdleTTL) {
		s.closeLocked(id, "idle")
	}
	for _, name := range s.reg.reapableContainers(s.cfg.ContainerIdleTTL) {
		if cs, ok := s.containers[name]; ok {
			_ = cs.engine.Close()
			delete(s.containers, name)
		}
		s.reg.forgetContainer(name)
	}
}
