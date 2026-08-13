package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// PreviewPort is a registered preview endpoint for an app_preview node.
type PreviewPort struct {
	RunID       string `json:"runId"`
	NodeID      string `json:"nodeId"`
	Port        int    `json:"port"`
	Label       string `json:"label,omitempty"`
	ProxyURL    string `json:"proxyUrl"`
	SandboxName string `json:"-"`
	// Host is the resolved upstream base the proxy dials (e.g.
	// "http://172.17.0.5:9090"), persisted so the proxy needn't re-resolve the
	// container IP through the sandbox manager on every request.
	Host string `json:"-"`
	// Mode is "direct" when the node switch direct_preview is on; empty/vnc otherwise.
	Mode string `json:"mode,omitempty"`
	// DirectURL is the browser-facing http://IP:port/ when Mode=direct.
	DirectURL string `json:"directUrl,omitempty"`
	// Healthy is true when ProbeHTTPPort succeeded at registration time.
	// set_preview only succeeds when Healthy; HasHealthyPreviewPorts gates
	// production-phase completion / early park into review.
	Healthy bool `json:"healthy"`
	// KeepalivePID is the setsid-detached listener pid (0 when unknown).
	KeepalivePID int       `json:"keepalivePid,omitempty"`
	RegisteredAt time.Time `json:"registeredAt"`
}

// PreviewStore persists preview port registrations.
type PreviewStore interface {
	UpsertPreviewPort(rec PreviewPort) error
	ListPreviewPorts(runID, nodeID string) ([]PreviewPort, error)
	GetPreviewPort(runID, nodeID string, port int) (*PreviewPort, bool)
	UpdatePreviewHealth(runID, nodeID string, port int, healthy bool) error
}

// PreviewSandboxOps resolves run sandboxes and probes in-container HTTP ports.
type PreviewSandboxOps interface {
	SandboxForRunNode(runID, nodeID string) (name string, ok bool)
	ProbeHTTPPort(ctx context.Context, sandboxName string, port int) bool
	// KeepalivePort detaches the listener (setsid + pidfile + log) and returns
	// the keepalive pid (0 when unknown / no-op).
	KeepalivePort(ctx context.Context, sandboxName string, port int) (pid int, err error)
	// PreviewUpstream resolves the reachable base URL (http://host:port) for an
	// in-sandbox app port via the gateway, so the registration can persist an
	// upstream host for the proxy to dial.
	PreviewUpstream(ctx context.Context, sandboxName string, port int) (string, bool)
}

// previewDirectChecker is optionally implemented by PreviewSandboxOps.
type previewDirectChecker interface {
	DirectPreview(runID, nodeID string) bool
}

// previewPortPublisher is optionally implemented to PATCH k8s Services for unpublished ports.
type previewPortPublisher interface {
	EnsurePublishedPort(ctx context.Context, sandboxName string, port int) (string, bool)
}

// PreviewVNCWarmer optionally warms the in-sandbox VNC stack after set_preview.
type PreviewVNCWarmer interface {
	WarmPreviewVNC(sandboxName string)
}

func previewKey(runID, nodeID string) string { return runID + "|" + nodeID }

// SetPreviewBaseURL sets the browser-facing base URL used to build proxy links.
func (h *Host) SetPreviewBaseURL(base string) {
	h.mu.Lock()
	h.previewBase = strings.TrimRight(strings.TrimSpace(base), "/")
	h.mu.Unlock()
}

// SetPreviewStore wires DB persistence for preview ports.
func (h *Host) SetPreviewStore(s PreviewStore) { h.previewStore = s }

// SetPreviewSandboxOps wires sandbox lookup and health probes.
func (h *Host) SetPreviewSandboxOps(ops PreviewSandboxOps) { h.previewOps = ops }

func (h *Host) previewProxyURL(runID, nodeID string, port int) string {
	return fmt.Sprintf("/preview/%s/%s/%d/", runID, nodeID, port)
}

// HasPreviewPorts reports whether at least one healthy preview port is registered.
// Unreachable registrations do not count (set_preview fails closed on probe).
func (h *Host) HasPreviewPorts(runID, nodeID string) bool {
	return h.HasHealthyPreviewPorts(runID, nodeID)
}

// HasHealthyPreviewPorts reports whether at least one registered port is Healthy.
func (h *Host) HasHealthyPreviewPorts(runID, nodeID string) bool {
	for _, p := range h.ListPreviewPorts(runID, nodeID) {
		if p.Healthy {
			return true
		}
	}
	return false
}

