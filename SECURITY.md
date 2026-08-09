# Security policy

Report vulnerabilities privately through GitHub
[Private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
on [github.com/cocofhu/approving](https://github.com/cocofhu/approving).

Do not disclose suspected vulnerabilities in public issues, pull requests,
discussions, logs, or screenshots.

## Supported versions

Only the latest published release line on GitHub Releases is supported for
security fixes until a longer support matrix is published.

## Response targets

- Acknowledgement: within 3 business days
- Status updates: at least every 7 days while the report remains open
- Coordinated disclosure: agree on a public date after a fix or mitigation is
  available, or after a reasonable remediation window

## Safe harbor

We will not pursue legal action against researchers who:

- make a good-faith effort to avoid privacy violations, service disruption, and
  data destruction;
- do not access or modify data that is not their own beyond what is needed to
  demonstrate the issue;
- report findings promptly through the private channel above and give us a
  reasonable chance to remediate before public disclosure.

## Sandbox data-plane trust boundary (CDP / noVNC)

Unauthenticated Chromium CDP (`:9222`, socat) and noVNC/websockify (`:6080`,
`x11vnc -nopw`) must not be reachable from users or untrusted networks.

- **Users** reach the desktop only through Approving WebSocket proxies:
  `/sandbox-vnc/:sandboxId/ws` and `/preview-vnc/:runId/:nodeId/:port/ws`.
  When platform Auth is injected (always on outside local-demo), these require
  a valid session cookie. Auth checks **Session validity only** — it does **not**
  check sandbox or run ownership; any logged-in user who knows the URL can
  connect. The UI does not expose or copy direct `cdp` / `novnc` addresses.
- **session / ide / ssh** may still be published. They use their own passwords
  or SSH keys (`ROOT_PASSWORD` / `SSH_KEY` / IDE password). Direct CDP/noVNC
  is not a substitute.
- **Approving** (and other in-cluster / Docker-network peers) may dial CDP and
  noVNC on the container IP or ClusterIP DNS. Approving running *outside* the
  cluster or Docker network is **not** a supported topology for CDP/VNC control.
- **Residual risk**: pods on the same cluster / Docker network can still reach
  `:9222` / `:6080`. This change does not add NetworkPolicy.
- **Docker inventory**: already-running containers keep their existing `-p`
  mappings until TTL expiry or Reinstall. Old `host:9222` / `host:6080`
  bookmarks becoming unreachable is an **expected breaking change**.
- **Kubernetes inventory**: existing `*-lb` Services that still list 9222/6080
  stay exposed until gateway **startup reconcile** (`ReconcileOnStartup`),
  `Start`, or `Reinstall` updates `Spec.Ports`. Until that heal completes, old
  bookmarks and untrusted scans may still succeed. After converge, Endpoints
  report ClusterIP DNS for CDP/noVNC; LB Ingress IPs are public ports only.
- Do not rely on traditional VNC 8-character passwords; isolation is by
  publish surface + platform Session, not RFB auth.
