package pmmcp

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestPmStartRunSchemaHasTriggerEnum(t *testing.T) {
	tools := toolSchemas(MCPWorkflowWrite)
	var start map[string]any
	for _, tool := range tools {
		if tool["name"] == "pm_start_run" {
			start = tool
			break
		}
	}
	if start == nil {
		t.Fatal("pm_start_run missing from workflow-write schema")
	}
	schema, _ := start["inputSchema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	trig, _ := props["trigger"].(map[string]any)
	if trig == nil {
		t.Fatalf("trigger property missing: %#v", start)
	}
	rawEnum, _ := trig["enum"].([]string)
	if len(rawEnum) == 0 {
		// JSON-style []any when built as []string may still be []string;
		// also accept []any from map literals.
		if anyEnum, ok := trig["enum"].([]any); ok {
			for _, v := range anyEnum {
				if s, ok := v.(string); ok {
					rawEnum = append(rawEnum, s)
				}
			}
		}
	}
	want := map[string]bool{models.TriggerManual: true, models.TriggerAPI: true, models.TriggerPMMCP: true}
	if len(rawEnum) != 3 {
		t.Fatalf("trigger enum=%v desc=%v", trig["enum"], trig["description"])
	}
	for _, code := range rawEnum {
		if !want[code] {
			t.Fatalf("unexpected enum value %q", code)
		}
	}
	if desc, _ := trig["description"].(string); desc == "" {
		t.Fatal("trigger description required")
	}
}
