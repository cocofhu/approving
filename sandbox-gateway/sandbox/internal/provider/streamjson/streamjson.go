// Package streamjson implements the Anthropic-style "stream-json" NDJSON codec
// (as emitted by `claude -p --output-format stream-json`). Each stdout line is a
// JSON object describing an init / assistant / user / result event. The codec is
// transport-agnostic and driven by the oneshot engine.
package streamjson

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"backend/internal/backend/common"
	"backend/internal/provider"
	"backend/internal/provider/oneshot"
)

// PromptMode selects how the user turn is delivered to the CLI. The three
// shapes cover the observed stream-json CLIs: some take the prompt as the value
// of the print flag (argv), some read it raw from stdin, and some read a
// stream-json user message envelope from stdin.
type PromptMode int

const (
	// PromptArg passes the prompt as the value of the print flag: `-p <prompt>`.
	PromptArg PromptMode = iota
	// PromptStdinRaw keeps `-p` standalone and feeds the prompt raw on stdin.
	PromptStdinRaw
	// PromptStdinJSON keeps `-p` standalone, adds `--input-format stream-json`,
	// and feeds a `{"type":"user","message":{...}}` envelope on stdin.
	PromptStdinJSON
)

// Config parameterizes a stream-json CLI so the same parser serves several
// Anthropic-compatible agents.
type Config struct {
	AgentName  provider.Name
	Bin        string   // executable, e.g. "claude"
	Runtime    string   // capabilities label
	ConfigRoot string   // default config tree
	BaseArgs   []string // fixed args after --output-format, e.g. ["--verbose"]
	// PromptMode selects prompt delivery (default PromptArg).
	PromptMode PromptMode
	// WorkspaceFlag, when set, pins the workspace: `<flag> <cwd>`.
	WorkspaceFlag string
	// ResumeFlag is the flag used to resume a prior session (e.g. "--resume").
	ResumeFlag string
	// ModelFlag is the flag used to pin a model (e.g. "--model"); empty => omit.
	ModelFlag string
	// PermissionFlag/PermissionValue applied when AutoPermission is set
	// (e.g. "--permission-mode" / "acceptEdits").
	PermissionFlag  string
	PermissionValue string
	// AuthEnvFn normalizes credentials (nil => passthrough).
	AuthEnvFn func(env []string) []string
	Catalog   []provider.Model
}

// New builds a one-shot provider for a stream-json CLI.
func New(c Config) provider.Provider { return oneshot.NewProvider(&codec{c: c}) }

type codec struct{ c Config }

func (d *codec) AgentName() provider.Name { return d.c.AgentName }
func (d *codec) Bin() string              { return d.c.Bin }
func (d *codec) Runtime() string          { return d.c.Runtime }
func (d *codec) ConfigRoot() string       { return d.c.ConfigRoot }
func (d *codec) ReportsUsage() bool       { return true }
func (d *codec) PromptViaStdin() bool     { return d.c.PromptMode != PromptArg }

// SupportsImages reports whether this codec can carry image attachments.
// Only PromptStdinJSON (Claude-family --input-format stream-json) can embed
// Anthropic image content blocks; argv / raw-stdin modes cannot.
func (d *codec) SupportsImages() bool { return d.c.PromptMode == PromptStdinJSON }

// StdinBytes is the payload written to stdin when PromptViaStdin is true. For
// PromptStdinJSON it is a stream-json user message (optionally with images);
// otherwise the raw prompt (images must already have been rejected by oneshot).
func (d *codec) StdinBytes(prompt string, images []provider.PromptImage) []byte {
	if d.c.PromptMode == PromptStdinJSON {
		return userEnvelope(prompt, images)
	}
	return []byte(prompt)
}

func (d *codec) AuthEnv(env []string) []string {
	if d.c.AuthEnvFn == nil {
		return env
	}
	return d.c.AuthEnvFn(env)
}

func (d *codec) Models(context.Context) ([]provider.Model, error) { return d.c.Catalog, nil }

func (d *codec) Args(opts provider.OpenOptions, prompt, resumeID string) []string {
	args := []string{d.c.Bin, "-p"}
	if d.c.PromptMode == PromptArg {
		args = append(args, prompt)
	}
	args = append(args, "--output-format", "stream-json")
	if d.c.PromptMode == PromptStdinJSON {
		args = append(args, "--input-format", "stream-json")
	}
	args = append(args, d.c.BaseArgs...)
	if d.c.WorkspaceFlag != "" && opts.Cwd != "" {
		args = append(args, d.c.WorkspaceFlag, opts.Cwd)
	}
	if d.c.ModelFlag != "" && opts.Model != "" && opts.Model != "auto" {
		args = append(args, d.c.ModelFlag, opts.Model)
	}
	if d.c.ResumeFlag != "" && resumeID != "" {
		args = append(args, d.c.ResumeFlag, resumeID)
	}
	if opts.AutoPermission && d.c.PermissionFlag != "" {
		args = append(args, d.c.PermissionFlag, d.c.PermissionValue)
	}
	args = append(args, opts.CustomArgs...)
	return args
}

// userEnvelope builds the single-line stream-json user message an
// `--input-format stream-json` CLI expects on stdin. Images use Anthropic
// content-block shape (source.base64) so Claude / CodeBuddy can consume them.
func userEnvelope(prompt string, images []provider.PromptImage) []byte {
	content := make([]any, 0, 1+len(images))
	if prompt != "" || len(images) == 0 {
		content = append(content, map[string]any{"type": "text", "text": prompt})
	}
	for _, img := range images {
		mime := strings.TrimSpace(img.MimeType)
		if mime == "" {
			mime = "image/png"
		}
		content = append(content, map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": mime,
				"data":       img.Data,
			},
		})
	}
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": content,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("streamjson: marshal user envelope failed: %v", err)
		return []byte(prompt)
	}
	return append(data, '\n')
}

