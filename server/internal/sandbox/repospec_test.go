package sandbox

import (
	"testing"
)

func TestRepoNameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://git.host.cc/org/web.git":  "web",
		"https://git.host.cc/org/web":      "web",
		"git@github.com:org/api.git":       "api",
		"ssh://git@host/org/infra.git/":    "infra",
		"https://git.host.cc/org/web.git/": "web",
		"":                                 "",
		"   ":                              "",
	}
	for in, want := range cases {
		if got := RepoNameFromURL(in); got != want {
			t.Errorf("RepoNameFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReposFromURL(t *testing.T) {
	if r := ReposFromURL(""); r != nil {
		t.Errorf("empty url should yield nil, got %+v", r)
	}
	if r := ReposFromURL("   "); r != nil {
		t.Errorf("blank url should yield nil, got %+v", r)
	}
	r := ReposFromURL("https://h/org/web.git")
	if len(r) != 1 || r[0] != (RepoSpec{Name: "web", URL: "https://h/org/web.git"}) {
		t.Fatalf("ReposFromURL = %+v", r)
	}
	// A URL whose last segment can't be derived falls back to "repo".
	r2 := ReposFromURL("https://host")
	if len(r2) != 1 || r2[0].Name != "host" {
		t.Errorf("host-only url = %+v", r2)
	}
}

func TestEncodeRepos(t *testing.T) {
	// name|url with an optional |branch, comma-separated.
	got := EncodeRepos([]RepoSpec{
		{Name: "web", URL: "https://h/w.git", Branch: "dev"},
		{Name: "api", URL: "https://h/a.git"},
	})
	want := "web|https://h/w.git|dev,api|https://h/a.git"
	if got != want {
		t.Errorf("EncodeRepos = %q, want %q", got, want)
	}

	// Entries missing a name or URL are skipped; surrounding whitespace trimmed.
	got2 := EncodeRepos([]RepoSpec{
		{Name: " web ", URL: " https://h/w.git "},
		{Name: "", URL: "https://h/x.git"},
		{Name: "api", URL: ""},
	})
	if got2 != "web|https://h/w.git" {
		t.Errorf("EncodeRepos (skip/trim) = %q", got2)
	}

	if got3 := EncodeRepos(nil); got3 != "" {
		t.Errorf("nil repos should encode to empty, got %q", got3)
	}
}
