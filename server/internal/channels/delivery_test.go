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
		// Stub bypasses bridge: empty FinalSummary must fail observably, never
		// leak Reply.Text or the deprecated #157 safe-notice shell.
		return Reply{Text: raw}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInbound("final-raw"))

	got := sentTexts(fa)
	for _, text := range got {
		if strings.Contains(text, "内部推理") || strings.Contains(text, "sk-secret") || strings.Contains(text, "提示词标签") {
			t.Fatalf("raw assistant final leaked: %v", got)
		}
		if strings.Contains(text, deprecatedSafeFinalNotice) {
			t.Fatalf("deprecated safe-notice fake completion must not be sent: %v", got)
		}
	}
	if countText(got, finalSummaryMissingNotice) != 1 {
		t.Fatalf("expected observable missing-summary failure notice, got %v", got)
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

// TestNaturalLanguageQQFinalContainsAnswerNotShellNotice is the cross-layer
// regression for ordinary QQ chat after #157: inbound NL → turn → FinalSummary
// construction → Sendable policy → fake QQ adapter outbound must carry a real
// answer, never only the Approving redirect shell.
func TestNaturalLanguageQQFinalContainsAnswerNotShellNotice(t *testing.T) {
	const userQ = "回的慢还不好"
	const assistantBody = "tool_call read_config\n内部推理：先自评再答\n抱歉回复慢了，也认同质量需要改进。我们会优先排查延迟与答复质量。"
	const wantAnswer = "抱歉回复慢了，也认同质量需要改进"

	fa := &fakeAdapter{}
	m, _ := policyManager(t, fa, nil)
	// Mimic ChannelBridge.Handle: FinalSummary is server-built, Text stays internal.
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		return Reply{
			Text:         assistantBody,
			FinalSummary: buildDeliverableFinalSummary(assistantBody),
		}, nil
	}

	rc := testRunningChannel(fa)
	rc.cfg.Type = "qq"
	m.dispatch(context.Background(), rc, testInboundText("nl-slow-bad", userQ))

	got := sentTexts(fa)
	if hasPrefixCount(got, ackProcessingPrefix) != 1 {
		t.Fatalf("expected processing ACK, got %v", got)
	}
	foundAnswer := false
	for _, text := range got {
		if strings.Contains(text, deprecatedSafeFinalNotice) || strings.Contains(text, "本回合已结束") {
			t.Fatalf("shell/fake-completion must not appear in QQ outbound: %v", got)
		}
		if strings.Contains(text, "tool_call") || strings.Contains(text, "内部推理") {
			t.Fatalf("tool/reasoning must not reach QQ outbound: %v", got)
		}
		if strings.Contains(text, wantAnswer) {
			foundAnswer = true
		}
	}
	if !foundAnswer {
		t.Fatalf("QQ outbound missing real answer %q; got %v", wantAnswer, got)
	}

	// Empty FinalSummary (noise-only body) → observable failure, not shell success.
	faFail := &fakeAdapter{}
	mFail, _ := policyManager(t, faFail, nil)
	mFail.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		noise := "tool_call x\nthinking: y"
		return Reply{Text: noise, FinalSummary: buildDeliverableFinalSummary(noise)}, nil
	}
	mFail.dispatch(context.Background(), testRunningChannel(faFail), testInboundText("nl-empty", userQ))
	failGot := sentTexts(faFail)
	if countText(failGot, finalSummaryMissingNotice) != 1 {
		t.Fatalf("expected missing-summary failure notice, got %v", failGot)
	}
	for _, text := range failGot {
		if strings.Contains(text, deprecatedSafeFinalNotice) {
			t.Fatalf("failure path must not send deprecated shell: %v", failGot)
		}
	}
	sawMissingReason := false
	faFail.mu.Lock()
	for _, out := range faFail.sent {
		if out.Envelope.Reason == "final_summary_missing" && out.Envelope.Kind == sendable.KindBlocked {
			sawMissingReason = true
		}
	}
	faFail.mu.Unlock()
	if !sawMissingReason {
		t.Fatalf("expected KindBlocked with reason=final_summary_missing; outbound=%v", failGot)
	}
}

