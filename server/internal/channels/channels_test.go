package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
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
		{"group image only", InboundMessage{Scene: SceneGroup, UserID: "u1"}, true, "[来自 u1] (用户发送了附件)"},
		{"c2c attachment only", InboundMessage{Scene: SceneC2C, UserID: "u1"}, true, "(用户发送了附件)"},
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
// messageIDs, when set, are handed back one per successful send to emulate a
// channel that reports its own message ids.
type fakeAdapter struct {
	mu         sync.Mutex
	sent       []OutboundMessage
	messageIDs []string
	onSend     func(out OutboundMessage) // optional hook (called under mu)
}

func (f *fakeAdapter) Type() string                                              { return "fake" }
func (f *fakeAdapter) Start(ctx context.Context, onInbound InboundHandler) error { return nil }
func (f *fakeAdapter) Stop() error                                               { return nil }
func (f *fakeAdapter) Send(ctx context.Context, out OutboundMessage) (SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, out)
	if f.onSend != nil {
		f.onSend(out)
	}
	messageID := ""
	if len(f.messageIDs) > 0 {
		messageID = f.messageIDs[0]
		f.messageIDs = f.messageIDs[1:]
	}
	return SendResult{MessageID: messageID}, nil
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

func TestManagerDeliverRunNotifyWithoutCronDeliver(t *testing.T) {
	fa := &fakeAdapter{}
	m := newTestManager(fa)
	// Bound QQ target without CronDeliver — Run notify must still work.
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: false, CronDeliverTarget: "c2c:user1",
	}})
	defer m.StopAll()
	if !m.HasRunNotifyTarget("proj") {
		t.Fatal("expected HasRunNotifyTarget")
	}
	if err := m.DeliverRunNotify("proj", "【Approving】等待人工处理\n打开：/runs/r1"); err != nil {
		t.Fatalf("DeliverRunNotify: %v", err)
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if len(fa.sent) != 1 {
		t.Fatalf("sent=%d want 1", len(fa.sent))
	}
	if fa.sent[0].ConversationID != "user1" {
		t.Errorf("conv=%q", fa.sent[0].ConversationID)
	}
	if !strings.Contains(fa.sent[0].Text, "等待人工处理") {
		t.Errorf("text=%q", fa.sent[0].Text)
	}
	// Cron path still blocked.
	if err := m.Deliver("proj", "cron"); err != ErrNoDeliveryChannel {
		t.Fatalf("Deliver cron still requires CronDeliver: %v", err)
	}
}

func TestManagerDeliverRunNotifyNoTarget(t *testing.T) {
	m := newTestManager(&fakeAdapter{})
	err := m.DeliverRunNotify("proj", "x")
	if !errors.Is(err, services.ErrRunNotifyNoTarget) {
		t.Fatalf("got %v want ErrRunNotifyNoTarget", err)
	}
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

func testInboundText(id, text string) InboundMessage {
	in := testInbound(id)
	in.Text = text
	return in
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
	var gotCaps SessionCaps
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		gotCaps = rc.Caps
		return Reply{FinalSummary: "ok"}, nil
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

// One user message produces one bot message. The reply is the acknowledgement;
// a separate "received, working on it" only pushes the answer further down the
// screen.
func TestDispatchAnswersOnceWithNoAcknowledgement(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		time.Sleep(30 * time.Millisecond)
		return Reply{FinalSummary: "首屏从 3.2s 降到 1.1s"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("m2"))
	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "首屏从 3.2s 降到 1.1s" {
		t.Fatalf("sends = %v want exactly the answer", got)
	}
}

// An internal error is never quoted back at the user; they get a cause they can
// act on instead.
func TestDispatchFailureExplainsInUserTerms(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		return Reply{}, errors.New("assistant produced no reply")
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("m3"))
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("failure sends = %v want exactly one message", got)
	}
	if strings.Contains(got[0], "assistant produced no reply") {
		t.Fatalf("internal error text leaked to the user: %q", got[0])
	}
	if ContainsInternalTerms(got[0]) {
		t.Fatalf("failure message exposes internals: %q", got[0])
	}
}

// A turn that produced nothing sendable must not emit a placeholder telling the
// user to go look elsewhere.
func TestDispatchMissingSummaryFallsBackWithoutPlaceholder(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		return Reply{Text: "内部推理，不应外发"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("m4"))
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want one fallback message", got)
	}
	if strings.Contains(got[0], "Approving") || strings.Contains(got[0], "内部推理") {
		t.Fatalf("fallback leaked raw output or punted to another surface: %q", got[0])
	}
}

