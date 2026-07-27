package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

// TestClarifySessionQueueAndCancelKeepNext: clarify Cancel keeps pending FIFO
// and pumps the next item (Demo semantics; NOT review clear-queue).
func TestClarifySessionQueueAndCancelKeepNext(t *testing.T) {
	eng, db, provider := setupEngineGraphP(t, reactOnlyGraph())
	hold := make(chan struct{})
	provider.reactHold = hold
	provider.reactPending = 10 // keep dialogue open so Cancel path is exercised

	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "clarify")

	if _, err := eng.EnqueueClarifyTurn(run.ID, "clarify", "第一条", nil, nil); err != nil {
		t.Fatalf("enqueue1: %v", err)
	}
	if _, err := eng.EnqueueClarifyTurn(run.ID, "clarify", "第二条", nil, nil); err != nil {
		t.Fatalf("enqueue2: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w, thinking := eng.ReviewSessionState(run.ID, "clarify")
		if thinking && w >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	w, thinking := eng.ReviewSessionState(run.ID, "clarify")
	if !thinking || w < 1 {
		t.Fatalf("expected active + pending, got waiting=%d thinking=%v", w, thinking)
	}

	if err := eng.CancelClarifyTurn(run.ID, "clarify"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// Clear hold BEFORE the pump starts item 2 (item 1 still has a local hold
	// ref and unblocks via ctx cancel). Otherwise item 2 blocks forever.
	provider.mu.Lock()
	provider.reactHold = nil
	provider.mu.Unlock()
	close(hold)

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		provider.mu.Lock()
		n := provider.reactReplyCalls["clarify"]
		provider.mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	provider.mu.Lock()
	calls := provider.reactReplyCalls["clarify"]
	provider.mu.Unlock()
	if calls < 2 {
		t.Fatalf("clarify Cancel must keep queue and pump next; reactReplyCalls=%d", calls)
	}
	if err := eng.waitReviewReadyForTest(run.ID, "clarify", 5*time.Second); err != nil {
		t.Fatalf("wait after cancel+next: %v", err)
	}

	var conv models.ReactConversation
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "clarify").First(&conv).Error; err != nil {
		t.Fatalf("load conv: %v", err)
	}
	var sawInterrupted, sawSecondHuman bool
	for _, m := range conv.Messages {
		if m.Role == "agent" && m.Interrupted {
			sawInterrupted = true
		}
		if m.Role == "human" && strings.Contains(m.Text, "第二条") {
			sawSecondHuman = true
		}
	}
	if !sawInterrupted {
		t.Fatalf("expected interrupted agent after Cancel: %+v", conv.Messages)
	}
	if !sawSecondHuman {
		t.Fatalf("expected second human turn after keep-queue Cancel: %+v", conv.Messages)
	}
}

// TestClarifyReactReplyEnqueues: classic ReactReply(!force) returns before the
// turn finishes and exposes waiting via ReviewSessionState.
func TestClarifyReactReplyEnqueues(t *testing.T) {
	eng, db, provider := setupEngineGraphP(t, reactOnlyGraph())
	hold := make(chan struct{})
	provider.reactHold = hold
	provider.reactPending = 1

	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "clarify")

	done := make(chan error, 1)
	go func() {
		done <- eng.ReactReply(run.ID, "clarify", "异步入队", nil, nil, false)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("enqueue reply: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReactReply(!force) should return immediately after enqueue")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, thinking := eng.ReviewSessionState(run.ID, "clarify"); thinking {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	_, thinking := eng.ReviewSessionState(run.ID, "clarify")
	if !thinking {
		t.Fatal("expected thinking after enqueue")
	}
	close(hold)
	provider.reactHold = nil
	if err := eng.waitReviewReadyForTest(run.ID, "clarify", 5*time.Second); err != nil {
		t.Fatalf("wait: %v", err)
	}
}
