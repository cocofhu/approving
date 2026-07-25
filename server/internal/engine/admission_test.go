package engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"

	"gorm.io/gorm"
)

// blockingFake blocks RunAgent on configured node ids while held is true, so
// admission tests can fill concurrency slots without finishing runs. releaseRun
// allows a single run through while global held remains set (FIFO tests).
// AbortRun (runtime.RunAborter) bumps abortGen so in-flight calls exit with an
// error without emitting node_complete; a later ResumeFrom visit starts with
// the new gen and is not sticky-aborted.
type blockingFake struct {
	fakeProvider
	mu           sync.Mutex
	blocking     map[string]bool
	releasedRuns map[string]bool
	abortGen     map[string]uint64
	held         atomic.Bool
}

func newBlockingFake(host *mcp.Host) *blockingFake {
	b := &blockingFake{
		fakeProvider: fakeProvider{host: host},
		blocking:     map[string]bool{},
		releasedRuns: map[string]bool{},
		abortGen:     map[string]uint64{},
	}
	b.held.Store(true)
	return b
}

func (b *blockingFake) blockNode(nodeID string) {
	b.mu.Lock()
	b.blocking[nodeID] = true
	b.mu.Unlock()
}

func (b *blockingFake) ReleaseAll() {
	b.held.Store(false)
}

func (b *blockingFake) releaseRun(runID string) {
	b.mu.Lock()
	b.releasedRuns[runID] = true
	b.mu.Unlock()
}

// AbortRun implements runtime.RunAborter for cancel/resume zombie tests.
func (b *blockingFake) AbortRun(runID string) {
	b.mu.Lock()
	b.abortGen[runID]++
	b.mu.Unlock()
}

func (b *blockingFake) RunAgent(ctx context.Context, req runtime.NodeReq) (runtime.NodeResult, error) {
	b.mu.Lock()
	wait := b.blocking[req.NodeID]
	startAbort := b.abortGen[req.RunID]
	b.mu.Unlock()
	for wait && b.held.Load() {
		b.mu.Lock()
		aborted := b.abortGen[req.RunID] > startAbort
		released := b.releasedRuns[req.RunID]
		b.mu.Unlock()
		if aborted {
			return runtime.NodeResult{}, errors.New("run aborted")
		}
		if released {
			break
		}
		select {
		case <-ctx.Done():
			return runtime.NodeResult{}, ctx.Err()
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	b.mu.Lock()
	aborted := b.abortGen[req.RunID] > startAbort
	b.mu.Unlock()
	if aborted {
		return runtime.NodeResult{}, errors.New("run aborted")
	}
	return b.fakeProvider.RunAgent(ctx, req)
}

func slowGraph() models.Graph {
	return models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "work", Type: "agent", Config: map[string]any{"skill_profile": "t", "prompt": "work"}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "work"},
			{ID: "e2", Source: "work", Target: "output", Kind: models.EdgeSuccess},
		},
	}
}

func gateThenWorkGraph() models.Graph {
	return models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "gate", Type: "human_gate", Config: map[string]any{
				"actions": []any{map[string]any{"id": "ok", "label": "OK"}},
			}},
			{ID: "work", Type: "agent", Config: map[string]any{"skill_profile": "t", "prompt": "work"}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "gate"},
			{ID: "e2", Source: "gate", Target: "work", Kind: models.EdgeSuccess},
			{ID: "e3", Source: "work", Target: "output", Kind: models.EdgeSuccess},
		},
	}
}

func countRunsByStatus(t *testing.T, db *gorm.DB, status string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&models.Run{}).Where("status = ?", status).Count(&n).Error; err != nil {
		t.Fatalf("count %s: %v", status, err)
	}
	return n
}

func setupBlockingEngine(t *testing.T, g models.Graph, maxRuns int) (*Engine, *gorm.DB, *blockingFake) {
	t.Helper()
	eng, db, _ := setupEngineGraphP(t, g)
	p := newBlockingFake(eng.host)
	p.blockNode("work")
	eng.provider = p
	eng.SetMaxConcurrent(maxRuns)
	return eng, db, p
}