// Queueing is invisible: messages that arrive during a turn wait their turn and
// are answered, with no queue-position narration.
func TestDispatchBusyQueuesSilentlyAndAnswersInOrder(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	started := make(chan struct{})
	var once sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		time.Sleep(80 * time.Millisecond)
		return Reply{FinalSummary: "final-" + in.MessageID}, nil
	}
	rc := testRunningChannel(fa)
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.dispatch(context.Background(), rc, testInbound("m5a"))
	}()
	<-started
	m.dispatch(context.Background(), rc, testInbound("m5b"))
	m.dispatch(context.Background(), rc, testInbound("m5c"))
	<-done

	got := sentTexts(fa)
	if len(got) != 3 {
		t.Fatalf("sends = %v want one answer per message and nothing else", got)
	}
	if countText(got, "final-m5a") != 1 || countText(got, "final-m5b") != 1 || countText(got, "final-m5c") != 1 {
		t.Fatalf("missing finals in %v", got)
	}
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
	// N≥3 inbound while busy → independent turns in arrival order; each gets queue ACK.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	started := make(chan struct{})
	var once sync.Once
	var orderMu sync.Mutex
	var handled []string
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		orderMu.Lock()
		handled = append(handled, in.MessageID)
		orderMu.Unlock()
		time.Sleep(20 * time.Millisecond)
		return Reply{FinalSummary: "final-" + in.MessageID}, nil
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
	if len(got) != len(wantOrder) {
		t.Fatalf("sends = %v want one answer per message, no queue narration", got)
	}
	for _, id := range wantOrder {
		if countText(got, "final-"+id) != 1 {
			t.Fatalf("missing final for %s in %v", id, got)
		}
	}
}

func TestDispatchQueueFullVisibleReject(t *testing.T) {
	// depth 16 pending; the next inbound is dropped but never silently.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	gate := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		<-gate
		return Reply{FinalSummary: "final-" + in.MessageID}, nil
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
	if countText(got, busyHintText) != 1 {
		t.Fatalf("full-queue sends = %v want exactly one %q", got, busyHintText)
	}
	if countText(got, "final-overflow") != 0 {
		t.Fatalf("overflow message must not be processed, got %v", got)
	}
	if countText(got, "final-inflight") != 1 {
		t.Fatalf("inflight turn missing in %v", got)
	}
}

// Across repeated busy cycles the conversation stays one-in one-out: five user
// messages, five answers, nothing else.
func TestDispatchOneReplyPerMessageAcrossBusyCycles(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	rc := testRunningChannel(fa)

	release1 := make(chan struct{})
	started1 := make(chan struct{})
	var once1 sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once1.Do(func() { close(started1) })
		if in.MessageID == "p0" {
			<-release1
		}
		return Reply{FinalSummary: "final-" + in.MessageID}, nil
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

	if got := sentTexts(fa); len(got) != 3 {
		t.Fatalf("first busy cycle sends = %v want exactly 3 answers", got)
	}

	release2 := make(chan struct{})
	started2 := make(chan struct{})
	var once2 sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once2.Do(func() { close(started2) })
		if in.MessageID == "p3" {
			<-release2
		}
		return Reply{FinalSummary: "final-" + in.MessageID}, nil
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

	got := sentTexts(fa)
	if len(got) != 5 {
		t.Fatalf("sends = %v want exactly 5 answers for 5 messages", got)
	}
	for _, id := range []string{"p0", "p1", "p2", "p3", "p4"} {
		if countText(got, "final-"+id) != 1 {
			t.Fatalf("missing exactly one answer for %s in %v", id, got)
		}
	}
}

