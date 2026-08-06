package pmmcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingIMNotifier struct {
	progressCalls []struct {
		projectID, runID, kind, text string
		target                       IMTarget
		actionRequired               bool
	}
	err error
}

func (n *recordingIMNotifier) NotifyRunAccepted(string, string, IMTarget, string, string) error {
	return n.err
}

func (n *recordingIMNotifier) NotifyProgress(projectID, runID string, target IMTarget, kind, text, stage, conclusion string, blocked, actionRequired bool) error {
	n.progressCalls = append(n.progressCalls, struct {
		projectID, runID, kind, text string
		target                       IMTarget
		actionRequired               bool
	}{projectID: projectID, runID: runID, kind: kind, text: text, target: target, actionRequired: actionRequired})
	return n.err
}

func setupPmMCPHost(t *testing.T) (*gorm.DB, *services.PmService, *Host, models.Project) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:pmmcp_more_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	ps := services.NewProjectService(db)
	p, err := ps.Create("MoreMCP", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pm := services.NewPmService(db, nil)
	en := true
	agent := "agent-a"
	if _, err := pm.UpdateBinding(p.ID, &en, &agent, []string{MCPProgress, MCPWorkflowRead, MCPWorkflowWrite}, nil, nil); err != nil {
		t.Fatal(err)
	}
	wf := services.NewWorkflowService(db)
	rs := services.NewRunService(db)
	h := NewHost(pm, services.NewPmProgress(pm, rs, nil), wf, rs, services.NewArtifactService(db), nil)
	return db, pm, h, p
}

func TestPmMCPSessionHelpers(t *testing.T) {
	_, _, h, p := setupPmMCPHost(t)
	tok := h.Register(p.ID, "thr-1", "alice", "agent-a")
	h.Restore(p.ID, "thr-1", "alice", "agent-a", tok)
	got, ok := h.TokenForThread(p.ID, "thr-1")
	if !ok || got != tok {
		t.Fatalf("TokenForThread=%q ok=%v", got, ok)
	}
	h.SetAttached(tok, &models.AttachedContext{Kind: "run", ID: "r1"})
	sess, ok := h.SessionFor(p.ID, tok)
	if !ok || sess.Attached == nil || sess.Attached.ID != "r1" {
		t.Fatalf("SessionFor attached=%v", sess)
	}
	h.UnregisterThread(p.ID, "thr-1")
	if _, ok := h.TokenForThread(p.ID, "thr-1"); ok {
		t.Fatal("thread should be unregistered")
	}
	h.Restore(p.ID, "thr-1", "alice", "agent-a", "")
}

