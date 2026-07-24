package browser

import (
	"testing"
	"time"
)

// clock is a controllable time source for deterministic TTL/LRU tests.
type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func newTestRegistry(maxTabs, maxPer int) (*tabRegistry, *clock) {
	c := &clock{t: time.Unix(1_700_000_000, 0)}
	r := newTabRegistry(maxTabs, maxPer)
	r.now = c.now
	return r, c
}

func TestRegistryAddRemoveCounts(t *testing.T) {
	r, _ := newTestRegistry(10, 5)
	r.add("s1", "c1")
	r.add("s2", "c1")
	if r.count() != 2 {
		t.Fatalf("count = %d, want 2", r.count())
	}
	if r.containerCount("c1") != 2 {
		t.Fatalf("containerCount = %d, want 2", r.containerCount("c1"))
	}
	if c, ok := r.remove("s1"); !ok || c != "c1" {
		t.Fatalf("remove = %q %v, want c1 true", c, ok)
	}
	if r.count() != 1 || r.containerCount("c1") != 1 {
		t.Fatalf("after remove: count=%d cc=%d", r.count(), r.containerCount("c1"))
	}
	if _, ok := r.remove("missing"); ok {
		t.Fatal("remove of missing session should return false")
	}
}

func TestRegistryFull(t *testing.T) {
	r, _ := newTestRegistry(2, 5)
	r.add("s1", "c1")
	if r.full() {
		t.Fatal("should not be full at 1/2")
	}
	r.add("s2", "c1")
	if !r.full() {
		t.Fatal("should be full at 2/2")
	}
}

func TestRegistryLRU(t *testing.T) {
	r, c := newTestRegistry(10, 10)
	r.add("s1", "c1")
	c.add(time.Second)
	r.add("s2", "c1")
	c.add(time.Second)
	r.add("s3", "c1")
	// s1 is oldest.
	if id, ok := r.lru(); !ok || id != "s1" {
		t.Fatalf("lru = %q %v, want s1", id, ok)
	}
	// Touch s1 → s2 becomes oldest.
	c.add(time.Second)
	r.touch("s1")
	if id, _ := r.lru(); id != "s2" {
		t.Fatalf("lru after touch = %q, want s2", id)
	}
}

func TestRegistryIdle(t *testing.T) {
	r, c := newTestRegistry(10, 10)
	r.add("s1", "c1")
	c.add(time.Second)
	r.add("s2", "c1")
	c.add(5 * time.Minute)
	// s1 (idle 5m1s) and s2 (idle 5m) both exceed 4m; none exceed 10m.
	if got := r.idle(4 * time.Minute); len(got) != 2 {
		t.Fatalf("idle(4m) = %v, want 2", got)
	}
	if got := r.idle(10 * time.Minute); len(got) != 0 {
		t.Fatalf("idle(10m) = %v, want 0", got)
	}
	if got := r.idle(0); got != nil {
		t.Fatalf("idle(0) should be nil, got %v", got)
	}
}

func TestRegistryReapableContainers(t *testing.T) {
	r, c := newTestRegistry(10, 10)
	r.add("s1", "c1")
	// Container not empty → not reapable.
	if got := r.reapableContainers(time.Minute); len(got) != 0 {
		t.Fatalf("non-empty container reapable: %v", got)
	}
	r.remove("s1") // c1 empty as of now
	c.add(2 * time.Minute)
	if got := r.reapableContainers(time.Minute); len(got) != 1 || got[0] != "c1" {
		t.Fatalf("reapable = %v, want [c1]", got)
	}
	// Adding a tab back clears the empty marker.
	r.add("s2", "c1")
	if got := r.reapableContainers(time.Minute); len(got) != 0 {
		t.Fatalf("revived container should not be reapable: %v", got)
	}
	r.forgetContainer("c1")
	if _, ok := r.emptySince["c1"]; ok {
		t.Fatal("forgetContainer should clear emptySince")
	}
}
