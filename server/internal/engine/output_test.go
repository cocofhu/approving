package engine

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestResolveOutputResults(t *testing.T) {
	got := resolveOutputResults(map[string]any{
		"results": []any{"{{nodes.a.outputs.plan}}", "{{artifact(\"x.md\")}}"},
	})
	if len(got) != 2 {
		t.Fatalf("results len = %d", len(got))
	}
	fallback := resolveOutputResults(map[string]any{"result": "{{nodes.old.outputs.content}}"})
	if len(fallback) != 1 || fallback[0] != "{{nodes.old.outputs.content}}" {
		t.Fatalf("fallback = %v", fallback)
	}
	if len(resolveOutputResults(map[string]any{})) != 0 {
		t.Fatal("empty config should yield no templates")
	}
}

func TestExecOutputMultiSourceAndFallback(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "research", Type: "research", Label: "技术调研", Config: map[string]any{"skill_profile": "ResearchAgent"}},
			{ID: "agent", Type: "agent", Label: "代码实现", Config: map[string]any{"skill_profile": "ImplementAgent"}},
			{ID: "output", Type: "output", Config: map[string]any{
				"results": []any{
					"{{nodes.research.outputs.research}}",
					"{{nodes.agent.outputs.content}}",
					"{{artifact(\"missing.md\")}}",
				},
			}},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "research"},
			{ID: "e2", Source: "research", Target: "agent"},
			{ID: "e3", Source: "agent", Target: "output"},
		},
	}
	eng, db, fp := setupEngineGraphP(t, g)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")

	var sr models.StateRun
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "output").First(&sr).Error; err != nil {
		t.Fatal(err)
	}
	if sr.OutputMd != outputCompleteMd {
		t.Errorf("outputMd = %q, want fixed complete message", sr.OutputMd)
	}
	if sr.Status != "completed" {
		t.Errorf("output status = %q", sr.Status)
	}
	cards, ok := sr.Outputs["outputCards"].([]any)
	if !ok || len(cards) != 3 {
		t.Fatalf("outputCards = %T %#v", sr.Outputs["outputCards"], sr.Outputs["outputCards"])
	}
	c0, _ := cards[0].(map[string]any)
	if c0["typeTag"] != "结构化产物" {
		t.Errorf("card0 type = %v", c0["typeTag"])
	}
	c2, _ := cards[2].(map[string]any)
	if c2["status"] != "failed" || c2["typeTag"] != "来源失败" {
		t.Errorf("missing artifact card = %#v", c2)
	}
	_ = fp
}

func TestExecOutputLegacyResultFallback(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "agent", Type: "agent", Label: "A", Config: map[string]any{"skill_profile": "ImplementAgent"}},
			{ID: "output", Type: "output", Config: map[string]any{
				"result": "{{nodes.agent.outputs.content}}",
			}},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "agent"},
			{ID: "e2", Source: "agent", Target: "output"},
		},
	}
	eng, db, _ := setupEngineGraphP(t, g)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")
	var sr models.StateRun
	db.Where("run_id = ? AND node_id = ?", run.ID, "output").First(&sr)
	cards := sr.Outputs["outputCards"].([]any)
	if len(cards) != 1 {
		t.Fatalf("expected 1 card from legacy result, got %d", len(cards))
	}
}

func TestExecOutputEmptyResults(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "output", Type: "output", Config: map[string]any{"results": []any{}}},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "output"}},
	}
	eng, db, _ := setupEngineGraphP(t, g)
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	var sr models.StateRun
	db.Where("run_id = ? AND node_id = ?", run.ID, "output").First(&sr)
	if sr.OutputMd != outputCompleteMd {
		t.Errorf("empty results outputMd = %q", sr.OutputMd)
	}
	cards, _ := sr.Outputs["outputCards"].([]any)
	if len(cards) != 0 {
		t.Errorf("expected 0 cards, got %d", len(cards))
	}
}
