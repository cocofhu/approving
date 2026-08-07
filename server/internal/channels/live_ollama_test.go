package channels

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cocofhu/approving/internal/liveagent"
)

// Against a real Ollama (or any OpenAI-compatible) endpoint, with the sandbox
// mocked. Skipped unless configured:
//
//	LIVE_TEST_BASE_URL=http://192.168.2.20:11434/v1 \
//	LIVE_TEST_MODEL=genesis-hermes-v7:latest \
//	go test ./internal/channels/ -run TestOllamaDirector -v -count=1
//
// These catch what the fakeLive suite cannot: a real model that thinks before
// it speaks, fills tool args in its own words, and may spend most of a small
// token budget on reasoning.
func ollamaDirector(t *testing.T) (*gptLive, *liveagent.Client) {
	t.Helper()
	base := os.Getenv("LIVE_TEST_BASE_URL")
	model := os.Getenv("LIVE_TEST_MODEL")
	if base == "" || model == "" {
		t.Skip("set LIVE_TEST_BASE_URL and LIVE_TEST_MODEL to run against Ollama")
	}
	c := liveagent.New()
	c.SetLiveEndpoint(base, os.Getenv("LIVE_TEST_API_KEY"), model, 3*time.Minute)
	g := newGPTLive(t)
	g.m.SetLiveModel(c)
	return g, c
}

func TestOllamaDirectorAnswersChitchatWithoutSandbox(t *testing.T) {
	g, _ := ollamaDirector(t)

	start := time.Now()
	g.say("m1", "你好，在吗")
	t.Logf("took %v", time.Since(start))

	if len(g.agent) != 0 {
		t.Fatalf("a greeting opened the sandbox: %+v", g.agent)
	}
	got := sentTexts(g.fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want exactly one reply", got)
	}
	t.Logf("reply: %s", got[0])
	if utf8.RuneCountInString(got[0]) < 2 {
		t.Fatalf("empty reply: %q", got[0])
	}
	assertNoReasoningLeak(t, got[0])
	assertNoBannedOutbound(t, got)
}

func TestOllamaDirectorAcksThenSummarisesSandboxWork(t *testing.T) {
	g, _ := ollamaDirector(t)

	// Long technical dump — the shape that used to be chopped at 240 runes
	// mid-sentence before the director could phrase it.
	long := "你说得对，这是代码缺口不是过滤问题。" +
		"1）项目审计日志没有「模型调用」事件类型，只记配置/Run/门禁/MCP/投递；" +
		"2）工作流/PM 模型只靠 prompt_done.usage 进 Token 统计，不进审计；" +
		"3）快模型调用写进了 LiveDecisionSample，但 Model 字段从未赋值，模型名是空的；" +
		"4）快模型做汇报改写的调用没有单独记，响应里的 token usage 也没解析。" +
		"因此目前审计页看不到快模型调用记录，要补事件类型并把样本/usage 接到审计出口。" +
		strings.Repeat("（补充背景：此前几轮路由都靠禁词表打补丁。）", 8)

	g.m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		g.agent = append(g.agent, in)
		// Prefer pm_reply capture path: that is what production agents use.
		if _, err := g.m.DeliverConversationReply(ctx, ConversationReply{
			ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
			UserID: in.UserID, Text: long,
		}); err != nil {
			t.Fatalf("pm_reply failed: %v", err)
		}
		return Reply{}, nil
	}

	start := time.Now()
	g.say("m1", "审计日志里似乎看不到快模型的调用记录，帮我查下根因")
	t.Logf("took %v", time.Since(start))

	if len(g.agent) != 1 {
		t.Fatalf("work was not dispatched to the sandbox: agent=%v", g.agent)
	}
	if g.agent[0].Dispatch == nil || strings.TrimSpace(g.agent[0].Dispatch.Brief) == "" {
		t.Fatalf("dispatch carried no brief: %+v", g.agent[0].Dispatch)
	}
	t.Logf("brief: %s", g.agent[0].Dispatch.Brief)
	t.Logf("title: %s", g.agent[0].Dispatch.ShortTitle)

	got := sentTexts(g.fa)
	if len(got) < 2 {
		t.Fatalf("sends = %v want acknowledgement then summarised conclusion", got)
	}
	ack, final := got[0], got[len(got)-1]
	t.Logf("ack: %s", ack)
	t.Logf("final: %s", final)

	if strings.TrimSpace(ack) == "" {
		t.Fatal("no acknowledgement before sandbox work")
	}
	for _, banned := range []string{"稍等，我看一下", "好的我看一下"} {
		if ack == banned {
			t.Fatalf("fixed-template ack came back: %q", ack)
		}
	}
	if utf8.RuneCountInString(final) < 8 {
		t.Fatalf("empty conclusion: %q", final)
	}
	// Must not be the unfinished 240-rune chop from the previous bug.
	if strings.HasSuffix(strings.TrimRight(final, "…."), "审") {
		t.Fatalf("mid-sentence truncation came back: %q", final)
	}
	assertNoReasoningLeak(t, ack)
	assertNoReasoningLeak(t, final)
	// Hard contract after the timeout fix: the director must phrase, not paste.
	if final == long {
		t.Fatalf("director degraded to the full agent dump instead of summarising")
	}
	if utf8.RuneCountInString(final) > 800 {
		t.Fatalf("conclusion still too long for IM (%d runes): %s", utf8.RuneCountInString(final), final)
	}
	assertNoBannedOutbound(t, got)
}

func assertNoReasoningLeak(t *testing.T, text string) {
	t.Helper()
	for _, leak := range []string{"<think", "</think>", "<reasoning", "</reasoning>", "<thinking", "</thinking>"} {
		if strings.Contains(strings.ToLower(text), leak) {
			t.Fatalf("reasoning leaked into user-visible text (%q): %s", leak, text)
		}
	}
}

func TestOllamaDirectorReportsStatusFromLedger(t *testing.T) {
	g, _ := ollamaDirector(t)
	identity := g.seedTask("run-audit", "审计日志缺口")
	g.m.noteWorkProgress("proj", "run-audit", "正在查审计事件类型", false)

	start := time.Now()
	g.say("m1", "好了没")
	t.Logf("took %v task=%s", time.Since(start), identity.ID)

	if len(g.agent) != 0 {
		t.Fatalf("a progress question opened a sandbox: %+v", g.agent)
	}
	got := sentTexts(g.fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v want one status reply", got)
	}
	t.Logf("status: %s", got[0])
	// Soft check: should reflect the ledger somehow. Models vary; if it
	// fabricates "done", that is a regression worth failing.
	lower := got[0]
	for _, bad := range []string{"已经修好了", "已经完成了", "弄完了"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("invented a finished status: %q", got[0])
		}
	}
}
