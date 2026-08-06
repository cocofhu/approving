package services

import (
	"errors"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func grantFixture(t *testing.T) (*TaskContextService, *time.Time) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.RiskConfirmationTicket{}, &models.ChannelActionGuard{}, &models.ConversationFocus{},
	); err != nil {
		t.Fatal(err)
	}
	clock := time.Now().UTC()
	svc := NewTaskContextService(db)
	svc.now = func() time.Time { return clock }
	return svc, &clock
}

func confirmedTicket(t *testing.T, svc *TaskContextService, runID, kind, threadID string) models.RiskConfirmationTicket {
	t.Helper()
	ticket, err := svc.CreateRiskConfirmationWithKind("p1", "qq:c2c:u1", runID, kind, kind+" "+runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConsumeRiskConfirmation("p1", "qq:c2c:u1", runID, ticket.Action, "确认"); err != nil {
		t.Fatal(err)
	}
	if threadID != "" {
		if err := svc.BindActionGrant(ticket.ID, threadID); err != nil {
			t.Fatal(err)
		}
	}
	return ticket
}

func TestChannelThreadGuardScopesEnforcement(t *testing.T) {
	svc, _ := grantFixture(t)
	if svc.IsGuardedThread("p1", "thr-web") {
		t.Fatal("an unknown (web/API) thread must not be guarded")
	}
	if err := svc.GuardChannelThread("p1", "thr-qq", "qq", "qq:c2c:u1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.GuardChannelThread("p1", "thr-qq", "qq", "qq:c2c:u1"); err != nil {
		t.Fatalf("guard upsert is not idempotent: %v", err)
	}
	if !svc.IsGuardedThread("p1", "thr-qq") {
		t.Fatal("channel thread must be guarded")
	}
	if svc.IsGuardedThread("p2", "thr-qq") {
		t.Fatal("guard leaked across projects")
	}
}

func TestActionGrantIsSingleUseAndExactlyScoped(t *testing.T) {
	svc, clock := grantFixture(t)

	// No ticket at all.
	if err := svc.ConsumeActionGrant("p1", "thr-qq", "run-1", "cancel"); !errors.Is(err, ErrActionNotAuthorized) {
		t.Fatalf("missing ticket was authorized: %v", err)
	}

	// A pending (unconfirmed) ticket is not a grant.
	pending, err := svc.CreateRiskConfirmationWithKind("p1", "qq:c2c:u1", "run-1", "cancel", "cancel run-1 (unconfirmed)")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.BindActionGrant(pending.ID, "thr-qq"); !errors.Is(err, ErrActionNotAuthorized) {
		t.Fatalf("pending ticket was bindable: %v", err)
	}
	if err := svc.ConsumeActionGrant("p1", "thr-qq", "run-1", "cancel"); !errors.Is(err, ErrActionNotAuthorized) {
		t.Fatalf("pending ticket authorized execution: %v", err)
	}

	ticket := confirmedTicket(t, svc, "run-1", "cancel", "thr-qq")
	for _, bad := range []struct{ project, thread, target, kind string }{
		{"p1", "thr-qq", "run-2", "cancel"},
		{"p1", "thr-qq", "run-1", "delete"},
		{"p1", "thr-other", "run-1", "cancel"},
		{"p2", "thr-qq", "run-1", "cancel"},
	} {
		if err := svc.ConsumeActionGrant(bad.project, bad.thread, bad.target, bad.kind); !errors.Is(err, ErrActionNotAuthorized) {
			t.Fatalf("grant accepted %+v: %v", bad, err)
		}
	}
	if err := svc.ConsumeActionGrant("p1", "thr-qq", "run-1", "cancel"); err != nil {
		t.Fatalf("exact confirmed grant denied: %v", err)
	}
	if err := svc.ConsumeActionGrant("p1", "thr-qq", "run-1", "cancel"); !errors.Is(err, ErrActionNotAuthorized) {
		t.Fatal("grant was spendable twice")
	}
	var row models.RiskConfirmationTicket
	if err := svc.db.First(&row, "id = ?", ticket.ID).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != models.RiskTicketExecuted || row.ExecutedAt == nil {
		t.Fatalf("ticket after execution = %#v", row)
	}

	// A grant the authorizing turn never spent is retired.
	unused := confirmedTicket(t, svc, "run-3", "approve", "thr-qq")
	if err := svc.ReleaseActionGrant(unused.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConsumeActionGrant("p1", "thr-qq", "run-3", "approve"); !errors.Is(err, ErrActionNotAuthorized) {
		t.Fatal("released grant was still spendable")
	}

	// Grants expire on their own even if never released.
	confirmedTicket(t, svc, "run-4", "reject", "thr-qq")
	*clock = clock.Add(RiskGrantTTL + time.Second)
	if err := svc.ConsumeActionGrant("p1", "thr-qq", "run-4", "reject"); !errors.Is(err, ErrActionNotAuthorized) {
		t.Fatal("expired grant was spendable")
	}
}

func TestConversationFocusTTLIsExactAndLanguageDoesNotRenew(t *testing.T) {
	svc, clock := grantFixture(t)
	start := *clock
	if err := svc.TouchConversationFocus("p1", "qq", "conv", "u1", "run-1"); err != nil {
		t.Fatal(err)
	}

	// Passive language tracking one second before expiry must not extend it.
	*clock = start.Add(ConversationFocusTTL - time.Second)
	if err := svc.RememberConversationLanguage("p1", "qq", "conv", "u1", "en"); err != nil {
		t.Fatal(err)
	}
	if focus, err := svc.GetConversationFocus("p1", "qq", "conv", "u1"); err != nil || focus.RunID != "run-1" {
		t.Fatalf("focus expired early: %#v %v", focus, err)
	}
	*clock = start.Add(ConversationFocusTTL)
	if _, err := svc.GetConversationFocus("p1", "qq", "conv", "u1"); err == nil {
		t.Fatal("focus outlived its exact 30m TTL after a language update")
	}
	if got := svc.ConversationLanguage("p1", "qq", "conv", "u1"); got != "en" {
		t.Fatalf("remembered language = %q", got)
	}

	// Only a successful bind/select/continue renews.
	if err := svc.TouchConversationFocus("p1", "qq", "conv", "u1", "run-1"); err != nil {
		t.Fatal(err)
	}
	*clock = start.Add(ConversationFocusTTL).Add(ConversationFocusTTL - time.Second)
	if focus, err := svc.GetConversationFocus("p1", "qq", "conv", "u1"); err != nil || focus.RunID != "run-1" {
		t.Fatalf("renewed focus = %#v %v", focus, err)
	}

	// A language-only row never creates an active focus.
	if err := svc.RememberConversationLanguage("p1", "qq", "fresh", "u2", "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetConversationFocus("p1", "qq", "fresh", "u2"); err == nil {
		t.Fatal("language memory created a focus")
	}
	if got := svc.ConversationLanguage("p1", "qq", "fresh", "u2"); got != "zh-CN" {
		t.Fatalf("fresh language = %q", got)
	}
}
