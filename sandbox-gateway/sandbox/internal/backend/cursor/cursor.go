// Package cursor implements the cursor-agent ACP backend.
package cursor

import "backend/internal/backend/common"

const (
	envCursorAPIKey = "CURSOR_API_KEY"
	envAcpCursorKey = "ACP_CURSOR_API_KEY"
)

// Backend spawns `cursor-agent ... acp`.
type Backend struct{ common.Base }

// New returns the cursor backend.
func New() common.Backend { return Backend{} }

func (Backend) Name() common.Name { return common.Cursor }

func (Backend) Runtime() string { return "cursor-agent" }

func (Backend) DefaultConfigRoot() string { return "/root/.cursor" }

func (Backend) Argv(model string) []string {
	if model == "" {
		model = "auto"
	}
	return []string{"cursor-agent", "--model", model, "acp"}
}

func (Backend) AuthEnv(env []string) []string {
	return common.SetIfEmpty(env, envCursorAPIKey, common.FirstNonEmptyEnv(envAcpCursorKey, envCursorAPIKey))
}