func TestChannelSessionRiskIdentityAndConfirmedRetry(t *testing.T) {
	db, _, h, p := setupPmMCPHost(t)
	risk := services.NewRiskConfirmationService(db)
	tasks := services.NewTaskContextService(db)
	notifier := &recordingIMNotifier{}
	h.SetTaskSafety(risk, tasks, notifier)

	tok := h.Register(p.ID, "thr-channel", "qq:group:conversation-1", "agent-a")
	h.SetChannelContext(tok, ChannelContext{
		ChannelType: "qq", Scene: "group",
		ConversationID: "conversation-1", ExternalUserID: "openid-1",
	})
	if err := db.Create(&models.Run{ID: "run-risk", Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-risk", ProjectID: p.ID, ShortTitle: "结算页性能", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}

	blocked, completed, prompt, err := h.requireRiskConfirmation(p.ID, tok, "run-risk", "cancel_run")
	if err != nil || !blocked || completed || !strings.Contains(prompt, "结算页性能") {
		t.Fatalf("first confirmation = blocked:%v completed:%v prompt:%q err:%v", blocked, completed, prompt, err)
	}
	if len(notifier.progressCalls) != 1 {
		t.Fatalf("confirmation notify calls = %d", len(notifier.progressCalls))
	}
	confirmation := notifier.progressCalls[0]
	if confirmation.kind != "action_required" || !confirmation.actionRequired ||
		confirmation.runID != "run-risk" || !strings.Contains(confirmation.text, "结算页性能") ||
		confirmation.target.UserID != "openid-1" {
		t.Fatalf("confirmation notify = %+v", confirmation)
	}
	scopeUser := services.SyntheticQQUserID("openid-1")
	pending, err := risk.LatestPending(scopeUser, p.ID)
	if err != nil || pending == nil {
		t.Fatalf("QQ sender must see MCP ticket: ticket=%+v err=%v", pending, err)
	}
	if pending.UserID == "qq:group:conversation-1" {
		t.Fatalf("ticket used conversation thread id instead of sender identity: %+v", pending)
	}
	resolved, err := risk.ResolveTicket(services.RiskTicketInput{
		ProjectID: p.ID, UserID: scopeUser, RunID: "run-risk", Action: "cancel_run",
	}, "确认")
	if err != nil || !resolved.Execute {
		t.Fatalf("QQ confirmation = %+v err=%v", resolved, err)
	}
	blocked, completed, _, err = h.requireRiskConfirmation(p.ID, tok, "run-risk", "cancel_run")
	if err != nil || blocked || !completed {
		t.Fatalf("confirmed MCP retry must be read-only: blocked=%v completed=%v err=%v", blocked, completed, err)
	}
	var count int64
	if err := db.Model(&models.RiskConfirmationTicket{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("ticket count=%d err=%v", count, err)
	}

	sess, _ := h.SessionFor(p.ID, tok)
	target := imTargetForSession(sess)
	if target.Scene != "group" || target.ConversationID != "conversation-1" || target.UserID != "openid-1" {
		t.Fatalf("IM target = %+v", target)
	}
}

func TestPmStartRunUsesExternalQQIdentityForTaskSearch(t *testing.T) {
	db, pm, _, p := setupPmMCPHost(t)
	wf := services.NewWorkflowService(db)
	wfDef := &models.WorkflowDef{
		ID: "wf-qq-task", ProjectID: p.ID, Name: "登录页工作",
		Graph: models.Graph{Nodes: []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}}},
	}
	if err := wf.Save(wfDef); err != nil {
		t.Fatal(err)
	}
	eng := &fakePmEngine{runTitle: "登录页性能优化"}
	rs := services.NewRunService(db)
	h := NewHost(pm, services.NewPmProgress(pm, rs, nil), wf, rs, services.NewArtifactService(db), eng)
	tasks := services.NewTaskContextService(db)
	h.SetTaskSafety(nil, tasks, nil)
	tok := h.Register(p.ID, "thr-qq-task", "qq:group:conversation-1", "agent-a")
	h.SetChannelContext(tok, ChannelContext{
		ChannelType: "qq", Scene: "group", ConversationID: "conversation-1", ExternalUserID: "openid-1",
	})

	if _, isErr := h.callTool(p.ID, tok, MCPWorkflowWrite, "pm_start_run", map[string]any{
		"workflowId": wfDef.ID, "inputs": map[string]any{"requirement": "优化登录页性能"},
	}); isErr {
		t.Fatal("pm_start_run returned an error")
	}

	scope := services.TaskScope{
		ProjectID: p.ID, UserID: services.SyntheticQQUserID("openid-1"),
		Channel: "qq", ConversationID: "conversation-1",
	}
	candidates, err := tasks.Search(scope, "登录页")
	if err != nil || len(candidates) != 1 || candidates[0].Identity.RunID != "run-pm-mcp" {
		t.Fatalf("QQ sender task search = %+v err=%v", candidates, err)
	}
	if candidates[0].Identity.UserID == "qq:group:conversation-1" {
		t.Fatalf("conversation identity owns task: %+v", candidates[0].Identity)
	}
}

func TestRiskConfirmationNotifyFailureIsReturned(t *testing.T) {
	db, _, h, p := setupPmMCPHost(t)
	risk := services.NewRiskConfirmationService(db)
	tasks := services.NewTaskContextService(db)
	h.SetTaskSafety(risk, tasks, &recordingIMNotifier{err: errors.New("transport down")})
	tok := h.Register(p.ID, "thr-notify-fail", "qq:group:c1", "agent-a")
	h.SetChannelContext(tok, ChannelContext{
		ChannelType: "qq", Scene: "group", ConversationID: "c1", ExternalUserID: "openid-1",
	})
	if err := db.Create(&models.Run{ID: "run-notify-fail", Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}

	blocked, completed, prompt, err := h.requireRiskConfirmation(p.ID, tok, "run-notify-fail", "cancel_run")
	if !blocked || completed || prompt == "" || err == nil || !strings.Contains(err.Error(), "transport down") {
		t.Fatalf("confirmation failure = blocked:%v completed:%v prompt:%q err:%v", blocked, completed, prompt, err)
	}
}

func TestPmMCPServeRPCBranches(t *testing.T) {
	db, pm, h, p := setupPmMCPHost(t)
	tok := h.Register(p.ID, "thr-rpc", "alice", "agent-a")

	st, _ := h.ServeRPC(p.ID, MCPProgress, "bad", []byte(`{}`))
	if st != 401 {
		t.Fatalf("unauth: %d", st)
	}
	st, body := h.ServeRPC(p.ID, "unknown-mcp", tok, []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if st != 404 || !strings.Contains(string(body), "unknown mcp") {
		t.Fatalf("unknown mcp: %d %s", st, body)
	}

	disabled, err := services.NewProjectService(db).Create("NoMCP", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	empty := []string{}
	if _, err := pm.UpdateBinding(disabled.ID, nil, nil, empty, nil, nil); err != nil {
		t.Fatal(err)
	}
	tok2 := h.Register(disabled.ID, "t", "u", "agent-a")
	st, body = h.ServeRPC(disabled.ID, MCPProgress, tok2, []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if st != 404 || !strings.Contains(string(body), "disabled") {
		t.Fatalf("disabled mcp: %d %s", st, body)
	}

	st, _ = h.ServeRPC(p.ID, MCPProgress, tok, []byte(`{`))
	if st != 400 {
		t.Fatalf("parse error: %d", st)
	}

	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-01-01"},
	})
	st, resp := h.ServeRPC(p.ID, MCPProgress, tok, initBody)
	if st != 200 || !strings.Contains(string(resp), "2025-01-01") {
		t.Fatalf("initialize: %d %s", st, resp)
	}

	st, resp = h.ServeRPC(p.ID, MCPProgress, tok, []byte(`{"jsonrpc":"2.0","id":2,"method":"ping"}`))
	if st != 200 {
		t.Fatalf("ping: %d %s", st, resp)
	}

	st, resp = h.ServeRPC(p.ID, MCPProgress, tok, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if st != 202 || len(resp) != 0 {
		t.Fatalf("notification: %d %s", st, resp)
	}

	st, resp = h.ServeRPC(p.ID, MCPProgress, tok, []byte(`{"jsonrpc":"2.0","id":3,"method":"nope"}`))
	if st != 200 || !strings.Contains(string(resp), "method not found") {
		t.Fatalf("unknown method: %d %s", st, resp)
	}

	st, resp = h.ServeRPC(p.ID, MCPProgress, tok, []byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":"bad"}`))
	if st != 200 || !strings.Contains(string(resp), "invalid tools/call params") {
		t.Fatalf("bad tools/call params: %d %s", st, resp)
	}
}

func TestPmMCPProgressTools(t *testing.T) {
	_, _, h, p := setupPmMCPHost(t)
	tok := h.Register(p.ID, "thr-tools", "alice", "agent-a")
	call := func(name string, args map[string]any) (int, []byte) {
		b, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": args},
		})
		return h.ServeRPC(p.ID, MCPProgress, tok, b)
	}
	for _, tool := range []string{
		"pm_get_progress", "pm_list_blockers", "pm_get_plan_summary",
		"pm_get_artifact_summary", "pm_get_risk_trends", "pm_compare_runs",
	} {
		st, resp := call(tool, map[string]any{"runId": "r1", "workflowId": "wf", "limit": 1})
		if st != 200 || strings.Contains(string(resp), `"isError":true`) {
			t.Fatalf("%s: %d %s", tool, st, resp)
		}
	}
	st, resp := call("pm_list_workflows", nil)
	if st != 200 || !strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("progress host rejects workflow tool: %d %s", st, resp)
	}
}

func TestPmMCPWorkflowList(t *testing.T) {
	_, _, h, p := setupPmMCPHost(t)
	tok := h.Register(p.ID, "thr-wf", "alice", "agent-a")
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "pm_list_workflows", "arguments": map[string]any{}},
	})
	st, resp := h.ServeRPC(p.ID, MCPWorkflowRead, tok, body)
	if st != 200 || strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("list workflows: %d %s", st, resp)
	}
}

