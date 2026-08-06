package channels

import (
	"context"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

func TestInboundOrchestrationAmbiguityAndContinuation(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	tasks := services.NewTaskContextService(db)
	risk := services.NewRiskConfirmationService(db)
	m.SetTaskContextService(tasks)
	m.SetRiskConfirmationService(risk)

	var cancelled []string
	m.SetRiskActionExecutor(func(projectID, runID, action string, meta map[string]string) error {
		cancelled = append(cancelled, runID+":"+action)
		return nil
	})

	turned := 0
	m.handleFunc = func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
		turned++
		return Reply{FinalSummary: "agent-handled:" + in.Text}, nil
	}

	projectID := "proj"
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
	in := InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m1", Text: "登录页怎么样了",
	}
	m.runTurn(context.Background(), rc, in, false)
	got := sentTexts(fa)
	if turned != 0 {
		t.Fatalf("ambiguous status query must not start a PM turn, turned=%d", turned)
	}
	if len(got) != 1 || !strings.Contains(got[0], "匹配到多个任务") {
		t.Fatalf("expected ambiguity clarification, got %v", got)
	}

	fa.mu.Lock()
	fa.sent = nil
	fa.mu.Unlock()
	m.runTurn(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m2", Text: "登录页性能优化",
	}, false)
	got = sentTexts(fa)
	if turned != 0 {
		t.Fatalf("short-title selection should answer without PM turn, turned=%d", turned)
	}
	if len(got) != 1 || !strings.Contains(got[0], "登录页性能优化") {
		t.Fatalf("short-title reply = %v", got)
	}

	fa.mu.Lock()
	fa.sent = nil
	fa.mu.Unlock()
	m.runTurn(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m3", Text: "那继续吧",
	}, false)
	if turned != 1 {
		t.Fatalf("continuation should enter PM turn with focus, turned=%d", turned)
	}
	got = sentTexts(fa)
	if len(got) != 1 || !strings.Contains(got[0], "agent-handled") {
		t.Fatalf("continuation final = %v", got)
	}

	fa.mu.Lock()
	fa.sent = nil
	fa.mu.Unlock()
	m.runTurn(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m4", Text: "取消登录页性能优化",
	}, false)
	got = sentTexts(fa)
	if len(cancelled) != 0 {
		t.Fatalf("high-risk action executed before confirmation: %v", cancelled)
	}
	if len(got) != 1 || !strings.Contains(got[0], "高风险确认") {
		t.Fatalf("expected confirmation prompt, got %v", got)
	}

	fa.mu.Lock()
	fa.sent = nil
	fa.mu.Unlock()
	m.runTurn(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m5", Text: "确认",
	}, false)
	if len(cancelled) != 1 || cancelled[0] != "r1:cancel_run" {
		t.Fatalf("confirmed cancel = %v", cancelled)
	}
}

func TestInboundOrchestrationGenericTitleAndGateConfirmation(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	tasks := services.NewTaskContextService(db)
	risk := services.NewRiskConfirmationService(db)
	m.SetTaskContextService(tasks)
	m.SetRiskConfirmationService(risk)

	const projectID = "proj"
	if err := db.Create(&models.Run{ID: "r-gate", Status: "waiting_human"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "r-gate", ProjectID: projectID, UserID: services.SyntheticQQUserID("u1"),
		ShortTitle: "结算页性能", Status: "waiting_human",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Gate{
		RunID: "r-gate", NodeID: "review-gate", Iteration: 1,
		Title: "上线审批", Resolved: false,
	}).Error; err != nil {
		t.Fatal(err)
	}

	turned := 0
	m.handleFunc = func(context.Context, ResolvedChannel, InboundMessage) (Reply, error) {
		turned++
		return Reply{}, nil
	}
	rc := &runningChannel{
		cfg:     models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: projectID},
		adapter: fa,
	}
	m.runTurn(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m1", Text: "结算页性能",
	}, false)
	if turned != 0 {
		t.Fatalf("generic exact short title should be handled before PM turn, turned=%d", turned)
	}
	if got := sentTexts(fa); len(got) != 1 || !strings.Contains(got[0], "结算页性能") {
		t.Fatalf("generic title reply = %v", got)
	}

	fa.mu.Lock()
	fa.sent = nil
	fa.mu.Unlock()
	var executedAction string
	var executedMeta map[string]string
	m.SetRiskActionExecutor(func(projectID, runID, action string, meta map[string]string) error {
		executedAction, executedMeta = action, meta
		return nil
	})
	m.runTurn(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m2", Text: "批准结算页性能",
	}, false)
	pending, err := risk.LatestPending(services.SyntheticQQUserID("u1"), projectID)
	if err != nil || pending == nil {
		t.Fatalf("pending gate ticket = %+v err=%v", pending, err)
	}
	if pending.Action != "resume_gate:review-gate:approve" {
		t.Fatalf("gate ticket action = %q", pending.Action)
	}
	if executedAction != "" {
		t.Fatalf("gate executed before confirmation: %s", executedAction)
	}

	m.runTurn(context.Background(), rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "c1", UserID: "u1",
		MessageID: "m3", Text: "确认",
	}, false)
	if executedAction != "resume_gate" ||
		executedMeta["nodeId"] != "review-gate" || executedMeta["gateAction"] != "approve" {
		t.Fatalf("confirmed gate action=%q meta=%v", executedAction, executedMeta)
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
}
