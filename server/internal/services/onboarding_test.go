package services_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

func TestOnboardingBootstrapRequiresAPIKey(t *testing.T) {
	svc, projectID := newOnboardingHarness(t)
	_, err := svc.Bootstrap(projectID, services.OnboardingBootstrapRequest{
		AcpBackend: "cursor",
		APIKey:     "  ",
	})
	if err != services.ErrOnboardingAPIKeyRequired {
		t.Fatalf("want ErrOnboardingAPIKeyRequired, got %v", err)
	}
	if n := len(svc.Skills.List()); n != 0 {
		t.Fatalf("no agents should be created without key, got %d", n)
	}
	if n := len(svc.WF.List(projectID)); n != 0 {
		t.Fatalf("no workflows should be created without key, got %d", n)
	}
}

func TestOnboardingBootstrapCreatesAgentsAndPublishedWorkflow(t *testing.T) {
	svc, projectID := newOnboardingHarness(t)
	res, err := svc.Bootstrap(projectID, services.OnboardingBootstrapRequest{
		AcpBackend: "cursor",
		APIKey:     "test-key-cursor",
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !res.Published {
		t.Fatal("expected published workflow")
	}
	if res.WorkflowID == "" {
		t.Fatal("missing workflowId")
	}
	if len(res.AgentIDs) != len(services.OnboardingAgentNames) {
		t.Fatalf("want %d agents, got %v", len(services.OnboardingAgentNames), res.AgentIDs)
	}
	if res.Repos != services.DefaultOnboardingRepos {
		t.Fatalf("repos = %q", res.Repos)
	}
	if !strings.Contains(res.Repos, "heroku/nodejs-getting-started") {
		t.Fatalf("repos must point at heroku well-known source: %q", res.Repos)
	}
	if strings.Contains(strings.ToLower(res.Repos), "approving-demo") {
		t.Fatal("must not use approving-demo")
	}

	p, ok := svc.Projects.Get(projectID)
	if !ok {
		t.Fatal("project missing")
	}
	foundKey := false
	for _, e := range p.SandboxEnv {
		if e.Key == "APPROVING_CURSOR_API_KEY" && e.Value == "test-key-cursor" && e.Secret {
			foundKey = true
		}
	}
	if !foundKey {
		t.Fatalf("project sandboxEnv missing auth key: %+v", p.SandboxEnv)
	}

	for _, name := range services.OnboardingAgentNames {
		a, ok := svc.Skills.Get(name)
		if !ok {
			t.Fatalf("agent %s missing", name)
		}
		if a.ProjectID != projectID {
			t.Fatalf("agent %s projectId = %q", name, a.ProjectID)
		}
		if a.Env["GIT_REPOS"] != "${vars.repos}" {
			t.Fatalf("agent %s GIT_REPOS = %q", name, a.Env["GIT_REPOS"])
		}
		if a.AcpBackend != "cursor" {
			t.Fatalf("agent %s backend = %q", name, a.AcpBackend)
		}
	}

	wf, ok := svc.WF.Get(res.WorkflowID)
	if !ok {
		t.Fatal("workflow missing")
	}
	if wf.Name != services.OnboardingWorkflowName || wf.Status != "published" || !wf.NeedsRepo {
		t.Fatalf("workflow meta: name=%s status=%s needsRepo=%v", wf.Name, wf.Status, wf.NeedsRepo)
	}
	assertOnboardingGraph(t, wf.Graph)
	assertOnboardingReposVar(t, wf.Graph)
}

func TestOnboardingBootstrapWritesCodeBuddyRegionToAgents(t *testing.T) {
	svc, projectID := newOnboardingHarness(t)
	res, err := svc.Bootstrap(projectID, services.OnboardingBootstrapRequest{
		AcpBackend: "codebuddy",
		APIKey:     "cb-key",
		Region:     "internal",
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	for _, name := range res.AgentIDs {
		a, ok := svc.Skills.Get(name)
		if !ok {
			t.Fatalf("agent %s missing", name)
		}
		if a.AcpBackend != "codebuddy" {
			t.Fatalf("agent %s backend = %q", name, a.AcpBackend)
		}
		if got := a.Env["APPROVING_CODEBUDDY_REGION"]; got != "internal" {
			t.Fatalf("agent %s region = %q, want internal", name, got)
		}
		if a.Layout.ConfigRoot != "/root/.codebuddy" {
			t.Fatalf("agent %s configRoot = %q", name, a.Layout.ConfigRoot)
		}
	}
	p, _ := svc.Projects.Get(projectID)
	foundRegion := false
	for _, e := range p.SandboxEnv {
		if e.Key == "APPROVING_CODEBUDDY_REGION" && e.Value == "internal" {
			foundRegion = true
		}
	}
	if !foundRegion {
		t.Fatalf("project sandboxEnv missing region: %+v", p.SandboxEnv)
	}
}

func TestOnboardingBootstrapDefaultsPublicRegionForCodeBuddy(t *testing.T) {
	svc, projectID := newOnboardingHarness(t)
	_, err := svc.Bootstrap(projectID, services.OnboardingBootstrapRequest{
		AcpBackend: "codebuddy",
		APIKey:     "cb-key",
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	a, ok := svc.Skills.Get("ImplementAgent")
	if !ok {
		t.Fatal("ImplementAgent missing")
	}
	if got := a.Env["APPROVING_CODEBUDDY_REGION"]; got != "public" {
		t.Fatalf("default region = %q, want public", got)
	}
}

func TestOnboardingBootstrapIdempotent(t *testing.T) {
	svc, projectID := newOnboardingHarness(t)
	req := services.OnboardingBootstrapRequest{AcpBackend: "cursor", APIKey: "k1"}
	r1, err := svc.Bootstrap(projectID, req)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	req.APIKey = "k2-rotated"
	r2, err := svc.Bootstrap(projectID, req)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if r1.WorkflowID != r2.WorkflowID {
		t.Fatalf("workflow id changed: %s vs %s", r1.WorkflowID, r2.WorkflowID)
	}
	if len(svc.Skills.List()) != len(services.OnboardingAgentNames) {
		t.Fatalf("agents doubled: %d", len(svc.Skills.List()))
	}
	if n := len(svc.WF.List(projectID)); n != 1 {
		t.Fatalf("workflows doubled: %d", n)
	}
	p, _ := svc.Projects.Get(projectID)
	for _, e := range p.SandboxEnv {
		if e.Key == "APPROVING_CURSOR_API_KEY" && e.Value != "k2-rotated" {
			t.Fatalf("auth not updated: %q", e.Value)
		}
	}
}

func TestOnboardingBootstrapRejectsCrossProjectAgentConflict(t *testing.T) {
	svc, projectA := newOnboardingHarness(t)
	_, err := svc.Bootstrap(projectA, services.OnboardingBootstrapRequest{
		AcpBackend: "cursor",
		APIKey:     "key-a",
	})
	if err != nil {
		t.Fatalf("bootstrap A: %v", err)
	}
	projectB, err := svc.Projects.Create("Other", "", nil, nil)
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	_, err = svc.Bootstrap(projectB.ID, services.OnboardingBootstrapRequest{
		AcpBackend: "cursor",
		APIKey:     "key-b",
	})
	if !errors.Is(err, services.ErrOnboardingAgentConflict) {
		t.Fatalf("want ErrOnboardingAgentConflict, got %v", err)
	}
	if n := len(svc.WF.List(projectB.ID)); n != 0 {
		t.Fatalf("project B must not get workflow on conflict, got %d", n)
	}
	pB, _ := svc.Projects.Get(projectB.ID)
	for _, e := range pB.SandboxEnv {
		if e.Key == "APPROVING_CURSOR_API_KEY" {
			t.Fatal("project B auth must not be written on agent conflict")
		}
	}
	for _, name := range services.OnboardingAgentNames {
		a, ok := svc.Skills.Get(name)
		if !ok {
			t.Fatalf("agent %s missing", name)
		}
		if a.ProjectID != projectA {
			t.Fatalf("agent %s rebound to %q", name, a.ProjectID)
		}
	}
}

func TestOnboardingBootstrapAllowsClaimingUnboundAgents(t *testing.T) {
	svc, projectID := newOnboardingHarness(t)
	unbound := services.Agent{
		Name:       "ClarifyAgent",
		AcpBackend: "cursor",
		ProjectID:  "",
		Files:      []services.AgentFile{{Path: "AGENTS.md", Content: "# unbound\n"}},
		Env:        map[string]string{},
	}
	if err := svc.Skills.Save(unbound); err != nil {
		t.Fatalf("seed unbound: %v", err)
	}
	res, err := svc.Bootstrap(projectID, services.OnboardingBootstrapRequest{
		AcpBackend: "cursor",
		APIKey:     "claim-key",
	})
	if err != nil {
		t.Fatalf("bootstrap should claim unbound: %v", err)
	}
	if len(res.AgentIDs) != len(services.OnboardingAgentNames) {
		t.Fatalf("want %d agents, got %v", len(services.OnboardingAgentNames), res.AgentIDs)
	}
	a, ok := svc.Skills.Get("ClarifyAgent")
	if !ok || a.ProjectID != projectID {
		t.Fatalf("ClarifyAgent not claimed: ok=%v projectId=%q", ok, a.ProjectID)
	}
}

func TestOnboardingLightGraphValidate(t *testing.T) {
	g := services.BuildOnboardingLightGraphForTest(
		services.DefaultOnboardingRepos,
		services.DefaultOnboardingFeature,
	)
	if err := g.Validate(); err != nil {
		t.Fatalf("graph invalid: %v", err)
	}
}

func assertOnboardingReposVar(t *testing.T, g models.Graph) {
	t.Helper()
	var reposVar *models.Variable
	for i := range g.Variables {
		if g.Variables[i].Name == "repos" {
			reposVar = &g.Variables[i]
			break
		}
	}
	if reposVar == nil {
		t.Fatal("missing repos variable")
	}
	if reposVar.Type != "repos" {
		t.Fatalf("repos type = %q, want repos", reposVar.Type)
	}
	list, ok := reposVar.Value.([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("repos value want non-empty []any, got %T %#v", reposVar.Value, reposVar.Value)
	}
	first, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("repos[0] = %T", list[0])
	}
	url, _ := first["url"].(string)
	if !strings.Contains(url, "heroku/nodejs-getting-started") {
		t.Fatalf("repos[0].url = %q", url)
	}
}

func assertOnboardingGraph(t *testing.T, g models.Graph) {
	t.Helper()
	byID := map[string]models.Node{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	for _, id := range []string{"input", "clarify", "visual", "gate", "implement", "test", "review", "preview", "output"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing node %s", id)
		}
	}
	for _, banned := range []string{"research", "proposal", "plan"} {
		for _, n := range g.Nodes {
			if n.Type == banned {
				t.Fatalf("unexpected node type %s", banned)
			}
		}
	}
	if byID["review"].Type != "review" {
		t.Fatalf("review node type = %s", byID["review"].Type)
	}
	// (0,0) is invalid for the canvas session layout (node gets shoved right).
	for _, n := range g.Nodes {
		if n.Position.X == 0 && n.Position.Y == 0 {
			t.Fatalf("node %s has invalid position (0,0)", n.ID)
		}
	}
	gate := byID["gate"]
	if bt, _ := gate.Config["body_template"].(string); bt != "{{nodes.visual.outputs.page}}" {
		t.Fatalf("gate body_template = %v", gate.Config["body_template"])
	}
	gateActions, _ := gate.Config["actions"].([]any)
	var approveGoto, reviseGoto string
	for _, raw := range gateActions {
		a, _ := raw.(map[string]any)
		switch a["id"] {
		case "approve":
			approveGoto, _ = a["goto"].(string)
		case "revise":
			reviseGoto, _ = a["goto"].(string)
		}
	}
	if approveGoto != "implement" || reviseGoto != "visual" {
		t.Fatalf("gate action goto approve=%q revise=%q", approveGoto, reviseGoto)
	}
	testCfg := byID["test"].Config
	exits, _ := testCfg["exits"].(map[string]any)
	fail, _ := exits["fail"].(map[string]any)
	pass, _ := exits["pass"].(map[string]any)
	if fail["goto"] != "implement" {
		t.Fatalf("test fail goto = %v", fail["goto"])
	}
	if pass["goto"] != "review" {
		t.Fatalf("test pass goto = %v want review", pass["goto"])
	}
	revExits, _ := byID["review"].Config["exits"].(map[string]any)
	revPass, _ := revExits["pass"].(map[string]any)
	revFail, _ := revExits["fail"].(map[string]any)
	if revPass["goto"] != "preview" || revFail["goto"] != "implement" {
		t.Fatalf("review exits pass=%v fail=%v", revPass["goto"], revFail["goto"])
	}
	previewActions, _ := byID["preview"].Config["actions"].([]any)
	var previewPassGoto, previewFailGoto string
	for _, raw := range previewActions {
		a, _ := raw.(map[string]any)
		switch a["id"] {
		case "pass":
			previewPassGoto, _ = a["goto"].(string)
		case "fail":
			previewFailGoto, _ = a["goto"].(string)
		}
	}
	if previewPassGoto != "output" || previewFailGoto != "implement" {
		t.Fatalf("preview action goto pass=%q fail=%q", previewPassGoto, previewFailGoto)
	}
	implPrompt, _ := byID["implement"].Config["prompt"].(string)
	if strings.Contains(implPrompt, "get_plan") {
		t.Fatal("light implement prompt must not require get_plan")
	}
	var hasRevise, hasApprove, hasPreviewFail, hasReviewEdge bool
	for _, e := range g.Edges {
		if e.Source == "gate" && e.Target == "visual" && strings.Contains(e.When, "revise") {
			hasRevise = true
		}
		if e.Source == "gate" && e.Target == "implement" && strings.Contains(e.When, "approve") {
			hasApprove = true
		}
		if e.Source == "preview" && e.Target == "implement" {
			hasPreviewFail = true
		}
		if e.Source == "test" && e.Target == "review" {
			hasReviewEdge = true
		}
	}
	if !hasRevise || !hasApprove || !hasPreviewFail || !hasReviewEdge {
		t.Fatalf("missing loops/edges: revise=%v approve=%v previewFail=%v reviewEdge=%v", hasRevise, hasApprove, hasPreviewFail, hasReviewEdge)
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("graph validate: %v", err)
	}
}

func newOnboardingHarness(t *testing.T) (*services.OnboardingService, string) {
	t.Helper()
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "onboarding.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	projects := services.NewProjectService(db)
	p, err := projects.Create("Onboard", "", nil, nil)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	skills := services.NewSkillService(t.TempDir())
	wf := services.NewWorkflowService(db)
	return services.NewOnboardingService(projects, skills, wf), p.ID
}
