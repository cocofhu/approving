package pmmcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/platformmcp"
	"github.com/cocofhu/approving/internal/services"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAgentFSHost(t *testing.T) (projectID, token string, h *Host, skill *services.SkillService, org *services.OrgService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:pmmcp_agent_fs_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	ps := services.NewProjectService(db)
	p, err := ps.Create("AgentFSProj", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	skill = services.NewSkillService(root)
	org = services.NewOrgService(root, skill)
	for _, name := range []string{"leader", "alice", "bob", "outsider"} {
		if err := skill.Save(services.Agent{Name: name, ProjectID: p.ID}); err != nil {
			t.Fatal(err)
		}
	}
	// Cross-project agent
	if err := skill.Save(services.Agent{Name: "otherproj", ProjectID: "other-project"}); err != nil {
		t.Fatal(err)
	}
	if _, err := org.Put(services.AgentOrg{
		Agents: map[string]services.OrgAgentMembership{
			"leader":   {},
			"alice":    {ParentAgent: "leader"},
			"bob":      {ParentAgent: "alice"}, // indirect
			"outsider": {},
		},
	}, 0); err != nil {
		t.Fatal(err)
	}

	pm := services.NewPmService(db, skill)
	en := true
	agent := "leader"
	if _, err := pm.UpdateBinding(p.ID, &en, &agent, []string{"pm-agent-fs"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	h = NewHost(pm, services.NewPmProgress(pm, nil, nil), nil, nil, services.NewArtifactService(db), nil)
	h.SetOrgAndSkill(org, skill)
	tok := platformmcp.NewToken()
	h.Restore(p.ID, "thr-fs", "alice-user", "leader", tok)
	return p.ID, tok, h, skill, org
}

func callAgentFSTool(t *testing.T, h *Host, projectID, token, tool string, args map[string]any) (status int, result map[string]any, isError bool, raw string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	st, resp := h.ServeRPC(projectID, MCPAgentFS, token, body)
	raw = string(resp)
	var rpc struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &rpc); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, resp)
	}
	isError = rpc.Result.IsError
	text := ""
	if len(rpc.Result.Content) > 0 {
		text = rpc.Result.Content[0].Text
	}
	_ = json.Unmarshal([]byte(text), &result)
	if result == nil {
		result = map[string]any{"_raw": text}
	}
	return st, result, isError, raw
}

func TestPmGetOrgRelations(t *testing.T) {
	pid, tok, h, _, _ := setupAgentFSHost(t)
	st, result, isErr, raw := callAgentFSTool(t, h, pid, tok, "pm_get_org", map[string]any{})
	if st != 200 || isErr {
		t.Fatalf("status=%d isErr=%v raw=%s", st, isErr, raw)
	}
	if result["leader"] != "leader" {
		t.Fatalf("leader=%v raw=%s", result["leader"], raw)
	}
	agents, _ := result["agents"].([]any)
	rel := map[string]string{}
	for _, a := range agents {
		m, _ := a.(map[string]any)
		rel[m["name"].(string)] = m["relation"].(string)
	}
	if rel["leader"] != "self" || rel["alice"] != "direct" || rel["bob"] != "indirect" || rel["outsider"] != "other" {
		t.Fatalf("relations=%v", rel)
	}
}

func TestPmFSDirectIndirectSelfWrite(t *testing.T) {
	pid, tok, h, skill, _ := setupAgentFSHost(t)
	for _, agent := range []string{"leader", "alice", "bob"} {
		st, result, isErr, raw := callAgentFSTool(t, h, pid, tok, "pm_fs_write", map[string]any{
			"agentName": agent,
			"path":      "AGENTS.md",
			"content":   "edited-by-leader:" + agent,
		})
		if st != 200 || isErr {
			t.Fatalf("%s write failed: %s result=%v", agent, raw, result)
		}
		got, err := skill.ReadWorkspaceFile(agent, "AGENTS.md")
		if err != nil || got != "edited-by-leader:"+agent {
			t.Fatalf("%s disk=%q err=%v", agent, got, err)
		}
		// agent.json must stay untouched (MCP/meta preserved)
		cfg := skill.Get // compile check
		_ = cfg
		ag, ok := skill.Get(agent)
		if !ok || ag.ProjectID != pid {
			t.Fatalf("%s project lost", agent)
		}
		b, _ := os.ReadFile(filepath.Join(skill.WorkDir(agent), "..", "agent.json"))
		if !strings.Contains(string(b), `"projectId"`) {
			t.Fatalf("agent.json missing projectId for %s: %s", agent, b)
		}
	}
}

