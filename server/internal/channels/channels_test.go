package channels

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cocofhu/approving/internal/models"
)

func TestSyntheticUserID(t *testing.T) {
	got := SyntheticUserID("qq", SceneGroup, "ABC")
	if got != "qq:group:ABC" {
		t.Fatalf("SyntheticUserID = %q", got)
	}
	// Distinct scenes/conversations never collide.
	if SyntheticUserID("qq", SceneC2C, "x") == SyntheticUserID("qq", SceneGroup, "x") {
		t.Fatal("scene should disambiguate synthetic user id")
	}
}

func TestFormatChannelUserText(t *testing.T) {
	cases := []struct {
		name string
		in   InboundMessage
		imgs bool
		want string
	}{
		{"c2c plain", InboundMessage{Scene: SceneC2C, UserID: "u1", Text: "hi"}, false, "hi"},
		{"group attributed", InboundMessage{Scene: SceneGroup, UserID: "u1", Text: "hi"}, false, "[来自 u1] hi"},
		{"guild attributed", InboundMessage{Scene: SceneGuild, UserID: "u2", Text: "yo"}, false, "[来自 u2] yo"},
		{"group image only", InboundMessage{Scene: SceneGroup, UserID: "u1"}, true, "[来自 u1] (用户发送了图片)"},
		{"group no speaker", InboundMessage{Scene: SceneGroup, Text: "hi"}, false, "hi"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatChannelUserText(c.in, c.imgs); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestSplitImageURLs(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantText string
		wantURLs []string
	}{
		{"none", "just text", "just text", nil},
		{
			"markdown",
			"see ![alt](https://x.com/a.png) done",
			"see  done",
			[]string{"https://x.com/a.png"},
		},
		{
			"bare",
			"pic https://x.com/b.jpg here",
			"pic https://x.com/b.jpg here",
			[]string{"https://x.com/b.jpg"},
		},
		{
			"dedup",
			"![](https://x.com/a.png) and https://x.com/a.png",
			"and https://x.com/a.png",
			[]string{"https://x.com/a.png"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, urls := splitImageURLs(c.in)
			if text != c.wantText {
				t.Errorf("text = %q want %q", text, c.wantText)
			}
			if !reflect.DeepEqual(urls, c.wantURLs) {
				t.Errorf("urls = %v want %v", urls, c.wantURLs)
			}
		})
	}
}

func TestSplitImageURLsCapsAtFour(t *testing.T) {
	in := "![](https://x/1.png) ![](https://x/2.png) ![](https://x/3.png) ![](https://x/4.png) ![](https://x/5.png)"
	_, urls := splitImageURLs(in)
	if len(urls) != 4 {
		t.Fatalf("expected cap at 4, got %d", len(urls))
	}
}

func TestParseTarget(t *testing.T) {
	cases := []struct {
		in    string
		scene Scene
		conv  string
	}{
		{"guild:123", SceneGuild, "123"},
		{"group:openid", SceneGroup, "openid"},
		{"c2c:user:with:colons", SceneC2C, "user:with:colons"},
		{"malformed", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		scene, conv := parseTarget(c.in)
		if scene != c.scene || conv != c.conv {
			t.Errorf("parseTarget(%q) = (%q,%q) want (%q,%q)", c.in, scene, conv, c.scene, c.conv)
		}
	}
}

func TestSessionCapsFromConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  map[string]any
		want SessionCaps
	}{
		{"nil defaults false", nil, SessionCaps{}},
		{"empty defaults false", map[string]any{}, SessionCaps{}},
		{"memory true only", map[string]any{"allowMemoryWrite": true}, SessionCaps{AllowMemoryWrite: true}},
		{"scheduler true only", map[string]any{"allowSchedulerWrite": true}, SessionCaps{AllowSchedulerWrite: true}},
		{"both true", map[string]any{"allowMemoryWrite": true, "allowSchedulerWrite": true}, SessionCaps{AllowMemoryWrite: true, AllowSchedulerWrite: true}},
		{"explicit false", map[string]any{"allowMemoryWrite": false, "allowSchedulerWrite": false}, SessionCaps{}},
		{"non-bool ignored", map[string]any{"allowMemoryWrite": "yes", "allowSchedulerWrite": 1}, SessionCaps{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SessionCapsFromConfig(c.cfg)
			if got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestFingerprintChanges(t *testing.T) {
	base := models.ChannelConfig{ID: "a", Type: "qq", ProjectID: "p", AppID: "app", AppSecretEnc: "enc", Enabled: true}
	fp := fingerprint(base)
	// ID/Name are not part of the fingerprint; secret & timeout are.
	same := base
	same.Name = "renamed"
	if fingerprint(same) != fp {
		t.Error("fingerprint changed on Name (should be ignored)")
	}
	changed := base
	changed.AppSecretEnc = "enc2"
	if fingerprint(changed) == fp {
		t.Error("fingerprint unchanged after secret rotation")
	}
	changed2 := base
	changed2.TurnTimeoutSeconds = 120
	if fingerprint(changed2) == fp {
		t.Error("fingerprint unchanged after turn timeout change")
	}
}

// fakeAdapter records outbound sends and satisfies the Adapter interface.
type fakeAdapter struct {
	mu   sync.Mutex
	sent []OutboundMessage
}

func (f *fakeAdapter) Type() string                                              { return "fake" }
func (f *fakeAdapter) Start(ctx context.Context, onInbound InboundHandler) error { return nil }
func (f *fakeAdapter) Stop() error                                               { return nil }
func (f *fakeAdapter) Send(ctx context.Context, out OutboundMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, out)
	return nil
}

func newTestManager(fa *fakeAdapter) *Manager {
	factories := map[string]AdapterFactory{
		"qq": func(cfg AdapterConfig) (Adapter, error) { return fa, nil },
	}
	return NewManager(nil, factories, func(s string) (string, error) { return s, nil })
}

func TestManagerDeliverNoChannel(t *testing.T) {
	m := newTestManager(&fakeAdapter{})
	if err := m.Deliver("nope", "hello"); err != ErrNoDeliveryChannel {
		t.Fatalf("Deliver with no channel: got %v want ErrNoDeliveryChannel", err)
	}
}

func TestManagerDeliverRoutesToTarget(t *testing.T) {
	fa := &fakeAdapter{}
	m := newTestManager(fa)
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "guild:123",
	}})
	defer m.StopAll()

	if err := m.Deliver("proj", "result ![](https://x.com/a.png)"); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if len(fa.sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(fa.sent))
	}
	out := fa.sent[0]
	if out.Scene != SceneGuild || out.ConversationID != "123" {
		t.Errorf("routed to (%q,%q) want (guild,123)", out.Scene, out.ConversationID)
	}
	if out.Text != "result" {
		t.Errorf("text = %q want %q", out.Text, "result")
	}
	if len(out.ImageURLs) != 1 || out.ImageURLs[0] != "https://x.com/a.png" {
		t.Errorf("image urls = %v", out.ImageURLs)
	}
}

