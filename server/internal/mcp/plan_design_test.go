package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParsePlanGoalsOnly(t *testing.T) {
	doc, err := parsePlan(map[string]any{
		"title": "P",
		"goals": []any{map[string]any{"title": "G1", "subgoals": []any{map[string]any{"title": "S1"}}}},
	})
	if err != nil {
		t.Fatalf("goals-only: %v", err)
	}
	if doc.Architecture != nil || doc.DataDesign != nil || doc.Interaction != nil || doc.TestDesign != "" {
		t.Fatalf("design sections should be absent: %+v", doc)
	}
	md := RenderPlanMarkdown(string(mustPlanJSON(doc)))
	if strings.Contains(md, "设计区") || strings.Contains(md, "Architecture") {
		t.Fatalf("goals-only markdown should not include design section:\n%s", md)
	}
	if !strings.Contains(md, "G1") {
		t.Fatalf("markdown missing goal:\n%s", md)
	}
}

func TestParsePlanFullSixSections(t *testing.T) {
	doc, err := parsePlan(map[string]any{
		"title": "完整",
		"architecture": map[string]any{
			"summary": "arch",
			"diagram": map[string]any{"source": "flowchart LR\n  A-->B", "caption": "架构"},
		},
		"data_design": map[string]any{
			"summary": "data",
			"entities": []any{map[string]any{
				"name": "planDoc",
				"fields": []any{
					map[string]any{"name": "title", "type": "string"},
					map[string]any{"name": "goals", "type": "json"},
				},
				"attributes": []any{"title", "goals"},
			}},
			"diagram": map[string]any{"format": "mermaid", "source": "erDiagram\n  A ||--o{ B : has"},
		},
		"interfaces": []any{map[string]any{"name": "set_plan", "kind": "software", "summary": "写入"}},
		"components": []any{map[string]any{"name": "plan.go", "responsibility": "parse"}},
		"interaction": map[string]any{
			"summary": "flow",
			"diagram": map[string]any{"source": "sequenceDiagram\n  A->>B: hi"},
		},
		"test_design": "S1-S7",
		"goals":       []any{map[string]any{"title": "G", "subgoals": []any{map[string]any{"title": "S"}}}},
	})
	if err != nil {
		t.Fatalf("full plan: %v", err)
	}
	if doc.Architecture == nil || doc.Architecture.Diagram == nil || doc.Architecture.Diagram.Format != "mermaid" {
		t.Fatalf("architecture diagram format default: %+v", doc.Architecture)
	}
	if doc.DataDesign == nil || len(doc.DataDesign.Entities) != 1 || doc.DataDesign.Entities[0].Name != "planDoc" {
		t.Fatalf("data_design: %+v", doc.DataDesign)
	}
	if len(doc.DataDesign.Entities[0].Fields) != 2 {
		t.Fatalf("fields: %+v", doc.DataDesign.Entities[0].Fields)
	}
	if len(doc.Interfaces) != 1 || doc.Interfaces[0].Name != "set_plan" {
		t.Fatalf("interfaces: %+v", doc.Interfaces)
	}
	if len(doc.Components) != 1 || doc.Components[0].Name != "plan.go" {
		t.Fatalf("components: %+v", doc.Components)
	}
	if doc.Interaction == nil || doc.Interaction.Diagram == nil {
		t.Fatalf("interaction: %+v", doc.Interaction)
	}
	if doc.TestDesign != "S1-S7" {
		t.Fatalf("test_design=%q", doc.TestDesign)
	}
	md := RenderPlanMarkdown(string(mustPlanJSON(doc)))
	for _, want := range []string{"设计区", "Architecture", "Data design", "Interfaces", "Components", "Interaction", "Test design", "flowchart LR", "field `title`"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestParsePlanAllNotApplicable(t *testing.T) {
	doc, err := parsePlan(map[string]any{
		"architecture": map[string]any{"summary": "不涉及"},
		"data_design":  map[string]any{"summary": "不涉及"},
		"interfaces":   []any{map[string]any{"name": "不涉及", "summary": "无对外接口"}},
		"components":   []any{map[string]any{"name": "不涉及"}},
		"interaction":  map[string]any{"summary": "不涉及"},
		"test_design":  "不涉及",
		"goals":        []any{map[string]any{"title": "G"}},
	})
	if err != nil {
		t.Fatalf("all NA: %v", err)
	}
	if doc.Architecture.Summary != "不涉及" || doc.TestDesign != "不涉及" {
		t.Fatalf("NA placeholders lost: %+v", doc)
	}
	md := RenderPlanMarkdown(string(mustPlanJSON(doc)))
	if !strings.Contains(md, "不涉及") {
		t.Fatalf("markdown should show 不涉及:\n%s", md)
	}
}

func TestParsePlanDiagramEmptySource(t *testing.T) {
	_, err := parsePlan(map[string]any{
		"architecture": map[string]any{"summary": "a", "diagram": map[string]any{"source": "  "}},
		"goals":        []any{map[string]any{"title": "G"}},
	})
	if err == nil || !strings.Contains(err.Error(), "architecture.diagram.source") {
		t.Fatalf("want source error, got %v", err)
	}
}

func TestParsePlanInterfaceMissingName(t *testing.T) {
	_, err := parsePlan(map[string]any{
		"interfaces": []any{map[string]any{"summary": "x"}},
		"goals":      []any{map[string]any{"title": "G"}},
	})
	if err == nil || !strings.Contains(err.Error(), "interfaces[0]") {
		t.Fatalf("want name error, got %v", err)
	}
}

func TestParsePlanEntityMissingName(t *testing.T) {
	_, err := parsePlan(map[string]any{
		"data_design": map[string]any{"summary": "d", "entities": []any{map[string]any{"description": "x"}}},
		"goals":       []any{map[string]any{"title": "G"}},
	})
	if err == nil || !strings.Contains(err.Error(), "data_design.entities[0]") {
		t.Fatalf("want entity name error, got %v", err)
	}
}

func TestApplyPlanStatusPreservesDesign(t *testing.T) {
	doc, err := parsePlan(map[string]any{
		"architecture": map[string]any{"summary": "keep-me", "diagram": map[string]any{"source": "flowchart LR\n  A-->B"}},
		"test_design":  "T",
		"goals": []any{map[string]any{
			"title": "G",
			"subgoals": []any{
				map[string]any{"title": "S1"},
				map[string]any{"title": "S2"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !applyPlanStatus(&doc, "g1.1", planStatusDone) {
		t.Fatal("status apply failed")
	}
	if doc.Architecture == nil || doc.Architecture.Summary != "keep-me" || doc.Architecture.Diagram == nil {
		t.Fatalf("design lost after status update: %+v", doc.Architecture)
	}
	if doc.TestDesign != "T" {
		t.Fatalf("test_design lost: %q", doc.TestDesign)
	}
	if doc.Goals[0].Subgoals[0].Status != planStatusDone {
		t.Fatalf("leaf status not updated: %+v", doc.Goals[0].Subgoals)
	}
	leaves := planLeafIDs(doc)
	if len(leaves) != 2 || leaves[0] != "g1.1" || leaves[1] != "g1.2" {
		t.Fatalf("leaves=%v", leaves)
	}
}

func TestPlanCoverageDenominatorIgnoresDesign(t *testing.T) {
	planJSON := `{
  "architecture": {"summary": "a", "diagram": {"source": "flowchart LR\n  A-->B"}},
  "data_design": {"summary": "不涉及"},
  "interfaces": [{"name": "不涉及"}],
  "components": [{"name": "不涉及"}],
  "interaction": {"summary": "不涉及"},
  "test_design": "不涉及",
  "goals": [{"id":"g1","title":"G","status":"pending","subgoals":[
    {"id":"g1.1","title":"S1","status":"pending"},
    {"id":"g1.2","title":"S2","status":"pending"}
  ]}]
}`
	leaves := PlanLeafIDs(planJSON)
	if len(leaves) != 2 {
		t.Fatalf("coverage denominator want 2 got %d (%v)", len(leaves), leaves)
	}
	ok, reason := PlanCoverageOK(`{"plan_coverage":[
		{"plan_id":"g1.1","passed":true,"evidence":"ok"},
		{"plan_id":"g1.2","passed":true,"evidence":"ok"}
	]}`, planJSON)
	if !ok {
		t.Fatalf("coverage should pass: %s", reason)
	}
}

func TestParsePlanDataDesignHardGate(t *testing.T) {
	base := map[string]any{
		"goals": []any{map[string]any{"title": "G"}},
	}
	substantive := map[string]any{
		"summary": "用户库表",
		"entities": []any{map[string]any{
			"name": "User",
			"fields": []any{
				map[string]any{"name": "id", "type": "uuid", "pk": true},
				map[string]any{"name": "email", "type": "string"},
			},
		}},
		"diagram": map[string]any{"source": "erDiagram\n  USER ||--o{ ORDER : places"},
	}

	t.Run("goals-only passes", func(t *testing.T) {
		if _, err := parsePlan(map[string]any{"goals": base["goals"]}); err != nil {
			t.Fatalf("goals-only: %v", err)
		}
	})

	t.Run("NA exempt", func(t *testing.T) {
		args := map[string]any{
			"data_design": map[string]any{"summary": "不涉及"},
			"goals":       base["goals"],
		}
		if _, err := parsePlan(args); err != nil {
			t.Fatalf("NA: %v", err)
		}
	})

	t.Run("NA N/A exempt", func(t *testing.T) {
		args := map[string]any{
			"data_design": map[string]any{"summary": "N/A"},
			"goals":       base["goals"],
		}
		if _, err := parsePlan(args); err != nil {
			t.Fatalf("N/A: %v", err)
		}
	})

	t.Run("substantive ok", func(t *testing.T) {
		args := map[string]any{"data_design": substantive, "goals": base["goals"]}
		doc, err := parsePlan(args)
		if err != nil {
			t.Fatalf("ok: %v", err)
		}
		if len(doc.DataDesign.Entities[0].Fields) != 2 {
			t.Fatalf("fields: %+v", doc.DataDesign.Entities[0].Fields)
		}
	})

	t.Run("missing diagram", func(t *testing.T) {
		dd := copyMap(substantive)
		delete(dd, "diagram")
		_, err := parsePlan(map[string]any{"data_design": dd, "goals": base["goals"]})
		if err == nil || !strings.Contains(err.Error(), "data_design.diagram.source") {
			t.Fatalf("want diagram error, got %v", err)
		}
	})

	t.Run("empty entities", func(t *testing.T) {
		dd := copyMap(substantive)
		dd["entities"] = []any{}
		_, err := parsePlan(map[string]any{"data_design": dd, "goals": base["goals"]})
		if err == nil || !strings.Contains(err.Error(), "data_design.entities") {
			t.Fatalf("want entities error, got %v", err)
		}
	})

	t.Run("empty fields", func(t *testing.T) {
		dd := copyMap(substantive)
		dd["entities"] = []any{map[string]any{"name": "User", "fields": []any{}}}
		_, err := parsePlan(map[string]any{"data_design": dd, "goals": base["goals"]})
		if err == nil || !strings.Contains(err.Error(), "data_design.entities[0].fields") {
			t.Fatalf("want fields error, got %v", err)
		}
	})

	t.Run("legacy attributes only", func(t *testing.T) {
		dd := map[string]any{
			"summary":  "db",
			"entities": []any{map[string]any{"name": "User", "attributes": []any{"id", "email"}}},
			"diagram":  map[string]any{"source": "erDiagram\n  USER {}"},
		}
		_, err := parsePlan(map[string]any{"data_design": dd, "goals": base["goals"]})
		if err == nil || !strings.Contains(err.Error(), "data_design.entities[0].fields") {
			t.Fatalf("want fields error for legacy attrs, got %v", err)
		}
	})

	t.Run("field missing name", func(t *testing.T) {
		dd := copyMap(substantive)
		dd["entities"] = []any{map[string]any{
			"name":   "User",
			"fields": []any{map[string]any{"type": "string"}},
		}}
		_, err := parsePlan(map[string]any{"data_design": dd, "goals": base["goals"]})
		if err == nil || !strings.Contains(err.Error(), "fields[0].name") {
			t.Fatalf("want name error, got %v", err)
		}
	})

	t.Run("field missing type", func(t *testing.T) {
		dd := copyMap(substantive)
		dd["entities"] = []any{map[string]any{
			"name":   "User",
			"fields": []any{map[string]any{"name": "id"}},
		}}
		_, err := parsePlan(map[string]any{"data_design": dd, "goals": base["goals"]})
		if err == nil || !strings.Contains(err.Error(), "fields[0].type") {
			t.Fatalf("want type error, got %v", err)
		}
	})
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mustPlanJSON(doc planDoc) []byte {
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return b
}
