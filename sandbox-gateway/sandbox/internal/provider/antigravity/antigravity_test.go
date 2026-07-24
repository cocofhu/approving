package antigravity

import (
	"strings"
	"testing"

	"backend/internal/provider"
	"backend/internal/provider/oneshot"
)

func TestAntigravityPlainTextStdout(t *testing.T) {
	c := &codec{c: Config{AgentName: provider.Antigravity, Bin: "agy"}}
	r := c.ParseLine([]byte("I will run the tests."))
	if len(r.Msgs) != 1 || r.Msgs[0].Kind != oneshot.KindText || r.Msgs[0].Text != "I will run the tests." {
		t.Fatalf("expected single text chunk, got %+v", r.Msgs)
	}
	if empty := c.ParseLine([]byte("   ")); len(empty.Msgs) != 0 {
		t.Fatalf("blank line should produce no messages")
	}
}

func TestAntigravityArgsWithLog(t *testing.T) {
	c := &codec{c: Config{AgentName: provider.Antigravity, Bin: "agy"}}
	args := c.ArgsWithLog(provider.OpenOptions{Model: "gemini-3", Cwd: "/work"}, "fix it", "conv-7", "/tmp/agy.log")
	joined := strings.Join(args, " ")
	for _, want := range []string{"agy", "-p fix it", "--dangerously-skip-permissions", "--model gemini-3", "--print-timeout 24h", "--log-file /tmp/agy.log", "--conversation conv-7", "--add-dir /work"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestAntigravityLogFileConversationID(t *testing.T) {
	c := &codec{}
	log := "I0528 13:36:23 printmode.go:130] Print mode: conversation=b8b263a4-4b2f-4339-acc9-78b248e2b606, sending message\n" +
		"I0528 13:36:24 more logs conversation=b8b263a4-4b2f-4339-acc9-78b248e2b606 stream\n"
	r := c.ParseLogFile([]byte(log))
	if r.SessionID != "b8b263a4-4b2f-4339-acc9-78b248e2b606" {
		t.Fatalf("session id = %q", r.SessionID)
	}
	if r.StopReason == "failed" {
		t.Fatalf("clean log should not fail")
	}
}

func TestAntigravityLogFilePromotesSilentFailures(t *testing.T) {
	c := &codec{}
	timeoutLog := "E0623 17:17:59 printmode.go:289] Print mode: timed out after 100 polls (printed=3)\n"
	if r := c.ParseLogFile([]byte(timeoutLog)); r.StopReason != "failed" {
		t.Fatalf("print-timeout should fail the turn, got %q", r.StopReason)
	}
	provErr := "E0101 agent executor error: model quota exceeded\n"
	r := c.ParseLogFile([]byte(provErr))
	if r.StopReason != "failed" {
		t.Fatalf("provider error should fail the turn")
	}
	found := false
	for _, m := range r.Msgs {
		if m.Kind == oneshot.KindError && strings.Contains(m.Text, "model quota exceeded") {
			found = true
		}
	}
	if !found {
		t.Fatalf("provider error message not surfaced: %+v", r.Msgs)
	}
}
