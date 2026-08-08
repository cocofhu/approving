package channels

import (
	"context"
	"errors"
	"testing"

	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"
)

// unboundManager wires a manager whose run r1 came from conversation user1 and
// has been detached from it.
func unboundManager(t *testing.T, fa *fakeAdapter) (*Manager, *services.TaskContextService) {
	t.Helper()
	m, svc := bindingManager(t, fa)
	if _, err := svc.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "r1", ProjectID: "proj", UserID: "user1",
		ShortTitle: "支付登录页", OriginalRequirement: "修支付登录页", Status: "active",
		OriginChannel: "c1", OriginScene: string(SceneC2C), OriginConversationID: "user1",
		OriginExternalUserID: "user1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetOriginBinding("proj", "r1", false); err != nil {
		t.Fatal(err)
	}
	return m, svc
}

// TestUnboundRunIsSilencedEvenWithAnExplicitConversation is the test the
// feature exists for.
//
// Every worker-originated path passes an explicit ConversationID, so a guard
// placed where the code looks the origin conversation up — which is only when
// the field is blank — would have let all of them straight through while the
// UI claimed the run was detached.
func TestUnboundRunIsSilencedEvenWithAnExplicitConversation(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := unboundManager(t, fa)

	req := progressRequest("r1", "user1", "已提交分支", "s1")
	if req.ConversationID == "" {
		t.Fatal("this test is only meaningful when the request names its conversation")
	}
	result, err := m.DeliverSendable(context.Background(), req)
	if err != nil {
		t.Fatalf("an unbound run is a suppression, not a failure: %v", err)
	}
	if result.Sent {
		t.Fatal("a detached run must not reach its former conversation")
	}
	if result.Reason() != ReasonOriginUnbound {
		t.Fatalf("reason = %q want %q", result.Reason(), ReasonOriginUnbound)
	}
	if got := sentTexts(fa); len(got) != 0 {
		t.Fatalf("detached run still spoke: %v", got)
	}
}

// TestUnboundSuppressionDoesNotLookLikeAFailure locks the contract
// pm_notify_progress makes to the worker: only a real transport failure comes
// back as an error. Getting this wrong would make the worker rephrase and
// resubmit forever, so detaching a run would make the system louder.
func TestUnboundSuppressionDoesNotLookLikeAFailure(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := unboundManager(t, fa)

	result, err := m.DeliverSendable(context.Background(), progressRequest("r1", "user1", "已提交分支", "s1"))
	if errors.Is(err, ErrDeliveryFailed) {
		t.Fatal("an unbound run reported as a delivery failure would be retried forever")
	}
	if result.Failed() {
		t.Fatalf("suppression must not read as failed: %+v", result)
	}
	if deliveryFailureReasons[ReasonOriginUnbound] {
		t.Fatal("origin_unbound is a policy suppression and must stay out of the failure set")
	}
}

