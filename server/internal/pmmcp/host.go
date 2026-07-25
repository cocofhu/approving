// Package pmmcp implements PM-only MCP hosts: pm-progress, pm-workflow-read and
// pm-workflow-write. Memory and conversation context live in memory-store /
// context-store.
package pmmcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/platformmcp"
	"github.com/cocofhu/approving/internal/services"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// MCP ids. Workflow tools are split into a read-only surface and a write
// surface so a project can grant one without the other.
const (
	MCPProgress      = "pm-progress"
	MCPWorkflowRead  = "pm-workflow-read"
	MCPWorkflowWrite = "pm-workflow-write"
)

// Session binds a PM MCP token to a project consult context.
type Session struct {
	Token     string
	ProjectID string
	ThreadID  string
	UserID    string
	AgentName string
	Attached  *models.AttachedContext
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
	eng      engineOps
}

// engineOps covers the run operations exposed through pm-workflow-write
// (narrow interface to avoid handler coupling).
type engineOps interface {
	StartRunWithPriority(workflowID string, inputs map[string]any, trigger, priority string) (*models.Run, error)
	ResumeGate(runID, nodeID, action string, form map[string]any) error
	Cancel(runID string) error
}

// NewHost builds a PM MCP host. eng/wf/runs may be nil until wired (workflow tools disabled).
func NewHost(pm *services.PmService, progress *services.PmProgress, wf *services.WorkflowService, runs *services.RunService, eng engineOps) *Host {
	return &Host{
		sessions: map[string]*Session{},
		byKey:    map[string]string{},
		pm:       pm,
		progress: progress,
		wf:       wf,
		runs:     runs,
		eng:      eng,
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
// (pm-progress | pm-workflow-read | pm-workflow-write).
func (h *Host) ServeRPC(projectID, mcpID, token string, body []byte) (status int, resp []byte) {
	if !h.Authorize(projectID, token) {
		return platformmcp.Unauthorized()
	}
	mcpID = strings.TrimSpace(mcpID)
	if mcpID == "" {
		mcpID = MCPProgress // legacy /mcp/pm/:projectId
	}
	if mcpID != MCPProgress && mcpID != MCPWorkflowRead && mcpID != MCPWorkflowWrite {
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
		return h.callWorkflowWrite(projectID, name, args)
	default:
		return map[string]any{"error": "unknown mcp"}, true
	}
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
		items, total := h.runs.PendingInboxItems("", projectID, 0, limit)
		return map[string]any{"items": items, "count": len(items), "total": total}, false
	default:
		return map[string]any{"error": "unknown tool: " + name}, true
	}
}

// callWorkflowWrite serves the mutating pm-workflow-write tools. All targets
// are enforced to belong to the session's project. Access is gated by whether
// pm-workflow-write is enabled for the project (see ServeRPC), not by session
// trust level.
func (h *Host) callWorkflowWrite(projectID, name string, args map[string]any) (any, bool) {
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
		run, err := h.eng.StartRunWithPriority(w.ID, platformmcp.MapArg(args, "inputs"), trigger, priority)
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"id": run.ID, "status": run.Status}, false
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
		if err := h.eng.ResumeGate(runID, nodeID, action, platformmcp.MapArg(args, "form")); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"status": "resumed"}, false
	case "pm_cancel_run":
		if h.eng == nil {
			return map[string]any{"error": "engine unavailable"}, true
		}
		runID := platformmcp.StrArg(args, "runId")
		if _, ok := h.runInProject(projectID, runID); !ok {
			return map[string]any{"error": "run not found"}, true
		}
		if err := h.eng.Cancel(runID); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"status": "cancelled"}, false
	default:
		return map[string]any{"error": "unknown tool: " + name}, true
	}
}

func toolSchemas(mcpID string) []map[string]any {
	switch mcpID {
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
			}),
			platformmcp.Tool("pm_resume_gate", "审批/推进某个待人工门禁。", map[string]any{
				"runId":  map[string]any{"type": "string"},
				"nodeId": map[string]any{"type": "string"},
				"action": map[string]any{"type": "string", "description": "如 approve|revise"},
				"form":   map[string]any{"type": "object"},
			}),
			platformmcp.Tool("pm_cancel_run", "取消一次运行中的 Run。", map[string]any{
				"runId": map[string]any{"type": "string"},
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