type fakePmEngine struct {
	resumed struct {
		runID, nodeID, action string
	}
	replied struct {
		runID, nodeID, text string
		force               bool
		annotations         []models.ReactAnnotation
	}
	cancelled   string
	lastTrigger string
	runTitle    string
	startCalls  int
	waiting     int
	thinking    bool
}

func (f *fakePmEngine) StartRunWithPriority(workflowID string, inputs map[string]any, trigger, priority string, tags ...[]string) (*models.Run, error) {
	f.lastTrigger = trigger
	f.startCalls++
	var normalized []string
	if len(tags) > 0 {
		normalized = append([]string{}, tags[0]...)
	}
	return &models.Run{
		ID: "run-pm-mcp", WorkflowID: workflowID, Status: "queued",
		Title: f.runTitle, Trigger: trigger, Tags: normalized, Inputs: inputs,
	}, nil
}

func (f *fakePmEngine) ResumeGate(runID, nodeID, action string, form map[string]any) error {
	f.resumed.runID, f.resumed.nodeID, f.resumed.action = runID, nodeID, action
	return nil
}

func (f *fakePmEngine) ReactReply(runID, nodeID, humanText string, images []models.PromptImage, annotations []models.ReactAnnotation, force bool) error {
	f.replied.runID, f.replied.nodeID, f.replied.text = runID, nodeID, humanText
	f.replied.force = force
	f.replied.annotations = annotations
	return nil
}

