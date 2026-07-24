package copilot

import (
	"strings"
	"testing"

	"backend/internal/provider"
	"backend/internal/provider/oneshot"
)

func newParser(model string) oneshot.LineParser {
	c := &codec{c: Config{AgentName: provider.Copilot, Bin: "copilot"}}
	return c.NewTurnParser(provider.OpenOptions{Model: model})
}

func collect(t *testing.T, p oneshot.LineParser, lines ...string) (msgs []oneshot.Msg, usage map[string]provider.TokenUsage, sid, stop string) {
	t.Helper()
	usage = map[string]provider.TokenUsage{}
	for _, ln := range lines {
		r := p.ParseLine([]byte(ln))
		msgs = append(msgs, r.Msgs...)
		if r.SessionID != "" {
			sid = r.SessionID
		}
		for m, u := range r.Usage {
			cur := usage[m]
			cur.OutputTokens += u.OutputTokens
			cur.InputTokens += u.InputTokens
			usage[m] = cur
		}
		if r.StopReason != "" {
			stop = r.StopReason
		}
	}
	return
}

func TestCopilotDeltaThenMessageNoDuplicate(t *testing.T) {
	p := newParser("gpt-5")
	msgs, usage, sid, stop := collect(t, p,
		`{"type":"session.start","data":{"sessionId":"s-1","selectedModel":"gpt-5"}}`,
		`{"type":"assistant.message_delta","data":{"messageId":"m1","deltaContent":"pong"}}`,
		`{"type":"assistant.message","data":{"messageId":"m1","content":"pong","outputTokens":7,"toolRequests":[{"toolCallId":"c1","name":"shell","arguments":{"cmd":"ls"}}]}}`,
		`{"type":"result","sessionId":"s-1","exitCode":0}`,
	)
	if sid != "s-1" {
		t.Fatalf("session id = %q, want s-1", sid)
	}
	if stop == "failed" {
		t.Fatalf("stop should not be failed on exitCode 0")
	}
	// Exactly one text ("pong" from the delta), one tool_use; no duplicate text.
	var texts, tools int
	for _, m := range msgs {
		switch m.Kind {
		case oneshot.KindText:
			texts++
			if m.Text != "pong" {
				t.Fatalf("unexpected text %q", m.Text)
			}
		case oneshot.KindToolUse:
			tools++
			if m.ToolTitle != "shell" || m.ToolCallID != "c1" {
				t.Fatalf("bad tool use %+v", m)
			}
		}
	}
	if texts != 1 {
		t.Fatalf("text messages = %d, want 1 (delta only)", texts)
	}
	if tools != 1 {
		t.Fatalf("tool_use messages = %d, want 1", tools)
	}
	if usage["gpt-5"].OutputTokens != 7 {
		t.Fatalf("output tokens = %d, want 7", usage["gpt-5"].OutputTokens)
	}
}

func TestCopilotMessageWithoutDeltaEmitsText(t *testing.T) {
	p := newParser("gpt-5")
	msgs, _, _, _ := collect(t, p,
		`{"type":"assistant.message","data":{"messageId":"m2","content":"final answer"}}`,
	)
	if len(msgs) != 1 || msgs[0].Kind != oneshot.KindText || msgs[0].Text != "final answer" {
		t.Fatalf("want single text 'final answer', got %+v", msgs)
	}
}

func TestCopilotToolResultAndError(t *testing.T) {
	p := newParser("")
	msgs, _, _, stop := collect(t, p,
		`{"type":"tool.execution_complete","data":{"toolCallId":"c1","model":"gpt-5","success":true,"result":{"content":"ok"}}}`,
		`{"type":"session.error","data":{"errorType":"fatal","message":"boom"}}`,
	)
	var gotResult, gotErr bool
	for _, m := range msgs {
		if m.Kind == oneshot.KindToolResult && m.ToolCallID == "c1" && m.Text == "ok" {
			gotResult = true
		}
		if m.Kind == oneshot.KindError && m.Text == "boom" {
			gotErr = true
		}
	}
	if !gotResult {
		t.Fatalf("missing tool result: %+v", msgs)
	}
	if !gotErr {
		t.Fatalf("missing error message: %+v", msgs)
	}
	if stop != "failed" {
		t.Fatalf("stop = %q, want failed", stop)
	}
}

func TestCopilotArgs(t *testing.T) {
	c := &codec{c: Config{AgentName: provider.Copilot, Bin: "copilot"}}
	args := c.Args(provider.OpenOptions{Model: "gpt-5"}, "hello", "sess-9")
	joined := strings.Join(args, " ")
	for _, want := range []string{"copilot", "-p hello", "--output-format json", "--allow-all", "--no-ask-user", "--model gpt-5", "--resume sess-9"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}
