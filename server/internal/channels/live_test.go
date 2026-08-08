package channels

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

// liveManager builds a Manager wired end to end: a policy over a real database,
// a task-context service, and one running QQ channel whose adapter is fa.
func liveManager(t *testing.T, fa *fakeAdapter) (*Manager, *services.TaskContextService) {
	t.Helper()
	m, db := policyManager(t, fa, nil)
	tasks := services.NewTaskContextService(db)
	m.SetTaskContextService(tasks)
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "c2c:cron-target",
	}})
	t.Cleanup(m.StopAll)
	m.mu.Lock()
	for _, rc := range m.running {
		rc.adapter = fa
	}
	m.mu.Unlock()
	return m, tasks
}

// A complaint is not a command. The keyword router read 「不要这样啊」 as a cancel
// and 「那这个是 BUG 吗」 as work to delegate, so both went somewhere the user did
// not ask for. Nothing classifies text before the model sees it now — the guard
// is that no message settles a ticket unless one is pending and the reply is a
// plain yes or no.
func TestCommentaryNeverAuthorizesADestructiveAction(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	risk := services.NewRiskConfirmationService(db)
	m.SetRiskConfirmationService(risk)
	m.SetTaskContextService(services.NewTaskContextService(db))
	executed := 0
	m.SetRiskActionExecutor(func(string, string, string, map[string]string) error {
		executed++
		return nil
	})
	turned := 0
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		turned++
		return Reply{FinalSummary: "agent-handled"}, nil
	}
	rc := &runningChannel{
		cfg:     models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: "proj"},
		adapter: fa,
	}
	ticket, err := risk.CreateTicket(services.RiskTicketInput{
		ProjectID: "proj", UserID: services.SyntheticQQUserID("u1"),
		RunID: "r1", Action: "cancel_run", ShortTitle: "登录页性能优化", Language: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := risk.MarkPrompted(ticket.ID); err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{
		"不要这样啊", "我不想取消这个", "为什么会取消", "这个功能没实现，不对吧",
		"顺便问一下批准流程是怎么走的", "那这个是 BUG 吗",
	} {
		m.handleInbound(context.Background(), rc, InboundMessage{
			Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
			MessageID: "c" + strconv.Itoa(i), Text: text,
		})
		if executed != 0 {
			t.Fatalf("%q authorized a destructive action", text)
		}
		if turned != i+1 {
			t.Fatalf("%q was swallowed before the agent, turns=%d", text, turned)
		}
	}
	// The ticket is still open, and a plain yes still settles it.
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "c-yes", Text: "确认",
	})
	if executed != 1 {
		t.Fatalf("an explicit confirmation did not settle the ticket (executed=%d)", executed)
	}
}

// The outbound vocabulary guard, mirroring the web client's userFacingCopy
// test: nothing the platform says to a user may mention how the platform is
// built. This covers the static copy; ScrubInternalTerms covers text a model
// composed.
func TestOutboundCopyNeverExposesInternals(t *testing.T) {
	copies := []string{
		busyHintText,
		turnTooSlowText("zh-CN"), turnTooSlowText("en"),
		interruptedTurnText("zh-CN"), interruptedTurnText("en"),
		runAcceptanceText("登录页性能", "zh-CN"), runAcceptanceText("Login perf", "en"),
		FormatProgressText(ProgressEvent{Kind: ProgressMilestone, Summary: "已提交分支"}),
	}
	for _, err := range []error{
		errNoReply(), context.DeadlineExceeded,
	} {
		copies = append(copies, turnFailureText(err))
	}
	for _, text := range copies {
		if strings.TrimSpace(text) == "" {
			continue
		}
		if ContainsInternalTerms(text) {
			t.Errorf("user-facing copy exposes internals: %q", text)
		}
		for _, banned := range []string{
			"Run ID", "run_id", "Sendable", "sandbox", "ACP", "node_complete",
			"suppressed", "assistant produced no reply", "请前往 Approving",
		} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(banned)) {
				t.Errorf("user-facing copy contains %q: %q", banned, text)
			}
		}
	}
	// Fixed stillWorking templates are banned from egress; scrub must drop them.
	for _, banned := range []string{stillWorkingText("zh-CN"), stillWorkingText("en")} {
		if got := ScrubInternalTerms(banned); got != "" {
			t.Errorf("stillWorking template must scrub to empty, got %q from %q", got, banned)
		}
	}
}

func errNoReply() error { return errNoReplyErr{} }

type errNoReplyErr struct{}

func (errNoReplyErr) Error() string { return "assistant produced no reply" }

// ScrubInternalTerms is the last line of defence on text a model wrote. It has
// to remove the internals and still leave a readable sentence.
func TestScrubInternalTermsCleansModelComposedText(t *testing.T) {
	cases := []struct {
		in       string
		mustDrop []string
		mustKeep []string
	}{
		{
			in:       "我已经在 run-1ca1876f 这个 sandbox 里跑完了，本回合已结束。",
			mustDrop: []string{"run-1ca1876f", "sandbox", "本回合"},
			mustKeep: []string{"跑完了"},
		},
		{
			in:       "tool_call gh pr view\n[摘要] 登录页首屏从 3.2s 降到 1.1s",
			mustDrop: []string{"tool_call", "[摘要]"},
			mustKeep: []string{"登录页首屏从 3.2s 降到 1.1s"},
		},
		{
			in:       "node_complete: 构建完成，delivery suppressed",
			mustDrop: []string{"node_complete", "suppressed"},
			mustKeep: []string{"构建完成"},
		},
	}
	for _, c := range cases {
		got := ScrubInternalTerms(c.in)
		for _, drop := range c.mustDrop {
			if strings.Contains(got, drop) {
				t.Errorf("ScrubInternalTerms(%q) still contains %q: %q", c.in, drop, got)
			}
		}
		for _, keep := range c.mustKeep {
			if !strings.Contains(got, keep) {
				t.Errorf("ScrubInternalTerms(%q) lost %q: %q", c.in, keep, got)
			}
		}
		if strings.Contains(got, "  ") || strings.HasPrefix(got, " ") {
			t.Errorf("ScrubInternalTerms(%q) left ragged spacing: %q", c.in, got)
		}
	}
}

