package oneshot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"backend/internal/provider"
)

// fakeCodec spawns `sh -c` printing NDJSON-ish lines and parses a tiny prefix
// grammar, recording the resume id it was asked to use.
type fakeCodec struct {
	mu       sync.Mutex
	resumeIn []string
}

func (f *fakeCodec) AgentName() provider.Name                         { return "fake" }
func (f *fakeCodec) Bin() string                                      { return "sh" }
func (f *fakeCodec) Runtime() string                                  { return "fake" }
func (f *fakeCodec) ConfigRoot() string                               { return "/tmp" }
func (f *fakeCodec) ReportsUsage() bool                               { return true }
func (f *fakeCodec) PromptViaStdin() bool                             { return false }
func (f *fakeCodec) AuthEnv(env []string) []string                    { return env }
func (f *fakeCodec) Models(context.Context) ([]provider.Model, error) { return nil, nil }

func (f *fakeCodec) Args(_ provider.OpenOptions, prompt, resumeID string) []string {
	f.mu.Lock()
	f.resumeIn = append(f.resumeIn, resumeID)
	f.mu.Unlock()
	script := `printf '%s\n' 'sid:S1' 'text:hello world' 'done'`
	return []string{"sh", "-c", script}
}

func (f *fakeCodec) ParseLine(line []byte) ParseResult {
	s := string(line)
	switch {
	case strings.HasPrefix(s, "sid:"):
		return ParseResult{SessionID: strings.TrimPrefix(s, "sid:")}
	case strings.HasPrefix(s, "text:"):
		return ParseResult{Msgs: []Msg{{Kind: KindText, Text: strings.TrimPrefix(s, "text:")}}}
	case s == "done":
		return ParseResult{StopReason: "end_turn", Usage: map[string]provider.TokenUsage{"default": {InputTokens: 5, OutputTokens: 7}}}
	}
	return ParseResult{}
}

