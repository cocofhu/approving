package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"

	"gorm.io/gorm"
)

func waitNodeStatus(t *testing.T, db *gorm.DB, runID, nodeID, status string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var sr models.StateRun
		if err := db.Where("run_id = ? AND node_id = ?", runID, nodeID).
			Order("iteration desc, id desc").First(&sr).Error; err == nil && sr.Status == status {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("node %s did not reach status %q", nodeID, status)
}

func assertRunVar(t *testing.T, db *gorm.DB, runID, name, want string) {
	t.Helper()
	var v models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", runID, name).First(&v).Error; err != nil {
		t.Fatalf("var %q missing: %v", name, err)
	}
	if v.Value != want {
		t.Fatalf("var %q = %q, want %q", name, v.Value, want)
	}
}

func assertNodeExecuted(t *testing.T, db *gorm.DB, runID, nodeID string, want bool) {
	t.Helper()
	var n int64
	db.Model(&models.StateRun{}).Where("run_id = ? AND node_id = ?", runID, nodeID).Count(&n)
	if want && n == 0 {
		t.Fatalf("expected node %s to execute", nodeID)
	}
	if !want && n > 0 {
		t.Fatalf("expected node %s not to execute, got %d runs", nodeID, n)
	}
}

// TestStructuredGatePassGoto: test node with exits.pass.goto routes directly on pass.
func TestStructuredGatePassGoto(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
				"reason_var": "reason",
				"exits": map[string]any{
					"pass": map[string]any{"goto": "ok"},
					"fail": map[string]any{"goto": "bad"},
				},
			}},
			{ID: "ok", Type: "output", Config: map[string]any{"result": "pass"}},
			{ID: "bad", Type: "output", Config: map[string]any{"result": "fail"}},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "test"}},
	}
	eng, db, _ := setupEngineGraphP(t, g)
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	assertNodeExecuted(t, db, run.ID, "ok", true)
	assertNodeExecuted(t, db, run.ID, "bad", false)
	assertRunVar(t, db, run.ID, "reason", "测试全部通过")
}

// TestStructuredGateFailGoto: fail goto still routes downstream while node stays failed.
func TestStructuredGateFailGoto(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
				"exits": map[string]any{
					"pass": map[string]any{"goto": "ok"},
					"fail": map[string]any{"goto": "fix"},
				},
			}},
			{ID: "ok", Type: "output", Config: map[string]any{"result": "pass"}},
			{ID: "fix", Type: "output", Config: map[string]any{"result": "fix"}},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "test"}},
	}
	eng, db, p := setupEngineGraphP(t, g)
	p.structuredBodies = map[string]string{
		"test": `{"summary":"bad","failed":1,"cases":[{"name":"x","status":"failed"}]}`,
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	waitNodeStatus(t, db, run.ID, "test", "failed")
	assertNodeExecuted(t, db, run.ID, "fix", true)
	assertNodeExecuted(t, db, run.ID, "ok", false)
	assertRunVar(t, db, run.ID, "reason", "测试未通过:1 个用例失败,需修复后重新测试")
	assertRunVar(t, db, run.ID, "last_error", "测试未通过:1 个用例失败,需修复后重新测试")
}

// TestStructuredGateFailWhenFallback: no fail goto falls back to when edge with action guard.
func TestStructuredGateFailWhenFallback(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
				"exits": map[string]any{
					"pass": map[string]any{"goto": ""},
					"fail": map[string]any{"goto": ""},
				},
			}},
			{ID: "ok", Type: "output", Config: map[string]any{"result": "pass"}},
			{ID: "fix", Type: "output", Config: map[string]any{"result": "fix"}},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "test"},
			{ID: "e2", Source: "test", Target: "fix", When: `action == "fail"`, Kind: models.EdgeSuccess},
		},
	}
	eng, db, p := setupEngineGraphP(t, g)
	p.structuredBodies = map[string]string{
		"test": `{"summary":"bad","failed":1,"cases":[{"name":"x","status":"failed"}]}`,
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	assertNodeExecuted(t, db, run.ID, "fix", true)
}

