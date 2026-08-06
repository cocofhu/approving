package liveagent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests talk to a real OpenAI-compatible endpoint instead of a stub, and
// are skipped unless one is configured:
//
//	LIVE_TEST_BASE_URL=http://host:11434/v1 \
//	LIVE_TEST_MODEL=some-model \
//	go test ./internal/liveagent/ -run TestAgainstARealEndpoint -v
//
// They exist because the stubs encode what the protocol says, and a served
// model is the only way to find out what it does: how much a routing decision
// actually costs, whether tool calls come back usable, and whether reasoning
// stays out of the reply. Set LIVE_TEST_API_KEY for hosted endpoints; local
// ones generally need no key.
func realEndpoint(t *testing.T) *Client {
	t.Helper()
	base, model := os.Getenv("LIVE_TEST_BASE_URL"), os.Getenv("LIVE_TEST_MODEL")
	if base == "" || model == "" {
		t.Skip("set LIVE_TEST_BASE_URL and LIVE_TEST_MODEL to run against a served model")
	}
	c := New()
	c.SetLiveEndpoint(base, os.Getenv("LIVE_TEST_API_KEY"), model, 120*time.Second)
	return c
}

const routingSystemPrompt = `你负责接待 IM 对话。能当场答的就直接用中文简短回答；` +
	`需要读项目代码或执行操作才能做的，调用 ask_project_agent 交给项目 Agent。`

func routingTools() []ToolSpec {
	return []ToolSpec{{
		Name:        "ask_project_agent",
		Description: "把消息交给项目 Agent 处理。用于需要读项目、跑命令或执行操作的请求。",
		Params: []Param{
			{Name: "question", Description: "要交给项目 Agent 的问题", Required: true},
			{Name: "say", Description: "先发给用户的一句话，说明你要去做什么"},
		},
	}}
}

func TestAgainstARealEndpointItRoutesWorkToTheAgent(t *testing.T) {
	c := realEndpoint(t)
	start := time.Now()
	got, err := c.Complete(context.Background(), Request{
		System:    routingSystemPrompt,
		Messages:  []Message{{Role: "user", Content: "帮我看看结算页那个工作流为什么老失败"}},
		Tools:     routingTools(),
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("after %v: %v", time.Since(start), err)
	}
	t.Logf("took %v", time.Since(start))
	if got.ToolName != "ask_project_agent" {
		t.Fatalf("answered inline instead of delegating real work: tool=%q text=%q", got.ToolName, got.Text)
	}
	if strings.TrimSpace(got.Args["question"]) == "" {
		t.Fatalf("delegated with no question: %v", got.Args)
	}
	t.Logf("say: %s", got.Args["say"])
	t.Logf("question: %s", got.Args["question"])
}

func TestAgainstARealEndpointItAnswersChitchatInline(t *testing.T) {
	c := realEndpoint(t)
	start := time.Now()
	got, err := c.Complete(context.Background(), Request{
		System:    routingSystemPrompt,
		Messages:  []Message{{Role: "user", Content: "你好，在吗"}},
		Tools:     routingTools(),
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("after %v: %v", time.Since(start), err)
	}
	t.Logf("took %v", time.Since(start))
	if got.ToolName != "" {
		t.Fatalf("opened a sandbox to say hello: tool=%q args=%v", got.ToolName, got.Args)
	}
	t.Logf("text: %s", got.Text)
	for _, leak := range []string{"<think", "reasoning", "The user", "I should"} {
		if strings.Contains(got.Text, leak) {
			t.Fatalf("reasoning leaked into the reply (%q): %s", leak, got.Text)
		}
	}
}

// A turn is bounded by the timeout, not the token cap, so a model that thinks
// too long has to surface as a fast failure the caller can escalate on.
func TestAgainstARealEndpointATightDeadlineFailsFast(t *testing.T) {
	c := realEndpoint(t)
	base, model := os.Getenv("LIVE_TEST_BASE_URL"), os.Getenv("LIVE_TEST_MODEL")
	c.SetLiveEndpoint(base, os.Getenv("LIVE_TEST_API_KEY"), model, 900*time.Millisecond)

	start := time.Now()
	_, err := c.Complete(context.Background(), Request{
		System:    routingSystemPrompt,
		Messages:  []Message{{Role: "user", Content: "帮我把整个仓库重构一遍并解释每个决定"}},
		Tools:     routingTools(),
		MaxTokens: 2000,
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Skipf("endpoint answered within 900ms, nothing to bound (took %v)", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("timeout did not bound the turn: gave up after %v", elapsed)
	}
	t.Logf("gave up after %v: %v", elapsed, err)
}