func TestDispatchFailureContinuesDrain(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	started := make(chan struct{})
	var once sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		time.Sleep(20 * time.Millisecond)
		if in.MessageID == "fail-me" {
			return Reply{}, errors.New("boom")
		}
		return Reply{FinalSummary: "final-" + in.MessageID}, nil
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
	if countText(got, turnFailureText(errors.New("boom"))) != 1 {
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
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
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
		return Reply{FinalSummary: "final-" + in.MessageID}, nil
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

func TestDispatchIdleSendsOnlyTheAnswer(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		time.Sleep(20 * time.Millisecond)
		return Reply{FinalSummary: "final-ok"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("idle-1"))
	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "final-ok" {
		t.Fatalf("idle path sends = %v want [final-ok]", got)
	}
}

// Whatever an internal error says, the user gets a short, actionable sentence
// with no stack, no internals and no raw error text.
func TestTurnFailureTextIsUserFacing(t *testing.T) {
	for _, err := range []error{
		errors.New(strings.Repeat("错", 250)),
		errors.New("assistant produced no reply"),
		errors.New("context deadline exceeded"),
		errors.New("goroutine 1 [running]: sandbox boom"),
	} {
		got := turnFailureText(err)
		if !utf8.ValidString(got) || strings.TrimSpace(got) == "" {
			t.Fatalf("turnFailureText(%v) = %q", err, got)
		}
		if utf8.RuneCountInString(got) > 80 {
			t.Fatalf("failure text is too long to read in chat: %q", got)
		}
		if strings.Contains(got, "goroutine") || ContainsInternalTerms(got) {
			t.Fatalf("failure text exposes internals: %q", got)
		}
		if strings.Contains(got, err.Error()) {
			t.Fatalf("failure text quotes the raw error: %q", got)
		}
	}
}

func TestClassifyProgressText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		kind ProgressKind
		ok   bool
	}{
		{"milestone marker", "[进度] 已打开 PR #12", ProgressMilestone, true},
		{"blocker marker", "[阻塞] CI 红了", ProgressBlocker, true},
		{"blocker strong phrase", "检查失败：权限不足", ProgressBlocker, true},
		{"confirm keyword", "请确认是否合并", ProgressConfirm, true},
		{"bare fail not blocker", "如果失败了再试一次吧这个说法", "", false},
		{"tool noise", "tool_call foo", "", false},
		{"short noise", "ok", "", false},
		{"plain chat", "正在想一下怎么做比较好呢这个说法", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev, ok := ClassifyProgressText(c.in)
			if ok != c.ok {
				t.Fatalf("ok=%v want %v (ev=%+v)", ok, c.ok, ev)
			}
			if ok && ev.Kind != c.kind {
				t.Fatalf("kind=%q want %q", ev.Kind, c.kind)
			}
		})
	}
}

func TestProgressAccumulatorStreamingChunks(t *testing.T) {
	// plan g2.2 / review v1: short ACP deltas coalesce; markers forward; tool noise skipped.
	acc := newProgressAccumulator()
	chunks := []string{
		"[进", "度] ", "已打", "开 PR #", "12\n",
		"tool_call foo bar\n",
		"[阻", "塞] ", "CI ", "红了",
	}
	var kinds []ProgressKind
	for _, c := range chunks {
		for _, pe := range acc.Feed(c) {
			kinds = append(kinds, pe.Kind)
		}
	}
	if len(kinds) != 2 || kinds[0] != ProgressMilestone || kinds[1] != ProgressBlocker {
		t.Fatalf("coalesced kinds = %v want [milestone blocker]", kinds)
	}
	// Dedup: feeding snapshot of same buffer must not re-emit.
	if more := acc.FeedSnapshot(acc.buf); len(more) != 0 {
		t.Fatalf("snapshot dedup leaked %v", more)
	}
}

func TestProgressAccumulatorFeedAfterSnapshotNoDouble(t *testing.T) {
	// review v1: production order is Status.partial update → ticker FeedSnapshot →
	// Subscribe Feed(same delta). Must not double-buffer or re-emit milestones.
	const delta = "[进度] 已打开 PR #12\n"
	acc := newProgressAccumulator()

	first := acc.FeedSnapshot(delta)
	if len(first) != 1 || first[0].Kind != ProgressMilestone {
		t.Fatalf("snapshot emit = %v want 1 milestone", first)
	}
	if acc.buf != delta {
		t.Fatalf("buf after snapshot = %q", acc.buf)
	}

	second := acc.Feed(delta)
	if len(second) != 0 {
		t.Fatalf("Feed after Snapshot re-emitted %v (buf doubled? %q)", second, acc.buf)
	}
	if acc.buf != delta {
		t.Fatalf("buf after duplicate Feed = %q want %q (must not append)", acc.buf, delta)
	}

	// Legitimate growth after snapshot still works (repeated identical chars too).
	acc2 := newProgressAccumulator()
	_ = acc2.Feed("x")
	_ = acc2.Feed("x")
	if acc2.buf != "xx" {
		t.Fatalf("repeated Feed deltas must append, got %q", acc2.buf)
	}

	// Feed-first then Snapshot of extended partial: catch-up Feed must not re-emit.
	acc3 := newProgressAccumulator()
	_ = acc3.Feed("[进度] 已")
	evSnap := acc3.FeedSnapshot("[进度] 已打开 PR #12\n")
	if len(evSnap) != 1 || evSnap[0].Kind != ProgressMilestone {
		t.Fatalf("snapshot after partial feed = %v want 1 milestone", evSnap)
	}
	evCatch := acc3.Feed("打开 PR #12\n")
	if len(evCatch) != 0 {
		t.Fatalf("Feed catch-up after Snapshot re-emitted %v buf=%q", evCatch, acc3.buf)
	}
	if acc3.buf != "[进度] 已打开 PR #12\n" {
		t.Fatalf("merged buf = %q", acc3.buf)
	}
}

