package services

import "testing"

func TestValidateSSHMetaLiteral(t *testing.T) {
	if err := ValidateSSHMetaLiteral(""); err != nil {
		t.Fatalf("empty: %v", err)
	}
	if err := ValidateSSHMetaLiteral("  "); err != nil {
		t.Fatalf("blank: %v", err)
	}
	if err := ValidateSSHMetaLiteral("host ssh-ed25519 AAAA\nhost2 ssh-rsa BBBB"); err != nil {
		t.Fatalf("literal multiline: %v", err)
	}
	if err := ValidateSSHMetaLiteral("${vars.git_ssh_known_hosts}"); err == nil {
		t.Fatal("expected reject whole vars ref")
	}
	if err := ValidateSSHMetaLiteral("prefix ${vars.x}"); err != nil {
		t.Fatalf("partial vars text should be allowed as literal: %v", err)
	}
}

func TestResolveSSHFieldPriority(t *testing.T) {
	v, meta := ResolveSSHField("agent-meta", "shared-meta", "agent-env", "shared-env")
	if v != "agent-meta" || !meta {
		t.Fatalf("got %q meta=%v", v, meta)
	}
	v, meta = ResolveSSHField("", "shared-meta", "agent-env", "shared-env")
	if v != "shared-meta" || !meta {
		t.Fatalf("got %q meta=%v", v, meta)
	}
	v, meta = ResolveSSHField("", "", "agent-env", "shared-env")
	if v != "agent-env" || meta {
		t.Fatalf("got %q meta=%v", v, meta)
	}
	v, meta = ResolveSSHField("", "", "", "shared-env")
	if v != "shared-env" || meta {
		t.Fatalf("got %q meta=%v", v, meta)
	}
}

func TestExtendOverlaySSHMeta(t *testing.T) {
	shared := SharedAgentConfig{
		ProjectID:        "p1",
		GitSshKnownHosts: "shared-hosts",
		GitSshPrivateKey: "shared-key",
		Env:              map[string]string{"GITHUB_TOKEN": "gh", "GITLAB_TOKEN": "gl"},
	}
	agent := Agent{
		Name:             "a",
		GitSshPrivateKey: "agent-key",
		Env:              map[string]string{"GITHUB_TOKEN": "agent-gh"},
	}
	out := ExtendOverlay(shared, agent)
	if out.GitSshPrivateKey != "agent-key" {
		t.Fatalf("key=%q", out.GitSshPrivateKey)
	}
	if out.GitSshKnownHosts != "shared-hosts" {
		t.Fatalf("hosts=%q", out.GitSshKnownHosts)
	}
	// Token-class: shared wins when present
	if out.Env["GITHUB_TOKEN"] != "gh" {
		t.Fatalf("GITHUB_TOKEN=%q", out.Env["GITHUB_TOKEN"])
	}
	if out.Env["GITLAB_TOKEN"] != "gl" {
		t.Fatalf("GITLAB_TOKEN=%q", out.Env["GITLAB_TOKEN"])
	}
}

func TestAgentSSHMetaPersistAndRejectVars(t *testing.T) {
	dir := t.TempDir()
	svc := NewAgentService(dir)
	a := Agent{
		Name:             "ssh-agent",
		GitSshKnownHosts: "example.com ssh-ed25519 AAAA",
		GitSshPrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nx\n-----END OPENSSH PRIVATE KEY-----",
		Env:              map[string]string{"GITHUB_TOKEN": "t"},
	}
	if err := svc.Save(a); err != nil {
		t.Fatal(err)
	}
	got, ok := svc.Get("ssh-agent")
	if !ok {
		t.Fatal("missing")
	}
	if got.GitSshKnownHosts != a.GitSshKnownHosts {
		t.Fatalf("hosts mismatch")
	}
	if got.GitSshPrivateKey != a.GitSshPrivateKey {
		t.Fatalf("key mismatch")
	}
	a.GitSshPrivateKey = "${vars.git_ssh_private_key}"
	if err := svc.Save(a); err == nil {
		t.Fatal("expected vars reject")
	}
}

func TestSharedAgentSSHMetaPersistAndRejectVars(t *testing.T) {
	dir := t.TempDir()
	svc := NewSharedAgentService(dir)
	cfg := SharedAgentConfig{
		ProjectID:        "proj-ssh",
		GitSshKnownHosts: "gitlab.com ssh-ed25519 AAAA\ngithub.com ssh-ed25519 BBBB",
		GitSshPrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\ny\n-----END OPENSSH PRIVATE KEY-----",
		Env:              map[string]string{"GITHUB_TOKEN": "gh", "GITLAB_TOKEN": "gl", "GIT_SSH_PRIVATE_KEY": "strip-me"},
	}
	if err := svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got := svc.Get("proj-ssh")
	if got.ProjectID == "" {
		t.Fatal("missing shared config")
	}
	if got.GitSshKnownHosts != cfg.GitSshKnownHosts {
		t.Fatalf("hosts mismatch")
	}
	if got.GitSshPrivateKey != cfg.GitSshPrivateKey {
		t.Fatalf("key mismatch")
	}
	if _, ok := got.Env["GIT_SSH_PRIVATE_KEY"]; ok {
		t.Fatal("GIT_SSH_* should be stripped from shared env on save")
	}
	if got.Env["GITHUB_TOKEN"] != "gh" || got.Env["GITLAB_TOKEN"] != "gl" {
		t.Fatalf("tokens must remain: %+v", got.Env)
	}
	cfg.GitSshKnownHosts = "${vars.git_ssh_known_hosts}"
	if err := svc.Save(cfg); err == nil {
		t.Fatal("expected vars reject on known_hosts")
	}
}
