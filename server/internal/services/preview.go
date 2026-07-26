package services

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/browser"
	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sandbox"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// PreviewService persists preview port registrations and implements sandbox ops.
type PreviewService struct {
	db      *gorm.DB
	mgr     *sandbox.Manager
	browser *browser.Service
}

// NewPreviewService builds a preview store backed by the platform DB.
func NewPreviewService(db *gorm.DB, mgr *sandbox.Manager) *PreviewService {
	return &PreviewService{db: db, mgr: mgr}
}

// SetBrowser wires the in-sandbox VNC helper used to warm CDP/websockify when
// set_preview registers a port (before a reviewer opens the noVNC tab).
func (s *PreviewService) SetBrowser(b *browser.Service) { s.browser = b }

// EnsurePreviewVNC idempotently starts the sandbox VNC stack (best-effort).
func (s *PreviewService) EnsurePreviewVNC(ctx context.Context, sandboxName string) error {
	if s.browser == nil || sandboxName == "" {
		return nil
	}
	ip, err := s.ContainerIP(ctx, sandboxName)
	if err != nil || ip == "" {
		return err
	}
	return s.browser.EnsureSandboxVNC(ctx, sandboxName, ip)
}

func (s *PreviewService) UpsertPreviewPort(rec mcp.PreviewPort) error {
	row := models.RunPreviewPort{
		RunID: rec.RunID, NodeID: rec.NodeID, Port: rec.Port, Label: rec.Label,
		ProxyURL: rec.ProxyURL, SandboxName: rec.SandboxName, Host: rec.Host, Healthy: rec.Healthy,
		RegisteredAt: rec.RegisteredAt,
	}
	var existing models.RunPreviewPort
	err := s.db.Where("run_id = ? AND node_id = ? AND port = ?", rec.RunID, rec.NodeID, rec.Port).First(&existing).Error
	if err == nil {
		row.ID = existing.ID
		return s.db.Save(&row).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return s.db.Create(&row).Error
}

func (s *PreviewService) ListPreviewPorts(runID, nodeID string) ([]mcp.PreviewPort, error) {
	var rows []models.RunPreviewPort
	if err := s.db.Where("run_id = ? AND node_id = ?", runID, nodeID).Order("port asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]mcp.PreviewPort, 0, len(rows))
	for _, r := range rows {
		out = append(out, mcp.PreviewPort{
			RunID: r.RunID, NodeID: r.NodeID, Port: r.Port, Label: r.Label,
			ProxyURL: r.ProxyURL, SandboxName: r.SandboxName, Host: r.Host, Healthy: r.Healthy,
			RegisteredAt: r.RegisteredAt,
		})
	}
	return out, nil
}

func (s *PreviewService) GetPreviewPort(runID, nodeID string, port int) (*mcp.PreviewPort, bool) {
	var row models.RunPreviewPort
	if err := s.db.Where("run_id = ? AND node_id = ? AND port = ?", runID, nodeID, port).First(&row).Error; err != nil {
		return nil, false
	}
	rec := mcp.PreviewPort{
		RunID: row.RunID, NodeID: row.NodeID, Port: row.Port, Label: row.Label,
		ProxyURL: row.ProxyURL, SandboxName: row.SandboxName, Host: row.Host, Healthy: row.Healthy,
		RegisteredAt: row.RegisteredAt,
	}
	return &rec, true
}

func (s *PreviewService) UpdatePreviewHealth(runID, nodeID string, port int, healthy bool) error {
	return s.db.Model(&models.RunPreviewPort{}).
		Where("run_id = ? AND node_id = ? AND port = ?", runID, nodeID, port).
		Update("healthy", healthy).Error
}

// UpdatePreviewHost persists the resolved upstream host so subsequent proxy
// requests (or a split-out proxy service) can dial it without re-resolving the
// container IP through the sandbox manager.
func (s *PreviewService) UpdatePreviewHost(runID, nodeID string, port int, host string) error {
	return s.db.Model(&models.RunPreviewPort{}).
		Where("run_id = ? AND node_id = ? AND port = ?", runID, nodeID, port).
		Update("host", host).Error
}

// ContainerIP resolves the sandbox container's bridge IP (used at registration
// to persist an upstream host, and by the proxy to self-heal a stale host after
// a container restart).
func (s *PreviewService) ContainerIP(ctx context.Context, sandboxName string) (string, error) {
	if s.mgr == nil || sandboxName == "" {
		return "", fmt.Errorf("no sandbox manager")
	}
	return s.mgr.ContainerIP(ctx, sandboxName)
}

// SandboxForRunNode returns the docker container name for a run/node sandbox.
func (s *PreviewService) SandboxForRunNode(runID, nodeID string) (string, bool) {
	var row models.Sandbox
	if err := s.db.Where("run_id = ? AND node_id = ? AND purpose = ?", runID, nodeID, "run").
		Order("updated_at desc").First(&row).Error; err != nil {
		return "", false
	}
	if strings.TrimSpace(row.Name) == "" {
		return "", false
	}
	return row.Name, true
}

// PreviewUpstream resolves the reachable base URL (http://host:hostPort) for an
// in-sandbox app port via the gateway. The gateway publishes app ports on
// ephemeral host ports, so the platform must ask it for the address rather than
// dialing the container port directly. Empty/ok=false when unresolved.
func (s *PreviewService) PreviewUpstream(ctx context.Context, sandboxName string, port int) (string, bool) {
	if s.mgr == nil || sandboxName == "" || port <= 0 {
		return "", false
	}
	addr, err := s.mgr.HostForPort(ctx, sandboxName, port)
	if err != nil || strings.TrimSpace(addr) == "" {
		return "", false
	}
	return "http://" + addr, true
}

// ProbeHTTPPort reports whether the preview is actually reachable the way the
// reverse proxy reaches it: an HTTP request from the platform to the gateway-
// published app address. Probing that path — rather than an in-container curl
// to 127.0.0.1 — makes "healthy" reflect real reachability: an app bound only
// to loopback would refuse the proxy's connection, which would otherwise report
// healthy=true and then 502 with a blank iframe.
func (s *PreviewService) ProbeHTTPPort(ctx context.Context, sandboxName string, port int) bool {
	if s.mgr == nil || sandboxName == "" || port <= 0 {
		return false
	}
	if s.mgr.Status(ctx, sandboxName) != "running" {
		return false
	}
	base, ok := s.PreviewUpstream(ctx, sandboxName, port)
	if !ok {
		return false
	}
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	// Any HTTP response means the proxy's dial will succeed; the proxy passes
	// non-2xx through unchanged (only a failed dial becomes 502), so a reachable
	// server — whatever its status — counts as healthy.
	return true
}

// KeepalivePort best-effort re-parents a listening process so it survives ACP session end.
func (s *PreviewService) KeepalivePort(ctx context.Context, sandboxName string, port int) error {
	if s.mgr == nil || sandboxName == "" || port <= 0 {
		return nil
	}
	script := fmt.Sprintf(`pid=$(ss -tlnp 2>/dev/null | grep ':%d ' | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1); if [ -n "$pid" ]; then nohup sh -c "while kill -0 $pid 2>/dev/null; do sleep 30; done" >/dev/null 2>&1 & fi`, port)
	_, err := s.mgr.ExecScript(ctx, sandboxName, 8*time.Second, "sh", script)
	return err
}

// WarmPreviewVNC starts in-sandbox VNC asynchronously after set_preview so the
// reviewer tab can attach without waiting on cold Chromium startup.
func (s *PreviewService) WarmPreviewVNC(sandboxName string) {
	if s.browser == nil || sandboxName == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := s.EnsurePreviewVNC(ctx, sandboxName); err != nil {
			log.Debug().Str("sandbox", sandboxName).Err(err).Msg("warm preview VNC failed")
		}
	}()
}
