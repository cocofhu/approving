package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestAcpTimelineStoreUpsertAndPage(t *testing.T) {
	s := newAcpTimelineStore()
	s.upsert("r1", "n1", []models.AcpEvent{{T: 1, Kind: "message", Text: "a"}})
	ev, next, more, live, ok := s.page("r1", "n1", "", 20)
	if !ok || !live || more || len(ev) != 1 || next != "0" {
		t.Fatalf("first page = ok=%v live=%v more=%v ev=%+v next=%q", ok, live, more, ev, next)
	}

	events := make([]models.AcpEvent, 25)
	for i := range events {
		events[i] = models.AcpEvent{T: i, Kind: "message", Text: "e"}
	}
	s.upsert("r1", "n1", events)
	ev, next, more, _, ok = s.page("r1", "n1", "", 20)
	if !ok || len(ev) != 20 || !more || next != "5" {
		t.Fatalf("paged want 20 hasMore, got len=%d more=%v next=%q", len(ev), more, next)
	}
	ev2, _, more2, _, _ := s.page("r1", "n1", next, 20)
	if len(ev2) != 5 || more2 {
		t.Fatalf("older page want 5 no more, got len=%d more=%v", len(ev2), more2)
	}
}

func TestAcpTimelineStoreStopMarksNotLive(t *testing.T) {
	s := newAcpTimelineStore()
	s.begin("r", "n")
	s.upsert("r", "n", []models.AcpEvent{{T: 1, Kind: "message", Text: "x"}})
	s.stop("r", "n")
	e, ok := s.get("r", "n")
	if !ok || e.live {
		t.Fatalf("after stop want ready snapshot live=false, got ok=%v live=%v", ok, e.live)
	}
}

func TestAcpTimelineIngestKeepsSnapshotOnRefreshFailure(t *testing.T) {
	s := newAcpTimelineStore()
	s.upsert("r", "n", []models.AcpEvent{{T: 1, Kind: "message", Text: "keep"}})
	// Unreachable host — must not clear the prior snapshot.
	s.refreshFromSandbox(context.Background(), "r", "n", "127.0.0.1", 1)
	e, ok := s.get("r", "n")
	if !ok || len(e.events) != 1 || e.events[0].Text != "keep" {
		t.Fatalf("snapshot should survive refresh failure: %+v ok=%v", e.events, ok)
	}
}

func TestPickLongerEvents(t *testing.T) {
	prev := []models.AcpEvent{{T: 1, Kind: "message", Text: "a"}}
	in := []models.AcpEvent{{T: 1, Kind: "message", Text: "a"}, {T: 2, Kind: "thought", Text: "b"}}
	got := pickLongerEvents(prev, in)
	if len(got) != 2 {
		t.Fatalf("want longer incoming, got %d", len(got))
	}
	if len(pickLongerEvents(in, prev)) != 2 {
		t.Fatal("shorter incoming must not shrink snapshot")
	}
}

func TestAcpTimelineIngestLoopCancels(t *testing.T) {
	s := newAcpTimelineStore()
	s.startIngest("r", "n", "127.0.0.1", 1)
	time.Sleep(10 * time.Millisecond)
	s.stop("r", "n")
	// stop is idempotent
	s.stop("r", "n")
}
