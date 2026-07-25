package runtime

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

// AcpBackend identifies which ACP CLI bridge a skill_profile binds to.
type AcpBackend string

const (
	BackendCursor     AcpBackend = "cursor"
	BackendClaudeCode AcpBackend = "claude_code"
	BackendCodeBuddy  AcpBackend = "codebuddy"
	BackendTrae       AcpBackend = "trae"
)

// Region / site env keys written by Agent Studio or set manually.
const (
	EnvCodeBuddyRegion = "APPROVING_CODEBUDDY_REGION"
	EnvTraeRegion      = "APPROVING_TRAE_REGION"

	EnvCodeBuddyInternet = "CODEBUDDY_INTERNET_ENVIRONMENT"
	EnvCodeBuddyBaseURL  = "CODEBUDDY_BASE_URL"
	EnvTraeCLIHost       = "TRAECLI_HOST"
	EnvTraeCLIToken      = "TRAECLI_PERSONAL_ACCESS_TOKEN"

	CodeBuddyStagingEndpoint = "https://staging-codebuddy.tencent.com"
	TraeIntlHost             = "https://www.trae.ai"
)

// CodeBuddySettingsForEnv returns settings.json contents for CodeBuddy when the
// agent targets staging (envRouteMode+endpoint). Nil means no settings file.
// Non-CodeBuddy backends always return nil so stray REGION env cannot pollute
// cursor/claude/trae config homes.
func CodeBuddySettingsForEnv(backend AcpBackend, env map[string]string) map[string]any {
	if NormalizeBackend(string(backend)) != BackendCodeBuddy {
		return nil
	}
	region := strings.ToLower(strings.TrimSpace(env[EnvCodeBuddyRegion]))
	if region != "staging" {
		return nil
	}
	endpoint := strings.TrimSpace(env[EnvCodeBuddyBaseURL])
	if endpoint == "" {
		endpoint = CodeBuddyStagingEndpoint
	}
	return map[string]any{
		"envRouteMode": "staging",
		"endpoint":     endpoint,
		"env": map[string]string{
			EnvCodeBuddyInternet: "public",
		},
	}
}

// DefaultConfigRoot returns the protocol default config root for a backend.
func DefaultConfigRoot(b AcpBackend) string {
	switch NormalizeBackend(string(b)) {
	case BackendClaudeCode:
		return "/root/.claude"
	case BackendCodeBuddy:
		return "/root/.codebuddy"
	case BackendTrae:
		return "/root/.trae"
	default:
		return "/root/.cursor"
	}
}

// NormalizeBackend coerces unknown/empty values to cursor for backward compat.
func NormalizeBackend(raw string) AcpBackend {
	switch AcpBackend(strings.TrimSpace(raw)) {
	case BackendCursor, BackendClaudeCode, BackendCodeBuddy, BackendTrae:
		return AcpBackend(strings.TrimSpace(raw))
	default:
		return BackendCursor
	}
}

// ResolveConfigRoot applies backend default when layout configRoot is empty.
func ResolveConfigRoot(backend AcpBackend, layoutConfigRoot string) string {
	if r := strings.TrimSpace(layoutConfigRoot); r != "" {
		return r
	}
	return DefaultConfigRoot(backend)
}

// authSpec describes how agent env keys map to in-container CLI env.
type authSpec struct {
	agentKeys []string // APPROVING_* aliases accepted in agent.json env
	cliKey    string   // env var the bridge CLI reads
}

func authSpecFor(b AcpBackend) authSpec {
	switch b {
	case BackendClaudeCode:
		return authSpec{
			agentKeys: []string{"APPROVING_CLAUDE_API_KEY", "ANTHROPIC_API_KEY"},
			cliKey:    "ANTHROPIC_API_KEY",
		}
	case BackendCodeBuddy:
		return authSpec{
			agentKeys: []string{"APPROVING_CODEBUDDY_API_KEY", "CODEBUDDY_API_KEY"},
			cliKey:    "CODEBUDDY_API_KEY",
		}
	case BackendTrae:
		// Official traecli headless auth uses TRAECLI_PERSONAL_ACCESS_TOKEN;
		// keep legacy TRAE_API_KEY / APPROVING_TRAE_API_KEY as aliases.
		return authSpec{
			agentKeys: []string{
				"APPROVING_TRAE_API_KEY",
				"TRAE_API_KEY",
				EnvTraeCLIToken,
			},
			cliKey: EnvTraeCLIToken,
		}
	default:
		return authSpec{
			agentKeys: []string{"APPROVING_CURSOR_API_KEY", "CURSOR_API_KEY"},
			cliKey:    "CURSOR_API_KEY",
		}
	}
}

