// Package trae implements the Trae CLI ACP backend.
package trae

import (
	"os"
	"strings"

	"backend/internal/backend/common"
)

const (
	envTraeCLIToken = "TRAECLI_PERSONAL_ACCESS_TOKEN"
	envAcpTraeKey   = "ACP_TRAE_API_KEY"
	envTraeAPIKey   = "TRAE_API_KEY"
	envTraeRegion   = "ACP_TRAE_REGION"
	envTraeCLIHost  = "TRAECLI_HOST"
)

// Backend spawns `traecli acp serve`.
type Backend struct{ common.Base }

// New returns the trae backend.
func New() common.Backend { return Backend{} }

func (Backend) Name() common.Name { return common.Trae }

func (Backend) Runtime() string { return "trae-acp" }

func (Backend) DefaultConfigRoot() string { return "/root/.trae" }

func (Backend) Argv(model string) []string {
	if model == "" {
		model = "auto"
	}
	return []string{"traecli", "acp", "serve", "--model", model}
}

// AuthEnv normalizes the token alias and maps ACP_TRAE_REGION into TRAECLI_HOST
// (unless the host is set explicitly).
func (Backend) AuthEnv(env []string) []string {
	if val := common.FirstNonEmptyEnv(envAcpTraeKey, envTraeAPIKey, envTraeCLIToken); val != "" {
		env = common.UpsertEnv(env, envTraeCLIToken, val)
	}
	region := strings.ToLower(strings.TrimSpace(os.Getenv(envTraeRegion)))
	explicitHost := strings.TrimSpace(os.Getenv(envTraeCLIHost)) != ""
	switch region {
	case "intl", "international", "public", "ai":
		if !explicitHost {
			env = common.UpsertEnv(env, envTraeCLIHost, "https://www.trae.ai")
		}
	case "", "cn", "china", "internal":
		// CN default: do not force TRAECLI_HOST (aligns with server mergeTraeRegion).
	}
	return env
}
