package channels

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func hardeningDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedReceipt(t *testing.T, db *gorm.DB, key, status string, attempts int, lastTried time.Time) {
	t.Helper()
	tried := lastTried
	row := models.DeliveryReceipt{
		DedupeKey: key, Status: status, Attempts: attempts, LastTriedAt: &tried,
		CreatedAt: lastTried, UpdatedAt: lastTried,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
}

func blockedEnvelope(key string) Envelope {
	return Envelope{
		Channels: []string{"qq"}, Priority: PriorityImmediate,
		Reason: ReasonBlocked, Type: DeliveryTypeBlocked, DedupeKey: key,
		Context: RunTaskContext{ProjectID: "p", RunID: "r", UserID: "u"},
	}
}

func TestDeliveryPendingLeaseReclaimsStaleAndProtectsActive(t *testing.T) {
	db := hardeningDB(t)
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.SetDeliveryPersistence(db, nil)
	rc := &runningChannel{cfg: models.ChannelConfig{ProjectID: "p", Type: "qq"}, adapter: fa}
	out := OutboundMessage{Scene: SceneC2C, ConversationID: "c", Text: "blocked"}

	// A crashed sender leaves a pending row nobody will ever finish.
	stale := time.Now().Add(-deliveryPendingLease - time.Minute)
	seedReceipt(t, db, "stale-key", models.DeliveryReceiptPending, 1, stale)
	if err := m.AppendSendable(context.Background(), rc, out, blockedEnvelope("stale-key")); err != nil {
		t.Fatalf("stale pending lease was not reclaimed after restart: %v", err)
	}
	var reclaimed models.DeliveryReceipt
	if err := db.First(&reclaimed, "dedupe_key = ?", "stale-key").Error; err != nil {
		t.Fatal(err)
	}
	if reclaimed.Status != models.DeliveryReceiptSent || reclaimed.Attempts != 2 {
		t.Fatalf("reclaimed receipt = %#v", reclaimed)
	}

	// A live concurrent sender still owns its lease.
	seedReceipt(t, db, "active-key", models.DeliveryReceiptPending, 1, time.Now())
	err := m.AppendSendable(context.Background(), rc, out, blockedEnvelope("active-key"))
	if !errors.Is(err, ErrDeliverySuppressed) {
		t.Fatalf("active pending lease was stolen: %v", err)
	}

	// An exhausted pending row from a crash is reclaimable once stale.
	seedReceipt(t, db, "exhausted-key", models.DeliveryReceiptPending, maxDeliveryAttempts, stale)
	if err := m.AppendSendable(context.Background(), rc, out, blockedEnvelope("exhausted-key")); err != nil {
		t.Fatalf("exhausted stale pending stayed permanently suppressed: %v", err)
	}

	fa.mu.Lock()
	defer fa.mu.Unlock()
	if len(fa.sent) != 2 {
		t.Fatalf("sends = %d, want 2 (stale reclaim + exhausted reclaim)", len(fa.sent))
	}
}

// switchableAdapter fails until healthy is set, so a rolled-back rate-limit
// window can be observed directly.
type switchableAdapter struct {
	mu      sync.Mutex
	healthy bool
	sent    []OutboundMessage
}

func (a *switchableAdapter) Type() string                                { return "qq" }
func (a *switchableAdapter) Start(context.Context, InboundHandler) error { return nil }
func (a *switchableAdapter) Stop() error                                 { return nil }
func (a *switchableAdapter) Send(_ context.Context, out OutboundMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.healthy {
		return errors.New("transport down")
	}
	a.sent = append(a.sent, out)
	return nil
}

func TestProgressWindowCommitsOnlyAfterAdapterSuccess(t *testing.T) {
	a := &switchableAdapter{}
	m := NewManager(nil, nil, nil)
	defer m.StopAll()
	rc := &runningChannel{cfg: models.ChannelConfig{ProjectID: "p", Type: "qq"}, adapter: a}
	env := Envelope{
		Channels: []string{"qq"}, Reason: ReasonProgress, Type: DeliveryTypeStage,
		Context: RunTaskContext{ProjectID: "p", RunID: "run-1"},
	}
	progress := func(text string) error {
		out := OutboundMessage{Scene: SceneC2C, ConversationID: "c", Text: text}
		e := env
		e.DedupeKey = "progress:" + text
		return m.AppendSendable(context.Background(), rc, out, e)
	}
	key := progressDeliveryKey(env, OutboundMessage{Scene: SceneC2C, ConversationID: "c"})
	state := func() deliveryProgressState {
		m.deliveryPolicy.mu.Lock()
		defer m.deliveryPolicy.mu.Unlock()
		return m.deliveryPolicy.progress[key]
	}

	if err := progress("step 1"); err == nil {
		t.Fatal("failing adapter should report the failed progress cycle")
	}
	if got := state(); !got.lastSent.IsZero() || got.inflight {
		t.Fatalf("failed send advanced the rate-limit window: %#v", got)
	}
	if got := state().latest; got != "step 1" {
		t.Fatalf("merged latest = %q", got)
	}

	a.mu.Lock()
	a.healthy = true
	a.mu.Unlock()

	// Because the window rolled back, the next progress goes out immediately.
	if err := progress("step 2"); err != nil {
		t.Fatalf("rolled-back window did not allow the next progress: %v", err)
	}
	committed := state()
	if committed.lastSent.IsZero() || committed.inflight {
		t.Fatalf("successful send did not commit the window: %#v", committed)
	}

	if err := progress("step 3"); !errors.Is(err, ErrDeliverySuppressed) {
		t.Fatalf("committed window did not rate-limit: %v", err)
	}
	merged := state()
	if merged.latest != "step 3" || !merged.lastSent.Equal(committed.lastSent) {
		t.Fatalf("merge lost the latest text or moved the window: %#v", merged)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.sent) != 1 || a.sent[0].Text != "step 2" {
		t.Fatalf("adapter sends = %#v", a.sent)
	}
}

func TestTurnProgressHandlerOnlyAuthorizesStructuredProducer(t *testing.T) {
	fa := &fakeAdapter{}
	// No handleFunc/handleFuncWithProgress: this is the production path.
	m := NewManager(nil, nil, nil)
	defer m.StopAll()
	rc := &runningChannel{cfg: models.ChannelConfig{ProjectID: "p", Type: "qq"}, adapter: fa}
	in := InboundMessage{Scene: SceneC2C, ConversationID: "c", MessageID: "m1", UserID: "u"}
	onProgress := m.turnProgressHandler(context.Background(), rc, in)

	classified, ok := ClassifyProgressText("【进度】已打开 PR #12")
	if !ok {
		t.Fatal("fixture text should classify")
	}
	classified.RunID = "run-1"
	onProgress(classified)
	fa.mu.Lock()
	if len(fa.sent) != 0 {
		fa.mu.Unlock()
		t.Fatalf("marker text reached the transport: %#v", fa.sent)
	}
	fa.mu.Unlock()

	onProgress(NewSendableProgressEvent(ProgressMilestone, "运行进度 40%", "run-1"))
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if len(fa.sent) != 1 || !strings.Contains(fa.sent[0].Text, "运行进度 40%") {
		t.Fatalf("structured producer did not reach the transport: %#v", fa.sent)
	}
}

type stubRunState struct {
	mu  sync.Mutex
	run models.Run
}

func (s *stubRunState) Get(string) (models.Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run, true
}

func (s *stubRunState) set(status string, progress float64) {
	s.mu.Lock()
	s.run.Status, s.run.Progress = status, progress
	s.mu.Unlock()
}

func TestRunStateProgressUsesServerStateOnly(t *testing.T) {
	first, snap, changed := runStateProgress(models.Run{Status: "running", Progress: 0.2}, runStateSnapshot{})
	if changed {
		t.Fatalf("baseline observation emitted %#v", first)
	}
	ev, snap, changed := runStateProgress(models.Run{Status: "running", Progress: 0.45}, snap)
	if !changed || ev.Kind != ProgressMilestone || ev.Summary != "运行进度 40%" {
		t.Fatalf("progress bucket = %#v changed=%v", ev, changed)
	}
	if _, snap, changed = runStateProgress(models.Run{Status: "running", Progress: 0.47}, snap); changed {
		t.Fatal("sub-bucket tick became a message")
	}
	ev, snap, changed = runStateProgress(models.Run{Status: "waiting_human", Progress: 0.47}, snap)
	if !changed || ev.Kind != ProgressConfirm {
		t.Fatalf("waiting_human = %#v changed=%v", ev, changed)
	}
	ev, _, changed = runStateProgress(models.Run{Status: "failed", Progress: 0.47}, snap)
	if !changed || ev.Kind != ProgressBlocker {
		t.Fatalf("failed = %#v changed=%v", ev, changed)
	}
}

func TestWatchRunStateEmitsAuthorizedEvents(t *testing.T) {
	src := &stubRunState{run: models.Run{ID: "run-1", Status: "running", Progress: 0.1}}
	bridge := NewChannelBridge(nil, nil, nil, MCPTokenHooks{})
	bridge.SetRunState(src)

	events := make(chan ProgressEvent, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bridge.watchRunState(ctx, "run-1", func(ev ProgressEvent) { events <- ev })

	time.Sleep(1200 * time.Millisecond) // let the watcher take its baseline
	src.set("waiting_human", 0.1)
	select {
	case ev := <-events:
		if ev.Kind != ProgressConfirm || !ev.authorized || ev.RunID != "run-1" {
			t.Fatalf("watcher event = %#v", ev)
		}
	case <-ctx.Done():
		t.Fatal("watcher produced no structured progress")
	}
}

func TestFinalAttachmentsRequireStructuredProducer(t *testing.T) {
	stripped, summary, images := finalFromAssistantText("看这里 ![](https://x.com/a.png)")
	if len(images) != 0 {
		t.Fatalf("raw assistant text authorized attachments: %v", images)
	}
	if stripped != "看这里" || summary != "处理完成，请在项目中查看结果。" {
		t.Fatalf("unmarked final = %q / %q", stripped, summary)
	}
	_, summary, images = finalFromAssistantText("[final] done ![](https://x.com/a.png)")
	if summary != "done" || len(images) != 1 || images[0] != "https://x.com/a.png" {
		t.Fatalf("structured final = %q / %v", summary, images)
	}
}

func TestManagerFinalSendsOnlyAuthorizedAttachments(t *testing.T) {
	fa := &fakeAdapter{}
	m := newTestManager(fa)
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		return Reply{Text: "raw ![](https://x.com/leak.png)", RunID: "run-1"}, nil
	}
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
	}})
	defer m.StopAll()
	m.mu.Lock()
	rc := m.running["c1"]
	m.mu.Unlock()

	in := InboundMessage{Scene: SceneC2C, ConversationID: "c", MessageID: "m1", Text: "hi"}
	m.runTurn(context.Background(), rc, in, false)

	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		return Reply{
			RunID: "run-2",
			Final: &TurnFinalReport{OK: true, Summary: "报告", ImageURLs: []string{"https://x.com/ok.png"}},
		}, nil
	}
	in2 := in
	in2.MessageID = "m2"
	m.runTurn(context.Background(), rc, in2, false)

	fa.mu.Lock()
	defer fa.mu.Unlock()
	if len(fa.sent) != 2 {
		t.Fatalf("sends = %d", len(fa.sent))
	}
	if len(fa.sent[0].ImageURLs) != 0 {
		t.Fatalf("raw assistant image leaked to final: %v", fa.sent[0].ImageURLs)
	}
	if len(fa.sent[1].ImageURLs) != 1 || fa.sent[1].ImageURLs[0] != "https://x.com/ok.png" {
		t.Fatalf("structured final attachment dropped: %v", fa.sent[1].ImageURLs)
	}
}

