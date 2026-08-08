package channels

import (
	"errors"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

func runNotifyManager(t *testing.T) (*Manager, *fakeAdapter) {
	t.Helper()
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.mu.Lock()
	m.running["c1"] = &runningChannel{
		cfg: models.ChannelConfig{
			ID: "c1", Type: "qq", ProjectID: "proj", Enabled: true,
			CronDeliverTarget: "c2c:user1",
		},
		adapter: fa,
	}
	m.mu.Unlock()
	return m, fa
}

func setConvBusy(m *Manager, key string, busy bool) {
	q := m.convQueueFor(key)
	q.mu.Lock()
	q.busy = busy
	q.mu.Unlock()
}

// The bug this covers: the push was queued behind a live user turn, but the
// caller was told it had been delivered, so the receipt was consumed and the
// notification was never sent again.
func TestDeliverRunNotifyTrackedReportsDeferredWhileBusy(t *testing.T) {
	m, fa := runNotifyManager(t)
	key := convKey("proj", SceneC2C, "user1")
	setConvBusy(m, key, true)

	err := m.DeliverRunNotifyTracked("proj", "【Approving】等待人工处理", "track-1")
	if !errors.Is(err, services.ErrRunNotifyDeferred) {
		t.Fatalf("err = %v, want ErrRunNotifyDeferred", err)
	}
	if got := sentTexts(fa); len(got) != 0 {
		t.Fatalf("nothing should have been sent while busy, got %v", got)
	}
}

func TestSweepPushQueuesDeliversDeferredNotify(t *testing.T) {
	m, fa := runNotifyManager(t)
	key := convKey("proj", SceneC2C, "user1")
	setConvBusy(m, key, true)

	var settled []string
	m.SetPushSentObserver(func(id string) { settled = append(settled, id) })

	if err := m.DeliverRunNotifyTracked("proj", "【Approving】等待人工处理", "track-2"); err == nil {
		t.Fatal("expected the busy conversation to defer the push")
	}

	// The turn ends; nothing else touches this conversation. Only the sweeper
	// is left to notice.
	setConvBusy(m, key, false)
	m.SweepPushQueues()

	if got := sentTexts(fa); countText(got, "【Approving】等待人工处理") != 1 {
		t.Fatalf("sweeper should have delivered the queued notification, got %v", got)
	}
	if len(settled) != 1 || settled[0] != "track-2" {
		t.Fatalf("push-sent observer = %v, want one call for track-2", settled)
	}
}

func TestDeliverRunNotifyTrackedReportsSent(t *testing.T) {
	m, fa := runNotifyManager(t)

	if err := m.DeliverRunNotifyTracked("proj", "【Approving】运行失败", "track-3"); err != nil {
		t.Fatalf("idle conversation should deliver immediately: %v", err)
	}
	if got := sentTexts(fa); countText(got, "【Approving】运行失败") != 1 {
		t.Fatalf("expected one send, got %v", got)
	}
}
