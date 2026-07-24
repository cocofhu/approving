package services

import (
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func reactGraph(autoVar string) models.Graph {
	return models.Graph{
		Nodes: []models.Node{
			{ID: "in", Type: "input", Label: "Start"},
			{ID: "react", Type: "react", Label: "需求澄清", Config: map[string]any{"auto_var": autoVar}},
			{ID: "out", Type: "output", Label: "End"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "in", Target: "react"},
			{ID: "e2", Source: "react", Target: "out"},
		},
	}
}

func TestAllPendingInboxItems(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	now := time.Now().UTC().Truncate(time.Second)
	gateAt := now.Add(-15 * time.Minute)
	clarifyAt := now.Add(-2 * time.Minute)

	db.Create(&models.Run{
		ID: "run-gate", WorkflowID: "wf1", WorkflowName: "API 重构",
		Title: "网关鉴权改造", Status: "waiting_human", StartedAt: now.Add(-time.Hour), Graph: validGraph(),
	})
	db.Create(&models.Gate{
		RunID: "run-gate", NodeID: "gate-proposal", WorkflowID: "wf1", WorkflowName: "API 重构",
		Title: "方案评审门禁", Resolved: false, RequestedAt: gateAt,
	})

	db.Create(&models.Run{
		ID: "run-clarify", WorkflowID: "wf2", WorkflowName: "功能迭代工作流",
		Title: "收件箱标题透出", Status: "waiting_human", StartedAt: now.Add(-30 * time.Minute), Graph: reactGraph(""),
	})
	db.Create(&models.ReactConversation{
		RunID: "run-clarify", NodeID: "react", Iteration: 1, Done: false,
		Messages: []models.ReactMessage{
			{Role: "agent", Text: "hello", At: clarifyAt.Format(time.RFC3339)},
		},
	})

	// Terminal run — excluded.
	db.Create(&models.Run{ID: "run-done", Status: "completed", StartedAt: now, Graph: reactGraph("")})
	db.Create(&models.ReactConversation{RunID: "run-done", NodeID: "react", Done: false})

	// Done conversation — excluded.
	db.Create(&models.Run{ID: "run-finished", Status: "waiting_human", StartedAt: now, Graph: reactGraph("")})
	db.Create(&models.ReactConversation{RunID: "run-finished", NodeID: "react", Done: true})

	// Auto-var react — excluded.
	db.Create(&models.Run{ID: "run-auto", Status: "waiting_human", StartedAt: now, Graph: reactGraph("auto_clarify")})
	db.Create(&models.RunVariable{RunID: "run-auto", Name: "auto_clarify", Type: "bool", Value: true})
	db.Create(&models.ReactConversation{RunID: "run-auto", NodeID: "react", Done: false})

	// Duplicate iteration — only one clarify item.
	db.Create(&models.ReactConversation{RunID: "run-clarify", NodeID: "react", Iteration: 1, Done: false,
		Messages: []models.ReactMessage{{Role: "agent", Text: "dup", At: clarifyAt.Format(time.RFC3339)}}})
	db.Create(&models.StateRun{RunID: "run-clarify", NodeID: "react", Iteration: 1, Status: "waiting_human"})

	// Failed react node with stale conversation — excluded from inbox.
	db.Create(&models.Run{ID: "run-sbx-fail", Status: "running", StartedAt: now, Graph: reactGraph("")})
	db.Create(&models.ReactConversation{RunID: "run-sbx-fail", NodeID: "react", Iteration: 1, Done: false,
		Messages: []models.ReactMessage{{Role: "agent", Text: "err", At: now.Format(time.RFC3339)}}})
	db.Create(&models.StateRun{RunID: "run-sbx-fail", NodeID: "react", Iteration: 1, Status: "failed",
		Error: "sandbox setup failed: docker pull failed"})

	items := s.AllPendingInboxItems()
	if len(items) != 2 {
		t.Fatalf("expected 2 inbox items, got %d", len(items))
	}
	for _, it := range items {
		if c, ok := it.(ClarifyInboxItem); ok && c.RunID == "run-sbx-fail" {
			t.Fatalf("failed react node must not appear in clarify inbox")
		}
	}

	clarify, ok := items[0].(ClarifyInboxItem)
	if !ok || clarify.Type != "clarify" || clarify.Label != "需求澄清" {
		t.Fatalf("newest should be clarify: %+v %v", items[0], ok)
	}
	if clarify.Kind != "clarify" {
		t.Fatalf("react clarify kind = %q, want clarify", clarify.Kind)
	}
	if clarify.RunTitle != "收件箱标题透出" {
		t.Fatalf("clarify runTitle = %q, want 收件箱标题透出", clarify.RunTitle)
	}
	gate, ok := items[1].(GateInboxItem)
	if !ok || gate.Type != "gate" || gate.Title != "方案评审门禁" {
		t.Fatalf("second should be gate: %+v %v", items[1], ok)
	}
	if gate.RunTitle != "网关鉴权改造" {
		t.Fatalf("gate runTitle = %q, want 网关鉴权改造", gate.RunTitle)
	}
}

func TestPendingInboxItemsOmitsEmptyRunTitle(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	now := time.Now().UTC().Truncate(time.Second)

	db.Create(&models.Run{
		ID: "run-empty-title", WorkflowID: "wf1", WorkflowName: "API 重构",
		Title: "", Status: "waiting_human", StartedAt: now, Graph: validGraph(),
	})
	db.Create(&models.Gate{
		RunID: "run-empty-title", NodeID: "gate-proposal", WorkflowID: "wf1", WorkflowName: "API 重构",
		Title: "方案评审门禁", Resolved: false, RequestedAt: now,
	})

	items := s.AllPendingInboxItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 inbox item, got %d", len(items))
	}
	gate, ok := items[0].(GateInboxItem)
	if !ok {
		t.Fatalf("expected gate, got %T", items[0])
	}
	if gate.RunTitle != "" {
		t.Fatalf("empty run title should omit/empty runTitle, got %q", gate.RunTitle)
	}
}

