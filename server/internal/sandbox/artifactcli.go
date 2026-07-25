package sandbox

import (
	"context"
	_ "embed"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// artifactUploadPath is where the helper CLI is seeded inside each sandbox.
const artifactUploadPath = "/usr/local/bin/artifact-upload"

// mcpSpaProxyPath is an optional local reverse-proxy helper retained for
// deployments that map a front-door host to the API ingress. The public map is
// empty (no-op).
const mcpSpaProxyPath = "/usr/local/bin/approving-mcp-spa-proxy"

//go:embed seedhelpers/artifact-upload
var artifactUploadScript string

//go:embed seedhelpers/approving-mcp-advertise.sh
var mcpAdvertiseProfileScript string

//go:embed seedhelpers/approving-mcp-spa-proxy.py
var mcpSpaProxyScript string

// EnsureHelpers re-seeds sandbox helper CLIs (artifact-upload, mcp advertise
// profile.d, optional mcp host proxy). Safe to call on Attach/reconnect so
// sandboxes created by an older control plane (or whose seed failed) heal after
// upgrade without requiring container recreate. No-op when InstallHelpers is off
// (unit tests). Best-effort; never fatal.
func (m *Manager) EnsureHelpers(ctx context.Context, sb *Sandbox) {
	if m == nil || !m.installHelpers {
		return
	}
	m.seedHelpers(ctx, sb)
}

// seedHelpers installs approving's sandbox-side helper CLIs over SSH. The
// universal-sandbox image ships no artifact-upload command, so approving writes
// it in itself (rather than extending the image): test/UI nodes tell the agent
// to run `artifact-upload <file>` to push screenshots into the run's artifact
// store. Best-effort — a failure only means screenshot upload is unavailable,
// it never blocks sandbox creation.
func (m *Manager) seedHelpers(ctx context.Context, sb *Sandbox) {
	if sb == nil {
		return
	}
	creds := sb.creds()
	// Bound the wait: sshd usually accepts shortly after the gateway reports
	// running. If it is not ready in time we skip (degraded, not fatal).
	rctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if err := creds.waitReady(rctx, 25*time.Second); err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Msg("seed helpers: ssh not ready; artifact-upload unavailable")
		return
	}
	qUpload, err := quoteShellPath(artifactUploadPath)
	if err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Msg("seed helpers: invalid artifact-upload path")
		return
	}
	cmd := "cat > " + qUpload + " && chmod +x " + qUpload
	if out, err := creds.runInput(ctx, 20*time.Second, cmd, strings.NewReader(artifactUploadScript)); err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Str("out", strings.TrimSpace(string(out))).
			Msg("seed helpers: install artifact-upload failed")
		return
	}
	// Best-effort: profile.d hook for interactive shells (login/profile).
	// Container create env is authoritative for ACP; this only helps SSH/login
	// and any tool that sources profile.d after seed. Public script is a no-op.
	profileCmd := "cat > /etc/profile.d/approving-mcp-advertise.sh && chmod 644 /etc/profile.d/approving-mcp-advertise.sh"
	if out, err := creds.runInput(ctx, 10*time.Second, profileCmd, strings.NewReader(mcpAdvertiseProfileScript)); err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Str("out", strings.TrimSpace(string(out))).
			Msg("seed helpers: install mcp_advertise profile.d failed")
	}
	// Best-effort: optional Host proxy (map empty in public tree → no-op).
	qProxy, err := quoteShellPath(mcpSpaProxyPath)
	if err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Msg("seed helpers: invalid mcp host proxy path")
		return
	}
	proxyCmd := "cat > " + qProxy + " && chmod +x " + qProxy
	if out, err := creds.runInput(ctx, 20*time.Second, proxyCmd, strings.NewReader(mcpSpaProxyScript)); err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Str("out", strings.TrimSpace(string(out))).
			Msg("seed helpers: install mcp host proxy failed")
	} else if out, err := creds.run(ctx, 15*time.Second, qProxy+" --ensure"); err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Str("out", strings.TrimSpace(string(out))).
			Msg("seed helpers: start mcp host proxy failed")
	}
	log.Debug().Str("id", sb.ID).Msg("seeded artifact-upload CLI")
}