func TestPmFSRejectNonReportAndCrossProject(t *testing.T) {
	pid, tok, h, _, _ := setupAgentFSHost(t)
	_, result, isErr, _ := callAgentFSTool(t, h, pid, tok, "pm_fs_write", map[string]any{
		"agentName": "outsider",
		"path":      "AGENTS.md",
		"content":   "nope",
	})
	if !isErr {
		t.Fatalf("outsider should be rejected: %v", result)
	}
	if !strings.Contains(result["error"].(string), "not self or a direct/indirect") {
		t.Fatalf("error=%v", result["error"])
	}
	_, result, isErr, _ = callAgentFSTool(t, h, pid, tok, "pm_fs_read", map[string]any{
		"agentName": "otherproj",
		"path":      "AGENTS.md",
	})
	if !isErr || !strings.Contains(result["error"].(string), "not in project") {
		t.Fatalf("cross-project: %v", result)
	}
}

func TestPmFSPathEscapeRejected(t *testing.T) {
	pid, tok, h, _, _ := setupAgentFSHost(t)
	_, result, isErr, _ := callAgentFSTool(t, h, pid, tok, "pm_fs_write", map[string]any{
		"agentName": "alice",
		"path":      "../escape.md",
		"content":   "x",
	})
	if !isErr || !strings.Contains(result["error"].(string), "invalid workspace path") {
		t.Fatalf("escape: %v", result)
	}
}

func TestPmFSDisabledMCP(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:pmmcp_agent_fs_off?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.AutoMigrate(models.AllModels()...)
	ps := services.NewProjectService(db)
	p, _ := ps.Create("Off", "", nil, nil)
	pm := services.NewPmService(db, nil)
	en := true
	agent := "leader"
	if _, err := pm.UpdateBinding(p.ID, &en, &agent, []string{"pm-progress"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	h := NewHost(pm, services.NewPmProgress(pm, nil, nil), nil, nil, services.NewArtifactService(db), nil)
	tok := platformmcp.NewToken()
	h.Restore(p.ID, "t", "u", agent, tok)
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "pm_get_org", "arguments": map[string]any{}},
	})
	st, resp := h.ServeRPC(p.ID, MCPAgentFS, tok, body)
	if st != 404 || !strings.Contains(string(resp), "mcp disabled") {
		t.Fatalf("want disabled: %d %s", st, resp)
	}
}

func TestPmFSTooLargeRejected(t *testing.T) {
	pid, tok, h, _, _ := setupAgentFSHost(t)
	big := strings.Repeat("x", services.WorkspaceFileMaxBytes+1)
	_, result, isErr, _ := callAgentFSTool(t, h, pid, tok, "pm_fs_write", map[string]any{
		"agentName": "alice",
		"path":      "big.md",
		"content":   big,
	})
	if !isErr || !strings.Contains(result["error"].(string), "1MiB") {
		t.Fatalf("size: %v", result)
	}
}

func TestPmFSListDeleteMkdirRename(t *testing.T) {
	pid, tok, h, skill, _ := setupAgentFSHost(t)
	callAgentFSTool(t, h, pid, tok, "pm_fs_mkdir", map[string]any{"agentName": "bob", "path": "rules"})
	_, _, isErr, raw := callAgentFSTool(t, h, pid, tok, "pm_fs_write", map[string]any{
		"agentName": "bob", "path": "rules/a.md", "content": "rule-a",
	})
	if isErr {
		t.Fatal(raw)
	}
	_, result, isErr, raw := callAgentFSTool(t, h, pid, tok, "pm_fs_list", map[string]any{
		"agentName": "bob", "path": "rules",
	})
	if isErr {
		t.Fatal(raw)
	}
	entries, _ := result["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries=%v", result)
	}
	_, _, isErr, raw = callAgentFSTool(t, h, pid, tok, "pm_fs_rename", map[string]any{
		"agentName": "bob", "path": "rules/a.md", "toPath": "rules/b.md",
	})
	if isErr {
		t.Fatal(raw)
	}
	got, err := skill.ReadWorkspaceFile("bob", "rules/b.md")
	if err != nil || got != "rule-a" {
		t.Fatalf("rename disk=%q err=%v", got, err)
	}
	_, _, isErr, raw = callAgentFSTool(t, h, pid, tok, "pm_fs_delete", map[string]any{
		"agentName": "bob", "path": "rules",
	})
	if isErr {
		t.Fatal(raw)
	}
	if _, err := skill.ReadWorkspaceFile("bob", "rules/b.md"); err == nil {
		t.Fatal("expected deleted")
	}
}

func TestPmAgentFSToolsList(t *testing.T) {
	pid, tok, h, _, _ := setupAgentFSHost(t)
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	st, resp := h.ServeRPC(pid, MCPAgentFS, tok, body)
	if st != 200 {
		t.Fatalf("status=%d", st)
	}
	var listResp struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	_ = json.Unmarshal(resp, &listResp)
	names := map[string]bool{}
	for _, tool := range listResp.Result.Tools {
		names[tool["name"].(string)] = true
	}
	for _, want := range []string{"pm_get_org", "pm_fs_list", "pm_fs_read", "pm_fs_write", "pm_fs_delete", "pm_fs_mkdir", "pm_fs_rename"} {
		if !names[want] {
			t.Fatalf("missing tool %s in %v", want, names)
		}
	}
}
