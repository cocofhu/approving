package sandbox

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestNewIdleWatchFires verifies the idle timer fires after the window with no
// reset, giving the chat loop its stuck-agent signal.
func TestNewIdleWatchFires(t *testing.T) {
	c, _, stop := newIdleWatch(20 * time.Millisecond)
	defer stop()
	select {
	case <-c:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("idle watch did not fire within the window")
	}
}

// TestNewIdleWatchReset confirms each event resets the timer so a
// slow-but-productive turn (steady events) never trips the idle timeout.
func TestNewIdleWatchReset(t *testing.T) {
	c, reset, stop := newIdleWatch(60 * time.Millisecond)
	defer stop()
	// Reset every 20ms for ~120ms: the 60ms window must never elapse.
	deadline := time.After(120 * time.Millisecond)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-c:
			t.Fatal("idle watch fired despite steady resets")
		case <-tick.C:
			reset()
		case <-deadline:
			return
		}
	}
}

// TestNewIdleWatchDisabled returns a nil channel (never fires) when idle <= 0,
// preserving the original single-deadline behavior.
func TestNewIdleWatchDisabled(t *testing.T) {
	c, reset, stop := newIdleWatch(0)
	defer stop()
	reset() // must be a safe no-op
	select {
	case <-c:
		t.Fatal("disabled idle watch should never fire")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestChatSentinelsAreDistinct guards the errors.Is wiring the retry classifier
// depends on across package boundaries.
func TestChatSentinelsAreDistinct(t *testing.T) {
	if !errors.Is(fmt.Errorf("wrap: %w", ErrChatIdle), ErrChatIdle) {
		t.Error("ErrChatIdle not matched through a wrap")
	}
	if !errors.Is(fmt.Errorf("wrap: %w", ErrConnClosed), ErrConnClosed) {
		t.Error("ErrConnClosed not matched through a wrap")
	}
	if errors.Is(ErrChatIdle, ErrConnClosed) {
		t.Error("ErrChatIdle and ErrConnClosed must be distinct sentinels")
	}
}
