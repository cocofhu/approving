package codex

import (
	"testing"

	"backend/internal/provider"
	"backend/internal/provider/oneshot"
)

func TestParseAgentMessage(t *testing.T) {
	var c codec
	pr := c.ParseLine([]byte(`{"msg":{"type":"agent_message","message":"hello"}}`))
	if len(pr.Msgs) != 1 || pr.Msgs[0].Kind != oneshot.KindText || pr.Msgs[0].Text != "hello" {
		t.Fatalf("pr=%+v", pr)
	}
}

func TestParseSessionAndUsageAndComplete(t *testing.T) {
	var c codec
	if sid := c.ParseLine([]byte(`{"msg":{"type":"session_configured","session_id":"S9"}}`)).SessionID; sid != "S9" {
		t.Fatalf("sid=%q", sid)
	}
	u := c.ParseLine([]byte(`{"msg":{"type":"token_count","info":{"input_tokens":10,"output_tokens":4,"cached_input_tokens":2}}}`)).Usage
	if u["default"].InputTokens != 10 || u["default"].OutputTokens != 4 || u["default"].CacheReadTokens != 2 {
		t.Fatalf("usage=%+v", u)
	}
	if stop := c.ParseLine([]byte(`{"msg":{"type":"task_complete"}}`)).StopReason; stop != "end_turn" {
		t.Fatalf("stop=%q", stop)
	}
}

func TestArgsResume(t *testing.T) {
	var c codec
	fresh := c.Args(provider.OpenOptions{Model: "gpt-5"}, "hi", "")
	if fresh[0] != "codex" || fresh[1] != "exec" || fresh[len(fresh)-1] != "hi" {
		t.Fatalf("fresh=%v", fresh)
	}
	res := c.Args(provider.OpenOptions{}, "hi", "S1")
	if !(res[2] == "resume" && res[3] == "S1") {
		t.Fatalf("resume args=%v", res)
	}
}