// TestReviewStructuredGateVerdicts exercises approve/reject routing and reason_var.
func TestReviewStructuredGateVerdicts(t *testing.T) {
	base := func(verdict, passGoto, failGoto string) models.Graph {
		return models.Graph{
			Nodes: []models.Node{
				{ID: "input", Type: "input"},
				{ID: "review", Type: "review", Config: map[string]any{
					"agent_profile": "v", "prompt": "评审",
					"reason_var": "review_reason",
					"exits": map[string]any{
						"pass": map[string]any{"goto": passGoto},
						"fail": map[string]any{"goto": failGoto},
					},
				}},
				{ID: "ok", Type: "output", Config: map[string]any{"result": "ok"}},
				{ID: "bad", Type: "output", Config: map[string]any{"result": "bad"}},
			},
			Edges: []models.Edge{{ID: "e1", Source: "input", Target: "review"}},
		}
	}

	t.Run("approve", func(t *testing.T) {
		eng, db, p := setupEngineGraphP(t, base("approve", "ok", "bad"))
		p.structuredBodies = map[string]string{
			"review": `{"summary":"ok","verdict":"approve"}`,
		}
		run, _ := eng.StartRun("wf", nil, "test")
		waitRunStatus(t, db, run.ID, "completed")
		assertNodeExecuted(t, db, run.ID, "ok", true)
		assertRunVar(t, db, run.ID, "review_reason", "评审已通过")
	})

	t.Run("reject goto", func(t *testing.T) {
		eng, db, p := setupEngineGraphP(t, base("reject", "ok", "bad"))
		p.structuredBodies = map[string]string{
			"review": `{"summary":"no","verdict":"reject"}`,
		}
		run, _ := eng.StartRun("wf", nil, "test")
		waitRunStatus(t, db, run.ID, "completed")
		waitNodeStatus(t, db, run.ID, "review", "failed")
		assertNodeExecuted(t, db, run.ID, "bad", true)
		assertRunVar(t, db, run.ID, "review_reason", "评审结论为 reject:方案/实现被否决,需整改后重新评审")
		assertRunVar(t, db, run.ID, "last_error", "评审结论为 reject:方案/实现被否决,需整改后重新评审")
	})
}

// TestLegacyStructuredGateRouting: graphs without exits keep success/failure edges.
func TestLegacyStructuredGateRouting(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Config: map[string]any{"agent_profile": "t", "prompt": "测试"}},
			{ID: "ok", Type: "output"},
			{ID: "bad", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "test"},
			{ID: "e2", Source: "test", Target: "ok", Kind: models.EdgeSuccess},
			{ID: "e3", Source: "test", Target: "bad", Kind: models.EdgeFailure},
		},
	}
	eng, db, p := setupEngineGraphP(t, g)
	p.structuredBodies = map[string]string{
		"test": `{"summary":"bad","failed":1,"cases":[{"name":"x","status":"failed"}]}`,
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	assertNodeExecuted(t, db, run.ID, "bad", true)
	assertNodeExecuted(t, db, run.ID, "ok", false)
	assertRunVar(t, db, run.ID, "reason", "测试未通过:1 个用例失败,需修复后重新测试")
}

// TestStructuredGateMalformed fails closed for bad test_result.json.
func TestStructuredGateMalformed(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
				"exits": map[string]any{
					"pass": map[string]any{"goto": ""},
					"fail": map[string]any{"goto": "bad"},
				},
			}},
			{ID: "bad", Type: "output"},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "test"}},
	}
	eng, db, p := setupEngineGraphP(t, g)
	p.structuredBodies = map[string]string{"test": `{bad`}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	assertNodeExecuted(t, db, run.ID, "bad", true)
}

