package liveagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// serveJSON stands in for a compatible endpoint, handing each request to fn.
func serveJSON(t *testing.T, fn func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(fn))
	t.Cleanup(srv.Close)
	c := New()
	c.SetLiveEndpoint(srv.URL, "test-key", "test-model", 2*time.Second)
	return c
}

func textResponse(content string) string {
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
	})
	return string(body)
}

func toolResponse(name, args string) string {
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{
				"content": "",
				"tool_calls": []map[string]any{
					{"function": map[string]any{"name": name, "arguments": args}},
				},
			}},
		},
	})
	return string(body)
}

func chatRequest() Request {
	return Request{
		System:   "you route conversations",
		Messages: []Message{{Role: "user", Content: "那个爬虫怎么样了"}},
		Tools: []ToolSpec{{
			Name:        "ask_project_agent",
			Description: "hand the message to the project agent",
			Params: []Param{
				{Name: "question", Required: true},
				{Name: "say"},
			},
		}},
		MaxTokens: 512,
	}
}

func TestPlainTextIsAnAnswer(t *testing.T) {
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth header = %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "test-model" {
			t.Errorf("model = %v", body["model"])
		}
		// The system prompt has to lead so a provider's prompt cache can hit it.
		msgs := body["messages"].([]any)
		if first := msgs[0].(map[string]any); first["role"] != "system" {
			t.Errorf("first message = %v", first)
		}
		io.WriteString(w, textResponse("还在跑，刚过了性能基线那步"))
	})

	got, err := c.Complete(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "还在跑，刚过了性能基线那步" || got.ToolName != "" {
		t.Fatalf("result = %+v", got)
	}
}

func TestToolCallBecomesAStructuredDecision(t *testing.T) {
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, toolResponse("ask_project_agent",
			`{"question":"这个工作流为什么老失败","say":"我去翻下它最近几次的记录"}`))
	})

	got, err := c.Complete(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.ToolName != "ask_project_agent" {
		t.Fatalf("tool = %q", got.ToolName)
	}
	if got.Args["say"] != "我去翻下它最近几次的记录" {
		t.Fatalf("args = %+v", got.Args)
	}
}

// A tool we never offered is a hallucination; acting on it would dispatch on
// something arbitrary, so it degrades to "no tool call".
func TestUnknownToolIsIgnoredRatherThanDispatched(t *testing.T) {
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": "好的",
					"tool_calls": []map[string]any{
						{"function": map[string]any{"name": "delete_everything", "arguments": "{}"}},
					},
				}},
			},
		})
		w.Write(body)
	})

	got, err := c.Complete(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.ToolName != "" {
		t.Fatalf("unknown tool must not be dispatched: %+v", got)
	}
	if got.Text != "好的" {
		t.Fatalf("text = %q", got.Text)
	}
}

// Reasoning reaching a user is the failure this whole layer exists to prevent,
// so content is scrubbed no matter what the provider promised.
func TestReasoningNeverSurvivesIntoTheAnswer(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"closed block", "<think>用户在问爬虫任务的状态，我应该查一下</think>还在跑", "还在跑"},
		{"uppercase tag", "<THINK>internal</THINK>done", "done"},
		{"alternate tag", "<reasoning>internal</reasoning>好了", "好了"},
		{"truncated by token cap", "还在跑<think>接下来我应该", "还在跑"},
		{"multiline", "<think>\nline one\nline two\n</think>\n结果出来了", "结果出来了"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, textResponse(tc.raw))
			})
			got, err := c.Complete(context.Background(), chatRequest())
			if err != nil {
				t.Fatal(err)
			}
			if got.Text != tc.want {
				t.Fatalf("text = %q, want %q", got.Text, tc.want)
			}
		})
	}
}

func TestContentPartsArrayIsAccepted(t *testing.T) {
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "两段"},
						{"type": "text", "text": "拼起来"},
					},
				}},
			},
		})
		w.Write(body)
	})

	got, err := c.Complete(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "两段拼起来" {
		t.Fatalf("text = %q", got.Text)
	}
}

