package channels

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

// The platform used to answer these itself, from a keyword scan and a scored
// title search. Two live tasks sharing a prefix made every one of them
// ambiguous, so 「登录页怎么样了」 and 「两个都修一下」 came back as a numbered menu
// that no answer could escape. Every message that is about work now reaches
// something that can read the conversation, and the menu is gone.
func TestTaskAddressingReachesTheAgentInsteadOfAMenu(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	tasks := services.NewTaskContextService(db)
	m.SetTaskContextService(tasks)
	m.SetRiskConfirmationService(services.NewRiskConfirmationService(db))

	var seen []string
	m.handleFunc = func(_ context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		seen = append(seen, in.Text)
		return Reply{FinalSummary: "agent-handled"}, nil
	}

	const projectID = "proj"
	for _, spec := range []struct{ run, title string }{
		{"r1", "登录页性能优化"},
		{"r2", "登录页文案整改"},
	} {
		if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
			RunID: spec.run, ProjectID: projectID, UserID: services.SyntheticQQUserID("u1"),
			ShortTitle: spec.title, Status: "running",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Precondition: these are exactly the conditions the old resolver called
	// ambiguous, so this test would have produced a menu before.
	if cands, err := tasks.Search(services.TaskScope{
		ProjectID: projectID, UserID: services.SyntheticQQUserID("u1"),
		Channel: "qq", ConversationID: "c1",
	}, "登录页"); err != nil || len(cands) != 2 {
		t.Fatalf("precondition search = %d err=%v", len(cands), err)
	}

	rc := &runningChannel{
		cfg:     models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: projectID},
		adapter: fa,
	}
	for i, text := range []string{
		"登录页怎么样了", "两个都修复下", "登录页性能优化", "那继续吧",
		"取消登录页性能优化", "什么进度了", "改成只优化首屏",
	} {
		fa.mu.Lock()
		fa.sent = nil
		fa.mu.Unlock()
		m.handleInbound(context.Background(), rc, InboundMessage{
			Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
			MessageID: "m" + strconv.Itoa(i), Text: text,
		})
		if len(seen) != i+1 || seen[i] != text {
			t.Fatalf("%q did not reach the agent; agent saw %v", text, seen)
		}
		got := sentTexts(fa)
		if len(got) != 1 || got[0] != "agent-handled" {
			t.Fatalf("%q outbound = %v want only the agent's answer", text, got)
		}
		for _, banned := range []string{"匹配到多个任务", "Several tasks match", "请回复序号", "没有找到匹配的任务"} {
			if strings.Contains(got[0], banned) {
				t.Fatalf("%q got a platform menu: %q", text, got[0])
			}
		}
	}
}

// A confirmation the platform already asked for is the one thing it still
// settles itself. The ticket comes from the agent's guarded write, so the reply
// only has to be recognised as yes or no — and until it is, nothing executes.
func TestPendingConfirmationIsSettledWithoutAModel(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	tasks := services.NewTaskContextService(db)
	risk := services.NewRiskConfirmationService(db)
	m.SetTaskContextService(tasks)
	m.SetRiskConfirmationService(risk)

	const projectID = "proj"
	if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "r-gate", ProjectID: projectID, UserID: services.SyntheticQQUserID("u1"),
		ShortTitle: "结算页性能", Status: "waiting_human",
	}); err != nil {
		t.Fatal(err)
	}

	var executedRun, executedAction string
	var executedMeta map[string]string
	m.SetRiskActionExecutor(func(_, runID, action string, meta map[string]string) error {
		executedRun, executedAction, executedMeta = runID, action, meta
		return nil
	})
	turned := 0
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		turned++
		return Reply{FinalSummary: "agent-handled"}, nil
	}
	rc := &runningChannel{
		cfg:     models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: projectID},
		adapter: fa,
	}

	// Asking to approve is a request, not an authorization: it goes to the
	// agent, which raises the ticket through the write it guards.
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m1", Text: "批准结算页性能",
	})
	if turned != 1 {
		t.Fatalf("a destructive-sounding request must reach the agent, turned=%d", turned)
	}
	if executedAction != "" {
		t.Fatalf("action executed with no ticket at all: %s", executedAction)
	}

	ticket, err := risk.CreateTicket(services.RiskTicketInput{
		ProjectID: projectID, UserID: services.SyntheticQQUserID("u1"),
		RunID: "r-gate", Action: "resume_gate:review-gate:approve",
		ShortTitle: "结算页性能", Language: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The platform asked the question; only then is 「确认」 an answer to it.
	if err := risk.MarkPrompted(ticket.ID); err != nil {
		t.Fatal(err)
	}

	fa.mu.Lock()
	fa.sent = nil
	fa.mu.Unlock()
	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m2", Text: "确认",
	})
	if turned != 1 {
		t.Fatalf("settling a ticket must not open an agent turn, turned=%d", turned)
	}
	if executedRun != "r-gate" || executedAction != "resume_gate" ||
		executedMeta["nodeId"] != "review-gate" || executedMeta["gateAction"] != "approve" {
		t.Fatalf("confirmed run=%q action=%q meta=%v", executedRun, executedAction, executedMeta)
	}
	if got := sentTexts(fa); len(got) != 1 || ContainsInternalTerms(got[0]) {
		t.Fatalf("settlement outbound = %v", got)
	}
}

