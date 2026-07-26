package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillServiceCRUD(t *testing.T) {
	root := t.TempDir()
	s := NewSkillService(root)

	if len(s.List()) != 0 {
		t.Fatal("empty list")
	}
	if s.Exists("nope") {
		t.Fatal("exists false")
	}

	if err := s.Save(Agent{Name: "a1", MCP: DefaultPlatformMCP()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Agent{Name: "a2"}); err != nil {
		t.Fatal(err)
	}
	if got := s.List(); len(got) != 2 || got[0].Name != "a1" {
		t.Fatalf("list sorted: %+v", got)
	}
	if !s.Exists("a1") {
		t.Fatal("a1 should exist")
	}

	// Rename validations.
	if err := s.Rename("", "x"); err == nil {
		t.Fatal("empty rename should error")
	}
	if err := s.Rename("a1", "a1"); err != nil {
		t.Fatal("same-name rename is a no-op")
	}
	if err := s.Rename("ghost", "z"); err == nil {
		t.Fatal("rename missing should error")
	}
	if err := s.Rename("a1", "a2"); err == nil {
		t.Fatal("rename onto existing should error")
	}
	if err := s.Rename("a1", "a1renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if s.Exists("a1") || !s.Exists("a1renamed") {
		t.Fatal("rename did not move dir")
	}

	if err := s.Delete("a1renamed"); err != nil {
		t.Fatal(err)
	}
	if s.Exists("a1renamed") {
		t.Fatal("delete failed")
	}
}

func TestSkillSaveFilesAndWorkDir(t *testing.T) {
	root := t.TempDir()
	s := NewSkillService(root)

	// Save an agent with a working-dir tree; a traversal path is dropped.
	err := s.Save(Agent{
		Name: "coder",
		Env:  map[string]string{"K": "V"},
		Files: []AgentFile{
			{Path: "rules/base.md", Content: "base"},
			{Path: "skills/x/y.md", Content: "nested"},
			{Path: "  ", Content: "blank"}, // rejected (blank)
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if wd := s.WorkDir("coder"); wd == "" {
		t.Fatal("WorkDir should exist after save")
	}
	if s.WorkDir("ghost") != "" {
		t.Error("WorkDir for missing agent should be empty")
	}

	got, ok := s.Get("coder")
	if !ok {
		t.Fatal("Get coder")
	}
	if got.Env["K"] != "V" {
		t.Errorf("env not persisted: %+v", got.Env)
	}
	var paths []string
	for _, f := range got.Files {
		paths = append(paths, f.Path)
	}
	if len(got.Files) != 2 {
		t.Fatalf("expected 2 files (blank dropped), got %v", paths)
	}
}

func TestSafeRel(t *testing.T) {
	cases := map[string]string{
		"a/b.md":    "a/b.md",
		"./a.md":    "a.md",
		"":          "",
		"   ":       "",
		"../x":      "x", // Clean confines to working dir; no leftover ".."
		"/abs/p":    "",  // absolute paths rejected (CodeQL #17/#18)
		"a/../b.md": "b.md",
		"..":        "",
	}
	for in, want := range cases {
		if got := safeRel(in); got != want {
			t.Errorf("safeRel(%q) = %q want %q", in, got, want)
		}
	}
}

func TestSkillMigrateLegacy(t *testing.T) {
	root := t.TempDir()
	// Hand-build a legacy agent: rules.md + skills/ but no cursor/ working dir.
	dir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(filepath.Join(dir, "skills", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules.md"), []byte("legacy rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "s", "recipe.md"), []byte("recipe"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Construction triggers migrateLegacy via EnsureSeed path.
	s := NewSkillService(root)
	s.migrateLegacy()

	// After migration the unified cursor/ working dir exists and legacy files
	// are gone.
	if s.WorkDir("legacy") == "" {
		t.Fatal("legacy agent not migrated to cursor/ working dir")
	}
	if _, err := os.Stat(filepath.Join(dir, "rules.md")); !os.IsNotExist(err) {
		t.Error("legacy rules.md should be removed after migration")
	}
	got, ok := s.Get("legacy")
	if !ok || len(got.Files) == 0 {
		t.Fatalf("migrated agent should expose files: %+v", got)
	}
}

func TestDefaultPlatformMCP(t *testing.T) {
	mcps := DefaultPlatformMCP()
	if len(mcps) != 1 || mcps[0].Name != ArtifactStoreMCP {
		t.Fatalf("default mcp: %+v", mcps)
	}
}