func TestServerErrorRetriesOnceThenGivesUp(t *testing.T) {
	var calls atomic.Int32
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	})

	if _, err := c.Complete(context.Background(), chatRequest()); err == nil {
		t.Fatal("expected error")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want one retry", got)
	}
}

func TestTransientErrorIsRecoveredByTheRetry(t *testing.T) {
	var calls atomic.Int32
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, textResponse("second time lucky"))
	})

	got, err := c.Complete(context.Background(), chatRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "second time lucky" {
		t.Fatalf("text = %q", got.Text)
	}
}

// Repeating a rejected request cannot fix it, and this path runs per inbound
// message, so a bad key must not double the load on the endpoint.
func TestClientErrorIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	if _, err := c.Complete(context.Background(), chatRequest()); err == nil {
		t.Fatal("expected error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want no retry", got)
	}
}

// The error surfaces to a path that falls back to the sandbox, so it must not
// carry anything that could be echoed onward.
func TestErrorTextCarriesNoCredential(t *testing.T) {
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"invalid api key: test-key"}`)
	})

	_, err := c.Complete(context.Background(), chatRequest())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "test-key") {
		t.Fatalf("credential leaked into error: %v", err)
	}
}

// A slow endpoint must not hold the turn: the caller needs the error quickly so
// it can escalate to a sandbox instead.
func TestSlowEndpointTimesOutSoTheTurnCanMoveOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		io.WriteString(w, textResponse("too late"))
	}))
	defer srv.Close()

	c := New()
	c.SetLiveEndpoint(srv.URL, "test-key", "test-model", 100*time.Millisecond)

	start := time.Now()
	if _, err := c.Complete(context.Background(), chatRequest()); err == nil {
		t.Fatal("expected timeout")
	}
	// The bound covers the retry too, so the caller waits once, not twice.
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout not honoured: %v", elapsed)
	}
}

func TestUnconfiguredClientReportsItselfRatherThanCalling(t *testing.T) {
	c := New()
	if c.Configured() {
		t.Fatal("fresh client should not be configured")
	}
	if _, err := c.Complete(context.Background(), chatRequest()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	// A partial endpoint is the same as none: the settings page can be
	// half-filled at any moment.
	c.SetLiveEndpoint("https://api.example.com/v1", "", "m", time.Second)
	if c.Configured() {
		t.Fatal("missing key should leave it unconfigured")
	}
}

func TestEndpointEditsTakeEffectWithoutRestart(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, textResponse("from first"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, textResponse("from second"))
	}))
	defer second.Close()

	c := New()
	// A trailing slash is what someone pastes from a vendor console.
	c.SetLiveEndpoint(first.URL+"/", "k", "m", time.Second)
	got, err := c.Complete(context.Background(), chatRequest())
	if err != nil || got.Text != "from first" {
		t.Fatalf("first: %+v %v", got, err)
	}

	c.SetLiveEndpoint(second.URL, "k", "m", time.Second)
	got, err = c.Complete(context.Background(), chatRequest())
	if err != nil || got.Text != "from second" {
		t.Fatalf("second: %+v %v", got, err)
	}
}

func TestToolSchemaStaysFlatStrings(t *testing.T) {
	var seen map[string]any
	c := serveJSON(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &seen)
		io.WriteString(w, textResponse("ok"))
	})
	req := chatRequest()
	req.Tools[0].Params = append(req.Tools[0].Params, Param{
		Name: "decision", Enum: []string{"confirmed", "cancelled", "unclear"}, Required: true,
	})
	if _, err := c.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	tools := seen["tools"].([]any)
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	props := fn["parameters"].(map[string]any)["properties"].(map[string]any)
	for name, schema := range props {
		if schema.(map[string]any)["type"] != "string" {
			t.Fatalf("param %q is not a flat string: %v", name, schema)
		}
	}
	enum := props["decision"].(map[string]any)["enum"].([]any)
	if len(enum) != 3 {
		t.Fatalf("enum not carried through: %v", enum)
	}
	required := fn["parameters"].(map[string]any)["required"].([]any)
	if len(required) != 2 {
		t.Fatalf("required = %v", required)
	}
}