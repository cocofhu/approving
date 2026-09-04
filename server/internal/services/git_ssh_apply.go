package services

import (
	"strings"

	"github.com/cocofhu/approving/internal/sandbox"
)

// ApplyAgentSSHToSpec resolves SSH known_hosts / private key from Agent meta
// (preferred) with Spec.Env fallback (already vars-expanded by caller), attaches
// them for file inject, and strips GIT_SSH_* from Spec.Env.
func ApplyAgentSSHToSpec(spec *sandbox.Spec, agent Agent) {
	if spec == nil {
		return
	}
	envKey := ""
	envHosts := ""
	if spec.Env != nil {
		envKey = spec.Env[EnvGitSSHPrivateKey]
		envHosts = spec.Env[EnvGitSSHKnownHosts]
	}
	key, _ := ResolveSSHField(agent.GitSshPrivateKey, "", envKey, "")
	hosts, _ := ResolveSSHField(agent.GitSshKnownHosts, "", envHosts, "")
	// Preserve original (possibly multi-line) meta bodies; ResolveSSHField may
	// return env values which are already expanded on Spec.Env.
	if strings.TrimSpace(agent.GitSshPrivateKey) != "" {
		key = agent.GitSshPrivateKey
	}
	if strings.TrimSpace(agent.GitSshKnownHosts) != "" {
		hosts = agent.GitSshKnownHosts
	}
	sandbox.ApplySSHCredentials(spec, key, hosts)
}
