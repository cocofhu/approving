package services

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

// TestOnboardingEmbedFSHasWorkspaceMarkdown guards the GHCR regression where
// .dockerignore excluded **/*.md and emptied go:embed workspaces.
func TestOnboardingEmbedFSHasWorkspaceMarkdown(t *testing.T) {
	for _, name := range OnboardingAgentNames {
		root := path.Join(onboardingEmbedRoot, name, WorkDirName)
		md := 0
		err := fs.WalkDir(onboardingEmbedFS, root, func(p string, d fs.DirEntry, err error) error {
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
		agent, err := loadOnboardingAgentTemplate(name)
		if err != nil {
			t.Fatalf("loadOnboardingAgentTemplate(%s): %v", name, err)
		}
		if len(agent.Files) == 0 {
			t.Fatalf("%s template Files empty after embed load", name)
		}
	}
}
