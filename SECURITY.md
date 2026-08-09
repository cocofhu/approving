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

## Temporary human_gate approval links

Inbox operators can mint a one-shot external approval URL for a single pending
`human_gate` instance (`/public/gate-approvals#t=…`).

- Token entropy is ≥256-bit CSPRNG. The server stores **only** SHA-256 hex;
  verification uses constant-time compare. Plaintext tokens are never written
  to disk, logs, or audit payloads.
- The share URL places the token **only in the fragment**. Public preview/decide
  APIs accept the token via `X-Gate-Share-Token` / JSON body — never path or
  query (so it is not copied into Referer or common access logs).
- Scope is one Run + one `human_gate` node + the current pending iteration.
  Holders cannot open projects, other runs/nodes, or authenticated `/api/*`.
- Default TTL is 24h (1h / 8h / 24h / 72h / 7d). One active link per instance.
  Regenerating immediately revokes the previous URL and reuses the same TTL
  tier from the new mint time. Revoke, expiry, successful decide, login-side
  resume, run cancel/complete, or a new gate iteration all invalidate unused
  links immediately.
- Public responses set `Cache-Control: no-store`, `Referrer-Policy: no-referrer`,
  and a strict CSP. The `/public/gate-approvals` prefix does **not** emit
  `Access-Control-Allow-Origin`. POST decide requires Origin/Referer + custom
  header + one-time nonce, and is per-IP rate limited.
- Audit records create / regen / revoke / use with `callerKind=external` on
  use, optional self-reported name, masked IP (last octet/group), and
  browser/OS UA only — still without the plaintext token.

## Safe harbor

We will not pursue legal action against researchers who:

- make a good-faith effort to avoid privacy violations, service disruption, and
  data destruction;
- do not access or modify data that is not their own beyond what is needed to
  demonstrate the issue;
- report findings promptly through the private channel above and give us a
  reasonable chance to remediate before public disclosure.