// failingAdapter fails the first failures sends, then succeeds.
type failingAdapter struct {
	fakeAdapter
	failures int
	attempts int
}

func (f *failingAdapter) Send(ctx context.Context, out OutboundMessage) (SendResult, error) {
	f.mu.Lock()
	f.attempts++
	attempt := f.attempts
	f.mu.Unlock()
	if attempt <= f.failures {
		return SendResult{}, errors.New("transport down")
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
	if _, err := m.SendRunAcceptanceAck(context.Background(), ack); err != nil {
		t.Fatalf("first ack: %v", err)
	}
	// The second ACK is idempotently suppressed: a structured result, not an error.
	result, err := m.SendRunAcceptanceAck(context.Background(), ack)
	if err != nil || result.Sent || !result.Suppressed() {
		t.Fatalf("second ack should report idempotent suppression: result=%+v err=%v", result, err)
	}
	if result.Reason() != "run_acceptance_ack_already_sent" && result.Reason() != "already_sent" {
		t.Fatalf("second ack reason = %q want an already-sent suppression", result.Reason())
	}
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("run acceptance ACK sent %v want exactly once per run", got)
	}
	if !strings.HasPrefix(got[0], "【登录页｜已接单】") {
		t.Fatalf("ack text = %q want short title prefix", got[0])
	}

	ack.RunID = ""
	if _, err := m.SendRunAcceptanceAck(context.Background(), ack); err == nil {
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

func noticeInbound(messageID string) InboundMessage {
	in := testInbound(messageID)
	in.Text = ""
	in.Safety = &SafetyNotice{
		Text:   "附件超过 20 MiB 上限，已拒绝：big.zip。请压缩后重试。",
		Reason: "oversized_attachment", DedupeKey: messageID + ":oversize", Only: true,
	}
	return in
}

func TestSafetyNoticeUsesManagerEgressAndStaysIdempotent(t *testing.T) {
	fa := &fakeAdapter{}
	var audits []sendable.AuditEntry
	m, db := policyManager(t, fa, func(e sendable.AuditEntry) { audits = append(audits, e) })
	rc := testRunningChannel(fa)
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		t.Fatal("a notice-only inbound must not start a turn")
		return Reply{}, nil
	}

	m.dispatch(context.Background(), rc, noticeInbound("m-oversize"))
	// A gateway reconnect replays the same inbound with the same dedupe key.
	m.dispatch(context.Background(), rc, noticeInbound("m-oversize"))

	got := sentTexts(fa)
	if len(got) != 1 || !strings.Contains(got[0], "big.zip") {
		t.Fatalf("safety notice sends = %v want exactly one tip", got)
	}

	var receipts []models.SendableDeliveryReceipt
	if err := db.Find(&receipts).Error; err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("receipts = %+v want one shared dedupe receipt", receipts)
	}
	if receipts[0].DedupeKey != "m-oversize:oversize" {
		t.Fatalf("dedupe key = %q want the inbound-scoped oversize key", receipts[0].DedupeKey)
	}
	if receipts[0].Status != "sent" || receipts[0].Kind != string(sendable.KindSafetyNotice) {
		t.Fatalf("receipt = %+v want a sent safety_notice", receipts[0])
	}
	if receipts[0].RunID != "" {
		t.Fatalf("safety notice fabricated a run id: %+v", receipts[0])
	}

	var sent, suppressed int
	for _, entry := range audits {
		switch entry.Result {
		case "sent":
			sent++
			if entry.Reason != "oversized_attachment" {
				t.Fatalf("sent audit reason = %q", entry.Reason)
			}
		case "suppressed":
			suppressed++
			if entry.Reason != "already_sent" {
				t.Fatalf("suppressed audit reason = %q want already_sent", entry.Reason)
			}
		}
	}
	if sent != 1 || suppressed != 1 {
		t.Fatalf("audit results sent=%d suppressed=%d want 1/1 (%+v)", sent, suppressed, audits)
	}
}

