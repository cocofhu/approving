package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

// TestReviewSessionQueueAndCancel: FIFO enqueue + Cancel clears pending and
// interrupts the held active turn (plan g1/g3 evidence).
func TestReviewSessionQueueAndCancel(t *testing.T) {
	eng, db, provider := setupReviewEngine(t, true)
	hold := make(chan struct{})
	provider.reviseHold = hold

	run, err := eng.StartRun("review-wf", map[string]any{"idea": "登录"}, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "prop")
	waitRunStatus(t, db, run.ID, "waiting_human")

	if _, err := eng.EnqueueReviewTurn(run.ID, "prop", "第一条", nil, nil, "node", ""); err != nil {
		t.Fatalf("enqueue1: %v", err)
	}
	if _, err := eng.EnqueueReviewTurn(run.ID, "prop", "第二条", nil, nil, "node", ""); err != nil {
		t.Fatalf("enqueue2: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w, thinking := eng.ReviewSessionState(run.ID, "prop")
		if thinking && w >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	w, thinking := eng.ReviewSessionState(run.ID, "prop")
	if !thinking || w < 1 {
		t.Fatalf("expected active turn + pending queue, got waiting=%d thinking=%v", w, thinking)
	}
	if eng.ReviewSessionReady(run.ID, "prop") {
		t.Fatal("expected not ready while queued/thinking")
	}

	if err := eng.CancelReviewSession(run.ID, "prop"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	close(hold)
	if err := eng.waitReviewReadyForTest(run.ID, "prop", 5*time.Second); err != nil {
		t.Fatalf("wait after cancel: %v", err)
	}
	w, thinking = eng.ReviewSessionState(run.ID, "prop")
	if w != 0 || thinking {
		t.Fatalf("after cancel want idle, got waiting=%d thinking=%v", w, thinking)
	}
	// At most one ReviseInPlace started (the active one); queued item dropped.
	if provider.reviseCalls["prop"] > 1 {
		t.Fatalf("Cancel should drop pending; reviseCalls=%d", provider.reviseCalls["prop"])
	}

	var conv models.ReactConversation
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "prop").First(&conv).Error; err != nil {
		t.Fatalf("load conv: %v", err)
	}
	var sawInterrupted bool
	for _, m := range conv.Messages {
		if m.Role == "agent" && m.Interrupted {
			sawInterrupted = true
		}
	}
	if !sawInterrupted {
		t.Fatalf("expected interrupted agent turn after Cancel: %+v", conv.Messages)
	}
}

// TestReviewEnqueueCapacity: platform FIFO rejects beyond MaxReviewQueueItems.
func TestReviewEnqueueCapacity(t *testing.T) {
	eng, db, provider := setupReviewEngine(t, true)
	hold := make(chan struct{})
	provider.reviseHold = hold

	run, err := eng.StartRun("review-wf", map[string]any{"idea": "登录"}, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "prop")
	waitRunStatus(t, db, run.ID, "waiting_human")

	// Start one held active turn, then fill pending up to MaxReviewQueueItems.
	if _, err := eng.EnqueueReviewTurn(run.ID, "prop", "active", nil, nil, "node", ""); err != nil {
		t.Fatalf("enqueue active: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, thinking := eng.ReviewSessionState(run.ID, "prop"); thinking {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	for i := 0; i < MaxReviewQueueItems; i++ {
		if _, err := eng.EnqueueReviewTurn(run.ID, "prop", "m", nil, nil, "node", ""); err != nil {
			t.Fatalf("enqueue pending %d: %v", i, err)
		}
	}
	_, err = eng.EnqueueReviewTurn(run.ID, "prop", "overflow", nil, nil, "node", "")
	if err == nil || !strings.Contains(err.Error(), "已满") {
		t.Fatalf("expected capacity error on overflow, got %v", err)
	}
	close(hold)
	_ = eng.CancelReviewSession(run.ID, "prop")
	_ = eng.waitReviewReadyForTest(run.ID, "prop", 5*time.Second)
}
