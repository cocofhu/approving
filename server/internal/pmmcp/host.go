// Package pmmcp implements PM-only MCP hosts: pm-progress, pm-workflow-read,
// pm-workflow-write and pm-agent-fs. Memory and conversation context live in
// memory-store / context-store.
package pmmcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/platformmcp"
	"github.com/cocofhu/approving/internal/services"
	"github.com/cocofhu/approving/internal/textutil"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// MCP ids. Workflow tools are split into a read-only surface and a write
// surface so a project can grant one without the other. pm-agent-fs exposes
// org read + host-side agent workspace FS (not Run sandbox).
const (
	MCPProgress      = "pm-progress"
	MCPWorkflowRead  = "pm-workflow-read"
	MCPWorkflowWrite = "pm-workflow-write"
	MCPAgentFS       = "pm-agent-fs"

	// pm_get_artifact paging: default page size is also the hard ceiling so a
	// caller cannot pull an unbounded artifact in one MCP response.
	pmGetArtifactDefaultLimit = 65536
	pmGetArtifactMaxLimit     = 65536
)

// Session binds a PM MCP token to a project consult context.
type Session struct {
	Token     string
	ProjectID string
	ThreadID  string
	UserID    string
	AgentName string
	Channel   ChannelContext
	Attached  *models.AttachedContext
}

// ChannelContext is the concrete external identity/destination for a channel
// MCP session. UserID on Session remains the conversation-scoped thread key.
type ChannelContext struct {
	ChannelType    string
	Scene          string
	ConversationID string
	ExternalUserID string
}

// IMTarget is passed to the external notifier so lifecycle messages stay bound
// to the active conversation instead of relying on a configured cron target.
type IMTarget struct {
	ChannelType    string
	Scene          string
	ConversationID string
	UserID         string
}

// Host manages project-scoped PM MCP sessions and tool dispatch.
type Host struct {
	mu       sync.RWMutex
	sessions map[string]*Session // token -> session
	byKey    map[string]string   // projectID|threadID -> token

	pm       *services.PmService
	progress *services.PmProgress
	wf       *services.WorkflowService
	runs     *services.RunService
	arts     *services.ArtifactService
	org      *services.OrgService
	skill    *services.SkillService
	eng      engineOps
	audit    func(services.AuditRecord)

	risk   *services.RiskConfirmationService
	tasks  *services.TaskContextService
	notify ExternalIMNotifier
}

// IMDeliveryOutcome is what an IM egress reports back about one notification.
// Sent=false with a Reason means the delivery policy withheld the message on
// purpose (rate limited, deduplicated, merged, already sent); that is a normal
// outcome and must not be reported to the agent as a tool error. Real failures
// (no target, transport error, retries exhausted) come back as an error instead.
type IMDeliveryOutcome struct {
	Sent   bool
	Reason string
}

// ExternalIMNotifier is the minimal IM egress used by PM MCP write tools.
// Implemented by the channel Manager in main; nil disables explicit IM notify.
type ExternalIMNotifier interface {
	NotifyRunAccepted(projectID, runID string, target IMTarget, shortTitle, language string) error
	NotifyProgress(projectID, runID string, target IMTarget, kind, text, stage, conclusion string, blocked, actionRequired bool) (IMDeliveryOutcome, error)
	// NotifyReply delivers the agent's answer for the current conversation turn.
	// This is the only way a conversational answer reaches the user: raw model
	// output is never forwarded, so an answer has to be submitted deliberately.
	NotifyReply(projectID, runID string, target IMTarget, text, shortTitle string) (IMDeliveryOutcome, error)
}

// engineOps covers the run operations exposed through pm-workflow-write
// (narrow interface to avoid handler coupling).
type engineOps interface {
	StartRunWithPriority(workflowID string, inputs map[string]any, trigger, priority string, tags ...[]string) (*models.Run, error)
	ResumeGate(runID, nodeID, action string, form map[string]any) error
	ReactReply(runID, nodeID, humanText string, images []models.PromptImage, annotations []models.ReactAnnotation, force bool) error
	ReviewSessionState(runID, nodeID string) (waiting int, thinking bool)
	Cancel(runID string) error
}

// NewHost builds a PM MCP host. eng/wf/runs may be nil until wired (workflow tools disabled).
func NewHost(pm *services.PmService, progress *services.PmProgress, wf *services.WorkflowService, runs *services.RunService, arts *services.ArtifactService, eng engineOps) *Host {
	return &Host{
		sessions: map[string]*Session{},
		byKey:    map[string]string{},
		pm:       pm,
		progress: progress,
		wf:       wf,
		runs:     runs,
		arts:     arts,
		eng:      eng,
	}
}

// SetOrgAndSkill wires OrgService + SkillService for pm-agent-fs tools.
// Safe to call after NewHost (main wiring); nil leaves org/FS tools unavailable.
func (h *Host) SetOrgAndSkill(org *services.OrgService, skill *services.SkillService) {
	h.mu.Lock()
	h.org = org
	h.skill = skill
	h.mu.Unlock()
}

// SetAuditRecorder wires project-scoped audit recording for PM MCP tools.
func (h *Host) SetAuditRecorder(fn func(services.AuditRecord)) {
	h.mu.Lock()
	h.audit = fn
	h.mu.Unlock()
}

// SetTaskSafety wires high-risk confirmation and task identity helpers used by
// write tools and explicit IM progress notifications.
func (h *Host) SetTaskSafety(risk *services.RiskConfirmationService, tasks *services.TaskContextService, notify ExternalIMNotifier) {
	h.mu.Lock()
	h.risk = risk
	h.tasks = tasks
	h.notify = notify
	h.mu.Unlock()
}

func (h *Host) recordAudit(rec services.AuditRecord) {
	h.mu.RLock()
	fn := h.audit
	h.mu.RUnlock()
	if fn != nil {
		fn(rec)
	}
}