func TestFinalizeStructuredGateUnit(t *testing.T) {
	e, db := setupEngine(t)
	_ = db
	c := &execCtx{
		run:  &models.Run{ID: "r1"},
		vars: map[string]any{},
	}
	node := &models.Node{
		ID: "t", Type: "test",
		Config: map[string]any{
			"reason_var": "my_reason",
			"exits": map[string]any{
				"pass": map[string]any{"goto": "next"},
				"fail": map[string]any{"goto": "fix"},
			},
		},
	}
	oc := nodeOutcome{status: "completed", outputs: map[string]any{}}
	out := e.finalizeStructuredGate(c, node, oc, nodereg.GateTest)
	if out.outputs["action"] != "pass" {
		t.Fatalf("action = %v", out.outputs["action"])
	}
	if out.goto_ != "next" {
		t.Fatalf("goto = %q", out.goto_)
	}
	if c.vars["my_reason"] != "测试全部通过" {
		t.Fatalf("reason = %v", c.vars["my_reason"])
	}
	if c.vars["action"] != nil {
		t.Fatal("action must not be persisted to vars")
	}

	node2 := &models.Node{ID: "r", Type: "review", Config: map[string]any{
		"exits": map[string]any{
			"pass": map[string]any{"goto": ""},
			"fail": map[string]any{"goto": ""},
		},
	}}
	oc2 := nodeOutcome{status: "failed", err: "reject reason", outputs: map[string]any{}}
	out2 := e.finalizeStructuredGate(c, node2, oc2, nodereg.GateReview)
	if out2.outputs["action"] != "reject" {
		t.Fatalf("action = %v", out2.outputs["action"])
	}
}

// TestStructuredGateRetrySnapshotsScreenshots: a test node that fails its gate,
// rolls back and re-runs, then passes must snapshot EACH attempt's own
// test_result.json — screenshots included — into that iteration's outputs. The
// run-scoped artifact store keeps only the latest test_result.json (same name,
// overwritten each attempt), so a retry that relied on the store would show the
// last attempt's screenshots for every historical tab. Persisting per-iteration
// (outputs.test_result_json) is what makes every retry independently reviewable,
// exactly like the first attempt.
func TestStructuredGateRetrySnapshotsScreenshots(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Checkpoint: true, Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
			}},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "test"},
			{ID: "e2", Source: "test", Target: "output", Kind: models.EdgeSuccess},
			{ID: "erb", Source: "test", Target: "test", Kind: models.EdgeRollback, MaxAttempts: 3},
		},
	}
	eng, db, p := setupEngineGraphP(t, g)
	// Attempt 1 fails the gate (a failed case) and carries screenshot SHOTA;
	// attempt 2 passes and carries a different screenshot SHOTB.
	p.structuredBodySeq = map[string][]string{
		"test": {
			`{"summary":"a1","failed":1,"cases":[{"name":"x","status":"failed"}],"screenshots":[{"data":"SHOTA","mimeType":"image/png","caption":"first"}]}`,
			`{"summary":"a2","passed":1,"screenshots":[{"data":"SHOTB","mimeType":"image/png","caption":"second"}]}`,
		},
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")

	var rows []models.StateRun
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "test").
		Order("iteration asc").Find(&rows).Error; err != nil {
		t.Fatalf("load test state runs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("test execution rows = %d, want 2 (failed attempt + retried pass)", len(rows))
	}
	snap := func(sr models.StateRun) string {
		s, _ := sr.Outputs["test_result_json"].(string)
		return s
	}
	first, second := snap(rows[0]), snap(rows[1])
	if !strings.Contains(first, `"data":"SHOTA"`) {
		t.Errorf("first attempt snapshot missing its own inline screenshot data (SHOTA): %q", first)
	}
	if strings.Contains(first, "SHOTB") {
		t.Errorf("first attempt snapshot leaked the retry's screenshot data (SHOTB): %q", first)
	}
	if !strings.Contains(second, `"data":"SHOTB"`) {
		t.Errorf("retry snapshot missing its own inline screenshot data (SHOTB): %q", second)
	}
	if strings.Contains(first, `"artifact"`) || strings.Contains(second, `"artifact"`) {
		t.Errorf("snapshots should not carry artifact refs after hydrate: first=%q second=%q", first, second)
	}
}