func setupBlockingEngineDual(t *testing.T, maxRuns int) (*Engine, *gorm.DB, *blockingFake) {
	t.Helper()
	eng, db, _ := setupEngineGraphP(t, slowGraph())
	if err := db.Create(&models.WorkflowDef{
		ID: "wf-gate", Name: "wf-gate", Status: "published", Version: 1, Graph: gateThenWorkGraph(),
	}).Error; err != nil {
		t.Fatalf("create gate workflow: %v", err)
	}
	if err := db.Create(&models.WorkflowVersion{WorkflowID: "wf-gate", Version: 1, Graph: gateThenWorkGraph()}).Error; err != nil {
		t.Fatalf("create gate version: %v", err)
	}
	p := newBlockingFake(eng.host)
	p.blockNode("work")
	eng.provider = p
	eng.SetMaxConcurrent(maxRuns)
	return eng, db, p
}

func waitAdmissionUntil(t *testing.T, fn func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func admissionFromInDB(t *testing.T, db *gorm.DB, runID string) string {
	t.Helper()
	var run models.Run
	if err := db.First(&run, "id = ?", runID).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Checkpoints == nil {
		return ""
	}
	cp, ok := run.Checkpoints[admissionFromCheckpoint]
	if !ok {
		return ""
	}
	node, _ := cp["node"].(string)
	return node
}

// TestAdmissionConcurrentLimit verifies max_concurrent_runs caps visible running
// runs and queues the overflow (f1/f2).
func TestAdmissionConcurrentLimit(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 5)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := eng.StartRun("wf", nil, "test"); err != nil {
				t.Errorf("StartRun: %v", err)
			}
		}()
	}
	wg.Wait()

	waitAdmissionUntil(t, func() bool {
		return countRunsByStatus(t, db, "running") == 5 && countRunsByStatus(t, db, "queued") == 3
	}, 3*time.Second)

	if got := countRunsByStatus(t, db, "running"); got != 5 {
		t.Fatalf("running = %d, want 5", got)
	}
	if got := countRunsByStatus(t, db, "queued"); got != 3 {
		t.Fatalf("queued = %d, want 3", got)
	}

	p.ReleaseAll()
	waitAdmissionUntil(t, func() bool {
		return countRunsByStatus(t, db, "running") == 0 && countRunsByStatus(t, db, "queued") == 0
	}, 5*time.Second)
}

// TestAdmissionSamePriorityFIFO admits the oldest queued run first when a slot
// frees and all queued runs share the same priority (default normal).
func TestAdmissionSamePriorityFIFO(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 2)

	runA, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun A: %v", err)
	}
	runB, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun B: %v", err)
	}
	runC, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun C: %v", err)
	}

	waitAdmissionUntil(t, func() bool {
		return countRunsByStatus(t, db, "running") == 2 && countRunsByStatus(t, db, "queued") == 1
	}, 3*time.Second)

	waitAdmissionUntil(t, func() bool {
		var sr models.StateRun
		return db.Where("run_id = ? AND node_id = ? AND status = ?", runA.ID, "work", "running").First(&sr).Error == nil
	}, 3*time.Second)

	p.releaseRun(runA.ID)
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runA.ID)
		return r.Status == "completed"
	}, 5*time.Second)
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runC.ID)
		return r.Status == "running"
	}, 5*time.Second)

	var b models.Run
	if err := db.First(&b, "id = ?", runB.ID).Error; err != nil {
		t.Fatalf("load run B: %v", err)
	}
	if b.Status != "running" {
		t.Fatalf("run B status = %q, want running (still holds slot)", b.Status)
	}
}

