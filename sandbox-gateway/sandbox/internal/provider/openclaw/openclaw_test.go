package openclaw

import (
	"strings"
	"testing"

	"backend/internal/provider"
	"backend/internal/provider/oneshot"
)

func TestOpenclawWholeDocument(t *testing.T) {
	c := &codec{c: Config{AgentName: provider.OpenClaw, Bin: "openclaw"}}
	doc := `{
  "payloads": [{"text": "Hello "}, {"text": "world"}],
  "meta": {
    "durationMs": 1234,
    "agentMeta": {
      "sessionId": "conv-42",
      "model": "claude-sonnet-4",
      "usage": {"input": 100, "output": 50, "cacheRead": 10}
    }
  }
}`
	r := c.ParseAll([]byte(doc))
	if r.SessionID != "conv-42" {
		t.Fatalf("session id = %q, want conv-42", r.SessionID)
	}
	var text string
	for _, m := range r.Msgs {
		if m.Kind == oneshot.KindText {
			text += m.Text
		}
	}
	if text != "Hello world" {
		t.Fatalf("text = %q, want 'Hello world'", text)
	}
	u := r.Usage["claude-sonnet-4"]
	if u.InputTokens != 100 || u.OutputTokens != 50 || u.CacheReadTokens != 10 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestOpenclawWholeDocumentWithLogPreamble(t *testing.T) {
	c := &codec{}
	doc := "some log line\nanother log\n{\"payloads\":[{\"text\":\"hi\"}],\"meta\":{\"durationMs\":1,\"agentMeta\":{}}}"
	r := c.ParseAll([]byte(doc))
	if len(r.Msgs) != 1 || r.Msgs[0].Text != "hi" {
		t.Fatalf("expected text 'hi', got %+v", r.Msgs)
	}
}

func TestOpenclawNDJSONFallback(t *testing.T) {
	c := &codec{}
	stream := strings.Join([]string{
		`{"type":"text","sessionId":"s1","text":"streamed"}`,
		`{"type":"tool_use","tool":"bash","callId":"c1","input":{"cmd":"ls"}}`,
		`{"type":"tool_result","callId":"c1","text":"out"}`,
		`{"type":"step_finish","usage":{"inputTokens":5,"outputTokens":6}}`,
	}, "\n")
	r := c.ParseAll([]byte(stream))
	if r.SessionID != "s1" {
		t.Fatalf("session id = %q, want s1", r.SessionID)
	}
	var kinds []oneshot.MsgKind
	for _, m := range r.Msgs {
		kinds = append(kinds, m.Kind)
	}
	if len(kinds) != 3 {
		t.Fatalf("expected text+tooluse+toolresult, got %+v", r.Msgs)
	}
	if r.Usage["openclaw"].InputTokens != 5 || r.Usage["openclaw"].OutputTokens != 6 {
		t.Fatalf("usage = %+v", r.Usage)
	}
}

func TestOpenclawArgs(t *testing.T) {
	c := &codec{c: Config{AgentName: provider.OpenClaw, Bin: "openclaw"}}
	args := c.Args(provider.OpenOptions{Model: "my-agent"}, "hi there", "sess-1")
	joined := strings.Join(args, " ")
	for _, want := range []string{"openclaw agent --local --json", "--session-id sess-1", "--agent my-agent", "--message hi there"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if args[len(args)-1] != "hi there" {
		t.Fatalf("prompt must follow --message: %v", args)
	}
}
