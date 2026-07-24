// Package opencodejson implements the NDJSON codec for CLIs that emit the
// `run --format json` event stream: one JSON object per stdout line with a
// top-level "type" and a nested "part" payload. Observed event types are
// step_start / text / tool_use / step_finish / error. Token usage is carried on
// step_finish parts. The same wire shape is shared by more than one CLI, so the
// codec is parameterized by Config.
package opencodejson

import (
	"context"
	"encoding/json"
	"log"

	"backend/internal/provider"
	"backend/internal/provider/oneshot"
)

// Config parameterizes an `run --format json` CLI.
type Config struct {
	AgentName  provider.Name
	Bin        string
	Runtime    string
	ConfigRoot string
	// BaseArgs precede the positional prompt, e.g.
	// ["run","--format","json","--dangerously-skip-permissions"].
	BaseArgs []string
	// ResumeFlag resumes a prior session (e.g. "--session").
	ResumeFlag string
	// ModelFlag pins a model (e.g. "--model"); empty => omit.
	ModelFlag string
	// WorkspaceFlag, when set, anchors project discovery: `<flag> <cwd>`.
	WorkspaceFlag string
	AuthEnvFn     func(env []string) []string
	Catalog       []provider.Model
}

// New builds a one-shot provider for a `run --format json` CLI.
func New(c Config) provider.Provider { return oneshot.NewProvider(&codec{c: c}) }

type codec struct{ c Config }

func (d *codec) AgentName() provider.Name { return d.c.AgentName }
func (d *codec) Bin() string              { return d.c.Bin }
func (d *codec) Runtime() string          { return d.c.Runtime }
func (d *codec) ConfigRoot() string       { return d.c.ConfigRoot }
func (d *codec) ReportsUsage() bool       { return true }
func (d *codec) PromptViaStdin() bool     { return false }

func (d *codec) AuthEnv(env []string) []string {
	if d.c.AuthEnvFn == nil {
		return env
	}
	return d.c.AuthEnvFn(env)
}

func (d *codec) Models(context.Context) ([]provider.Model, error) { return d.c.Catalog, nil }

func (d *codec) Args(opts provider.OpenOptions, prompt, resumeID string) []string {
	args := []string{d.c.Bin}
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
	args = append(args, opts.CustomArgs...)
	args = append(args, prompt) // positional prompt, last
	return args
}

// --- wire shapes -----------------------------------------------------------

type tokens struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Cache  *struct {
		Read  int64 `json:"read"`
		Write int64 `json:"write"`
	} `json:"cache"`
}

type toolState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output any             `json:"output"`
}

type part struct {
	ID     string     `json:"id"`
	Type   string     `json:"type"`
	Text   string     `json:"text"`
	Tool   string     `json:"tool"`
	CallID string     `json:"callID"`
	State  *toolState `json:"state"`
	Tokens *tokens    `json:"tokens"`
	Reason string     `json:"reason"`
}

type event struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionID"`
	Part      part   `json:"part"`
	Error     *struct {
		Name string `json:"name"`
		Data *struct {
			Message string `json:"message"`
		} `json:"data"`
	} `json:"error"`
}

func (d *codec) ParseLine(line []byte) oneshot.ParseResult {
	var ev event
	if err := json.Unmarshal(line, &ev); err != nil {
		log.Printf("opencodejson: skip non-json line: %v (snippet=%q)", err, truncateForLog(line, 120))
		return oneshot.ParseResult{}
	}
	var res oneshot.ParseResult
	res.SessionID = ev.SessionID

	switch ev.Type {
	case "text":
		if ev.Part.Type == "reasoning" {
			if ev.Part.Text != "" {
				res.Msgs = append(res.Msgs, oneshot.Msg{Kind: oneshot.KindThinking, Text: ev.Part.Text})
			}
			break
		}
		if ev.Part.Text != "" {
			res.Msgs = append(res.Msgs, oneshot.Msg{Kind: oneshot.KindText, Text: ev.Part.Text})
		}
	case "reasoning":
		if ev.Part.Text != "" {
			res.Msgs = append(res.Msgs, oneshot.Msg{Kind: oneshot.KindThinking, Text: ev.Part.Text})
		}
	case "tool_use":
		var input json.RawMessage
		if ev.Part.State != nil {
			input = ev.Part.State.Input
		}
		res.Msgs = append(res.Msgs, oneshot.Msg{
			Kind: oneshot.KindToolUse, ToolCallID: ev.Part.CallID, ToolTitle: ev.Part.Tool, RawInput: input,
		})
		// A single tool_use event carries both the call and (when finished)
		// its result in part.state, so emit a matching tool result too.
		if ev.Part.State != nil && ev.Part.State.Status == "completed" {
			res.Msgs = append(res.Msgs, oneshot.Msg{
				Kind: oneshot.KindToolResult, ToolCallID: ev.Part.CallID, Text: extractToolOutput(ev.Part.State.Output),
			})
		}
	case "error":
		res.StopReason = "failed"
		if msg := errMessage(ev); msg != "" {
			res.Msgs = append(res.Msgs, oneshot.Msg{Kind: oneshot.KindError, Text: msg})
		}
	case "step_finish":
		if t := ev.Part.Tokens; t != nil {
			u := provider.TokenUsage{InputTokens: t.Input, OutputTokens: t.Output}
			if t.Cache != nil {
				u.CacheReadTokens = t.Cache.Read
				u.CacheWriteTokens = t.Cache.Write
			}
			res.Usage = map[string]provider.TokenUsage{"default": u}
		}
		// A step that ends needing another tool round is not terminal.
		if ev.Part.Reason != "tool-calls" {
			res.StopReason = "end_turn"
		}
	}
	return res
}

// extractToolOutput renders a tool's state.output (string or arbitrary JSON)
// into a displayable string.
func extractToolOutput(output any) string {
	if output == nil {
		return ""
	}
	if s, ok := output.(string); ok {
		return s
	}
	data, _ := json.Marshal(output)
	return string(data)
}

func errMessage(ev event) string {
	if ev.Error == nil {
		return ""
	}
	if ev.Error.Data != nil && ev.Error.Data.Message != "" {
		return ev.Error.Data.Message
	}
	return ev.Error.Name
}

func truncateForLog(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
