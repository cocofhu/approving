package services

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTeamBootstrap_CreatesRoster(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:team_boot_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	skills := NewAgentService(filepath.Join(root, "profiles"))
	org := NewOrgService(filepath.Join(root, "profiles"), skills)
	projects := NewProjectService(db)
	pm := NewPmService(db, skills)
	team := NewTeamService(projects, skills, org, pm, nil)

	sess, err := team.Bootstrap(context.Background(), TeamBootstrapRequest{
		ProjectName: "TeamProj",
		Prefix:      "Demo",
		PMName:      "Demo项目经理",
		Background:  "demo background for pipeline team",
		AcpBackend:  "cursor",
		APIKey:      "sk-test",
		MCP:         DefaultPlatformMCP(),
		Env:         map[string]string{"GIT_REPOS": "${vars.repos}"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var cur TeamBootstrapSession
	for i := 0; i < 80; i++ {
		cur, err = team.GetSession(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		if cur.Status == "ready" || cur.Status == "failed" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if cur.Status != "ready" {
		t.Fatalf("status=%s err=%s events=%v", cur.Status, cur.Error, cur.Events)
	}
	if cur.PMAgent != "Demo项目经理" {
		t.Fatalf("pm=%s", cur.PMAgent)
	}
	if len(cur.AgentNames) != 10 {
		t.Fatalf("agents=%d want 10: %v", len(cur.AgentNames), cur.AgentNames)
	}
	pmAg, ok := skills.Get("Demo项目经理")
	if !ok {
		t.Fatal("missing PM agent")
	}
	if !agentHasFilePath(pmAg, "rules/role.md") {
		t.Fatalf("PM missing role.md files=%v", filePaths(pmAg))
	}
	if !agentHasFilePath(pmAg, "rules/project-context.md") {
		t.Fatalf("PM missing project-context.md files=%v", filePaths(pmAg))
	}
	if !agentHasFilePath(pmAg, "skills/pm-orchestrate/SKILL.md") {
		t.Fatalf("PM missing orchestrate skill files=%v", filePaths(pmAg))
	}
	ctx := agentFileContent(pmAg, "rules/project-context.md")
	if !strings.Contains(ctx, "demo background for pipeline team") {
		t.Fatalf("project-context missing background: %q", ctx)
	}
	if !strings.Contains(ctx, "alwaysApply: true") {
		t.Fatalf("project-context should be alwaysApply: %q", ctx)
	}
	impl := "Demo实现工程师"
	ag, ok := skills.Get(impl)
	if !ok {
		t.Fatalf("missing %s", impl)
	}
	if ag.ProjectID != cur.ProjectID {
		t.Fatalf("projectId=%s want %s", ag.ProjectID, cur.ProjectID)
	}
	if len(ag.MCP) == 0 || ag.MCP[0].Name != "artifact-store" {
		t.Fatalf("mcp=%+v", ag.MCP)
	}
	doc, err := org.Get()
	if err != nil {
		t.Fatal(err)
	}
	mem := doc.Agents[impl]
	if _, ok := doc.Agents[impl]; !ok || len(mem.GroupIDs) != 1 || mem.GroupIDs[0] != cur.PipelineGroupID {
		t.Fatalf("groups=%v want pipeline %s", mem.GroupIDs, cur.PipelineGroupID)
	}
}

func TestLoadTeamAgentTemplates_AllPackages(t *testing.T) {
	for _, name := range TeamEmbedPackageNames() {
		ag, err := loadTeamAgentTemplate(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(ag.Files) == 0 {
			t.Fatalf("%s: empty workspace files", name)
		}
	}
	pm, err := loadTeamAgentTemplate(TeamPMEmbedName)
	if err != nil {
		t.Fatal(err)
	}
	if !agentHasFilePath(pm, "rules/role.md") || !agentHasFilePath(pm, "skills/pm-orchestrate/SKILL.md") {
		t.Fatalf("PM template incomplete: %v", filePaths(pm))
	}
}

func agentHasFilePath(a Agent, path string) bool {
	for _, f := range a.Files {
		if f.Path == path {
			return true
		}
	}
	return false
}

func agentFileContent(a Agent, path string) string {
	for _, f := range a.Files {
		if f.Path == path {
			return f.Content
		}
	}
	return ""
}

func filePaths(a Agent) []string {
	out := make([]string, 0, len(a.Files))
	for _, f := range a.Files {
		out = append(out, f.Path)
	}
	return out
}

func TestCreateAgentFromTemplate_Conflict(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:team_scope_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	skills := NewAgentService(filepath.Join(root, "profiles"))
	org := NewOrgService(filepath.Join(root, "profiles"), skills)
	projects := NewProjectService(db)
	pm := NewPmService(db, skills)
	team := NewTeamService(projects, skills, org, pm, nil)

	p, err := projects.Create("P1", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = team.CreateAgentFromTemplate(CreateFromTemplateArgs{
		TemplateID: "implement",
		Name:       "X实现工程师",
		ProjectID:  p.ID,
		AcpBackend: "cursor",
		MCP:        DefaultPlatformMCP(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = team.CreateAgentFromTemplate(CreateFromTemplateArgs{
		TemplateID: "implement",
		Name:       "X实现工程师",
		ProjectID:  p.ID,
		AcpBackend: "cursor",
	})
	if !errors.Is(err, ErrTeamAgentConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestSetOrgMembership_ScopeDenied(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:team_scope_org_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	skills := NewAgentService(filepath.Join(root, "profiles"))
	org := NewOrgService(filepath.Join(root, "profiles"), skills)
	projects := NewProjectService(db)
	pm := NewPmService(db, skills)
	team := NewTeamService(projects, skills, org, pm, nil)

	sess, err := team.Bootstrap(context.Background(), TeamBootstrapRequest{
		ProjectName: "ScopeProj",
		Prefix:      "Sc",
		PMName:      "Sc项目经理",
		Background:  "scope test",
		AcpBackend:  "cursor",
		MCP:         DefaultPlatformMCP(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var cur TeamBootstrapSession
	for i := 0; i < 80; i++ {
		cur, err = team.GetSession(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		if cur.Status == "ready" || cur.Status == "failed" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if cur.Status != "ready" {
		t.Fatalf("status=%s err=%s", cur.Status, cur.Error)
	}

	err = team.SetOrgMembership(SetOrgMembershipArgs{
		SessionID: cur.ID,
		AgentName: "Sc实现工程师",
		GroupIDs:  []string{"grp_foreign"},
	})
	if !errors.Is(err, ErrTeamScopeDenied) {
		t.Fatalf("want scope denied, got %v", err)
	}

	err = team.SetOrgMembership(SetOrgMembershipArgs{
		SessionID: cur.ID,
		AgentName: "Sc实现工程师",
		GroupIDs:  []string{cur.PipelineGroupID},
	})
	if err != nil {
		t.Fatalf("valid membership: %v", err)
	}

	_, err = team.CreateAgentFromTemplate(CreateFromTemplateArgs{
		SessionID:  cur.ID,
		TemplateID: "implement",
		Name:       "Other实现工程师",
		ProjectID:  "not-" + cur.ProjectID,
		AcpBackend: "cursor",
	})
	if !errors.Is(err, ErrTeamScopeDenied) {
		t.Fatalf("want project mismatch, got %v", err)
	}
}