func TestSafetyNoticeSendFailureIsRecorded(t *testing.T) {
	fa := &failingAdapter{failures: 99}
	var audits []sendable.AuditEntry
	m, db := policyManager(t, &fa.fakeAdapter, func(e sendable.AuditEntry) { audits = append(audits, e) })
	rc := testRunningChannel(fa)

	in := noticeInbound("m-fail")
	result := m.sendSafetyNotice(context.Background(), rc, in, *in.Safety)
	if result.Sent || !result.Failed() {
		t.Fatalf("failed notice = %+v want a delivery failure", result)
	}
	if fa.attempts != 3 {
		t.Fatalf("adapter attempts = %d want the bounded 3", fa.attempts)
	}
	var receipt models.SendableDeliveryReceipt
	if err := db.First(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "failed" {
		t.Fatalf("receipt = %+v want failed", receipt)
	}
	failures := 0
	for _, entry := range audits {
		if entry.Result == "failed" && entry.Reason == "transport_failed" {
			failures++
		}
	}
	if failures == 0 {
		t.Fatalf("no transport failure audited: %+v", audits)
	}
}

func TestDeliverSendableSuppressionIsNotAFailure(t *testing.T) {
	// Every normal policy outcome (dedupe, merge, rate limit, validation gate)
	// must come back as a structured result so callers do not retry it.
	cases := []struct {
		name       string
		prepare    func(t *testing.T, m *Manager)
		request    func() SendableRequest
		wantReason string
	}{
		{
			name: "already sent receipt",
			prepare: func(t *testing.T, m *Manager) {
				if _, err := m.DeliverSendable(context.Background(),
					progressRequest("r1", "user1", "已提交分支", "same")); err != nil {
					t.Fatal(err)
				}
			},
			request:    func() SendableRequest { return progressRequest("r1", "user1", "已提交分支", "same") },
			wantReason: "already_sent",
		},
		{
			name: "duplicate content under a new key",
			prepare: func(t *testing.T, m *Manager) {
				if _, err := m.DeliverSendable(context.Background(),
					progressRequest("r1", "user1", "已提交分支", "first")); err != nil {
					t.Fatal(err)
				}
			},
			request:    func() SendableRequest { return progressRequest("r1", "user1", "已提交分支", "second") },
			wantReason: "duplicate_content",
		},
		{
			name: "progress rate limited and merged",
			prepare: func(t *testing.T, m *Manager) {
				if _, err := m.DeliverSendable(context.Background(),
					progressRequest("r1", "user1", "已提交分支", "first")); err != nil {
					t.Fatal(err)
				}
			},
			request:    func() SendableRequest { return progressRequest("r1", "user1", "已开始跑测试", "second") },
			wantReason: "progress_rate_limited_merged",
		},
		{
			name: "progress without substance",
			request: func() SendableRequest {
				req := progressRequest("r1", "user1", "在忙", "")
				return req
			},
			wantReason: "non_substantive_progress",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fa := &fakeAdapter{}
			m, _ := bindingManager(t, fa)
			if c.prepare != nil {
				c.prepare(t, m)
			}
			result, err := m.DeliverSendable(context.Background(), c.request())
			if err != nil {
				t.Fatalf("suppression returned an error: %v (result=%+v)", err, result)
			}
			if result.Sent || !result.Suppressed() || result.Failed() {
				t.Fatalf("result = %+v want suppressed", result)
			}
			if result.Reason() != c.wantReason {
				t.Fatalf("reason = %q want %q", result.Reason(), c.wantReason)
			}
		})
	}
}

