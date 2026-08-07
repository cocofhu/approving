package liveagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ErrNotConfigured means no endpoint has been set. Callers treat this as
// "there is no conversation layer" and go straight to a sandbox.
var ErrNotConfigured = errors.New("liveagent: endpoint not configured")

// ErrBudgetExhausted means the model hit the token cap without producing a
// reply or a tool call. Callers treat it like any other failure and escalate.
var ErrBudgetExhausted = errors.New("liveagent: token budget exhausted before a reply")

// ErrEmptyResponse means the endpoint answered with neither text nor a tool
// call. ErrBadResponse means the answer could not be read as a completion at
// all, which usually means the address does not point at an OpenAI-compatible
// API. They are distinct sentinels because they send you to different places:
// one is the model, the other is the URL.
var (
	ErrEmptyResponse = errors.New("liveagent: response was empty")
	ErrBadResponse   = errors.New("liveagent: response was not a completion")
)

// StatusError reports a non-2xx response.
//
// The body is deliberately not carried. It can echo request fields — including
// the conversation — so nothing from it is safe to surface; the status code
// alone is enough to say which part of the configuration to look at.
type StatusError struct{ StatusCode int }

func (e *StatusError) Error() string {
	return fmt.Sprintf("liveagent: endpoint returned %d", e.StatusCode)
}

// Message is one turn of conversation sent to the model.
type Message struct {
	// Role is "user" or "assistant". The system prompt travels in Request.
	Role    string
	Content string
}

// Param is one tool argument. Arguments are deliberately flat strings: the
// decisions this layer makes need no structure, and small models are markedly
// more reliable with a shallow schema.
type Param struct {
	Name        string
	Description string
	// Enum, when set, constrains the value. Decisions that must be acted on
	// (a risk verdict, an action name) use it so an unexpected value is a
	// parse failure rather than a silent misread.
	Enum []string
	// Required marks the argument as mandatory in the schema.
	Required bool
}

// ToolSpec declares one decision the model may return.
type ToolSpec struct {
	Name        string
	Description string
	Params      []Param
}

// Request is one call. Tools are optional; a response with no tool call is a
// plain answer.
type Request struct {
	System   string
	Messages []Message
	Tools    []ToolSpec
	// MaxTokens caps the reply. Size it for the reply plus whatever the model
	// thinks first: a reasoning model can spend a thousand tokens deliberating
	// over a one-line routing decision, and a cap tight enough to be a real
	// cost control just truncates it into an empty answer. The timeout, not
	// this, is what bounds a turn.
	MaxTokens int
}

// Result is what the model decided. Exactly one of Text and ToolName is
// meaningful: a tool call is a structured decision, anything else is an answer
// to send as-is.
type Result struct {
	Text     string
	ToolName string
	Args     map[string]string
}

// Client calls an OpenAI-compatible chat/completions endpoint.
type Client struct {
	cur   current
	http  *http.Client
	stats stats
}

// New returns a client with no endpoint set; it stays unconfigured until
// SetLiveEndpoint is called from the settings layer.
func New() *Client {
	return &Client{
		// Per-call deadlines come from the context; this bound only stops a
		// connection from hanging forever if a context is ever missing one.
		http: &http.Client{Timeout: 2 * time.Minute},
	}
}

// SetLiveEndpoint implements services.LiveTuner, letting a settings-page edit
// take effect without a restart.
func (c *Client) SetLiveEndpoint(baseURL, apiKey, model string, timeout time.Duration) {
	c.cur.store(Endpoint{BaseURL: baseURL, APIKey: apiKey, Model: model, Timeout: timeout})
}

// Configured reports whether a call would have somewhere to go.
func (c *Client) Configured() bool { return c.cur.load().Configured() }

// ModelName is the configured conversation model, for decision samples / traces.
func (c *Client) ModelName() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.cur.load().Model)
}

// Complete makes one call and returns the model's decision. It retries once on
// a network error or 5xx; a 4xx is not retried because repeating a rejected
// request cannot fix it.
func (c *Client) Complete(ctx context.Context, req Request) (Result, error) {
	ep := c.cur.load()
	if !ep.Configured() {
		return Result{}, ErrNotConfigured
	}

	body, err := json.Marshal(buildPayload(ep, req))
	if err != nil {
		return Result{}, fmt.Errorf("liveagent: encode request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, ep.Timeout)
	defer cancel()

	allowed := make(map[string]bool, len(req.Tools))
	for _, t := range req.Tools {
		allowed[t.Name] = true
	}

	started := time.Now()
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		res, retryable, err := c.call(callCtx, ep, body, allowed)
		if err == nil {
			c.stats.record(time.Since(started), time.Now(), "")
			return res, nil
		}
		lastErr = err
		if !retryable || callCtx.Err() != nil {
			break
		}
	}
	// The recorded reason is the operator-facing one rather than the transport
	// error, so what shows up on the settings page after a real failure reads
	// the same as what the test button says.
	c.stats.record(time.Since(started), time.Now(), describeFailure(lastErr, ep.Timeout))
	return Result{}, lastErr
}

func (c *Client) call(ctx context.Context, ep Endpoint, body []byte, allowed map[string]bool) (Result, bool, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, ep.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Result{}, false, fmt.Errorf("liveagent: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if ep.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+ep.APIKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Result{}, true, fmt.Errorf("liveagent: call: %w", err)
	}
	defer resp.Body.Close()

	// Bound the read: a malfunctioning endpoint must not be able to exhaust
	// memory on a path that runs for every inbound message.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, true, fmt.Errorf("liveagent: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, resp.StatusCode >= 500, &StatusError{StatusCode: resp.StatusCode}
	}

	var parsed completionResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Result{}, false, fmt.Errorf("%w: decode: %w", ErrBadResponse, err)
	}
	if len(parsed.Choices) == 0 {
		return Result{}, false, fmt.Errorf("%w: no choices", ErrBadResponse)
	}

	msg := parsed.Choices[0].Message
	out := Result{Text: stripReasoning(decodeContent(msg.Content))}
	for _, tc := range msg.ToolCalls {
		name := strings.TrimSpace(tc.Function.Name)
		// A name we did not offer is a hallucination. Dropping it degrades to
		// "no tool call", which the caller already handles, rather than
		// dispatching on something arbitrary.
		if !allowed[name] {
			continue
		}
		out.ToolName = name
		out.Args = decodeArgs(tc.Function.Arguments)
		break
	}
	if out.ToolName == "" && out.Text == "" {
		// A reasoning model can spend the whole budget thinking and return
		// nothing at all. Retrying would just buy the same stall, so this is
		// reported as final and the caller escalates.
		if parsed.Choices[0].FinishReason == "length" {
			return Result{}, false, ErrBudgetExhausted
		}
		return Result{}, false, ErrEmptyResponse
	}
	return out, false, nil
}

