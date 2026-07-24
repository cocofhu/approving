package engine

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestBuildOutputCardBranches(t *testing.T) {
	e, db := setupEngine(t)
	run := &models.Run{ID: "run-card", WorkflowID: "wf", Status: "running", Graph: models.Graph{
		Nodes: []models.Node{{ID: "n1", Label: "N1"}, {ID: "n2", Label: "N2"}},
	}}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	db.Create(&models.StateRun{RunID: run.ID, NodeID: "n1", Status: "failed", Iteration: 1})
	db.Create(&models.StateRun{RunID: run.ID, NodeID: "n2", Status: "succeeded", Iteration: 1})

	c := &execCtx{
		run:   run,
		graph: run.Graph,
		nodeOutputs: map[string]map[string]any{
			"n2": {
				"content":      "hello",
				"page":         "<html/>",
				"plan":         "# plan",
				"plan_json":    `{"goals":[]}`,
				"custom":       "x",
				"empty_custom": "  ",
			},
		},
	}

	// failed upstream
	card := e.buildOutputCard(c, 1, "{{nodes.n1.outputs.content}}")
	if card["status"] != "failed" {
		t.Fatalf("failed node: %+v", card)
	}
	// missing outputs
	c2 := &execCtx{run: run, graph: run.Graph, nodeOutputs: map[string]map[string]any{}}
	card = e.buildOutputCard(c2, 1, "{{nodes.n2.outputs.content}}")
	if card["status"] != "failed" {
		t.Fatalf("no outs: %+v", card)
	}
	// missing key
	card = e.buildOutputCard(c, 1, "{{nodes.n2.outputs.missing}}")
	if card["status"] != "failed" {
		t.Fatalf("missing key: %+v", card)
	}
	// content ok
	card = e.buildOutputCard(c, 1, "{{nodes.n2.outputs.content}}")
	if card["status"] != "ok" || card["typeTag"] != "Markdown" {
		t.Fatalf("content: %+v", card)
	}
	// page is a structured artifact key
	card = e.buildOutputCard(c, 1, "{{nodes.n2.outputs.page}}")
	if card["status"] != "ok" || card["structuredArtifactName"] != "page.html" {
		t.Fatalf("page: %+v", card)
	}
	card = e.buildOutputCard(c, 1, "{{nodes.n2.outputs.plan}}")
	if card["typeTag"] != "结构化产物" {
		t.Fatalf("plan: %+v", card)
	}
	card = e.buildOutputCard(c, 1, "{{nodes.n2.outputs.empty_custom}}")
	if card["status"] != "failed" {
		t.Fatalf("empty custom: %+v", card)
	}
	card = e.buildOutputCard(c, 1, "{{nodes.n2.outputs.custom}}")
	if card["status"] != "ok" {
		t.Fatalf("custom: %+v", card)
	}
	card = e.buildOutputCard(c, 1, `{{artifact("missing.json")}}`)
	if card["status"] != "failed" {
		t.Fatalf("artifact miss: %+v", card)
	}
	card = e.buildOutputCard(c, 1, "plain-text")
	if card["status"] != "ok" && card["status"] != "failed" {
		t.Fatalf("unknown tmpl: %+v", card)
	}
}
