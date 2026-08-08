package channels

import (
	"context"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"
)

// TestPlainProgressIsRecordedNotAnnounced is the behaviour change: a worker
// that narrates every step turns the conversation into a build log, so the
// step is kept as a fact and not sent. Keeping the record is the other half —
// dropping it would leave nothing to answer "how's it going" with.
func TestPlainProgressIsRecordedNotAnnounced(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := bindingManager(t, fa)
	ensureTestIdentity(t, svc, "r1", "proj", services.SyntheticQQUserID("user1"), "支付登录页")

	result, err := m.ReportRunProgress(context.Background(),
		progressRequest("r1", "user1", "已提交分支", "正在查代码"))
	if err != nil {
		t.Fatalf("plain progress must not look like a failure: %v", err)
	}
	if result.Sent || result.Reason() != ReasonLedgerOnly {
		t.Fatalf("progress report = %+v want a ledger-only suppression", result)
	}
	if got := sentTexts(fa); len(got) != 0 {
		t.Fatalf("plain progress interrupted the user: %v", got)
	}

	identity, err := svc.IdentityForRun("r1", "proj")
	if err != nil || identity == nil {
		t.Fatalf("identity: %+v err=%v", identity, err)
	}
	if identity.LastStage != "正在查代码" {
		t.Fatalf("last stage = %q; the report has to survive as a fact", identity.LastStage)
	}
	if identity.LastStageAt == nil {
		t.Fatal("a stage with no timestamp cannot be aged, so it cannot be trusted later")
	}
}

// TestWorkNoteSurvivesRestart pins why the note moved out of memory. For a task
// that runs for hours this is the only concrete thing the conversation can say
// about it, and a process restart used to wipe every one at once.
func TestWorkNoteSurvivesRestart(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := bindingManager(t, fa)
	ensureTestIdentity(t, svc, "r1", "proj", services.SyntheticQQUserID("user1"), "支付登录页")
	if _, err := m.ReportRunProgress(context.Background(),
		progressRequest("r1", "user1", "已提交分支", "正在查代码")); err != nil {
		t.Fatal(err)
	}

	// A fresh manager over the same store is what a restart looks like.
	fresh := NewManager(nil, nil, nil)
	fresh.SetTaskContextService(svc)
	identity, err := svc.IdentityForRun("r1", "proj")
	if err != nil || identity == nil {
		t.Fatalf("identity: %+v err=%v", identity, err)
	}
	entries := fresh.entriesFromIdentities("proj", []models.TaskIdentity{*identity})
	if len(entries) != 1 || entries[0].Stage != "正在查代码" {
		t.Fatalf("briefing after restart = %+v; the stage did not survive", entries)
	}
}

// TestBlockedReportStillInterrupts: quieting progress must not quiet the cases
// that actually need someone.
func TestBlockedReportStillInterrupts(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind sendable.Kind
	}{
		{"blocked", sendable.KindBlocked},
		{"needs a decision", sendable.KindActionRequired},
		{"final", sendable.KindFinal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fa := &fakeAdapter{}
			m, svc := bindingManager(t, fa)
			ensureTestIdentity(t, svc, "r1", "proj", services.SyntheticQQUserID("user1"), "支付登录页")

			req := progressRequest("r1", "user1", "依赖装不上，卡住了", "装依赖")
			req.Kind = tc.kind
			result, err := m.ReportRunProgress(context.Background(), req)
			if err != nil || !result.Sent {
				t.Fatalf("%s report = %+v err=%v", tc.name, result, err)
			}
			if got := sentTexts(fa); len(got) != 1 {
				t.Fatalf("%s report sent %v", tc.name, got)
			}
		})
	}
}

