package services

import (
	"sync"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

// fakeTrackingDeliverer stands in for the channel manager: it can report that a
// push was queued rather than sent, and it remembers the token it was handed so
// the test can settle the receipt the way a later flush would.
type fakeTrackingDeliverer struct {
	mu     sync.Mutex
	tracks []string
	err    error
}

func (f *fakeTrackingDeliverer) DeliverRunNotify(projectID, text string) error {
	return f.DeliverRunNotifyTracked(projectID, text, "")
}

func (f *fakeTrackingDeliverer) DeliverRunNotifyTracked(projectID, text, trackID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tracks = append(f.tracks, trackID)
	return f.err
}

func (f *fakeTrackingDeliverer) lastTrack() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.tracks) == 0 {
		return ""
	}
	return f.tracks[len(f.tracks)-1]
}

func receiptFor(t *testing.T, svc *RunNotifyService, runID string) models.NotifyDeliveryReceipt {
	t.Helper()
	var receipt models.NotifyDeliveryReceipt
	if err := svc.db.Where("run_id = ?", runID).First(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	return receipt
}

// A push parked behind a busy conversation has not reached anyone. Recording it
// as delivered is what made a run that stopped for a human look like a run that
// had been told about.
func TestAttemptDeliver_deferredIsNotDelivered(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, true, []string{"waiting_human"}, "inherit", nil)
	d := &fakeTrackingDeliverer{err: ErrRunNotifyDeferred}
	svc := NewRunNotifyService(db, d, "")
	svc.SetRetryDelays([]time.Duration{0, 0, 0})

	svc.AttemptDeliver(RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-def", WorkflowID: "wf-n1",
		NodeID: "gate", Iteration: 1, Kind: "waiting_human",
	})

	if got := len(d.tracks); got != 1 {
		t.Fatalf("attempts=%d want 1 — a queued push must not be retried into duplicates", got)
	}
	if status := receiptFor(t, svc, "run-def").DeliveryStatus; status != "deferred" {
		t.Fatalf("delivery status = %q want deferred", status)
	}
}

func TestSettlePushSent_deferredBecomesDelivered(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, true, []string{"waiting_human"}, "inherit", nil)
	d := &fakeTrackingDeliverer{err: ErrRunNotifyDeferred}
	svc := NewRunNotifyService(db, d, "")
	svc.SetRetryDelays([]time.Duration{0})

	svc.AttemptDeliver(RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-settle", WorkflowID: "wf-n1",
		NodeID: "gate", Iteration: 3, Kind: "waiting_human",
	})
	track := d.lastTrack()
	if track == "" {
		t.Fatal("deliverer was not handed a tracking token")
	}

	svc.SettlePushSent(track)

	if status := receiptFor(t, svc, "run-settle").DeliveryStatus; status != "delivered" {
		t.Fatalf("delivery status = %q want delivered", status)
	}
}

// A late flush must not rewrite a receipt that already has a settled story.
func TestSettlePushSent_leavesSettledReceiptsAlone(t *testing.T) {
	db := setupRunNotifyDB(t)
	seedNotifyProject(t, db, true, []string{"waiting_human"}, "inherit", nil)
	d := &fakeTrackingDeliverer{err: ErrRunNotifyNoTarget}
	svc := NewRunNotifyService(db, d, "")

	svc.AttemptDeliver(RunNotifyEvent{
		ProjectID: "proj-n1", RunID: "run-notarget", WorkflowID: "wf-n1",
		NodeID: "gate", Iteration: 1, Kind: "waiting_human",
	})

	svc.SettlePushSent(receiptTrackID("run-notarget", "gate", 1, "waiting_human"))

	if status := receiptFor(t, svc, "run-notarget").DeliveryStatus; status != "no_target" {
		t.Fatalf("delivery status = %q want no_target", status)
	}
}