// A title cut in half was stored long before the message quoting it was
// composed, so the egress guard is the only place that can still catch it.
func TestRepairTruncatedCopyFixesQuotedTitleDebris(t *testing.T) {
	got := RepairTruncatedCopy("刚才那个「调研 Approving 最近关于快模型和 wo」已经跑完了。")
	if strings.Contains(got, "和 wo") {
		t.Fatalf("mid-word debris survived: %q", got)
	}
	if !strings.Contains(got, "「调研 Approving 最近关于快模型和…」") {
		t.Fatalf("title not repaired in place: %q", got)
	}
	if !strings.HasSuffix(got, "已经跑完了。") {
		t.Fatalf("repair damaged the rest of the sentence: %q", got)
	}
}

func TestRepairTruncatedCopyLeavesGoodCopyAlone(t *testing.T) {
	intact := []string{
		"「登录页性能优化」跑完了，首屏从 3.2s 降到 1.1s，改动在 https://example.com/pr/1 。",
		"两个检查还在跑，其余全部通过。",
		"「快模型和 worker 架构精简分析」还在跑，大概过半。",
		"CI 全绿了，可以合 PR",
	}
	for _, in := range intact {
		if got := RepairTruncatedCopy(in); got != in {
			t.Errorf("RepairTruncatedCopy(%q) = %q, want unchanged", in, got)
		}
	}
}

// A byte-sliced payload reaches the composer as U+FFFD. Showing a user a black
// diamond is not better than showing them nothing.
func TestRepairTruncatedCopyDropsReplacementChars(t *testing.T) {
	if got := RepairTruncatedCopy("结论：首屏更快了\uFFFD"); strings.Contains(got, "\uFFFD") {
		t.Fatalf("replacement char survived: %q", got)
	}
}

// The identifier guard has to cut both ways. Deleting a real id is the point;
// deleting a chunk of an ordinary sentence is a worse failure than the jargon
// would have been, because the user cannot even tell something went missing.
func TestIdentifierScrubbingLeavesOrdinaryProseAlone(t *testing.T) {
	mustStrip := []string{
		"run-1ca1876f", "run_1ca1876f", "task-9f8e7d6c5b",
		"Run ID: 1ca1876f", "run #1ca1876f",
	}
	for _, text := range mustStrip {
		if !ContainsInternalTerms(text) {
			t.Errorf("%q was not recognised as an identifier", text)
		}
		if got := ScrubInternalTerms("结果在 " + text + " 里"); strings.Contains(got, "1ca1876f") ||
			strings.Contains(got, "9f8e7d6c5b") {
			t.Errorf("scrubbing %q left the identifier: %q", text, got)
		}
	}

	keep := []struct{ text, mustSurvive string }{
		{"we run 5000 iterations before merging", "5000 iterations"},
		{"跑了 run 5000 次迭代", "5000"},
		{"run 3 times and compare", "3 times"},
		{"这个 task 2 天就能做完", "2 天"},
		{"run-optimization 那个分支", "run-optimization"},
		{"the long-run tradeoff here", "long-run tradeoff"},
		{"overrun2024 is a variable name", "overrun2024"},
	}
	for _, c := range keep {
		if ContainsInternalTerms(c.text) {
			t.Errorf("%q was wrongly flagged as containing an identifier", c.text)
		}
		if got := ScrubInternalTerms(c.text); !strings.Contains(got, c.mustSurvive) {
			t.Errorf("ScrubInternalTerms(%q) = %q, lost %q", c.text, got, c.mustSurvive)
		}
	}
}

// One message in, one message out. No acknowledgement, no queue narration, no
// trailing wrap-up alongside the answer.
func TestOneUserMessageProducesOneReply(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		return Reply{Text: "内部推理若干", FinalSummary: "不是 BUG，是缓存没刷新。"}, nil
	}
	rc := testRunningChannel(fa)
	m.dispatch(context.Background(), rc, testInboundText("q1", "那这个是 BUG 吗"))

	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "不是 BUG，是缓存没刷新。" {
		t.Fatalf("sends = %v want exactly the answer", got)
	}
}

// An answer the agent submitted through pm_reply is the turn's answer. Even
// when Bridge builds a non-empty FinalSummary from delivery-receipt asides,
// the wrap-up must not append a second message.
func TestExplicitReplySuppressesTheTurnWrapUp(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := liveManager(t, fa)
	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"
	m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		// What pm_reply does, through the same egress the MCP host uses.
		_, err := m.DeliverConversationReply(ctx, ConversationReply{
			ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
			UserID: in.UserID, Text: "缓存没刷新而已。",
		})
		return Reply{}, err
	}
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
		MessageID: "q1", Text: "那这个是 BUG 吗",
	})

	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "缓存没刷新而已。" {
		t.Fatalf("sends = %v want only the agent's own reply", got)
	}
}

