package channels

import (
	"sync"
	"testing"
)

// The three markers are not interchangeable, and every time they have been
// collapsed into one the same bug came back: the answer the user was waiting
// for got suppressed because something else had already spoken.
func TestProgressDoesNotCountAsAnAnswer(t *testing.T) {
	turns := newTurnState()
	turns.markReplied("turn")

	if !turns.hasReplied("turn") {
		t.Fatal("a progress line is user-visible; it must set replied")
	}
	if turns.hasAnswered("turn") {
		t.Fatal("a progress line is not the answer and must never suppress one")
	}
	if turns.hasAcknowledged("turn") {
		t.Fatal("a progress line is not an acceptance notice")
	}
}

// Acknowledgement survives clearReplied on purpose: a turn that hands off
// mid-flight may speak again, but it must not announce the same work twice.
func TestHandoffForgetsWhatWasSaidButNotThatWorkWasAccepted(t *testing.T) {
	turns := newTurnState()
	turns.markReplied("turn")
	turns.markAnswered("turn")
	turns.markAcknowledged("turn")

	turns.clearReplied("turn")
	if turns.hasReplied("turn") || turns.hasAnswered("turn") {
		t.Fatal("the handoff must leave the turn free to speak again")
	}
	if !turns.hasAcknowledged("turn") {
		t.Fatal("the acceptance notice would be sent a second time")
	}

	turns.clearTurn("turn")
	if turns.hasAcknowledged("turn") {
		t.Fatal("a finished turn may not suppress anything in the next one")
	}
}

// An empty scope is every conversation at once. Marking on one would silence
// the next unrelated turn, so it is dropped rather than stored.
func TestAnEmptyScopeMarksNothing(t *testing.T) {
	turns := newTurnState()
	turns.markReplied("")
	turns.markAnswered("   ")
	turns.markAcknowledged("")
	if turns.hasReplied("") || turns.hasAnswered("   ") || turns.hasAcknowledged("") {
		t.Fatal("an unscoped mark was stored")
	}
}

// Two conversations move at once in production; the markers are read on the
// message path, so this runs under -race in CI.
func TestConcurrentTurnsDoNotShareMarkers(t *testing.T) {
	turns := newTurnState()
	var wg sync.WaitGroup
	for _, scope := range []string{"a", "b", "c", "d"} {
		wg.Add(1)
		go func(scope string) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				turns.markReplied(scope)
				turns.hasAnswered(scope)
				turns.clearTurn(scope)
			}
		}(scope)
	}
	wg.Wait()
}
