package engine

import (
	"testing"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
)

func approveOnlyGraph() models.Graph {
	return models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "predev", Type: "approve", Config: map[string]any{"skill_profile": "pm"}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "predev"},
			{ID: "e2", Source: "predev", Target: "output"},
		},
	}
}

func startApproveAndReply(t *testing.T, eng *Engine, db *gorm.DB) *models.Run {
	t.Helper()
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "predev")
	if err := eng.ReactReply(run.ID, "predev", "确认", nil, nil, false); err != nil {
		t.Fatalf("reply: %v", err)
	}
	return run
}

func TestApproveCompletesWithClarifiedAndPlan(t *testing.T) {
	eng, db, _ := setupEngineGraphP(t, approveOnlyGraph())
	run := startApproveAndReply(t, eng, db)
	waitRunStatus(t, db, run.ID, "completed")
	for _, name := range []string{mcp.ClarifiedRequirementArtifactName, mcp.PlanArtifactName} {
		var c int64
		db.Model(&models.Artifact{}).Where("run_id = ? AND name = ?", run.ID, name).Count(&c)
		if c == 0 {
			t.Errorf("expected %s", name)
		}
	}
	var sr models.StateRun
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "predev").Order("iteration desc").First(&sr).Error; err != nil {
		t.Fatalf("state: %v", err)
	}
	if sr.Status != "completed" {
		t.Fatalf("node status=%s", sr.Status)
	}
	if _, ok := sr.Outputs["clarified_requirement"]; !ok {
		t.Error("missing outputs.clarified_requirement")
	}
	if _, ok := sr.Outputs["plan"]; !ok {
		t.Error("missing outputs.plan")
	}
	if _, ok := sr.Outputs["research"]; ok {
		t.Error("optional research should be absent when not written")
	}
}

func TestApproveFailsWithoutPlan(t *testing.T) {
	eng, db, p := setupEngineGraphP(t, approveOnlyGraph())
	p.approveSkipPlan = true
	run := startApproveAndReply(t, eng, db)
	waitRunStatus(t, db, run.ID, "failed")
}

func TestApproveFailsWithoutClarified(t *testing.T) {
	eng, db, p := setupEngineGraphP(t, approveOnlyGraph())
	p.reactSkipProduces = true
	run := startApproveAndReply(t, eng, db)
	waitRunStatus(t, db, run.ID, "failed")
}

func TestApproveOptionalResearchLifted(t *testing.T) {
	eng, db, p := setupEngineGraphP(t, approveOnlyGraph())
	p.approveWriteOptional = true
	run := startApproveAndReply(t, eng, db)
	waitRunStatus(t, db, run.ID, "completed")
	var c int64
	db.Model(&models.Artifact{}).Where("run_id = ? AND name = ?", run.ID, mcp.ResearchArtifactName).Count(&c)
	if c == 0 {
		t.Error("expected research.json")
	}
	var sr models.StateRun
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "predev").Order("iteration desc").First(&sr).Error; err != nil {
		t.Fatalf("state: %v", err)
	}
	if _, ok := sr.Outputs["research"]; !ok {
		t.Error("optional research should be lifted into outputs")
	}
}

func TestApproveIgnoresLeftoverAutoVar(t *testing.T) {
	g := approveOnlyGraph()
	g.Variables = []models.Variable{{Name: "auto_clarify", Type: "bool", Value: true}}
	g.Nodes[1].Config["auto_var"] = "auto_clarify"
	eng, db, _ := setupEngineGraphP(t, g)
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "predev")
	waitRunStatus(t, db, run.ID, "waiting_human")
}

// Upstream plan.json must not satisfy Approve's required set_plan delivery.
func TestApproveRejectsUpstreamPlanArtifact(t *testing.T) {
	eng, db, p := setupEngineGraphP(t, approveOnlyGraph())
	p.approveSkipPlan = true
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "predev")
	_, planBody := fakeStructured("plan")
	if _, err := eng.store.Save(run.ID, "upstream_plan", mcp.PlanArtifactName, "json", planBody); err != nil {
		t.Fatalf("seed upstream plan: %v", err)
	}
	if err := eng.ReactReply(run.ID, "predev", "确认", nil, nil, false); err != nil {
		t.Fatalf("reply: %v", err)
	}
	waitRunStatus(t, db, run.ID, "failed")
}

// Optional research written by another node must not be lifted into Approve outputs.
func TestApproveDoesNotLiftUpstreamOptionalResearch(t *testing.T) {
	eng, db, _ := setupEngineGraphP(t, approveOnlyGraph())
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "predev")
	_, researchBody := fakeStructured("research")
	if _, err := eng.store.Save(run.ID, "upstream_research", mcp.ResearchArtifactName, "json", researchBody); err != nil {
		t.Fatalf("seed upstream research: %v", err)
	}
	if err := eng.ReactReply(run.ID, "predev", "确认", nil, nil, false); err != nil {
		t.Fatalf("reply: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")
	var sr models.StateRun
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "predev").Order("iteration desc").First(&sr).Error; err != nil {
		t.Fatalf("state: %v", err)
	}
	if _, ok := sr.Outputs["research"]; ok {
		t.Error("upstream research must not be lifted into approve outputs")
	}
}