// pm_reply already answered; Bridge still builds a non-empty FinalSummary from
// model asides (「已发送。」). hasReplied must short-circuit structured_turn_final
// so the user sees exactly one message — the real answer.
func TestPmReplyPlusNonEmptyFinalSummarySendsExactlyOnce(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := liveManager(t, fa)
	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"
	const answer = "今天在看登录页首屏延迟。"
	m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		if _, err := m.DeliverConversationReply(ctx, ConversationReply{
			ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
			UserID: in.UserID, Text: answer,
		}); err != nil {
			return Reply{}, err
		}
		aside := "已通过 QQ 回复用户。\n已发送。\n" + answer
		return Reply{
			Text:         aside,
			FinalSummary: buildDeliverableFinalSummary(aside),
		}, nil
	}
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
		MessageID: "q-pm-final", Text: "你在看什么",
	})
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want exactly 1 (pm_reply only)", got)
	}
	if got[0] != answer {
		t.Fatalf("outbound = %q want %q", got[0], answer)
	}
	assertNoBannedOutbound(t, got)
}

// One question, one answer — even when the agent submits twice.
//
// This is what shipped to production and got caught: an agent answered 「你好」
// with two pm_reply calls thirteen seconds apart, and the user watched the
// platform greet them twice. Dedupe could not collapse them because its key
// includes the text and the two answers were worded differently, and the
// existing marker was only consulted at the turn's wrap-up. The second call is
// now withheld and told so, which is the only outcome that keeps the agent from
// rewording and trying again.
func TestSecondPmReplyInOneTurnIsWithheldAndReported(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := liveManager(t, fa)
	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"

	var second DeliveryResult
	m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		reply := func(text string) (DeliveryResult, error) {
			return m.DeliverConversationReply(ctx, ConversationReply{
				ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
				UserID: in.UserID, Text: text,
			})
		}
		if _, err := reply("你好，我在。你继续说。"); err != nil {
			return Reply{}, err
		}
		var err error
		if second, err = reply("你好，有什么需要我帮你看的？"); err != nil {
			return Reply{}, err
		}
		return Reply{}, nil
	}
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
		MessageID: "q-double", Text: "你好",
	})

	got := sentTexts(fa)
	if len(got) != 1 || got[0] != "你好，我在。你继续说。" {
		t.Fatalf("sends = %v want only the first answer", got)
	}
	if second.Sent {
		t.Fatal("the second answer was delivered")
	}
	if second.Reason() != ReasonAlreadyReplied {
		t.Fatalf("second reason = %q want %q", second.Reason(), ReasonAlreadyReplied)
	}
	// A withheld duplicate is a normal outcome; reporting it as a transport
	// failure would make the agent retry the very thing being prevented.
	if second.Failed() || !second.Suppressed() {
		t.Fatalf("second outcome: failed=%v suppressed=%v", second.Failed(), second.Suppressed())
	}
}

// The gate is per turn, not per conversation: the next question gets its own
// answer. A marker that outlived its turn would mute the conversation from the
// second message onward, which is worse than what it set out to fix.
func TestTheNextTurnCanBeAnsweredAgain(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := liveManager(t, fa)
	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"
	m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		_, err := m.DeliverConversationReply(ctx, ConversationReply{
			ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
			UserID: in.UserID, Text: "答:" + in.Text,
		})
		return Reply{}, err
	}
	for _, q := range []string{"你好", "什么进度了"} {
		m.handleInbound(context.Background(), rc, InboundMessage{
			Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
			MessageID: "q-" + q, Text: q,
		})
	}
	want := []string{"答:你好", "答:什么进度了"}
	got := sentTexts(fa)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sends = %v want %v", got, want)
	}
}

// A progress milestone is not the answer. It sets the wrap-up marker so the
// turn does not append a summary, but it must never be the reason the answer
// itself is withheld — silence would be a far worse failure than a duplicate.
func TestProgressMilestoneDoesNotWithholdTheAnswer(t *testing.T) {
	fa := &fakeAdapter{}
	m, _ := liveManager(t, fa)
	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"
	const answer = "首屏慢在字体加载，我改了预加载。"
	m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		scope := conversationTurnScope("proj", in.Scene, in.ConversationID)
		m.markReplied(scope) // stands in for a delivered progress milestone
		_, err := m.DeliverConversationReply(ctx, ConversationReply{
			ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
			UserID: in.UserID, Text: answer,
		})
		return Reply{}, err
	}
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
		MessageID: "q-progress", Text: "首屏为什么慢",
	})
	if got := sentTexts(fa); len(got) != 1 || got[0] != answer {
		t.Fatalf("sends = %v want the answer to survive a progress milestone", got)
	}
}

// A turn that overruns has started nothing and is running nothing, so the user
// is offered the delegation rather than told it already happened.
func TestForegroundTurnTimeoutOffersDelegationInsteadOfPromising(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(ctx context.Context, _ ResolvedChannel, _ InboundMessage) (Reply, error) {
		<-ctx.Done()
		return Reply{}, ctx.Err()
	}
	rc := testRunningChannel(fa)
	rc.cfg.TurnTimeoutSeconds = 1
	m.openBudget = -1 // no cold-start allowance; this turn is pure thinking time

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.dispatch(context.Background(), rc, testInboundText("slow", "帮我把整个仓库重构一遍"))
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a foreground turn held the conversation past its budget")
	}

	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want one message", got)
	}
	if got[0] != turnTooSlowText("zh-CN") {
		t.Fatalf("timeout message = %q want the delegation offer", got[0])
	}
	if strings.Contains(got[0], "失败") || strings.Contains(got[0], "错误") {
		t.Fatalf("timeout was reported as a failure: %q", got[0])
	}
	// The offer must not claim work is already under way; nothing is running.
	for _, promise := range []string{"放到后台接着弄", "有结果就来说", "正在处理"} {
		if strings.Contains(got[0], promise) {
			t.Fatalf("timeout message promises work nobody is doing: %q", got[0])
		}
	}
}