// MergeAuthEnv maps agent-configured API keys into CLI env names and applies
// CodeBuddy / Trae region (intl vs CN) normalization.
func MergeAuthEnv(backend AcpBackend, env map[string]string) (map[string]string, error) {
	return mergeAuthEnv(backend, env)
}

// by the sandbox bridge. Returns an error when no key is configured.
func mergeAuthEnv(backend AcpBackend, env map[string]string) (map[string]string, error) {
	spec := authSpecFor(backend)
	out := map[string]string{}
	for k, v := range env {
		out[k] = v
	}
	var val string
	for _, k := range spec.agentKeys {
		if v := strings.TrimSpace(out[k]); v != "" {
			val = v
			break
		}
	}
	if val == "" {
		if v := strings.TrimSpace(out[spec.cliKey]); v != "" {
			val = v
		}
	}
	if val == "" {
		return out, fmt.Errorf("鉴权未配置:请在项目沙箱 env 或 Agent 环境变量中设置 %s", strings.Join(spec.agentKeys, " 或 "))
	}
	out[spec.cliKey] = val
	mergeRegionEnv(backend, out)
	return out, nil
}

// mergeRegionEnv normalizes intl/CN (and CodeBuddy staging) site settings into
// the env vars each CLI actually reads. Explicit official vars win over region aliases.
func mergeRegionEnv(backend AcpBackend, env map[string]string) {
	switch backend {
	case BackendCodeBuddy:
		mergeCodeBuddyRegion(env)
	case BackendTrae:
		mergeTraeRegion(env)
	}
}

func mergeCodeBuddyRegion(env map[string]string) {
	region := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		env[EnvCodeBuddyRegion],
		env[EnvCodeBuddyInternet],
	)))
	explicitInternet := strings.TrimSpace(env[EnvCodeBuddyInternet]) != ""
	explicitBase := strings.TrimSpace(env[EnvCodeBuddyBaseURL]) != ""

	switch region {
	case "", "public", "intl", "international":
		if !explicitInternet {
			env[EnvCodeBuddyInternet] = "public"
		}
	case "internal", "cn", "china":
		if !explicitInternet {
			env[EnvCodeBuddyInternet] = "internal"
		}
	case "ioa":
		if !explicitInternet {
			env[EnvCodeBuddyInternet] = "ioa"
		}
	case "staging":
		// Staging needs settings.json envRouteMode+endpoint (BASE_URL alone
		// hits the wrong chat path). Mark via region; config-home writer applies it.
		if !explicitInternet {
			env[EnvCodeBuddyInternet] = "public"
		}
		_ = explicitBase // keep any user BASE_URL; do not invent one for staging
		env[EnvCodeBuddyRegion] = "staging"
	default:
		// Unknown region alias: leave as-is; if it looks like an official
		// INTERNET_ENVIRONMENT value already present, keep it.
		if !explicitInternet && region != "" {
			env[EnvCodeBuddyInternet] = region
		}
	}
}

func mergeTraeRegion(env map[string]string) {
	region := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		env[EnvTraeRegion],
	)))
	explicitHost := strings.TrimSpace(env[EnvTraeCLIHost]) != ""
	switch region {
	case "intl", "international", "public", "ai":
		if !explicitHost {
			env[EnvTraeCLIHost] = TraeIntlHost
		}
	case "", "cn", "china", "internal":
		// CN is the default for the sandbox Trae install (docs.trae.cn); do not
		// force TRAECLI_HOST so enterprise custom domains remain unsettable.
	default:
		// Unknown: no host mutation.
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// IsPlatformAuthEnvKey reports official ACP CLI auth keys that must not be
// injected via platform-level sandbox.env / opts.Env. Project sandbox env may
// store them as a workflow baseline (forced Secret); Agent env overrides.
func IsPlatformAuthEnvKey(k string) bool {
	switch k {
	case "CURSOR_API_KEY", "ANTHROPIC_API_KEY", "CODEBUDDY_API_KEY",
		"TRAE_API_KEY", EnvTraeCLIToken:
		return true
	default:
		return false
	}
}

// AgentRuntimeLabel is the capabilities.agent.runtime string for logging.
func AgentRuntimeLabel(b AcpBackend) string {
	switch b {
	case BackendClaudeCode:
		return "claude-code-acp"
	case BackendCodeBuddy:
		return "codebuddy-acp"
	case BackendTrae:
		return "trae-acp"
	default:
		return "cursor-agent"
	}
}

// WarnDeprecatedExecProvider logs when APPROVING_EXEC_PROVIDER is set but ignored.
func WarnDeprecatedExecProvider(name string) {
	if n := strings.TrimSpace(name); n != "" && n != "sandbox" && n != "cursor" {
		log.Warn().Str("APPROVING_EXEC_PROVIDER", n).
			Msg("APPROVING_EXEC_PROVIDER is deprecated and ignored; route agents via skill_profile acpBackend")
	}
}
