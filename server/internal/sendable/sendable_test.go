package sendable

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testPolicy(t *testing.T, audit AuditFunc) (*Policy, *gorm.DB, *time.Time) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.SendableDeliveryReceipt{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	p := NewPolicy(db, audit)
	p.SetClock(func() time.Time { return now })
	return p, db, &now
}

func external(run, kind, key string) DeliveryEnvelope {
	e := DeliveryEnvelope{
		RunID: run, ProjectID: "p", ConversationID: "c", UserID: "u",
		Kind: Kind(kind), DedupeKey: key, Reason: "test",
	}
	return AppendSendable(e, ChannelQQ)
}

func turnScoped(scope, kind, key string) DeliveryEnvelope {
	e := DeliveryEnvelope{
		TaskContext: scope, ProjectID: "p", ConversationID: "c", UserID: "u",
		Kind: Kind(kind), DedupeKey: key, Reason: "test",
	}
	return AppendSendable(e, ChannelQQ)
}

func sendAndMark(t *testing.T, p *Policy, e DeliveryEnvelope, body string) bool {
	t.Helper()
	d, err := p.Evaluate(context.Background(), e, ChannelQQ, body)
	if err != nil {
		t.Fatal(err)
	}
	if d.Send {
		if err := p.MarkSent(context.Background(), d, e, ChannelQQ); err != nil {
			t.Fatal(err)
		}
	}
	return d.Send
}

func TestPolicyBoundsBackgroundEventsAndKeepsCritical(t *testing.T) {
	p, _, _ := testPolicy(t, nil)
	sent := 0
	if sendAndMark(t, p, external("r1", string(KindRunAcceptanceAck), "ack"), "received") {
		sent++
	}
	for i := 0; i < 128; i++ {
		e := external("r1", string(KindProgress), "")
		e.Progress.Stage = "building"
		if sendAndMark(t, p, e, "building event "+string(rune(i))) {
			sent++
		}
	}
	blocked := external("r1", string(KindBlocked), "blocked")
	blocked.Progress.Blocked = true
	if sendAndMark(t, p, blocked, "blocked by permission") {
		sent++
	}
	final := external("r1", string(KindFinal), "final")
	final.Structured = true
	if sendAndMark(t, p, final, "safe structured summary") {
		sent++
	}
	if sent > 5 || sent != 4 {
		t.Fatalf("sent=%d want ACK + one progress + blocked + final", sent)
	}
}

// Several Runs each stay inside their own progress budget and still bury the
// conversation between them, so the conversation has a budget of its own. What
// must never be dropped is something the user has to act on.
func TestPolicyBudgetsProgressPerConversationNotOnlyPerRun(t *testing.T) {
	p, _, _ := testPolicy(t, nil)
	sent := 0
	for i := 0; i < 8; i++ {
		e := external("run-"+string(rune('a'+i)), string(KindProgress), "")
		e.Progress.Stage = "stage-" + string(rune('a'+i))
		if sendAndMark(t, p, e, "parallel run update "+string(rune('a'+i))) {
			sent++
		}
	}
	if sent > conversationProgressQuota {
		t.Fatalf("%d parallel progress messages reached one conversation, budget is %d",
			sent, conversationProgressQuota)
	}
	if sent == 0 {
		t.Fatal("the conversation budget silenced progress entirely")
	}

	blocked := external("run-z", string(KindBlocked), "needs-you")
	blocked.Priority = PriorityCritical
	blocked.Progress.Blocked = true
	if !sendAndMark(t, p, blocked, "CI is red, need a decision") {
		t.Fatal("a decision the user has to make was dropped by the progress budget")
	}
}