func TestStructuredGateArtifactNames(t *testing.T) {
	if mcp.TestResultArtifactName == "" || mcp.ReviewArtifactName == "" {
		t.Fatal("artifact names should be set")
	}
}

// TestStructuredGateSkippedOnlyPass: skipped-only results pass when block_on_skipped is false (default).
func TestStructuredGateSkippedOnlyPass(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
				"exits": map[string]any{
					"pass": map[string]any{"goto": "ok"},
					"fail": map[string]any{"goto": "bad"},
				},
			}},
			{ID: "ok", Type: "output", Config: map[string]any{"result": "pass"}},
			{ID: "bad", Type: "output", Config: map[string]any{"result": "fail"}},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "test"}},
	}
	eng, db, p := setupEngineGraphP(t, g)
	p.structuredBodies = map[string]string{
		"test": `{"summary":"skipped only","skipped":2,"cases":[{"name":"a","status":"skipped"},{"name":"b","status":"skipped"}]}`,
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	waitNodeStatus(t, db, run.ID, "test", "completed")
	assertNodeExecuted(t, db, run.ID, "ok", true)
	assertNodeExecuted(t, db, run.ID, "bad", false)
	assertRunVar(t, db, run.ID, "reason", "测试全部通过")
}

// TestStructuredGateBlockOnSkipped: skipped cases fail the gate when block_on_skipped is true.
func TestStructuredGateBlockOnSkipped(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
				"block_on_skipped": true,
				"exits": map[string]any{
					"pass": map[string]any{"goto": "ok"},
					"fail": map[string]any{"goto": "bad"},
				},
			}},
			{ID: "ok", Type: "output", Config: map[string]any{"result": "pass"}},
			{ID: "bad", Type: "output", Config: map[string]any{"result": "fail"}},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "test"}},
	}
	eng, db, p := setupEngineGraphP(t, g)
	p.structuredBodies = map[string]string{
		"test": `{"summary":"skipped","skipped":1,"cases":[{"name":"x","status":"skipped"}]}`,
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	waitNodeStatus(t, db, run.ID, "test", "failed")
	assertNodeExecuted(t, db, run.ID, "bad", true)
	assertNodeExecuted(t, db, run.ID, "ok", false)
	assertRunVar(t, db, run.ID, "reason", "测试未通过:1 个用例被跳过,需修复后重新测试")
}

func TestTestGateUnit(t *testing.T) {
	pass, reason := testGate(`{"summary":"s","skipped":1,"cases":[{"name":"x","status":"skipped"}]}`, false, "")
	if !pass || reason != "" {
		t.Errorf("skipped-only default pass: pass=%v reason=%q", pass, reason)
	}
	pass, reason = testGate(`{"summary":"s","skipped":1,"cases":[{"name":"x","status":"skipped"}]}`, true, "")
	if pass || !strings.Contains(reason, "跳过") {
		t.Errorf("block_on_skipped: pass=%v reason=%q", pass, reason)
	}
	pass, reason = testGate(`{"summary":"s","failed":1,"cases":[{"name":"x","status":"failed"}]}`, false, "")
	if pass || !strings.Contains(reason, "失败") {
		t.Errorf("failed blocks: pass=%v reason=%q", pass, reason)
	}
	pass, _ = testGate(`{bad`, false, "")
	if pass {
		t.Error("malformed should fail")
	}

	plan := `{"goals":[{"id":"g1","title":"A","subgoals":[{"id":"g1.1","title":"x"},{"id":"g1.2","title":"y"}]}]}`
	pass, reason = testGate(`{"summary":"s","cases":[{"name":"a","status":"passed"}]}`, false, plan)
	if pass || !strings.Contains(reason, "plan_coverage") {
		t.Errorf("missing plan_coverage should fail: pass=%v reason=%q", pass, reason)
	}
	pass, reason = testGate(`{"summary":"s","plan_coverage":[
		{"plan_id":"g1.1","passed":true,"evidence":"ok"},
		{"plan_id":"g1.2","passed":true,"evidence":"ok"}
	]}`, false, plan)
	if !pass || reason != "" {
		t.Errorf("full coverage should pass: pass=%v reason=%q", pass, reason)
	}
}

