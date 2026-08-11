package config

import (
	"fmt"
	"strings"
)

// knownSandboxBackends are acpBackend values that have dedicated images.
var knownSandboxBackends = []string{"cursor", "claude_code", "codebuddy", "trae"}

// DefaultSandboxImage returns the local image tag for an acpBackend when no
// config override is set. Unknown/empty backends fall back to cursor.
// Tags match images built from sandbox-gateway/sandbox (see ./start.sh sandbox).
func DefaultSandboxImage(backend string) string {
	b := strings.TrimSpace(backend)
	switch b {
	case "cursor", "claude_code", "codebuddy", "trae":
		// ok
	default:
		b = "cursor"
	}
	return fmt.Sprintf("universal-sandbox-%s:local", b)
}

// ResolveSandboxImage picks the sandbox image for an acpBackend:
//  1. sandbox.image / APPROVING_SANDBOX_IMAGE when non-empty (global force)
//  2. sandbox.images[backend] / APPROVING_SANDBOX_IMAGE_<BACKEND>
//  3. DefaultSandboxImage(backend)
func (c *Config) ResolveSandboxImage(backend string) string {
	if c != nil {
		if img := strings.TrimSpace(c.Sandbox.Image); img != "" {
			return img
		}
		b := strings.TrimSpace(backend)
		if b == "" {
			b = "cursor"
		}
		if c.Sandbox.Images != nil {
			if img := strings.TrimSpace(c.Sandbox.Images[b]); img != "" {
				return img
			}
		}
		return DefaultSandboxImage(b)
	}
	return DefaultSandboxImage(backend)
}

// KnownSandboxBackends returns the acpBackend keys with dedicated images.
func knownSandboxBackendKeys() []string {
	out := make([]string, len(knownSandboxBackends))
	copy(out, knownSandboxBackends)
	return out
}