// Confirming one task must never destroy another.
//
// This shipped. The agent asked to cancel task A and the user hesitated; the
// agent then asked to cancel task B and that question was suppressed on its way
// out. The user's 「确认」 — the only question they had ever seen was A's — was
// matched against the newest pending ticket, which was B, and B was cancelled.
// The giveaway was the reply naming a task the user had never been asked about.
// A ticket whose question never reached the user is now not a candidate.
func TestConfirmationSettlesTheTaskTheUserWasActuallyAskedAbout(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	risk := services.NewRiskConfirmationService(db)
	m.SetRiskConfirmationService(risk)
	m.SetTaskContextService(services.NewTaskContextService(db))

	const projectID = "proj"
	var cancelled []string
	m.SetRiskActionExecutor(func(_, runID, _ string, _ map[string]string) error {
		cancelled = append(cancelled, runID)
		return nil
	})
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		return Reply{}, nil
	}
	rc := &runningChannel{
		cfg:     models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: projectID},
		adapter: fa,
	}

	newTicket := func(runID, title string) *models.RiskConfirmationTicket {
		t.Helper()
		ticket, err := risk.CreateTicket(services.RiskTicketInput{
			ProjectID: projectID, UserID: services.SyntheticQQUserID("u1"),
			RunID: runID, Action: "cancel_run", ShortTitle: title, Language: "zh-CN",
		})
		if err != nil {
			t.Fatal(err)
		}
		return ticket
	}

	asked := newTicket("run-a", "检查并修复 QQ 两项问题")
	if err := risk.MarkPrompted(asked.ID); err != nil {
		t.Fatal(err)
	}
	// Newer, and never delivered: the user has no idea it exists.
	newTicket("run-b", "直接检查 approving 仓库当前主干代码")

	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m-confirm", Text: "确认",
	})

	if len(cancelled) != 1 || cancelled[0] != "run-a" {
		t.Fatalf("cancelled = %v want only the task the user was asked about", cancelled)
	}
	got := sentTexts(fa)
	if len(got) != 1 || !strings.Contains(got[0], "检查并修复 QQ 两项问题") {
		t.Fatalf("reply = %v must name the task that was confirmed", got)
	}
	if strings.Contains(got[0], "主干代码") {
		t.Fatalf("reply named a task the user never authorized: %q", got[0])
	}
}

// A question that never reached the user cannot be answered by 「确认」, so the
// message goes to the agent instead — nothing is executed on a guess.
func TestConfirmationWithNoDeliveredQuestionExecutesNothing(t *testing.T) {
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
		return Reply{}, nil
	}
	rc := &runningChannel{
		cfg:     models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: "proj"},
		adapter: fa,
	}
	if _, err := risk.CreateTicket(services.RiskTicketInput{
		ProjectID: "proj", UserID: services.SyntheticQQUserID("u1"),
		RunID: "run-unseen", Action: "cancel_run", ShortTitle: "没问出去的那个", Language: "zh-CN",
	}); err != nil {
		t.Fatal(err)
	}

	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m-blind", Text: "确认",
	})
	if executed != 0 {
		t.Fatalf("a question the user never saw authorized %d action(s)", executed)
	}
	if turned != 1 {
		t.Fatalf("the message should reach the agent instead, turned=%d", turned)
	}
}