func (f *fakePmEngine) ReviewSessionState(runID, nodeID string) (waiting int, thinking bool) {
	return f.waiting, f.thinking
}

func (f *fakePmEngine) Cancel(runID string) error {
	f.cancelled = runID
	return nil
}

func TestPmMCPWorkflowToolsWithEngine(t *testing.T) {
	db, pm, _, p := setupPmMCPHost(t)
	wf := services.NewWorkflowService(db)
	g := models.Graph{Nodes: []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}}}
	wfDef := &models.WorkflowDef{
		ID: "wf-pm-mcp", ProjectID: p.ID, Name: "PM MCP WF", Graph: g,
	}
	if err := wf.Save(wfDef); err != nil {
		t.Fatal(err)
	}
	eng := &fakePmEngine{}
	rs := services.NewRunService(db)
	h := NewHost(pm, services.NewPmProgress(pm, rs, nil), wf, rs, services.NewArtifactService(db), eng)
	tok := h.Register(p.ID, "thr-wf2", "alice", "agent-a")
	call := func(mcpID, name string, args map[string]any) (int, []byte) {
		b, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": args},
		})
		return h.ServeRPC(p.ID, mcpID, tok, b)
	}
	okCall := func(mcpID, name string, args map[string]any) []byte {
		t.Helper()
		st, resp := call(mcpID, name, args)
		if st != 200 || strings.Contains(string(resp), `"isError":true`) {
			t.Fatalf("%s: %d %s", name, st, resp)
		}
		return resp
	}

	// --- read surface ---
	okCall(MCPWorkflowRead, "pm_get_workflow", map[string]any{"workflowId": wfDef.ID})
	okCall(MCPWorkflowRead, "pm_get_workflow_graph", map[string]any{"workflowId": wfDef.ID})
	okCall(MCPWorkflowRead, "pm_list_versions", map[string]any{"workflowId": wfDef.ID})
	listRuns := okCall(MCPWorkflowRead, "pm_list_runs", map[string]any{"workflowId": wfDef.ID, "limit": 1})
	if strings.Contains(string(listRuns), "diffHighlights") {
		t.Fatalf("pm_list_runs should not reuse compare payload: %s", listRuns)
	}
	okCall(MCPWorkflowRead, "pm_list_pending_gates", map[string]any{"limit": 5})

	// --- write surface: create → get_graph → update → publish → start ---
	created := okCall(MCPWorkflowWrite, "pm_create_workflow", map[string]any{
		"name":  "Created WF",
		"nodes": []map[string]any{{"id": "in", "type": "input"}, {"id": "out", "type": "output"}},
	})
	var createResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(created, &createResp); err != nil {
		t.Fatal(err)
	}
	var createdWF struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(createResp.Result.Content[0].Text), &createdWF)
	if createdWF.ID == "" {
		t.Fatalf("create returned no id: %s", created)
	}
	okCall(MCPWorkflowRead, "pm_get_workflow_graph", map[string]any{"workflowId": createdWF.ID})
	okCall(MCPWorkflowWrite, "pm_update_workflow", map[string]any{
		"workflowId":  createdWF.ID,
		"description": "updated",
		"nodes":       []map[string]any{{"id": "in", "type": "input"}, {"id": "out", "type": "output"}},
	})
	if st, resp := call(MCPWorkflowWrite, "pm_update_workflow", map[string]any{
		"workflowId": createdWF.ID,
		"nodes":      "bad",
	}); st != 200 || !strings.Contains(string(resp), `"isError":true`) || !strings.Contains(string(resp), "invalid graph") {
		t.Fatalf("malformed graph should error: %d %s", st, resp)
	}
	okCall(MCPWorkflowWrite, "pm_copy_workflow", map[string]any{"workflowId": createdWF.ID, "name": "Copied WF"})
	okCall(MCPWorkflowWrite, "pm_delete_workflow", map[string]any{"workflowId": createdWF.ID})

	okCall(MCPWorkflowWrite, "pm_publish_workflow", map[string]any{"workflowId": wfDef.ID})
	okCall(MCPWorkflowWrite, "pm_start_run", map[string]any{"workflowId": wfDef.ID, "inputs": map[string]any{"k": "v"}})
	if eng.lastTrigger != models.TriggerPMMCP {
		t.Fatalf("pm_start_run empty trigger: got %q want pm_mcp", eng.lastTrigger)
	}
	okCall(MCPWorkflowWrite, "pm_start_run", map[string]any{"workflowId": wfDef.ID, "trigger": "manual"})
	if eng.lastTrigger != models.TriggerManual {
		t.Fatalf("pm_start_run explicit manual: got %q", eng.lastTrigger)
	}
	beforeIllegal := eng.startCalls
	if st, resp := call(MCPWorkflowWrite, "pm_start_run", map[string]any{
		"workflowId": wfDef.ID, "trigger": "channel",
	}); st != 200 || !strings.Contains(string(resp), `"isError":true`) || !strings.Contains(string(resp), "manual|api|pm_mcp") {
		t.Fatalf("illegal trigger should error: %d %s", st, resp)
	}
	if eng.startCalls != beforeIllegal {
		t.Fatalf("illegal trigger must not call engine (calls %d → %d)", beforeIllegal, eng.startCalls)
	}

	// --- resume_gate / cancel_run go through a real run so project check passes ---
	if err := rs.DB().Create(&models.Run{ID: "run-1", WorkflowID: wfDef.ID, Status: "waiting_human"}).Error; err != nil {
		t.Fatal(err)
	}
	if st, resp := call(MCPWorkflowWrite, "pm_resume_gate", map[string]any{"runId": "run-1", "nodeId": "", "action": "approve"}); st != 200 || !strings.Contains(string(resp), "nodeId and action required") {
		t.Fatalf("empty nodeId should error: %d %s", st, resp)
	}
	okCall(MCPWorkflowWrite, "pm_resume_gate", map[string]any{"runId": "run-1", "nodeId": "gate-1", "action": "approve"})
	if eng.resumed.runID != "run-1" || eng.resumed.action != "approve" {
		t.Fatalf("resume not forwarded: %+v", eng.resumed)
	}
	okCall(MCPWorkflowWrite, "pm_cancel_run", map[string]any{"runId": "run-1"})
	if eng.cancelled != "run-1" {
		t.Fatalf("cancel not forwarded: %q", eng.cancelled)
	}

	// --- not found + cross-project rejection ---
	if st, resp := call(MCPWorkflowRead, "pm_get_workflow", map[string]any{"workflowId": "missing"}); st != 200 || !strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("missing wf: %d %s", st, resp)
	}
	otherProj, err := services.NewProjectService(db).Create("OtherProj", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherWF := &models.WorkflowDef{ID: "wf-other", ProjectID: otherProj.ID, Name: "Other", Graph: g}
	if err := wf.Save(otherWF); err != nil {
		t.Fatal(err)
	}
	if st, resp := call(MCPWorkflowWrite, "pm_delete_workflow", map[string]any{"workflowId": otherWF.ID}); st != 200 || !strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("cross-project delete should be rejected: %d %s", st, resp)
	}
}

