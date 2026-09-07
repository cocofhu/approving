package services

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestFirstInstallEmbedFSHasWorkspaceMarkdown(t *testing.T) {
	for _, name := range OnboardingAgentNames {
		root := path.Join(firstInstallEmbedRoot, "folder", name, WorkDirName)
		md := 0
		err := fs.WalkDir(firstInstallEmbedFS, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(p, ".md") {
				md++
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s workspace walk: %v", name, err)
		}
		if md == 0 {
			t.Fatalf("%s workspace has no .md files in embed FS (check .dockerignore allowlist)", name)
		}
		agent, err := loadFirstInstallAgentTemplate(name)
		if err != nil {
			t.Fatalf("loadFirstInstallAgentTemplate(%s): %v", name, err)
		}
		if len(agent.Files) == 0 {
			t.Fatalf("%s template Files empty after embed load", name)
		}
		for _, f := range agent.Files {
			if strings.Contains(f.Content, "git.woa.com") || strings.Contains(f.Content, "git.cocofhu.cc") {
				t.Fatalf("%s file %s contains internal git host", name, f.Path)
			}
		}
	}
	env, err := loadFirstInstallWorkflowEnvelope()
	if err != nil {
		t.Fatalf("workflow envelope: %v", err)
	}
	if env.Name != OnboardingWorkflowName {
		t.Fatalf("workflow name = %q", env.Name)
	}
}

// The shipped templates are public: no internal hosts, regions, or project names.
func TestFirstInstallEmbedCarriesNoBusinessIdentifiers(t *testing.T) {
	denied := []string{
		"git.woa.com",
		"git.cocofhu.cc",
		"CODEBUDDY_REGION",
		"skillhub",
	}
	err := fs.WalkDir(firstInstallEmbedFS, firstInstallEmbedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := firstInstallEmbedFS.ReadFile(p)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(b))
		for _, bad := range denied {
			if strings.Contains(lower, strings.ToLower(bad)) {
				t.Errorf("%s contains business identifier %q", p, bad)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk first-install embed: %v", err)
	}
}

// Users pick their own repo at launch; shipping a prefilled name leaks the origin project.
func TestFirstInstallWorkflowReposAreBlank(t *testing.T) {
	env, err := loadFirstInstallWorkflowEnvelope()
	if err != nil {
		t.Fatalf("workflow envelope: %v", err)
	}
	var repos *models.Variable
	for i := range env.Graph.Variables {
		if env.Graph.Variables[i].Name == "repos" {
			repos = &env.Graph.Variables[i]
			break
		}
	}
	if repos == nil {
		t.Fatal("default workflow has no repos variable")
	}
	items, ok := repos.Value.([]any)
	if !ok {
		t.Fatalf("repos value is %T, want a list", repos.Value)
	}
	for i, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("repos[%d] is %T, want an object", i, item)
		}
		for _, field := range []string{"url", "name", "branch"} {
			if got, _ := row[field].(string); strings.TrimSpace(got) != "" {
				t.Errorf("repos[%d].%s = %q, want blank so the user fills it at launch", i, field, got)
			}
		}
	}
}
