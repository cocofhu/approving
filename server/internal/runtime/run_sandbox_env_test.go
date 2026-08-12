package runtime

import "testing"

func TestIsDeniedRunSandboxEnvKey(t *testing.T) {
	denied := []string{
		"CURSOR_API_KEY", "ANTHROPIC_API_KEY", "CODEBUDDY_API_KEY", "TRAE_API_KEY",
		EnvTraeCLIToken,
		"APPROVING_CURSOR_API_KEY", "APPROVING_CLAUDE_API_KEY",
		"APPROVING_CODEBUDDY_API_KEY", "APPROVING_TRAE_API_KEY",
		"PASSWORD", "ROOT_PASSWORD", "ACP_BRIDGE_PASSWORD", "CURSOR_ACP_PASSWORD",
		"APPROVING_ARTIFACT_URL", "APPROVING_ARTIFACT_TOKEN", "APPROVING_ARTIFACT_FOO",
		"APPROVING_RUN_ID", "APPROVING_NODE_ID",
		"ACP_BACKEND", "CONFIG_ROOT", "SSH_KEY", "GIT_REPOS",
	}
	for _, k := range denied {
		if !IsDeniedRunSandboxEnvKey(k) {
			t.Fatalf("expected denied: %s", k)
		}
	}
	allowed := []string{"LOG_LEVEL", "FEATURE_FLAG", "DB_PASSWORD", "MY_APPROVING_CUSTOM", ""}
	for _, k := range allowed {
		if IsDeniedRunSandboxEnvKey(k) {
			t.Fatalf("expected allowed: %q", k)
		}
	}
}
