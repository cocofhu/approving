package runtime

import "strings"

// IsDeniedRunSandboxEnvKey reports keys that must not appear in a StartRun
// run-scoped sandbox env snapshot. Aligns with platform auth keys, APPROVING_*
// auth aliases, ApplyPasswords, mcpVars reserved keys, and manager injects
// (ACP_BACKEND / CONFIG_ROOT / SSH_KEY / GIT_REPOS). Callers reject the whole
// start when any such key is present (no silent drop).
func IsDeniedRunSandboxEnvKey(k string) bool {
	k = strings.TrimSpace(k)
	if k == "" {
		return false
	}
	if IsPlatformAuthEnvKey(k) {
		return true
	}
	switch k {
	case // ApplyPasswords
		"PASSWORD", "ROOT_PASSWORD", "ACP_BRIDGE_PASSWORD", "CURSOR_ACP_PASSWORD",
		// mcpVars reserved (exact)
		"APPROVING_ARTIFACT_URL", "APPROVING_ARTIFACT_TOKEN",
		"APPROVING_RUN_ID", "APPROVING_NODE_ID",
		// platform write-backs / manager injects
		"ACP_BACKEND", "CONFIG_ROOT", "SSH_KEY", "GIT_REPOS",
		// APPROVING_* auth aliases (all backends)
		"APPROVING_CURSOR_API_KEY", "APPROVING_CLAUDE_API_KEY",
		"APPROVING_CODEBUDDY_API_KEY", "APPROVING_TRAE_API_KEY":
		return true
	}
	// Future APPROVING_ARTIFACT_* reserved names
	if strings.HasPrefix(k, "APPROVING_ARTIFACT_") {
		return true
	}
	return false
}