// TestReportFallsBackToTheWorkersWordsWithoutASynthesizer: phrasing that is
// unavailable degrades to the worker's own text, never to silence.
func TestReportFallsBackToTheWorkersWordsWithoutASynthesizer(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := bindingManager(t, fa)
	ensureTestIdentity(t, svc, "r1", "proj", services.SyntheticQQUserID("user1"), "支付登录页")

	req := progressRequest("r1", "user1", "依赖装不上，卡住了", "装依赖")
	req.Kind = sendable.KindBlocked
	if _, err := m.ReportRunProgress(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got := sentTexts(fa)
	if len(got) != 1 || !strings.Contains(got[0], "依赖装不上") {
		t.Fatalf("without a synthesizer the worker's words go out, got %v", got)
	}
}

// TestReportIsPhrasedByTheConversationLayer is the point of routing these
// through synthesis: the user hears one voice, not the worker's register on
// alternate messages.
func TestReportIsPhrasedByTheConversationLayer(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := bindingManager(t, fa)
	ensureTestIdentity(t, svc, "r1", "proj", services.SyntheticQQUserID("user1"), "支付登录页")

	var brief string
	m.SetSynthesizer(func(_ context.Context, req SynthesisRequest) (string, error) {
		brief = req.Brief
		return "装依赖那步卡住了，得你看一眼。", nil
	})

	req := progressRequest("r1", "user1", "[node:install] dependency resolution failed", "装依赖")
	req.Kind = sendable.KindBlocked
	if _, err := m.ReportRunProgress(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "装依赖那步卡住了，得你看一眼。" {
		t.Fatalf("report went out unphrased: %v", got)
	}
	if !strings.Contains(brief, "支付登录页") {
		t.Fatalf("brief did not name the task: %q", brief)
	}
	if !strings.Contains(brief, "卡住") {
		t.Fatalf("brief did not say what kind of report this is: %q", brief)
	}
}

// TestBackgroundReplyIsPhrasedToo closes the other half of the gap: pm_reply
// carried a worker's conclusion straight to IM, so the same conversation
// alternated between two voices depending on which layer spoke.
func TestBackgroundReplyIsPhrasedToo(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := bindingManager(t, fa)
	ensureTestIdentity(t, svc, "r1", "proj", services.SyntheticQQUserID("user1"), "支付登录页")
	m.SetSynthesizer(func(_ context.Context, _ SynthesisRequest) (string, error) {
		return "修好了，登录页现在能正常跳转。", nil
	})

	result, err := m.DeliverConversationReply(context.Background(), ConversationReply{
		ProjectID: "proj", RunID: "r1", Scene: SceneC2C, ConversationID: "user1",
		UserID: "user1", Text: "run r1 completed: login redirect fixed at node[auth]",
		ShortTitle: "支付登录页",
	})
	if err != nil || !result.Sent {
		t.Fatalf("reply = %+v err=%v", result, err)
	}
	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "修好了，登录页现在能正常跳转。" {
		t.Fatalf("worker's raw conclusion reached the user: %v", got)
	}
}

// TestReplyWithoutARunIsLeftAlone: ordinary conversation has no task to brief
// against, and rewriting it would be the conversation layer talking to itself.
func TestReplyWithoutARunIsLeftAlone(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := bindingManager(t, fa)
	m.SetSynthesizer(func(_ context.Context, _ SynthesisRequest) (string, error) {
		t.Fatal("a run-less reply must not be handed to synthesis")
		return "", nil
	})

	result, err := m.DeliverConversationReply(context.Background(), ConversationReply{
		ProjectID: "proj", Scene: SceneC2C, ConversationID: "user1",
		UserID: "user1", Text: "在的，你说。",
	})
	if err != nil || !result.Sent {
		t.Fatalf("reply = %+v err=%v", result, err)
	}
	if got := sentTexts(fa); len(got) != 1 || got[0] != "在的，你说。" {
		t.Fatalf("plain conversation was rewritten: %v", got)
	}
}

func TestReportedStagePrefersTheStageThenTheConclusion(t *testing.T) {
	req := SendableRequest{Text: "正文"}
	if got := reportedStage(req); got != "正文" {
		t.Fatalf("with nothing else the message itself is the stage, got %q", got)
	}
	req.Progress.Conclusion = "结论"
	if got := reportedStage(req); got != "结论" {
		t.Fatalf("a conclusion beats the raw message, got %q", got)
	}
	req.Progress.Stage = "阶段"
	if got := reportedStage(req); got != "阶段" {
		t.Fatalf("an explicit stage wins, got %q", got)
	}
}
