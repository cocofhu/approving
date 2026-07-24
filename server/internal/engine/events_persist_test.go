package engine

import (
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

// TestAgentErrPathPersistsEvents asserts that when RunAgent returns an error
// with res.Events populated, saveState writes them to StateRun (aligning
// execAgent/execStructuredAgent/execPlan with execAppPreview).
func TestAgentErrPathPersistsEvents(t *testing.T) {
	autoRetryBackoff = 0
	t.Cleanup(func() { autoRetryBackoff = 5 * time.Second })
	eng, db, p := setupEngineGraphP(t, autoRetryGraph())
	p.failLeft = map[string]int{"risky": 1}
	p.reason = "计划未全部完成,仍有 1 项未完成"
	p.failWithEvents = []models.AcpEvent{{Kind: "message", Text: "fake acp snapshot"}}

	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitRunStatus(t, db, run.ID, "failed")

	var sr models.StateRun
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "risky").First(&sr).Error; err != nil {
		t.Fatalf("load state_run: %v", err)
	}
	if len(sr.Events) == 0 {
		t.Fatal("expected StateRun.Events non-empty on RunAgent err path")
	}
	if sr.Events[0].Text != "fake acp snapshot" {
		t.Errorf("Events[0].Text = %q, want fake acp snapshot", sr.Events[0].Text)
	}
}
