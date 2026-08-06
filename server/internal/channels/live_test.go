package channels

import (
	"context"
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

// A question is a question. The router must not turn one into a Run, and it
// must not read a complaint as a command — both were real failures: 「那这个是
// BUG 吗」 opened a background task, and 「不要这样啊」 asked to confirm a cancel.
func TestClassifyIntentKeepsConversationsConversational(t *testing.T) {
	cases := []struct {
		text           string
		pendingRisk    bool
		taskResolvable bool
		want           LiveIntent
	}{
		{text: "那这个是 BUG 吗", taskResolvable: true, want: IntentConversation},
		{text: "不要这样啊", taskResolvable: true, want: IntentConversation},
		{text: "没有实现啊，怎么回事", taskResolvable: true, want: IntentConversation},
		{text: "我不想取消", taskResolvable: true, want: IntentConversation},
		{text: "帮我调研并实现登录页性能优化", taskResolvable: false, want: IntentConversation},
		{text: "现在什么进展了", taskResolvable: true, want: IntentTaskQuery},
		{text: "现在什么进展了", taskResolvable: false, want: IntentConversation},
		{text: "取消登录页性能优化", taskResolvable: true, want: IntentTaskControl},
		{text: "改成只优化首屏", taskResolvable: true, want: IntentTaskControl},
		{text: "确认", pendingRisk: true, taskResolvable: true, want: IntentClarificationReply},
	}
	for _, c := range cases {
		got := classifyIntent(c.text, c.pendingRisk, c.taskResolvable)
		if got != c.want {
			t.Errorf("classifyIntent(%q, risk=%v, resolvable=%v) = %q want %q",
				c.text, c.pendingRisk, c.taskResolvable, got, c.want)
		}
	}
}

// A destructive action needs the user to have asked for it, in those words, at
// the front of the message.
func TestDetectHighRiskIntentRequiresAnExplicitCommand(t *testing.T) {
	for _, text := range []string{
		"取消登录页性能优化", "停止任务", "批准结算页性能", "delete task foo",
	} {
		if action, _ := detectHighRiskIntent(text); action == "" {
			t.Errorf("detectHighRiskIntent(%q) found no action", text)
		}
	}
	for _, text := range []string{
		"不要这样啊", "我不想取消这个", "为什么会取消", "这个功能没实现，不对吧",
		"顺便问一下批准流程是怎么走的",
	} {
		if action, _ := detectHighRiskIntent(text); action != "" {
			t.Errorf("detectHighRiskIntent(%q) = %q, commentary must not authorize anything", text, action)
		}
	}
}

// The outbound vocabulary guard, mirroring the web client's userFacingCopy
// test: nothing the platform says to a user may mention how the platform is
// built. This covers the static copy; ScrubInternalTerms covers text a model
// composed.
func TestOutboundCopyNeverExposesInternals(t *testing.T) {
	copies := []string{
		busyHintText,
		turnHandoffText("zh-CN"), turnHandoffText("en"),
		interruptedTurnText("zh-CN"), interruptedTurnText("en"),
		runAcceptanceText("登录页性能", "zh-CN"), runAcceptanceText("Login perf", "en"),
		taskStatusLabel("completed", "zh-CN"), taskStatusLabel("failed", "en"),
		soloTaskStatusText("active", "zh-CN"), soloTaskStatusText("cancelled", "en"),
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

// An answer the agent submitted through pm_reply is the turn's answer. The
// wrap-up must not append a second message on top of it.
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

// A turn that overruns is work that belongs in the background. The user is told
// that, once, instead of being held on the line or handed an error.
func TestForegroundTurnTimeoutReadsAsAHandoff(t *testing.T) {
	fa := &fakeAdapter{}
	m := NewManager(nil, nil, nil)
	m.handleFunc = func(ctx context.Context, _ ResolvedChannel, _ InboundMessage) (Reply, error) {
		<-ctx.Done()
		return Reply{}, ctx.Err()
	}
	rc := testRunningChannel(fa)
	rc.cfg.TurnTimeoutSeconds = 1

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.dispatch(context.Background(), rc, testInboundText("slow", "帮我把整个仓库重构一遍"))
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a foreground turn held the conversation past its budget")
	}

	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want one handoff message", got)
	}
	if got[0] != turnHandoffText("zh-CN") {
		t.Fatalf("timeout message = %q want the background handoff wording", got[0])
	}
	if strings.Contains(got[0], "失败") || strings.Contains(got[0], "错误") {
		t.Fatalf("timeout was reported as a failure: %q", got[0])
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

	outcome := TaskOutcome{ProjectID: "proj", RunID: "run-abc123def456", Status: "completed"}
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
	if !strings.Contains(got[0], "登录页性能优化") {
		t.Fatalf("conclusion does not say which task it is about: %q", got[0])
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

// Two live tasks make a bare "how's that one going?" ambiguous; the answer must
// name the task rather than guess silently.
func TestParallelTasksAnswerByShortTitle(t *testing.T) {
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

	turned := 0
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		turned++
		return Reply{}, nil
	}
	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
		MessageID: "m1", Text: "登录页性能优化怎么样了",
	})
	if turned != 0 {
		t.Fatalf("a status question opened a sandbox turn (%d)", turned)
	}
	got := sentTexts(fa)
	if len(got) != 1 || !strings.Contains(got[0], "登录页性能优化") {
		t.Fatalf("status answer = %v want the named task", got)
	}
	if strings.Contains(got[0], "结算页重构") {
		t.Fatalf("status answer mixed in the other task: %q", got[0])
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