func TestPolicyRunIsolationDefaultsAndRawFinal(t *testing.T) {
	p, _, _ := testPolicy(t, nil)
	p1 := external("r1", string(KindProgress), "")
	p1.Progress.Stage = "build"
	if !sendAndMark(t, p, p1, "same") {
		t.Fatal("first run progress suppressed")
	}
	p2 := external("r2", string(KindProgress), "")
	p2.Progress.Stage = "build"
	if !sendAndMark(t, p, p2, "same") {
		t.Fatal("progress must not merge across runs")
	}
	for _, e := range []DeliveryEnvelope{
		{},
		turnScoped("t", string(KindUnknown), ""),
		turnScoped("t", string(KindAgentRaw), ""),
		turnScoped("t", string(KindTool), ""),
		turnScoped("t", string(KindReasoning), ""),
		turnScoped("t", string(KindFinal), ""),
		// No Run and no task context: there is no legitimate delivery bucket.
		AppendSendable(DeliveryEnvelope{Kind: KindProgress, ProjectID: "p"}, ChannelQQ),
		// Run acceptance ACK without a real Run id must never be sendable.
		turnScoped("turn:abc", string(KindRunAcceptanceAck), ""),
	} {
		d, err := p.Evaluate(context.Background(), e, ChannelQQ, "secret raw body")
		if err != nil {
			t.Fatal(err)
		}
		if d.Send {
			t.Fatalf("unsafe envelope was sendable: %+v", e)
		}
	}
}

func TestTurnAckIsNotRunAcceptanceAck(t *testing.T) {
	p, _, _ := testPolicy(t, nil)
	// Two turns in one conversation each get their own processing ACK.
	if !sendAndMark(t, p, turnScoped("turn:m1", string(KindTurnProcessingAck), "m1:ack"), "received m1") {
		t.Fatal("first turn ACK suppressed")
	}
	if !sendAndMark(t, p, turnScoped("turn:m2", string(KindTurnProcessingAck), "m2:ack"), "received m2") {
		t.Fatal("second turn ACK wrongly treated as a run acceptance ACK")
	}
	// The run acceptance ACK is once per run × conversation × channel.
	if !sendAndMark(t, p, external("run-1", string(KindRunAcceptanceAck), "run-ack-1"), "accepted run-1") {
		t.Fatal("first run acceptance ACK suppressed")
	}
	if sendAndMark(t, p, external("run-1", string(KindRunAcceptanceAck), "run-ack-1-again"), "accepted run-1 again") {
		t.Fatal("run acceptance ACK must be delivered at most once per run")
	}
	if !sendAndMark(t, p, external("run-2", string(KindRunAcceptanceAck), "run-ack-2"), "accepted run-2") {
		t.Fatal("a different run must still get its acceptance ACK")
	}
}

func TestPolicyRetainsLatestProgressStageWithinWindow(t *testing.T) {
	p, _, now := testPolicy(t, nil)
	first := external("r1", string(KindProgress), "p-a")
	first.Progress.Stage = "A"
	if !sendAndMark(t, p, first, "stage A") {
		t.Fatal("first stage suppressed")
	}
	*now = now.Add(30 * time.Second)
	same := external("r1", string(KindProgress), "p-a2")
	same.Progress.Stage = "A"
	if sendAndMark(t, p, same, "stage A again") {
		t.Fatal("same stage within window must stay rate-limited")
	}
	newer := external("r1", string(KindProgress), "p-b")
	newer.Progress.Stage = "B"
	if sendAndMark(t, p, newer, "stage B") {
		t.Fatal("changed stage within window must be merged, not delivered immediately")
	}
	*now = now.Add(31 * time.Second)
	latest := external("r1", string(KindProgress), "p-c")
	latest.Progress.Stage = "C"
	if !sendAndMark(t, p, latest, "stage C") {
		t.Fatal("first latest snapshot after the window must be delivered")
	}

	blocked := external("r1", string(KindBlocked), "blocked-now")
	blocked.Progress.Blocked = true
	if !sendAndMark(t, p, blocked, "blocked now") {
		t.Fatal("blocked must bypass the ordinary progress window")
	}
}