// ListPreviewKeepalivePIDs returns setsid-detached preview pids for whitelist
// during Cancel/Abort session cleanup (sandbox Destroy still reclaims all).
func (h *Host) ListPreviewKeepalivePIDs(runID, nodeID string) []int {
	ports := h.ListPreviewPorts(runID, nodeID)
	out := make([]int, 0, len(ports))
	seen := map[int]bool{}
	for _, p := range ports {
		if p.KeepalivePID > 0 && !seen[p.KeepalivePID] {
			seen[p.KeepalivePID] = true
			out = append(out, p.KeepalivePID)
		}
	}
	return out
}

// IsPreviewKeepalivePID reports whether pid is a registered preview keepalive
// pid for this run (any node). Used to skip killing preview processes when
// ending an Agent/ACP session without destroying the sandbox.
func (h *Host) IsPreviewKeepalivePID(runID string, pid int) bool {
	if h == nil || pid <= 0 || runID == "" {
		return false
	}
	h.mu.RLock()
	mem := h.previewMem
	h.mu.RUnlock()
	for key, ports := range mem {
		if !strings.HasPrefix(key, runID+"|") {
			continue
		}
		for _, p := range ports {
			if p.KeepalivePID == pid {
				return true
			}
		}
	}
	// Also check DB-backed ports for this run (best-effort via store list is
	// per-node; callers with nodeID should use ListPreviewKeepalivePIDs).
	_ = mem
	return false
}

// SignalPreviewReady marks that a healthy set_preview completed for run/node
// and wakes any WaitPreviewReady / early-finish watchers in the provider.
func (h *Host) SignalPreviewReady(runID, nodeID string) {
	if h == nil || runID == "" || nodeID == "" {
		return
	}
	key := previewKey(runID, nodeID)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.previewReady == nil {
		h.previewReady = map[string]chan struct{}{}
	}
	ch, ok := h.previewReady[key]
	if !ok {
		ch = make(chan struct{})
		h.previewReady[key] = ch
		close(ch)
		return
	}
	select {
	case <-ch:
		// already signaled
	default:
		close(ch)
	}
}

// PreviewReadyChan returns a channel closed when a healthy set_preview lands
// for this run/node. The channel may already be closed.
func (h *Host) PreviewReadyChan(runID, nodeID string) <-chan struct{} {
	key := previewKey(runID, nodeID)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.previewReady == nil {
		h.previewReady = map[string]chan struct{}{}
	}
	ch, ok := h.previewReady[key]
	if !ok {
		ch = make(chan struct{})
		h.previewReady[key] = ch
	}
	return ch
}

// ResetPreviewReady clears the ready signal for a fresh production attempt
// (e.g. nodeReq / ClearOutcome path). Idempotent.
func (h *Host) ResetPreviewReady(runID, nodeID string) {
	if h == nil {
		return
	}
	key := previewKey(runID, nodeID)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.previewReady == nil {
		return
	}
	delete(h.previewReady, key)
}

// ListPreviewPorts returns all preview ports for a run/node (memory + DB).
func (h *Host) ListPreviewPorts(runID, nodeID string) []PreviewPort {
	h.mu.RLock()
	mem := append([]PreviewPort(nil), h.previewMem[previewKey(runID, nodeID)]...)
	store := h.previewStore
	h.mu.RUnlock()
	if store == nil {
		return h.annotatePreviewTransport(runID, nodeID, mem)
	}
	dbPorts, err := store.ListPreviewPorts(runID, nodeID)
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("node_id", nodeID).
			Msg("list preview ports from store failed; using memory")
		return h.annotatePreviewTransport(runID, nodeID, mem)
	}
	if len(dbPorts) == 0 {
		return h.annotatePreviewTransport(runID, nodeID, mem)
	}
	if len(mem) == 0 {
		return h.annotatePreviewTransport(runID, nodeID, dbPorts)
	}
	byPort := map[int]PreviewPort{}
	for _, p := range dbPorts {
		byPort[p.Port] = p
	}
	for _, p := range mem {
		byPort[p.Port] = p
	}
	out := make([]PreviewPort, 0, len(byPort))
	for _, p := range byPort {
		out = append(out, p)
	}
	return h.annotatePreviewTransport(runID, nodeID, out)
}

