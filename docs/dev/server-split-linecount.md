# Same-package split line counts (PR3)

Measured after mechanical same-package file moves (no behavior changes). Soft target ≈800 lines for non-generated Go sources.

| File | Lines | Notes |
|------|------:|-------|
| `internal/handlers/handlers.go` | ~72 | Handlers struct + wiring only |
| `internal/handlers/agent.go` | ~433 | Agent CRUD/org/import |
| `internal/handlers/workflow.go` | ~300 | |
| `internal/handlers/run.go` | ~335 | |
| `internal/handlers/gate.go` | ~210 | |
| `internal/engine/engine.go` | ~208 | Engine struct + constructors |
| `internal/engine/execute.go` | ~397 | |
| `internal/engine/start_run.go` | ~328 | |
| `internal/engine/nodes.go` | ~192 | Dispatch + small nodes |
| `internal/engine/node_*.go` | ≤286 | Per-domain node exec |
| `internal/runtime/acp_provider.go` | ~246 | Types + constructors + chat helpers |
| `internal/runtime/acp_run_agent.go` | ~579 | RunAgent + harvest/MR (kept cohesive) |
| `internal/runtime/acp_react.go` | ~408 | |
| `internal/runtime/acp_sandbox.go` | ~320 | |
| `internal/runtime/acp_config.go` | ~329 | Includes 0.2.0 legacy workdir const (not deleted) |
| `internal/runtime/acp_repos.go` | ~333 | |
| `internal/runtime/acp_ensure.go` | ~199 | |
| `internal/runtime/acp_prompt.go` | ~173 | |

## Soft-cap exceptions

None required: all split targets are under ≈800 lines. `acp_run_agent.go` (~579) remains the largest cohesive ACP execution file; further splitting `runAgentOnce` would harm readability without package-boundary benefit.
