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
	Host         string    `json:"-"`
	Healthy      bool      `json:"healthy"`
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
	KeepalivePort(ctx context.Context, sandboxName string, port int) error
	// PreviewUpstream resolves the reachable base URL (http://host:port) for an
	// in-sandbox app port via the gateway, so the registration can persist an
	// upstream host for the proxy to dial.
	PreviewUpstream(ctx context.Context, sandboxName string, port int) (string, bool)
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

// HasPreviewPorts reports whether at least one preview port is registered.
func (h *Host) HasPreviewPorts(runID, nodeID string) bool {
	return len(h.ListPreviewPorts(runID, nodeID)) > 0
}

// ListPreviewPorts returns all preview ports for a run/node (memory + DB).
func (h *Host) ListPreviewPorts(runID, nodeID string) []PreviewPort {
	h.mu.RLock()
	mem := append([]PreviewPort(nil), h.previewMem[previewKey(runID, nodeID)]...)
	store := h.previewStore
	h.mu.RUnlock()
	if store == nil {
		return mem
	}
	dbPorts, err := store.ListPreviewPorts(runID, nodeID)
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("node_id", nodeID).
			Msg("list preview ports from store failed; using memory")
		return mem
	}
	if len(dbPorts) == 0 {
		return mem
	}
	if len(mem) == 0 {
		return dbPorts
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
	return out
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
	healthy := false
	host := ""
	if ops != nil && sandboxName != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		if err := ops.KeepalivePort(ctx, sandboxName, port); err != nil {
			log.Warn().Err(err).Str("run_id", runID).Str("node_id", nodeID).
				Int("port", port).Msg("preview keepalive failed")
		}
		healthy = ops.ProbeHTTPPort(ctx, sandboxName, port)
		if base, ok := ops.PreviewUpstream(ctx, sandboxName, port); ok && base != "" {
			host = base
		}
		cancel()
	}
	proxyURL := h.previewProxyURL(runID, nodeID, port)
	rec := PreviewPort{
		RunID: runID, NodeID: nodeID, Port: port, Label: strings.TrimSpace(label),
		ProxyURL: proxyURL, SandboxName: sandboxName, Host: host, Healthy: healthy, RegisteredAt: time.Now(),
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
	// Warm in-sandbox VNC early so PreviewVNC can attach without a cold start.
	if warmer, ok := ops.(PreviewVNCWarmer); ok && sandboxName != "" {
		warmer.WarmPreviewVNC(sandboxName)
	}
	return proxyURL, nil
}

// PutPreviewPortForTest seeds a memory-only preview port without sandbox ops.
// Used by engine unit tests that exercise app_preview pause paths offline.
func (h *Host) PutPreviewPortForTest(runID, nodeID string, port int, label string) {
	if h == nil || port <= 0 {
		return
	}
	rec := PreviewPort{
		RunID: runID, NodeID: nodeID, Port: port, Label: strings.TrimSpace(label),
		ProxyURL: h.previewProxyURL(runID, nodeID, port), Healthy: true, RegisteredAt: time.Now(),
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	key := previewKey(runID, nodeID)
	if h.previewMem == nil {
		h.previewMem = map[string][]PreviewPort{}
	}
	h.previewMem[key] = append(h.previewMem[key], rec)
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
