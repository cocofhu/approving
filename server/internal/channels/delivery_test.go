package channels

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func deliveryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(
		sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Discard},
	)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	return db
}

// policyManager builds a Manager with a DB-backed policy and no retry sleeping.
func policyManager(t *testing.T, fa *fakeAdapter, audit sendable.AuditFunc) (*Manager, *gorm.DB) {
	t.Helper()
	db := deliveryDB(t)
	m := newTestManager(fa)
	m.SetSendablePolicy(sendable.NewPolicy(db, audit))
	m.SetRetryBackoff(func(int) time.Duration { return 0 })
	return m, db
}

func TestFinalOnlySendsStructuredSummaryNeverRawAssistantText(t *testing.T) {
	const raw = "内部推理：我先读了配置文件，然后决定改动 auth 模块，密钥是 sk-secret。\n[摘要] 即使带提示词标签也不能外发"
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		return Reply{Text: raw}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("final-raw"))

	got := sentTexts(fa)
	for _, text := range got {
		if strings.Contains(text, "内部推理") || strings.Contains(text, "sk-secret") || strings.Contains(text, "提示词标签") {
			t.Fatalf("raw assistant final leaked: %v", got)
		}
	}
	if countText(got, safeFinalNotice) != 1 {
		t.Fatalf("expected the safe notice when no structured summary exists, got %v", got)
	}

	fa2 := &fakeAdapter{}
	m2 := NewManager(nil, nil, nil)
	m2.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		return Reply{Text: raw, FinalSummary: "已完成 auth 模块改动"}, nil
	}
	m2.dispatch(context.Background(), testRunningChannel(fa2), testInbound("final-structured"))
	got2 := sentTexts(fa2)
	if countText(got2, "已完成 auth 模块改动") != 1 {
		t.Fatalf("structured summary missing in %v", got2)
	}
	for _, text := range got2 {
		if strings.Contains(text, "内部推理") {
			t.Fatalf("raw text leaked alongside summary: %v", got2)
		}
	}
}

// failingAdapter fails the first failures sends, then succeeds.
type failingAdapter struct {
	fakeAdapter
	failures int
	attempts int
}

func (f *failingAdapter) Send(ctx context.Context, out OutboundMessage) error {
	f.mu.Lock()
	f.attempts++
	attempt := f.attempts
	f.mu.Unlock()
	if attempt <= f.failures {
		return errors.New("transport down")
	}
	return f.fakeAdapter.Send(ctx, out)
}

func TestSendOutboundRetriesThenSucceedsExactlyOnce(t *testing.T) {
	fa := &failingAdapter{failures: 2}
	var audits []sendable.AuditEntry
	m, db := policyManager(t, &fa.fakeAdapter, func(e sendable.AuditEntry) { audits = append(audits, e) })
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "c2c:user1",
	}})
	defer m.StopAll()
	// Replace the started adapter with the failing one.
	m.mu.Lock()
	for _, rc := range m.running {
		rc.adapter = fa
	}
	m.mu.Unlock()

	if err := m.DeliverCron(cronDelivery("proj", "每小时PR", "changed", "PR 有更新")); err != nil {
		t.Fatalf("DeliverCron: %v", err)
	}
	if fa.attempts != 3 {
		t.Fatalf("adapter attempts = %d want 3 bounded attempts", fa.attempts)
	}
	if got := sentTexts(&fa.fakeAdapter); len(got) != 1 {
		t.Fatalf("delivered %v want exactly one successful send", got)
	}

	var receipt models.SendableDeliveryReceipt
	if err := db.First(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "sent" || receipt.Attempts != 3 {
		t.Fatalf("receipt = %+v want sent after 3 attempts", receipt)
	}
	sent, failed := 0, 0
	for _, entry := range audits {
		switch entry.Result {
		case "sent":
			sent++
		case "failed":
			failed++
		}
	}
	if sent != 1 || failed != 2 {
		t.Fatalf("audit results sent=%d failed=%d want 1/2", sent, failed)
	}
}

func TestSendOutboundStopsAfterExhaustingAttempts(t *testing.T) {
	fa := &failingAdapter{failures: 99}
	m, db := policyManager(t, &fa.fakeAdapter, nil)
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "c2c:user1",
	}})
	defer m.StopAll()
	m.mu.Lock()
	for _, rc := range m.running {
		rc.adapter = fa
	}
	m.mu.Unlock()

	if err := m.DeliverCron(cronDelivery("proj", "每小时PR", "changed", "PR 有更新")); err != nil {
		t.Fatalf("DeliverCron: %v", err)
	}
	if fa.attempts != 3 {
		t.Fatalf("adapter attempts = %d want 3", fa.attempts)
	}
	var receipt models.SendableDeliveryReceipt
	if err := db.First(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "failed" || receipt.Attempts != 3 {
		t.Fatalf("receipt = %+v want failed after 3 attempts", receipt)
	}
}