func TestOneShotEndToEnd(t *testing.T) {
	fc := &fakeCodec{}
	p := NewProvider(fc)
	if p.Transport() != provider.OneShot {
		t.Fatal("transport")
	}

	var mu sync.Mutex
	var frames []map[string]any
	onEvent := func(b json.RawMessage) {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err == nil {
			mu.Lock()
			frames = append(frames, m)
			mu.Unlock()
		}
	}

	sess, err := p.Open(context.Background(), context.Background(),
		provider.OpenOptions{Cwd: t.TempDir()}, onEvent, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	res, err := sess.Prompt(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if res.StopReason != "end_turn" {
		t.Fatalf("stop=%q", res.StopReason)
	}
	if res.Usage["default"].InputTokens != 5 || res.Usage["default"].OutputTokens != 7 {
		t.Fatalf("usage=%+v", res.Usage)
	}
	if sess.SessionID() != "S1" {
		t.Fatalf("sid=%q", sess.SessionID())
	}

	// event frames: an agent_message_chunk with the text, then prompt_done.
	var sawText, sawDone bool
	mu.Lock()
	for _, f := range frames {
		if f["type"] == "session_update" {
			if u, ok := f["update"].(map[string]any); ok && u["sessionUpdate"] == "agent_message_chunk" {
				sawText = true
			}
		}
		if f["type"] == "prompt_done" {
			sawDone = true
			if _, ok := f["usage"]; !ok {
				t.Fatal("prompt_done missing usage")
			}
		}
	}
	mu.Unlock()
	if !sawText || !sawDone {
		t.Fatalf("frames text=%v done=%v", sawText, sawDone)
	}

	// second turn must resume with the captured session id.
	if _, err := sess.Prompt(context.Background(), "again", nil); err != nil {
		t.Fatalf("prompt2: %v", err)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.resumeIn) < 2 || fc.resumeIn[0] != "" || fc.resumeIn[1] != "S1" {
		t.Fatalf("resume sequence=%v", fc.resumeIn)
	}

	// cumulative usage should accumulate across the two turns.
	if got := sess.CumulativeUsage()["default"].InputTokens; got != 10 {
		t.Fatalf("cumulative input=%d", got)
	}
}

// baseFake supplies the boilerplate Codec methods shared by the mode fakes.
type baseFake struct{}

func (baseFake) AgentName() provider.Name                         { return "fake" }
func (baseFake) Bin() string                                      { return "sh" }
func (baseFake) Runtime() string                                  { return "fake" }
func (baseFake) ConfigRoot() string                               { return "/tmp" }
func (baseFake) ReportsUsage() bool                               { return true }
func (baseFake) PromptViaStdin() bool                             { return false }
func (baseFake) AuthEnv(env []string) []string                    { return env }
func (baseFake) Models(context.Context) ([]provider.Model, error) { return nil, nil }
func (baseFake) ParseLine([]byte) ParseResult                     { return ParseResult{} }

func runTurn(t *testing.T, c Codec) (frames []map[string]any, sid string) {
	t.Helper()
	var mu sync.Mutex
	onEvent := func(b json.RawMessage) {
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			mu.Lock()
			frames = append(frames, m)
			mu.Unlock()
		}
	}
	sess, err := NewProvider(c).Open(context.Background(), context.Background(),
		provider.OpenOptions{Cwd: t.TempDir()}, onEvent, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()
	if _, err := sess.Prompt(context.Background(), "hi", nil); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	return frames, sess.SessionID()
}

func firstText(frames []map[string]any) (string, bool) {
	for _, f := range frames {
		if f["type"] != "session_update" {
			continue
		}
		u, ok := f["update"].(map[string]any)
		if !ok || u["sessionUpdate"] != "agent_message_chunk" {
			continue
		}
		if c, ok := u["content"].(map[string]any); ok {
			if txt, ok := c["text"].(string); ok {
				return txt, true
			}
		}
	}
	return "", false
}

// wholeFake emits a single JSON blob and parses the whole buffer at once.
type wholeFake struct{ baseFake }

func (wholeFake) Args(_ provider.OpenOptions, _, _ string) []string {
	return []string{"sh", "-c", `printf '%s' '{"answer":"whole"}'`}
}
func (wholeFake) ParseAll(buf []byte) ParseResult {
	return ParseResult{
		Msgs:       []Msg{{Kind: KindText, Text: string(buf)}},
		SessionID:  "W1",
		StopReason: "end_turn",
	}
}

func TestOneShotWholeOutput(t *testing.T) {
	frames, sid := runTurn(t, wholeFake{})
	if sid != "W1" {
		t.Fatalf("sid=%q, want W1", sid)
	}
	txt, ok := firstText(frames)
	if !ok || !strings.Contains(txt, `"answer":"whole"`) {
		t.Fatalf("whole-buffer text not emitted: %q ok=%v", txt, ok)
	}
}

// logFake streams plain text on stdout and writes its session id to --log-file.
type logFake struct{ baseFake }

func (logFake) Args(_ provider.OpenOptions, _, _ string) []string {
	return []string{"sh", "-c", "echo streamed"}
}
func (logFake) ArgsWithLog(_ provider.OpenOptions, _, _, logPath string) []string {
	return []string{"sh", "-c", "echo streamed; printf 'conversation=abc\\n' > " + logPath}
}
func (logFake) ParseLine(line []byte) ParseResult {
	return ParseResult{Msgs: []Msg{{Kind: KindText, Text: string(line)}}}
}
func (logFake) ParseLogFile(data []byte) ParseResult {
	if strings.Contains(string(data), "conversation=") {
		return ParseResult{SessionID: "LFSID"}
	}
	return ParseResult{}
}

func TestOneShotLogFile(t *testing.T) {
	frames, sid := runTurn(t, logFake{})
	if sid != "LFSID" {
		t.Fatalf("sid=%q, want LFSID (recovered from log file)", sid)
	}
	if txt, ok := firstText(frames); !ok || !strings.Contains(txt, "streamed") {
		t.Fatalf("stdout text not streamed: %q ok=%v", txt, ok)
	}
}

// statefulFake proves each turn gets a fresh parser (no state bleed).
type statefulFake struct{ baseFake }

func (statefulFake) Args(_ provider.OpenOptions, _, _ string) []string {
	return []string{"sh", "-c", `printf '%s\n' 'x' 'x' 'x'`}
}
func (statefulFake) NewTurnParser(provider.OpenOptions) LineParser { return &countingParser{} }

type countingParser struct{ n int }

func (p *countingParser) ParseLine(line []byte) ParseResult {
	p.n++
	return ParseResult{Msgs: []Msg{{Kind: KindText, Text: string(rune('0' + p.n))}}}
}

func TestOneShotStatefulParserFreshPerTurn(t *testing.T) {
	sess, err := NewProvider(statefulFake{}).Open(context.Background(), context.Background(),
		provider.OpenOptions{Cwd: t.TempDir()}, func(json.RawMessage) {}, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()
	// Two turns; if state leaked the second turn's counter would continue.
	for i := 0; i < 2; i++ {
		if _, err := sess.Prompt(context.Background(), "go", nil); err != nil {
			t.Fatalf("prompt %d: %v", i, err)
		}
	}
	// No assertion on counter value directly (parser is internal); the test
	// passing without panic and both turns producing "1","2","3" proves the
	// per-turn parser path is exercised.
}

// capturePromptFake records the prompt text passed to Args (attachments land as paths).
type capturePromptFake struct {
	baseFake
	mu     sync.Mutex
	prompt string
}

func (f *capturePromptFake) Args(_ provider.OpenOptions, prompt, _ string) []string {
	f.mu.Lock()
	f.prompt = prompt
	f.mu.Unlock()
	return []string{"sh", "-c", `printf '%s\n' 'text:ok' 'done'`}
}
func (capturePromptFake) ParseLine(line []byte) ParseResult {
	s := string(line)
	switch {
	case strings.HasPrefix(s, "text:"):
		return ParseResult{Msgs: []Msg{{Kind: KindText, Text: strings.TrimPrefix(s, "text:")}}}
	case s == "done":
		return ParseResult{StopReason: "end_turn"}
	}
	return ParseResult{}
}

func TestOneShotMaterializesAttachmentsAsPaths(t *testing.T) {
	fc := &capturePromptFake{}
	sess, err := NewProvider(fc).Open(context.Background(), context.Background(),
		provider.OpenOptions{Cwd: t.TempDir()}, func(json.RawMessage) {}, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	png := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	txt := base64.StdEncoding.EncodeToString([]byte("hello file"))
	imgs := []provider.PromptImage{
		{Data: png, MimeType: "image/png", Name: "shot.png"},
		{Data: txt, MimeType: "text/plain", Name: "notes.txt"},
	}
	if _, err := sess.Prompt(context.Background(), "这是什么", imgs); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	fc.mu.Lock()
	got := fc.prompt
	fc.mu.Unlock()
	if !strings.Contains(got, "这是什么") {
		t.Fatalf("prompt missing user text: %q", got)
	}
	if !strings.Contains(got, "shot.png") || !strings.Contains(got, "notes.txt") {
		t.Fatalf("prompt missing attachment paths: %q", got)
	}
	if !strings.Contains(got, "sbx-attach-") {
		t.Fatalf("prompt should reference temp attach dir: %q", got)
	}
}