// Register creates (or replaces) a session for project+thread and returns the token.
func (h *Host) Register(projectID, threadID, userID, agentName string) string {
	return h.restore(projectID, threadID, userID, agentName, platformmcp.NewToken())
}

// Restore re-binds an existing token.
func (h *Host) Restore(projectID, threadID, userID, agentName, token string) {
	if strings.TrimSpace(token) == "" {
		return
	}
	_ = h.restore(projectID, threadID, userID, agentName, token)
}

func (h *Host) restore(projectID, threadID, userID, agentName, tok string) string {
	key := projectID + "|" + threadID
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.byKey[key]; ok && old != tok {
		delete(h.sessions, old)
	}
	h.sessions[tok] = &Session{
		Token: tok, ProjectID: projectID, ThreadID: threadID, UserID: userID, AgentName: agentName,
	}
	h.byKey[key] = tok
	return tok
}

// Unregister drops a session by token.
func (h *Host) Unregister(token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.sessions[token]; ok {
		delete(h.byKey, s.ProjectID+"|"+s.ThreadID)
		delete(h.sessions, token)
	}
}

// UnregisterThread drops the session bound to project+thread.
func (h *Host) UnregisterThread(projectID, threadID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := projectID + "|" + threadID
	if tok, ok := h.byKey[key]; ok {
		delete(h.sessions, tok)
		delete(h.byKey, key)
	}
}

// TokenForThread returns the active token for project+thread when present.
func (h *Host) TokenForThread(projectID, threadID string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	tok, ok := h.byKey[projectID+"|"+threadID]
	return tok, ok
}

// SetAttached updates the attached context for an active session.
func (h *Host) SetAttached(token string, attached *models.AttachedContext) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.sessions[token]; ok {
		s.Attached = attached
	}
}

// SetChannelContext updates the active external sender and destination after a
// fresh register or sandbox reuse.
func (h *Host) SetChannelContext(token string, channel ChannelContext) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.sessions[token]; ok {
		s.Channel = channel
	}
}

// Authorize reports whether token is valid for projectID.
func (h *Host) Authorize(projectID, token string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[token]
	return ok && s.ProjectID == projectID
}

// SessionFor returns the session for a token when authorized.
func (h *Host) SessionFor(projectID, token string) (*Session, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[token]
	if !ok || s.ProjectID != projectID {
		return nil, false
	}
	cp := *s
	return &cp, true
}

// ServeRPC handles one JSON-RPC message for mcpId
// (pm-progress | pm-workflow-read | pm-workflow-write | pm-agent-fs).
func (h *Host) ServeRPC(projectID, mcpID, token string, body []byte) (status int, resp []byte) {
	if !h.Authorize(projectID, token) {
		return platformmcp.Unauthorized()
	}
	mcpID = strings.TrimSpace(mcpID)
	if mcpID == "" {
		mcpID = MCPProgress // legacy /mcp/pm/:projectId
	}
	if mcpID != MCPProgress && mcpID != MCPWorkflowRead && mcpID != MCPWorkflowWrite && mcpID != MCPAgentFS {
		return 404, platformmcp.MustJSON(platformmcp.RPCResponse{
			JSONRPC: "2.0",
			Error:   &platformmcp.RPCError{Code: -32004, Message: "unknown mcp: " + mcpID},
		})
	}
	// Enforce project enabledMcps (empty list disables all). Fail closed when
	// the binding cannot be loaded so a missing project never widens access.
	if h.pm != nil {
		b, err := h.pm.GetBinding(projectID)
		if err != nil {
			return 404, platformmcp.MustJSON(platformmcp.RPCResponse{
				JSONRPC: "2.0",
				Error:   &platformmcp.RPCError{Code: -32004, Message: "mcp unavailable for project: " + mcpID},
			})
		}
		allowed := false
		for _, id := range b.EnabledMcps {
			if id == mcpID {
				allowed = true
				break
			}
		}
		if !allowed {
			return 404, platformmcp.MustJSON(platformmcp.RPCResponse{
				JSONRPC: "2.0",
				Error:   &platformmcp.RPCError{Code: -32004, Message: "mcp disabled for project: " + mcpID},
			})
		}
	}
	var req platformmcp.RPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("pm mcp rpc parse error")
		return platformmcp.ParseError()
	}
	switch req.Method {
	case "initialize":
		ver := platformmcp.ProtocolVersion
		var ip struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(req.Params, &ip) == nil && ip.ProtocolVersion != "" {
			ver = ip.ProtocolVersion
		}
		return platformmcp.Ok(req, map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": mcpID, "version": "1.0.0"},
		})
	case "notifications/initialized", "notifications/cancelled":
		return 202, nil
	case "ping":
		return platformmcp.Ok(req, map[string]any{})
	case "tools/list":
		return platformmcp.Ok(req, map[string]any{"tools": toolSchemas(mcpID)})
	case "tools/call":
		var p platformmcp.ToolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return platformmcp.Fail(req, -32602, "invalid tools/call params")
		}
		result, isErr := h.callTool(projectID, token, mcpID, p.Name, p.Arguments)
		h.auditToolCall(projectID, token, mcpID, p.Name, p.Arguments, result, isErr)
		return platformmcp.Ok(req, platformmcp.ToolResult(result, isErr))
	default:
		if platformmcp.IsNotification(req) {
			return 202, nil
		}
		return platformmcp.Fail(req, -32601, "method not found: "+req.Method)
	}
}

func (h *Host) callTool(projectID, token, mcpID, name string, args map[string]any) (any, bool) {
	if _, ok := h.SessionFor(projectID, token); !ok {
		return map[string]any{"error": "unauthorized"}, true
	}
	if args == nil {
		args = map[string]any{}
	}
	switch mcpID {
	case MCPProgress:
		return h.callProgress(projectID, name, args)
	case MCPWorkflowRead:
		return h.callWorkflowRead(projectID, name, args)
	case MCPWorkflowWrite:
		return h.callWorkflowWrite(projectID, token, name, args)
	case MCPAgentFS:
		return h.callAgentFS(projectID, token, name, args)
	default:
		return map[string]any{"error": "unknown mcp"}, true
	}
}