func TestProgressAccumulatorBridgeDualChannelOrder(t *testing.T) {
	// bridge.forwardProgress: ticker FeedSnapshot(partial) interleaved with
	// Subscribe Feed(delta). Simulate Status-ahead race across several chunks.
	acc := newProgressAccumulator()
	var partial string
	var forwarded []string
	emit := func(events []ProgressEvent) {
		for _, pe := range events {
			forwarded = append(forwarded, string(pe.Kind)+":"+pe.Summary)
		}
	}
	chunks := []string{"[进度] ", "打开 PR", " #12\n", "[阻塞] CI 红了\n"}
	for _, d := range chunks {
		partial += d
		// Production: partial += delta happens before fanout; ticker may win.
		emit(acc.FeedSnapshot(partial))
		emit(acc.Feed(d))
	}
	if len(forwarded) != 2 {
		t.Fatalf("dual-channel forwarded %v want exactly 2 (milestone+blocker)", forwarded)
	}
	if !strings.HasPrefix(forwarded[0], "milestone:") || !strings.HasPrefix(forwarded[1], "blocker:") {
		t.Fatalf("kinds order = %v", forwarded)
	}
}

func TestProgressAccumulatorMultiChunkACPSequence(t *testing.T) {
	// Integration-style: multi-chunk ACP agent_message frames → accumulator → QQ kinds.
	acc := newProgressAccumulator()
	frames := []string{
		`{"type":"session_update","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"[确认]"}}}`,
		`{"type":"session_update","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":" 是否合并 PR"}}}`,
		`{"type":"session_update","update":{"sessionUpdate":"tool_call","content":{"type":"text","text":"gh pr view"}}}`,
	}
	var got []ProgressKind
	for _, raw := range frames {
		delta := services.ExtractAgentMessageText(json.RawMessage(raw))
		for _, pe := range acc.Feed(delta) {
			got = append(got, pe.Kind)
		}
	}
	if len(got) != 1 || got[0] != ProgressConfirm {
		t.Fatalf("ACP multi-chunk kinds = %v want [confirm]; tool_call must not forward", got)
	}
}

func TestDispatchForwardsOnlyExplicitlySendableProgress(t *testing.T) {
	// Only orchestration-marked progress, or a blocker/decision backed by a
	// structured conclusion, may leave the platform.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFuncWithProgress = func(ctx context.Context, rc ResolvedChannel, in InboundMessage, onProgress func(ProgressEvent)) (Reply, error) {
		onProgress(ProgressEvent{
			Kind: ProgressMilestone, Summary: "已提交分支",
			Stage: "branch_pushed", RunID: "run-1", Sendable: true,
		})
		onProgress(ProgressEvent{
			Kind: ProgressBlocker, Summary: "CI 红了",
			Blocked: true, Conclusion: "CI 检查失败，需要人工介入", RunID: "run-1",
		})
		return Reply{FinalSummary: "final-ok"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("prog-1"))
	got := sentTexts(fa)
	// A milestone is stated plainly; only a blocker is worth flagging as one.
	if countText(got, "已提交分支") != 1 {
		t.Fatalf("missing sendable milestone in %v", got)
	}
	if countText(got, "卡住了：CI 红了") != 1 {
		t.Fatalf("missing structured blocker in %v", got)
	}
	// Progress is not an answer. Suppressing the conclusion because an update
	// went out first is how "还在跑" became the last thing a user heard about a
	// task that had already finished.
	if countText(got, "final-ok") != 1 {
		t.Fatalf("the conclusion was swallowed by earlier progress: %v", got)
	}
	if len(got) != 3 {
		t.Fatalf("sends = %v want milestone + blocker + conclusion", got)
	}
	if got[len(got)-1] != "final-ok" {
		t.Fatalf("the conclusion did not come last: %v", got)
	}
}

