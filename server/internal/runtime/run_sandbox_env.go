package runtime

import (
	"strings"

	"github.com/cocofhu/grasp/internal/envauth"
)

// IsDeniedRunSandboxEnvKey reports keys that must not appear in a StartRun
// run-scoped sandbox env snapshot. Aligns with platform auth keys, GRASP_*
// auth aliases, ApplyPasswords, mcpVars reserved keys, and manager injects
// (AGENT_PROVIDER / CONFIG_ROOT / SSH_KEY / GIT_REPOS). Callers reject the whole
// start when any such key is present (no silent drop).
func IsDeniedRunSandboxEnvKey(k string) bool {
	k = strings.TrimSpace(k)
	if k == "" {
		return false
	}
	if envauth.IsPlatformAuthEnvKey(k) {
		return true
	}
	switch k {
	case // ApplyPasswords
		"PASSWORD", "ROOT_PASSWORD", "ACP_BRIDGE_PASSWORD", "CURSOR_ACP_PASSWORD",
		// mcpVars reserved (exact)
		"GRASP_ARTIFACT_URL", "GRASP_ARTIFACT_TOKEN",
		"GRASP_RUN_ID", "GRASP_NODE_ID",
		// platform write-backs / manager injects
		"AGENT_PROVIDER", "CONFIG_ROOT", "SSH_KEY", "GIT_REPOS",
		// GRASP_* auth aliases (all backends)
		"GRASP_CURSOR_API_KEY",
		"GRASP_CLAUDE_API_KEY",
		"GRASP_CODEBUDDY_API_KEY",
		"GRASP_TRAE_API_KEY",
		"GRASP_OPENCODE_API_KEY":
		return true
	}
	if strings.HasPrefix(k, "GRASP_ARTIFACT_") {
		return true
	}
	return false
}
