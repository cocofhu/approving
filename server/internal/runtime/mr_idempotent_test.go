package runtime

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/sandbox"
)

func TestClassifyMRIdempotentFourKinds(t *testing.T) {
	cases := []struct {
		msg  string
		kind string
	}{
		// GitHub-style
		{"a pull request for branch feature/x already exists: https://github.com/o/r/pull/3", MRIdempotentExists},
		{"GraphQL: pull request already exists", MRIdempotentExists},
		{"Error: pull request is already merged https://github.com/o/r/pull/9", MRIdempotentMerged},
		{"No commits between main and feature/x", MRIdempotentNoDiff},
		{"there are no differences between branches", MRIdempotentNoDiff},
		{"duplicate pull request for this head", MRIdempotentDuplicate},
		// GitLab-style
		{"Merge request already exists: https://gitlab.com/g/p/-/merge_requests/4", MRIdempotentExists},
		{"merge request !4 was already merged", MRIdempotentMerged},
		{"nothing to merge; branches are up-to-date", MRIdempotentNoDiff},
		{"Another merge request already exists for source branch", MRIdempotentDuplicate},
		{"glab: already has an open MR for this branch", MRIdempotentDuplicate},
	}
	for _, tc := range cases {
		got, ok := ClassifyMRIdempotent(tc.msg)
		if !ok || got != tc.kind {
			t.Errorf("Classify(%q) = %q,%v want %q,true", tc.msg, got, ok, tc.kind)
		}
	}
	if _, ok := ClassifyMRIdempotent("network timeout dialing api"); ok {
		t.Fatal("unrelated failure must not classify as idempotent")
	}
}

func TestResolveIdempotentMRURL(t *testing.T) {
	u, k, ok := ResolveIdempotentMRURL(
		"pull request already exists https://github.com/o/r/pull/12", "")
	if !ok || k != MRIdempotentExists || u != "https://github.com/o/r/pull/12" {
		t.Fatalf("exists resolve = %q %q %v", u, k, ok)
	}
	u, k, ok = ResolveIdempotentMRURL("No commits between main and feature/x", "")
	if !ok || k != MRIdempotentNoDiff || u != "idempotent:no_diff" {
		t.Fatalf("no_diff resolve = %q %q %v", u, k, ok)
	}
	if _, _, ok := ResolveIdempotentMRURL("already exists", ""); ok {
		t.Fatal("exists without URL must not succeed")
	}
	if _, _, ok := ResolveIdempotentMRURL("https://github.com/o/r/pull/1 only", ""); ok {
		t.Fatal("bare URL without idempotent phrase must not succeed")
	}
}

func TestLookupExistingMRViaCLI_GitHubAndGitLab(t *testing.T) {
	type hit struct {
		url  string
		kind string
	}
	cases := []struct {
		name   string
		repo   string
		script func(body string) ([]byte, error)
		want   hit
	}{
		{
			name: "gh-open-exists",
			repo: "https://github.com/o/r.git",
			script: func(body string) ([]byte, error) {
				if strings.Contains(body, "gh pr list --state open") {
					return []byte(`[{"url":"https://github.com/o/r/pull/7","state":"OPEN","headRefName":"feature-x","baseRefName":"main"}]`), nil
				}
				return []byte("[]"), nil
			},
			want: hit{"https://github.com/o/r/pull/7", MRIdempotentExists},
		},
		{
			name: "gh-merged",
			repo: "https://github.com/o/r.git",
			script: func(body string) ([]byte, error) {
				if strings.Contains(body, "gh pr list --state open") {
					return []byte(`[]`), nil
				}
				if strings.Contains(body, "gh pr list --state merged") {
					return []byte(`[{"url":"https://github.com/o/r/pull/8","state":"MERGED","headRefName":"feature-x","baseRefName":"main"}]`), nil
				}
				return []byte("[]"), nil
			},
			want: hit{"https://github.com/o/r/pull/8", MRIdempotentMerged},
		},
		{
			name: "gl-open-exists",
			repo: "https://gitlab.com/g/p.git",
			script: func(body string) ([]byte, error) {
				if strings.Contains(body, "glab mr list") && !strings.Contains(body, "--merged") {
					return []byte(`[{"web_url":"https://gitlab.com/g/p/-/merge_requests/3","state":"opened","target_branch":"main"}]`), nil
				}
				return []byte("[]"), nil
			},
			want: hit{"https://gitlab.com/g/p/-/merge_requests/3", MRIdempotentExists},
		},
		{
			name: "gl-merged",
			repo: "https://gitlab.com/g/p.git",
			script: func(body string) ([]byte, error) {
				if strings.Contains(body, "glab mr list") && !strings.Contains(body, "--merged") {
					return []byte(`[]`), nil
				}
				if strings.Contains(body, "glab mr list --merged") {
					return []byte(`[{"web_url":"https://gitlab.com/g/p/-/merge_requests/5","state":"merged","target_branch":"main"}]`), nil
				}
				return []byte("[]"), nil
			},
			want: hit{"https://gitlab.com/g/p/-/merge_requests/5", MRIdempotentMerged},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := sandbox.SetExecHook(func(_ context.Context, _ string, _ int, command string, stdin io.Reader) ([]byte, error) {
				return tc.script(execHookBody(command, stdin))
			})
			defer restore()
			sb := &sandbox.Sandbox{Name: "sb", Host: "127.0.0.1", Port: 1, WorkspaceDir: "/root/workspace"}
			u, k, ok := LookupExistingMRViaCLI(context.Background(), sb, "/root/workspace/r", "feature-x", "main", tc.repo, "")
			if !ok || u != tc.want.url || k != tc.want.kind {
				t.Fatalf("got (%q,%q,%v) want (%q,%q,true)", u, k, ok, tc.want.url, tc.want.kind)
			}
		})
	}
}

func TestLookupExistingMRViaCLI_NoDiffAndDuplicateMessages(t *testing.T) {
	// no_diff / duplicate are primarily message-classified; CLI path still
	// returns exists when a PR is listed (duplicate of an open PR).
	restore := sandbox.SetExecHook(func(_ context.Context, _ string, _ int, command string, stdin io.Reader) ([]byte, error) {
		body := execHookBody(command, stdin)
		if strings.Contains(body, "gh pr list --state open") {
			return []byte(`[{"url":"https://github.com/o/r/pull/2","state":"OPEN","headRefName":"feature-x","baseRefName":"main"}]`), nil
		}
		return []byte("[]"), nil
	})
	defer restore()
	sb := &sandbox.Sandbox{Name: "sb", Host: "127.0.0.1", Port: 1}
	u, k, ok := LookupExistingMRViaCLI(context.Background(), sb, "/root/workspace/r", "feature-x", "main", "https://github.com/o/r", "")
	if !ok || k != MRIdempotentExists || u != "https://github.com/o/r/pull/2" {
		t.Fatalf("duplicate/open reuse via CLI = %q %q %v", u, k, ok)
	}

	u, k, ok = ResolveIdempotentMRURL("duplicate submission; already exists https://github.com/o/r/pull/2", "")
	if !ok || k != MRIdempotentDuplicate {
		t.Fatalf("duplicate message = %q %q %v", u, k, ok)
	}
	u, k, ok = ResolveIdempotentMRURL("branches are identical; no changes to submit", "")
	if !ok || k != MRIdempotentNoDiff || u != "idempotent:no_diff" {
		t.Fatalf("no_diff message = %q %q %v", u, k, ok)
	}
}
