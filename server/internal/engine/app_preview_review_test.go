package engine

import (
	"testing"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
)

// TestAppPreviewSetPreviewEntersReviewAndGate: healthy set_preview (fake
// PutPreviewPortForTest) must pause waiting_human with BOTH a Gate shell and a
// seeded ReAct review conversation — without requiring node_complete.
func TestAppPreviewSetPreviewEntersReviewAndGate(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "preview", Type: "app_preview", Label: "预览", Config: map[string]any{
				"title": "应用预览",
			}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "preview"},
			{ID: "e2", Source: "preview", Target: "output", When: "action == 'pass'"},
		},
	}
	eng, db, provider := setupEngineGraphP(t, g)
	provider.skipOutcome = true

	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	waitRunStatus(t, db, run.ID, "waiting_human")

	if !eng.host.HasHealthyPreviewPorts(run.ID, "preview") {
		t.Fatal("expected healthy preview port after RunAgent")
	}

	var gate models.Gate
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "preview").First(&gate).Error; err != nil {
		t.Fatalf("expected app_preview gate: %v", err)
	}
	if gate.Resolved {
		t.Fatal("gate should be unresolved while in review")
	}

	var conv models.ReactConversation
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "preview").First(&conv).Error; err != nil {
		t.Fatalf("expected review conversation after set_preview: %v", err)
	}
	if conv.Done || len(conv.Messages) == 0 {
		t.Fatalf("review conv should be open with agent summary: %+v", conv)
	}

	pid, alive := eng.GateReactInfo(run.ID, "preview")
	if pid != "preview" {
		t.Fatalf("GateReactInfo producer=%q want preview", pid)
	}
	if !alive {
		t.Fatal("expected parked review session alive (fakeProvider)")
	}

	// node_complete must not be required on the success path.
	if _, ok := eng.host.TakeOutcome(run.ID, "preview"); ok {
		t.Fatal("node_complete should not be required for app_preview success")
	}
}

func TestReviewEnabledAlwaysForAppPreview(t *testing.T) {
	eng, _, _ := setupEngineGraphP(t, models.Graph{
		Nodes: []models.Node{{ID: "input", Type: "input"}, {ID: "preview", Type: "app_preview"}},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "preview"}},
	})
	c := &execCtx{vars: map[string]any{}}
	if !eng.reviewEnabled(c, &models.Node{ID: "preview", Type: "app_preview"}) {
		t.Fatal("app_preview must always be review-enabled")
	}
	_ = mcp.PreviewPort{}
}