func TestClassifiedProgressMarkersNeverReachChannel(t *testing.T) {
	// A prompt-shaped marker in model output must not be able to drive an
	// external send: classification produces internal-only events.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFuncWithProgress = func(ctx context.Context, rc ResolvedChannel, in InboundMessage, onProgress func(ProgressEvent)) (Reply, error) {
		for _, raw := range []string{"[进度] 已打开 PR #12", "[阻塞] CI 红了", "[确认] 是否合并"} {
			ev, ok := ClassifyProgressText(raw)
			if !ok {
				t.Fatalf("classification failed for %q", raw)
			}
			if ev.Deliverable() {
				t.Fatalf("classified event %q must not be deliverable", raw)
			}
			onProgress(ev)
		}
		return Reply{FinalSummary: "final-ok"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("prog-marker"))
	got := sentTexts(fa)
	for _, text := range got {
		if strings.HasPrefix(text, "进度：") || strings.HasPrefix(text, "阻塞：") || strings.HasPrefix(text, "需确认：") {
			t.Fatalf("classified progress leaked to channel: %v", got)
		}
	}
	if countText(got, "final-ok") != 1 {
		t.Fatalf("final missing in %v", got)
	}
}

func TestDeliverCronIdleImmediate(t *testing.T) {
	fa := &fakeAdapter{}
	m := newTestManager(fa)
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "c2c:user1",
	}})
	defer m.StopAll()

	if err := m.DeliverCron(cronDelivery("proj", "每小时PR", "unchanged", "PR 检查完毕，无变化")); err != nil {
		t.Fatalf("DeliverCron: %v", err)
	}
	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "PR：无变化" {
		t.Fatalf("idle unchanged push = %v want [PR：无变化]", got)
	}
}

func TestDeliverCronBusySilentEnqueueThenFlush(t *testing.T) {
	// plan g1.3 / g3.2: busy → silent enqueue (no side-chat); idle flush sends body only.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.mu.Lock()
	m.running["c1"] = &runningChannel{
		cfg: models.ChannelConfig{
			ID: "c1", Type: "qq", ProjectID: "proj", Enabled: true,
			CronDeliver: true, CronDeliverTarget: "c2c:user1",
		},
		adapter: fa,
	}
	m.mu.Unlock()

	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		once.Do(func() { close(started) })
		<-release
		return Reply{FinalSummary: "final-user"}, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.dispatch(context.Background(), rc, testInbound("u1"))
	}()
	<-started

	if !m.IsConversationBusy("proj", SceneC2C, "user1") {
		t.Fatal("expected busy during turn")
	}

	if err := m.DeliverCron(cronDelivery("proj", "每小时PR", "changed", "有 2 个新 PR")); err != nil {
		t.Fatalf("DeliverCron busy: %v", err)
	}
	mid := sentTexts(fa)
	for _, s := range mid {
		if strings.Contains(s, "入队") || strings.Contains(s, "有 2 个新 PR") {
			t.Fatalf("busy path must be silent (no side-chat / no body yet), got %v", mid)
		}
	}

	close(release)
	<-done

	got := sentTexts(fa)
	if countText(got, "有 2 个新 PR") != 1 {
		t.Fatalf("expected flush of cron body after idle, got %v", got)
	}
	if countText(got, "final-user") != 1 {
		t.Fatalf("expected user final, got %v", got)
	}
}