func TestManagerDeliverSkipsDisabledDelivery(t *testing.T) {
	fa := &fakeAdapter{}
	m := newTestManager(fa)
	// Enabled channel but CronDeliver=false → not a delivery target.
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: false, CronDeliverTarget: "guild:123",
	}})
	defer m.StopAll()
	if err := m.Deliver("proj", "x"); err != ErrNoDeliveryChannel {
		t.Fatalf("got %v want ErrNoDeliveryChannel", err)
	}
}

// failAckAdapter records sends and fails only the delayed-ack message.
type failAckAdapter struct {
	fakeAdapter
}

func (f *failAckAdapter) Send(ctx context.Context, out OutboundMessage) error {
	f.mu.Lock()
	f.sent = append(f.sent, out)
	fail := out.Text == delayedAckText
	f.mu.Unlock()
	if fail {
		return errors.New("ack send failed")
	}
	return nil
}

func testRunningChannel(adapter Adapter) *runningChannel {
	return &runningChannel{
		cfg: models.ChannelConfig{
			ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		},
		adapter: adapter,
	}
}

func testInbound(id string) InboundMessage {
	return InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "user1",
		MessageID: id, Text: "hello",
	}
}

func sentTexts(fa *fakeAdapter) []string {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	out := make([]string, len(fa.sent))
	for i, s := range fa.sent {
		out[i] = s.Text
	}
	return out
}

