package services

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestMigrateLegacyToWorkDir(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "code-agent")
	if err := os.MkdirAll(filepath.Join(agent, "skills", "gitlab"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "rules.md"), []byte("# code-agent\n\n实现代码并 push。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "skills", "gitlab", "SKILL.md"), []byte("---\nname: gitlab\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "agent.json"), []byte(`{"mcp":[{"name":"artifact-store","url":"u"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// NewAgentService runs seed()+migrateLegacy().
	s := NewAgentService(root)

	if _, err := os.Stat(filepath.Join(agent, "rules.md")); !os.IsNotExist(err) {
		t.Fatalf("legacy rules.md should be gone, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(agent, "skills")); !os.IsNotExist(err) {
		t.Fatalf("legacy skills/ should be gone, err=%v", err)
	}
	work := s.WorkDir("code-agent")
	if work == "" {
		t.Fatal("WorkDir should exist after migration")
	}

	a, ok := s.Get("code-agent")
	if !ok {
		t.Fatal("agent not found")
	}
	paths := make([]string, 0, len(a.Files))
	for _, f := range a.Files {
		paths = append(paths, f.Path)
	}
	sort.Strings(paths)
	want := []string{"rules/code-agent.md", "skills/gitlab/SKILL.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("files = %v, want %v", paths, want)
	}
	for _, f := range a.Files {
		if f.Path == "rules/code-agent.md" && !strings.HasPrefix(f.Content, "---") {
			t.Fatalf("rules file should gain frontmatter, got %q", f.Content)
		}
	}
	if len(a.MCP) != 1 || a.MCP[0].Name != "artifact-store" {
		t.Fatalf("mcp not preserved: %+v", a.MCP)
	}
}

func TestMigrateCursorWorkDirOnlyLegacy(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent-a")
	legacy := filepath.Join(agent, legacyWorkDirName)
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "rules.md"), []byte("legacy rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &AgentService{root: root}
	s.migrateCursorWorkDir("agent-a")

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy cursor/ should be removed after migration, err=%v", err)
	}
	workspace := filepath.Join(agent, WorkDirName)
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("workspace/ should exist after migration: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(workspace, "rules.md")); string(b) != "legacy rule" {
		t.Fatalf("migrated content = %q", b)
	}
}

func TestMigrateCursorWorkDirSkipWhenWorkspaceExists(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent-b")
	workspace := filepath.Join(agent, WorkDirName)
	legacy := filepath.Join(agent, legacyWorkDirName)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "keep.md"), []byte("workspace wins"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "old.md"), []byte("cursor stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &AgentService{root: root}
	s.migrateCursorWorkDir("agent-b")

	if b, _ := os.ReadFile(filepath.Join(workspace, "keep.md")); string(b) != "workspace wins" {
		t.Fatalf("workspace content changed: %q", b)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("cursor/ should remain when workspace/ already exists: %v", err)
	}
}

func TestMigrateCursorWorkDirOnlyWorkspace(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent-c")
	workspace := filepath.Join(agent, WorkDirName)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "ok.md"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &AgentService{root: root}
	s.migrateCursorWorkDir("agent-c")

	if wd := s.WorkDir("agent-c"); wd != workspace {
		t.Fatalf("WorkDir = %q, want %q", wd, workspace)
	}
}