// Foreground turns must stay silent until the single final answer: the fixed
// stillWorking template is hard-disabled (Demo: no 「稍等，我看一下。」).
func TestForegroundTurnsNeverSendStillWorkingTemplate(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		return Reply{FinalSummary: "不是 BUG。"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInboundText("fast", "那这个是 BUG 吗"))
	if got := sentTexts(fa); len(got) != 1 || got[0] != "不是 BUG。" {
		t.Fatalf("fast turn sends = %v want only the answer", got)
	}

	slow := &fakeAdapter{}
	m2 := NewManager(nil, nil, nil)
	rc := testRunningChannel(slow)
	release := make(chan struct{})
	m2.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		<-release
		return Reply{FinalSummary: "查完了，是缓存。"}, nil
	}
	scope := conversationTurnScope(rc.cfg.ProjectID, SceneC2C, "user1")
	m2.stillWorkingAfter = 20 * time.Millisecond
	slowIn := testInboundText("slow", "帮我查一下登录页为什么慢")
	stop := m2.sayStillWorking(context.Background(), rc, slowIn, scope)
	time.Sleep(80 * time.Millisecond)
	stop()
	close(release)

	if got := sentTexts(slow); len(got) != 0 {
		t.Fatalf("sayStillWorking must be a no-op; got %v", got)
	}
	// Stopping is idempotent.
	stop()
	if n := len(sentTexts(slow)); n != 0 {
		t.Fatalf("waiting notice must not appear: %d sends", n)
	}
}

// A full backlog must not swallow messages, and must not answer each rejected
// one either: the user hears about it once per cooldown.
func TestQueueFullHintIsSentOnceAndNotSilent(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	release := make(chan struct{})
	var once sync.Once
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		<-release
		return Reply{FinalSummary: "done"}, nil
	}
	rc := testRunningChannel(fa)

	go m.dispatch(context.Background(), rc, testInbound("head"))
	// Wait for the head turn to occupy the conversation.
	deadline := time.Now().Add(2 * time.Second)
	key := convKey(rc.cfg.ProjectID, SceneC2C, "user1")
	for {
		q := m.convQueueFor(key)
		q.mu.Lock()
		busy := q.busy
		q.mu.Unlock()
		if busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("head turn never started")
		}
		time.Sleep(2 * time.Millisecond)
	}
	// Fill the pending FIFO, then overflow it several times.
	for i := 0; i < convQueueDepth+5; i++ {
		m.dispatch(context.Background(), rc, testInbound(string(rune('a'+i%26))+"-fill"))
	}
	if n := countText(sentTexts(fa), busyHintText); n != 1 {
		t.Fatalf("busy hint sent %d times, want exactly one per cooldown", n)
	}
	once.Do(func() { close(release) })
}

func TestClaimsActiveWorkFinished(t *testing.T) {
	for _, s := range []string{
		"快模型和 worker 架构精简分析已经做完了。",
		"结论是弄完了，可以精简。",
		"已经查完了。",
	} {
		if !claimsActiveWorkFinished(s) {
			t.Fatalf("should flag finished claim: %q", s)
		}
	}
	for _, s := range []string{"还在方案报告页，大概还要一会儿。", "刚派下去。"} {
		if claimsActiveWorkFinished(s) {
			t.Fatalf("false finished claim: %q", s)
		}
	}
}

// TestPhrasePromptSharedFragmentsLock locks the rule-level dedup: one shared
// header for all five spoken prompts, three shared ack rule lines reused by
// the four acks only, and each event's distinctive semantics kept in place.
func TestPhrasePromptSharedFragmentsLock(t *testing.T) {
	if phrasePromptHeader == "" {
		t.Fatal("phrasePromptHeader must be defined once")
	}
	allFive := []string{
		statusWhileRunningPhrasePrompt,
		retryAckPhrasePrompt,
		fallthroughAckPhrasePrompt,
		dispatchAckPhrasePrompt,
		refineAckPhrasePrompt,
	}
	for i, p := range allFive {
		if !strings.HasPrefix(p, phrasePromptHeader) {
			t.Fatalf("prompt %d missing shared header prefix", i)
		}
		if strings.Count(p, phrasePromptHeader) != 1 {
			t.Fatalf("prompt %d should embed shared header exactly once", i)
		}
	}

	acks := []string{
		retryAckPhrasePrompt,
		fallthroughAckPhrasePrompt,
		dispatchAckPhrasePrompt,
		refineAckPhrasePrompt,
	}
	for _, rule := range []string{
		phraseAckRuleColloquial,
		phraseAckRuleNoInternal,
		phraseAckRuleOneLine,
	} {
		if rule == "" {
			t.Fatal("ack common rule constant must be non-empty")
		}
		for i, p := range acks {
			if strings.Count(p, rule) != 1 {
				t.Fatalf("ack %d must reuse common rule exactly once: %q", i, rule)
			}
		}
		if strings.Contains(statusWhileRunningPhrasePrompt, rule) {
			t.Fatalf("statusWhileRunning must not inherit ack rule %q", rule)
		}
	}
	if strings.Contains(statusWhileRunningPhrasePrompt, "规矩：") {
		t.Fatal("statusWhileRunning must keep its own body, not the ack「规矩」block")
	}

	mustContain := func(name, prompt string, needles ...string) {
		t.Helper()
		for _, n := range needles {
			if !strings.Contains(prompt, n) {
				t.Fatalf("%s missing event-specific %q:\n%s", name, n, prompt)
			}
		}
	}
	mustContain("statusWhileRunning", statusWhileRunningPhrasePrompt,
		"禁止说已经做完", "标题截断写法", "只输出要发出去的话。")
	mustContain("retry", retryAckPhrasePrompt,
		"正在重试", "已经重新跑过了", "不要复述任务标题或原要求")
	mustContain("fallthrough", fallthroughAckPhrasePrompt,
		"还要再查", "正在查/正在弄", "已经查完", "不要复述对方原话")
	mustContain("dispatch", dispatchAckPhrasePrompt,
		"刚把一件事派人", "正在做", "不要复述完整任务标题或原要求")
	mustContain("refine", refineAckPhrasePrompt,
		"按新重点继续", "不要复述完整任务标题；不要用书名号")
}

