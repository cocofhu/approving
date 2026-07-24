package handlers

import (
	"reflect"
	"testing"

	"github.com/cocofhu/approving/internal/sandbox"
)

func TestResolveTestRepos(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		repos   []testRepoInput
		repoURL string
		want    []sandbox.RepoSpec
	}{
		{
			name: "empty",
			want: nil,
		},
		{
			name:    "repoUrl fallback",
			repoURL: "https://host/org/web.git",
			want:    []sandbox.RepoSpec{{Name: "web", URL: "https://host/org/web.git"}},
		},
		{
			name: "repos priority over repoUrl",
			repos: []testRepoInput{
				{Name: "a", URL: "https://h/a.git"},
			},
			repoURL: "https://ignored/x.git",
			want:    []sandbox.RepoSpec{{Name: "a", URL: "https://h/a.git"}},
		},
		{
			name: "skip partial rows",
			repos: []testRepoInput{
				{Name: "only-name", URL: ""},
				{Name: "", URL: "https://h/u.git"},
				{Name: "ok", URL: "https://h/ok.git"},
			},
			want: []sandbox.RepoSpec{{Name: "ok", URL: "https://h/ok.git"}},
		},
		{
			name: "dedupe by name",
			repos: []testRepoInput{
				{Name: "dup", URL: "https://h/1.git"},
				{Name: "dup", URL: "https://h/2.git"},
				{Name: "other", URL: "https://h/3.git"},
			},
			want: []sandbox.RepoSpec{
				{Name: "dup", URL: "https://h/1.git"},
				{Name: "other", URL: "https://h/3.git"},
			},
		},
		{
			name: "trim and empty branch",
			repos: []testRepoInput{
				{Name: "  web  ", URL: "  https://h/web.git  ", Branch: "  feat  "},
				{Name: "lib", URL: "https://h/lib.git", Branch: "   "},
			},
			want: []sandbox.RepoSpec{
				{Name: "web", URL: "https://h/web.git", Branch: "feat"},
				{Name: "lib", URL: "https://h/lib.git"},
			},
		},
		{
			name: "repos array all invalid falls back to empty list not repoUrl",
			repos: []testRepoInput{
				{Name: "", URL: ""},
			},
			repoURL: "https://h/ignored.git",
			want:    []sandbox.RepoSpec{},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveTestRepos(tc.repos, tc.repoURL)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("resolveTestRepos() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveTestReposEncodeMatchesWorkflow(t *testing.T) {
	t.Parallel()
	repos := resolveTestRepos([]testRepoInput{
		{Name: "web", URL: "https://h/web.git", Branch: "feat/web"},
		{Name: "api", URL: "https://h/api.git"},
	}, "")
	want := "web|https://h/web.git|feat/web,api|https://h/api.git"
	if got := sandbox.EncodeRepos(repos); got != want {
		t.Fatalf("EncodeRepos = %q, want %q", got, want)
	}
}