func (h *Host) auditToolCall(projectID, token, mcpID, tool string, args map[string]any, result any, isErr bool) {
	if strings.TrimSpace(projectID) == "" || tool == "" {
		return
	}
	actor := services.SystemActor()
	if sess, ok := h.SessionFor(projectID, token); ok {
		actor = services.ActorFromUsername(sess.UserID)
	}
	outcome := models.AuditOutcomeOK
	if isErr {
		outcome = models.AuditOutcomeFail
	}
	resultPayload := any(result)
	if s, ok := result.(string); ok && len(s) > 2000 {
		resultPayload = s[:2000] + "…"
	}
	h.recordAudit(services.AuditRecord{
		ProjectID:    projectID,
		Actor:        actor,
		Action:       models.AuditActionMCPCall,
		ResourceType: "mcp",
		ResourceID:   tool,
		Outcome:      outcome,
		Summary:      "mcp " + mcpID + "/" + tool,
		Payload: map[string]any{
			"mcp":       mcpID,
			"tool":      tool,
			"arguments": args,
			"result":    resultPayload,
			"isError":   isErr,
		},
	})
}

// workflowInProject loads a workflow and enforces it belongs to projectID.
func (h *Host) workflowInProject(projectID, id string) (models.WorkflowDef, bool) {
	if h.wf == nil {
		return models.WorkflowDef{}, false
	}
	w, ok := h.wf.Get(id)
	if !ok || w.ProjectID != projectID {
		return models.WorkflowDef{}, false
	}
	return w, true
}

// runInProject loads a run and enforces its workflow belongs to projectID.
func (h *Host) runInProject(projectID, runID string) (models.Run, bool) {
	if h.runs == nil {
		return models.Run{}, false
	}
	r, ok := h.runs.Get(runID)
	if !ok {
		return models.Run{}, false
	}
	if _, ok := h.workflowInProject(projectID, r.WorkflowID); !ok {
		return models.Run{}, false
	}
	return r, true
}

func (h *Host) artifactInProject(projectID, artifactID string) (models.Artifact, bool) {
	if h.arts == nil {
		return models.Artifact{}, false
	}
	art, ok := h.arts.GetByID(artifactID)
	if !ok {
		return models.Artifact{}, false
	}
	if strings.TrimSpace(art.RunID) != "" {
		if _, ok := h.runInProject(projectID, art.RunID); !ok {
			return models.Artifact{}, false
		}
		return art, true
	}
	if strings.TrimSpace(art.WorkflowID) != "" {
		if _, ok := h.workflowInProject(projectID, art.WorkflowID); !ok {
			return models.Artifact{}, false
		}
		return art, true
	}
	return models.Artifact{}, false
}

func parseReactAnnotations(args map[string]any, key string) ([]models.ReactAnnotation, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", key, err)
	}
	var anns []models.ReactAnnotation
	if err := json.Unmarshal(b, &anns); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", key, err)
	}
	return anns, nil
}

// parseGraphArgs extracts a Graph from tool args. present reports whether any
// of nodes/edges/variables keys were supplied (so callers can distinguish a
// graph replacement from a metadata-only update). Malformed graph payloads
// return an error instead of a silently empty graph.
func parseGraphArgs(args map[string]any) (graph models.Graph, present bool, err error) {
	_, hasN := args["nodes"]
	_, hasE := args["edges"]
	_, hasV := args["variables"]
	if !hasN && !hasE && !hasV {
		return models.Graph{}, false, nil
	}
	sub := map[string]any{
		"nodes":     args["nodes"],
		"edges":     args["edges"],
		"variables": args["variables"],
	}
	b, mErr := json.Marshal(sub)
	if mErr != nil {
		return models.Graph{}, true, fmt.Errorf("invalid graph args: %w", mErr)
	}
	if uErr := json.Unmarshal(b, &graph); uErr != nil {
		return models.Graph{}, true, fmt.Errorf("invalid graph args: %w", uErr)
	}
	return graph, true, nil
}

func (h *Host) callProgress(projectID, name string, args map[string]any) (any, bool) {
	switch name {
	case "pm_get_progress":
		return h.progress.OverallProgress(projectID), false
	case "pm_list_blockers":
		return h.progress.ListBlockers(projectID), false
	case "pm_get_plan_summary":
		return h.progress.PlanSummary(projectID, platformmcp.StrArg(args, "runId")), false
	case "pm_get_artifact_summary":
		return h.progress.ArtifactSummary(projectID, platformmcp.StrArg(args, "runId"), platformmcp.IntArg(args, "limit", 20)), false
	case "pm_get_risk_trends":
		return h.progress.RiskTrends(projectID), false
	case "pm_compare_runs":
		return h.progress.CompareRuns(projectID, platformmcp.StrArg(args, "workflowId"), platformmcp.IntArg(args, "limit", 5)), false
	default:
		return map[string]any{"error": "unknown tool: " + name}, true
	}
}

