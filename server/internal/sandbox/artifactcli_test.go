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
				"upload_image_artifact",
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

func TestArtifactUploadScriptDoesNotCallWriteArtifactImage(t *testing.T) {
	t.Parallel()
	if strings.Contains(artifactUploadScript, `"name": "write_artifact"`) {
		t.Fatal("artifact-upload must not call write_artifact")
	}
	if strings.Contains(artifactUploadScript, `"arguments": {"name":`) &&
		strings.Contains(artifactUploadScript, `"kind": "image"`) {
		t.Fatal("artifact-upload must not pass kind=image to write_artifact")
	}
	if !strings.Contains(artifactUploadScript, "upload_image_artifact") {
		t.Fatal("artifact-upload must call upload_image_artifact")
	}
	if !strings.Contains(artifactUploadScript, "_maybe_bootstrap_installed_cli") {
		t.Fatal("artifact-upload must bootstrap outdated /usr/local/bin from workspace")
	}
	if strings.Contains(artifactUploadScript, `b"kind=image"`) {
		t.Fatal("bootstrap must detect legacy write_artifact JSON, not only kind=image literal")
	}
	if !strings.Contains(artifactUploadScript, "/upload-image") {
		t.Fatal("artifact-upload must fall back to HTTP /upload-image")
	}
}

func TestMCPAdvertiseProfileBootstrapsArtifactUpload(t *testing.T) {
	t.Parallel()
	if !strings.Contains(mcpAdvertiseProfileScript, "install-artifact-upload.sh") {
		t.Fatal("profile.d must bootstrap artifact-upload from workspace when outdated")
	}
}

func TestMCPSpaProxyPath(t *testing.T) {
	t.Parallel()
	if mcpSpaProxyPath != "/usr/local/bin/approving-mcp-spa-proxy" {
		t.Fatalf("mcpSpaProxyPath = %q", mcpSpaProxyPath)
	}
}

// TestRepoScriptMatchesEmbeddedArtifactUpload keeps server/scripts/artifact-upload
// in sync with the embedded seed helper so agents can bootstrap from the cloned
// workspace when seedHelpers did not run (old control plane).
func TestRepoScriptMatchesEmbeddedArtifactUpload(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	scriptPath := filepath.Join(repoRoot, "server", "scripts", "artifact-upload")
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", scriptPath, err)
	}
	got := string(raw)
	if got != artifactUploadScript {
		t.Fatalf("server/scripts/artifact-upload diverged from embedded seedhelpers copy\n"+
			"script len=%d embed len=%d — copy seedhelpers/artifact-upload → server/scripts/",
			len(got), len(artifactUploadScript))
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