func TestPushQueueMergesUnchangedAndPriority(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.mu.Lock()
	m.running["c1"] = &runningChannel{
		cfg: models.ChannelConfig{
			ID: "c1", Type: "qq", ProjectID: "proj", Enabled: true,
			CronDeliver: true, CronDeliverTarget: "c2c:user1",
		},
		adapter: fa,
	}
	m.mu.Unlock()

	key := convKey("proj", SceneC2C, "user1")
	// Simulate busy so all enqueue.
	q := m.convQueueFor(key)
	q.mu.Lock()
	q.busy = true
	q.mu.Unlock()

	_ = m.DeliverCron(cronDelivery("proj", "每小时PR", "unchanged", "a"))
	_ = m.DeliverCron(cronDelivery("proj", "每小时PR", "unchanged", "b"))
	_ = m.DeliverCron(cronDelivery("proj", "日报", "changed", "日报有更新"))
	_ = m.DeliverCron(cronDelivery("proj", "每小时PR", "failed", "PR 拉取失败"))

	q.mu.Lock()
	q.busy = false
	q.mu.Unlock()
	m.flushPushQueue(key)

	got := sentTexts(fa)
	// Unchanged merged to latest template; changed/failed flushed; unchanged last by priority sort.
	if countText(got, "PR：无变化") != 1 {
		t.Fatalf("unchanged should merge to one, got %v", got)
	}
	if countText(got, "日报有更新") != 1 {
		t.Fatalf("expected changed daily push in %v", got)
	}
	if countText(got, "PR：PR 拉取失败") != 1 {
		t.Fatalf("expected failed push in %v", got)
	}
	// Priority: high (changed/failed) before unchanged.
	idxUnchanged := indexOf(got, "PR：无变化")
	if idxUnchanged < 0 {
		t.Fatalf("missing unchanged in %v", got)
	}
	for i, s := range got {
		if s == "日报有更新" || s == "PR：PR 拉取失败" {
			if i > idxUnchanged {
				t.Fatalf("high-priority item after unchanged: %v", got)
			}
		}
	}
}

func TestFormatCronPushTemplates(t *testing.T) {
	if got := FormatCronPush("每小时PR", CronResultUnchanged, "x"); got != "PR：无变化" {
		t.Fatalf("unchanged pr = %q", got)
	}
	if got := FormatCronPush("每日总结", CronResultUnchanged, "x"); got != "日报：无变化" {
		t.Fatalf("unchanged daily = %q", got)
	}
	if got := FormatCronPush("每小时PR", CronResultChanged, "有 1 个新 PR"); got != "有 1 个新 PR" {
		t.Fatalf("changed = %q", got)
	}
}

func TestFlushPushQueueMidBusyRequeuesRemaining(t *testing.T) {
	// review v2: ≥2 queued pushes; after first send becomes busy → remaining stay queued and flush later.
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.mu.Lock()
	m.running["c1"] = &runningChannel{
		cfg: models.ChannelConfig{
			ID: "c1", Type: "qq", ProjectID: "proj", Enabled: true,
			CronDeliver: true, CronDeliverTarget: "c2c:user1",
		},
		adapter: fa,
	}
	m.mu.Unlock()

	key := convKey("proj", SceneC2C, "user1")
	q := m.convQueueFor(key)
	q.mu.Lock()
	q.busy = true
	q.mu.Unlock()

	_ = m.DeliverCron(cronDelivery("proj", "每小时PR", "changed", "有 2 个新 PR"))
	_ = m.DeliverCron(cronDelivery("proj", "日报", "changed", "日报有更新"))
	_ = m.DeliverCron(cronDelivery("proj", "每小时PR", "failed", "PR 拉取失败"))

	// During flush of the first item, mark busy again so remaining must requeue.
	sendCount := 0
	fa.onSend = func(out OutboundMessage) {
		sendCount++
		if sendCount == 1 {
			q.mu.Lock()
			q.busy = true
			q.mu.Unlock()
		}
	}

	q.mu.Lock()
	q.busy = false
	q.mu.Unlock()
	m.flushPushQueue(key)

	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("mid-busy flush should send exactly 1 item, got %v", got)
	}
	pq := m.pushQueueFor(key)
	pq.mu.Lock()
	remaining := len(pq.pending)
	pq.mu.Unlock()
	if remaining < 2 {
		t.Fatalf("expected ≥2 remaining after mid-busy requeue, got %d (sent=%v)", remaining, got)
	}

	// Idle again → remaining flush completely.
	q.mu.Lock()
	q.busy = false
	q.mu.Unlock()
	fa.onSend = nil
	m.flushPushQueue(key)
	got = sentTexts(fa)
	if countText(got, "有 2 个新 PR") != 1 || countText(got, "日报有更新") != 1 || countText(got, "PR：PR 拉取失败") != 1 {
		t.Fatalf("all pushes must eventually send, got %v", got)
	}
}

