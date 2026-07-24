package pi

import (
	"os"
	"strings"
	"testing"

	"backend/internal/provider"
	"backend/internal/provider/oneshot"
)

func TestPiStreamParsing(t *testing.T) {
	c := &codec{c: Config{AgentName: provider.Pi, Bin: "pi"}}
	p := c.NewTurnParser(provider.OpenOptions{Model: "anthropic/claude"})

	var texts, thinks, tools, results int
	var usage provider.TokenUsage
	feed := func(line string) {
		r := p.ParseLine([]byte(line))
		for _, m := range r.Msgs {
			switch m.Kind {
			case oneshot.KindText:
				texts++
			case oneshot.KindThinking:
				thinks++
			case oneshot.KindToolUse:
				tools++
				if m.ToolTitle != "bash" || m.ToolCallID != "t1" {
					t.Fatalf("bad tool use %+v", m)
				}
			case oneshot.KindToolResult:
				results++
			}
		}
		for _, u := range r.Usage {
			usage.InputTokens += u.InputTokens
			usage.OutputTokens += u.OutputTokens
		}
	}

	feed(`{"type":"agent_start"}`)
	feed(`{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","delta":"hmm"}}`)
	feed(`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"hello "}}`)
	feed(`{"type":"tool_execution_start","toolCallId":"t1","toolName":"bash","args":{"cmd":"ls"}}`)
	feed(`{"type":"tool_execution_end","toolCallId":"t1","result":"file.txt"}`)
	feed(`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"world"}}`)
	feed(`{"type":"turn_end","message":{"model":"claude","usage":{"input":10,"output":20}}}`)

	if thinks != 1 {
		t.Fatalf("thinking chunks = %d, want 1", thinks)
	}
	if tools != 1 || results != 1 {
		t.Fatalf("tools=%d results=%d, want 1/1", tools, results)
	}
	if texts < 1 {
		t.Fatalf("expected at least one text chunk, got %d", texts)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 20 {
		t.Fatalf("usage = %+v, want in10/out20", usage)
	}
}

func TestPiControlTokenStripping(t *testing.T) {
	c := &codec{}
	tr := &turn{}
	_ = c
	// A control token bleeds into the text stream and must be stripped.
	out := tr.drain("safe<|tool>call more")
	out += tr.flush()
	if strings.Contains(out, "<|tool>") {
		t.Fatalf("control token not stripped: %q", out)
	}
	if !strings.Contains(out, "safe") || !strings.Contains(out, "more") {
		t.Fatalf("legitimate text lost: %q", out)
	}
}

func TestPiArgsAndSession(t *testing.T) {
	dir := t.TempDir()
	c := &codec{c: Config{AgentName: provider.Pi, Bin: "pi", SessionDir: dir}}

	sid, err := c.InitSession(provider.OpenOptions{})
	if err != nil {
		t.Fatalf("InitSession: %v", err)
	}
	if _, err := os.Stat(sid); err != nil {
		t.Fatalf("session file not created: %v", err)
	}

	args := c.Args(provider.OpenOptions{Model: "anthropic/claude-3"}, "do it", sid)
	joined := strings.Join(args, " ")
	for _, want := range []string{"pi", "-p", "--mode json", "--session " + sid, "--provider anthropic", "--model claude-3"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	// Prompt must be the final positional argument.
	if args[len(args)-1] != "do it" {
		t.Fatalf("prompt not last: %v", args)
	}
}
