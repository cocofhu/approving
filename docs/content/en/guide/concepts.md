---
title: Core concepts
description: FSM orchestration, human gates, sandboxed execution, and artifact contracts.
---

## FSM orchestration

Approving turns coding agents into steps in a workflow. You orchestrate on a finite state machine:

- **Nodes** are states (agent / react / gate / …)
- **Edges** are transitions, with configurable success, failure, and rollback paths
- Use `when` guards and checkpoints to make risky steps explicit

This is not a one-shot, irreversible agent run: design the path first, then gate the critical steps.

## Human gates

When a step needs a human decision, the run stops at a **gate** until someone approves or rejects.

Approval moments are first-class — not an afterthought. Approving bets that agents can be fast while people still own the critical decisions.

### Temporary approval links (human_gate)

In the pending-gates inbox, only **human_gate** cards (and the visual preview toolbar when a page artifact exists) offer **Copy temp link**. A signed-in operator can mint a one-shot URL so an unauthenticated person can approve or reject once.

- Default TTL is 24 hours (1h / 8h / 24h / 72h / 7d). At most one active link per instance.
- The management panel masks the URL by default; Copy writes the full fragment URL. Refreshing the same browser tab still lets you copy the same active URL. Regenerating immediately revokes the old link and reuses the same TTL tier; Revoke now disables the link. While the gate is still pending, revoked/expired links can be replaced; after the gate is decided the entry is read-only. `proposal_select` and pending clarify have no share entry (app preview uses the review-share path below).
- The external page needs no login. It shows only the title, description, redacted visual/structured artifacts, approve/reject, comment, and an optional name. It does not expose project, run, members, or internal URLs.
- The token is bound to that one approval. Expiry, revoke, a successful submit, a login-side decision, or run completion invalidate unused links immediately.

### Temporary review links (Inbox kind=review / app_preview)

Inbox **pending review** and **app preview** cards reuse the same management panel and token rules (`ShareLinkKindReview`), but authenticated APIs live under `/api/runs/:id/reviews/:nodeId/share-link*` — not `/gates/...`, and no fake Gate row is created. In-product entries: card **Copy temp link**, app-preview workbench toolbar **Share approval**, and the Agent sidebar **Open share panel**. The public page is labeled **External review**; hot sessions support multi-turn ReAct. For `productKind=app_preview` the public page defaults to remote desktop and picking via a short-lived ticket channel (desensitized ports; API ports use a same-origin iframe); mobile shows a degrade hint only. The only footer action is **Confirm and advance**. Run-detail review tabs and the logged-in review composer do not add a temp-link entry; `proposal_select` and pending clarify stay out of scope.

## Real Docker sandboxes

Agents are not black-box prompts on a laptop. They execute in Docker containers through the in-repo [sandbox-gateway](https://github.com/cocofhu/approving/tree/main/sandbox-gateway), talking over ACP.

Supported backends: **Cursor**, **Claude Code**, **CodeBuddy**, and **Trae**. Configure `acpBackend` per agent; keep secrets in agent meta env.

## Artifact contract and MCP

Each run has an isolated artifact MCP. Agents call tools such as:

- `write_artifact`
- `set_*`
- `node_complete`

Isolation is by run token, leaving an inspectable paper trail.

## PM: `pm-agent-fs` (org + Agent workspace)

A project-bound PM Leader can enable the dedicated MCP `pm-agent-fs` (on by default for new projects; older projects with an explicit EnabledMcps list must opt in under PM settings):

- `pm_get_org`: read the org and label self / direct / indirect / other relative to the Leader
- `pm_fs_*`: list/read/write/delete/mkdir/rename the **host-side** `workspace/` of self and reporting-closure reports (**not** Run sandbox FS)

Writes land on the same disk tree as Agent Studio「Agent workspace」and are visible after **refresh or reopen** (no live hot-reload). If Studio still has an unsaved dirty draft for the same Agent, a later Save may overwrite MCP writes — refresh and avoid concurrent dirty edits during demos.

## Single-repo self-hosting

`sandbox-gateway` and the generic sandbox image sources live in this repository. One clone is enough to self-host.
