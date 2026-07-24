package engine

import (
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestStructuredGatesFailClosed(t *testing.T) {
	if pass, _ := testGate(`{bad`, false); pass {
		t.Error("malformed test_result should fail gate")
	}
	if pass, _ := reviewGate(`{"verdict":"bogus"}`); pass {
		t.Error("invalid review should fail gate")
	}
	if pass, reason := reviewGate(`{"summary":"s","verdict":"reject"}`); pass || reason == "" {
		t.Errorf("reject verdict should fail gate: pass=%v reason=%q", pass, reason)
	}
}

func TestUnknownNodeTypeFails(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "bad", Type: "not_a_real_type"},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{Source: "input", Target: "bad", Kind: models.EdgeSuccess},
			{Source: "bad", Target: "output", Kind: models.EdgeSuccess},
		},
	}
	eng, db, _ := setupEngineGraphP(t, g)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var r models.Run
		db.First(&r, "id = ?", run.ID)
		if r.Status == "failed" || r.Status == "completed" {
			if r.Status != "failed" {
				t.Fatalf("run status = %q, want failed", r.Status)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("run did not fail on unknown node type")
}