func TestPmMCPGetArtifactAndReactReply(t *testing.T) {
	db, pm, _, p := setupPmMCPHost(t)
	wf := services.NewWorkflowService(db)
	rs := services.NewRunService(db)
	arts := services.NewArtifactService(db)
	wfDef := &models.WorkflowDef{
		ID: "wf-pm-art", ProjectID: p.ID, Name: "PM Artifact WF",
		Graph: models.Graph{Nodes: []models.Node{{ID: "visual", Type: "visual"}, {ID: "out", Type: "output"}}},
	}
	if err := wf.Save(wfDef); err != nil {
		t.Fatal(err)
	}
	if err := rs.DB().Create(&models.Run{ID: "run-own", WorkflowID: wfDef.ID, Status: "waiting_human"}).Error; err != nil {
		t.Fatal(err)
	}
	content := "0123456789abcdef"
	artifactID, err := arts.Save("run-own", "visual", "page.html", "html", content)
	if err != nil {
		t.Fatal(err)
	}

	eng := &fakePmEngine{waiting: 2}
	h := NewHost(pm, services.NewPmProgress(pm, rs, arts), wf, rs, arts, eng)
	tok := h.Register(p.ID, "thr-react", "alice", "agent-a")
	call := func(mcpID, name string, args map[string]any) (int, []byte) {
		b, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": args},
		})
		return h.ServeRPC(p.ID, mcpID, tok, b)
	}
	readToolJSON := func(resp []byte) string {
		t.Helper()
		var toolResp struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		if err := json.Unmarshal(resp, &toolResp); err != nil {
			t.Fatalf("unmarshal tool response: %v (%s)", err, resp)
		}
		if len(toolResp.Result.Content) == 0 {
			t.Fatalf("tool response missing content: %s", resp)
		}
		return toolResp.Result.Content[0].Text
	}

	st, resp := call(MCPWorkflowRead, "pm_get_artifact", map[string]any{
		"artifactId": artifactID,
		"offset":     2,
		"limit":      5,
	})
	if st != 200 || strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("pm_get_artifact by id: %d %s", st, resp)
	}
	artifactJSON := readToolJSON(resp)
	for _, want := range []string{`"content": "23456"`, `"truncated": true`, `"remaining": 9`} {
		if !strings.Contains(artifactJSON, want) {
			t.Fatalf("unexpected artifact page: missing %s in %s", want, artifactJSON)
		}
	}

	st, resp = call(MCPWorkflowRead, "pm_get_artifact", map[string]any{
		"runId":  "run-own",
		"name":   "page.html",
		"offset": 14,
		"limit":  10,
	})
	if st != 200 || strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("pm_get_artifact by run+name: %d %s", st, resp)
	}
	artifactJSON = readToolJSON(resp)
	for _, want := range []string{
		`"content": "ef"`,
		`"truncated": false`,
		`"artifactId": "` + artifactID + `"`,
		`"kind": "html"`,
		`"nodeId": "visual"`,
	} {
		if !strings.Contains(artifactJSON, want) {
			t.Fatalf("unexpected artifact tail: missing %s in %s", want, artifactJSON)
		}
	}

	st, resp = call(MCPWorkflowRead, "pm_get_artifact", map[string]any{
		"artifactId": artifactID,
		"limit":      pmGetArtifactMaxLimit + 1,
	})
	if st != 200 || strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("pm_get_artifact oversized limit: %d %s", st, resp)
	}
	artifactJSON = readToolJSON(resp)
	if !strings.Contains(artifactJSON, fmt.Sprintf(`"limit": %d`, pmGetArtifactMaxLimit)) {
		t.Fatalf("oversized limit should clamp to %d: %s", pmGetArtifactMaxLimit, artifactJSON)
	}

	otherProj, err := services.NewProjectService(db).Create("OtherProjArtifact", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherWF := &models.WorkflowDef{
		ID: "wf-other-art", ProjectID: otherProj.ID, Name: "Other Artifact WF",
		Graph: models.Graph{Nodes: []models.Node{{ID: "visual", Type: "visual"}}},
	}
	if err := wf.Save(otherWF); err != nil {
		t.Fatal(err)
	}
	if err := rs.DB().Create(&models.Run{ID: "run-other", WorkflowID: otherWF.ID, Status: "waiting_human"}).Error; err != nil {
		t.Fatal(err)
	}
	otherArtifactID, err := arts.Save("run-other", "visual", "page.html", "html", "secret")
	if err != nil {
		t.Fatal(err)
	}
	st, resp = call(MCPWorkflowRead, "pm_get_artifact", map[string]any{"artifactId": otherArtifactID})
	if st != 200 || !strings.Contains(string(resp), `"isError":true`) || !strings.Contains(string(resp), "artifact not found") {
		t.Fatalf("cross-project artifact should be rejected: %d %s", st, resp)
	}

	st, resp = call(MCPWorkflowWrite, "pm_react_reply", map[string]any{
		"runId":  "run-own",
		"nodeId": "visual",
		"text":   "please fix title",
		"annotations": []map[string]any{
			{"selector": "#hero", "note": "update heading"},
		},
	})
	if st != 200 || strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("pm_react_reply normal: %d %s", st, resp)
	}
	if eng.replied.runID != "run-own" || eng.replied.nodeID != "visual" || eng.replied.text != "please fix title" || eng.replied.force {
		t.Fatalf("reply not forwarded: %+v", eng.replied)
	}
	if len(eng.replied.annotations) != 1 || eng.replied.annotations[0].Selector != "#hero" {
		t.Fatalf("annotations not forwarded: %+v", eng.replied.annotations)
	}
	replyJSON := readToolJSON(resp)
	if !strings.Contains(replyJSON, `"status": "accepted"`) || !strings.Contains(replyJSON, `"waiting": 2`) {
		t.Fatalf("normal reply should stay accepted: %s", replyJSON)
	}

	eng.waiting = 0
	st, resp = call(MCPWorkflowWrite, "pm_react_reply", map[string]any{
		"runId":  "run-own",
		"nodeId": "visual",
		"text":   "confirm flow",
		"force":  true,
	})
	if st != 200 || strings.Contains(string(resp), `"isError":true`) {
		t.Fatalf("pm_react_reply force: %d %s", st, resp)
	}
	replyJSON = readToolJSON(resp)
	if !eng.replied.force || !strings.Contains(replyJSON, `"status": "ok"`) {
		t.Fatalf("force reply should finish: replied=%+v resp=%s", eng.replied, replyJSON)
	}
}