func TestOutcomeBriefForbidsHollowArchitectureConclusion(t *testing.T) {
	id := &models.TaskIdentity{ShortTitle: "快模型架构", OriginalRequirement: "看看能不能精简"}
	empty := outcomeBrief(id, TaskOutcome{Status: "completed"}, "zh-CN")
	for _, need := range []string{"没有留下可读结论", "禁止编造", "可以精简"} {
		if !strings.Contains(empty, need) {
			t.Fatalf("empty brief missing %q:\n%s", need, empty)
		}
	}
	withFacts := outcomeBrief(id, TaskOutcome{
		Status: "completed", ResultSummary: "Live 与 worker 可合并超时配置。",
	}, "zh-CN")
	if !strings.Contains(withFacts, "空结论") || !strings.Contains(withFacts, "合并超时") {
		t.Fatalf("facts brief = %s", withFacts)
	}
}

// Completed reflow must carry the run's findings, not a hollow "弄完了 / ask for details".
func TestCompletedOutcomeFallbackIncludesResultSummary(t *testing.T) {
	id := &models.TaskIdentity{ShortTitle: "直接检查 approving 仓库当前主干代码", Language: "zh-CN"}
	got := outcomeFallbackText(id, TaskOutcome{
		Status: "completed", ResultSummary: "主干错误处理覆盖了 fallthrough ack，Live 超时仍可能是 8s。",
	}, "zh-CN")
	if !strings.Contains(got, "fallthrough") || !strings.Contains(got, "8s") {
		t.Fatalf("fallback dropped findings: %q", got)
	}
	if strings.Contains(got, "直接检查") {
		t.Fatalf("findings path should not glue short title: %q", got)
	}
	broken := outcomeFallbackText(&models.TaskIdentity{
		ShortTitle: "调研 Approving 最近关于快模型和 wo", Language: "zh-CN",
	}, TaskOutcome{Status: "completed", ResultSummary: "分层合理，建议收敛接话出口。"}, "zh-CN")
	if strings.Contains(broken, "wo") {
		t.Fatalf("broken title leaked into completion: %q", broken)
	}
	for _, bad := range []string{"想看细节", "Ask me if you want the details"} {
		if strings.Contains(got, bad) {
			t.Fatalf("hollow deferral still present (%s): %q", bad, got)
		}
	}
	empty := outcomeFallbackText(id, TaskOutcome{Status: "completed"}, "zh-CN")
	if strings.Contains(empty, "想看细节") {
		t.Fatalf("empty fallback still defers details: %q", empty)
	}
	if !strings.Contains(empty, "可读结论") {
		t.Fatalf("empty fallback should admit missing summary: %q", empty)
	}
}

// A finished Run reports back to the conversation that asked for it, exactly
// once, and the task stops answering "still running" afterwards.
func TestTaskOutcomeReturnsToTheOriginConversation(t *testing.T) {
	fa := &fakeAdapter{}
	m, tasks := liveManager(t, fa)

	if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-abc123def456", ProjectID: "proj",
		UserID:     services.SyntheticQQUserID("u1"),
		ShortTitle: "登录页性能优化", Status: "running",
		OriginChannel: "qq", OriginScene: string(SceneC2C),
		OriginConversationID: "user1", OriginExternalUserID: "u1",
		Language:            "zh-CN",
		OriginalRequirement: "调研并实现登录页性能优化",
	}); err != nil {
		t.Fatal(err)
	}

	outcome := TaskOutcome{
		ProjectID: "proj", RunID: "run-abc123def456", Status: "completed",
		ResultSummary: "超时与重试已对齐。\n交付链接：https://github.com/org/repo/pull/9",
	}
	if err := m.ReflowTaskOutcome(context.Background(), outcome); err != nil {
		t.Fatalf("ReflowTaskOutcome: %v", err)
	}
	// A duplicate terminal event must not produce a second message.
	if err := m.ReflowTaskOutcome(context.Background(), outcome); err != nil {
		t.Fatalf("ReflowTaskOutcome (repeat): %v", err)
	}

	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("outcome sends = %v want exactly one conclusion", got)
	}
	if !strings.Contains(got[0], "超时与重试") || !strings.Contains(got[0], "pull/9") {
		t.Fatalf("conclusion dropped findings: %q", got[0])
	}
	if strings.Contains(got[0], "Approving") || ContainsInternalTerms(got[0]) {
		t.Fatalf("conclusion is not self-contained: %q", got[0])
	}
	// It went to the conversation that started the task, not the cron target.
	fa.mu.Lock()
	conv := fa.sent[0].ConversationID
	fa.mu.Unlock()
	if conv != "user1" {
		t.Fatalf("conclusion delivered to %q want the origin conversation", conv)
	}

	identity, err := tasks.IdentityForRun("run-abc123def456", "proj")
	if err != nil || identity == nil {
		t.Fatalf("identity after reflow = %+v err=%v", identity, err)
	}
	if !services.IsTerminalTaskStatus(identity.Status) {
		t.Fatalf("task status = %q, a finished task must not still read as running", identity.Status)
	}
	if !strings.Contains(identity.RecentContext, "pull/9") {
		t.Fatalf("completed digest not persisted for PR follow-up: %q", identity.RecentContext)
	}
}

