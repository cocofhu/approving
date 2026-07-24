// Package codebuddy implements the CodeBuddy ACP backend.
package codebuddy

import (
	"os"
	"strings"

	"backend/internal/backend/common"
)

const (
	envCodeBuddyRegion   = "ACP_CODEBUDDY_REGION"
	envCodeBuddyInternet = "CODEBUDDY_INTERNET_ENVIRONMENT"
	envCodeBuddyBaseURL  = "CODEBUDDY_BASE_URL"

	envCodeBuddyAPIKey      = "CODEBUDDY_API_KEY"
	envAcpCodeBuddyKey      = "ACP_CODEBUDDY_API_KEY"
	envCodeBuddyKeyDisabled = "CODEBUDDY_API_KEY_DISABLED"
)

// Backend spawns `codebuddy --acp`.
type Backend struct{ common.Base }

// New returns the codebuddy backend.
func New() common.Backend { return Backend{} }

func (Backend) Name() common.Name { return common.CodeBuddy }

func (Backend) Runtime() string { return "codebuddy-acp" }

func (Backend) DefaultConfigRoot() string { return "/root/.codebuddy" }

func (Backend) Argv(model string) []string {
	if model == "" {
		model = "auto"
	}
	return []string{"codebuddy", "--acp", "--model", model}
}

// AuthEnv normalizes the API key alias and maps ACP_CODEBUDDY_REGION into
// CODEBUDDY_INTERNET_ENVIRONMENT (unless the latter is set explicitly).
func (Backend) AuthEnv(env []string) []string {
	env = common.SetIfEmpty(env, envCodeBuddyAPIKey, common.FirstNonEmptyEnv(envAcpCodeBuddyKey, envCodeBuddyAPIKey))
	_ = envCodeBuddyKeyDisabled // reserved: some flows disable the key via settings.json

	region := strings.ToLower(strings.TrimSpace(common.FirstNonEmptyEnv(envCodeBuddyRegion, envCodeBuddyInternet)))
	explicitInternet := strings.TrimSpace(os.Getenv(envCodeBuddyInternet)) != ""

	switch region {
	case "", "public", "intl", "international":
		if !explicitInternet {
			env = common.UpsertEnv(env, envCodeBuddyInternet, "public")
		}
	case "internal", "cn", "china":
		if !explicitInternet {
			env = common.UpsertEnv(env, envCodeBuddyInternet, "internal")
		}
	case "ioa":
		if !explicitInternet {
			env = common.UpsertEnv(env, envCodeBuddyInternet, "ioa")
		}
	case "staging":
		if !explicitInternet {
			env = common.UpsertEnv(env, envCodeBuddyInternet, "public")
		}
		_ = envCodeBuddyBaseURL // staging endpoint is written via settings.json in container flows
	default:
		if !explicitInternet && region != "" {
			env = common.UpsertEnv(env, envCodeBuddyInternet, region)
		}
	}
	return env
}
