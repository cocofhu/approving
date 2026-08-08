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

// TestSubmitMRIdempotentReuse: agent reports failure but platform MRReuser finds
// an existing PR/MR → completed with mr_url (no second create).
func TestSubmitMRIdempotentReuse(t *testing.T) {
	eng, db, p := setupEngineGraphP(t, submitMRGraph())
	p.outcomeFailed = true
	p.mrReuseURL = "https://github.com/o/r/pull/42"
	p.mrReuseKind = "already_exists"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")
	var rv models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", run.ID, "mr_url").First(&rv).Error; err != nil {
		t.Fatalf("mr_url not set: %v", err)
	}
	if rv.Value != "https://github.com/o/r/pull/42" {
		t.Fatalf("mr_url = %v", rv.Value)
	}
}

// TestSubmitMRIdempotentFromAgentMessage: RunAgent error text classifies as
// already-exists with URL → completed without MRReuser.
func TestSubmitMRIdempotentFromAgentMessage(t *testing.T) {
	eng, db, p := setupEngineGraphP(t, submitMRGraph())
	p.failLeft = map[string]int{"mr": 1}
	p.reason = "a pull request for branch feature/x already exists: https://gitlab.com/g/p/-/merge_requests/9"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")
	var rv models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", run.ID, "mr_url").First(&rv).Error; err != nil {
		t.Fatalf("mr_url not set: %v", err)
	}
	if rv.Value != "https://gitlab.com/g/p/-/merge_requests/9" {
		t.Fatalf("mr_url = %v", rv.Value)
	}
}
