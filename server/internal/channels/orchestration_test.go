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
		cfg: models.ChannelConfig{ID: "c1", Type: models.ChannelTypeQQ, ProjectID: projectID},
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

func TestExtractStructuredFinalSummary(t *testing.T) {
	got := extractStructuredFinalSummary("推理一堆\n[摘要] 已完成登录页性能优化\n其它")
	if got != "已完成登录页性能优化" {
		t.Fatalf("summary = %q", got)
	}
	if extractStructuredFinalSummary("只有普通正文") != "" {
		t.Fatal("unmarked text must not become FinalSummary")
	}
}