func TestRetryClaimsBoundedAttempts(t *testing.T) {
	var audits []AuditEntry
	p, _, _ := testPolicy(t, func(entry AuditEntry) { audits = append(audits, entry) })
	e := external("r", string(KindFinal), "retry-key")
	e.Structured = true

	d, err := p.Evaluate(context.Background(), e, ChannelQQ, "summary")
	if err != nil || !d.Send || d.Attempt != 1 {
		t.Fatalf("initial decision=%+v err=%v", d, err)
	}
	attempts := []int{d.Attempt}
	for {
		if err := p.MarkFailed(context.Background(), d, e, ChannelQQ, errors.New("network")); err != nil {
			t.Fatal(err)
		}
		next, err := p.Retry(context.Background(), d, e, ChannelQQ)
		if err != nil {
			t.Fatal(err)
		}
		if !next.Send {
			if next.Reason != "retry_exhausted" {
				t.Fatalf("final retry reason=%q want retry_exhausted", next.Reason)
			}
			break
		}
		d = next
		attempts = append(attempts, d.Attempt)
	}
	if len(attempts) != p.MaxAttempts() {
		t.Fatalf("attempts=%v want %d bounded attempts", attempts, p.MaxAttempts())
	}
	if attempts[0] != 1 || attempts[len(attempts)-1] != p.MaxAttempts() {
		t.Fatalf("attempt numbering=%v", attempts)
	}
	if len(audits) == 0 {
		t.Fatal("retry path produced no audit entries")
	}
}

func TestRetryStopsOnceSent(t *testing.T) {
	p, _, _ := testPolicy(t, nil)
	e := external("r", string(KindFinal), "sent-key")
	e.Structured = true
	d, err := p.Evaluate(context.Background(), e, ChannelQQ, "summary")
	if err != nil || !d.Send {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
	if err := p.MarkSent(context.Background(), d, e, ChannelQQ); err != nil {
		t.Fatal(err)
	}
	next, err := p.Retry(context.Background(), d, e, ChannelQQ)
	if err != nil {
		t.Fatal(err)
	}
	if next.Send || next.Reason != "already_sent" {
		t.Fatalf("retry after success=%+v", next)
	}
}

func TestPolicyDedupeRetryIdempotencyAndMetadataAudit(t *testing.T) {
	var audits []AuditEntry
	p, db, now := testPolicy(t, func(entry AuditEntry) { audits = append(audits, entry) })
	e := external("r", string(KindFinal), "stable-key")
	e.Structured = true
	d, err := p.Evaluate(context.Background(), e, ChannelQQ, "summary")
	if err != nil || !d.Send || d.Attempt != 1 {
		t.Fatalf("first decision=%+v err=%v", d, err)
	}
	if err := p.MarkFailed(context.Background(), d, e, ChannelQQ, errors.New("network")); err != nil {
		t.Fatal(err)
	}
	d, _ = p.Evaluate(context.Background(), e, ChannelQQ, "summary")
	if d.Send || d.Reason != "retry_backoff" {
		t.Fatalf("immediate retry=%+v", d)
	}
	*now = now.Add(2 * time.Second)
	d, err = p.Evaluate(context.Background(), e, ChannelQQ, "summary")
	if err != nil || !d.Send || d.Attempt != 2 {
		t.Fatalf("retry decision=%+v err=%v", d, err)
	}
	if err := p.MarkSent(context.Background(), d, e, ChannelQQ); err != nil {
		t.Fatal(err)
	}
	d, _ = p.Evaluate(context.Background(), e, ChannelQQ, "summary")
	if d.Send || d.Reason != "already_sent" {
		t.Fatalf("sent idempotency=%+v", d)
	}
	var receipt models.SendableDeliveryReceipt
	if err := db.First(&receipt, "dedupe_key = ?", "stable-key").Error; err != nil {
		t.Fatal(err)
	}
	if receipt.Attempts != 2 || receipt.Status != "sent" {
		t.Fatalf("receipt=%+v", receipt)
	}
	if len(audits) == 0 || audits[len(audits)-1].Result != "suppressed" {
		t.Fatalf("audits=%+v", audits)
	}
}
