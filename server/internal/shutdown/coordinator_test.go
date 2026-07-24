package shutdown

import (
	"testing"
	"time"
)

func TestCoordinatorGraceRemaining(t *testing.T) {
	c := New(600 * time.Second)
	if c.IsDraining() {
		t.Fatal("expected not draining initially")
	}
	c.BeginDraining()
	if !c.IsDraining() {
		t.Fatal("expected draining after BeginDraining")
	}
	rem := c.GraceRemainingSeconds()
	if rem < 598 || rem > 600 {
		t.Fatalf("grace remaining = %d, want ~600", rem)
	}
	if c.GracePeriod() != 600*time.Second {
		t.Fatalf("grace period = %v", c.GracePeriod())
	}
}

func TestCoordinatorDefaultGrace(t *testing.T) {
	c := New(0)
	if c.GracePeriod() != 600*time.Second {
		t.Fatalf("default grace = %v", c.GracePeriod())
	}
}

func TestCoordinatorBeginDrainingIdempotent(t *testing.T) {
	c := New(30 * time.Second)
	c.BeginDraining()
	first := c.Deadline()
	c.BeginDraining()
	if !c.Deadline().Equal(first) {
		t.Fatal("second BeginDraining should not reset deadline")
	}
}

func TestCoordinatorDeadlineNotDraining(t *testing.T) {
	c := New(120 * time.Second)
	before := time.Now()
	dl := c.Deadline()
	if dl.Before(before.Add(119 * time.Second)) {
		t.Fatalf("deadline too soon: %v", dl)
	}
}

func TestCoordinatorGraceExpired(t *testing.T) {
	c := &Coordinator{grace: time.Second}
	c.BeginDraining()
	c.mu.Lock()
	c.startedAt = time.Now().Add(-2 * time.Second)
	c.mu.Unlock()
	if rem := c.GraceRemainingSeconds(); rem != 0 {
		t.Fatalf("expired grace = %d, want 0", rem)
	}
}