func reviewCapableGraph(nodeType, label string) models.Graph {
	return models.Graph{
		Nodes: []models.Node{
			{ID: "in", Type: "input", Label: "Start"},
			{ID: nodeType, Type: nodeType, Label: label},
			{ID: "out", Type: "output", Label: "End"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "in", Target: nodeType},
			{ID: "e2", Source: nodeType, Target: "out"},
		},
	}
}

func TestPendingClarificationsKind(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	now := time.Now().UTC().Truncate(time.Second)

	// react → kind=clarify, type remains clarify
	db.Create(&models.Run{
		ID: "run-react", WorkflowID: "wf-r", WorkflowName: "澄清流",
		Status: "waiting_human", StartedAt: now, Graph: reactGraph(""),
	})
	db.Create(&models.ReactConversation{
		RunID: "run-react", NodeID: "react", Iteration: 1, Done: false,
		Messages: []models.ReactMessage{{Role: "agent", Text: "q", At: now.Format(time.RFC3339)}},
	})
	db.Create(&models.StateRun{RunID: "run-react", NodeID: "react", Iteration: 1, Status: "waiting_human"})

	// research review session → kind=review, type still clarify
	db.Create(&models.Run{
		ID: "run-research", WorkflowID: "wf-rs", WorkflowName: "调研流",
		Status: "waiting_human", StartedAt: now, Graph: reviewCapableGraph("research", "调研"),
	})
	db.Create(&models.ReactConversation{
		RunID: "run-research", NodeID: "research", Iteration: 1, Done: false,
		Messages: []models.ReactMessage{{Role: "agent", Text: "review", At: now.Add(time.Minute).Format(time.RFC3339)}},
	})
	db.Create(&models.StateRun{RunID: "run-research", NodeID: "research", Iteration: 1, Status: "waiting_human"})

	// proposal review session → kind=review
	db.Create(&models.Run{
		ID: "run-proposal", WorkflowID: "wf-p", WorkflowName: "方案流",
		Status: "waiting_human", StartedAt: now, Graph: reviewCapableGraph("proposal", "方案"),
	})
	db.Create(&models.ReactConversation{
		RunID: "run-proposal", NodeID: "proposal", Iteration: 1, Done: false,
		Messages: []models.ReactMessage{{Role: "agent", Text: "proposals", At: now.Add(2 * time.Minute).Format(time.RFC3339)}},
	})
	db.Create(&models.StateRun{RunID: "run-proposal", NodeID: "proposal", Iteration: 1, Status: "waiting_human"})

	items := s.AllPendingInboxItems()
	byRun := map[string]ClarifyInboxItem{}
	for _, it := range items {
		if c, ok := it.(ClarifyInboxItem); ok {
			byRun[c.RunID] = c
		}
	}
	for _, id := range []string{"run-react", "run-research", "run-proposal"} {
		if _, ok := byRun[id]; !ok {
			t.Fatalf("missing inbox item for %s among %d items", id, len(items))
		}
	}
	if byRun["run-react"].Type != "clarify" || byRun["run-react"].Kind != "clarify" {
		t.Fatalf("react: type=%q kind=%q", byRun["run-react"].Type, byRun["run-react"].Kind)
	}
	if byRun["run-research"].Type != "clarify" || byRun["run-research"].Kind != "review" {
		t.Fatalf("research: type=%q kind=%q", byRun["run-research"].Type, byRun["run-research"].Kind)
	}
	if byRun["run-proposal"].Type != "clarify" || byRun["run-proposal"].Kind != "review" {
		t.Fatalf("proposal: type=%q kind=%q", byRun["run-proposal"].Type, byRun["run-proposal"].Kind)
	}
	if byRun["run-research"].Label != "调研" || byRun["run-proposal"].Label != "方案" {
		t.Fatalf("labels: research=%q proposal=%q", byRun["run-research"].Label, byRun["run-proposal"].Label)
	}
}

