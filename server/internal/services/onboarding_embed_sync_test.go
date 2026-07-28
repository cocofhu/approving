package services_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cocofhu/approving/internal/services"
)

// TestOnboardingEmbedMatchesAgentsSource guards against agents/ vs onboarding_embed/ drift
// for bootstrap packages (includes ReviewAgent).
func TestOnboardingEmbedMatchesAgentsSource(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))
	agentsRoot := filepath.Join(repoRoot, "agents")
	embedRoot := filepath.Join(repoRoot, "server/internal/services/onboarding_embed")

	for _, name := range services.OnboardingAgentNames {
		srcDir := filepath.Join(agentsRoot, name)
		embDir := filepath.Join(embedRoot, name)
		if st, err := os.Stat(srcDir); err != nil || !st.IsDir() {
			t.Fatalf("agents/%s missing: %v", name, err)
		}
		if st, err := os.Stat(embDir); err != nil || !st.IsDir() {
			t.Fatalf("onboarding_embed/%s missing: %v", name, err)
		}
		srcFiles := map[string]string{}
		err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(srcDir, p)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			srcFiles[filepath.ToSlash(rel)] = string(b)
			return nil
		})
		if err != nil {
			t.Fatalf("walk agents/%s: %v", name, err)
		}
		embFiles := map[string]string{}
		err = filepath.WalkDir(embDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(embDir, p)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			embFiles[filepath.ToSlash(rel)] = string(b)
			return nil
		})
		if err != nil {
			t.Fatalf("walk embed/%s: %v", name, err)
		}
		if len(srcFiles) != len(embFiles) {
			t.Fatalf("%s file count agents=%d embed=%d", name, len(srcFiles), len(embFiles))
		}
		for rel, content := range srcFiles {
			got, ok := embFiles[rel]
			if !ok {
				t.Fatalf("%s missing in embed: %s", name, rel)
			}
			if got != content {
				t.Fatalf("%s content drift: %s (sync agents/ → onboarding_embed/)", name, rel)
			}
		}
	}
}