func TestDispatchPassesSessionCapsFromConfig(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = time.Hour // never fire ack
	var gotCaps SessionCaps
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		gotCaps = rc.Caps
		return Reply{Text: "ok"}, nil
	}
	rc := testRunningChannel(fa)
	rc.cfg.Config = map[string]any{
		"allowMemoryWrite":    true,
		"allowSchedulerWrite": false,
		"sandbox":             true,
	}
	m.dispatch(context.Background(), rc, testInbound("caps-1"))
	if !gotCaps.AllowMemoryWrite || gotCaps.AllowSchedulerWrite {
		t.Fatalf("caps = %+v want memory=true scheduler=false", gotCaps)
	}

	rc.cfg.Config = map[string]any{
		"allowMemoryWrite":    false,
		"allowSchedulerWrite": true,
	}
	m.dispatch(context.Background(), rc, testInbound("caps-2"))
	if gotCaps.AllowMemoryWrite || !gotCaps.AllowSchedulerWrite {
		t.Fatalf("caps after config change = %+v want memory=false scheduler=true", gotCaps)
	}
}

func TestDispatchFastPathNoDelayedAck(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = 80 * time.Millisecond
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		return Reply{Text: "final-ok"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("m1"))
	// Wait past ack threshold to ensure a late ack does not appear.
	time.Sleep(120 * time.Millisecond)
	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "final-ok" {
		t.Fatalf("fast path sends = %v want [final-ok]", got)
	}
}

func TestDispatchSlowSuccessAckThenFinal(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = 40 * time.Millisecond
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		time.Sleep(80 * time.Millisecond)
		return Reply{Text: "final-ok"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("m2"))
	got := sentTexts(fa)
	if len(got) != 2 || got[0] != delayedAckText || got[1] != "final-ok" {
		t.Fatalf("slow success sends = %v want [%q final-ok]", got, delayedAckText)
	}
}

func TestDispatchSlowFailureAckThenFailPrefix(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = 40 * time.Millisecond
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		time.Sleep(80 * time.Millisecond)
		return Reply{}, errors.New("沙箱未就绪")
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("m3"))
	got := sentTexts(fa)
	wantFail := failReplyPrefix + "沙箱未就绪"
	if len(got) != 2 || got[0] != delayedAckText || got[1] != wantFail {
		t.Fatalf("slow failure sends = %v want [%q %q]", got, delayedAckText, wantFail)
	}
}

func TestDispatchAckSendFailureDoesNotBlockFinal(t *testing.T) {
	fa := &failAckAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = 40 * time.Millisecond
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		time.Sleep(80 * time.Millisecond)
		return Reply{Text: "final-ok"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("m4"))
	got := sentTexts(&fa.fakeAdapter)
	if len(got) != 2 || got[0] != delayedAckText || got[1] != "final-ok" {
		t.Fatalf("ack-fail path sends = %v want [%q final-ok]", got, delayedAckText)
	}
}

func TestDispatchBusyEnqueuesNoDelayedAckOnQueued(t *testing.T) {
	// plan g2.1: rewrite former busy-drop assertions — second message is queued,
	// processed after the first, with one queue ACK and no delayedAck on dequeue.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = 40 * time.Millisecond
	started := make(chan struct{})
	var once sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		time.Sleep(120 * time.Millisecond)
		return Reply{Text: "final-" + in.MessageID}, nil
	}
	rc := testRunningChannel(fa)
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.dispatch(context.Background(), rc, testInbound("m5a"))
	}()
	<-started
	m.dispatch(context.Background(), rc, testInbound("m5b"))
	<-done

	got := sentTexts(fa)
	queueAck, delayedAck, finalA, finalB := 0, 0, 0, 0
	for _, text := range got {
		switch text {
		case queueAckText:
			queueAck++
		case delayedAckText:
			delayedAck++
		case "final-m5a":
			finalA++
		case "final-m5b":
			finalB++
		}
	}
	if queueAck != 1 || finalA != 1 || finalB != 1 {
		t.Fatalf("busy enqueue sends = %v (queueAck=%d finalA=%d finalB=%d)", got, queueAck, finalA, finalB)
	}
	if delayedAck != 1 {
		t.Fatalf("expected exactly 1 delayedAck on idle-first turn, got %d in %v", delayedAck, got)
	}
	// Queued continuation must not emit a second delayedAck; finals stay ordered.
	idxA, idxB := indexOf(got, "final-m5a"), indexOf(got, "final-m5b")
	if idxA < 0 || idxB < 0 || idxA > idxB {
		t.Fatalf("expected final-m5a before final-m5b in %v", got)
	}
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func countText(ss []string, want string) int {
	n := 0
	for _, s := range ss {
		if s == want {
			n++
		}
	}
	return n
}

