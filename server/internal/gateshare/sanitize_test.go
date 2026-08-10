package gateshare

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func mustFuture() time.Time {
	return time.Now().Add(24 * time.Hour)
}

func TestSanitizeTurnsDropsIdentityAndCaps(t *testing.T) {
	msgs := []models.ReactMessage{
		{Role: "agent", Text: "请审阅 page.html，勿访问 http://10.1.2.3/api/runs/abc", At: "2026-08-01T00:00:00Z"},
		{Role: "human", Text: "改标题", At: "2026-08-01T00:01:00Z", Annotations: []models.ReactAnnotation{
			{Selector: "#title", Note: "改成交付确认", URL: "http://127.0.0.1:8080/preview/run-1/n/1/"},
		}},
		{Role: "system", Text: "should skip"},
	}
	turns := SanitizeTurns(msgs)
	if len(turns) != 2 {
		t.Fatalf("turns=%d %+v", len(turns), turns)
	}
	if strings.Contains(turns[0].Text, "10.1.2.3") || strings.Contains(turns[0].Text, "/api/runs") {
		t.Fatalf("agent text leaked: %s", turns[0].Text)
	}
	if turns[1].Role != "human" || len(turns[1].Annotations) == 0 || turns[1].Annotations[0].Selector != "#title" {
		t.Fatalf("human ann: %+v", turns[1])
	}
}

func TestSanitizeUpstreamKeepsThesisWithoutRunID(t *testing.T) {
	raw := `{
		"title":"澄清需求",
		"summary":"把临时页做成审批工作台",
		"goals":["三区布局","ReAct 复审"],
		"runId":"run-secret",
		"projectId":"p1",
		"url":"http://127.0.0.1/api/blobs/x"
	}`
	up := SanitizeUpstream("clarified_requirement.json", raw)
	if up == nil {
		t.Fatal("expected upstream")
	}
	if up["title"] != "澄清需求" {
		t.Fatalf("title: %+v", up)
	}
	b, err := json.Marshal(up)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "run-secret") || strings.Contains(s, "127.0.0.1") || strings.Contains(s, "projectId") {
		t.Fatalf("upstream leak: %s", s)
	}
}

func TestBuildReviewPreviewDTOIncludesWorkbenchFields(t *testing.T) {
	alive := true
	lookup := &LookupResult{
		Kind: models.ShareLinkKindReview,
		Link: models.GateShareLink{ExpiresAt: mustFuture()},
		Node: &models.Node{ID: "research1", Type: "research", Label: "调研"},
	}
	dto := BuildReviewPreviewDTO(models.ShareLinkStateActive, lookup, "", "research.json", `{"title":"调研摘要","goals":["g1"],"runId":"hide-me"}`, "nonce-1", PreviewExtras{
		Turns: []models.ReactMessage{
			{Role: "agent", Text: "请复审 research.json", At: "2026-08-01T00:00:00Z"},
		},
		UpstreamName:      "clarified_requirement.json",
		UpstreamContent:   `{"title":"澄清","summary":"对照审阅","runId":"nope"}`,
		ReactSessionAlive: alive,
		ProductKind:       ProductKindStructured,
		ProductName:       "research.json",
	})
	if dto.Kind != models.ShareLinkKindReview {
		t.Fatalf("kind=%s", dto.Kind)
	}
	if dto.Actions["confirm"] != "confirm" || dto.Actions["reply"] != "reply" || dto.Actions["cancel"] != "cancel" {
		t.Fatalf("actions=%+v", dto.Actions)
	}
	if dto.Actions["reject"] != "" {
		t.Fatalf("review must not expose reject: %+v", dto.Actions)
	}
	if dto.ReactSessionAlive == nil || !*dto.ReactSessionAlive {
		t.Fatal("expected alive")
	}
	if dto.ProductKind != ProductKindStructured || dto.ProductName != "research.json" {
		t.Fatalf("product=%s/%s", dto.ProductKind, dto.ProductName)
	}
	if len(dto.Turns) != 1 || dto.Upstream == nil || dto.Upstream["title"] != "澄清" {
		t.Fatalf("turns/upstream: turns=%+v upstream=%+v", dto.Turns, dto.Upstream)
	}
	raw, _ := json.Marshal(dto)
	s := string(raw)
	if strings.Contains(s, "hide-me") || strings.Contains(s, "nope") || strings.Contains(s, "runId") {
		t.Fatalf("preview leak: %s", s)
	}
}

func TestBuildReviewPreviewDTOIncludesQueueState(t *testing.T) {
	lookup := &LookupResult{
		Kind: models.ShareLinkKindReview,
		Link: models.GateShareLink{ExpiresAt: mustFuture()},
		Node: &models.Node{ID: "research1", Type: "research", Label: "调研"},
	}
	dto := BuildReviewPreviewDTO(models.ShareLinkStateActive, lookup, "", "research.json", `{"title":"调研摘要"}`, "nonce-q", PreviewExtras{
		ReactSessionAlive: true,
		SessionBusy:       true,
		Waiting:           1,
		QueueItems:        []map[string]any{{"id": "q1", "text": "请改标题，勿访问 http://10.1.2.3/api/runs/x"}},
		ActiveItem: map[string]any{
			"id":   "q1",
			"text": "请改标题，勿访问 http://10.1.2.3/api/runs/x",
			"images": []any{map[string]any{"url": "blob:http://127.0.0.1/abc"}},
		},
		ProductKind: ProductKindStructured,
		ProductName: "research.json",
	})
	if !dto.SessionBusy || dto.Waiting != 1 || len(dto.QueueItems) != 1 || dto.ActiveItem == nil {
		t.Fatalf("queue dto: busy=%v waiting=%d items=%+v active=%+v", dto.SessionBusy, dto.Waiting, dto.QueueItems, dto.ActiveItem)
	}
	if strings.Contains(dto.QueueItems[0].Text, "10.1.2.3") || strings.Contains(dto.ActiveItem.Text, "/api/runs") {
		t.Fatalf("queue leak: %+v %+v", dto.QueueItems[0], dto.ActiveItem)
	}
	raw, _ := json.Marshal(dto)
	if strings.Contains(string(raw), "blob:") || strings.Contains(string(raw), "127.0.0.1") {
		t.Fatalf("activeItem leaked images/host: %s", raw)
	}
}