// TestAdmissionWaitingHumanReleaseAndResumeQueued covers waiting_human slot release
// and resume under full load entering queued instead of ghost running (f4).
func TestAdmissionWaitingHumanReleaseAndResumeQueued(t *testing.T) {
	eng, db, p := setupBlockingEngineDual(t, 2)

	runGate, err := eng.StartRun("wf-gate", nil, "test")
	if err != nil {
		t.Fatalf("StartRun gate run: %v", err)
	}
	waitRunStatus(t, db, runGate.ID, "waiting_human")

	runB, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun B: %v", err)
	}
	waitRunStatus(t, db, runB.ID, "running")

	runC, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun C: %v", err)
	}
	waitRunStatus(t, db, runC.ID, "running")

	runD, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun D: %v", err)
	}
	waitRunStatus(t, db, runD.ID, "queued")

	if err := eng.ResumeGate(runGate.ID, "gate", "ok", nil); err != nil {
		t.Fatalf("ResumeGate: %v", err)
	}

	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runGate.ID)
		return r.Status == "queued"
	}, 3*time.Second)

	var gate, b, c, d models.Run
	db.First(&gate, "id = ?", runGate.ID)
	db.First(&b, "id = ?", runB.ID)
	db.First(&c, "id = ?", runC.ID)
	db.First(&d, "id = ?", runD.ID)
	if gate.Status != "queued" {
		t.Fatalf("gate run status = %q, want queued", gate.Status)
	}
	if from := admissionFromInDB(t, db, runGate.ID); from != "work" {
		t.Fatalf("admission from = %q, want work", from)
	}
	if b.Status != "running" || c.Status != "running" || d.Status != "queued" {
		t.Fatalf("expected B,C running and D queued; got %q %q %q", b.Status, c.Status, d.Status)
	}
	if countRunsByStatus(t, db, "running") != 2 {
		t.Fatalf("running count must match occupied slots")
	}

	p.ReleaseAll()
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runGate.ID)
		return r.Status == "running" || r.Status == "completed"
	}, 5*time.Second)
}

// TestExecuteEarlyExitRequeuesRun verifies acquireExecuteSlot failure rolls DB
// back to queued so sem release in runAdmitted defer cannot drift below running
// count.
func TestExecuteEarlyExitRequeuesRun(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 5)

	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Wait until the live driver holds the execute slot while blocked on "work".
	// Pre-setting execRuns before that races with StartRun's runAdmitted
	// (execute from "input"): both then time out, and the "input" requeue can
	// win — leaving admission from = "input" (CI flake).
	waitAdmissionUntil(t, func() bool {
		eng.execMu.Lock()
		held := eng.execRuns[run.ID]
		eng.execMu.Unlock()
		if !held {
			return false
		}
		var r models.Run
		if err := db.First(&r, "id = ?", run.ID).Error; err != nil {
			return false
		}
		if r.Status != "running" {
			return false
		}
		for _, te := range r.Trace {
			if te.NodeID == "work" && te.Event == "enter" {
				return true
			}
		}
		return false
	}, 3*time.Second)

	// Second execute cannot acquire while the live driver still holds the slot.
	eng.execute(run.ID, "work")

	var r models.Run
	db.First(&r, "id = ?", run.ID)
	if r.Status != "queued" {
		t.Fatalf("status = %q, want queued after execute early exit", r.Status)
	}
	from, ok := eng.admissionFromNode(r)
	if !ok || from != "work" {
		t.Fatalf("admission from = %q ok=%v, want work true", from, ok)
	}

	p.ReleaseAll()
}

// TestAdmissionMixedPriority promotes higher priority queued runs before lower
// ones, regardless of creation order.
func TestAdmissionMixedPriority(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 1)

	runLow, err := eng.StartRunWithPriority("wf", nil, "test", "low")
	if err != nil {
		t.Fatalf("StartRun low: %v", err)
	}
	waitRunStatus(t, db, runLow.ID, "running")

	runNormal, err := eng.StartRunWithPriority("wf", nil, "test", "normal")
	if err != nil {
		t.Fatalf("StartRun normal: %v", err)
	}
	runHigh, err := eng.StartRunWithPriority("wf", nil, "test", "high")
	if err != nil {
		t.Fatalf("StartRun high: %v", err)
	}
	waitAdmissionUntil(t, func() bool {
		return countRunsByStatus(t, db, "queued") == 2
	}, 3*time.Second)

	p.releaseRun(runLow.ID)
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runLow.ID)
		return r.Status == "completed"
	}, 5*time.Second)
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runHigh.ID)
		return r.Status == "running"
	}, 5*time.Second)

	var normal models.Run
	db.First(&normal, "id = ?", runNormal.ID)
	if normal.Status != "queued" {
		t.Fatalf("normal status = %q, want queued (high admitted first)", normal.Status)
	}

	p.ReleaseAll()
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runNormal.ID)
		return r.Status == "running" || r.Status == "completed"
	}, 5*time.Second)
}

