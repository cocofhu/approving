package channels

import (
	"context"
	"errors"
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

func TestDispatchBusyNoDelayedAck(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.ackDelay = 40 * time.Millisecond
	started := make(chan struct{})
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		close(started)
		time.Sleep(150 * time.Millisecond)
		return Reply{Text: "final-ok"}, nil
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
	busyCount, ackCount, finalCount := 0, 0, 0
	for _, text := range got {
		switch text {
		case busyReplyText:
			busyCount++
		case delayedAckText:
			ackCount++
		case "final-ok":
			finalCount++
		}
	}
	if busyCount != 1 || finalCount != 1 || ackCount != 1 {
		// Busy path must not start its own delayed ack; only the in-flight turn may ack once.
		t.Fatalf("busy path sends = %v (busy=%d ack=%d final=%d)", got, busyCount, ackCount, finalCount)
	}
	// Second inbound must be busy-only: ensure busy appears and no second ack from busy path.
	if got[0] != busyReplyText && got[1] != busyReplyText {
		// Order: busy may arrive before or after the first turn's ack; just require exactly one busy.
		t.Fatalf("expected busy reply among sends, got %v", got)
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
