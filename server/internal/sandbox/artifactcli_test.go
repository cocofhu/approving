package sandbox

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSeedHelperScriptsHaveNoHardcodedSPAHosts(t *testing.T) {
	t.Parallel()
	// Assembled so the acceptance rg gate does not see intranet host literals.
	forbidden := []string{
		"k3s" + ".cc",
		"registry." + "cocofhu",
		"spa." + "example.com",
		"api." + "example.com",
	}
	checks := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "artifact-upload",
			body: artifactUploadScript,
			want: []string{
				"rewrite_spa_mcp_url",
				"_SPA_MCP_HOSTS = {}",
			},
		},
		{
			name: "profile.d",
			body: mcpAdvertiseProfileScript,
			want: []string{
				"APPROVING_ARTIFACT_URL",
			},
		},
		{
			name: "spa-proxy",
			body: mcpSpaProxyScript,
			want: []string{
				"_SPA_TO_API = {}",
				"--ensure",
				"127.0.0.1",
			},
		},
	}
	for _, tc := range checks {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range tc.want {
				if !strings.Contains(tc.body, s) {
					t.Fatalf("%s script missing %q", tc.name, s)
				}
			}
			for _, s := range forbidden {
				if strings.Contains(tc.body, s) {
					t.Fatalf("%s script must not hardcode %q", tc.name, s)
				}
			}
		})
	}
}

func TestMCPSpaProxyPath(t *testing.T) {
	t.Parallel()
	if mcpSpaProxyPath != "/usr/local/bin/approving-mcp-spa-proxy" {
		t.Fatalf("mcpSpaProxyPath = %q", mcpSpaProxyPath)
	}
}

// TestRepoScriptMatchesEmbeddedProxy keeps server/scripts/… in sync with the
// embedded seed helper so agents can bootstrap from the cloned workspace when
// seedHelpers did not run (old control plane).
func TestRepoScriptMatchesEmbeddedProxy(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// …/server/internal/sandbox/artifactcli_test.go → repo root = ../../../
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	scriptPath := filepath.Join(repoRoot, "server", "scripts", "approving-mcp-spa-proxy.py")
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", scriptPath, err)
	}
	got := string(raw)
	if got != mcpSpaProxyScript {
		t.Fatalf("server/scripts/approving-mcp-spa-proxy.py diverged from embedded seedhelpers copy\n"+
			"script len=%d embed len=%d — copy seedhelpers/approving-mcp-spa-proxy.py → server/scripts/",
			len(got), len(mcpSpaProxyScript))
	}
}
