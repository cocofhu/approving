package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVcsMessageReasonSpaces(t *testing.T) {
	subject := formatVcsMessage(VcsCommitMeta{
		Source: VcsSourcePmMCP, Author: "tester", Reason: "init agents md",
		Changes: []VcsPathChange{{Op: "write", Path: "AGENTS.md"}},
	})
	meta := parseVcsMessage("abc", subject)
	if meta.Reason != "init agents md" {
		t.Fatalf("reason=%q subject=%q", meta.Reason, subject)
	}
}

func TestWorkspaceVcsBaselineAndWrite(t *testing.T) {
	root := t.TempDir()
	s := NewSkillService(root)
	if err := s.Save(Agent{Name: "coder", ProjectID: "p1"}); err != nil {
		t.Fatal(err)
	}
	sha, err := s.WriteWorkspaceFileVcs("coder", "AGENTS.md", "# hi", WorkspaceWriteOpts{
		Author: "tester", Source: VcsSourcePmMCP, Reason: "init agents md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sha == "" {
		t.Fatal("expected commit sha")
	}
	revs, err := s.Vcs.ListRevisions("coder", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) < 2 {
		t.Fatalf("expected baseline+write, got %d", len(revs))
	}
	if revs[0].Reason != "init agents md" {
		t.Fatalf("latest reason=%q", revs[0].Reason)
	}
	// sidecar git dir must not live under workspace/
	if _, err := os.Stat(filepath.Join(root, "coder", workspaceVcsDirName)); err != nil {
		t.Fatalf("missing sidecar vcs dir: %v", err)
	}
	ws := filepath.Join(root, "coder", WorkDirName)
	if _, err := os.Stat(filepath.Join(ws, workspaceVcsDirName)); err == nil {
		t.Fatal("vcs dir must not be inside workspace")
	}
}

func TestWorkspaceVcsReasonRequired(t *testing.T) {
	s := NewSkillService(t.TempDir())
	if err := s.Save(Agent{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.WriteWorkspaceFileVcs("a", "x.md", "x", WorkspaceWriteOpts{Author: "u", Source: VcsSourcePmMCP})
	if err != ErrVcsReasonRequired {
		t.Fatalf("got %v", err)
	}
	content, _ := s.ReadWorkspaceFile("a", "x.md")
	if content != "" {
		t.Fatal("write without reason must not persist")
	}
}

func TestWorkspaceVcsRestore(t *testing.T) {
	s := NewSkillService(t.TempDir())
	if err := s.Save(Agent{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	sha1, err := s.WriteWorkspaceFileVcs("a", "a.md", "v1", WorkspaceWriteOpts{
		Author: "u", Source: VcsSourcePmMCP, Reason: "seed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteWorkspaceFileVcs("a", "a.md", "v2", WorkspaceWriteOpts{
		Author: "u", Source: VcsSourcePmMCP, Reason: "change",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreWorkspaceVcs("a", sha1, "u", "rollback"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadWorkspaceFile("a", "a.md")
	if err != nil || got != "v1" {
		t.Fatalf("after restore got=%q err=%v", got, err)
	}
}

func TestWorkspaceVcsRenameAgent(t *testing.T) {
	s := NewSkillService(t.TempDir())
	if err := s.Save(Agent{Name: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteWorkspaceFileVcs("old", "f.md", "x", WorkspaceWriteOpts{
		Author: "u", Source: VcsSourceStudio, Reason: "seed",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("old", "new"); err != nil {
		t.Fatal(err)
	}
	revs, err := s.Vcs.ListRevisions("new", 5)
	if err != nil || len(revs) == 0 {
		t.Fatalf("history after rename=%v err=%v", revs, err)
	}
}

func TestWorkspaceVcsDiffRevision(t *testing.T) {
	s := NewSkillService(t.TempDir())
	if err := s.Save(Agent{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	sha, err := s.WriteWorkspaceFileVcs("a", "a.md", "hello", WorkspaceWriteOpts{
		Author: "u", Source: VcsSourcePmMCP, Reason: "seed",
	})
	if err != nil {
		t.Fatal(err)
	}
	diff, err := s.Vcs.DiffRevision("a", sha)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "hello") {
		t.Fatalf("diff missing content: %q", diff)
	}
	if _, err := s.Vcs.DiffRevision("a", "not-a-sha"); err != ErrVcsRevisionMiss {
		t.Fatalf("want miss, got %v", err)
	}
}

func TestDiffAgentFiles(t *testing.T) {
	before := []AgentFile{{Path: "a.md", Content: "1"}, {Path: "b.md", Content: "2"}}
	after := []AgentFile{{Path: "a.md", Content: "1"}, {Path: "c.md", Content: "3"}}
	changes := DiffAgentFiles(before, after)
	if len(changes) != 2 {
		t.Fatalf("changes=%v", changes)
	}
}
