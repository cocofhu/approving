package channels

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeliveryFloodSuppressesInternalAndBoundsExternal(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	defer m.StopAll()
	rc := &runningChannel{adapter: fa}
	out := OutboundMessage{Scene: SceneC2C, ConversationID: "conv"}

	for i := 0; i < 128; i++ {
		m.AppendInternal(InternalEnvelope(fmt.Sprintf("raw_tool_%d", i)))
	}
	send := func(run string, reason DeliveryReason, typ, dedupe string) error {
		priority := PriorityOrdinary
		if reason == ReasonRunAcceptanceACK || reason == ReasonBlocked || reason == ReasonActionRequired || reason == ReasonFinal {
			priority = PriorityImmediate
		}
		env := Envelope{
			Channels: []string{"qq"}, Priority: priority, Reason: reason, Type: typ, DedupeKey: dedupe,
			Context: RunTaskContext{RunID: run, ProjectID: "p", UserID: "u"},
		}
		return m.AppendSendable(context.Background(), rc, out, env)
	}
	if err := send("r1", ReasonRunAcceptanceACK, "run_acceptance", "ack"); err != nil {
		t.Fatal(err)
	}
	if err := send("r1", ReasonProgress, DeliveryTypeStage, "p1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := send("r1", ReasonProgress, DeliveryTypeStage, fmt.Sprintf("p-%d", i)); !errors.Is(err, ErrDeliverySuppressed) {
			t.Fatalf("ordinary progress %d should be merged/suppressed: %v", i, err)
		}
	}
	if err := send("r1", ReasonBlocked, DeliveryTypeBlocked, "blocked"); err != nil {
		t.Fatal(err)
	}
	if err := send("r1", ReasonFinal, DeliveryTypeStructuredSummary, "final"); err != nil {
		t.Fatal(err)
	}

	fa.mu.Lock()
	defer fa.mu.Unlock()
	if got := len(fa.sent); got > 5 || got != 4 {
		t.Fatalf("128 internal + lifecycle flood sent %d external messages, want 4 (<=5)", got)
	}
}

func TestProgressRateLimitNeverCrossesRuns(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	rc := &runningChannel{adapter: fa}
	for _, run := range []string{"run-a", "run-b"} {
		err := m.AppendSendable(context.Background(), rc, OutboundMessage{
			Scene: SceneGroup, ConversationID: "same", Text: run,
		}, Envelope{
			Channels: []string{"qq"}, Reason: ReasonProgress, Type: DeliveryTypeStage,
			Context: RunTaskContext{RunID: run}, DedupeKey: run,
		})
		if err != nil {
			t.Fatalf("%s: %v", run, err)
		}
	}
	if len(fa.sent) != 2 {
		t.Fatalf("cross-run progress merged: got %d sends", len(fa.sent))
	}
}

func TestMarkerClassificationDoesNotAuthorizeEgress(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	rc := &runningChannel{adapter: fa}
	env := Envelope{
		Delivery: DeliverySendable, Channels: []string{"qq"}, Reason: ReasonProgress,
		Type: DeliveryTypeStage, Context: RunTaskContext{RunID: "r"},
	}
	err := m.sendEnvelope(context.Background(), rc, OutboundMessage{
		Scene: SceneC2C, ConversationID: "c", Text: "【进度】完成",
	}, env)
	if !errors.Is(err, ErrDeliverySuppressed) || len(fa.sent) != 0 {
		t.Fatalf("marker/raw envelope authorized egress: err=%v sent=%d", err, len(fa.sent))
	}
}

func TestStructuredFinalAndLanguageFallback(t *testing.T) {
	if _, ok := ExtractStructuredFinalSummary("raw assistant terminal output"); ok {
		t.Fatal("raw output must not be promoted")
	}
	if got, ok := ExtractStructuredFinalSummary("[final] safe result"); !ok || got != "safe result" {
		t.Fatalf("explicit final = %q, %v", got, ok)
	}
	if got := DetectIMLanguage("", "hello there"); got != "en" {
		t.Fatalf("language = %q", got)
	}
	msg := FormatTaskMessage("支付修复", "blocked", "需要权限", "Permission required", "en")
	if !strings.HasPrefix(msg, "【支付修复｜blocked】Permission") {
		t.Fatalf("fallback prefix/copy = %q", msg)
	}
}

type retryAdapter struct{ attempts int }

func (a *retryAdapter) Type() string                                { return "qq" }
func (a *retryAdapter) Start(context.Context, InboundHandler) error { return nil }
func (a *retryAdapter) Stop() error                                 { return nil }
func (a *retryAdapter) Send(context.Context, OutboundMessage) error {
	a.attempts++
	if a.attempts == 1 {
		return errors.New("temporary")
	}
	return nil
}

type delayedRecoveryAdapter struct{ attempts int }

