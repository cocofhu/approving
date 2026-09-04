package sandbox

// ApplySSHCredentials sets Spec SSH file-inject fields and strips GIT_SSH_*
// from Env so multiline material does not ride ordinary Create env / docker -e.
// Empty key/hosts are left unset (do not create/clear the corresponding file).
func ApplySSHCredentials(spec *Spec, privateKey, knownHosts string) {
	if spec == nil {
		return
	}
	if spec.Env == nil {
		spec.Env = map[string]string{}
	}
	spec.SSHPrivateKey = privateKey
	spec.SSHKnownHosts = knownHosts
	delete(spec.Env, "GIT_SSH_PRIVATE_KEY")
	delete(spec.Env, "GIT_SSH_KNOWN_HOSTS")
}