func planCoverageGateGraph() models.Graph {
	return models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "plan", Type: "plan", Config: map[string]any{"agent_profile": "p", "prompt": "计划"}},
			{ID: "test", Type: "test", Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
				"exits": map[string]any{
					"pass": map[string]any{"goto": "ok"},
					"fail": map[string]any{"goto": "fix"},
				},
			}},
			{ID: "ok", Type: "output", Config: map[string]any{"result": "pass"}},
			{ID: "fix", Type: "output", Config: map[string]any{"result": "fix"}},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "plan"},
			{ID: "e2", Source: "plan", Target: "test", Kind: models.EdgeSuccess},
		},
	}
}

func TestStructuredGatePlanCoverageMissingFailGoto(t *testing.T) {
	eng, db, p := setupEngineGraphP(t, planCoverageGateGraph())
	p.structuredBodies = map[string]string{
		"plan": `{"goals":[{"id":"g1","title":"A","subgoals":[{"id":"g1.1","title":"x"},{"id":"g1.2","title":"y"}]}]}`,
		"test": `{"summary":"missing coverage","cases":[{"name":"a","status":"passed"}]}`,
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	waitNodeStatus(t, db, run.ID, "test", "failed")
	assertNodeExecuted(t, db, run.ID, "fix", true)
	assertNodeExecuted(t, db, run.ID, "ok", false)
	assertRunVar(t, db, run.ID, "reason", "计划贴合度校验失败:缺少 plan_coverage(有计划叶子时必填)")
}

func TestStructuredGatePlanCoveragePassGoto(t *testing.T) {
	eng, db, p := setupEngineGraphP(t, planCoverageGateGraph())
	p.structuredBodies = map[string]string{
		"plan": `{"goals":[{"id":"g1","title":"A","subgoals":[{"id":"g1.1","title":"x"},{"id":"g1.2","title":"y"}]}]}`,
		"test": `{"summary":"covered","cases":[{"name":"a","status":"passed"}],"plan_coverage":[
			{"plan_id":"g1.1","passed":true,"evidence":"implemented x"},
			{"plan_id":"g1.2","passed":true,"evidence":"implemented y"}
		]}`,
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	waitNodeStatus(t, db, run.ID, "test", "completed")
	assertNodeExecuted(t, db, run.ID, "ok", true)
	assertNodeExecuted(t, db, run.ID, "fix", false)
	assertRunVar(t, db, run.ID, "reason", "测试全部通过")
}

func TestStructuredGatePlanCoverageNoPlanFailOpen(t *testing.T) {
	// No plan node: empty plan.json → coverage not required.
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "test", Type: "test", Config: map[string]any{
				"agent_profile": "t", "prompt": "测试",
				"exits": map[string]any{
					"pass": map[string]any{"goto": "ok"},
					"fail": map[string]any{"goto": "fix"},
				},
			}},
			{ID: "ok", Type: "output", Config: map[string]any{"result": "pass"}},
			{ID: "fix", Type: "output", Config: map[string]any{"result": "fix"}},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "test"}},
	}
	eng, db, p := setupEngineGraphP(t, g)
	p.structuredBodies = map[string]string{
		"test": `{"summary":"no plan","cases":[{"name":"a","status":"passed"}]}`,
	}
	run, _ := eng.StartRun("wf", nil, "test")
	waitRunStatus(t, db, run.ID, "completed")
	assertNodeExecuted(t, db, run.ID, "ok", true)
	assertNodeExecuted(t, db, run.ID, "fix", false)
}
