package runtime

import (
	"regexp"
	"strings"
)

const (
	EnvOpenCodeAPIKey          = "OPENCODE_API_KEY"
	EnvApprovingOpenCodeAPIKey = "APPROVING_OPENCODE_API_KEY"
	EnvOpenCodeProvider        = "APPROVING_OPENCODE_PROVIDER"
	EnvOpenCodeBaseURL         = "APPROVING_OPENCODE_BASE_URL"
	EnvACPBridgeModel          = "ACP_BRIDGE_MODEL"
	DefaultOpenCodeProvider    = "openai"
	openCodeCompatibleNPM      = "@ai-sdk/openai-compatible"
)

// openCodeProviderID is the shape of an OpenCode catalog provider id.
var openCodeProviderID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// NormalizeOpenCodeProvider keeps a catalog provider id as it is. OpenCode
// resolves providers against models.dev, which holds hundreds of them and grows
// without our involvement, so the id is only checked for shape; anything else
// collapses onto "custom", which carries its own base URL and model.
func NormalizeOpenCodeProvider(raw string) string {
	id := strings.ToLower(strings.TrimSpace(raw))
	if id == "" {
		return DefaultOpenCodeProvider
	}
	if !openCodeProviderID.MatchString(id) {
		return "custom"
	}
	return id
}

func openCodeNativeAPIKey(provider string) string {
	switch NormalizeOpenCodeProvider(provider) {
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "google":
		return "GOOGLE_GENERATIVE_AI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	case "moonshotai":
		return "MOONSHOT_API_KEY"
	case "alibaba":
		return "DASHSCOPE_API_KEY"
	case "xai":
		return "XAI_API_KEY"
	default:
		return ""
	}
}

func mergeOpenCodeVendorEnv(env map[string]string) {
	if env == nil {
		return
	}
	provider := NormalizeOpenCodeProvider(env[EnvOpenCodeProvider])
	env[EnvOpenCodeProvider] = provider
	key := strings.TrimSpace(env[EnvOpenCodeAPIKey])
	if key == "" {
		return
	}
	if native := openCodeNativeAPIKey(provider); native != "" {
		if strings.TrimSpace(env[native]) == "" {
			env[native] = key
		}
	}
}

// OpenCodeConfigForEnv returns opencode.json contents generated from Agent env.
// Nil means do not write a file (rely on native vendor env). User-authored
// opencode.json in the config home is never overwritten by the writer.
func OpenCodeConfigForEnv(backend AcpBackend, env map[string]string) map[string]any {
	if NormalizeBackend(string(backend)) != BackendOpenCode {
		return nil
	}
	provider := NormalizeOpenCodeProvider("")
	baseURL := ""
	model := ""
	key := ""
	if env != nil {
		provider = NormalizeOpenCodeProvider(env[EnvOpenCodeProvider])
		baseURL = strings.TrimSpace(env[EnvOpenCodeBaseURL])
		model = strings.TrimSpace(env[EnvACPBridgeModel])
		key = strings.TrimSpace(env[EnvOpenCodeAPIKey])
	}
	if provider != "custom" && baseURL == "" && model == "" && key == "" {
		return nil
	}
	doc := map[string]any{
		"$schema": "https://opencode.ai/config.json",
	}
	if model != "" {
		doc["model"] = model
	}
	// Naming the key on the provider is what lets any catalog vendor work: only a
	// handful publish an env var we know by name, and OPENCODE_API_KEY is the one
	// place the Agent's key always lands.
	if provider != "custom" && baseURL == "" && key == "" {
		return doc
	}
	options := map[string]any{
		"apiKey": "{env:OPENCODE_API_KEY}",
	}
	if baseURL != "" {
		options["baseURL"] = baseURL
	}
	prov := map[string]any{"options": options}
	if provider == "custom" {
		prov["npm"] = openCodeCompatibleNPM
		prov["name"] = "Custom"
		modelID := openCodeModelID(model, provider)
		if modelID == "" {
			modelID = "default"
		}
		prov["models"] = map[string]any{
			modelID: map[string]any{"name": modelID},
		}
		if model == "" {
			doc["model"] = "custom/" + modelID
		} else if !strings.Contains(model, "/") {
			doc["model"] = "custom/" + model
		}
	}
	doc["provider"] = map[string]any{provider: prov}
	return doc
}

func openCodeModelID(model, provider string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		return ""
	}
	if prefix, rest, ok := strings.Cut(m, "/"); ok {
		if prefix == provider || prefix == "custom" {
			return rest
		}
	}
	return m
}