func TestFlushPushQueueMidBusyRespectsDepth(t *testing.T) {
	// review v2: during flush, new DeliverCron fills queue; mid-busy requeue must
	// still honor pushQueueDepth + unchanged merge (not blind prepend).
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.mu.Lock()
	m.running["c1"] = &runningChannel{
		cfg: models.ChannelConfig{
			ID: "c1", Type: "qq", ProjectID: "proj", Enabled: true,
			CronDeliver: true, CronDeliverTarget: "c2c:user1",
		},
		adapter: fa,
	}
	m.mu.Unlock()

	key := convKey("proj", SceneC2C, "user1")
	q := m.convQueueFor(key)
	q.mu.Lock()
	q.busy = true
	q.mu.Unlock()

	// Seed flush batch.
	for i := 0; i < 3; i++ {
		_ = m.DeliverCron(cronDelivery("proj", fmt.Sprintf("job-%d", i), "changed", fmt.Sprintf("body-%d", i)))
	}

	sendCount := 0
	fa.onSend = func(out OutboundMessage) {
		sendCount++
		if sendCount == 1 {
			// While first item sends, flood push queue + mark busy for requeue.
			for i := 0; i < pushQueueDepth; i++ {
				m.enqueuePush(key, CronPushItem{
					ProjectID: "proj", Scene: SceneC2C, Conv: "user1",
					Category: fmt.Sprintf("flood-%d", i), Kind: CronResultChanged,
					Text: fmt.Sprintf("flood-body-%d", i),
				})
			}
			q.mu.Lock()
			q.busy = true
			q.mu.Unlock()
		}
	}

	q.mu.Lock()
	q.busy = false
	q.mu.Unlock()
	m.flushPushQueue(key)

	pq := m.pushQueueFor(key)
	pq.mu.Lock()
	n := len(pq.pending)
	pq.mu.Unlock()
	if n > pushQueueDepth {
		t.Fatalf("requeue exceeded depth: pending=%d want ≤%d", n, pushQueueDepth)
	}
	if n == 0 {
		t.Fatalf("expected remaining items after mid-busy requeue")
	}
}

func TestPushQueueFullRetainsNewestFailed(t *testing.T) {
	// review v4: depth full of high-priority → newest failed/changed retained (not silent drop).
	m := NewManager(nil, nil, nil)
	key := convKey("proj", SceneC2C, "user1")
	for i := 0; i < pushQueueDepth; i++ {
		m.enqueuePush(key, CronPushItem{
			ProjectID: "proj", Scene: SceneC2C, Conv: "user1",
			Category: fmt.Sprintf("cat-%d", i), Kind: CronResultChanged,
			Text: fmt.Sprintf("changed-%d", i),
		})
	}
	m.enqueuePush(key, CronPushItem{
		ProjectID: "proj", Scene: SceneC2C, Conv: "user1",
		Category: "critical", Kind: CronResultFailed, Text: "最新失败不可丢",
	})
	pq := m.pushQueueFor(key)
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if len(pq.pending) != pushQueueDepth {
		t.Fatalf("queue len = %d want %d", len(pq.pending), pushQueueDepth)
	}
	found := false
	for _, p := range pq.pending {
		if p.Kind == CronResultFailed && strings.Contains(p.Text, "最新失败不可丢") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("newest failed missing from full queue: %+v", pq.pending)
	}
}

