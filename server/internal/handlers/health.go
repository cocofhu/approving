package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cocofhu/approving/internal/nodereg"
	"github.com/cocofhu/approving/internal/services"
	"github.com/cocofhu/approving/internal/version"
	"github.com/gin-gonic/gin"
)

// SandboxInject serves a short-lived ConfigHome .tgz for gateway
// config.bundleUrl / SANDBOX_INJECT. Auth is the one-shot Bearer from Create
// (not session cookie). Must stay outside /api auth middleware.
func (h *Handlers) SandboxInject(c *gin.Context) {
	if h.InjectBundles == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "inject store unavailable"})
		return
	}
	h.InjectBundles.ServeHTTP(c.Writer, c.Request, c.Param("id"))
}

// MCPRPC is the run-scoped artifact-store MCP endpoint. The in-container
// cursor-agent connects here (URL + Bearer token injected at ACP
// session/new) and calls write_artifact / read_artifact / list_artifacts.
// Streamable-HTTP: POST carries a JSON-RPC message; GET/DELETE are no-ops.
func (h *Handlers) MCPRPC(c *gin.Context) {
	runID := c.Param("runId")
	token := bearer(c.GetHeader("Authorization"))
	if h.MCP == nil || !h.MCP.AuthorizeRun(runID, token) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if c.Request.Method != http.MethodPost {

		c.Status(http.StatusOK)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body"})
		return
	}
	status, resp := h.MCP.ServeRPC(runID, token, body)
	if resp == nil {
		c.Status(status)
		return
	}
	c.Data(status, "application/json", resp)
}

// MCPUploadImage is a run-scoped HTTP entry for sandbox artifact-upload when
// tools/call upload_image_artifact is unavailable (older control planes). Same
// Bearer token as MCPRPC; hidden from tools/list like the MCP tool.
func (h *Handlers) MCPUploadImage(c *gin.Context) {
	runID := c.Param("runId")
	token := bearer(c.GetHeader("Authorization"))
	if h.MCP == nil || !h.MCP.AuthorizeRun(runID, token) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if c.Request.Method != http.MethodPost {
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body"})
		return
	}
	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	id, err := h.MCP.UploadImageArtifact(runID, token, h.MCP.ActiveNode(runID), req.Name, req.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": strings.TrimSpace(req.Name), "id": id})
}

func bearer(h string) string {
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(h)
}

// Health is the readiness probe. During shutdown it returns 503 with grace info.
func (h *Handlers) Health(c *gin.Context) {
	if h.Shutdown != nil && h.Shutdown.IsDraining() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":                  "shutting_down",
			"ready":                   false,
			"message":                 "服务正在关闭，不接受新请求",
			"grace_remaining_seconds": h.Shutdown.GraceRemainingSeconds(),
		})
		return
	}
	body := gin.H{"status": "ok", "ready": true, "vnc_preview": h.Browser != nil}
	if sha := version.ShortSHA(); sha != "" {
		body["commit"] = sha
	}
	c.JSON(http.StatusOK, body)
}

// Live is the liveness probe: always 200 while the process is up.
func (h *Handlers) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "alive"})
}

// NodeRegistry returns the structured-product contract manifest (single source
// of truth for backend + frontend artifact mappings).
func (h *Handlers) NodeRegistry(c *gin.Context) {
	c.JSON(http.StatusOK, nodereg.BuildManifest())
}

// DashboardStats returns summary counters.
func (h *Handlers) DashboardStats(c *gin.Context) {
	c.JSON(http.StatusOK, h.Dash.Compute())
}

// PlatformStatus returns AppTopbar StatusMetrics (Auth-protected via /api group).
// Query: timezone=IANA (preferred), utcOffsetMinutes=int (fallback, east of UTC positive).
func (h *Handlers) PlatformStatus(c *gin.Context) {
	if h.Dash == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "dashboard unavailable"})
		return
	}
	q := services.PlatformStatusQuery{
		Timezone: c.Query("timezone"),
	}
	if raw := strings.TrimSpace(c.Query("utcOffsetMinutes")); raw != "" {
		mins, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid utcOffsetMinutes"})
			return
		}
		q.UTCOffsetMinutes = &mins
	}
	st, err := h.Dash.PlatformStatus(c.Request.Context(), q)
	if err != nil {
		if errors.Is(err, services.ErrInvalidTokenStatsTimezone) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, st)
}