func TestDeliverSendableRealFailuresReturnErrors(t *testing.T) {
	t.Run("no target", func(t *testing.T) {
		m, _ := policyManager(t, &fakeAdapter{}, nil)
		result, err := m.DeliverSendable(context.Background(), progressRequest("r1", "user1", "已提交分支", "s1"))
		if !errors.Is(err, ErrNoSendableTarget) || result.Sent {
			t.Fatalf("no target = %+v err=%v want ErrNoSendableTarget", result, err)
		}
	})

	t.Run("empty text", func(t *testing.T) {
		m, _ := policyManager(t, &fakeAdapter{}, nil)
		req := progressRequest("r1", "user1", "", "s1")
		if _, err := m.DeliverSendable(context.Background(), req); err == nil {
			t.Fatal("empty text must be rejected")
		}
	})

	t.Run("transport failure", func(t *testing.T) {
		fa := &failingAdapter{failures: 99}
		m, _ := bindingManager(t, &fa.fakeAdapter)
		m.mu.Lock()
		for _, rc := range m.running {
			rc.adapter = fa
		}
		m.mu.Unlock()

		// The context ends during the retry backoff, so the delivery stops right
		// after a transport error: the message did not reach the channel.
		m.SetRetryBackoff(func(int) time.Duration { return time.Hour })
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		result, err := m.DeliverSendable(ctx, progressRequest("r1", "user1", "已提交分支", "s1"))
		if !errors.Is(err, ErrDeliveryFailed) || !result.Failed() || result.Suppressed() {
			t.Fatalf("transport failure = %+v err=%v want ErrDeliveryFailed", result, err)
		}
		if result.Reason() != "transport_failed" {
			t.Fatalf("reason = %q want transport_failed", result.Reason())
		}
	})

	t.Run("retries exhausted", func(t *testing.T) {
		fa := &failingAdapter{failures: 99}
		m, _ := bindingManager(t, &fa.fakeAdapter)
		m.mu.Lock()
		for _, rc := range m.running {
			rc.adapter = fa
		}
		m.mu.Unlock()

		req := progressRequest("r1", "user1", "已提交分支", "s1")
		result, err := m.DeliverSendable(context.Background(), req)
		if !errors.Is(err, ErrDeliveryFailed) || !result.Failed() {
			t.Fatalf("exhausted attempts = %+v err=%v want ErrDeliveryFailed", result, err)
		}
		if result.Reason() != "retry_exhausted" {
			t.Fatalf("reason = %q want retry_exhausted", result.Reason())
		}
		if fa.attempts != 3 {
			t.Fatalf("adapter attempts = %d want the bounded 3", fa.attempts)
		}
		// A later attempt on the same key stays a failure, never a suppression
		// the agent is told to accept.
		result, err = m.DeliverSendable(context.Background(), req)
		if !errors.Is(err, ErrDeliveryFailed) || result.Reason() != "retry_exhausted" {
			t.Fatalf("second call = %+v err=%v want retry_exhausted failure", result, err)
		}
	})

	t.Run("policy evaluation failed closed", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, _ := bindingManager(t, fa)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // the policy store cannot be consulted → fail closed
		result, err := m.DeliverSendable(ctx, progressRequest("r1", "user1", "已提交分支", "s1"))
		if !errors.Is(err, ErrDeliveryFailed) || result.Reason() != "policy_error" {
			t.Fatalf("fail-closed policy = %+v err=%v want policy_error failure", result, err)
		}
		if got := sentTexts(fa); len(got) != 0 {
			t.Fatalf("fail-closed policy still sent %v", got)
		}
	})

	t.Run("no adapter", func(t *testing.T) {
		m := NewManager(nil, nil, nil)
		result := m.sendOutboundResult(context.Background(), &runningChannel{
			cfg: models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: "proj"},
		}, OutboundMessage{Scene: SceneC2C, ConversationID: "user1", Text: "x"})
		if !result.Failed() || result.Reason() != "no_adapter" {
			t.Fatalf("missing adapter = %+v want a no_adapter failure", result)
		}
	})
}

