package engine

import (
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func reposFixture() []any {
	return []any{
		map[string]any{"name": "alpha", "url": "https://h/alpha.git", "branch": "feat/a"},
		map[string]any{"name": "beta", "url": "https://h/beta.git", "branch": "feat/b"},
		map[string]any{"name": "gamma", "url": "https://h/gamma.git", "branch": "feat/c"},
	}
}

func submitMRGraphWith(repoCfg string, vars []models.Variable) models.Graph {
	cfg := map[string]any{"target_branch": "main"}
	if repoCfg != "" {
		cfg["repo"] = repoCfg
	}
	return models.Graph{
		Variables: vars,
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "mr", Type: "submit_mr", Config: cfg},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "mr"},
			{ID: "e2", Source: "mr", Target: "output"},
		},
	}
}

func TestSubmitMRListModeOrderAndLastURL(t *testing.T) {
	g := submitMRGraphWith("{{vars.repos}}", []models.Variable{
		{Name: "repos", Type: "repos", Value: reposFixture()},
	})
	eng, db, p := setupEngineGraphP(t, g)
	p.mrURLByRepo = map[string]string{
		"alpha": "http://gitlab/mr/a",
		"beta":  "http://gitlab/mr/b",
		"gamma": "http://gitlab/mr/c",
	}
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")

	p.mu.Lock()
	calls := append([]string{}, p.mrRepoCalls...)
	p.mu.Unlock()
	want := []string{"alpha", "beta", "gamma"}
	if len(calls) != 3 || calls[0] != want[0] || calls[1] != want[1] || calls[2] != want[2] {
		t.Fatalf("repo call order = %v, want %v", calls, want)
	}

	var rv models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", run.ID, "mr_url").First(&rv).Error; err != nil {
		t.Fatalf("mr_url: %v", err)
	}
	if rv.Value != "http://gitlab/mr/c" {
		t.Fatalf("mr_url last-wins = %v, want http://gitlab/mr/c", rv.Value)
	}

	var st models.StateRun
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "mr").First(&st).Error; err != nil {
		t.Fatalf("state: %v", err)
	}
	if !strings.Contains(st.OutputMd, "alpha") || !strings.Contains(st.OutputMd, "http://gitlab/mr/a") {
		t.Fatalf("outputMd should summarize repos: %s", st.OutputMd)
	}
}

func TestSubmitMRListModeFailFast(t *testing.T) {
	g := submitMRGraphWith("{{vars.repos}}", []models.Variable{
		{Name: "repos", Type: "repos", Value: reposFixture()},
	})
	eng, db, p := setupEngineGraphP(t, g)
	p.mrURL = "http://gitlab/mr/ok"
	p.mrFailOnRepo = "beta"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "failed")

	p.mu.Lock()
	calls := append([]string{}, p.mrRepoCalls...)
	p.mu.Unlock()
	if len(calls) != 2 || calls[0] != "alpha" || calls[1] != "beta" {
		t.Fatalf("fail-fast calls = %v, want [alpha beta] (gamma not run)", calls)
	}

	var st models.StateRun
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "mr").First(&st).Error; err != nil {
		t.Fatalf("state: %v", err)
	}
	if !strings.Contains(st.Error, "beta") {
		t.Fatalf("error should mention failed repo: %q", st.Error)
	}
}

func TestSubmitMRListModeEmptyReposFails(t *testing.T) {
	g := submitMRGraphWith("{{vars.repos}}", []models.Variable{
		{Name: "repos", Type: "repos", Value: []any{}},
	})
	eng, db, p := setupEngineGraphP(t, g)
	p.mrURL = "http://gitlab/mr/1"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "failed")
	p.mu.Lock()
	n := len(p.mrRepoCalls)
	p.mu.Unlock()
	if n != 0 {
		t.Fatalf("should not RunAgent when list empty, calls=%d", n)
	}
}

func TestSubmitMRSingleLiteralAndVar(t *testing.T) {
	vars := []models.Variable{
		{Name: "repos", Type: "repos", Value: reposFixture()},
		{Name: "target_repo", Type: "string", Value: "beta"},
	}

	// Literal name.
	g1 := submitMRGraphWith("alpha", vars)
	eng1, db1, p1 := setupEngineGraphP(t, g1)
	p1.mrURL = "http://gitlab/mr/alpha"
	run1, err := eng1.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start literal: %v", err)
	}
	waitRunStatus(t, db1, run1.ID, "completed")
	p1.mu.Lock()
	calls1 := append([]string{}, p1.mrRepoCalls...)
	p1.mu.Unlock()
	if len(calls1) != 1 || calls1[0] != "alpha" {
		t.Fatalf("literal calls = %v", calls1)
	}

	// String variable.
	g2 := submitMRGraphWith("{{vars.target_repo}}", vars)
	eng2, db2, p2 := setupEngineGraphP(t, g2)
	p2.mrURL = "http://gitlab/mr/beta"
	run2, err := eng2.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start var: %v", err)
	}
	waitRunStatus(t, db2, run2.ID, "completed")
	p2.mu.Lock()
	calls2 := append([]string{}, p2.mrRepoCalls...)
	p2.mu.Unlock()
	if len(calls2) != 1 || calls2[0] != "beta" {
		t.Fatalf("var calls = %v", calls2)
	}
}