// After a completed delivery, briefing / get_status must surface the digest so
// clarifying follow-ups answer from stored facts — not glossary definitions.
func TestCompletedDigestSurfacesForFollowup(t *testing.T) {
	fa := &fakeAdapter{}
	m, tasks := liveManager(t, fa)
	const link = "https://github.com/org/repo/pull/12"
	if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-follow01", ProjectID: "proj",
		UserID:     services.SyntheticQQUserID("u1"),
		ShortTitle: "对齐超时", Status: "completed",
		OriginChannel: "qq", OriginScene: string(SceneC2C),
		OriginConversationID: "user1", OriginExternalUserID: "u1",
		Language: "zh-CN", RecentContext: "关键发现已写入。\n交付链接：" + link,
	}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	var rc *runningChannel
	for _, c := range m.running {
		rc = c
		break
	}
	m.mu.Unlock()
	if rc == nil {
		t.Fatal("no running channel")
	}
	in := InboundMessage{UserID: "u1", ConversationID: "user1", Scene: SceneC2C}
	brief := m.buildDirectorContext(rc, in).render()
	for _, need := range []string{link, "结论摘要", "名词百科"} {
		if !strings.Contains(brief, need) {
			t.Fatalf("briefing missing %q:\n%s", need, brief)
		}
	}
	raw := m.runGetStatus(rc, in, "")
	if !strings.Contains(raw, link) || !strings.Contains(raw, "result_summary") {
		t.Fatalf("get_status missing digest: %s", raw)
	}
	if !strings.Contains(raw, "名词百科") {
		t.Fatalf("get_status missing delivery follow-up note: %s", raw)
	}
}

// A failed Run explains itself in terms the user can act on, without quoting
// the diagnostic that caused it.
func TestFailedTaskOutcomeExplainsWithoutDiagnostics(t *testing.T) {
	fa := &fakeAdapter{}
	m, tasks := liveManager(t, fa)
	if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-fail0001", ProjectID: "proj",
		UserID:     services.SyntheticQQUserID("u1"),
		ShortTitle: "结算页重构", Status: "running",
		OriginChannel: "qq", OriginScene: string(SceneC2C),
		OriginConversationID: "user1", OriginExternalUserID: "u1", Language: "zh-CN",
	}); err != nil {
		t.Fatal(err)
	}
	err := m.ReflowTaskOutcome(context.Background(), TaskOutcome{
		ProjectID: "proj", RunID: "run-fail0001", Status: "failed",
		FailureReason: "node build-1 exited: context deadline exceeded in sandbox",
	})
	if err != nil {
		t.Fatalf("ReflowTaskOutcome: %v", err)
	}
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want one explanation", got)
	}
	if !strings.Contains(got[0], "超时") {
		t.Fatalf("failure was not translated into a cause: %q", got[0])
	}
	if strings.Contains(got[0], "build-1") || ContainsInternalTerms(got[0]) {
		t.Fatalf("failure quoted the diagnostic: %q", got[0])
	}
	if !strings.Contains(got[0], "重试") || !strings.Contains(got[0], "搁置") {
		t.Fatalf("failure fallback should let the user choose next step: %q", got[0])
	}
}

// A restart loses the sandbox but must not lose the user's message in silence.
func TestRecoverInterruptedTurnsTellsTheUser(t *testing.T) {
	fa := &fakeAdapter{}
	m, tasks := liveManager(t, fa)
	if err := tasks.DB().Create(&models.PendingChannelTurn{
		ID: "turn:proj|m9", ProjectID: "proj", Channel: "qq",
		Scene: string(SceneC2C), ConversationID: "user1", ExternalUserID: "u1",
		MessageID: "m9", Language: "zh-CN", StartedAt: time.Now().Add(-time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}

	m.RecoverInterruptedTurns(context.Background())
	got := sentTexts(fa)
	if len(got) != 1 || got[0] != interruptedTurnText("zh-CN") {
		t.Fatalf("recovery sends = %v want one plain explanation", got)
	}

	// The record is cleared, so a second boot stays quiet.
	m.RecoverInterruptedTurns(context.Background())
	if n := len(sentTexts(fa)); n != 1 {
		t.Fatalf("recovery repeated itself: %d sends", n)
	}
}

// With two tasks live, a status question is exactly where the platform used to
// answer on the agent's behalf — from a status column, in one canned line. The
// question reaches the agent now, and the agent's own words are what go out.
func TestStatusQuestionWithParallelTasksReachesTheAgent(t *testing.T) {
	fa := &fakeAdapter{}
	m, tasks := liveManager(t, fa)
	for _, task := range []struct{ run, title string }{
		{"run-aaa111222333", "登录页性能优化"},
		{"run-bbb444555666", "结算页重构"},
	} {
		if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
			RunID: task.run, ProjectID: "proj",
			UserID:     services.SyntheticQQUserID("u1"),
			ShortTitle: task.title, Status: "running",
			OriginChannel: "qq", OriginScene: string(SceneC2C),
			OriginConversationID: "user1", OriginExternalUserID: "u1", Language: "zh-CN",
		}); err != nil {
			t.Fatal(err)
		}
	}

	const answer = "登录页 profiling 做完了，结算页那边还在跑。"
	turned := 0
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		turned++
		return Reply{FinalSummary: answer}, nil
	}
	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
		MessageID: "m1", Text: "登录页性能优化怎么样了",
	})
	if turned != 1 {
		t.Fatalf("a status question did not reach the agent (%d turns)", turned)
	}
	got := sentTexts(fa)
	if len(got) != 1 || got[0] != answer {
		t.Fatalf("status answer = %v want the agent's own words", got)
	}
}