func TestDispatchFIFOOrderMultipleQueued(t *testing.T) {
	// plan g2.2: N≥3 inbound while busy → independent turns in arrival order.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = time.Hour
	started := make(chan struct{})
	var once sync.Once
	var orderMu sync.Mutex
	var handled []string
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		orderMu.Lock()
		handled = append(handled, in.MessageID)
		orderMu.Unlock()
		time.Sleep(40 * time.Millisecond)
		return Reply{Text: "final-" + in.MessageID}, nil
	}
	rc := testRunningChannel(fa)
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.dispatch(context.Background(), rc, testInbound("a"))
	}()
	<-started
	m.dispatch(context.Background(), rc, testInbound("b"))
	m.dispatch(context.Background(), rc, testInbound("c"))
	m.dispatch(context.Background(), rc, testInbound("d"))
	<-done

	orderMu.Lock()
	gotOrder := append([]string(nil), handled...)
	orderMu.Unlock()
	wantOrder := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("handle order = %v want %v", gotOrder, wantOrder)
	}
	got := sentTexts(fa)
	if countText(got, queueAckText) != 1 {
		t.Fatalf("queue ACK count = %d want 1 in %v", countText(got, queueAckText), got)
	}
	for _, id := range wantOrder {
		if countText(got, "final-"+id) != 1 {
			t.Fatalf("missing final for %s in %v", id, got)
		}
	}
}

func TestDispatchQueueFullVisibleReject(t *testing.T) {
	// plan g2.2: depth 16 pending; next inbound gets queueFullText, not silent drop.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = time.Hour
	gate := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		<-gate
		return Reply{Text: "final-" + in.MessageID}, nil
	}
	rc := testRunningChannel(fa)
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.dispatch(context.Background(), rc, testInbound("inflight"))
	}()
	<-started
	for i := 0; i < convQueueDepth; i++ {
		m.dispatch(context.Background(), rc, testInbound(fmt.Sprintf("q%d", i)))
	}
	// 17th pending attempt (after 16 queued) must be visibly rejected.
	m.dispatch(context.Background(), rc, testInbound("overflow"))
	close(gate)
	<-done

	got := sentTexts(fa)
	if countText(got, queueFullText) != 1 {
		t.Fatalf("full-queue sends = %v want exactly one %q", got, queueFullText)
	}
	if countText(got, "final-overflow") != 0 {
		t.Fatalf("overflow message must not be processed, got %v", got)
	}
	if countText(got, "final-inflight") != 1 {
		t.Fatalf("inflight turn missing in %v", got)
	}
}

func TestDispatchQueueAckOncePerBusyCycle(t *testing.T) {
	// plan g2.2: first busy enqueue ACKs once; after drain, next busy cycle may ACK again.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = time.Hour
	rc := testRunningChannel(fa)

	release1 := make(chan struct{})
	started1 := make(chan struct{})
	var once1 sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once1.Do(func() { close(started1) })
		if in.MessageID == "p0" {
			<-release1
		}
		return Reply{Text: "final-" + in.MessageID}, nil
	}
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		m.dispatch(context.Background(), rc, testInbound("p0"))
	}()
	<-started1
	m.dispatch(context.Background(), rc, testInbound("p1"))
	m.dispatch(context.Background(), rc, testInbound("p2"))
	close(release1)
	<-done1

	got1 := sentTexts(fa)
	if countText(got1, queueAckText) != 1 {
		t.Fatalf("first busy cycle ACK count = %d want 1 in %v", countText(got1, queueAckText), got1)
	}

	// Idle again: start a new busy cycle and enqueue once more.
	release2 := make(chan struct{})
	started2 := make(chan struct{})
	var once2 sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once2.Do(func() { close(started2) })
		if in.MessageID == "p3" {
			<-release2
		}
		return Reply{Text: "final-" + in.MessageID}, nil
	}
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		m.dispatch(context.Background(), rc, testInbound("p3"))
	}()
	<-started2
	m.dispatch(context.Background(), rc, testInbound("p4"))
	close(release2)
	<-done2

	got2 := sentTexts(fa)
	if countText(got2, queueAckText) != 2 {
		t.Fatalf("after second busy cycle ACK total = %d want 2 in %v", countText(got2, queueAckText), got2)
	}
}