// --- wire shapes (Anthropic stream-json) -----------------------------------

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	// Thinking carries reasoning text in the dialect that puts it in a
	// dedicated field (e.g. `{"type":"thinking","thinking":"..."}`) rather than
	// reusing `text`.
	Thinking string          `json:"thinking"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	// ToolUseID correlates a tool_result back to its tool_use; the block uses
	// `tool_use_id`, not `id`.
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// thinkingText returns the reasoning text regardless of which field carries it.
func (b contentBlock) thinkingText() string {
	if b.Thinking != "" {
		return b.Thinking
	}
	return b.Text
}

// resultID returns the tool-call id a tool_result refers to.
func (b contentBlock) resultID() string {
	if b.ToolUseID != "" {
		return b.ToolUseID
	}
	return b.ID
}

type message struct {
	Model      string         `json:"model"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      *wireUsage     `json:"usage"`
}

// wireUsage accepts both the Anthropic snake_case spelling
// (input_tokens / cache_read_input_tokens / cache_creation_input_tokens) and
// the camelCase variant some CLIs emit (inputTokens / cacheReadTokens /
// cacheWriteTokens), so one parser serves several stream-json dialects.
type wireUsage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
}

func (u *wireUsage) UnmarshalJSON(b []byte) error {
	var m map[string]int64
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	pick := func(keys ...string) int64 {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				return v
			}
		}
		return 0
	}
	u.InputTokens = pick("input_tokens", "inputTokens")
	u.OutputTokens = pick("output_tokens", "outputTokens")
	u.CacheReadTokens = pick("cache_read_input_tokens", "cacheReadTokens", "cache_read_tokens")
	u.CacheCreationTokens = pick("cache_creation_input_tokens", "cacheWriteTokens", "cache_creation_tokens")
	return nil
}

type streamEvent struct {
	Type      string     `json:"type"`
	Subtype   string     `json:"subtype"`
	SessionID string     `json:"session_id"`
	Model     string     `json:"model"`
	Text      string     `json:"text"` // top-level text (e.g. cursor's thinking deltas)
	Message   *message   `json:"message"`
	Usage     *wireUsage `json:"usage"`
	Result    string     `json:"result"`
	IsError   bool       `json:"is_error"`
}

func (d *codec) ParseLine(line []byte) oneshot.ParseResult {
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		log.Printf("streamjson: skip non-json line: %v (snippet=%q)", err, truncateForLog(line, 120))
		return oneshot.ParseResult{}
	}
	var res oneshot.ParseResult
	res.SessionID = ev.SessionID

	addUsage := func(u *wireUsage, model string) {
		if u == nil {
			return
		}
		if model == "" {
			model = "default"
		}
		if res.Usage == nil {
			res.Usage = map[string]provider.TokenUsage{}
		}
		res.Usage[model] = provider.TokenUsage{
			InputTokens:      u.InputTokens,
			OutputTokens:     u.OutputTokens,
			CacheReadTokens:  u.CacheReadTokens,
			CacheWriteTokens: u.CacheCreationTokens,
		}
	}

	switch ev.Type {
	case "thinking":
		// Some CLIs stream reasoning as top-level events with subtype
		// delta/completed rather than as assistant content blocks.
		if ev.Text != "" {
			res.Msgs = append(res.Msgs, oneshot.Msg{Kind: oneshot.KindThinking, Text: ev.Text})
		}
	case "assistant":
		if ev.Message == nil {
			break
		}
		for _, b := range ev.Message.Content {
			switch b.Type {
			case "text":
				if b.Text != "" {
					res.Msgs = append(res.Msgs, oneshot.Msg{Kind: oneshot.KindText, Text: b.Text})
				}
			case "thinking", "redacted_thinking":
				if t := b.thinkingText(); t != "" {
					res.Msgs = append(res.Msgs, oneshot.Msg{Kind: oneshot.KindThinking, Text: t})
				}
			case "tool_use":
				res.Msgs = append(res.Msgs, oneshot.Msg{
					Kind: oneshot.KindToolUse, ToolCallID: b.ID, ToolTitle: b.Name, RawInput: b.Input,
				})
			}
		}
		addUsage(ev.Message.Usage, ev.Message.Model)
	case "user":
		if ev.Message == nil {
			break
		}
		for _, b := range ev.Message.Content {
			if b.Type == "tool_result" {
				res.Msgs = append(res.Msgs, oneshot.Msg{
					Kind: oneshot.KindToolResult, ToolCallID: b.resultID(), Text: extractText(b.Content),
				})
			}
		}
	case "result":
		addUsage(ev.Usage, ev.Model)
		if ev.IsError {
			res.StopReason = "failed"
			if ev.Result != "" {
				res.Msgs = append(res.Msgs, oneshot.Msg{Kind: oneshot.KindError, Text: ev.Result})
			}
		} else {
			res.StopReason = "end_turn"
		}
	}
	return res
}

// extractText flattens a tool_result content payload (string or block array).
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var out string
		for _, b := range blocks {
			out += b.Text
		}
		return out
	}
	return ""
}

// ClaudeAuthEnv normalizes the Anthropic key aliases.
func ClaudeAuthEnv(env []string) []string {
	return common.SetIfEmpty(env, "ANTHROPIC_API_KEY",
		common.FirstNonEmptyEnv("ACP_CLAUDE_API_KEY", "ANTHROPIC_API_KEY"))
}

func truncateForLog(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