// Synthesis lets a model phrase a background result, which means model output
// reaches the channel. Anything it drags along with it is scrubbed on the way
// out; the user still gets the conclusion.
func TestSynthesizedOutcomeIsScrubbedBeforeItLeaves(t *testing.T) {
	fa := &fakeAdapter{}
	m, tasks := liveManager(t, fa)
	if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-leak000001", ProjectID: "proj",
		UserID:     services.SyntheticQQUserID("u1"),
		ShortTitle: "登录页性能优化", Status: "running",
		OriginChannel: "qq", OriginScene: string(SceneC2C),
		OriginConversationID: "user1", OriginExternalUserID: "u1", Language: "zh-CN",
	}); err != nil {
		t.Fatal(err)
	}
	m.SetSynthesizer(func(context.Context, SynthesisRequest) (string, error) {
		return "tool_call gh pr view --json\n" +
			"[摘要] 登录页首屏从 3.2s 降到 1.1s，run-leak000001 在 sandbox 里跑完了。", nil
	})

	if err := m.ReflowTaskOutcome(context.Background(), TaskOutcome{
		ProjectID: "proj", RunID: "run-leak000001", Status: "completed",
	}); err != nil {
		t.Fatalf("ReflowTaskOutcome: %v", err)
	}
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want one conclusion", got)
	}
	if !strings.Contains(got[0], "3.2s 降到 1.1s") {
		t.Fatalf("scrubbing removed the conclusion: %q", got[0])
	}
	for _, leak := range []string{"tool_call", "[摘要]", "run-leak000001", "sandbox"} {
		if strings.Contains(got[0], leak) {
			t.Fatalf("synthesized text leaked %q: %q", leak, got[0])
		}
	}
}

// Short titles are how a user refers to a task, so they must be readable. A
// Run identifier is neither readable nor stable vocabulary for a conversation.
func TestShortTitlesNeverCarryRunIdentifiers(t *testing.T) {
	for _, in := range []string{
		"Run run-1ca1876f", "run-1ca1876f", "修复 Run run-1ca1876f / PR",
		"task-9f8e7d6c5b", "Run",
	} {
		got := services.SanitizeShortTitle(in)
		if ContainsInternalTerms(got) {
			t.Errorf("SanitizeShortTitle(%q) = %q still carries an identifier", in, got)
		}
		if strings.EqualFold(strings.TrimSpace(got), "run") {
			t.Errorf("SanitizeShortTitle(%q) = %q is not a title", in, got)
		}
	}
}

// bannedOutboundPhrases are process/receipt templates that must never appear
// in user-visible QQ outbound (Demo / clarified requirement).
var bannedOutboundPhrases = []string{
	"稍等，我看一下",
	"Give me a moment on this one",
	"已发送",
	"已通过 QQ 回复用户",
	"已通过QQ回复用户",
	"本回合已结束",
	"请前往 Approving",
	"已开始处理",
	"任务已启动",
	"收到，正在处理",
}

func assertNoBannedOutbound(t *testing.T, texts []string) {
	t.Helper()
	for _, text := range texts {
		for _, banned := range bannedOutboundPhrases {
			if strings.Contains(text, banned) {
				t.Fatalf("banned outbound phrase %q in %q (all=%v)", banned, text, texts)
			}
		}
		if strings.Contains(text, deprecatedSafeFinalNotice) {
			t.Fatalf("shell notice leaked: %v", texts)
		}
	}
}

