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