// bindingManager wires a DB-backed policy plus task context and a started
// channel for project "proj" bound to conversation "user1".
func bindingManager(t *testing.T, fa *fakeAdapter) (*Manager, *services.TaskContextService) {
	t.Helper()
	m, db := policyManager(t, fa, nil)
	svc := services.NewTaskContextService(db)
	m.SetTaskContextService(svc)
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: models.ChannelTypeQQ, ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "c2c:user1",
	}})
	t.Cleanup(m.StopAll)
	return m, svc
}

func ensureTestIdentity(t *testing.T, svc *services.TaskContextService, runID, projectID, userID, title string) {
	t.Helper()
	if _, err := svc.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: runID, ProjectID: projectID, UserID: userID,
		ShortTitle: title, OriginalRequirement: "实现" + title, Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
}

func progressRequest(runID, userID, text, stage string) SendableRequest {
	return SendableRequest{
		ProjectID: "proj", Scene: SceneC2C, ConversationID: "user1", UserID: userID,
		RunID: runID, Kind: sendable.KindProgress, Reason: "pm_notify_progress",
		DedupeKey: "test:" + runID + ":" + stage,
		Progress:  sendable.ProgressFields{Stage: stage},
		Text:      text,
	}
}

func bindings(t *testing.T, svc *services.TaskContextService) []models.MessageBinding {
	t.Helper()
	var out []models.MessageBinding
	if err := svc.DB().Find(&out).Error; err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOutboundSuccessBindsExternalMessageIDAndReplyWins(t *testing.T) {
	fa := &fakeAdapter{messageIDs: []string{"qq-msg-1"}}
	m, svc := bindingManager(t, fa)
	qqUser := "user1"
	userID := services.SyntheticQQUserID(qqUser)
	ensureTestIdentity(t, svc, "r1", "proj", userID, "支付登录页")
	ensureTestIdentity(t, svc, "r2", "proj", userID, "用户登录页")

	result, err := m.ReportRunProgress(context.Background(), progressRequest("r1", qqUser, "已提交分支", "branch_pushed"))
	if err != nil || !result.Sent {
		t.Fatalf("progress delivery = %+v err=%v", result, err)
	}
	if result.ExternalMessageID != "qq-msg-1" {
		t.Fatalf("external message id = %q want the id the channel reported", result.ExternalMessageID)
	}

	all := bindings(t, svc)
	if len(all) != 1 {
		t.Fatalf("message bindings = %+v want exactly one", all)
	}
	if all[0].MessageID != "qq-msg-1" || all[0].RunID != "r1" ||
		all[0].UserID != userID || all[0].Channel != models.ChannelTypeQQ ||
		all[0].ConversationID != "user1" || all[0].ProjectID != "proj" {
		t.Fatalf("binding = %+v want project/user/channel/conversation scoped r1 binding", all[0])
	}

	// A reply to that message wins over a query that matches the other task.
	scope := services.TaskScope{
		ProjectID: "proj", UserID: userID,
		Channel: models.ChannelTypeQQ, ConversationID: "user1",
	}
	candidates, err := svc.Search(scope, "用户登录页")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ResolveTask(services.ResolveTaskInput{
		Scope: scope, Query: "用户登录页", ReplyMessageID: "qq-msg-1", Candidates: candidates,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Identity == nil || res.Identity.RunID != "r1" || res.Reason != "reply_binding" {
		t.Fatalf("reply reference = %+v want the bound r1 task", res)
	}

	// Another user in the same conversation must not resolve through it.
	otherScope := scope
	otherScope.UserID = services.SyntheticQQUserID("user2")
	res, err = svc.ResolveTask(services.ResolveTaskInput{Scope: otherScope, ReplyMessageID: "qq-msg-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Identity != nil {
		t.Fatalf("cross-user reply reference resolved to %+v", res.Identity)
	}
	// Same for another project.
	otherProject := scope
	otherProject.ProjectID = "other-proj"
	res, err = svc.ResolveTask(services.ResolveTaskInput{Scope: otherProject, ReplyMessageID: "qq-msg-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Identity != nil {
		t.Fatalf("cross-project reply reference resolved to %+v", res.Identity)
	}
}

func TestOutboundBindingSkippedWithoutRealIDRunOrOwnership(t *testing.T) {
	qqUser := "user1"
	userID := services.SyntheticQQUserID(qqUser)

	t.Run("channel reported no message id", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, svc := bindingManager(t, fa)
		ensureTestIdentity(t, svc, "r1", "proj", userID, "支付登录页")
		result, err := m.ReportRunProgress(context.Background(), progressRequest("r1", qqUser, "已提交分支", "s1"))
		if err != nil || !result.Sent {
			t.Fatalf("delivery = %+v err=%v", result, err)
		}
		if all := bindings(t, svc); len(all) != 0 {
			t.Fatalf("bindings without a channel message id = %+v", all)
		}
	})

	t.Run("send failed", func(t *testing.T) {
		fa := &failingAdapter{failures: 99}
		fa.messageIDs = []string{"qq-msg-fail"}
		m, svc := bindingManager(t, &fa.fakeAdapter)
		m.mu.Lock()
		for _, rc := range m.running {
			rc.adapter = fa
		}
		m.mu.Unlock()
		ensureTestIdentity(t, svc, "r1", "proj", userID, "支付登录页")
		result, err := m.ReportRunProgress(context.Background(), progressRequest("r1", qqUser, "已提交分支", "s1"))
		if !errors.Is(err, ErrDeliveryFailed) || result.Sent {
			t.Fatalf("failed delivery = %+v err=%v want ErrDeliveryFailed", result, err)
		}
		if all := bindings(t, svc); len(all) != 0 {
			t.Fatalf("bindings after transport failure = %+v", all)
		}
	})

	t.Run("turn traffic has no run id", func(t *testing.T) {
		fa := &fakeAdapter{messageIDs: []string{"qq-msg-turn", "qq-msg-final"}}
		m, svc := bindingManager(t, fa)
		m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
			return Reply{FinalSummary: "done"}, nil
		}
		ensureTestIdentity(t, svc, "r1", "proj", userID, "支付登录页")
		m.dispatch(context.Background(), testRunningChannel(fa), testInbound("turn-bind"))
		if all := bindings(t, svc); len(all) != 0 {
			t.Fatalf("turn deliveries must not bind: %+v", all)
		}
	})

	t.Run("run belongs to another user", func(t *testing.T) {
		fa := &fakeAdapter{messageIDs: []string{"qq-msg-2"}}
		m, svc := bindingManager(t, fa)
		ensureTestIdentity(t, svc, "r1", "proj", services.SyntheticQQUserID("owner"), "支付登录页")
		result, err := m.ReportRunProgress(context.Background(), progressRequest("r1", "intruder", "已提交分支", "s1"))
		if err != nil || !result.Sent {
			t.Fatalf("delivery = %+v err=%v", result, err)
		}
		if all := bindings(t, svc); len(all) != 0 {
			t.Fatalf("binding crossed task ownership: %+v", all)
		}
	})

	t.Run("run has no task identity", func(t *testing.T) {
		fa := &fakeAdapter{messageIDs: []string{"qq-msg-3"}}
		m, svc := bindingManager(t, fa)
		result, err := m.ReportRunProgress(context.Background(), progressRequest("r-unknown", qqUser, "已提交分支", "s1"))
		if err != nil || !result.Sent {
			t.Fatalf("delivery = %+v err=%v", result, err)
		}
		if all := bindings(t, svc); len(all) != 0 {
			t.Fatalf("binding invented an identity: %+v", all)
		}
	})
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
	// Another project's task must stay invisible even with a similar title.
	if _, err := svc.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "r3", ProjectID: "other-proj", UserID: userID,
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
