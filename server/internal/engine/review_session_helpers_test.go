package engine

import (
	"fmt"
	"time"
)

// waitReviewReadyForTest polls until the producer session is ready or timeout.
// Used by engine tests after async EnqueueReviewTurn.
func (e *Engine) waitReviewReadyForTest(runID, producerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e.ReviewSessionReady(runID, producerID) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	w, thinking := e.ReviewSessionState(runID, producerID)
	return fmt.Errorf("review session not ready (waiting=%d thinking=%v)", w, thinking)
}