// callWorkflowRead serves the read-only pm-workflow-read tools.
func (h *Host) callWorkflowRead(projectID, name string, args map[string]any) (any, bool) {
	if h.wf == nil {
		return map[string]any{"error": "workflow service unavailable"}, true
	}
	switch name {
	case "pm_list_workflows":
		list := h.wf.List(projectID)
		rows := make([]map[string]any, 0, len(list))
		for _, w := range list {
			rows = append(rows, map[string]any{
				"id": w.ID, "name": w.Name, "status": w.Status, "version": w.Version,
				"updatedAt": w.UpdatedAt,
			})
		}
		return map[string]any{"workflows": rows, "count": len(rows)}, false
	case "pm_get_workflow":
		w, ok := h.workflowInProject(projectID, platformmcp.StrArg(args, "workflowId"))
		if !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		return map[string]any{
			"id": w.ID, "name": w.Name, "status": w.Status, "version": w.Version,
			"description": w.Description, "needsRepo": w.NeedsRepo,
		}, false
	case "pm_get_workflow_graph":
		w, ok := h.workflowInProject(projectID, platformmcp.StrArg(args, "workflowId"))
		if !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		return map[string]any{
			"id": w.ID, "nodes": w.Graph.Nodes, "edges": w.Graph.Edges, "variables": w.Graph.Variables,
		}, false
	case "pm_list_versions":
		w, ok := h.workflowInProject(projectID, platformmcp.StrArg(args, "workflowId"))
		if !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		vers := h.wf.Versions(w.ID)
		return map[string]any{"workflowId": w.ID, "versions": vers, "count": len(vers)}, false
	case "pm_list_runs":
		if h.runs == nil {
			return map[string]any{"error": "run service unavailable"}, true
		}
		wfID := platformmcp.StrArg(args, "workflowId")
		if wfID == "" {
			return map[string]any{"error": "workflowId required"}, true
		}
		if _, ok := h.workflowInProject(projectID, wfID); !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		limit := platformmcp.IntArg(args, "limit", 10)
		if limit <= 0 {
			limit = 10
		}
		runs := h.runs.List(nil, wfID, projectID)
		if len(runs) > limit {
			runs = runs[:limit]
		}
		rows := make([]map[string]any, 0, len(runs))
		for _, r := range runs {
			row := map[string]any{
				"id": r.ID, "status": r.Status, "progress": r.Progress,
				"title": r.Title, "startedAt": r.StartedAt, "durationSec": r.DurationSec,
				"attempt": r.Attempt, "workflowVersion": r.WorkflowVersion,
			}
			if r.Status == "failed" {
				info := h.runs.AggregateRunFailure(r.ID)
				if short := info.ShortDisplayReason(200); short != "" {
					row["error"] = short
				}
			}
			rows = append(rows, row)
		}
		return map[string]any{"workflowId": wfID, "runs": rows, "count": len(rows)}, false
	case "pm_list_pending_gates":
		if h.runs == nil {
			return map[string]any{"error": "run service unavailable"}, true
		}
		limit := platformmcp.IntArg(args, "limit", 50)
		items, total := h.runs.PendingInboxItems("", projectID, nil, 0, limit)
		return map[string]any{"items": items, "count": len(items), "total": total}, false
	case "pm_get_artifact":
		if h.arts == nil {
			return map[string]any{"error": "artifact service unavailable"}, true
		}
		offset := platformmcp.IntArg(args, "offset", 0)
		limit := platformmcp.IntArg(args, "limit", pmGetArtifactDefaultLimit)
		if offset < 0 {
			offset = 0
		}
		if limit <= 0 {
			limit = pmGetArtifactDefaultLimit
		}
		if limit > pmGetArtifactMaxLimit {
			limit = pmGetArtifactMaxLimit
		}
		artifactID := strings.TrimSpace(platformmcp.StrArg(args, "artifactId"))
		runID := strings.TrimSpace(platformmcp.StrArg(args, "runId"))
		nameArg := strings.TrimSpace(platformmcp.StrArg(args, "name"))
		var art models.Artifact
		var ok bool
		if artifactID != "" {
			art, ok = h.artifactInProject(projectID, artifactID)
		} else {
			if runID == "" || nameArg == "" {
				return map[string]any{"error": "artifactId or runId+name required"}, true
			}
			if _, runOK := h.runInProject(projectID, runID); !runOK {
				return map[string]any{"error": "run not found"}, true
			}
			art, ok = h.arts.GetRecord(runID, nameArg)
		}
		if !ok {
			return map[string]any{"error": "artifact not found"}, true
		}
		total := len(art.Content)
		if offset > total {
			offset = total
		}
		end := offset + limit
		truncated := end < total
		content := art.Content[offset:]
		if truncated {
			content = textutil.TruncateBytes(content, limit, "")
			end = offset + len(content)
		} else {
			end = total
		}
		return map[string]any{
			"artifactId": art.ID,
			"runId":      art.RunID,
			"workflowId": art.WorkflowID,
			"name":       art.Name,
			"kind":       art.Kind,
			"nodeId":     art.NodeID,
			"content":    content,
			"offset":     offset,
			"limit":      limit,
			"returned":   len(content),
			"totalBytes": total,
			"nextOffset": end,
			"remaining":  total - end,
			"truncated":  truncated,
		}, false
	default:
		return map[string]any{"error": "unknown tool: " + name}, true
	}
}