// completionResponse is the subset of the OpenAI response shape we read.
//
// Reasoning is deliberately absent: providers put chain-of-thought in varying
// side fields ("reasoning", "reasoning_content", …) and we want none of them.
// Ignoring rather than parsing them is what keeps reasoning out of the reply by
// construction.
type completionResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   json.RawMessage `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func buildPayload(ep Endpoint, req Request) map[string]any {
	messages := make([]map[string]any, 0, len(req.Messages)+1)
	if s := strings.TrimSpace(req.System); s != "" {
		messages = append(messages, map[string]any{"role": "system", "content": s})
	}
	for _, m := range req.Messages {
		messages = append(messages, map[string]any{"role": m.Role, "content": m.Content})
	}

	payload := map[string]any{
		"model":    ep.Model,
		"messages": messages,
	}
	if req.MaxTokens > 0 {
		// max_tokens rather than max_completion_tokens: this has to work
		// against any OpenAI-compatible endpoint, and it is the field they all
		// still accept.
		payload["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toolSchemas(req.Tools)
	}
	return payload
}

func toolSchemas(tools []ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		props := map[string]any{}
		var required []string
		for _, p := range t.Params {
			schema := map[string]any{"type": "string"}
			if p.Description != "" {
				schema["description"] = p.Description
			}
			if len(p.Enum) > 0 {
				schema["enum"] = p.Enum
			}
			props[p.Name] = schema
			if p.Required {
				required = append(required, p.Name)
			}
		}
		params := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			params["required"] = required
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		})
	}
	return out
}

// thinkBlock matches a chain-of-thought wrapper. Providers are asked to keep
// reasoning out of content, and most do, but this path feeds a user-visible
// channel: a leak here is exactly the failure the whole conversation layer
// exists to prevent, so content is scrubbed regardless of what was promised.
var thinkBlock = regexp.MustCompile(`(?is)<(think|thinking|reasoning)>.*?</(think|thinking|reasoning)>`)

// unclosedThink catches a reasoning block truncated by the token cap, which
// would otherwise survive as a half-open tag and everything after it.
var unclosedThink = regexp.MustCompile(`(?is)<(think|thinking|reasoning)>.*\z`)

// orphanCloseThink catches a lone closing tag some local reasoning models put
// between a draft reply and the same reply again — e.g. "在的。\n</think>\n\n在的。"
var orphanCloseThink = regexp.MustCompile(`(?is)</(think|thinking|reasoning)>`)

func stripReasoning(s string) string {
	s = thinkBlock.ReplaceAllString(s, "")
	s = unclosedThink.ReplaceAllString(s, "")
	s = orphanCloseThink.ReplaceAllString(s, "")
	s = collapseDuplicateParagraphs(s)
	return strings.TrimSpace(s)
}

// collapseDuplicateParagraphs drops a paragraph that is repeated right after
// itself. Reasoning models that echo the final answer on both sides of a think
// tag would otherwise greet the user twice in one bubble.
func collapseDuplicateParagraphs(s string) string {
	parts := strings.Split(s, "\n")
	var out []string
	var prev string
	for _, p := range parts {
		cur := strings.TrimSpace(p)
		if cur == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		if cur == prev {
			continue
		}
		out = append(out, p)
		prev = cur
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// decodeContent accepts both the plain string form and the content-parts array
// some compatible endpoints return.
func decodeContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

// decodeArgs flattens a tool call's JSON arguments to strings. Non-string
// scalars are accepted and rendered, because models occasionally emit a bare
// number or boolean where the schema asked for a string.
func decodeArgs(s string) map[string]string {
	out := map[string]string{}
	var raw map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &raw); err != nil {
		return out
	}
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			out[k] = strings.TrimSpace(t)
		case nil:
			out[k] = ""
		default:
			out[k] = strings.TrimSpace(fmt.Sprint(t))
		}
	}
	return out
}
