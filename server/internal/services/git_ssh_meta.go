package services

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	EnvGitSSHPrivateKey = "GIT_SSH_PRIVATE_KEY"
	EnvGitSSHKnownHosts = "GIT_SSH_KNOWN_HOSTS"
)

// wholeVarsRef matches a value that is entirely a ${vars.name} reference.
// Meta SSH fields must store literals; such references are rejected on save.
var wholeVarsRef = regexp.MustCompile(`^\$\{vars\.[A-Za-z_][A-Za-z0-9_.-]*\}$`)

// ErrSSHMetaVarsRef is returned when a meta SSH field is a whole-string vars ref.
var ErrSSHMetaVarsRef = fmt.Errorf("元信息 SSH 字段不支持 ${vars.*} 引用，请填写原文")

// ValidateSSHMetaLiteral rejects whole-string ${vars.*} references.
// Empty / whitespace-only values are allowed (means "unset").
func ValidateSSHMetaLiteral(value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	if wholeVarsRef.MatchString(v) {
		return ErrSSHMetaVarsRef
	}
	return nil
}

// ValidateAgentSSHMeta validates both SSH meta fields on an Agent-shaped value.
func ValidateAgentSSHMeta(knownHosts, privateKey string) error {
	if err := ValidateSSHMetaLiteral(knownHosts); err != nil {
		return err
	}
	return ValidateSSHMetaLiteral(privateKey)
}

// ResolveSSHField picks one SSH credential field with priority:
// Agent meta → Shared meta → Agent env → Shared env.
// fromMeta is true when the chosen source is a meta literal (must not run vars expand).
func ResolveSSHField(agentMeta, sharedMeta, agentEnv, sharedEnv string) (value string, fromMeta bool) {
	if v := strings.TrimSpace(agentMeta); v != "" {
		return agentMeta, true
	}
	if v := strings.TrimSpace(sharedMeta); v != "" {
		return sharedMeta, true
	}
	if v := strings.TrimSpace(agentEnv); v != "" {
		return agentEnv, false
	}
	if v := strings.TrimSpace(sharedEnv); v != "" {
		return sharedEnv, false
	}
	return "", false
}

// StripSSHEnvKeys removes GIT_SSH_* from an env map so multiline SSH material
// does not ride ordinary Create env / docker -e.
func StripSSHEnvKeys(env map[string]string) {
	if env == nil {
		return
	}
	delete(env, EnvGitSSHPrivateKey)
	delete(env, EnvGitSSHKnownHosts)
}

// EnvSSHValue returns a trimmed env value for the given key (nil-safe).
func EnvSSHValue(env map[string]string, key string) string {
	if env == nil {
		return ""
	}
	return env[key]
}
