// Package handlers implements the REST + WS API binding services and the
// FSM engine to HTTP. Response shapes mirror the frontend types so the Vue
// app maps responses to its view models with minimal transformation.
package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/cocofhu/approving/internal/auth"
	"github.com/cocofhu/approving/internal/blob"
	"github.com/cocofhu/approving/internal/browser"
	"github.com/cocofhu/approving/internal/contextmcp"
	"github.com/cocofhu/approving/internal/engine"
	"github.com/cocofhu/approving/internal/gateshare"
	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/memorymcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
	"github.com/cocofhu/approving/internal/pmmcp"
	"github.com/cocofhu/approving/internal/sandbox"
	"github.com/cocofhu/approving/internal/schedulermcp"
	"github.com/cocofhu/approving/internal/services"
	"github.com/cocofhu/approving/internal/shutdown"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handlers bundles dependencies for route handlers.
type Handlers struct {
	WF               *services.WorkflowService
	Projects         *services.ProjectService
	Runs             *services.RunService
	Arts             *services.ArtifactService
	APIKeys          *services.APIKeyService
	Skill            *services.SkillService
	Org              *services.OrgService
	Dash             *services.DashboardService
	Sbx              *services.SandboxService
	Eng              *engine.Engine
	MCP              *mcp.Host
	Pm               *services.PmService
	PmProgress       *services.PmProgress
	PmTurns          *services.PmTurnRunner
	PMMCP            *pmmcp.Host
	MemoryMCP        *memorymcp.Host
	ContextMCP       *contextmcp.Host
	SchedulerMCP     *schedulermcp.Host
	Preview          *services.PreviewService
	Issues           *services.IssueService
	Settings         *services.SettingsService
	Shutdown         *shutdown.Coordinator
	Auth             *auth.Service
	PlatformRules    *services.PlatformRuleService
	Channels         *services.ChannelConfigService
	Browser          *browser.Service
	Audit            *services.ProjectAuditService
	Onboarding       *services.OnboardingService
	GateShare        *gateshare.Service
	GateShareNonces  *gateshare.NonceStore
	GateShareLimiter *gateshare.IPLimiter
	// PublicAdvertise is the browser-facing origin for share URLs and public
	// CSRF host checks. Never fall back to client X-Forwarded-Host.
	PublicAdvertise string
	Team            *services.TeamService
	// CanViewProjectAudit optionally overrides the default audit ACL
	// (is_admin OR authenticated user who can UpdateProject). Tests use this
	// to simulate a read-only member denial while production keeps the hook nil.
	CanViewProjectAudit func(username, projectID string) bool
	// InjectBundles serves ConfigHome .tgz for gateway SANDBOX_INJECT (no session auth).
	InjectBundles *sandbox.BundleStore
	// Blobs serves externalized attachment bytes (GET /api/blobs/:id).
	Blobs          blob.Store
	doctorMu       sync.Mutex
	doctorSessions map[string]doctorArtifactSession
}

// GetSettings returns the effective platform scheduling params (value +
// provenance + env-lock) for the settings page.
func (h *Handlers) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": h.Settings.Effective()})
}

