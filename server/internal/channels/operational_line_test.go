package channels

import (
	"context"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/models"
)

// The platform owns the fact; the model owns the wording. A fixed string is
// what put 「你可以接着问别的」 in someone's chat window, so the fixed strings
// are demoted to what an unreachable model falls back to.
func TestOperationalLinePrefersTheModelAndFallsBackOnlyWhenItCannotSpeak(t *testing.T) {
	const fallback = "我这边还在处理前面几条，稍等一下。"

	t.Run("model speaks", func(t *testing.T) {
		m := NewManager(nil, nil, nil)
		m.SetLiveModel(&fakeLive{configured: true, report: &liveagent.Result{Text: "前面还压着几条，等我一下。"}})
		got := m.speakOperationalLine(context.Background(), operationalLine{
			Situation: "你手上还压着几条没回完。", Fallback: fallback,
		})
		if got != "前面还压着几条，等我一下。" {
			t.Fatalf("got %q want the model's line", got)
		}
	})

	t.Run("no model configured", func(t *testing.T) {
		m := NewManager(nil, nil, nil)
		got := m.speakOperationalLine(context.Background(), operationalLine{
			Situation: "你手上还压着几条没回完。", Fallback: fallback,
		})
		if got != fallback {
			t.Fatalf("got %q want the fallback", got)
		}
	})

	t.Run("model call fails", func(t *testing.T) {
		m := NewManager(nil, nil, nil)
		m.SetLiveModel(&fakeLive{configured: true})
		got := m.speakOperationalLine(context.Background(), operationalLine{
			Situation: "你手上还压着几条没回完。", Fallback: fallback,
		})
		if got != fallback {
			t.Fatalf("got %q want the fallback", got)
		}
	})
}

// The model is not trusted blindly: a line that claims the work is done, or
// pastes the ledger title back at the user, is worse than the template.
func TestOperationalLineRejectsUnusableModelOutput(t *testing.T) {
	for name, reply := range map[string]string{
		"claims finished": "那事已经跑完了，结果我发你。",
		"pastes title":    "「调研快模型和 worker 架构」这事我先放着。",
	} {
		t.Run(name, func(t *testing.T) {
			m := NewManager(nil, nil, nil)
			m.SetLiveModel(&fakeLive{configured: true, report: &liveagent.Result{Text: reply}})
			got := m.speakOperationalLine(context.Background(), operationalLine{
				Situation: "这一轮没拿出结论。",
				Avoid:     "调研快模型和 worker 架构",
				Fallback:  "这条我没能给出结论。",
			})
			if got != "这条我没能给出结论。" {
				t.Fatalf("got %q want the fallback", got)
			}
		})
	}
}

// Wiring check on a real path: with a model configured, the queue-full notice
// is the model's sentence, not the constant.
func TestQueueFullNoticeIsPhrasedByTheModel(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.SetLiveModel(&fakeLive{configured: true, report: &liveagent.Result{Text: "前面几条还没弄完，稍等我一下。"}})
	rc := testRunningChannel(fa)

	m.sendBusyHint(context.Background(), rc, testInbound("overflow"), "conv-key")

	got := sentTexts(fa)
	if countText(got, busyHintText) != 0 {
		t.Fatalf("the fixed queue-full template still went out: %v", got)
	}
	if countText(got, "前面几条还没弄完，稍等我一下。") != 1 {
		t.Fatalf("sends = %v want the model's queue-full line exactly once", got)
	}
}

// The same path with no model reachable still says something: the notice is
// the point, the wording is the improvement.
func TestQueueFullNoticeStillGoesOutWithoutAModel(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	rc := testRunningChannel(fa)

	m.sendBusyHint(context.Background(), rc, testInbound("overflow"), "conv-key")

	if got := sentTexts(fa); countText(got, busyHintText) != 1 {
		t.Fatalf("sends = %v want the fallback notice exactly once", got)
	}
}

// Whatever any of these lines say, none of them may narrate what the user is
// allowed to do next. That is the shape of the original complaint.
func TestNoOperationalCopyCoachesTheUser(t *testing.T) {
	m := NewManager(nil, nil, nil)
	copies := []string{
		busyHintText,
		turnTooSlowText("zh-CN"), turnTooSlowText("en"),
		riskExecutionFailedText("zh-CN"), riskExecutionFailedText("en"),
		interruptedTurnText("zh-CN"), interruptedTurnText("en"),
		runAcceptanceText("登录页", "zh-CN"), runAcceptanceText("登录页", "en"),
		m.noAnswerFallback(&runningChannel{cfg: models.ChannelConfig{ProjectID: "p"}}, InboundMessage{}),
	}
	for _, text := range copies {
		for _, banned := range []string{"接着问", "keep chatting", "Feel free", "随时叫我", "随时找我"} {
			if strings.Contains(text, banned) {
				t.Errorf("copy %q coaches the user with %q", text, banned)
			}
		}
	}
}
