package config

import "testing"

func TestDefaultSandboxImage(t *testing.T) {
	cases := []struct {
		backend, want string
	}{
		{"cursor", "universal-sandbox-cursor:local"},
		{"claude_code", "universal-sandbox-claude_code:local"},
		{"codebuddy", "universal-sandbox-codebuddy:local"},
		{"trae", "universal-sandbox-trae:local"},
		{"", "universal-sandbox-cursor:local"},
		{"nope", "universal-sandbox-cursor:local"},
	}
	for _, tc := range cases {
		if got := DefaultSandboxImage(tc.backend); got != tc.want {
			t.Errorf("DefaultSandboxImage(%q) = %q, want %q", tc.backend, got, tc.want)
		}
	}
}

func TestResolveSandboxImage(t *testing.T) {
	if got := (*Config)(nil).ResolveSandboxImage("trae"); got != DefaultSandboxImage("trae") {
		t.Fatalf("nil config: %q", got)
	}
	c := &Config{Sandbox: SandboxConfig{
		Images: map[string]string{"cursor": "reg/cursor:pin"},
	}}
	if got := c.ResolveSandboxImage("cursor"); got != "reg/cursor:pin" {
		t.Fatalf("per-backend: %q", got)
	}
	if got := c.ResolveSandboxImage("claude_code"); got != DefaultSandboxImage("claude_code") {
		t.Fatalf("fallback default: %q", got)
	}
	c.Sandbox.Image = "reg/force:1"
	if got := c.ResolveSandboxImage("claude_code"); got != "reg/force:1" {
		t.Fatalf("global force: %q", got)
	}
}

func TestApplySandboxImageEnv(t *testing.T) {
	t.Setenv("APPROVING_SANDBOX_IMAGE_CURSOR", "env/cursor:1")
	t.Setenv("APPROVING_SANDBOX_IMAGE_CLAUDE_CODE", "env/claude:1")
	c := &Config{}
	applySandboxImageEnv(c)
	if c.Sandbox.Images["cursor"] != "env/cursor:1" {
		t.Fatalf("cursor env: %v", c.Sandbox.Images)
	}
	if c.Sandbox.Images["claude_code"] != "env/claude:1" {
		t.Fatalf("claude_code env: %v", c.Sandbox.Images)
	}
}