func TestClarifyInboxKind(t *testing.T) {
	if got := clarifyInboxKind(&models.Node{Type: "react"}); got != "clarify" {
		t.Fatalf("react → %q", got)
	}
	if got := clarifyInboxKind(&models.Node{Type: "research"}); got != "review" {
		t.Fatalf("research → %q", got)
	}
	if got := clarifyInboxKind(&models.Node{Type: "proposal"}); got != "review" {
		t.Fatalf("proposal → %q", got)
	}
	if got := clarifyInboxKind(&models.Node{Type: "proposal_select"}); got != "clarify" {
		t.Fatalf("proposal_select is gate channel, not review kind: %q", got)
	}
	if got := clarifyInboxKind(nil); got != "clarify" {
		t.Fatalf("nil → %q", got)
	}
}

func TestReactAutoEnabled(t *testing.T) {
	node := &models.Node{Config: map[string]any{"auto_var": "auto_flag"}}
	if reactAutoEnabled(node, map[string]any{"auto_flag": true}) != true {
		t.Fatal("truthy auto var")
	}
	if reactAutoEnabled(node, map[string]any{"auto_flag": false}) != false {
		t.Fatal("falsy auto var")
	}
	if reactAutoEnabled(&models.Node{Config: map[string]any{}}, nil) != false {
		t.Fatal("empty auto_var")
	}
}

func TestLatestMessageAt(t *testing.T) {
	fallback := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := fallback.Add(time.Hour)
	msgs := []models.ReactMessage{
		{At: fallback.Format(time.RFC3339)},
		{At: later.Format(time.RFC3339)},
	}
	got := latestMessageAt(msgs, fallback)
	if !got.Equal(later) {
		t.Fatalf("latest: got %v want %v", got, later)
	}
	if !latestMessageAt(nil, fallback).Equal(fallback) {
		t.Fatal("fallback when no messages")
	}
}
