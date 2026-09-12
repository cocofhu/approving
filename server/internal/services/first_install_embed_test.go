package services

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/cocofhu/grasp/internal/models"
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
		if got := agent.Env["GIT_REPOS"]; got != "${vars.repos}" {
			t.Fatalf("%s GIT_REPOS = %q, want ${vars.repos}", name, got)
		}
		for _, f := range agent.Files {
			if looksLikeCloneURL(f.Content) {
				t.Fatalf("%s file %s contains a clone URL; first-install files must stay host-agnostic", name, f.Path)
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

func looksLikeCloneURL(s string) bool {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "git@") {
		return true
	}
	if strings.Contains(lower, "heroku") {
		return true
	}
	return strings.Contains(lower, "://") && (strings.Contains(lower, ".git") || strings.Contains(lower, "github.com") || strings.Contains(lower, "gitlab"))
}

func looksLikeSecretLiteral(s string) bool {
	if strings.Contains(s, "BEGIN ") && strings.Contains(s, "PRIVATE KEY") {
		return true
	}
	for _, key := range []string{"GITHUB_TOKEN", "GITLAB_TOKEN", "GRASP_CURSOR_API_KEY", "GRASP_CODEBUDDY_API_KEY"} {
		if i := strings.Index(s, key); i >= 0 {
			rest := strings.TrimSpace(s[i+len(key):])
			if strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, "\":") {
				return true
			}
		}
	}
	return false
}

// Shipped templates are public: no clone URLs, tokens, or private keys.
func TestFirstInstallEmbedCarriesNoSecretsOrHosts(t *testing.T) {
	err := fs.WalkDir(firstInstallEmbedFS, firstInstallEmbedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := firstInstallEmbedFS.ReadFile(p)
		if err != nil {
			return err
		}
		body := string(b)
		if looksLikeCloneURL(body) {
			t.Errorf("%s contains a clone URL", p)
		}
		if looksLikeSecretLiteral(body) {
			t.Errorf("%s contains a credential literal", p)
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
