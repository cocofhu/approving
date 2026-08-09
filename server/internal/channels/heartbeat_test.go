package channels

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/services"
)

func heartbeatManager(t *testing.T, fa *fakeAdapter) (*Manager, *services.TaskContextService) {
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
	return m, svc
}

func longRun() RunHeartbeat {
	return RunHeartbeat{
		ProjectID: "proj", RunID: "r1", NodeLabel: "跑测试", RunningFor: 42 * time.Minute,
	}
}

// TestHeartbeatBreaksTheSilenceOnALongRun is the whole point: a task that runs
// for an hour between two reportable edges looks exactly like a hung one.
func TestHeartbeatBreaksTheSilenceOnALongRun(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := heartbeatManager(t, fa)

	if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
		t.Fatal(err)
	}
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("a long-running task said nothing: %v", got)
	}
	if !strings.Contains(got[0], "支付登录页") {
		t.Fatalf("the update did not name the task: %q", got[0])
	}

	identity, err := svc.IdentityForRun("r1", "proj")
	if err != nil || identity == nil || identity.LastHeartbeatAt == nil {
		t.Fatalf("heartbeat not recorded: %+v err=%v", identity, err)
	}
}

// TestHeartbeatDoesNotRepeatWithinTheInterval: the sweep runs far more often
// than the reporting interval, so without this every tick would be a message.
func TestHeartbeatDoesNotRepeatWithinTheInterval(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := heartbeatManager(t, fa)

	for i := 0; i < 4; i++ {
		if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
			t.Fatal(err)
		}
	}
	if got := sentTexts(fa); len(got) != 1 {
		t.Fatalf("repeated sweeps produced %d messages, want 1: %v", len(got), got)
	}
}

// TestHeartbeatIsMarkedEvenWhenSuppressed guards the burst. A busy or rate
// limited conversation suppresses the message; treating that as "not reported
// yet" would retry every sweep and then deliver all of them at once the moment
// the conversation opened up.
func TestHeartbeatIsMarkedEvenWhenSuppressed(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := heartbeatManager(t, fa)
	m.SetSynthesizer(func(_ context.Context, _ SynthesisRequest) (string, error) {
		return "", nil
	})
	// Synthesis returning empty falls back to the structured line, so force the
	// suppression at the target instead: no channel, nothing to deliver to.
	m.StopAll()

	_ = m.ReportRunHeartbeat(context.Background(), longRun())
	identity, err := svc.IdentityForRun("r1", "proj")
	if err != nil || identity == nil {
		t.Fatalf("identity: %+v err=%v", identity, err)
	}
	if identity.LastHeartbeatAt == nil {
		t.Fatal("an undelivered heartbeat left no mark, so every later sweep would retry it")
	}
}

// TestHeartbeatStaysQuietWhenTheTaskJustReported: the platform saying the same
// thing right after the worker did is two voices repeating each other.
func TestHeartbeatStaysQuietWhenTheTaskJustReported(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := heartbeatManager(t, fa)
	if err := svc.RecordWorkNote("proj", "r1", "正在查代码", false); err != nil {
		t.Fatal(err)
	}

	if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
		t.Fatal(err)
	}
	if got := sentTexts(fa); len(got) != 0 {
		t.Fatalf("heartbeat talked over a fresh report from the task itself: %v", got)
	}
}

// TestHeartbeatUsesTheTasksOwnWordsWhenItHasThem: the note the worker left is
// more specific than the node label, so it wins.
func TestHeartbeatUsesTheTasksOwnWordsWhenItHasThem(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := heartbeatManager(t, fa)
	if err := svc.RecordWorkNote("proj", "r1", "正在查代码", false); err != nil {
		t.Fatal(err)
	}
	// Age the note past the interval so a heartbeat is due again.
	if err := svc.DB().Exec(
		"UPDATE task_identities SET last_stage_at = ? WHERE run_id = ?",
		time.Now().Add(-2*time.Hour), "r1").Error; err != nil {
		t.Fatal(err)
	}

	var brief string
	m.SetSynthesizer(func(_ context.Context, req SynthesisRequest) (string, error) {
		brief = req.Brief
		return "还在跑，现在在查代码。", nil
	})
	if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(brief, "正在查代码") {
		t.Fatalf("brief ignored what the task last reported: %q", brief)
	}
	if strings.Contains(brief, "跑测试") {
		t.Fatalf("brief preferred the node label over the task's own words: %q", brief)
	}
}

// TestHeartbeatRespectsTheOffSwitch: a project that does not want unprompted
// updates has to be able to say so, which is why 0 is a value and not "unset".
func TestHeartbeatRespectsTheOffSwitch(t *testing.T) {
	fa := &fakeAdapter{}
	m, svc := heartbeatManager(t, fa)
	m.SetHeartbeatInterval(0)

	if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
		t.Fatal(err)
	}
	if got := sentTexts(fa); len(got) != 0 {
		t.Fatalf("heartbeats are off, got %v", got)
	}
	identity, _ := svc.IdentityForRun("r1", "proj")
	if identity != nil && identity.LastHeartbeatAt != nil {
		t.Fatal("a disabled heartbeat must not touch the ledger")
	}
}

// TestHeartbeatSkipsRunsNobodyIsWaitingOn covers the three cases where there is
// no audience: web-triggered runs, detached runs, and tasks already settled.
func TestHeartbeatSkipsRunsNobodyIsWaitingOn(t *testing.T) {
	t.Run("no origin conversation", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, svc := bindingManager(t, fa)
		ensureTestIdentity(t, svc, "r1", "proj", "user1", "网页发起的活")
		if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
			t.Fatal(err)
		}
		if got := sentTexts(fa); len(got) != 0 {
			t.Fatalf("a web-triggered run spoke to nobody in particular: %v", got)
		}
	})

	t.Run("detached from its conversation", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, svc := heartbeatManager(t, fa)
		if _, err := svc.SetOriginBinding("proj", "r1", false); err != nil {
			t.Fatal(err)
		}
		if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
			t.Fatal(err)
		}
		if got := sentTexts(fa); len(got) != 0 {
			t.Fatalf("a detached run volunteered an update: %v", got)
		}
	})

	t.Run("task already settled", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, svc := heartbeatManager(t, fa)
		if _, err := svc.UpdateIdentity(services.EnsureTaskIdentityInput{
			RunID: "r1", ProjectID: "proj", UserID: "user1", Status: "completed",
		}); err != nil {
			t.Fatal(err)
		}
		if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
			t.Fatal(err)
		}
		if got := sentTexts(fa); len(got) != 0 {
			t.Fatalf("a settled task was reopened by a stale tick: %v", got)
		}
	})
}

// TestHeartbeatFallbackStandsOnItsOwn: with no model the line still has to be
// a sentence a person would send, not a status template.
func TestHeartbeatFallbackStandsOnItsOwn(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := heartbeatManager(t, fa)
	if err := m.ReportRunHeartbeat(context.Background(), longRun()); err != nil {
		t.Fatal(err)
	}
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("want one line, got %v", got)
	}
	for _, banned := range []string{"r1", "Approving", "run"} {
		if strings.Contains(got[0], banned) {
			t.Fatalf("fallback leaked %q: %q", banned, got[0])
		}
	}
	if strings.Contains(got[0], "完成") || strings.Contains(got[0], "失败") {
		t.Fatalf("a check-in must not read as an ending: %q", got[0])
	}
}
