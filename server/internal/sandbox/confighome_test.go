package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildConfigHome(t *testing.T) {
	// A verbatim-copied agent working dir (rules + a nested file).
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "AGENTS.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "skills", "s.md"), []byte("skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	HomeBaseDir = t.TempDir()
	t.Cleanup(func() { HomeBaseDir = "" })

	dir, err := BuildConfigHome(ConfigHomeSpec{
		WorkDirSrc: src,
		EmbeddedRules: []string{
			"rules/react.md", "rules/plan.md", "rules/implement.md",
			"rules/research.md", "rules/test.md", "rules/review.md", "rules/proposal.md",
		},
		IncludeArtifactStore: true,
		MCP: []MCPServerSpec{
			{Name: "artifact-store", URL: "http://host:9099", Headers: map[string]string{"Authorization": "Bearer x"}},
			{Name: "local", Command: "node", Args: []string{"server.js"}, Env: map[string]string{"K": "V"}},
			{Name: ""},           // skipped (no name)
			{Name: "empty-drop"}, // skipped (no url/command)
		},
	})
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Verbatim copy landed.
	if b, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(b) != "hi" {
		t.Error("agent working dir not copied verbatim")
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "s.md")); err != nil {
		t.Error("nested skill file not copied")
	}
	// Base rule always present; a node-specific rule too.
	for _, r := range []string{"rules/base.md", "rules/react.md", "rules/implement.md"} {
		if _, err := os.Stat(filepath.Join(dir, r)); err != nil {
			t.Errorf("missing embedded rule %s: %v", r, err)
		}
	}
	// mcp.json contains only the two valid servers.
	b, err := os.ReadFile(filepath.Join(dir, "mcp.json"))
	if err != nil {
		t.Fatalf("mcp.json: %v", err)
	}
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("mcp.json decode: %v", err)
	}
	if len(doc.MCPServers) != 2 {
		t.Errorf("mcp servers = %d, want 2", len(doc.MCPServers))
	}
	// HTTP MCP entries must carry type:http (Claude Code / CodeBuddy skip url-only).
	for name, raw := range doc.MCPServers {
		var entry map[string]any
		if err := json.Unmarshal(raw, &entry); err != nil {
			t.Fatalf("server %s: %v", name, err)
		}
		if _, hasURL := entry["url"]; !hasURL {
			continue
		}
		if entry["type"] != "http" {
			t.Errorf("server %s: type=%v, want http (url-only is skipped by codebuddy)", name, entry["type"])
		}
	}
}

