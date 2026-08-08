package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

type recordingRunPaused struct {
	mu  sync.Mutex
	evs []RunPausedEvent
}

func (r *recordingRunPaused) OnRunPaused(ev RunPausedEvent) {
	r.mu.Lock()
	r.evs = append(r.evs, ev)
	r.mu.Unlock()
}

func (r *recordingRunPaused) wait(t *testing.T, n int) []RunPausedEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		if len(r.evs) >= n {
			out := append([]RunPausedEvent(nil), r.evs...)
			r.mu.Unlock()
			return out
		}
		r.mu.Unlock()
		time.Sleep(15 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("want >=%d run-paused events, got %d", n, len(r.evs))
	return nil
}

func (r *recordingRunPaused) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.evs)
}

// A run that stops at a gate has to announce it on both channels: the
// project-level notify and the conversation that dispatched the work. Only the
// first one existed, so a task waiting on a human looked like a task that had
// gone quiet.
func TestRunPausedFiresAlongsideRunNotify(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input", Label: "输入"},
			{ID: "gate", Type: "human_gate", Label: "评审", Config: map[string]any{
				"title": "设计评审",
				"actions": []any{
					map[string]any{"id": "approve", "label": "批准"},
				},
			}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "gate"},
			{ID: "e2", Source: "gate", Target: "output", When: "action == 'approve'"},
		},
	}
	eng, db, _ := setupEngineGraphP(t, g)
	proj := models.Project{ID: "proj-run-paused", Name: "Paused", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.WorkflowDef{}).Where("id = ?", "wf").
		Update("project_id", proj.ID).Error; err != nil {
		t.Fatal(err)
	}

	notify := &recordingRunNotify{}
	paused := &recordingRunPaused{}
	eng.SetRunNotifier(notify)
	eng.SetRunPausedObserver(paused)

	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	waitRunStatus(t, db, run.ID, "waiting_human")

	evs := paused.wait(t, 1)
	if evs[0].RunID != run.ID || evs[0].NodeID != "gate" || evs[0].Iteration < 1 {
		t.Fatalf("unexpected event: %+v", evs[0])
	}
	if evs[0].ProjectID != proj.ID {
		t.Fatalf("project = %q want %q", evs[0].ProjectID, proj.ID)
	}
	notify.wait(t, 1)

	time.Sleep(50 * time.Millisecond)
	if paused.count() != 1 {
		t.Fatalf("paused count=%d want 1", paused.count())
	}
}

// The observer is optional; a run must pause identically without one.
func TestRunPausedWithoutObserver(t *testing.T) {
	eng, db, _ := setupEngineGraphP(t, models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "gate", Type: "human_gate", Label: "G", Config: map[string]any{
				"title":   "确认",
				"actions": []any{map[string]any{"id": "approve", "label": "批准"}},
			}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "gate"},
			{ID: "e2", Source: "gate", Target: "output", When: "action == 'approve'"},
		},
	})
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	waitRunStatus(t, db, run.ID, "waiting_human")
}