// TestRebindingRestoresDelivery keeps the switch reversible: an operator who
// detaches a run by mistake has to be able to put it back.
func TestRebindingRestoresDelivery(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := unboundManager(t, fa)

	if _, err := svc.SetOriginBinding("proj", "r1", true); err != nil {
		t.Fatal(err)
	}
	result, err := m.DeliverSendable(context.Background(), progressRequest("r1", "user1", "已提交分支", "s2"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Sent {
		t.Fatalf("a rebound run talks again, got %+v", result)
	}
}

// TestUnbindKeepsTheOriginRecorded guards against the obvious wrong
// implementation. Clearing OriginConversationID would look equivalent and is
// not: the origin fields are write-once so the next update would restore them,
// and an empty conversation makes delivery fall back to the project push
// target — the detached run would keep talking, to the wrong audience.
func TestUnbindKeepsTheOriginRecorded(t *testing.T) {
	_, svc := unboundManager(t, &fakeAdapter{})
	identity, err := svc.IdentityForRun("r1", "proj")
	if err != nil || identity == nil {
		t.Fatalf("identity: %+v err=%v", identity, err)
	}
	if identity.OriginConversationID != "user1" {
		t.Fatalf("origin conversation = %q; detaching must not erase where the run came from",
			identity.OriginConversationID)
	}
	if identity.OriginUnboundAt == nil {
		t.Fatal("detached run carries no unbind mark")
	}
}

// TestUnboundRunStillTracksStatus separates the two things a detach is often
// confused for. It stops the talking, not the tracking: a status question in
// any conversation, and the task ledger behind the run list, must still say
// the run finished.
func TestUnboundRunStillTracksStatus(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := unboundManager(t, fa)

	if err := m.ReflowTaskOutcome(context.Background(), TaskOutcome{
		ProjectID: "proj", RunID: "r1", Status: "completed",
		ResultSummary: "已经修完了",
	}); err != nil {
		t.Fatal(err)
	}
	identity, err := svc.IdentityForRun("r1", "proj")
	if err != nil || identity == nil {
		t.Fatalf("identity: %+v err=%v", identity, err)
	}
	if !services.IsTerminalTaskStatus(identity.Status) {
		t.Fatalf("task status = %q; a detached run is still tracked to completion", identity.Status)
	}
	if got := sentTexts(fa); len(got) != 0 {
		t.Fatalf("detached run announced its own outcome: %v", got)
	}
}

// TestUnbindingARunWithNoOriginIsANoop: web-triggered runs have nothing to
// detach, and pretending otherwise would write a mark that nothing reads.
func TestUnbindingARunWithNoOriginIsANoop(t *testing.T) {
	_, svc := bindingManager(t, &fakeAdapter{})
	ensureTestIdentity(t, svc, "web1", "proj", "user1", "网页发起的活")
	identity, err := svc.SetOriginBinding("proj", "web1", false)
	if err != nil {
		t.Fatal(err)
	}
	if identity == nil || identity.OriginUnboundAt != nil {
		t.Fatalf("a run with no origin has no binding to break: %+v", identity)
	}
}

// TestAnnounceOriginBindingSpeaksBeforeTheMarkIsWritten pins the ordering the
// handler depends on. Writing the mark first would have the guard swallow the
// goodbye, and the person waiting on the run would simply stop hearing back
// with no explanation.
func TestAnnounceOriginBindingSpeaksBeforeTheMarkIsWritten(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := bindingManager(t, fa)
	if _, err := svc.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "r1", ProjectID: "proj", UserID: "user1",
		ShortTitle: "支付登录页", Status: "active",
		OriginChannel: "c1", OriginScene: string(SceneC2C), OriginConversationID: "user1",
	}); err != nil {
		t.Fatal(err)
	}

	if !m.AnnounceOriginBinding(context.Background(), "proj", "r1", false) {
		t.Fatal("the goodbye must go out while the run is still bound")
	}
	if got := sentTexts(fa); len(got) != 1 {
		t.Fatalf("want exactly one goodbye, got %v", got)
	}

	if _, err := svc.SetOriginBinding("proj", "r1", false); err != nil {
		t.Fatal(err)
	}
	if m.AnnounceOriginBinding(context.Background(), "proj", "r1", false) {
		t.Fatal("once detached there is nobody left to tell")
	}
}

func TestAnnounceOriginBindingIgnoresRunsWithNoConversation(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := bindingManager(t, fa)
	ensureTestIdentity(t, svc, "web1", "proj", "user1", "网页发起的活")
	if m.AnnounceOriginBinding(context.Background(), "proj", "web1", false) {
		t.Fatal("a web-triggered run has no origin conversation to say goodbye to")
	}
	if m.AnnounceOriginBinding(context.Background(), "proj", "missing", false) {
		t.Fatal("an unknown run has nothing to announce")
	}
	if got := sentTexts(fa); len(got) != 0 {
		t.Fatalf("unexpected sends: %v", got)
	}
}

// TestUnboundRunNeverFallsBackToTheProjectTarget is the failure this design
// avoids. With the origin merely blanked, a message carrying no conversation
// falls through to lookupRunNotifyTarget and lands in the ops session — one
// user's results delivered to an audience that never asked for them.
func TestUnboundRunNeverFallsBackToTheProjectTarget(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := unboundManager(t, fa)

	req := progressRequest("r1", "user1", "已提交分支", "s1")
	req.ConversationID = ""
	req.Scene = ""
	result, err := m.DeliverSendable(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent || result.Reason() != ReasonOriginUnbound {
		t.Fatalf("detached run fell through to the project target: %+v", result)
	}
}

// TestSendableRequestWithoutRunIDIsUnaffected: ordinary conversation, which
// belongs to no run at all, must not be touched by any of this.
func TestSendableRequestWithoutRunIDIsUnaffected(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := unboundManager(t, fa)

	result, err := m.DeliverSendable(context.Background(), SendableRequest{
		ProjectID: "proj", Scene: SceneC2C, ConversationID: "user1", UserID: "user1",
		Kind: sendable.KindFinal, Reason: ReasonPMReply, TaskContext: "闲聊",
		DedupeKey: "chat:1", Text: "在的",
	})
	if err != nil || !result.Sent {
		t.Fatalf("plain conversation = %+v err=%v", result, err)
	}
}

func TestNormalizeSceneDefaultsToC2C(t *testing.T) {
	if got := normalizeScene(""); got != SceneC2C {
		t.Fatalf("blank scene = %q want %q", got, SceneC2C)
	}
	if got := normalizeScene(SceneGroup); got != SceneGroup {
		t.Fatalf("group scene = %q want it left alone", got)
	}
}