// TestAdmissionRequeueUsesUpdatedPriority verifies that raising priority while
// queued immediately affects the next claim.
func TestAdmissionRequeueUsesUpdatedPriority(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 1)

	runHold, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun hold: %v", err)
	}
	waitRunStatus(t, db, runHold.ID, "running")

	runEarly, err := eng.StartRunWithPriority("wf", nil, "test", "low")
	if err != nil {
		t.Fatalf("StartRun early low: %v", err)
	}
	runLate, err := eng.StartRunWithPriority("wf", nil, "test", "low")
	if err != nil {
		t.Fatalf("StartRun late low: %v", err)
	}
	waitAdmissionUntil(t, func() bool {
		return countRunsByStatus(t, db, "queued") == 2
	}, 3*time.Second)

	if _, err := eng.UpdateRunPriority(runLate.ID, "high"); err != nil {
		t.Fatalf("UpdateRunPriority: %v", err)
	}

	p.releaseRun(runHold.ID)
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runHold.ID)
		return r.Status == "completed"
	}, 5*time.Second)
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", runLate.ID)
		return r.Status == "running"
	}, 5*time.Second)

	var early models.Run
	db.First(&early, "id = ?", runEarly.ID)
	if early.Status != "queued" {
		t.Fatalf("early status = %q, want queued after late was raised to high", early.Status)
	}
	p.ReleaseAll()
}

// TestStartRunDefaultPriorityIsNormal ensures empty/missing priority persists as normal.
func TestStartRunDefaultPriorityIsNormal(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 5)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	var stored models.Run
	if err := db.First(&stored, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if stored.Priority != models.PriorityNormal {
		t.Fatalf("priority = %d, want %d", stored.Priority, models.PriorityNormal)
	}
	p.ReleaseAll()
}

// TestUpdateRunPriorityTerminalRejected covers completed/failed/cancelled rejection.
func TestUpdateRunPriorityTerminalRejected(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 5)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	p.ReleaseAll()
	waitAdmissionUntil(t, func() bool {
		var r models.Run
		db.First(&r, "id = ?", run.ID)
		return r.Status == "completed"
	}, 5*time.Second)

	if _, err := eng.UpdateRunPriority(run.ID, "high"); err == nil {
		t.Fatal("expected error updating terminal run priority")
	}
}

// TestUpdateRunPriorityInvalidLabel rejects unknown priority strings.
func TestUpdateRunPriorityInvalidLabel(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 5)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := eng.UpdateRunPriority(run.ID, "urgent"); err == nil {
		t.Fatal("expected invalid priority error")
	}
	var stored models.Run
	db.First(&stored, "id = ?", run.ID)
	if stored.Priority != models.PriorityNormal {
		t.Fatalf("priority mutated on invalid update: %d", stored.Priority)
	}
	p.ReleaseAll()
}

// TestStartRunFromPublishedForcesNormal ensures v1/non-UI path cannot set priority.
func TestStartRunFromPublishedForcesNormal(t *testing.T) {
	eng, db, p := setupBlockingEngine(t, slowGraph(), 5)
	// Mark workflow published with a version snapshot.
	if err := db.Model(&models.WorkflowDef{}).Where("id = ?", "wf").Updates(map[string]any{
		"status": "published", "version": 1,
	}).Error; err != nil {
		t.Fatalf("publish wf: %v", err)
	}
	if err := db.Create(&models.WorkflowVersion{WorkflowID: "wf", Version: 1, Graph: slowGraph()}).Error; err != nil {
		t.Fatalf("version: %v", err)
	}
	run, err := eng.StartRunFromPublished("wf", nil, "")
	if err != nil {
		t.Fatalf("StartRunFromPublished: %v", err)
	}
	if run.Priority != models.PriorityNormal {
		t.Fatalf("priority = %d, want normal", run.Priority)
	}
	if run.Trigger != models.TriggerAPI {
		t.Fatalf("trigger = %q, want api", run.Trigger)
	}
	p.ReleaseAll()
}
