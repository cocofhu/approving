// Package backend exposes concrete ACP CLI backends (argv / AuthEnv / config root).
//
// Selection of the active agent lives in internal/agents (AGENT_PROVIDER /
// ACP_BACKEND). This package only supplies the long-lived ACP implementations
// that acpx.FromBackend wraps — it is not a second provider registry.
package backend

import (
	"backend/internal/backend/claude"
	"backend/internal/backend/codebuddy"
	"backend/internal/backend/common"
	"backend/internal/backend/cursor"
	"backend/internal/backend/trae"
)

// Name identifies which ACP CLI backend to spawn.
type Name = common.Name

const (
	Cursor     = common.Cursor
	ClaudeCode = common.ClaudeCode
	CodeBuddy  = common.CodeBuddy
	Trae       = common.Trae
)

// Backend is the ACP backend abstraction; re-exported for callers that need the
// per-backend hooks (e.g. OnEvent) via Get().
type Backend = common.Backend

// registry maps each backend name to its singleton implementation.
var registry = map[Name]Backend{
	Cursor:     cursor.New(),
	ClaudeCode: claude.New(),
	CodeBuddy:  codebuddy.New(),
	Trae:       trae.New(),
}

// Get returns the backend for the given name (falls back to cursor).
func Get(n Name) Backend {
	if be, ok := registry[n]; ok {
		return be
	}
	return registry[Cursor]
}