// callWorkflowWrite serves the mutating pm-workflow-write tools. All targets
// are enforced to belong to the session's project. Access is gated by whether
// pm-workflow-write is enabled for the project (see ServeRPC), not by session
// trust level.
func (h *Host) callWorkflowWrite(projectID, token, name string, args map[string]any) (any, bool) {
	if h.wf == nil {
		return map[string]any{"error": "workflow service unavailable"}, true
	}
	switch name {
	case "pm_create_workflow":
		wfName := strings.TrimSpace(platformmcp.StrArg(args, "name"))
		if wfName == "" {
			return map[string]any{"error": "name required"}, true
		}
		graph, _, err := parseGraphArgs(args)
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		services.LiftInputVariables(&graph)
		wf := &models.WorkflowDef{
			ID:          "wf-" + uuid.NewString()[:8],
			ProjectID:   projectID,
			Name:        wfName,
			Description: platformmcp.StrArg(args, "description"),
			NeedsRepo:   platformmcp.BoolArg(args, "needsRepo"),
			Graph:       graph,
		}
		if err := h.wf.Save(wf); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"id": wf.ID, "name": wf.Name, "status": wf.Status, "version": wf.Version}, false
	case "pm_update_workflow":
		w, ok := h.workflowInProject(projectID, platformmcp.StrArg(args, "workflowId"))
		if !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		if _, ok := args["name"]; ok {
			name := strings.TrimSpace(platformmcp.StrArg(args, "name"))
			if name == "" {
				return map[string]any{"error": "name required"}, true
			}
			w.Name = name
		}
		if v, ok := args["description"].(string); ok {
			w.Description = v
		}
		if _, ok := args["needsRepo"]; ok {
			w.NeedsRepo = platformmcp.BoolArg(args, "needsRepo")
		}
		if graph, present, err := parseGraphArgs(args); err != nil {
			return map[string]any{"error": err.Error()}, true
		} else if present {
			services.LiftInputVariables(&graph)
			w.Graph = graph
		}
		if err := h.wf.Save(&w); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"id": w.ID, "name": w.Name, "status": w.Status, "version": w.Version}, false
	case "pm_copy_workflow":
		if _, ok := h.workflowInProject(projectID, platformmcp.StrArg(args, "workflowId")); !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		out, err := h.wf.Copy(platformmcp.StrArg(args, "workflowId"), platformmcp.StrArg(args, "name"))
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"id": out.ID, "name": out.Name, "status": out.Status, "version": out.Version}, false
	case "pm_delete_workflow":
		if _, ok := h.workflowInProject(projectID, platformmcp.StrArg(args, "workflowId")); !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		if err := h.wf.Delete(platformmcp.StrArg(args, "workflowId")); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"id": platformmcp.StrArg(args, "workflowId"), "deleted": true}, false
	case "pm_publish_workflow":
		w, ok := h.workflowInProject(projectID, platformmcp.StrArg(args, "workflowId"))
		if !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		out, err := h.wf.Publish(w.ID)
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"id": out.ID, "status": out.Status, "version": out.Version}, false
	case "pm_start_run":
		if h.eng == nil {
			return map[string]any{"error": "engine unavailable"}, true
		}
		w, ok := h.workflowInProject(projectID, platformmcp.StrArg(args, "workflowId"))
		if !ok {
			return map[string]any{"error": "workflow not found"}, true
		}
		trigger, err := models.ResolveTrigger(platformmcp.StrArg(args, "trigger"), models.TriggerPMMCP)
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		priority := platformmcp.StrArg(args, "priority")
		if priority == "" {
			priority = "normal"
		}
		tags, err := models.NormalizeRunTags(platformmcp.StringSliceArg(args, "tags"))
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		run, err := h.eng.StartRunWithPriority(w.ID, platformmcp.MapArg(args, "inputs"), trigger, priority, tags)
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		actor := services.SystemActor()
		if sess, ok := h.SessionFor(projectID, token); ok {
			actor = services.ActorFromUsername(sess.UserID)
		}
		h.recordAudit(services.AuditRecord{
			ProjectID:    projectID,
			Actor:        actor,
			Action:       models.AuditActionRunStart,
			ResourceType: "run",
			ResourceID:   run.ID,
			RunID:        run.ID,
			Outcome:      models.AuditOutcomeOK,
			Summary:      "start run (pm_mcp)",
			Payload: map[string]any{
				"workflowId": w.ID,
				"trigger":    trigger,
				"priority":   priority,
				"source":     "pm_mcp",
				"runId":      run.ID,
			},
		})
		shortTitle := run.Title
		sess, hasSess := h.SessionFor(projectID, token)
		if h.tasks != nil {
			sessUser := ""
			if hasSess {
				sessUser = riskUserIDForSession(sess)
			}
			if identity, err := h.tasks.EnsureIdentityForRun(*run, projectID, sessUser); err == nil && identity != nil {
				shortTitle = identity.ShortTitle
				// Bind the task to the conversation that asked for it. Results
				// have to come back here, and a project-wide push target is not
				// the same place — this is the only record of where "here" is,
				// and it must survive a restart.
				if hasSess && strings.TrimSpace(sess.Channel.ConversationID) != "" {
					if _, err := h.tasks.UpdateIdentity(services.EnsureTaskIdentityInput{
						RunID: run.ID, ProjectID: projectID, UserID: sessUser,
						OriginChannel:        sess.Channel.ChannelType,
						OriginScene:          sess.Channel.Scene,
						OriginConversationID: sess.Channel.ConversationID,
						OriginExternalUserID: sess.Channel.ExternalUserID,
						// The task is conducted in the language it was asked
						// in; later updates follow the task, not whichever
						// message happens to trigger them.
						Language: services.DetectLanguage(identity.OriginalRequirement, ""),
					}); err != nil {
						log.Warn().Err(err).Str("run", run.ID).
							Msg("pm_start_run: binding task to origin conversation failed")
					}
				}
			}
		}
		if h.notify != nil {
			target := IMTarget{}
			if hasSess {
				target = imTargetForSession(sess)
			}
			if err := h.notify.NotifyRunAccepted(projectID, run.ID, target, shortTitle, ""); err != nil {
				log.Warn().Err(err).Str("run", run.ID).Msg("pm_start_run acceptance ack failed")
			}
		}
		return map[string]any{"id": run.ID, "status": run.Status, "shortTitle": shortTitle}, false
	case "pm_reply":
		if h.notify == nil {
			return map[string]any{"error": "im notifier unavailable"}, true
		}
		text := strings.TrimSpace(platformmcp.StrArg(args, "text"))
		if text == "" {
			return map[string]any{"error": "text is required"}, true
		}
		// A runId is optional, but when given it must be this project's Run —
		// an answer must never be attributed to another project's task.
		runID := strings.TrimSpace(platformmcp.StrArg(args, "runId"))
		if runID != "" {
			if _, ok := h.runInProject(projectID, runID); !ok {
				return map[string]any{"error": "run not found"}, true
			}
		}
		target := IMTarget{}
		if sess, ok := h.SessionFor(projectID, token); ok {
			target = imTargetForSession(sess)
		}
		if strings.TrimSpace(target.ConversationID) == "" {
			return map[string]any{"error": "no external conversation bound to this session"}, true
		}
		outcome, err := h.notify.NotifyReply(projectID, runID, target, text,
			strings.TrimSpace(platformmcp.StrArg(args, "shortTitle")))
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		if !outcome.Sent {
			reason := outcome.Reason
			if reason == "" {
				reason = "suppressed"
			}
			return map[string]any{"status": "suppressed", "sent": false, "reason": reason}, false
		}
		return map[string]any{"status": "sent", "sent": true}, false
	case "pm_notify_progress":
		if h.notify == nil {
			return map[string]any{"error": "im notifier unavailable"}, true
		}
		runID := platformmcp.StrArg(args, "runId")
		if _, ok := h.runInProject(projectID, runID); !ok {
			return map[string]any{"error": "run not found"}, true
		}
		text := strings.TrimSpace(platformmcp.StrArg(args, "text"))
		stage := strings.TrimSpace(platformmcp.StrArg(args, "stage"))
		conclusion := strings.TrimSpace(platformmcp.StrArg(args, "conclusion"))
		kind := strings.TrimSpace(platformmcp.StrArg(args, "kind"))
		if kind == "" {
			kind = "progress"
		}
		blocked := platformmcp.BoolArg(args, "blocked")
		actionRequired := platformmcp.BoolArg(args, "actionRequired")
		if text == "" {
			text = stage
			if text == "" {
				text = conclusion
			}
		}
		if text == "" {
			return map[string]any{"error": "text, stage or conclusion required"}, true
		}
		target := IMTarget{}
		if sess, ok := h.SessionFor(projectID, token); ok {
			target = imTargetForSession(sess)
		}
		outcome, err := h.notify.NotifyProgress(projectID, runID, target, kind, text, stage, conclusion, blocked, actionRequired)
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		if !outcome.Sent {
			// A policy suppression is a successful call with a different
			// outcome. Reporting it as a tool error made agents rephrase and
			// resend the very message the policy just merged or deduplicated.
			reason := outcome.Reason
			if reason == "" {
				reason = "suppressed"
			}
			return map[string]any{
				"status": "suppressed", "sent": false, "reason": reason, "kind": kind,
			}, false
		}
		return map[string]any{"status": "sent", "sent": true, "kind": kind}, false
	case "pm_resume_gate":
		if h.eng == nil {
			return map[string]any{"error": "engine unavailable"}, true
		}
		runID := platformmcp.StrArg(args, "runId")
		if _, ok := h.runInProject(projectID, runID); !ok {
			return map[string]any{"error": "run not found"}, true
		}
		nodeID := strings.TrimSpace(platformmcp.StrArg(args, "nodeId"))
		action := strings.TrimSpace(platformmcp.StrArg(args, "action"))
		if nodeID == "" || action == "" {
			return map[string]any{"error": "nodeId and action required"}, true
		}
		if blocked, completed, prompt, confirmErr := h.requireRiskConfirmation(projectID, token, runID, "resume_gate:"+nodeID+":"+action); confirmErr != nil {
			return map[string]any{"error": confirmErr.Error()}, true
		} else if completed {
			return map[string]any{"status": "already_confirmed"}, false
		} else if blocked {
			return map[string]any{"status": "needs_confirmation", "prompt": prompt}, false
		}
		if err := h.eng.ResumeGate(runID, nodeID, action, platformmcp.MapArg(args, "form")); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"status": "resumed"}, false
	case "pm_react_reply":
		if h.eng == nil {
			return map[string]any{"error": "engine unavailable"}, true
		}
		runID := platformmcp.StrArg(args, "runId")
		if _, ok := h.runInProject(projectID, runID); !ok {
			return map[string]any{"error": "run not found"}, true
		}
		nodeID := strings.TrimSpace(platformmcp.StrArg(args, "nodeId"))
		if nodeID == "" {
			return map[string]any{"error": "nodeId required"}, true
		}
		text := platformmcp.StrArg(args, "text")
		annotations, err := parseReactAnnotations(args, "annotations")
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		force := platformmcp.BoolArg(args, "force")
		if err := h.eng.ReactReply(runID, nodeID, text, nil, annotations, force); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		if !force {
			if waiting, thinking := h.eng.ReviewSessionState(runID, nodeID); thinking || waiting > 0 {
				return map[string]any{"status": "accepted", "waiting": waiting}, false
			}
		}
		return map[string]any{"status": "ok"}, false
	case "pm_cancel_run":
		if h.eng == nil {
			return map[string]any{"error": "engine unavailable"}, true
		}
		runID := platformmcp.StrArg(args, "runId")
		run, ok := h.runInProject(projectID, runID)
		if !ok {
			return map[string]any{"error": "run not found"}, true
		}
		if blocked, completed, prompt, confirmErr := h.requireRiskConfirmation(projectID, token, runID, "cancel_run"); confirmErr != nil {
			return map[string]any{"error": confirmErr.Error()}, true
		} else if completed {
			return map[string]any{"status": "already_confirmed", "runId": runID}, false
		} else if blocked {
			return map[string]any{"status": "needs_confirmation", "prompt": prompt, "runId": runID}, false
		}
		if err := h.eng.Cancel(runID); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		actor := services.SystemActor()
		if sess, sOk := h.SessionFor(projectID, token); sOk {
			actor = services.ActorFromUsername(sess.UserID)
		}
		h.recordAudit(services.AuditRecord{
			ProjectID:    projectID,
			Actor:        actor,
			Action:       models.AuditActionRunCancel,
			ResourceType: "run",
			ResourceID:   runID,
			RunID:        runID,
			Outcome:      models.AuditOutcomeOK,
			Summary:      "cancel run (pm_mcp)",
			Payload:      map[string]any{"workflowId": run.WorkflowID, "source": "pm_mcp", "runId": runID},
		})
		return map[string]any{"status": "cancelled"}, false
	default:
		return map[string]any{"error": "unknown tool: " + name}, true
	}
}