func TestParseReactAnnotations(t *testing.T) {
	anns, err := parseReactAnnotations(map[string]any{
		"annotations": []map[string]any{{"jsonPath": "f1.title", "note": "fix"}},
	}, "annotations")
	if err != nil {
		t.Fatal(err)
	}
	if len(anns) != 1 || anns[0].JSONPath != "f1.title" || anns[0].Note != "fix" {
		t.Fatalf("unexpected annotations: %+v", anns)
	}
	if _, err := parseReactAnnotations(map[string]any{"annotations": fmt.Errorf("bad")}, "annotations"); err == nil {
		t.Fatal("expected invalid annotations error")
	}
}

func TestPmMCPStartCancelWritesRunAudit(t *testing.T) {
	db, pm, _, p := setupPmMCPHost(t)
	auditSvc := services.NewProjectAuditService(db)
	wf := services.NewWorkflowService(db)
	g := models.Graph{Nodes: []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}}}
	wfDef := &models.WorkflowDef{
		ID: "wf-pm-audit", ProjectID: p.ID, Name: "PM Audit WF", Graph: g,
	}
	if err := wf.Save(wfDef); err != nil {
		t.Fatal(err)
	}
	eng := &fakePmEngine{}
	rs := services.NewRunService(db)
	h := NewHost(pm, services.NewPmProgress(pm, rs, nil), wf, rs, services.NewArtifactService(db), eng)
	h.SetAuditRecorder(auditSvc.Record)
	tok := h.Register(p.ID, "thr-audit", "alice", "agent-a")

	call := func(name string, args map[string]any) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": args},
		})
		st, resp := h.ServeRPC(p.ID, MCPWorkflowWrite, tok, b)
		if st != 200 || strings.Contains(string(resp), `"isError":true`) {
			t.Fatalf("%s: %d %s", name, st, resp)
		}
	}

	call("pm_start_run", map[string]any{"workflowId": wfDef.ID, "inputs": map[string]any{}})
	if err := rs.DB().Create(&models.Run{ID: "run-cancel-audit", WorkflowID: wfDef.ID, Status: "queued"}).Error; err != nil {
		t.Fatal(err)
	}
	call("pm_cancel_run", map[string]any{"runId": "run-cancel-audit"})

	items, total, err := auditSvc.ListPage(services.AuditListFilter{ProjectID: p.ID, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total < 2 {
		t.Fatalf("expected audit events, total=%d", total)
	}
	var sawStart, sawCancel, sawMCP bool
	for _, ev := range items {
		switch ev.Action {
		case models.AuditActionRunStart:
			if ev.ResourceID == "run-pm-mcp" {
				sawStart = true
				if ev.Actor != "alice" || ev.Unattributable {
					t.Fatalf("start actor: %#v", ev)
				}
			}
		case models.AuditActionRunCancel:
			if ev.ResourceID == "run-cancel-audit" {
				sawCancel = true
				if ev.Actor != "alice" || ev.Unattributable {
					t.Fatalf("cancel actor: %#v", ev)
				}
			}
		case models.AuditActionMCPCall:
			sawMCP = true
		}
	}
	if !sawStart || !sawCancel {
		t.Fatalf("missing run start/cancel audit: start=%v cancel=%v total=%d", sawStart, sawCancel, total)
	}
	if !sawMCP {
		t.Fatalf("expected mcp.call events from pm tools/call")
	}
}
