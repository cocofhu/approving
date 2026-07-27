package handlers

import (
	"testing"

	"github.com/cocofhu/approving/internal/engine"
	"github.com/gin-gonic/gin"
)

func TestReactSessionsDTO(t *testing.T) {
	if reactSessionsDTO(nil) != nil {
		t.Fatal("nil snaps → nil DTO")
	}
	if reactSessionsDTO([]engine.ReviewSessionSnapshot{}) != nil {
		t.Fatal("empty snaps → nil DTO")
	}
	out := reactSessionsDTO([]engine.ReviewSessionSnapshot{{
		NodeID:  "react",
		Kind:    "clarify",
		Waiting: 1,
		Busy:    true,
		Items:   []map[string]any{{"id": "q2", "text": "乙"}},
		ActiveItem: map[string]any{
			"id": "q1", "text": "甲",
		},
	}})
	if out == nil {
		t.Fatal("expected DTO")
	}
	node, ok := out["react"].(gin.H)
	if !ok {
		t.Fatalf("node entry type %T", out["react"])
	}
	if node["busy"] != true {
		t.Fatalf("busy=%v", node["busy"])
	}
	if node["waiting"] != 1 {
		t.Fatalf("waiting=%v", node["waiting"])
	}
	if node["kind"] != "clarify" {
		t.Fatalf("kind=%v", node["kind"])
	}
	active, _ := node["activeItem"].(map[string]any)
	if active == nil {
		t.Fatal("missing activeItem")
	}
	if active["text"] != "甲" {
		t.Fatalf("activeItem.text=%v", active["text"])
	}
}