func TestSendRunAcceptanceAckOncePerRunAndRejectsMissingRun(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := policyManager(t, fa, nil)
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "c2c:user1",
	}})
	defer m.StopAll()

	ack := RunAcceptanceAck{
		ProjectID: "proj", RunID: "run-1", Scene: SceneC2C,
		ConversationID: "user1", UserID: "u1", ShortTitle: "登录页", Language: "zh-CN",
	}
	if err := m.SendRunAcceptanceAck(context.Background(), ack); err != nil {
		t.Fatalf("first ack: %v", err)
	}
	if err := m.SendRunAcceptanceAck(context.Background(), ack); err != nil {
		t.Fatalf("second ack: %v", err)
	}
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("run acceptance ACK sent %v want exactly once per run", got)
	}
	if !strings.HasPrefix(got[0], "【登录页｜已接单】") {
		t.Fatalf("ack text = %q want short title prefix", got[0])
	}

	ack.RunID = ""
	if err := m.SendRunAcceptanceAck(context.Background(), ack); err == nil {
		t.Fatal("run acceptance ACK without a real run id must be rejected")
	}
}

func TestTurnProcessingAckIsNotRunScoped(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		return Reply{FinalSummary: "done-" + in.MessageID}, nil
	}
	rc := testRunningChannel(fa)
	m.dispatch(context.Background(), rc, testInbound("turn-1"))
	m.dispatch(context.Background(), rc, testInbound("turn-2"))

	if n := hasPrefixCount(sentTexts(fa), ackProcessingPrefix); n != 2 {
		t.Fatalf("processing ACK count = %d want one per turn", n)
	}
	var receipts []models.SendableDeliveryReceipt
	if err := db.Find(&receipts).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range receipts {
		if r.RunID != "" {
			t.Fatalf("turn delivery carried a fabricated run id: %+v", r)
		}
		if r.TaskContext == "" {
			t.Fatalf("turn delivery has no task scope: %+v", r)
		}
		if r.Kind == string(sendable.KindRunAcceptanceAck) {
			t.Fatalf("turn ACK was recorded as a run acceptance ACK: %+v", r)
		}
	}
}

func TestResolveTaskReferenceQQFlow(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	svc := services.NewTaskContextService(db)
	m.SetTaskContextService(svc)

	projectID, qqUser := "proj", "u1"
	userID := services.SyntheticQQUserID(qqUser)
	for _, spec := range []struct{ run, title string }{
		{"r1", "支付登录页"},
		{"r2", "用户登录页"},
	} {
		if _, err := svc.EnsureIdentity(services.EnsureTaskIdentityInput{
			RunID: spec.run, ProjectID: projectID, UserID: userID,
			ShortTitle: spec.title, OriginalRequirement: "实现" + spec.title, Status: "active",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Another QQ identity in the same project must stay invisible.
	if _, err := svc.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "r3", ProjectID: projectID, UserID: services.SyntheticQQUserID("intruder"),
		ShortTitle: "登录页", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}

	base := TaskReferenceRequest{
		ProjectID: projectID, ChannelType: models.ChannelTypeQQ, Scene: SceneC2C,
		ConversationID: "c1", QQUserID: qqUser,
	}

	// QQ has no reply reference, so a quoted message id is ignored entirely.
	ambiguous := base
	ambiguous.Text = "登录页"
	ambiguous.ReplyToMessageID = "m-quoted"
	res, err := m.ResolveTaskReference(ambiguous)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != TaskReferenceAmbiguous || len(res.Options) != 2 {
		t.Fatalf("ambiguous result = %+v", res)
	}
	if res.ReplyRefSupported {
		t.Fatal("QQ must report reply references as unsupported")
	}
	if !strings.Contains(res.Message, "不支持引用回复") || !strings.Contains(res.Message, "1. ") {
		t.Fatalf("ambiguity prompt = %q", res.Message)
	}

	// The ordinal selects from exactly the options the user was shown.
	pick := base
	pick.Text = "2"
	res, err = m.ResolveTaskReference(pick)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != TaskReferenceResolved || res.Task == nil {
		t.Fatalf("ordinal selection = %+v", res)
	}
	picked := res.Task.RunID

	// Focus now answers a follow-up with no task words at all.
	followUp := base
	followUp.Text = ""
	res, err = m.ResolveTaskReference(followUp)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != TaskReferenceResolved || res.Task.RunID != picked {
		t.Fatalf("focus follow-up = %+v want %s", res, picked)
	}

	// A unique short title resolves directly and is formatted for the user.
	unique := base
	unique.Text = "支付登录页"
	res, err = m.ResolveTaskReference(unique)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != TaskReferenceResolved || res.Task.RunID != "r1" {
		t.Fatalf("unique match = %+v", res)
	}
	if !strings.HasPrefix(res.Message, "【支付登录页｜进行中】") {
		t.Fatalf("resolved message = %q", res.Message)
	}

	// No match must not guess.
	missing := base
	missing.Text = "结算页面"
	res, err = m.ResolveTaskReference(missing)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != TaskReferenceNotFound || res.Task != nil {
		t.Fatalf("no-match result = %+v", res)
	}

	// English input switches the copy.
	english := base
	english.Text = "unknown checkout task"
	res, err = m.ResolveTaskReference(english)
	if err != nil {
		t.Fatal(err)
	}
	if res.Language != "en" || !strings.Contains(res.Message, "No matching task") {
		t.Fatalf("english fallback = %+v", res)
	}
}
