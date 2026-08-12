package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/gateshare"
	"github.com/gorilla/websocket"
)

func TestPublicGateEventsWSStreamsSanitizedAcp(t *testing.T) {
	h := newHarness(t)
	seedInboxReview(t, h, "run-pub-ws", "research-ws", true)
	created := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-pub-ws/reviews/research-ws/share-link", map[string]any{"ttlTier": "24h"}))
	url, _ := created["url"].(string)
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")
	if !gateshare.ValidTokenShape(token) {
		t.Fatalf("token: %s", token)
	}

	srv := httptest.NewServer(h.r)
	defer srv.Close()

	c, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL, "/public/gate-approvals/events"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := c.WriteJSON(map[string]any{"token": token}); err != nil {
		t.Fatalf("auth: %v", err)
	}
	_, ready, err := c.ReadMessage()
	if err != nil || !strings.Contains(string(ready), `"type":"ready"`) {
		t.Fatalf("ready: %v %s", err, ready)
	}

	h.h.Eng.Broker().Publish("run-pub-ws", mustJSON(t, map[string]any{
		"type":   "acp",
		"runId":  "run-pub-ws",
		"nodeId": "research-ws",
		"busy":   true,
		"events": []any{
			map[string]any{"kind": "thought", "text": "思考 http://127.0.0.1/api/runs/x"},
			map[string]any{"kind": "message", "text": "标题已改为绿色"},
			map[string]any{"kind": "tool_call", "title": "write", "text": "secret"},
		},
	}))
	h.h.Eng.Broker().Publish("run-pub-ws", mustJSON(t, map[string]any{
		"type":   "acp",
		"runId":  "run-pub-ws",
		"nodeId": "other-node",
		"events": []any{map[string]any{"kind": "message", "text": "should-not-leak"}},
	}))
	h.h.Eng.Broker().Publish("run-pub-ws", mustJSON(t, map[string]any{
		"type":   "review",
		"runId":  "run-pub-ws",
		"nodeId": "research-ws",
		"event":  "turn_begin",
		"item":   map[string]any{"text": "改成绿的"},
	}))

	sawAcp, sawReview := false, false
	for i := 0; i < 6; i++ {
		_, msg, err := c.ReadMessage()
		if err != nil {
			break
		}
		s := string(msg)
		if strings.Contains(s, "run-pub-ws") || strings.Contains(s, "research-ws") || strings.Contains(s, "should-not-leak") || strings.Contains(s, "tool_call") {
			t.Fatalf("leaked: %s", s)
		}
		if strings.Contains(s, `"type":"acp"`) && strings.Contains(s, "标题已改为绿色") {
			sawAcp = true
			if strings.Contains(s, "127.0.0.1") {
				t.Fatalf("url leak: %s", s)
			}
		}
		if strings.Contains(s, `"event":"turn_begin"`) && strings.Contains(s, "改成绿的") {
			sawReview = true
		}
	}
	if !sawAcp || !sawReview {
		t.Fatalf("sawAcp=%v sawReview=%v", sawAcp, sawReview)
	}
}

func TestPublicGateEventsWSRejectsBadToken(t *testing.T) {
	h := newHarness(t)
	srv := httptest.NewServer(h.r)
	defer srv.Close()
	c, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL, "/public/gate-approvals/events"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := c.WriteJSON(map[string]any{"token": "ab"}); err != nil {
		t.Fatalf("auth: %v", err)
	}
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(msg), `"status":"invalid"`) {
		t.Fatalf("want invalid, got %s", msg)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