// The reply is rendered after the action, not before. Rendering it first read
// the task as still running and produced 「已经取消了。现在是 running」 — two
// contradictory claims about the same task in one message.
func TestCancelReplyDoesNotReportTheStatusFromBeforeTheCancel(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	risk := services.NewRiskConfirmationService(db)
	m.SetRiskConfirmationService(risk)
	m.SetTaskContextService(services.NewTaskContextService(db))

	if err := db.Create(&models.Run{ID: "run-live", Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	m.SetRiskActionExecutor(func(_, runID, _ string, _ map[string]string) error {
		return db.Model(&models.Run{}).Where("id = ?", runID).
			Update("status", "cancelled").Error
	})
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		return Reply{}, nil
	}
	rc := &runningChannel{
		cfg:     models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: "proj"},
		adapter: fa,
	}
	ticket, err := risk.CreateTicket(services.RiskTicketInput{
		ProjectID: "proj", UserID: services.SyntheticQQUserID("u1"),
		RunID: "run-live", Action: "cancel_run", ShortTitle: "登录页性能", Language: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := risk.MarkPrompted(ticket.ID); err != nil {
		t.Fatal(err)
	}

	m.handleInbound(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m-cancel", Text: "确认",
	})
	got := sentTexts(fa)
	if len(got) != 1 {
		t.Fatalf("outbound = %v want one settlement reply", got)
	}
	if strings.Contains(got[0], "running") {
		t.Fatalf("reply reports the pre-cancel status: %q", got[0])
	}
	if !strings.Contains(got[0], "取消") {
		t.Fatalf("reply does not say what happened: %q", got[0])
	}
}

func TestExtractStructuredFinalSummary(t *testing.T) {
	got := extractStructuredFinalSummary("推理一堆\n[摘要] 已完成登录页性能优化\n其它")
	if got != "已完成登录页性能优化" {
		t.Fatalf("summary = %q", got)
	}
	if extractStructuredFinalSummary("只有普通正文") != "" {
		t.Fatal("unmarked text must not become FinalSummary via marker extraction alone")
	}
}

func TestBuildDeliverableFinalSummaryConversationalFallback(t *testing.T) {
	marked := buildDeliverableFinalSummary("内部推理：先想清楚\n[摘要] 已修好延迟\n其它")
	if marked != "已修好延迟" {
		t.Fatalf("marker path = %q", marked)
	}

	plain := buildDeliverableFinalSummary("抱歉回复慢了，也认同质量需要改进。我们会优先排查延迟与答复质量。")
	if plain == "" || !strings.Contains(plain, "抱歉回复慢了") {
		t.Fatalf("conversational fallback missing answer: %q", plain)
	}
	if plain == deprecatedSafeFinalNotice || strings.Contains(plain, "本回合已结束") {
		t.Fatalf("fallback must not be shell notice: %q", plain)
	}

	noisy := buildDeliverableFinalSummary("tool_call foo\n内部推理：密钥 sk-secret\n[进度] 读文件中\n对用户可见的答复在这里")
	if !strings.Contains(noisy, "对用户可见的答复在这里") {
		t.Fatalf("expected user-visible line kept: %q", noisy)
	}
	if strings.Contains(noisy, "tool_call") || strings.Contains(noisy, "内部推理") ||
		strings.Contains(noisy, "sk-secret") || strings.Contains(noisy, "读文件中") {
		t.Fatalf("noise leaked into summary: %q", noisy)
	}

	if buildDeliverableFinalSummary("tool_call x\nthinking: y") != "" {
		t.Fatal("noise-only body must yield empty FinalSummary")
	}

	receiptOnly := buildDeliverableFinalSummary("已发送。\n已通过 QQ 回复用户。\n稍等，我看一下。")
	if receiptOnly != "" {
		t.Fatalf("receipt/process-only body must yield empty FinalSummary, got %q", receiptOnly)
	}
	mixed := buildDeliverableFinalSummary("你好，我在。\n已发送。")
	if !strings.Contains(mixed, "你好") || strings.Contains(mixed, "已发送") {
		t.Fatalf("mixed body should keep answer and drop receipt: %q", mixed)
	}
}
