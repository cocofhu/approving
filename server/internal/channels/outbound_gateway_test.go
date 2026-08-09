package channels

import (
	"errors"
	"sync"
	"testing"
)

// fakeTransport is the whole channel layer as far as the queue is concerned.
// That it fits in twenty lines is the point of the split: the queue's rules can
// now be exercised without a bridge, an adapter or a running channel.
type fakeTransport struct {
	mu   sync.Mutex
	busy bool
	// busyAfter turns the conversation busy once this many items have gone
	// out, which is how a user arriving mid-flush looks from in here. Zero
	// means never.
	busyAfter int
	noTarget  bool
	sent      []CronPushItem
}

func (f *fakeTransport) conversationBusy(string, Scene, string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.busy || (f.busyAfter > 0 && len(f.sent) >= f.busyAfter)
}

func (f *fakeTransport) pushTarget(string, string) (*runningChannel, error) {
	if f.noTarget {
		return nil, errors.New("no target")
	}
	return &runningChannel{}, nil
}

func (f *fakeTransport) deliverPush(item CronPushItem, _ *runningChannel) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, item)
	return true
}

func (f *fakeTransport) delivered() []CronPushItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]CronPushItem(nil), f.sent...)
}

func trackedItem(id string, kind CronResultKind) CronPushItem {
	return CronPushItem{ID: id, ProjectID: "proj", Conv: "c1", Category: runNotifyCategory, Kind: kind, Text: id}
}

// A busy conversation defers rather than drops. The distinction is the whole
// reason the caller gets an outcome map back: a deferred id is absent, so the
// notification stays unsettled instead of being recorded as delivered.
func TestBusyConversationDefersInsteadOfDropping(t *testing.T) {
	transport := &fakeTransport{busy: true}
	g := newOutboundGateway(transport)
	g.enqueue("k", trackedItem("n1", CronResultChanged))

	outcome := g.flush("k")
	if _, present := outcome["n1"]; present {
		t.Fatalf("a deferred item must not report an outcome, got %v", outcome)
	}
	if len(transport.delivered()) != 0 {
		t.Fatal("nothing may reach the wire while the user holds the turn")
	}

	transport.mu.Lock()
	transport.busy = false
	transport.mu.Unlock()
	if outcome := g.flush("k"); !outcome["n1"] {
		t.Fatalf("the deferred item was never re-sent: %v", outcome)
	}
}

// The tail is what used to be lost. Once one item defers mid-flush, every item
// behind it has already been taken out of the queue and has to go back.
func TestDeferMidFlushKeepsTheTail(t *testing.T) {
	transport := &fakeTransport{busyAfter: 1}
	g := newOutboundGateway(transport)
	for _, id := range []string{"a", "b", "c"} {
		g.enqueue("k", trackedItem(id, CronResultChanged))
	}
	g.flush("k")

	if got := len(transport.delivered()); got != 1 {
		t.Fatalf("delivered %d items; the user arrived after the first", got)
	}
	if pending := len(g.queueFor("k").pending); pending != 2 {
		t.Fatalf("%d items requeued; the two behind the interruption must both survive", pending)
	}

	transport.mu.Lock()
	transport.busyAfter = 0
	transport.mu.Unlock()
	g.flush("k")
	if got := len(transport.delivered()); got != 3 {
		t.Fatalf("delivered %d of 3 once the conversation freed up", got)
	}
}

// A tracked item that reaches the wire fires the observer once. That callback
// is how a deferred receipt settles to delivered instead of staying deferred
// forever, so a silent gateway here means a stuck notification in production.
func TestSentObserverFiresForTrackedItems(t *testing.T) {
	transport := &fakeTransport{}
	g := newOutboundGateway(transport)
	var mu sync.Mutex
	var seen []string
	g.setSentObserver(func(id string) {
		mu.Lock()
		seen = append(seen, id)
		mu.Unlock()
	})
	g.enqueue("k", trackedItem("n1", CronResultChanged))
	g.enqueue("k", CronPushItem{ProjectID: "proj", Conv: "c1", Category: "cron", Kind: CronResultChanged, Text: "untracked"})
	g.flush("k")

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "n1" {
		t.Fatalf("observer saw %v; only the tracked item should be reported", seen)
	}
}

// A project with no bound channel is a failure, not a deferral: there is
// nothing to wait for, so the caller must be told now rather than left holding
// an item that will never move.
func TestMissingTargetFailsRatherThanDefers(t *testing.T) {
	g := newOutboundGateway(&fakeTransport{noTarget: true})
	g.enqueue("k", trackedItem("n1", CronResultChanged))
	outcome := g.flush("k")
	sent, reported := outcome["n1"]
	if !reported || sent {
		t.Fatalf("outcome = %v; want an explicit failure", outcome)
	}
	if pending := len(g.queueFor("k").pending); pending != 0 {
		t.Fatalf("%d items still queued for a project that has nowhere to send", pending)
	}
}

// The sweep exists because nothing else touches an idle conversation. Without
// it a push enqueued mid-turn waits for the user's next message.
func TestSweepMovesQueuesNothingElseWouldTouch(t *testing.T) {
	transport := &fakeTransport{busy: true}
	g := newOutboundGateway(transport)
	g.enqueue("k", trackedItem("n1", CronResultChanged))
	g.flush("k")

	transport.mu.Lock()
	transport.busy = false
	transport.mu.Unlock()
	g.sweep()
	if len(transport.delivered()) != 1 {
		t.Fatal("the sweep left the queue where it found it")
	}
	if keys := g.pendingKeys(); len(keys) != 0 {
		t.Fatalf("pending after a successful sweep: %v", keys)
	}
}