func TestBuildConfigHomeHTTPTypeRequired(t *testing.T) {
	HomeBaseDir = ""
	dir, err := BuildConfigHome(ConfigHomeSpec{
		MCP: []MCPServerSpec{{
			Name: "artifact-store",
			URL:  "http://host.docker.internal:8080/mcp/runs/r1",
			Headers: map[string]string{
				"Authorization": "Bearer tok",
			},
		}},
	})
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	b, err := os.ReadFile(filepath.Join(dir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	got := doc.MCPServers["artifact-store"]
	if got["type"] != "http" {
		t.Fatalf("mcp.json missing type:http: %s", b)
	}
	if got["url"] == nil || got["headers"] == nil {
		t.Fatalf("mcp.json incomplete: %s", b)
	}
}

func TestBuildConfigHomeMinimal(t *testing.T) {
	HomeBaseDir = ""
	dir, err := BuildConfigHome(ConfigHomeSpec{}) // no workdir, no MCP, no extra rules
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if _, err := os.Stat(filepath.Join(dir, "rules/base.md")); err != nil {
		t.Errorf("base rule missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp.json")); !os.IsNotExist(err) {
		t.Error("mcp.json should be absent with no MCP servers")
	}
}

func TestBuildConfigHomeSettings(t *testing.T) {
	HomeBaseDir = ""
	dir, err := BuildConfigHome(ConfigHomeSpec{
		Settings: map[string]any{
			"envRouteMode": "staging",
			"endpoint":     "https://staging-codebuddy.tencent.com",
			"env": map[string]string{
				"CODEBUDDY_INTERNET_ENVIRONMENT": "public",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["envRouteMode"] != "staging" {
		t.Fatalf("settings=%v", got)
	}
	if got["endpoint"] != "https://staging-codebuddy.tencent.com" {
		t.Fatalf("endpoint=%v", got["endpoint"])
	}
	envNested, ok := got["env"].(map[string]any)
	if !ok || envNested["CODEBUDDY_INTERNET_ENVIRONMENT"] != "public" {
		t.Fatalf("settings.env=%v", got["env"])
	}
}

func TestBuildConfigHomeSettingsMergeUserWins(t *testing.T) {
	HomeBaseDir = ""
	src := t.TempDir()
	userSettings := `{"env":{"ANTHROPIC_API_KEY":"user-key"},"custom":true}`
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(userSettings), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err := BuildConfigHome(ConfigHomeSpec{
		WorkDirSrc: src,
		Settings: map[string]any{
			"envRouteMode": "staging",
			"endpoint":     "https://staging-codebuddy.tencent.com",
			"env": map[string]string{
				"CODEBUDDY_INTERNET_ENVIRONMENT": "public",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	env, ok := got["env"].(map[string]any)
	if !ok {
		t.Fatalf("env=%v", got["env"])
	}
	if env["ANTHROPIC_API_KEY"] != "user-key" {
		t.Fatalf("user env key lost: %v", env)
	}
	if env["CODEBUDDY_INTERNET_ENVIRONMENT"] != "public" {
		t.Fatalf("platform env not merged: %v", env)
	}
	if got["envRouteMode"] != "staging" {
		t.Fatalf("platform field missing: %v", got)
	}
	if got["custom"] != true {
		t.Fatalf("user custom field lost: %v", got)
	}
}

func TestBuildConfigHomeNoSettingsWhenEmpty(t *testing.T) {
	HomeBaseDir = ""
	dir, err := BuildConfigHome(ConfigHomeSpec{
		Settings: map[string]any{},
	})
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("empty Settings must not write settings.json")
	}
}

func TestBuildConfigHomeRulePriority(t *testing.T) {
	HomeBaseDir = t.TempDir()
	t.Cleanup(func() { HomeBaseDir = "" })

	profiles := filepath.Join(t.TempDir(), "profiles")
	global := filepath.Join(t.TempDir(), "global")
	agent := "TestAgent"
	overrideDir := filepath.Join(profiles, agent, "platform-rules")
	if err := os.MkdirAll(overrideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(global, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(global, "test.md"), []byte("global-test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overrideDir, "test.md"), []byte("override-test"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, err := BuildConfigHome(ConfigHomeSpec{
		EmbeddedRules:  []string{"rules/test.md"},
		AgentName:      agent,
		ProfilesRoot:   profiles,
		GlobalRulesDir: global,
	})
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	b, err := os.ReadFile(filepath.Join(dir, "rules/test.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "override-test" {
		t.Fatalf("override priority: got %q", b)
	}

	dir2, err := BuildConfigHome(ConfigHomeSpec{
		EmbeddedRules:  []string{"rules/test.md"},
		AgentName:      "OtherAgent",
		ProfilesRoot:   profiles,
		GlobalRulesDir: global,
	})
	if err != nil {
		t.Fatalf("BuildConfigHome global: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir2) })
	b, err = os.ReadFile(filepath.Join(dir2, "rules/test.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "global-test" {
		t.Fatalf("global priority: got %q", b)
	}

	dir3, err := BuildConfigHome(ConfigHomeSpec{
		EmbeddedRules: []string{"rules/research.md"},
	})
	if err != nil {
		t.Fatalf("BuildConfigHome embed: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir3) })
	b, err = os.ReadFile(filepath.Join(dir3, "rules/research.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("embed fallback empty")
	}
}

func TestBuildConfigHomeArtifactStoreConditional(t *testing.T) {
	HomeBaseDir = ""
	dir, err := BuildConfigHome(ConfigHomeSpec{IncludeArtifactStore: false})
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if _, err := os.Stat(filepath.Join(dir, "rules/artifact-store.md")); !os.IsNotExist(err) {
		t.Fatal("artifact-store should be omitted when IncludeArtifactStore=false")
	}

	dir2, err := BuildConfigHome(ConfigHomeSpec{IncludeArtifactStore: true})
	if err != nil {
		t.Fatalf("BuildConfigHome: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir2) })
	if _, err := os.Stat(filepath.Join(dir2, "rules/artifact-store.md")); err != nil {
		t.Fatalf("artifact-store missing when enabled: %v", err)
	}
}