func TestRunAcceptanceFiresOnAssociationAndSurvivesReconnect(t *testing.T) {
	db := hardeningDB(t)
	if err := db.Create(&models.WorkflowDef{ID: "wf-1", ProjectID: "p1", Name: "发布工作流"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Run{
		ID: "run-a", WorkflowID: "wf-1", Title: "支付任务一", Status: "running",
		Inputs: map[string]any{"requirement": "支付服务"}, StartedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	in := InboundMessage{
		Scene: SceneC2C, ConversationID: "openid-1", UserID: "openid-1",
		Text: "支付任务一 状态", MessageID: "m1",
	}
	cfg := models.ChannelConfig{ID: "c1", Type: "qq", ProjectID: "p1", AppID: "app", Enabled: true}

	newStack := func(fa *fakeAdapter) (*Manager, *ChannelBridge, ResolvedChannel) {
		bridge := NewChannelBridge(nil, nil, nil, MCPTokenHooks{})
		bridge.SetTaskContext(services.NewTaskContextService(db))
		m := NewManager(bridge, map[string]AdapterFactory{
			"qq": func(AdapterConfig) (Adapter, error) { return fa, nil },
		}, func(s string) (string, error) { return s, nil })
		m.SetDeliveryPersistence(db, nil)
		m.Apply([]models.ChannelConfig{cfg})
		return m, bridge, ResolvedChannel{Type: "qq", ProjectID: "p1"}
	}

	fa := &fakeAdapter{}
	m, bridge, rc := newStack(fa)
	defer m.StopAll()
	if _, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: in}); err != nil {
		t.Fatal(err)
	}
	fa.mu.Lock()
	acks := len(fa.sent)
	var first string
	if acks > 0 {
		first = fa.sent[0].Text
	}
	fa.mu.Unlock()
	if acks != 1 || !strings.Contains(first, "已接收") || !strings.HasPrefix(first, "【支付任务一") {
		t.Fatalf("first association ack = %d %q", acks, first)
	}

	// Same association again inside the same process.
	repeat := in
	repeat.MessageID = "m2"
	if _, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: repeat}); err != nil {
		t.Fatal(err)
	}
	fa.mu.Lock()
	again := len(fa.sent)
	fa.mu.Unlock()
	if again != 1 {
		t.Fatalf("acceptance ack repeated in-process: %d", again)
	}

	// Reconnect: fresh manager/bridge, same persistent receipts.
	fa2 := &fakeAdapter{}
	m2, bridge2, rc2 := newStack(fa2)
	defer m2.StopAll()
	reconnect := in
	reconnect.MessageID = "m3"
	if _, err := bridge2.PreflightInbound(InboundPreflightRequest{Channel: rc2, Message: reconnect}); err != nil {
		t.Fatal(err)
	}
	fa2.mu.Lock()
	defer fa2.mu.Unlock()
	if len(fa2.sent) != 0 {
		t.Fatalf("reconnect re-sent the acceptance ack: %#v", fa2.sent)
	}
}
