package sandbox

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestMapGitStatus(t *testing.T) {
	cases := map[string]string{
		"":   "modified",
		"A":  "added",
		"AM": "added",
		"D":  "deleted",
		"M":  "modified",
		"R":  "renamed",
		"C":  "copied",
		"T":  "typechange",
		"X":  "modified",
		"?":  "modified",
	}
	for in, want := range cases {
		if got := mapGitStatus(in); got != want {
			t.Errorf("mapGitStatus(%q)=%q want %q", in, got, want)
		}
	}
}

func TestGitChangesScriptQuotesDir(t *testing.T) {
	script := gitChangesScript(`/tmp/work 'dir'`)
	if !strings.Contains(script, `d=`) {
		t.Fatal("missing d=")
	}
	if !strings.Contains(script, "NONE") || !strings.Contains(script, "printf 'VCS\\tgit\\n'") {
		t.Fatalf("script missing expected markers")
	}
	// shellQuote should wrap/escape the path so the cd line is present.
	if !strings.Contains(script, `cd "$d"`) {
		t.Fatal("expected cd \"$d\"")
	}
}

func TestParseGitChangesNONE(t *testing.T) {
	ch, ok := parseGitChanges("NONE\n")
	if ok || ch != nil {
		t.Fatalf("NONE should fail: ok=%v ch=%v", ok, ch)
	}
}

func TestParseGitChangesMissingVCS(t *testing.T) {
	ch, ok := parseGitChanges("HEAD\tabc\n")
	if ok || ch != nil {
		t.Fatalf("missing VCS marker should fail: ok=%v", ok)
	}
}

func TestParseGitChangesFullReport(t *testing.T) {
	out := strings.Join([]string{
		"VCS\tgit",
		"HEAD\tabc123",
		"BRANCH\tfeature",
		"BASEBRANCH\tmain",
		"BASE\tdef456",
		"DIRTY\t1",
		"PUSHED\t0",
		"REMOTESHA\tremote99",
		"UNPUSHED\t2",
		"AHEAD\t3",
		"COMMIT\tsha1\tAlice\t2024-01-01T00:00:00Z\tfirst commit",
		"COMMIT\tsha2\tBob\t2024-01-02T00:00:00Z\tsecond",
		"NUM\t10\t2\tsrc/a.go",
		"NAME\tM\tsrc/a.go",
		"NAME\tA\tsrc/b.go",
		"NUM\t-\t-\tbin/x",
		"NAME\tD\told.go",
		"UNTRACKED\tnew.txt",
		"UNTRACKED\tsrc/a.go", // already present — ignored
		"",
		"IGNORED\tfoo",
	}, "\n")

	ch, ok := parseGitChanges(out)
	if !ok || ch == nil {
		t.Fatalf("parse failed: ok=%v", ok)
	}
	if ch.VCS != "git" || ch.HeadSHA != "abc123" || ch.Branch != "feature" || ch.BaseBranch != "main" {
		t.Fatalf("meta: %+v", ch)
	}
	if !ch.Dirty || ch.Pushed || ch.RemoteSHA != "remote99" || ch.Unpushed != 2 || ch.Ahead != 3 {
		t.Fatalf("flags: dirty=%v pushed=%v remote=%s unpushed=%d ahead=%d",
			ch.Dirty, ch.Pushed, ch.RemoteSHA, ch.Unpushed, ch.Ahead)
	}
	if !ch.NewBranch {
		t.Fatal("expected NewBranch")
	}
	if len(ch.Commits) != 2 || ch.Commits[0].Subject != "first commit" {
		t.Fatalf("commits: %+v", ch.Commits)
	}
	if len(ch.ChangedFiles) < 4 {
		t.Fatalf("changedFiles=%d %+v", len(ch.ChangedFiles), ch.ChangedFiles)
	}
	byPath := map[string]ChangedFile{}
	for _, f := range ch.ChangedFiles {
		byPath[f.Path] = f
	}
	if byPath["src/a.go"].Status != "modified" || byPath["src/a.go"].Added != 10 {
		t.Fatalf("a.go: %+v", byPath["src/a.go"])
	}
	if byPath["src/b.go"].Status != "added" {
		t.Fatalf("b.go: %+v", byPath["src/b.go"])
	}
	if byPath["old.go"].Status != "deleted" {
		t.Fatalf("old.go: %+v", byPath["old.go"])
	}
	if byPath["new.txt"].Status != "untracked" {
		t.Fatalf("new.txt: %+v", byPath["new.txt"])
	}
	if byPath["bin/x"].Added != 0 {
		t.Fatalf("binary numstat should parse as 0: %+v", byPath["bin/x"])
	}
	if ch.DiffStat == "" || !strings.Contains(ch.DiffStat, "file(s) changed") {
		t.Fatalf("DiffStat=%q", ch.DiffStat)
	}
}