// requireRiskConfirmation creates a one-shot ticket and blocks the write until
// the user confirms via IM. Returns blocked=false when risk service is unset
// (unit tests without IM wiring) or when a ticket was already confirmed in-band
// via confirmed=true after the IM path executed the action separately.
func (h *Host) requireRiskConfirmation(projectID, token, runID, action string) (blocked, completed bool, prompt string, err error) {
	if h.risk == nil {
		return false, false, "", nil
	}
	sess, ok := h.SessionFor(projectID, token)
	if !ok {
		return true, false, "session required for high-risk confirmation", nil
	}
	riskUserID := riskUserIDForSession(sess)
	// If the IM orchestration already confirmed and executed, MCP should not
	// create another ticket. A fresh pending ticket always blocks.
	if pending, err := h.risk.LatestPending(riskUserID, projectID); err == nil && pending != nil &&
		pending.RunID == runID && pending.Action == action {
		prompt := h.risk.ConfirmationPrompt(*pending)
		return true, false, prompt, h.notifyConfirmation(projectID, runID, sess, prompt)
	}
	if settled, err := h.risk.LatestForAction(riskUserID, projectID, runID, action); err == nil &&
		settled != nil && settled.Status == "confirmed" && settled.ExpiresAt.After(time.Now()) {
		return false, true, "", nil
	}
	shortTitle := ""
	if h.tasks != nil {
		var identity models.TaskIdentity
		if err := h.tasks.DB().Where("run_id = ? AND project_id = ?", runID, projectID).First(&identity).Error; err == nil {
			shortTitle = identity.ShortTitle
		}
	}
	ticket, err := h.risk.CreateTicket(services.RiskTicketInput{
		ProjectID: projectID, UserID: riskUserID, RunID: runID,
		Action: action, ShortTitle: shortTitle, Language: "",
	})
	if err != nil {
		return true, false, "failed to create confirmation ticket: " + err.Error(), nil
	}
	prompt = h.risk.ConfirmationPrompt(*ticket)
	return true, false, prompt, h.notifyConfirmation(projectID, runID, sess, prompt)
}

