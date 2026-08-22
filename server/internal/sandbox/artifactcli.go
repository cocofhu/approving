package sandbox

import (
	"context"
	_ "embed"
	"fmt"
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

const mcpAdvertiseProfilePath = "/etc/profile.d/approving-mcp-advertise.sh"

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
	if err := seedExecutable(ctx, creds, artifactUploadPath, artifactUploadScript); err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Msg("seed helpers: install artifact-upload failed")
		return
	}
	// When the control plane still embeds a pre-#375 script, upgrade from the
	// cloned workspace checkout (available after startup.sh finishes git clone).
	healArtifactUploadFromWorkspace(ctx, creds, sb.ID)
	// Best-effort: profile.d hook for interactive shells (login/profile).
	// Container create env is authoritative for ACP; this only helps SSH/login
	// and any tool that sources profile.d after seed. Public script is a no-op.
	if err := seedFileMode(ctx, creds, mcpAdvertiseProfilePath, mcpAdvertiseProfileScript, "644"); err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Msg("seed helpers: install mcp_advertise profile.d failed")
	}
	// Best-effort: optional Host proxy (map empty in public tree → no-op).
	if err := seedExecutable(ctx, creds, mcpSpaProxyPath, mcpSpaProxyScript); err != nil {
		log.Warn().Str("id", sb.ID).Err(err).Msg("seed helpers: install mcp host proxy failed")
	} else {
		ensureCmd, err := newSafeCmd(mcpSpaProxyPath, "--ensure")
		if err != nil {
			log.Warn().Str("id", sb.ID).Err(err).Msg("seed helpers: invalid mcp host proxy path")
			return
		}
		if out, err := creds.run(ctx, 15*time.Second, ensureCmd); err != nil {
			log.Warn().Str("id", sb.ID).Err(err).Str("out", strings.TrimSpace(string(out))).
				Msg("seed helpers: start mcp host proxy failed")
		}
	}
	log.Debug().Str("id", sb.ID).Msg("seeded artifact-upload CLI")
}

func seedExecutable(ctx context.Context, creds sshCreds, path, content string) error {
	return seedFileMode(ctx, creds, path, content, "+x")
}

// healArtifactUploadFromWorkspace runs install-artifact-upload.sh from a cloned
// repo when present. Best-effort; never fatal. Idempotent when CLI is current.
func healArtifactUploadFromWorkspace(ctx context.Context, creds sshCreds, sandboxID string) {
	script := `for inst in /root/workspace/*/server/scripts/install-artifact-upload.sh; do
  [ -x "$inst" ] && "$inst" && exit 0
done
for src in /root/workspace/*/server/scripts/artifact-upload; do
  if [ -f "$src" ] && grep -q upload_image_artifact "$src" 2>/dev/null; then
    install -m 755 "$src" /usr/local/bin/artifact-upload && exit 0
  fi
done
exit 0`
	cmd, err := newSafeCmd("sh", "-c", script)
	if err != nil {
		return
	}
	if out, err := creds.run(ctx, 20*time.Second, cmd); err != nil {
		log.Debug().Str("id", sandboxID).Err(err).Str("out", strings.TrimSpace(string(out))).
			Msg("seed helpers: workspace artifact-upload heal skipped")
	}
}

func seedFileMode(ctx context.Context, creds sshCreds, path, content, mode string) error {
	teeCmd, err := newSafeCmd("tee", path)
	if err != nil {
		return err
	}
	if out, err := creds.runInput(ctx, 20*time.Second, teeCmd, strings.NewReader(content)); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	chmodCmd, err := newSafeCmd("chmod", mode, path)
	if err != nil {
		return err
	}
	if out, err := creds.run(ctx, 10*time.Second, chmodCmd); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
