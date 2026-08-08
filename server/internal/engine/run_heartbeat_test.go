package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

type recordingHeartbeat struct {
	mu  sync.Mutex
	evs []RunHeartbeatEvent
}

func (r *recordingHeartbeat) OnRunHeartbeat(ev RunHeartbeatEvent) {
	r.mu.Lock()
	r.evs = append(r.evs, ev)
	r.mu.Unlock()
}

func (r *recordingHeartbeat) settle() []RunHeartbeatEvent {
	// The observer runs in its own goroutine; give it a moment to land.
	time.Sleep(120 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RunHeartbeatEvent(nil), r.evs...)
}

// seedRunningRun writes a run that has been going for the given duration,
// without executing anything: the sweep only reads state, and driving a real
// long run through the engine would take as long as the threshold.
func seedRunningRun(t *testing.T, eng *Engine, projectID, runID, nodeID string, age time.Duration) {
	t.Helper()
	graph := models.Graph{Nodes: []models.Node{{ID: nodeID, Type: "agent", Label: "跑测试"}}}
	wf := models.WorkflowDef{ID: "wf-" + runID, ProjectID: projectID, Name: "wf " + runID, Status: "published"}
	if err := eng.db.Create(&wf).Error; err != nil {
		t.Fatal(err)
	}
	run := models.Run{
		ID: runID, WorkflowID: wf.ID, Status: "running",
		StartedAt: time.Now().Add(-age), Graph: graph,
	}
	if err := eng.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := eng.db.Create(&models.StateRun{
		RunID: runID, NodeID: nodeID, Status: "running", Iteration: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func heartbeatEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	eng, db, _ := setupEngineGraphP(t, models.Graph{
		Nodes: []models.Node{{ID: "input", Type: "input"}, {ID: "output", Type: "output"}},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "output"}},
	})
	proj := models.Project{ID: "proj-heartbeat", Name: "HB", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	return eng, proj.ID
}

// TestSweepReportsRunsThatHaveBeenGoingAWhile is the gap this closes: between
// two reportable edges a slow run emits nothing, so it is indistinguishable
// from one that hung.
func TestSweepReportsRunsThatHaveBeenGoingAWhile(t *testing.T) {
	eng, projectID := heartbeatEngine(t)
	seedRunningRun(t, eng, projectID, "run-long", "build", 90*time.Minute)

	hb := &recordingHeartbeat{}
	eng.SetRunHeartbeatObserver(hb)
	eng.SweepRunHeartbeats(30 * time.Minute)

	evs := hb.settle()
	if len(evs) != 1 {
		t.Fatalf("want one heartbeat, got %+v", evs)
	}
	if evs[0].RunID != "run-long" || evs[0].ProjectID != projectID {
		t.Fatalf("unexpected event: %+v", evs[0])
	}
	if evs[0].NodeID != "build" || evs[0].NodeLabel != "跑测试" {
		t.Fatalf("event did not say where the run is: %+v", evs[0])
	}
	if evs[0].RunningFor < 89*time.Minute {
		t.Fatalf("running-for = %v; the conversation layer needs this to say how long", evs[0].RunningFor)
	}
}

// TestSweepIgnoresYoungAndSettledRuns: a run that finishes quickly needs no
// reassurance, and a finished one must never be reported as still going.
func TestSweepIgnoresYoungAndSettledRuns(t *testing.T) {
	eng, projectID := heartbeatEngine(t)
	seedRunningRun(t, eng, projectID, "run-young", "build", 2*time.Minute)
	seedRunningRun(t, eng, projectID, "run-done", "build", 90*time.Minute)
	if err := eng.db.Model(&models.Run{}).Where("id = ?", "run-done").
		Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	// A run that stopped for a person already announced itself through the
	// pause event; saying "still going" on top of that would be wrong.
	seedRunningRun(t, eng, projectID, "run-waiting", "build", 90*time.Minute)
	if err := eng.db.Model(&models.Run{}).Where("id = ?", "run-waiting").
		Update("status", "waiting_human").Error; err != nil {
		t.Fatal(err)
	}

	hb := &recordingHeartbeat{}
	eng.SetRunHeartbeatObserver(hb)
	eng.SweepRunHeartbeats(30 * time.Minute)

	if evs := hb.settle(); len(evs) != 0 {
		t.Fatalf("swept runs nobody needs reassuring about: %+v", evs)
	}
}

// TestSweepIsInertWithoutAnObserver: the sweeper ticks on a timer whether or
// not an IM channel is configured.
func TestSweepIsInertWithoutAnObserver(t *testing.T) {
	eng, projectID := heartbeatEngine(t)
	seedRunningRun(t, eng, projectID, "run-long", "build", 90*time.Minute)
	eng.SweepRunHeartbeats(30 * time.Minute)
	eng.SetRunHeartbeatObserver(&recordingHeartbeat{})
	eng.SweepRunHeartbeats(0)
}