func TestParseGitChangesSameBranchNotNew(t *testing.T) {
	out := "VCS\tgit\nHEAD\th\nBRANCH\tmain\nBASEBRANCH\tmain\nBASE\tb\nDIRTY\t0\nPUSHED\t1\nREMOTESHA\th\nUNPUSHED\t0\nAHEAD\t0\n"
	ch, ok := parseGitChanges(out)
	if !ok {
		t.Fatal("parse")
	}
	if ch.NewBranch {
		t.Fatal("same branch should not be NewBranch")
	}
	if ch.DiffStat != "" || len(ch.ChangedFiles) != 0 {
		t.Fatalf("empty changes expected: %+v", ch)
	}
}

func TestParseGitChangesCRLFAndPartialCOMMIT(t *testing.T) {
	out := "VCS\tgit\r\nHEAD\th\r\nBRANCH\tHEAD\r\nBASEBRANCH\t\r\nCOMMIT\tonly-three\ta\tb\r\nNAME\tM\r\n"
	ch, ok := parseGitChanges(out)
	if !ok {
		t.Fatal("parse")
	}
	if len(ch.Commits) != 0 {
		t.Fatalf("partial COMMIT should be dropped: %+v", ch.Commits)
	}
	if ch.NewBranch {
		t.Fatal("HEAD branch should not be NewBranch")
	}
}

func TestGitChangesViaExecHook(t *testing.T) {
	gw, fg := newInlineGW(t)
	fg.seed("gw-git", "running")
	m := NewManager(gw, ManagerOptions{WorkspaceDir: "/root/workspace"})

	report := "VCS\tgit\nHEAD\th1\nBRANCH\tfeat\nBASEBRANCH\tmain\nBASE\tb1\nDIRTY\t0\nPUSHED\t1\nREMOTESHA\th1\nUNPUSHED\t0\nAHEAD\t1\n"
	restore := SetExecHook(func(_ context.Context, _ string, _ int, command string, _ io.Reader) ([]byte, error) {
		if strings.Contains(command, "bash") || strings.Contains(command, "VCS") || strings.Contains(command, "git") {
			return []byte(report), nil
		}
		return nil, nil
	})
	defer restore()

	sb, err := m.Attach(context.Background(), "gw-git")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	ch, ok := sb.GitChanges(context.Background(), "/root/workspace/repo")
	if !ok || ch == nil || ch.Branch != "feat" {
		t.Fatalf("GitChanges = %+v ok=%v", ch, ok)
	}

	restore2 := SetExecHook(func(_ context.Context, _ string, _ int, _ string, _ io.Reader) ([]byte, error) {
		return []byte("NONE\n"), nil
	})
	defer restore2()
	if _, ok := sb.GitChanges(context.Background(), "/tmp/x"); ok {
		t.Fatal("NONE should yield ok=false")
	}

	restore3 := SetExecHook(func(_ context.Context, _ string, _ int, _ string, _ io.Reader) ([]byte, error) {
		return nil, errors.New("ssh fail")
	})
	defer restore3()
	if _, ok := sb.GitChanges(context.Background(), "/tmp/x"); ok {
		t.Fatal("empty+err should yield ok=false")
	}
}