func (h *Host) notifyConfirmation(projectID, runID string, sess *Session, prompt string) error {
	if h.notify == nil {
		return nil
	}
	// A suppressed confirmation prompt (the same ticket already went out) is not
	// a failure: only a real delivery failure blocks the write.
	if _, err := h.notify.NotifyProgress(projectID, runID, imTargetForSession(sess),
		"action_required", prompt, "", "", false, true); err != nil {
		return fmt.Errorf("send confirmation prompt: %w", err)
	}
	return nil
}

func riskUserIDForSession(sess *Session) string {
	if sess == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(sess.Channel.ChannelType), "qq") &&
		strings.TrimSpace(sess.Channel.ExternalUserID) != "" {
		return services.SyntheticQQUserID(sess.Channel.ExternalUserID)
	}
	return strings.TrimSpace(sess.UserID)
}

func imTargetForSession(sess *Session) IMTarget {
	if sess == nil {
		return IMTarget{}
	}
	return IMTarget{
		ChannelType: sess.Channel.ChannelType, Scene: sess.Channel.Scene,
		ConversationID: sess.Channel.ConversationID, UserID: sess.Channel.ExternalUserID,
	}
}

func toolSchemas(mcpID string) []map[string]any {
	switch mcpID {
	case MCPAgentFS:
		return []map[string]any{
			platformmcp.Tool("pm_get_org", "读取组织架构：全量 groups/agents（含相对当前 Leader 的 self/direct/indirect/other 标注）以及以 Leader 为根的下属树与直接/间接列表。只读，不可改 parent/groups。", nil),
			platformmcp.Tool("pm_fs_list", "列出授权 Agent 的 host 侧 workspace 目录（非 Run 沙箱）。", map[string]any{
				"agentName": map[string]any{"type": "string", "description": "目标 Agent 名（自身或汇报闭包内下属）"},
				"path":      map[string]any{"type": "string", "description": "workspace 相对目录，空或省略表示根"},
			}),
			platformmcp.Tool("pm_fs_read", "读取授权 Agent workspace 内文件（单文件硬上限 1MiB）。", map[string]any{
				"agentName": map[string]any{"type": "string"},
				"path":      map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_fs_write", "写入/创建授权 Agent workspace 内文件（覆盖已存在；自动创建父目录；单文件硬上限 1MiB）。不改 agent.json / Run 沙箱。Studio 刷新或重开后可见；若 Studio 有未保存草稿再 Save 可能覆盖 MCP 写入。", map[string]any{
				"agentName": map[string]any{"type": "string"},
				"path":      map[string]any{"type": "string"},
				"content":   map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_fs_delete", "删除授权 Agent workspace 内文件，或递归删除目录。", map[string]any{
				"agentName": map[string]any{"type": "string"},
				"path":      map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_fs_mkdir", "在授权 Agent workspace 内创建目录（含缺失父目录）。空目录可能不在 Studio 树显示；写入文件后刷新可见。", map[string]any{
				"agentName": map[string]any{"type": "string"},
				"path":      map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_fs_rename", "在同一 Agent workspace 内重命名/移动路径。", map[string]any{
				"agentName": map[string]any{"type": "string"},
				"path":      map[string]any{"type": "string", "description": "源相对路径"},
				"toPath":    map[string]any{"type": "string", "description": "目标相对路径"},
			}),
		}
	case MCPWorkflowRead:
		return []map[string]any{
			platformmcp.Tool("pm_list_workflows", "列出当前项目下的工作流。", nil),
			platformmcp.Tool("pm_get_workflow", "获取工作流元数据（不含完整 Graph）。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_get_workflow_graph", "获取工作流的完整 Graph（nodes/edges/variables）。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_list_versions", "列出工作流已发布版本。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_list_runs", "列出某工作流近期 Run。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
				"limit":      map[string]any{"type": "number"},
			}),
			platformmcp.Tool("pm_list_pending_gates", "列出当前项目待审批的门禁/澄清项。", map[string]any{
				"limit": map[string]any{"type": "number"},
			}),
			platformmcp.Tool("pm_get_artifact", "按 artifactId 或 runId+name 读取当前项目产物全文片段；支持 offset/limit 分页并返回截断信息。", map[string]any{
				"artifactId": map[string]any{"type": "string", "description": "优先使用；与 runId+name 同时提供时以 artifactId 为准"},
				"runId":      map[string]any{"type": "string"},
				"name":       map[string]any{"type": "string"},
				"offset":     map[string]any{"type": "number", "description": "从第几个字节开始读取，默认 0"},
				"limit":      map[string]any{"type": "number", "description": "本次最多返回多少字节，默认 65536，硬上限 65536"},
			}),
		}
	case MCPWorkflowWrite:
		return []map[string]any{
			platformmcp.Tool("pm_create_workflow", "在当前项目创建工作流草稿，可附带完整 Graph。", map[string]any{
				"name":        map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
				"needsRepo":   map[string]any{"type": "boolean"},
				"nodes":       map[string]any{"type": "array"},
				"edges":       map[string]any{"type": "array"},
				"variables":   map[string]any{"type": "array"},
			}),
			platformmcp.Tool("pm_update_workflow", "更新工作流元数据与/或整图替换（含任一 nodes/edges/variables 键即替换 Graph）。", map[string]any{
				"workflowId":  map[string]any{"type": "string"},
				"name":        map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
				"needsRepo":   map[string]any{"type": "boolean"},
				"nodes":       map[string]any{"type": "array"},
				"edges":       map[string]any{"type": "array"},
				"variables":   map[string]any{"type": "array"},
			}),
			platformmcp.Tool("pm_copy_workflow", "在当前项目内复制工作流。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
				"name":       map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_delete_workflow", "删除工作流（级联删除其 Run 与产物）。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_publish_workflow", "发布工作流草稿为新版本。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_start_run", "启动一次工作流 Run。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
				"trigger": map[string]any{
					"type":        "string",
					"enum":        []string{models.TriggerManual, models.TriggerAPI, models.TriggerPMMCP},
					"description": "Run trigger code: manual|api|pm_mcp (default pm_mcp when omitted)",
				},
				"inputs":   map[string]any{"type": "object"},
				"priority": map[string]any{"type": "string", "description": "high|normal|low"},
				"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			}),
			platformmcp.Tool("pm_resume_gate", "审批/推进某个待人工门禁。", map[string]any{
				"runId":  map[string]any{"type": "string"},
				"nodeId": map[string]any{"type": "string"},
				"action": map[string]any{"type": "string", "description": "如 approve|revise"},
				"form":   map[string]any{"type": "object"},
			}),
			platformmcp.Tool("pm_react_reply", "向节点复审 ReAct 会话提交一轮 reply；默认 force=false 继续会话，force=true 才确认并流转。非 human_gate 审批。", map[string]any{
				"runId":  map[string]any{"type": "string"},
				"nodeId": map[string]any{"type": "string", "description": "复审生产者节点 id（如 visual），不是 human_gate 节点"},
				"text":   map[string]any{"type": "string"},
				"force":  map[string]any{"type": "boolean", "description": "默认 false；true 时结束复审并推进流程"},
				"annotations": map[string]any{
					"type":        "array",
					"description": "可选：精确字段/页面元素标注；首版不支持 images",
				},
			}),
			platformmcp.Tool("pm_cancel_run", "取消一次运行中的 Run（需用户短标题二次确认后才会真正取消）。", map[string]any{
				"runId": map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_reply", "把这一轮对话的回答发给用户。这是回答外发的唯一通道——你在正文里写的内容不会被发出去，只有这里提交的 text 会。text 必须是用户直接能读懂的人话：不要出现 Run ID、工作流名、沙箱/工具/内部事件等实现细节，也不要写推理过程。", map[string]any{
				"text":       map[string]any{"type": "string", "description": "发给用户的回答，人话，直接可读"},
				"runId":      map[string]any{"type": "string", "description": "可选：这条回答关联的 Run"},
				"shortTitle": map[string]any{"type": "string", "description": "可选：关联任务的短标题（人话，禁止填 Run ID）"},
			}),
			platformmcp.Tool("pm_notify_progress", "向外部 IM 显式提交一条 Sendable 进度/阻塞/确认消息（须含 stage/conclusion 等实质字段）。返回 status=sent 表示已外发；status=suppressed 表示被限频/去重/合并等策略正常抑制（不是失败，不要改措辞重发）；只有真实投递失败才返回错误。", map[string]any{
				"runId":          map[string]any{"type": "string"},
				"kind":           map[string]any{"type": "string", "description": "progress|blocked|action_required|final"},
				"text":           map[string]any{"type": "string"},
				"stage":          map[string]any{"type": "string"},
				"conclusion":     map[string]any{"type": "string"},
				"blocked":        map[string]any{"type": "boolean"},
				"actionRequired": map[string]any{"type": "boolean"},
			}),
		}
	default:
		return []map[string]any{
			platformmcp.Tool("pm_get_progress", "聚合项目下全部工作流/Run 的整体进度概览。", nil),
			platformmcp.Tool("pm_list_blockers", "列出当前阻塞（waiting_human / 门禁 / 澄清等待）。", nil),
			platformmcp.Tool("pm_get_plan_summary", "解读某次 Run 的 plan.json 完成度与未完成项。", map[string]any{
				"runId": map[string]any{"type": "string"},
			}),
			platformmcp.Tool("pm_get_artifact_summary", "摘要某次 Run 或项目近期产物要点。", map[string]any{
				"runId": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "number"},
			}),
			platformmcp.Tool("pm_get_risk_trends", "基于历史运行识别失败/门禁堆积等风险趋势。", nil),
			platformmcp.Tool("pm_compare_runs", "对比同一工作流最近几次 Run 的差异。", map[string]any{
				"workflowId": map[string]any{"type": "string"},
				"limit":      map[string]any{"type": "number"},
			}),
		}
	}
}
