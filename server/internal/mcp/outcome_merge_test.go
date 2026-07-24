package mcp

import "testing"

func TestParseOutcomeChecksAndMergeOutputs(t *testing.T) {
	if parseOutcomeChecks(nil) != nil {
		t.Fatal("nil")
	}
	if parseOutcomeChecks("bad") != nil {
		t.Fatal("non-slice")
	}
	checks := parseOutcomeChecks([]any{
		"skip",
		map[string]any{"name": "  ", "passed": true},
		map[string]any{"name": "c1", "passed": true, "detail": " ok "},
		map[string]any{"name": "c2", "passed": false},
	})
	if len(checks) != 2 || checks[0].Name != "c1" || !checks[0].Passed || checks[0].Detail != "ok" {
		t.Fatalf("%+v", checks)
	}
	o, err := ParseNodeOutcome(map[string]any{
		"status":  "success",
		"summary": "s",
		"outputs": map[string]any{"x": 1},
		"checks":  []any{map[string]any{"name": "c1", "passed": true}},
	})
	if err != nil || len(o.Checks) != 1 || o.Outputs["x"] != 1 {
		t.Fatalf("%+v %v", o, err)
	}
	if js := OutcomeJSON(o); js == "{}" || js == "" {
		t.Fatalf("OutcomeJSON=%q", js)
	}

	out := MergeOutcomeOutputs(nil, NodeOutcome{
		Status:  "success",
		Summary: "done",
		Outputs: map[string]any{"a": 1},
	})
	if out["a"] != 1 || out["outcome_summary"] != "done" || out["outcome_status"] != "success" {
		t.Fatalf("%+v", out)
	}
	dst := map[string]any{"keep": true}
	out2 := MergeOutcomeOutputs(dst, NodeOutcome{Outputs: map[string]any{"b": 2}})
	if out2["keep"] != true || out2["b"] != 2 {
		t.Fatalf("%+v", out2)
	}
	if _, ok := out2["outcome_summary"]; ok {
		t.Fatal("empty summary should not set key")
	}
}