func TestSubmitMRSingleNotInReposFails(t *testing.T) {
	g := submitMRGraphWith("ghost", []models.Variable{
		{Name: "repos", Type: "repos", Value: reposFixture()},
	})
	eng, db, p := setupEngineGraphP(t, g)
	p.mrURL = "http://gitlab/mr/1"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "failed")
	p.mu.Lock()
	n := len(p.mrRepoCalls)
	p.mu.Unlock()
	if n != 0 {
		t.Fatalf("should not run agent for unknown repo, calls=%d", n)
	}
	var st models.StateRun
	db.Where("run_id = ? AND node_id = ?", run.ID, "mr").First(&st)
	if !strings.Contains(st.Error, "ghost") {
		t.Fatalf("error should name ghost: %q", st.Error)
	}
}

func TestSubmitMRUnknownVarFails(t *testing.T) {
	g := submitMRGraphWith("{{vars.no_such}}", []models.Variable{
		{Name: "repos", Type: "repos", Value: reposFixture()},
	})
	eng, db, p := setupEngineGraphP(t, g)
	p.mrURL = "http://gitlab/mr/1"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "failed")
	p.mu.Lock()
	n := len(p.mrRepoCalls)
	p.mu.Unlock()
	if n != 0 {
		t.Fatalf("should not run on unknown var, calls=%d", n)
	}
}

func TestSubmitMRBlankPathUnchanged(t *testing.T) {
	// Blank repo: single RunAgent with empty pinned repo (legacy semantics).
	g := submitMRGraphWith("", []models.Variable{
		{Name: "repos", Type: "repos", Value: reposFixture()},
	})
	eng, db, p := setupEngineGraphP(t, g)
	p.mrURL = "http://gitlab/mr/legacy"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")
	p.mu.Lock()
	calls := append([]string{}, p.mrRepoCalls...)
	p.mu.Unlock()
	if len(calls) != 1 || calls[0] != "" {
		t.Fatalf("blank path should one call with empty repo, got %v", calls)
	}
	var rv models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", run.ID, "mr_url").First(&rv).Error; err != nil {
		t.Fatalf("mr_url: %v", err)
	}
	if rv.Value != "http://gitlab/mr/legacy" {
		t.Fatalf("mr_url = %v", rv.Value)
	}
}

func TestSubmitMRSharedBranchesOnList(t *testing.T) {
	cfg := map[string]any{
		"repo":          "{{vars.repos}}",
		"source_branch": "{{vars.feature_branch}}",
		"target_branch": "{{vars.base_branch}}",
	}
	g := models.Graph{
		Variables: []models.Variable{
			{Name: "repos", Type: "repos", Value: reposFixture()[:2]},
			{Name: "feature_branch", Type: "string", Value: "feat/shared"},
			{Name: "base_branch", Type: "string", Value: "develop"},
		},
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "mr", Type: "submit_mr", Config: cfg},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "mr"},
			{ID: "e2", Source: "mr", Target: "output"},
		},
	}
	eng, db, p := setupEngineGraphP(t, g)
	p.mrURL = "http://gitlab/mr/x"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "completed")

	p.mu.Lock()
	calls := append([]string{}, p.mrRepoCalls...)
	srcs := append([]string{}, p.mrSourceCalls...)
	tgts := append([]string{}, p.mrTargetCalls...)
	p.mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("expected 2 repos, got %v", calls)
	}
	for i, src := range srcs {
		if src != "feat/shared" {
			t.Fatalf("source[%d]=%q, want feat/shared", i, src)
		}
	}
	for i, tgt := range tgts {
		if tgt != "develop" {
			t.Fatalf("target[%d]=%q, want develop", i, tgt)
		}
	}
}

func TestSubmitMREmptyInterpResultFails(t *testing.T) {
	g := submitMRGraphWith("{{vars.target_repo}}", []models.Variable{
		{Name: "repos", Type: "repos", Value: reposFixture()},
		{Name: "target_repo", Type: "string", Value: ""},
	})
	eng, db, p := setupEngineGraphP(t, g)
	p.mrURL = "http://gitlab/mr/1"
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitRunStatus(t, db, run.ID, "failed")
	p.mu.Lock()
	n := len(p.mrRepoCalls)
	p.mu.Unlock()
	if n != 0 {
		t.Fatalf("empty interp should not RunAgent, calls=%d", n)
	}
}
