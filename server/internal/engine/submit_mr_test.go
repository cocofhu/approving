package engine

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func submitMRGraph() models.Graph {
	return models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "mr", Type: "submit_mr", Config: map[string]any{"target_branch": "main"}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "mr"},
			{ID: "e2", Source: "mr", Target: "output"},
		},
	}
}

// TestSubmitMRNode exercises execSubmitMR after verifyMR removal: success depends
// on node_complete (and optional outputs.mr_url), not platform git/glab checks.
func TestSubmitMRNode(t *testing.T) {
	// Clean mark + mr_url → completed, mr_url exported.
	eng, db, p := setupEngineGraphP(t, submitMRGraph())
	p.mrURL = "http://gitlab/mr/1"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")
	var rv models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", run.ID, "mr_url").First(&rv).Error; err != nil {
		t.Fatalf("mr_url var not persisted: %v", err)
	}
	if rv.Value != "http://gitlab/mr/1" {
		t.Fatalf("mr_url = %v, want http://gitlab/mr/1", rv.Value)
	}

	// Missing node_complete → failed.
	eng2, db2, p2 := setupEngineGraphP(t, submitMRGraph())
	p2.mrURL = "http://gitlab/mr/1"
	p2.skipOutcome = true
	run2, err := eng2.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db2, run2.ID, "failed")

	// Agent-reported failure → failed.
	eng3, db3, p3 := setupEngineGraphP(t, submitMRGraph())
	p3.mrURL = "http://gitlab/mr/2"
	p3.outcomeFailed = true
	run3, err := eng3.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db3, run3.ID, "failed")

	// Empty mr_url with success outcome → still completed (no verifyMR gate).
	eng4, db4, p4 := setupEngineGraphP(t, submitMRGraph())
	p4.mrURL = ""
	run4, err := eng4.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db4, run4.ID, "completed")
}