func (a *delayedRecoveryAdapter) Type() string                                { return "qq" }
func (a *delayedRecoveryAdapter) Start(context.Context, InboundHandler) error { return nil }
func (a *delayedRecoveryAdapter) Stop() error                                 { return nil }
func (a *delayedRecoveryAdapter) Send(context.Context, OutboundMessage) error {
	a.attempts++
	if a.attempts <= maxDeliveryAttempts {
		return errors.New("still down")
	}
	return nil
}

func TestDeliveryReceiptRetrySentIdempotencyAndContentFreeAudit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DeliveryReceipt{}, &models.ProjectAuditEvent{}, &models.MessageBinding{}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil, nil, nil)
	m.SetDeliveryPersistence(db, services.NewProjectAuditService(db))
	m.SetTaskContext(services.NewTaskContextService(db))
	a := &retryAdapter{}
	rc := &runningChannel{cfg: models.ChannelConfig{ProjectID: "p", Type: "qq"}, adapter: a}
	env := Envelope{
		Channels: []string{"qq"}, Priority: PriorityImmediate, Reason: ReasonBlocked, Type: DeliveryTypeBlocked,
		Context: RunTaskContext{ProjectID: "p", RunID: "r", UserID: "u"}, DedupeKey: "same-key",
	}
	out := OutboundMessage{Scene: SceneC2C, ConversationID: "c", MessageID: "out-1", Text: "secret internal content"}
	if err := m.AppendSendable(context.Background(), rc, out, env); err != nil {
		t.Fatalf("bounded retry: %v", err)
	}
	if err := m.AppendSendable(context.Background(), rc, out, env); !errors.Is(err, ErrDeliverySuppressed) {
		t.Fatalf("sent receipt should suppress duplicate: %v", err)
	}
	if a.attempts != 2 {
		t.Fatalf("adapter attempts = %d", a.attempts)
	}
	var receipt models.DeliveryReceipt
	if err := db.First(&receipt, "dedupe_key = ?", "same-key").Error; err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "sent" || receipt.Attempts != 2 {
		t.Fatalf("receipt = %#v", receipt)
	}
	var binding models.MessageBinding
	if err := db.First(&binding, "message_id = ?", "out-1").Error; err != nil {
		t.Fatal(err)
	}
	if binding.Direction != "outbound" || binding.RunID != "r" {
		t.Fatalf("outbound binding = %#v", binding)
	}
	var audits []models.ProjectAuditEvent
	if err := db.Find(&audits).Error; err != nil {
		t.Fatal(err)
	}
	for _, audit := range audits {
		if strings.Contains(audit.Summary, "secret") || strings.Contains(fmt.Sprint(audit.Payload), "secret internal") {
			t.Fatalf("audit leaked content: %#v", audit)
		}
	}
}

func TestRunAcceptanceCanRetryAfterExhaustedFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DeliveryReceipt{}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil, nil, nil)
	m.SetDeliveryPersistence(db, nil)
	a := &delayedRecoveryAdapter{}
	rc := &runningChannel{cfg: models.ChannelConfig{ProjectID: "p", Type: "qq"}, adapter: a}
	env := Envelope{
		Channels: []string{"qq"}, Priority: PriorityImmediate,
		Reason: ReasonRunAcceptanceACK, Type: "run_acceptance", DedupeKey: "ack-retry",
		Context: RunTaskContext{ProjectID: "p", RunID: "r", UserID: "u"},
	}
	out := OutboundMessage{Scene: SceneC2C, ConversationID: "c", Text: "accepted"}
	if err := m.AppendSendable(context.Background(), rc, out, env); err == nil {
		t.Fatal("first bounded cycle should exhaust")
	}
	if a.attempts != maxDeliveryAttempts {
		t.Fatalf("first cycle attempts = %d", a.attempts)
	}
	if err := m.AppendSendable(context.Background(), rc, out, env); err != nil {
		t.Fatalf("later ACK retry was permanently suppressed: %v", err)
	}
	if a.attempts != maxDeliveryAttempts+1 {
		t.Fatalf("recovery attempts = %d", a.attempts)
	}
	if err := m.AppendSendable(context.Background(), rc, out, env); !errors.Is(err, ErrDeliverySuppressed) {
		t.Fatalf("sent ACK was not idempotent: %v", err)
	}

	memoryOnly := NewManager(nil, nil, nil)
	a2 := &delayedRecoveryAdapter{}
	rc2 := &runningChannel{cfg: rc.cfg, adapter: a2}
	env.DedupeKey = "ack-memory"
	if err := memoryOnly.AppendSendable(context.Background(), rc2, out, env); err == nil {
		t.Fatal("memory-only first cycle should exhaust")
	}
	if err := memoryOnly.AppendSendable(context.Background(), rc2, out, env); err != nil {
		t.Fatalf("memory-only later retry suppressed: %v", err)
	}
	if err := memoryOnly.AppendSendable(context.Background(), rc2, out, env); !errors.Is(err, ErrDeliverySuppressed) {
		t.Fatalf("memory-only sent ACK was not idempotent: %v", err)
	}
}