// UpdateSettings persists a patch of platform scheduling params and applies
// them at runtime. Only keys present are changed; env-locked keys are ignored.
func (h *Handlers) UpdateSettings(c *gin.Context) {
	var body map[string]int
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	items, err := h.Settings.Update(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

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
		// No server-initiated stream; ack GET/DELETE so clients don't error.
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
	c.JSON(http.StatusOK, gin.H{"status": "ok", "ready": true, "vnc_preview": h.Browser != nil})
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

// --- workflows ------------------------------------------------------------

func (h *Handlers) ListWorkflows(c *gin.Context) {
	wfs := h.WF.List(c.Query("projectId"))
	out := make([]gin.H, 0, len(wfs))
	for _, wf := range wfs {
		out = append(out, workflowDTO(wf))
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) GetWorkflow(c *gin.Context) {
	wf, ok := h.WF.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, workflowDTO(wf))
}

type workflowBody struct {
	ID           string                       `json:"id"`
	ProjectID    string                       `json:"projectId"`
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	NeedsRepo    bool                         `json:"needsRepo"`
	NotifyPolicy *models.WorkflowNotifyPolicy `json:"notifyPolicy"`
	Nodes        []models.Node                `json:"nodes"`
	Edges        []models.Edge                `json:"edges"`
	Variables    []models.Variable            `json:"variables"`
}

func (h *Handlers) SaveWorkflow(c *gin.Context) {
	var b workflowBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if id := c.Param("id"); id != "" {
		b.ID = id
	}
	isCreate := b.ID == "" || c.Request.Method == http.MethodPost
	if b.ID == "" {
		b.ID = "wf-" + uuid.NewString()[:8]
		isCreate = true
	}
	// Create requires projectId; updates ignore/reject ownership changes in the service.
	if isCreate && strings.TrimSpace(b.ProjectID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": services.ErrWorkflowProjectRequired.Error()})
		return
	}
	graph := models.Graph{Nodes: b.Nodes, Edges: b.Edges, Variables: b.Variables}
	services.LiftInputVariables(&graph)
	// Status is decided solely in WorkflowService.Save (create → draft;
	// update → draft only when the lifted graph differs from the stored head).
	wf := models.WorkflowDef{
		ID: b.ID, ProjectID: b.ProjectID, Name: b.Name, Description: b.Description, NeedsRepo: b.NeedsRepo,
		Graph: graph,
	}
	if b.NotifyPolicy != nil {
		wf.NotifyPolicy = *b.NotifyPolicy
	} else if !isCreate {
		// Preserve existing notify policy when the client omits the field
		// (editor saves that only touch graph/meta).
		if existing, ok := h.WF.Get(b.ID); ok {
			wf.NotifyPolicy = existing.NotifyPolicy
		}
	}
	if err := h.WF.Save(&wf); err != nil {
		switch {
		case errors.Is(err, services.ErrEmptyWorkflowName),
			errors.Is(err, services.ErrWorkflowProjectRequired),
			errors.Is(err, services.ErrWorkflowProjectNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrWorkflowNameExists),
			errors.Is(err, services.ErrWorkflowProjectImmutable):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	action := models.AuditActionWorkflowUpdate
	summary := "update workflow"
	if isCreate {
		action = models.AuditActionWorkflowCreate
		summary = "create workflow"
	}
	h.recordAudit(services.AuditRecord{
		ProjectID:    wf.ProjectID,
		Actor:        h.auditActorFromContext(c),
		Action:       action,
		ResourceType: "workflow",
		ResourceID:   wf.ID,
		Outcome:      models.AuditOutcomeOK,
		Summary:      summary,
		Payload: map[string]any{
			"name":    wf.Name,
			"status":  wf.Status,
			"version": wf.Version,
			"nodes":   len(wf.Graph.Nodes),
		},
	})
	c.JSON(http.StatusOK, workflowDTO(wf))
}

type workflowNotifyPolicyBody struct {
	NotifyPolicy models.WorkflowNotifyPolicy `json:"notifyPolicy"`
}

// PatchWorkflowNotifyPolicy handles PATCH /api/workflows/:id/notify-policy.
// Notify-only write path: does not accept or rewrite Graph (review v1).
func (h *Handlers) PatchWorkflowNotifyPolicy(c *gin.Context) {
	var b workflowNotifyPolicyBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id := c.Param("id")
	wf, err := h.WF.UpdateNotifyPolicy(id, b.NotifyPolicy)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrWorkflowNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	h.recordAudit(services.AuditRecord{
		ProjectID:    wf.ProjectID,
		Actor:        h.auditActorFromContext(c),
		Action:       models.AuditActionWorkflowUpdate,
		ResourceType: "workflow",
		ResourceID:   wf.ID,
		Outcome:      models.AuditOutcomeOK,
		Summary:      "update workflow notify policy",
		Payload: map[string]any{
			"notifyPolicy": wf.NotifyPolicy,
			"status":       wf.Status,
			"version":      wf.Version,
		},
	})
	c.JSON(http.StatusOK, workflowDTO(wf))
}

func (h *Handlers) PublishWorkflow(c *gin.Context) {
	wf, err := h.WF.Publish(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.recordAudit(services.AuditRecord{
		ProjectID:    wf.ProjectID,
		Actor:        h.auditActorFromContext(c),
		Action:       models.AuditActionWorkflowPublish,
		ResourceType: "workflow",
		ResourceID:   wf.ID,
		Outcome:      models.AuditOutcomeOK,
		Summary:      fmt.Sprintf("publish workflow v%d", wf.Version),
		Payload:      map[string]any{"name": wf.Name, "version": wf.Version},
	})
	c.JSON(http.StatusOK, workflowDTO(wf))
}

func (h *Handlers) WorkflowVersions(c *gin.Context) {
	c.JSON(http.StatusOK, h.WF.Versions(c.Param("id")))
}

func (h *Handlers) WorkflowVersionGraph(c *gin.Context) {
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version"})
		return
	}
	graph, err := h.WF.VersionGraph(c.Param("id"), version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, graphDTO(graph))
}

func (h *Handlers) ImportWorkflow(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取请求体"})
		return
	}
	wf, err := h.WF.Import(raw, c.Query("projectId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, workflowDTO(wf))
}

func (h *Handlers) RestoreWorkflowVersion(c *gin.Context) {
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version"})
		return
	}
	wf, err := h.WF.Restore(c.Param("id"), version)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, workflowDTO(wf))
}

func (h *Handlers) DeleteWorkflow(c *gin.Context) {
	id := c.Param("id")
	projectID := ""
	if wf, ok := h.WF.Get(id); ok {
		projectID = wf.ProjectID
	}
	actor := h.auditActorFromContext(c)
	if err := h.WF.Delete(id); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if projectID != "" {
		h.recordAudit(services.AuditRecord{
			ProjectID:    projectID,
			Actor:        actor,
			Action:       models.AuditActionWorkflowDelete,
			ResourceType: "workflow",
			ResourceID:   id,
			Outcome:      models.AuditOutcomeOK,
			Summary:      "delete workflow",
			Payload:      map[string]any{"deleted": true},
		})
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *Handlers) CopyWorkflowPreview(c *gin.Context) {
	suggested, sourceName, sourceID, err := h.WF.CopyPreview(c.Param("id"))
	if errors.Is(err, services.ErrWorkflowNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"suggestedName": suggested,
		"sourceName":    sourceName,
		"sourceId":      sourceID,
	})
}

type copyWorkflowBody struct {
	Name string `json:"name"`
}

func (h *Handlers) CopyWorkflow(c *gin.Context) {
	var b copyWorkflowBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	wf, err := h.WF.Copy(c.Param("id"), b.Name)
	if errors.Is(err, services.ErrWorkflowNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if errors.Is(err, services.ErrEmptyWorkflowName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, services.ErrWorkflowNameExists) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, workflowDTO(wf))
}

// --- runs -----------------------------------------------------------------

type startRunBody struct {
	Inputs   map[string]any `json:"inputs"`
	Trigger  string         `json:"trigger"`
	Priority string         `json:"priority"` // high|normal|low; empty → normal
	Tags     []string       `json:"tags"`
}

func (h *Handlers) StartRun(c *gin.Context) {
	var b startRunBody
	if err := c.ShouldBindJSON(&b); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	trigger, err := models.ResolveTrigger(b.Trigger, models.TriggerManual)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tags, err := models.NormalizeRunTags(b.Tags)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	run, err := h.Eng.StartRunWithPriority(c.Param("id"), b.Inputs, trigger, b.Priority, tags)
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if wf, ok := h.WF.Get(c.Param("id")); ok && wf.ProjectID != "" {
		h.recordAudit(services.AuditRecord{
			ProjectID:    wf.ProjectID,
			Actor:        h.auditActorFromContext(c),
			Action:       models.AuditActionRunStart,
			ResourceType: "run",
			ResourceID:   run.ID,
			RunID:        run.ID,
			Outcome:      models.AuditOutcomeOK,
			Summary:      "start run",
			Payload: map[string]any{
				"workflowId": c.Param("id"),
				"trigger":    trigger,
				"priority":   models.PriorityLabel(run.Priority),
				"runId":      run.ID,
			},
		})
	}
	c.JSON(http.StatusOK, gin.H{"id": run.ID, "status": run.Status, "priority": models.PriorityLabel(run.Priority)})
}

var validRunStatuses = map[string]bool{
	"running":       true,
	"waiting_human": true,
	"queued":        true,
	"completed":     true,
	"failed":        true,
	"cancelled":     true,
}

func parseRunStatuses(raw string) []string {
	if raw == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(raw, ",") {
		s := strings.TrimSpace(part)
		if s != "" && validRunStatuses[s] && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseRunTags(values ...string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			tag, err := models.NormalizeRunTags([]string{part})
			if err != nil || len(tag) == 0 {
				continue
			}
			if _, ok := seen[tag[0]]; ok {
				continue
			}
			seen[tag[0]] = struct{}{}
			out = append(out, tag[0])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *Handlers) ListRuns(c *gin.Context) {
	statuses := parseRunStatuses(c.Query("status"))
	tags := parseRunTags(append(c.QueryArray("tag"), c.Query("tag"))...)
	wf := c.Query("wf")
	projectID := c.Query("projectId")
	sort, order := parseRunListSort(c.Query("sort"), c.Query("order"))
	pg, ok := parsePagination(c)
	if !ok {
		return
	}
	if !pg.Active {
		runs := h.Runs.ListByTags(statuses, wf, projectID, tags, sort, order)
		labels := h.Runs.CurrentNodeLabels(runs)
		out := make([]gin.H, 0, len(runs))
		for _, r := range runs {
			out = append(out, runSummaryDTO(r, labels[r.ID]))
		}
		c.JSON(http.StatusOK, out)
		return
	}
	runs, total := h.Runs.ListPageByTags(statuses, wf, projectID, tags, pg.Page, pg.PageSize, sort, order)
	labels := h.Runs.CurrentNodeLabels(runs)
	items := make([]gin.H, 0, len(runs))
	for _, r := range runs {
		items = append(items, runSummaryDTO(r, labels[r.ID]))
	}
	c.JSON(http.StatusOK, paginatedResponse(items, int(total), pg.Page, pg.PageSize))
}

// parseRunListSort returns whitelist sort/order for ListRuns.
// Both must be valid as a pair; otherwise empty strings signal default order.
func parseRunListSort(sort, order string) (string, string) {
	sort = strings.TrimSpace(sort)
	order = strings.ToLower(strings.TrimSpace(order))
	switch sort {
	case "started_at", "priority":
	default:
		return "", ""
	}
	switch order {
	case "asc", "desc":
		return sort, order
	default:
		return "", ""
	}
}

func (h *Handlers) GetRun(c *gin.Context) {
	run, ok := h.Runs.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, h.runDetailDTO(run))
}

func (h *Handlers) CancelRun(c *gin.Context) {
	runID := c.Param("id")
	run, ok := h.Runs.Get(runID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	actor := h.auditActorFromContext(c)
	if err := h.Eng.Cancel(runID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	projectID := ""
	if h.WF != nil {
		if wf, wOk := h.WF.Get(run.WorkflowID); wOk {
			projectID = wf.ProjectID
		}
	}
	if projectID != "" {
		h.recordAudit(services.AuditRecord{
			ProjectID:    projectID,
			Actor:        actor,
			Action:       models.AuditActionRunCancel,
			ResourceType: "run",
			ResourceID:   runID,
			RunID:        runID,
			Outcome:      models.AuditOutcomeOK,
			Summary:      "cancel run",
			Payload:      map[string]any{"workflowId": run.WorkflowID, "runId": runID},
		})
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// DeleteRun hard-deletes a completed/failed/cancelled run and its associated
// data. Missing id → 404; non-deletable status → 409; success → 200
// {status:deleted} (aligned with DeleteWorkflow). Permission matches
// cancel/resume (same /api session auth).
func (h *Handlers) DeleteRun(c *gin.Context) {
	if err := h.Runs.Delete(c.Param("id")); err != nil {
		switch {
		case errors.Is(err, services.ErrRunNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, services.ErrRunNotDeletable):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type updateRunPriorityBody struct {
	Priority string `json:"priority"`
}

// UpdateRunPriority changes the admission priority of a non-terminal run.
// Permission matches cancel/resume (same /api session auth). Terminal runs
// (completed/failed/cancelled) are rejected with a clear error.
func (h *Handlers) UpdateRunPriority(c *gin.Context) {
	runID := c.Param("id")
	if _, ok := h.Runs.Get(runID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b updateRunPriorityBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	run, err := h.Eng.UpdateRunPriority(runID, b.Priority)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":       run.ID,
		"status":   run.Status,
		"priority": models.PriorityLabel(run.Priority),
	})
}

type resumeRunBody struct {
	// NodeID chooses where to continue; empty resumes from the node that failed.
	NodeID string `json:"nodeId"`
}

// ResumeRun continues a failed/cancelled run from a node (default: the failed
// one), reusing everything the original run already produced.
func (h *Handlers) ResumeRun(c *gin.Context) {
	var b resumeRunBody
	if err := c.ShouldBindJSON(&b); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	runID := c.Param("id")
	if _, ok := h.Runs.Get(runID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err := h.Eng.ResumeFrom(runID, b.NodeID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resumed"})
}

func (h *Handlers) RunVariables(c *gin.Context) {
	c.JSON(http.StatusOK, h.Runs.Variables(c.Param("id")))
}

func (h *Handlers) RunArtifacts(c *gin.Context) {
	c.JSON(http.StatusOK, h.Arts.ByRun(c.Param("id")))
}

// NodeEvents returns a run node's agent event log. While the node is executing
// it is read live from its sandbox (so it survives a UI refresh / re-entry);
// once the sandbox is gone it falls back to the node's persisted final snapshot.
// When a live sandbox is registered but the bridge read fails, the handler
// returns 502 with an error body so the UI can show a rehydrate failure
// instead of a fake empty "waiting for first event" state.
func (h *Handlers) NodeEvents(c *gin.Context) {
	runID := c.Param("id")
	nodeID := c.Param("nodeId")
	cp, ok := parseCursorPagination(c)
	if !ok {
		return
	}
	if !cp.Active {
		ev, live, err := h.Eng.LiveNodeEvents(c.Request.Context(), runID, nodeID)
		if err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "live event log read failed", "live": false})
			return
		}
		if live {
			c.JSON(http.StatusOK, gin.H{"events": ev, "live": true})
			return
		}
		if sr, ok := h.Runs.StateRun(runID, nodeID); ok {
			c.JSON(http.StatusOK, gin.H{"events": sr.Events, "live": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"events": []models.AcpEvent{}, "live": false})
		return
	}

	ev, next, hasMore, live, err := h.Eng.LiveNodeEventsPage(c.Request.Context(), runID, nodeID, cp.Cursor, cp.Limit)
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "live event log read failed", "live": false})
		return
	}
	if live {
		c.JSON(http.StatusOK, gin.H{"events": ev, "nextCursor": next, "hasMore": hasMore, "live": true})
		return
	}
	if sr, ok := h.Runs.StateRun(runID, nodeID); ok {
		ev, next, hasMore := pagePersistedEvents(sr.Events, cp.Cursor, cp.Limit)
		c.JSON(http.StatusOK, gin.H{"events": ev, "nextCursor": next, "hasMore": hasMore, "live": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": []models.AcpEvent{}, "nextCursor": "", "hasMore": false, "live": false})
}

type gateResumeBody struct {
	Action string         `json:"action"`
	Form   map[string]any `json:"form"`
}

func (h *Handlers) ResumeGate(c *gin.Context) {
	var b gateResumeBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	actor := h.auditActorFromContext(c)
	reviewer := ""
	if !actor.Unattributable {
		reviewer = actor.Username
	}
	if err := h.Eng.ResumeGateAs(c.Param("id"), c.Param("nodeId"), b.Action, b.Form, reviewer); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resumed"})
}

type gateArtifactSaveBody struct {
	Content string `json:"content"`
}

// ListGatePrimaryArtifacts returns the editable primary products for a pending gate.
func (h *Handlers) ListGatePrimaryArtifacts(c *gin.Context) {
	items, err := h.Eng.ListGatePrimaryProducts(c.Param("id"), c.Param("nodeId"))
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		c.JSON(http.StatusOK, gin.H{"items": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// SaveGateArtifact updates a gate-scoped primary artifact (waiting_human only).
// Optional If-Match header enables external-change detection (409 on mismatch).
func (h *Handlers) SaveGateArtifact(c *gin.Context) {
	var b gateArtifactSaveBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := c.Param("name")
	ifMatch := strings.TrimSpace(c.GetHeader("If-Match"))
	res, err := h.Eng.SaveGateArtifact(c.Param("id"), c.Param("nodeId"), name, b.Content, ifMatch)
	if err != nil {
		_ = c.Error(err)
		if engine.IsArtifactConflict(err) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("ETag", res.ETag)
	c.JSON(http.StatusOK, gin.H{
		"id": res.ID, "name": res.Name, "kind": res.Kind, "sizeBytes": res.SizeBytes,
		"updatedAt": res.UpdatedAt, "etag": res.ETag, "nodeId": res.NodeID,
		"content": res.Content,
	})
}

type reactReplyBody struct {
	Text   string               `json:"text"`
	Images []models.PromptImage `json:"images"`
	// Annotations are precise field/element references (JSON path or DOM
	// selector + note) the human attached to this review turn.
	Annotations []models.ReactAnnotation `json:"annotations"`
	// Force finishes the clarification/review early: the agent is asked to wrap
	// up and the node completes regardless of any further questions. For a
	// review node force=true is "确认并流转"; force=false is one in-place edit.
	Force bool `json:"force"`
}

func (h *Handlers) ReactReply(c *gin.Context) {
	var b reactReplyBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	runID, nodeID := c.Param("id"), c.Param("nodeId")
	if err := h.Eng.ReactReply(runID, nodeID, b.Text, b.Images, b.Annotations, b.Force); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Review !force enqueues and returns before the turn finishes.
	if !b.Force {
		if w, thinking := h.Eng.ReviewSessionState(runID, nodeID); thinking || w > 0 {
			c.JSON(http.StatusOK, gin.H{"status": "accepted", "waiting": w})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ReactCancel aborts the current turn. Review clears the pending FIFO (#77);
// classic clarify keeps the queue and lets the pump start the next item (Demo).
func (h *Handlers) ReactCancel(c *gin.Context) {
	runID, nodeID := c.Param("id"), c.Param("nodeId")
	clearQueue := true
	if run, ok := h.Runs.Get(runID); ok {
		if n := run.Graph.FindNode(nodeID); n != nil && n.Type == "react" {
			clearQueue = false
		}
	}
	var err error
	if clearQueue {
		err = h.Eng.CancelReviewSession(runID, nodeID)
	} else {
		err = h.Eng.CancelClarifyTurn(runID, nodeID)
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type gateReactReviseBody struct {
	Text        string                   `json:"text"`
	Images      []models.PromptImage     `json:"images"`
	Annotations []models.ReactAnnotation `json:"annotations"`
}

// GateReactRevise issues a ReAct reject-and-annotate against a pending approval
// gate: the annotation/text/images are sent to the upstream producer's still-
// alive sandbox session, which edits the product in place; the gate body is
// refreshed and stays pending for further rounds.
func (h *Handlers) GateReactRevise(c *gin.Context) {
	var b gateReactReviseBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(b.Text) == "" && len(b.Images) == 0 && len(b.Annotations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text, images, or annotations required"})
		return
	}
	runID, gateNodeID := c.Param("id"), c.Param("nodeId")
	if err := h.Eng.GateReactRevise(runID, gateNodeID, b.Text, b.Images, b.Annotations); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	producerID, _ := h.Eng.GateReactInfo(runID, gateNodeID)
	waiting, _ := h.Eng.ReviewSessionState(runID, producerID)
	c.JSON(http.StatusOK, gin.H{"status": "accepted", "waiting": waiting, "producerNodeId": producerID})
}

// GateReactCancel cancels the upstream producer's review turn/queue from a gate.
func (h *Handlers) GateReactCancel(c *gin.Context) {
	runID, gateNodeID := c.Param("id"), c.Param("nodeId")
	producerID, alive := h.Eng.GateReactInfo(runID, gateNodeID)
	if producerID == "" || !alive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上游复审会话不可用"})
		return
	}
	if err := h.Eng.CancelReviewSession(runID, producerID); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "producerNodeId": producerID})
}

// --- gates / artifacts / profiles ----------------------------

func (h *Handlers) ListGates(c *gin.Context) {
	wf := c.Query("wf")
	projectID := c.Query("projectId")
	tags := parseRunTags(append(c.QueryArray("tag"), c.Query("tag"))...)
	pg, ok := parsePagination(c)
	if !ok {
		return
	}
	if !pg.Active {
		items, _ := h.Runs.PendingInboxItems(wf, projectID, tags, 0, 0)
		if h.GateShare != nil {
			h.GateShare.AttachInboxStatus(items)
		}
		c.JSON(http.StatusOK, items)
		return
	}
	offset := (pg.Page - 1) * pg.PageSize
	items, total := h.Runs.PendingInboxItems(wf, projectID, tags, offset, pg.PageSize)
	if h.GateShare != nil {
		h.GateShare.AttachInboxStatus(items)
	}
	c.JSON(http.StatusOK, paginatedResponse(items, total, pg.Page, pg.PageSize))
}

func (h *Handlers) ListArtifacts(c *gin.Context) {
	wf := c.Query("wf")
	projectID := c.Query("projectId")
	pg, ok := parsePagination(c)
	if !ok {
		return
	}
	if !pg.Active {
		c.JSON(http.StatusOK, h.Arts.All())
		return
	}
	arts, total := h.Arts.AllPage(wf, projectID, pg.Page, pg.PageSize, c.Query("q"))
	c.JSON(http.StatusOK, paginatedResponse(arts, int(total), pg.Page, pg.PageSize))
}

// ArtifactContent returns a single artifact's full record including its
// content (the list/run DTOs omit content to stay lightweight).
func (h *Handlers) ArtifactContent(c *gin.Context) {
	a, ok := h.Arts.GetByID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	// Return stored content as-is. Do not hydrate test_result screenshots here:
	// the web UI lazy-loads by artifact name; injecting data would force the
	// legacy inline path and hide the reference-based load.
	content := a.Content
	etag := engine.ArtifactETag(content, a.SizeBytes, a.UpdatedAt)
	out := gin.H{
		"id": a.ID, "runId": a.RunID, "nodeId": a.NodeID, "workflowId": a.WorkflowID, "workflowName": a.WorkflowName,
		"name": a.Name, "kind": a.Kind, "sizeBytes": a.SizeBytes,
		"createdAt": a.CreatedAt, "content": content, "etag": etag,
	}
	if !a.UpdatedAt.IsZero() {
		out["updatedAt"] = a.UpdatedAt
	}
	c.Header("ETag", etag)
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) DownloadArtifact(c *gin.Context) {
	a, ok := h.Arts.GetByID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	body, mime := decodeArtifactDownloadBody(a)
	c.Header("Content-Disposition", "attachment; filename="+a.Name)
	c.Data(http.StatusOK, mime, body)
}

// DeleteArtifact hard-deletes one artifact by id. Success is 204 No Content
// (no body). Missing id → 404; owning run not terminal → 409.
func (h *Handlers) DeleteArtifact(c *gin.Context) {
	if err := h.Arts.DeleteByID(c.Param("id")); err != nil {
		switch {
		case errors.Is(err, services.ErrArtifactNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, services.ErrArtifactRunNotTerminal):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) ListAgents(c *gin.Context) {
	c.JSON(http.StatusOK, h.Skill.List())
}

func (h *Handlers) GetAgent(c *gin.Context) {
	a, ok := h.Skill.Get(c.Param("name"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, a)
}

type agentBody struct {
	Name              string               `json:"name"`
	ProjectID         *string              `json:"projectId"`
	AcpBackend        string               `json:"acpBackend"`
	GitCredentialType string               `json:"gitCredentialType"`
	Files             []services.AgentFile `json:"files"`
	MCP               []services.MCPServer `json:"mcp"`
	Env               map[string]string    `json:"env"`
	Layout            services.AgentLayout `json:"layout"`
	Prompts           *models.AgentPrompts `json:"prompts"`
}

// toAgent builds an Agent. When projectId is omitted (nil), prevProjectID is kept
// so older clients cannot accidentally unbind+purge by leaving the field out.
// Explicit "" unbinds.
func (b agentBody) toAgent(name, prevProjectID string) services.Agent {
	projectID := strings.TrimSpace(prevProjectID)
	if b.ProjectID != nil {
		projectID = strings.TrimSpace(*b.ProjectID)
	}
	return services.Agent{
		Name: name, ProjectID: projectID, AcpBackend: b.AcpBackend,
		GitCredentialType: b.GitCredentialType,
		Files:             b.Files, MCP: b.MCP, Env: b.Env, Layout: b.Layout, Prompts: b.Prompts,
	}
}

// validateAgentProjectBinding enforces Agent↔project rules: unbound Agents may
// not declare project-scoped platform MCPs; a bound Agent must point at an
// existing project. Pure validation — no destructive side effects.
func (h *Handlers) validateAgentProjectBinding(agent services.Agent) error {
	if !services.AgentMayUseProjectPlatformMCP(agent) && services.AgentDeclaresProjectPlatformMCP(agent.MCP) {
		return errors.New("未绑定主项目的 Agent 只能使用 artifact-store；请先绑定主项目再添加 memory-store / context-store / task-scheduler")
	}
	projectID := strings.TrimSpace(agent.ProjectID)
	if projectID == "" {
		return nil
	}
	if h.Projects == nil {
		return errors.New("项目管理服务不可用，无法校验主项目绑定")
	}
	if _, ok := h.Projects.Get(projectID); !ok {
		return errors.New("绑定的主项目不存在")
	}
	return nil
}

// CreateAgent registers a new user-defined Agent.
func (h *Handlers) CreateAgent(c *gin.Context) {
	var b agentBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := services.NormalizeAndValidateAgentName(b.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.Skill.Exists(name) {
		c.JSON(http.StatusConflict, gin.H{"error": "agent already exists"})
		return
	}
	agent := b.toAgent(name, "")
	// New agents get the platform's built-in MCP (artifact-store) by default so
	// they can read/write artifacts and use the plan/ask tools without manual
	// setup. A caller that explicitly sends its own mcp list keeps full control.
	if len(agent.MCP) == 0 {
		agent.MCP = services.DefaultPlatformMCP()
	}
	if err := h.validateAgentProjectBinding(agent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.Skill.Save(agent); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	a, _ := h.Skill.Get(name)
	c.JSON(http.StatusCreated, a)
}

// PatchAgentProject handles PATCH /api/agents/:name/project.
// Group-level assign: only changes projectId (via UpdateProjectID). Empty
// projectId is rejected — unbind stays on the full SaveAgent path.
func (h *Handlers) PatchAgentProject(c *gin.Context) {
	name := c.Param("name")
	var b struct {
		ProjectID string `json:"projectId"`
	}
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	projectID := strings.TrimSpace(b.ProjectID)
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "组级指定不支持解绑主项目"})
		return
	}
	prev, ok := h.Skill.Get(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	oldProjectID := strings.TrimSpace(prev.ProjectID)
	agent := prev
	agent.ProjectID = projectID
	if err := h.validateAgentProjectBinding(agent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if oldProjectID != "" && oldProjectID != projectID && h.Pm != nil {
		if err := h.Pm.PurgeAgentProjectData(oldProjectID, name); err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "清除旧项目数据失败：" + err.Error()})
			return
		}
	}
	if err := h.Skill.UpdateProjectID(name, projectID); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved", "projectId": projectID})
}

func (h *Handlers) SaveAgent(c *gin.Context) {
	var b agentBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := c.Param("name")
	oldProjectID := ""
	if prev, ok := h.Skill.Get(name); ok {
		oldProjectID = strings.TrimSpace(prev.ProjectID)
	}
	agent := b.toAgent(name, oldProjectID)
	if err := h.validateAgentProjectBinding(agent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Switch (A→B) or unbind (A→"") purges the OLD project's agent-scoped data
	// before persisting. Purge failure aborts the whole save. Omitted projectId
	// preserves the previous binding (see agentBody.toAgent) and skips purge.
	newProjectID := strings.TrimSpace(agent.ProjectID)
	if oldProjectID != "" && oldProjectID != newProjectID && h.Pm != nil {
		if err := h.Pm.PurgeAgentProjectData(oldProjectID, name); err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "清除旧项目数据失败：" + err.Error()})
			return
		}
	}
	if err := h.Skill.Save(agent); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

func (h *Handlers) DeleteAgent(c *gin.Context) {
	name := c.Param("name")
	if h.Pm != nil {
		if err := h.Pm.PurgeAgentEverywhere(name); err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "清除 Agent 项目数据失败：" + err.Error()})
			return
		}
	}
	if err := h.Skill.Delete(name); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Cascade organization references after the agent directory is gone so a
	// failed delete never leaves org half-updated without the agent. If the
	// cascade write fails, Get() prune self-heals dangling parentAgent on read.
	if h.Org != nil {
		if err := h.Org.OnDeleteAgent(name); err != nil {
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// renameAgentResp is the RenameAgent success payload: agent fields plus the
// count of WorkflowDef rows whose Def and/or Version graphs were rewritten.
type renameAgentResp struct {
	services.Agent
	UpdatedWorkflowCount int `json:"updatedWorkflowCount"`
}

// RenameAgent atomically renames an existing Agent to the name in the body.
func (h *Handlers) RenameAgent(c *gin.Context) {
	old := c.Param("name")
	var b struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := services.NormalizeAndValidateAgentName(b.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.Skill.Exists(old) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if name != old && h.Skill.Exists(name) {
		c.JSON(http.StatusConflict, gin.H{"error": "agent already exists"})
		return
	}
	if err := h.Skill.Rename(old, name); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.Pm != nil && name != old {
		if err := h.Pm.RenameAgentScopedData(old, name); err != nil {
			if rbErr := h.Skill.Rename(name, old); rbErr != nil {
				_ = c.Error(err)
				_ = c.Error(rbErr)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error() + "; rename rollback failed: " + rbErr.Error(),
				})
				return
			}
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "重命名 Agent 数据失败：" + err.Error()})
			return
		}
	}
	if h.Org != nil {
		if err := h.Org.OnRenameAgent(old, name); err != nil {
			// Roll back the directory rename so org and skill stay aligned.
			if rbErr := h.Skill.Rename(name, old); rbErr != nil {
				_ = c.Error(err)
				_ = c.Error(rbErr)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error() + "; rename rollback failed: " + rbErr.Error(),
				})
				return
			}
			if h.Pm != nil && name != old {
				if rbData := h.Pm.RenameAgentScopedData(name, old); rbData != nil {
					_ = c.Error(rbData)
				}
			}
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	updatedWorkflowCount := 0
	if h.WF != nil && name != old {
		n, err := h.WF.RenameSkillProfileRefs(old, name)
		if err != nil {
			// Roll back Skill/Pm/Org so workflow refs and directory stay aligned.
			if rbErr := h.Skill.Rename(name, old); rbErr != nil {
				_ = c.Error(err)
				_ = c.Error(rbErr)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error() + "; rename rollback failed: " + rbErr.Error(),
				})
				return
			}
			if h.Pm != nil {
				if rbData := h.Pm.RenameAgentScopedData(name, old); rbData != nil {
					_ = c.Error(rbData)
				}
			}
			if h.Org != nil {
				if rbOrg := h.Org.OnRenameAgent(name, old); rbOrg != nil {
					_ = c.Error(rbOrg)
				}
			}
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "重命名工作流引用失败：" + err.Error()})
			return
		}
		updatedWorkflowCount = n
	}
	a, _ := h.Skill.Get(name)
	c.JSON(http.StatusOK, renameAgentResp{Agent: a, UpdatedWorkflowCount: updatedWorkflowCount})
}

// GetAgentsOrg returns the central Agent organization index.
func (h *Handlers) GetAgentsOrg(c *gin.Context) {
	if h.Org == nil {
		c.JSON(http.StatusOK, services.AgentOrg{Groups: []services.OrgGroup{}, Agents: map[string]services.OrgAgentMembership{}})
		return
	}
	org, err := h.Org.Get()
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, org)
}

type putAgentsOrgBody struct {
	Revision int                                    `json:"revision"`
	Groups   []services.OrgGroup                    `json:"groups"`
	Agents   map[string]services.OrgAgentMembership `json:"agents"`
}

// PutAgentsOrg replaces the organization index (optimistic concurrency via revision).
func (h *Handlers) PutAgentsOrg(c *gin.Context) {
	if h.Org == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "org service unavailable"})
		return
	}
	var b putAgentsOrgBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	org, err := h.Org.Put(services.AgentOrg{
		Groups: b.Groups,
		Agents: b.Agents,
	}, b.Revision)
	if err != nil {
		if errors.Is(err, services.ErrOrgConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, services.ErrOrgValidation) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, org)
}

// ExportAgent streams a ZIP export of one agent (on-disk state only).
func (h *Handlers) ExportAgent(c *gin.Context) {
	name := c.Param("name")
	raw, err := h.Skill.ExportZIP(name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename="+name+".zip")
	c.Data(http.StatusOK, "application/zip", raw)
}

// ImportAgent accepts a multipart ZIP and creates or overwrites an agent.
func (h *Handlers) ImportAgent(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	targetName := strings.TrimSpace(c.PostForm("targetName"))
	if targetName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "targetName is required"})
		return
	}
	mode := services.ImportZIPMode(strings.TrimSpace(c.PostForm("mode")))
	if mode == "" {
		mode = services.ImportZIPCreate
	}
	// Create / conflict-rename targets use strict write identity rules.
	// Overwrite of an existing agent keeps the path layer so legacy dotted
	// names (e.g. clarify.v1) remain importable.
	if mode == services.ImportZIPCreate {
		normalized, err := services.NormalizeAndValidateAgentName(targetName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		targetName = normalized
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	raw, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agent, err := h.Skill.ImportZIP(raw, targetName, mode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// ImportZIP already preserves ProjectID on overwrite and strips project
	// platform MCPs when unbound. Re-validate so a stale Projects service
	// mismatch surfaces as 400 rather than a silently imported agent.
	if err := h.validateAgentProjectBinding(agent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, agent)
}