// Cross-layer Demo scenarios: greeting / status / tool-then-reply / background
// start. Each asserts outbound count, order, and banned process copy.
func TestOutboundBoundaryFourScenarios(t *testing.T) {
	t.Run("greeting_one_natural_reply", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, _ := liveManager(t, fa)
		rc := testRunningChannel(fa)
		rc.cfg.ProjectID = "proj"
		const answer = "你好，我在。"
		m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
			if _, err := m.DeliverConversationReply(ctx, ConversationReply{
				ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
				UserID: in.UserID, Text: answer,
			}); err != nil {
				return Reply{}, err
			}
			body := answer + "\n已发送。\n稍等，我看一下。"
			return Reply{Text: body, FinalSummary: buildDeliverableFinalSummary(body)}, nil
		}
		m.handleInbound(context.Background(), rc, InboundMessage{
			Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
			MessageID: "s1", Text: "你好",
		})
		got := sentTexts(fa)
		if len(got) != 1 || got[0] != answer {
			t.Fatalf("greeting sends = %v want [%q]", got, answer)
		}
		assertNoBannedOutbound(t, got)
	})

	t.Run("status_query_one_contextual_reply", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, _ := liveManager(t, fa)
		rc := testRunningChannel(fa)
		rc.cfg.ProjectID = "proj"
		const answer = "登录页性能优化还在跑，首屏 profiling 做完了。"
		m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
			if _, err := m.DeliverConversationReply(ctx, ConversationReply{
				ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
				UserID: in.UserID, Text: answer,
			}); err != nil {
				return Reply{}, err
			}
			body := "已通过 QQ 回复用户。\n" + answer
			return Reply{Text: body, FinalSummary: buildDeliverableFinalSummary(body)}, nil
		}
		m.handleInbound(context.Background(), rc, InboundMessage{
			Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
			MessageID: "s2", Text: "什么进度了",
		})
		got := sentTexts(fa)
		if len(got) != 1 || got[0] != answer {
			t.Fatalf("status sends = %v want [%q]", got, answer)
		}
		assertNoBannedOutbound(t, got)
	})

	t.Run("tool_then_pm_reply_no_receipt_aside", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, _ := liveManager(t, fa)
		rc := testRunningChannel(fa)
		rc.cfg.ProjectID = "proj"
		const answer = "缓存没刷新，不是 BUG。"
		m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
			if _, err := m.DeliverConversationReply(ctx, ConversationReply{
				ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
				UserID: in.UserID, Text: answer,
			}); err != nil {
				return Reply{}, err
			}
			// Simulate tool call + pm_reply + model re-greeting + receipt aside.
			body := "tool_call lookup_status\n你好呀\n已发送。\n已通过 QQ 回复用户。\n" + answer
			return Reply{Text: body, FinalSummary: buildDeliverableFinalSummary(body)}, nil
		}
		m.handleInbound(context.Background(), rc, InboundMessage{
			Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
			MessageID: "s3", Text: "那这个是 BUG 吗",
		})
		got := sentTexts(fa)
		if len(got) != 1 || got[0] != answer {
			t.Fatalf("tool-then-reply sends = %v want [%q]", got, answer)
		}
		assertNoBannedOutbound(t, got)
		for _, text := range got {
			if strings.Count(text, "你好") > 0 && text != answer {
				t.Fatalf("duplicate greeting leaked: %v", got)
			}
		}
	})

	t.Run("background_start_one_acceptance_ack", func(t *testing.T) {
		fa := &fakeAdapter{}
		m, _ := liveManager(t, fa)
		rc := testRunningChannel(fa)
		rc.cfg.ProjectID = "proj"
		m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
			if _, err := m.SendRunAcceptanceAck(ctx, RunAcceptanceAck{
				ProjectID: "proj", RunID: "run-bg0011223344",
				Scene: in.Scene, ConversationID: in.ConversationID,
				UserID: in.UserID, ShortTitle: "登录页性能优化", Language: "zh-CN",
			}); err != nil {
				return Reply{}, err
			}
			// Model also tries to confirm + fixed ack; must not stack.
			body := "已开始处理。\n任务已启动。\n稍等，我看一下。\n好的我去弄登录页。"
			return Reply{Text: body, FinalSummary: buildDeliverableFinalSummary(body)}, nil
		}
		m.handleInbound(context.Background(), rc, InboundMessage{
			Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
			MessageID: "s4", Text: "帮我调研并实现登录页性能优化",
		})
		got := sentTexts(fa)
		if len(got) != 1 {
			t.Fatalf("background start sends = %v want exactly 1 acceptance ack", got)
		}
		want := runAcceptanceText("登录页性能优化", "zh-CN")
		if got[0] != want {
			t.Fatalf("acceptance = %q want %q", got[0], want)
		}
		assertNoBannedOutbound(t, got)
	})
}

// FinalSummary-only path (no pm_reply) still delivers one substantive answer —
// #161 reachability — without process templates.
func TestFinalSummaryFallbackAloneSendsOneAnswer(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	const answer = "你在看什么？我这边刚看完登录页 profiling。"
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		body := "内部推理：先答\n" + answer
		return Reply{Text: body, FinalSummary: buildDeliverableFinalSummary(body)}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInboundText("fb1", "你在看什么"))
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("fallback sends = %v want 1", got)
	}
	if !strings.Contains(got[0], "登录页") {
		t.Fatalf("fallback lost answer: %v", got)
	}
	assertNoBannedOutbound(t, got)
}

// Receipt-only assistant body with no prior pm_reply → 0 outbound (not shell).
func TestReceiptOnlyBodySendsNothing(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		body := "已发送。\n已通过 QQ 回复用户。"
		return Reply{Text: body, FinalSummary: buildDeliverableFinalSummary(body)}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInboundText("r0", "你好"))
	if got := sentTexts(fa); len(got) != 0 {
		t.Fatalf("receipt-only sends = %v want 0", got)
	}
}

// live_first_sentence progress must not reach the adapter even if a bridge
// regression re-emits it.
func TestLiveFirstSentenceProgressIsBlocked(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFuncWithProgress = func(ctx context.Context, _ ResolvedChannel, _ InboundMessage, onProgress func(ProgressEvent)) (Reply, error) {
		onProgress(ProgressEvent{
			Kind: ProgressMilestone, Summary: "你好", Stage: "你好",
			Sendable: true, Reason: "live_first_sentence", At: time.Now(),
		})
		return Reply{FinalSummary: "你好，有什么我可以帮你的？"}, nil
	}
	m.dispatch(context.Background(), testRunningChannel(fa), testInboundText("opener", "你好"))
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want only the final answer", got)
	}
	if got[0] != "你好，有什么我可以帮你的？" {
		t.Fatalf("outbound = %q", got[0])
	}
	assertNoBannedOutbound(t, got)
}
