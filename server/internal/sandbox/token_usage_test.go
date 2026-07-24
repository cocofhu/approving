package sandbox

import (
	"encoding/json"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestParsePromptDoneUsage(t *testing.T) {
	t.Parallel()
	if parsePromptDoneUsage(nil) != nil {
		t.Fatal("nil raw should be nil")
	}
	if parsePromptDoneUsage(json.RawMessage(`null`)) != nil {
		t.Fatal("null should be nil")
	}
	empty := parsePromptDoneUsage(json.RawMessage(`{}`))
	if empty == nil {
		t.Fatal("empty object should be present (reported 0)")
	}
	if empty.Total() != 0 {
		t.Fatalf("empty total = %d", empty.Total())
	}

	raw := json.RawMessage(`{
		"model-a": {"inputTokens":10,"outputTokens":5,"cacheReadTokens":2,"cacheWriteTokens":1},
		"model-b": {"inputTokens":3,"outputTokens":7,"cacheReadTokens":0,"cacheWriteTokens":4}
	}`)
	u := parsePromptDoneUsage(raw)
	if u == nil {
		t.Fatal("expected usage")
	}
	if u.InputTokens != 13 || u.OutputTokens != 12 || u.CacheReadTokens != 2 || u.CacheWriteTokens != 5 {
		t.Fatalf("sum = %+v", u)
	}
	if u.Total() != 32 {
		t.Fatalf("total = %d", u.Total())
	}
}

func TestDispatchEventDataPromptDoneUsage(t *testing.T) {
	t.Parallel()
	c := NewACPClient("127.0.0.1", 1)
	r := &ChatResult{}
	frame := json.RawMessage(`{"op":"event","data":{"type":"prompt_done","usage":{"m":{"inputTokens":1,"outputTokens":2,"cacheReadTokens":3,"cacheWriteTokens":4}}}}`)
	if !c.dispatchEventData(frame, r) {
		t.Fatal("prompt_done should signal done")
	}
	if r.Usage == nil || r.Usage.Total() != 10 {
		t.Fatalf("usage = %+v", r.Usage)
	}

	// No usage field → stay nil (not 0).
	r2 := &ChatResult{}
	if !c.dispatchEventData(json.RawMessage(`{"op":"event","data":{"type":"prompt_done"}}`), r2) {
		t.Fatal("prompt_done without usage should still finish")
	}
	if r2.Usage != nil {
		t.Fatalf("missing usage must stay nil, got %+v", r2.Usage)
	}
}

func TestAddTokenUsageAcrossTurns(t *testing.T) {
	t.Parallel()
	var acc *models.TokenUsage
	acc = models.AddTokenUsage(acc, &models.TokenUsage{InputTokens: 10, OutputTokens: 1})
	acc = models.AddTokenUsage(acc, &models.TokenUsage{InputTokens: 5, CacheReadTokens: 2})
	if acc.InputTokens != 15 || acc.OutputTokens != 1 || acc.CacheReadTokens != 2 {
		t.Fatalf("acc = %+v", acc)
	}
	// Explicit zero still establishes presence when starting from nil.
	z := models.AddTokenUsage(nil, &models.TokenUsage{})
	if z == nil || z.Total() != 0 {
		t.Fatalf("zero report = %+v", z)
	}
}
