package pmmcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"gorm.io/gorm"
)

// channelActionFixture wires a real host with the real grant store so the test
// proves server-side enforcement rather than a stubbed decision.
func channelActionFixture(t *testing.T) (*gorm.DB, *Host, *fakePmEngine, *services.TaskContextService, models.Project) {
	t.Helper()
	db, pm, _, p := setupPmMCPHost(t)
	wf := services.NewWorkflowService(db)
	graph := models.Graph{Nodes: []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}}}
	if err := wf.Save(&models.WorkflowDef{ID: "wf-guard", ProjectID: p.ID, Name: "Guarded WF", Graph: graph}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run-guard", "run-other"} {
		if err := db.Create(&models.Run{
			ID: id, WorkflowID: "wf-guard", Status: "running", StartedAt: time.Now().UTC(),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	eng := &fakePmEngine{}
	rs := services.NewRunService(db)
	h := NewHost(pm, services.NewPmProgress(pm, rs, nil), wf, rs, services.NewArtifactService(db), eng)
	tasks := services.NewTaskContextService(db)
	h.SetChannelActionAuthorizer(tasks)
	return db, h, eng, tasks, p
}

func callWrite(t *testing.T, h *Host, projectID, token, name string, args map[string]any) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	st, resp := h.ServeRPC(projectID, MCPWorkflowWrite, token, body)
	if st != 200 {
		t.Fatalf("%s status = %d", name, st)
	}
	return string(resp)
}

func grantFor(t *testing.T, tasks *services.TaskContextService, projectID, threadID, runID, kind string) {
	t.Helper()
	ticket, err := tasks.CreateRiskConfirmationWithKind(projectID, "qq:c2c:u1", runID, kind, kind+" "+runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.ConsumeRiskConfirmation(projectID, "qq:c2c:u1", runID, ticket.Action, "确认"); err != nil {
		t.Fatal(err)
	}
	if err := tasks.BindActionGrant(ticket.ID, threadID); err != nil {
		t.Fatal(err)
	}
}

func TestDestructiveMutationsRequireConfirmedTicketOnChannelThreads(t *testing.T) {
	_, h, eng, tasks, p := channelActionFixture(t)
	tok := h.Register(p.ID, "thr-channel", "qq:c2c:u1", "agent-a")
	if err := tasks.GuardChannelThread(p.ID, "thr-channel", "qq", "qq:c2c:u1"); err != nil {
		t.Fatal(err)
	}

	// No ticket: the prompt alone must not execute anything.
	resp := callWrite(t, h, p.ID, tok, "pm_cancel_run", map[string]any{"runId": "run-guard"})
	if !strings.Contains(resp, "confirmed authorization") || eng.cancelled != "" {
		t.Fatalf("unauthorized cancel executed: %s (cancelled=%q)", resp, eng.cancelled)
	}

	// Ticket for another run must not authorize this one.
	grantFor(t, tasks, p.ID, "thr-channel", "run-other", "cancel")
	resp = callWrite(t, h, p.ID, tok, "pm_cancel_run", map[string]any{"runId": "run-guard"})
	if !strings.Contains(resp, "confirmed authorization") || eng.cancelled != "" {
		t.Fatalf("wrong-run ticket authorized cancel: %s", resp)
	}

	// Ticket for another action on the right run must not authorize a cancel.
	grantFor(t, tasks, p.ID, "thr-channel", "run-guard", "approve")
	resp = callWrite(t, h, p.ID, tok, "pm_cancel_run", map[string]any{"runId": "run-guard"})
	if !strings.Contains(resp, "confirmed authorization") || eng.cancelled != "" {
		t.Fatalf("wrong-action ticket authorized cancel: %s", resp)
	}

	// Exact confirmed ticket: allowed exactly once.
	grantFor(t, tasks, p.ID, "thr-channel", "run-guard", "cancel")
	resp = callWrite(t, h, p.ID, tok, "pm_cancel_run", map[string]any{"runId": "run-guard"})
	if strings.Contains(resp, `"isError":true`) || eng.cancelled != "run-guard" {
		t.Fatalf("confirmed cancel denied: %s (cancelled=%q)", resp, eng.cancelled)
	}
	eng.cancelled = ""
	resp = callWrite(t, h, p.ID, tok, "pm_cancel_run", map[string]any{"runId": "run-guard"})
	if !strings.Contains(resp, "confirmed authorization") || eng.cancelled != "" {
		t.Fatalf("ticket was replayable: %s", resp)
	}
}

func TestGuardedGateResumeAndWorkflowDeleteNeedMatchingKind(t *testing.T) {
	_, h, eng, tasks, p := channelActionFixture(t)
	tok := h.Register(p.ID, "thr-channel", "qq:c2c:u1", "agent-a")
	if err := tasks.GuardChannelThread(p.ID, "thr-channel", "qq", "qq:c2c:u1"); err != nil {
		t.Fatal(err)
	}

	args := map[string]any{"runId": "run-guard", "nodeId": "gate-1", "action": "approve"}
	if resp := callWrite(t, h, p.ID, tok, "pm_resume_gate", args); !strings.Contains(resp, "confirmed authorization") {
		t.Fatalf("unauthorized gate resume: %s", resp)
	}
	if eng.resumed.runID != "" {
		t.Fatalf("gate resumed without a ticket: %#v", eng.resumed)
	}
	grantFor(t, tasks, p.ID, "thr-channel", "run-guard", "reject")
	if resp := callWrite(t, h, p.ID, tok, "pm_resume_gate", args); !strings.Contains(resp, "confirmed authorization") {
		t.Fatalf("reject ticket approved a gate: %s", resp)
	}
	grantFor(t, tasks, p.ID, "thr-channel", "run-guard", "approve")
	if resp := callWrite(t, h, p.ID, tok, "pm_resume_gate", args); strings.Contains(resp, `"isError":true`) {
		t.Fatalf("confirmed approve denied: %s", resp)
	}
	if eng.resumed.runID != "run-guard" || eng.resumed.action != "approve" {
		t.Fatalf("gate resume = %#v", eng.resumed)
	}

	del := map[string]any{"workflowId": "wf-guard"}
	if resp := callWrite(t, h, p.ID, tok, "pm_delete_workflow", del); !strings.Contains(resp, "confirmed authorization") {
		t.Fatalf("unauthorized workflow delete: %s", resp)
	}
	// A run-scoped delete ticket must not authorize deleting the workflow.
	grantFor(t, tasks, p.ID, "thr-channel", "run-guard", "delete")
	if resp := callWrite(t, h, p.ID, tok, "pm_delete_workflow", del); !strings.Contains(resp, "confirmed authorization") {
		t.Fatalf("run ticket deleted a workflow: %s", resp)
	}
	grantFor(t, tasks, p.ID, "thr-channel", "wf-guard", "delete")
	if resp := callWrite(t, h, p.ID, tok, "pm_delete_workflow", del); strings.Contains(resp, `"isError":true`) {
		t.Fatalf("confirmed workflow delete denied: %s", resp)
	}
}

func TestUnguardedThreadsKeepExistingBehaviour(t *testing.T) {
	_, h, eng, _, p := channelActionFixture(t)
	// A web/API PM consult thread is never registered as channel-guarded.
	tok := h.Register(p.ID, "thr-web", "alice", "agent-a")
	resp := callWrite(t, h, p.ID, tok, "pm_cancel_run", map[string]any{"runId": "run-guard"})
	if strings.Contains(resp, `"isError":true`) || eng.cancelled != "run-guard" {
		t.Fatalf("unguarded cancel was blocked: %s (cancelled=%q)", resp, eng.cancelled)
	}
}
