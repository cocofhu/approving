package engine

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

type recordingRunTerminal struct {
	mu  sync.Mutex
	evs []RunTerminalEvent
}

func (r *recordingRunTerminal) OnRunTerminal(ev RunTerminalEvent) {
	r.mu.Lock()
	r.evs = append(r.evs, ev)
	r.mu.Unlock()
}

func (r *recordingRunTerminal) wait(t *testing.T, n int) []RunTerminalEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		if len(r.evs) >= n {
			out := append([]RunTerminalEvent(nil), r.evs...)
			r.mu.Unlock()
			return out
		}
		r.mu.Unlock()
		time.Sleep(15 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("want >=%d terminal events, got %d", n, len(r.evs))
	return nil
}

func (r *recordingRunTerminal) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.evs)
}

// A run that succeeds is the case the old notification path never covered — it
// only knew about waiting_human and failed, so a user who delegated work was
// never told it had finished.
func TestRunTerminalFiresOnceOnSuccess(t *testing.T) {
	eng, db, _ := setupEngineGraphP(t, models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input", Label: "输入"},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "output"}},
	})
	proj := models.Project{ID: "proj-terminal", Name: "T", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.WorkflowDef{}).Where("id = ?", "wf").
		Update("project_id", proj.ID).Error; err != nil {
		t.Fatal(err)
	}

	rec := &recordingRunTerminal{}
	eng.SetRunTerminalObserver(rec)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	waitRunStatus(t, db, run.ID, "completed")

	evs := rec.wait(t, 1)
	if evs[0].Status != "completed" || evs[0].RunID != run.ID || evs[0].ProjectID != proj.ID {
		t.Fatalf("terminal event = %+v", evs[0])
	}
	// finish() is the only writer of a terminal status, so the conclusion must
	// reach the conversation exactly once.
	time.Sleep(50 * time.Millisecond)
	if rec.count() != 1 {
		t.Fatalf("terminal event count = %d want 1", rec.count())
	}
}

// A failure carries the reason so the conversation can explain the outcome
// without anyone reading logs.
func TestRunTerminalCarriesFailureReason(t *testing.T) {
	eng, db, prov := setupEngineGraphP(t, models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input", Label: "输入"},
			{ID: "boom", Type: "agent", Label: "实现", Config: map[string]any{
				"prompt": "x", "produces": "out.md",
			}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "boom"},
			{ID: "e2", Source: "boom", Target: "output"},
		},
	})
	prov.failLeft = map[string]int{"boom": 99}
	proj := models.Project{ID: "proj-terminal-fail", Name: "TF", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.WorkflowDef{}).Where("id = ?", "wf").
		Update("project_id", proj.ID).Error; err != nil {
		t.Fatal(err)
	}

	rec := &recordingRunTerminal{}
	eng.SetRunTerminalObserver(rec)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	waitRunStatus(t, db, run.ID, "failed")

	evs := rec.wait(t, 1)
	if evs[0].Status != "failed" || strings.TrimSpace(evs[0].FailureSummary) == "" {
		t.Fatalf("failed terminal event = %+v want a reason", evs[0])
	}
}

// Not every status change is an ending. A non-terminal status must not be
// announced as one, and a run the engine cannot place in a project has no
// conversation to report to.
func TestRunTerminalIgnoresNonTerminalAndUnknownRuns(t *testing.T) {
	eng, _, _ := setupEngineGraphP(t, models.Graph{
		Nodes: []models.Node{{ID: "input", Type: "input"}},
	})
	rec := &recordingRunTerminal{}
	eng.SetRunTerminalObserver(rec)
	for _, status := range []string{"running", "waiting_human", "paused", ""} {
		eng.fireRunTerminal("run-anything", status)
	}
	eng.fireRunTerminal("run-does-not-exist", "completed")
	time.Sleep(80 * time.Millisecond)
	if rec.count() != 0 {
		t.Fatalf("observer fired %d times for non-endings", rec.count())
	}
}
