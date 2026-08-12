package gateshare

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestSanitizeLiveEventsKeepsRailsAndDropsTools(t *testing.T) {
	ev := SanitizeLiveEvents([]models.AcpEvent{
		{Kind: "thought", Text: "正在改 http://127.0.0.1/api/runs/secret"},
		{Kind: "tool_call", Title: "write", Text: "should-drop"},
		{Kind: "message", Text: "标题已改为绿色"},
		{Kind: "plan", Text: "plan-secret"},
	})
	if len(ev) != 2 {
		t.Fatalf("events=%d %+v", len(ev), ev)
	}
	if ev[0].Kind != "thought" || strings.Contains(ev[0].Text, "127.0.0.1") || strings.Contains(ev[0].Text, "/api/runs") {
		t.Fatalf("thought leaked: %+v", ev[0])
	}
	if ev[1].Kind != "message" || ev[1].Text != "标题已改为绿色" {
		t.Fatalf("message: %+v", ev[1])
	}
}

func TestFilterPublicBrokerFrameStripsRunAndRewritesNode(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"type":   "acp",
		"runId":  "run-secret",
		"nodeId": "research1",
		"busy":   true,
		"events": []any{
			map[string]any{"kind": "message", "text": "流式正文 http://10.1.2.3/api/x"},
			map[string]any{"kind": "tool_call", "title": "write", "text": "leak"},
		},
	})
	out, ok := FilterPublicBrokerFrame(raw, "research1")
	if !ok {
		t.Fatal("expected filtered frame")
	}
	s := string(out)
	if strings.Contains(s, "run-secret") || strings.Contains(s, "research1") || strings.Contains(s, "tool_call") {
		t.Fatalf("leaked: %s", s)
	}
	if !strings.Contains(s, PublicDialogueNodeID) || !strings.Contains(s, "流式正文") {
		t.Fatalf("missing public payload: %s", s)
	}
	if strings.Contains(s, "10.1.2.3") {
		t.Fatalf("url leak: %s", s)
	}

	other, ok := FilterPublicBrokerFrame(raw, "other-node")
	if ok || other != nil {
		t.Fatalf("other node must drop: ok=%v %s", ok, other)
	}
}

func TestFilterPublicBrokerFrameReviewTurnBegin(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"type":   "review",
		"runId":  "run-secret",
		"nodeId": "research1",
		"event":  "turn_begin",
		"item": map[string]any{
			"id":     "q1",
			"text":   "改成绿的",
			"images": []any{map[string]any{"data": "AAAA", "mimeType": "image/png"}},
		},
	})
	out, ok := FilterPublicBrokerFrame(raw, "research1")
	if !ok {
		t.Fatal("expected review frame")
	}
	s := string(out)
	if strings.Contains(s, "run-secret") || strings.Contains(s, "AAAA") || strings.Contains(s, "images") {
		t.Fatalf("leaked: %s", s)
	}
	if !strings.Contains(s, `"event":"turn_begin"`) || !strings.Contains(s, "改成绿的") {
		t.Fatalf("payload: %s", s)
	}
}
