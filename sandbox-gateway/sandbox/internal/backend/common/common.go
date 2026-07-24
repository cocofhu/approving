// Package common defines the ACP backend abstraction shared by every concrete
// backend (cursor / claude / codebuddy / trae).
//
// Each backend lives in its own package under internal/backend/<name> and
// implements Backend. This keeps per-backend specifics — how to spawn the CLI,
// where its config lives, how to normalize auth env, and how to handle any
// backend-specific ACP extension events — isolated and independently evolvable.
package common

import (
	"encoding/json"
	"os"
	"strings"
)

// Name identifies which ACP CLI backend to spawn.
type Name string

const (
	Cursor     Name = "cursor"
	ClaudeCode Name = "claude_code"
	CodeBuddy  Name = "codebuddy"
	Trae       Name = "trae"
)

// Backend describes one ACP CLI backend.
type Backend interface {
	// Name is the backend identifier (matches ACP_BACKEND).
	Name() Name
	// Runtime is the capabilities.agent.runtime label.
	Runtime() string
	// DefaultConfigRoot is the config tree root when CONFIG_ROOT is unset.
	DefaultConfigRoot() string
	// Argv is the subprocess argv to launch the backend for the given model.
	Argv(model string) []string
	// AuthEnv returns env (a copy/superset of the passed environ) with this
	// backend's ACP_* aliases normalized into the native CLI variables it reads.
	AuthEnv(env []string) []string
	// OnEvent lets a backend inspect / rewrite / drop a session event frame
	// before it is broadcast to clients. Backends with no extension events
	// return (ev, true) unchanged — see Base.OnEvent.
	OnEvent(ev json.RawMessage) (out json.RawMessage, keep bool)
}

// Base provides the default, backend-agnostic behavior. Concrete backends embed
// it and override only what they need (typically Argv/AuthEnv, and OnEvent when
// the backend emits extension events that need targeted handling).
type Base struct{}

// OnEvent passes every event through unchanged. Override to handle a backend's
// extension events (e.g. rewrite a vendor-specific update into a standard one,
// or drop noise before broadcast).
func (Base) OnEvent(ev json.RawMessage) (json.RawMessage, bool) { return ev, true }

// --- shared env helpers (used by concrete backends' AuthEnv) ----------------

// FirstNonEmptyEnv returns the first non-empty value among the given env keys.
func FirstNonEmptyEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func envIndex(env []string, key string) int {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			return i
		}
	}
	return -1
}

// GetEnvValue reads key's value from an env slice ("" if absent).
func GetEnvValue(env []string, key string) string {
	idx := envIndex(env, key)
	if idx < 0 {
		return ""
	}
	return strings.TrimPrefix(env[idx], key+"=")
}

// SetIfEmpty sets key=val only if val is non-empty and key is not already set.
func SetIfEmpty(env []string, key, val string) []string {
	if val == "" || GetEnvValue(env, key) != "" {
		return env
	}
	return UpsertEnv(env, key, val)
}

// UpsertEnv inserts or replaces key=val in the env slice (returns a new slice
// when replacing, to avoid mutating the caller's backing array).
func UpsertEnv(env []string, key, val string) []string {
	if idx := envIndex(env, key); idx >= 0 {
		out := append([]string(nil), env...)
		out[idx] = key + "=" + val
		return out
	}
	return append(env, key+"="+val)
}
