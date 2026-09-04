package sandbox

import "strings"

// ApplySSHCredentials sets Spec SSH file-inject fields and strips GIT_SSH_*
// from Env so multiline material does not ride ordinary Create env / docker -e.
// Empty (trim-space) key/hosts are left unset — do not clear a previously set
// Spec field with an empty string.
func ApplySSHCredentials(spec *Spec, privateKey, knownHosts string) {
	if spec == nil {
		return
	}
	if spec.Env == nil {
		spec.Env = map[string]string{}
	}
	if strings.TrimSpace(privateKey) != "" {
		spec.SSHPrivateKey = privateKey
	}
	if strings.TrimSpace(knownHosts) != "" {
		spec.SSHKnownHosts = knownHosts
	}
	delete(spec.Env, "GIT_SSH_PRIVATE_KEY")
	delete(spec.Env, "GIT_SSH_KNOWN_HOSTS")
}