func (h *Host) setPreviewPort(runID, nodeID string, port int, label string) (string, error) {
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("port must be 1-65535")
	}
	h.mu.RLock()
	ops := h.previewOps
	store := h.previewStore
	h.mu.RUnlock()
	sandboxName := ""
	if ops != nil {
		if name, ok := ops.SandboxForRunNode(runID, nodeID); ok {
			sandboxName = name
		}
	}
	if ops == nil || sandboxName == "" {
		return "", fmt.Errorf("无法探测预览端口:沙箱未就绪")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	direct := h.previewDirect(runID, nodeID)
	if direct {
		if pub, ok := ops.(previewPortPublisher); ok {
			_, _ = pub.EnsurePublishedPort(ctx, sandboxName, port)
		}
	}
	keepalivePID, err := ops.KeepalivePort(ctx, sandboxName, port)
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("node_id", nodeID).
			Int("port", port).Msg("preview keepalive failed")
		return "", fmt.Errorf("预览保活脱钩失败: %w", err)
	}
	healthy := ops.ProbeHTTPPort(ctx, sandboxName, port)
	if !healthy {
		return "", fmt.Errorf("预览端口 %d 不可达(须监听 0.0.0.0 且服务已启动);修复后可重试 set_preview", port)
	}
	host := ""
	if base, ok := ops.PreviewUpstream(ctx, sandboxName, port); ok && base != "" {
		host = base
	}
	proxyURL := h.previewProxyURL(runID, nodeID, port)
	rec := PreviewPort{
		RunID: runID, NodeID: nodeID, Port: port, Label: strings.TrimSpace(label),
		ProxyURL: proxyURL, SandboxName: sandboxName, Host: host, Healthy: true,
		KeepalivePID: keepalivePID, RegisteredAt: time.Now(),
	}
	h.mu.Lock()
	key := previewKey(runID, nodeID)
	if h.previewMem == nil {
		h.previewMem = map[string][]PreviewPort{}
	}
	ports := h.previewMem[key]
	found := false
	for i, p := range ports {
		if p.Port == port {
			ports[i] = rec
			found = true
			break
		}
	}
	if !found {
		ports = append(ports, rec)
	}
	h.previewMem[key] = ports
	h.mu.Unlock()
	if store != nil {
		if err := store.UpsertPreviewPort(rec); err != nil {
			return "", err
		}
	}
	if !direct {
		if warmer, ok := ops.(PreviewVNCWarmer); ok && sandboxName != "" {
			warmer.WarmPreviewVNC(sandboxName)
		}
	}
	h.SignalPreviewReady(runID, nodeID)
	return proxyURL, nil
}

// PutPreviewPortForTest seeds a memory-only healthy preview port without sandbox ops.
// Used by engine unit tests that exercise app_preview pause paths offline.
func (h *Host) PutPreviewPortForTest(runID, nodeID string, port int, label string) {
	if h == nil || port <= 0 {
		return
	}
	rec := PreviewPort{
		RunID: runID, NodeID: nodeID, Port: port, Label: strings.TrimSpace(label),
		ProxyURL: h.previewProxyURL(runID, nodeID, port), Healthy: true,
		// Non-zero sentinel so ListPreviewKeepalivePIDs / AbortRun whitelist
		// paths exercise the same registration shape as real set_preview.
		KeepalivePID: 1, RegisteredAt: time.Now(),
	}
	h.mu.Lock()
	key := previewKey(runID, nodeID)
	if h.previewMem == nil {
		h.previewMem = map[string][]PreviewPort{}
	}
	h.previewMem[key] = append(h.previewMem[key], rec)
	h.mu.Unlock()
	h.SignalPreviewReady(runID, nodeID)
}

func parsePreviewPort(v any) (int, error) {
	switch t := v.(type) {
	case float64:
		return int(t), nil
	case int:
		return t, nil
	case int64:
		return int(t), nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0, fmt.Errorf("invalid port")
		}
		return n, nil
	default:
		return 0, fmt.Errorf("port is required")
	}
}

func (h *Host) previewDirect(runID, nodeID string) bool {
	h.mu.RLock()
	ops := h.previewOps
	h.mu.RUnlock()
	c, ok := ops.(previewDirectChecker)
	return ok && c.DirectPreview(runID, nodeID)
}

func (h *Host) annotatePreviewTransport(runID, nodeID string, ports []PreviewPort) []PreviewPort {
	if !h.previewDirect(runID, nodeID) {
		return ports
	}
	out := make([]PreviewPort, len(ports))
	copy(out, ports)
	for i := range out {
		out[i].Mode = "direct"
		if out[i].Host != "" {
			out[i].DirectURL = strings.TrimRight(out[i].Host, "/") + "/"
		}
	}
	return out
}
