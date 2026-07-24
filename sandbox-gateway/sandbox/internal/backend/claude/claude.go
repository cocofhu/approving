// Package claude implements the Claude Code ACP backend
// (@zed-industries/claude-code-acp).
package claude

import "backend/internal/backend/common"

const (
	envClaudeAPIKey = "ANTHROPIC_API_KEY"
	envAcpClaudeKey = "ACP_CLAUDE_API_KEY"
)

// Backend spawns `npx @zed-industries/claude-code-acp`.
type Backend struct{ common.Base }

// New returns the claude-code backend.
func New() common.Backend { return Backend{} }

func (Backend) Name() common.Name { return common.ClaudeCode }

func (Backend) Runtime() string { return "claude-code-acp" }

func (Backend) DefaultConfigRoot() string { return "/root/.claude" }

func (Backend) Argv(model string) []string {
	if model == "" {
		model = "auto"
	}
	return []string{"npx", "--yes", "@zed-industries/claude-code-acp", "--model", model}
}

func (Backend) AuthEnv(env []string) []string {
	return common.SetIfEmpty(env, envClaudeAPIKey, common.FirstNonEmptyEnv(envAcpClaudeKey, envClaudeAPIKey))
}
