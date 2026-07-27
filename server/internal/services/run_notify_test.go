package services

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeRunDeliverer struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (f *fakeRunDeliverer) DeliverRunNotify(projectID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, projectID+"|"+text)
	return f.err
}

func (f *fakeRunDeliverer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func setupRunNotifyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Project{}, &models.WorkflowDef{}, &models.NotifyDeliveryReceipt{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedNotifyProject(t *testing.T, db *gorm.DB, enabled bool, events []string, wfMode string, wfEvents []string) (models.Project, models.WorkflowDef) {
	t.Helper()
	p := models.Project{
		ID:   "proj-n1",
		Name: "Demo",
		NotifyPolicy: models.ProjectNotifyPolicy{
			Enabled: boolPtr(enabled), DefaultEvents: events,
		},
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	wf := models.WorkflowDef{
		ID: "wf-n1", ProjectID: p.ID, Name: "自我迭代", Status: "published", Version: 1,
		NotifyPolicy: models.WorkflowNotifyPolicy{Mode: wfMode, Events: wfEvents},
	}
	if err := db.Create(&wf).Error; err != nil {
		t.Fatal(err)
	}
	return p, wf
}

func TestAttemptDeliver_claimAndDedup(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, true, []string{"waiting_human", "failed"}, "custom", []string{"waiting_human", "failed"})
	d := &fakeRunDeliverer{}
	svc := NewRunNotifyService(db, d, "https://app.example")

	ev := RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-1", WorkflowID: "wf-n1",
		NodeID: "gate", NodeLabel: "门禁", Iteration: 1, Kind: "waiting_human",
	}
	svc.AttemptDeliver(ev)
	svc.AttemptDeliver(ev)
	if d.count() != 1 {
		t.Fatalf("deliver calls=%d want 1", d.count())
	}
	if !svc.HasClaimedForTest("run-1", "gate", 1, "waiting_human") {
		t.Fatal("expected receipt")
	}
	text := d.calls[0]
	if !containsAll(text, "等待人工处理", "https://app.example/runs/run-1", "自我迭代", "Demo") {
		t.Fatalf("unexpected text: %s", text)
	}
}

func TestAttemptDeliver_policyMissNoClaim(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, true, []string{"failed"}, "inherit", nil)
	d := &fakeRunDeliverer{}
	svc := NewRunNotifyService(db, d, "")
	svc.AttemptDeliver(RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-2", WorkflowID: "wf-n1",
		NodeID: "gate", Iteration: 1, Kind: "waiting_human",
	})
	if d.count() != 0 {
		t.Fatal("should not deliver")
	}
	if svc.HasClaimedForTest("run-2", "gate", 1, "waiting_human") {
		t.Fatal("policy miss must not claim")
	}
}

func TestAttemptDeliver_hardClose(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, false, []string{"waiting_human", "failed"}, "custom", []string{"waiting_human", "failed"})
	d := &fakeRunDeliverer{}
	svc := NewRunNotifyService(db, d, "")
	svc.AttemptDeliver(RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-3", WorkflowID: "wf-n1",
		NodeID: "n", Iteration: 1, Kind: "failed",
	})
	if d.count() != 0 || svc.HasClaimedForTest("run-3", "n", 1, "failed") {
		t.Fatal("hard close should skip")
	}
}

func TestAttemptDeliver_noTargetStillClaims(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, true, []string{"failed"}, "custom", []string{"failed"})
	d := &fakeRunDeliverer{err: ErrRunNotifyNoTarget}
	svc := NewRunNotifyService(db, d, "")
	svc.AttemptDeliver(RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-4", WorkflowID: "wf-n1",
		NodeID: "impl", Iteration: 2, Kind: "failed",
	})
	if d.count() != 1 {
		t.Fatal("deliverer should still be invoked once")
	}
	if !svc.HasClaimedForTest("run-4", "impl", 2, "failed") {
		t.Fatal("no-op must keep claim")
	}
	// Second attempt must not re-send
	svc.AttemptDeliver(RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-4", WorkflowID: "wf-n1",
		NodeID: "impl", Iteration: 2, Kind: "failed",
	})
	if d.count() != 1 {
		t.Fatalf("retry after no-op claim: calls=%d", d.count())
	}
}

func TestAttemptDeliver_sendFailNoRetry(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, true, []string{"failed"}, "inherit", nil)
	d := &fakeRunDeliverer{err: errors.New("qq down")}
	svc := NewRunNotifyService(db, d, "")
	ev := RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-5", WorkflowID: "wf-n1",
		NodeID: "x", Iteration: 1, Kind: "failed",
	}
	svc.AttemptDeliver(ev)
	svc.AttemptDeliver(ev)
	if d.count() != 1 {
		t.Fatalf("calls=%d want 1", d.count())
	}
}

func TestAttemptDeliver_missingNodeContext(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, true, []string{"failed"}, "inherit", nil)
	d := &fakeRunDeliverer{}
	svc := NewRunNotifyService(db, d, "")
	svc.AttemptDeliver(RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-6", WorkflowID: "wf-n1",
		Kind: "failed", // no node / iteration
	})
	if d.count() != 0 {
		t.Fatal("early fail without node must skip")
	}
}

func TestClaimReceiptUnique(t *testing.T) {
	db := setupRunNotifyDB(t)
	svc := NewRunNotifyService(db, nil, "")
	ok1, err := svc.ClaimReceiptForTest("r", "n", 1, "waiting_human")
	if err != nil || !ok1 {
		t.Fatalf("first claim: ok=%v err=%v", ok1, err)
	}
	ok2, err := svc.ClaimReceiptForTest("r", "n", 1, "waiting_human")
	if err != nil || ok2 {
		t.Fatalf("second claim: ok=%v err=%v", ok2, err)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