// TestPushQueueProtectsRunNotify covers review v3: when the high-priority ring
// is full, run_notify (already receipt-claimed) must not be silently dropped.
func TestPushQueueProtectsRunNotify(t *testing.T) {
	m := NewManager(nil, nil, nil)
	key := convKey("proj", SceneC2C, "user1")

	// Fill with cron changed items + one protected run_notify.
	for i := 0; i < pushQueueDepth-1; i++ {
		m.enqueuePush(key, CronPushItem{
			ProjectID: "proj", Scene: SceneC2C, Conv: "user1",
			Category: fmt.Sprintf("cron-%d", i), Kind: CronResultChanged,
			Text: fmt.Sprintf("cron-%d", i),
		})
	}
	m.enqueuePush(key, CronPushItem{
		ProjectID: "proj", Scene: SceneC2C, Conv: "user1",
		Category: runNotifyCategory, Kind: CronResultChanged,
		Text: "gate waiting_human",
	})

	// Incoming cron failed should drop an unprotected cron changed, not run_notify.
	m.enqueuePush(key, CronPushItem{
		ProjectID: "proj", Scene: SceneC2C, Conv: "user1",
		Category: "cron-new", Kind: CronResultFailed, Text: "cron failed",
	})

	pq := m.pushQueueFor(key)
	pq.mu.Lock()
	defer pq.mu.Unlock()
	foundRunNotify := false
	for _, p := range pq.pending {
		if p.Category == runNotifyCategory && strings.Contains(p.Text, "gate waiting_human") {
			foundRunNotify = true
			break
		}
	}
	if !foundRunNotify {
		t.Fatalf("run_notify was dropped under pressure: %+v", pq.pending)
	}

	// When queue is all run_notify, a new run_notify soft-overflows past depth.
	m2 := NewManager(nil, nil, nil)
	key2 := convKey("proj2", SceneC2C, "user2")
	for i := 0; i < pushQueueDepth; i++ {
		m2.enqueuePush(key2, CronPushItem{
			ProjectID: "proj2", Scene: SceneC2C, Conv: "user2",
			Category: runNotifyCategory, Kind: CronResultChanged,
			Text: fmt.Sprintf("rn-%d", i),
		})
	}
	m2.enqueuePush(key2, CronPushItem{
		ProjectID: "proj2", Scene: SceneC2C, Conv: "user2",
		Category: runNotifyCategory, Kind: CronResultChanged,
		Text: "rn-overflow",
	})
	pq2 := m2.pushQueueFor(key2)
	pq2.mu.Lock()
	defer pq2.mu.Unlock()
	if len(pq2.pending) < pushQueueDepth+1 {
		t.Fatalf("expected soft-overflow len>=%d got %d", pushQueueDepth+1, len(pq2.pending))
	}
	foundOverflow := false
	for _, p := range pq2.pending {
		if p.Text == "rn-overflow" {
			foundOverflow = true
			break
		}
	}
	if !foundOverflow {
		t.Fatalf("soft-overflow run_notify missing: %+v", pq2.pending)
	}
}

// The work-layer preamble is an operational contract, not a second persona.
// Voice, identity, and filler bans live on the conversation layer; this text
// only has to name the two endings, the capture handoff, and confirmation.
func TestChannelPreambleStatesTheLiveTurnContract(t *testing.T) {
	p := ChannelPreamble("qq")
	for _, required := range []string{
		"pm_reply", "pm_start_run", "pm_notify_progress",
		"会话层", "already_replied", "needs_confirmation", "不能代替用户确认", "兜底",
	} {
		if !strings.Contains(p, required) {
			t.Fatalf("preamble never mentions %s: %s", required, p)
		}
	}
	// Style/persona belong on liveSystemPrompt — repeating them here is what
	// made sandbox sessions look like a second director prompt.
	for _, voiceOnly := range []string{"你是什么模型", "厂商", "稍等，我看一下", "收到，正在处理"} {
		if strings.Contains(p, voiceOnly) {
			t.Fatalf("work-layer preamble still carries conversation-layer voice rule %q: %s", voiceOnly, p)
		}
	}
}

// The conversation model shares the persona, so it needs the same rule: it is
// the one layer that answers chit-chat directly, which is exactly where 「你是
// 什么模型」 lands.
func TestLiveSystemPromptWithholdsTheModelIdentity(t *testing.T) {
	for _, required := range []string{"你是什么模型", "厂商", "负责人"} {
		if !strings.Contains(liveSystemPrompt, required) {
			t.Fatalf("live prompt does not close the identity question (%s): %s", required, liveSystemPrompt)
		}
	}
}

func TestLiveSystemPromptAnswersFollowupsFromDeliveryFacts(t *testing.T) {
	for _, required := range []string{"追问", "result_summary", "名词百科"} {
		if !strings.Contains(liveSystemPrompt, required) {
			t.Fatalf("live prompt missing delivery follow-up rule (%s): %s", required, liveSystemPrompt)
		}
	}
}

func TestClassifyCronResultDelegatesToServices(t *testing.T) {
	// review v5: single shared classification with services.ClassifyCronDeliveryText.
	cases := []struct {
		in   string
		want CronResultKind
	}{
		{"PR：无变化", CronResultUnchanged},
		{"失败：timeout", CronResultFailed},
		{"opened PR #12", CronResultChanged},
	}
	for _, c := range cases {
		if got := ClassifyCronResult(c.in); got != c.want {
			t.Errorf("ClassifyCronResult(%q) = %q want %q (services=%q)",
				c.in, got, c.want, services.ClassifyCronDeliveryText(c.in))
		}
	}
}

func cronDelivery(projectID, category, kind, text string) services.CronDelivery {
	return services.CronDelivery{ProjectID: projectID, Category: category, Kind: kind, Text: text}
}