func TestDispatchFailureContinuesDrain(t *testing.T) {
	// plan g2.2: one failed turn still drains the next queued message.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = time.Hour
	started := make(chan struct{})
	var once sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		time.Sleep(20 * time.Millisecond)
		if in.MessageID == "fail-me" {
			return Reply{}, errors.New("boom")
		}
		return Reply{Text: "final-" + in.MessageID}, nil
	}
	rc := testRunningChannel(fa)
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.dispatch(context.Background(), rc, testInbound("first"))
	}()
	<-started
	m.dispatch(context.Background(), rc, testInbound("fail-me"))
	m.dispatch(context.Background(), rc, testInbound("after-fail"))
	<-done

	got := sentTexts(fa)
	if countText(got, failReplyPrefix+"boom") != 1 {
		t.Fatalf("expected failure reply in %v", got)
	}
	if countText(got, "final-after-fail") != 1 {
		t.Fatalf("expected continuation after failure in %v", got)
	}
	if countText(got, "final-first") != 1 {
		t.Fatalf("expected first final in %v", got)
	}
}

func TestDispatchCrossConversationIndependent(t *testing.T) {
	// plan g2.2: two conversations do not block each other's enqueue/processing.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = time.Hour
	releaseA := make(chan struct{})
	startedA := make(chan struct{})
	startedB := make(chan struct{})
	var onceA, onceB sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		if in.ConversationID == "userA" {
			onceA.Do(func() { close(startedA) })
			<-releaseA
		} else {
			onceB.Do(func() { close(startedB) })
		}
		return Reply{Text: "final-" + in.MessageID}, nil
	}
	rc := testRunningChannel(fa)
	inA := func(id string) InboundMessage {
		return InboundMessage{Scene: SceneC2C, ConversationID: "userA", UserID: "userA", MessageID: id, Text: "hello"}
	}
	inB := func(id string) InboundMessage {
		return InboundMessage{Scene: SceneC2C, ConversationID: "userB", UserID: "userB", MessageID: id, Text: "hello"}
	}

	doneA := make(chan struct{})
	go func() {
		defer close(doneA)
		m.dispatch(context.Background(), rc, inA("a1"))
	}()
	<-startedA

	doneB := make(chan struct{})
	go func() {
		defer close(doneB)
		m.dispatch(context.Background(), rc, inB("b1"))
	}()
	select {
	case <-startedB:
	case <-time.After(2 * time.Second):
		t.Fatal("conversation B blocked by A")
	}
	close(releaseA)
	<-doneA
	<-doneB

	got := sentTexts(fa)
	if countText(got, "final-a1") != 1 || countText(got, "final-b1") != 1 {
		t.Fatalf("cross-conversation finals missing in %v", got)
	}
}

func TestDispatchIdleSingleKeepsDelayedAck(t *testing.T) {
	// plan g2.2 / g1.5: idle single-message path still emits delayedAck (no queue ACK).
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = 40 * time.Millisecond
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		time.Sleep(80 * time.Millisecond)
		return Reply{Text: "final-ok"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("idle-1"))
	got := sentTexts(fa)
	if countText(got, queueAckText) != 0 {
		t.Fatalf("idle path must not send queue ACK, got %v", got)
	}
	if len(got) != 2 || got[0] != delayedAckText || got[1] != "final-ok" {
		t.Fatalf("idle delayedAck path sends = %v want [%q final-ok]", got, delayedAckText)
	}
}

func TestFriendlyErrRuneTruncation(t *testing.T) {
	long := strings.Repeat("错", 250)
	got := friendlyErr(errors.New(long))
	if !utf8.ValidString(got) {
		t.Fatal("friendlyErr produced invalid UTF-8")
	}
	runes := []rune(got)
	// 200 runes + ellipsis
	if len(runes) != 201 || string(runes[:200]) != strings.Repeat("错", 200) || runes[200] != '…' {
		t.Fatalf("friendlyErr truncation = %q (rune len %d)", got, len(runes))
	}
	// Fail prefix composition stays short and stack-free.
	full := failReplyPrefix + got
	if !strings.HasPrefix(full, failReplyPrefix) || strings.Contains(full, "goroutine") {
		t.Fatalf("fail reply unexpected: %q", full)
	}
}
