package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeOpenCodeProvider(t *testing.T) {
	if got := NormalizeOpenCodeProvider(""); got != DefaultOpenCodeProvider {
		t.Fatalf("empty=%q", got)
	}
	if got := NormalizeOpenCodeProvider(" Anthropic "); got != "anthropic" {
		t.Fatalf("anthropic=%q", got)
	}
	// Any catalog id survives: the vendor list comes from models.dev, not from here.
	if got := NormalizeOpenCodeProvider("ZAI"); got != "zai" {
		t.Fatalf("catalog id=%q", got)
	}
	if got := NormalizeOpenCodeProvider("my-gateway.v2"); got != "my-gateway.v2" {
		t.Fatalf("gateway id=%q", got)
	}
	if got := NormalizeOpenCodeProvider("has space"); got != "custom" {
		t.Fatalf("malformed id=%q", got)
	}
}

func TestOpenCodeConfigForEnv_NilWhenBuiltInNoExtras(t *testing.T) {
	if OpenCodeConfigForEnv(BackendOpenCode, map[string]string{
		EnvOpenCodeProvider: "openai",
	}) != nil {
		t.Fatal("built-in vendor without baseURL/model should skip generated opencode.json")
	}
}

func TestAuthConfigFileExists_OpenCodeJSON(t *testing.T) {
	dir := t.TempDir()
	if AuthConfigFileExists(dir, BackendOpenCode) {
		t.Fatal("empty dir")
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !AuthConfigFileExists(dir, BackendOpenCode) {
		t.Fatal("opencode.json should satisfy OpenCode auth gate")
	}
	if AuthConfigFileExists(dir, BackendCursor) {
		t.Fatal("cursor must not treat opencode.json as settings.json")
	}
}

func TestOpenCodeConfigForEnv_NilOnOtherBackends(t *testing.T) {
	if OpenCodeConfigForEnv(BackendCursor, map[string]string{EnvACPBridgeModel: "x"}) != nil {
		t.Fatal("cursor must not emit opencode.json")
	}
}

func TestOpenCodeConfigForEnv_ModelOnly(t *testing.T) {
	doc := OpenCodeConfigForEnv(BackendOpenCode, map[string]string{
		EnvOpenCodeProvider: "anthropic",
		EnvACPBridgeModel:   "anthropic/claude-sonnet-4-5",
	})
	if doc == nil {
		t.Fatal("expected config")
	}
	if doc["model"] != "anthropic/claude-sonnet-4-5" {
		t.Fatalf("model=%v", doc["model"])
	}
	if _, ok := doc["provider"]; ok {
		t.Fatalf("built-in without a key should omit provider: %#v", doc)
	}
}

// A vendor the catalog knows needs no base URL and no npm adapter, but it does
// need the key named on it: only a handful of vendors publish an env var this
// server knows, while OPENCODE_API_KEY always holds the Agent's key.
func TestOpenCodeConfigForEnv_CatalogVendorNamesTheKey(t *testing.T) {
	doc := OpenCodeConfigForEnv(BackendOpenCode, map[string]string{
		EnvOpenCodeProvider: "zai",
		EnvOpenCodeAPIKey:   "sk-oc",
		EnvACPBridgeModel:   "zai/glm-5",
	})
	if doc == nil {
		t.Fatal("expected config")
	}
	if doc["model"] != "zai/glm-5" {
		t.Fatalf("model=%v", doc["model"])
	}
	prov, _ := doc["provider"].(map[string]any)
	zai, _ := prov["zai"].(map[string]any)
	if zai == nil {
		t.Fatalf("provider block missing: %#v", doc)
	}
	opts, _ := zai["options"].(map[string]any)
	if opts["apiKey"] != "{env:OPENCODE_API_KEY}" {
		t.Fatalf("apiKey=%v", opts["apiKey"])
	}
	if _, ok := opts["baseURL"]; ok {
		t.Fatalf("catalog vendor must keep its own endpoint: %#v", opts)
	}
	if _, ok := zai["npm"]; ok {
		t.Fatalf("catalog vendor must keep its own adapter: %#v", zai)
	}
	if _, ok := zai["models"]; ok {
		t.Fatalf("catalog vendor must keep its own model list: %#v", zai)
	}
}

func TestOpenCodeConfigForEnv_CustomRequiresProviderBlock(t *testing.T) {
	doc := OpenCodeConfigForEnv(BackendOpenCode, map[string]string{
		EnvOpenCodeProvider: "custom",
		EnvOpenCodeBaseURL:  "https://llm.example/v1",
		EnvACPBridgeModel:   "my-model",
	})
	if doc == nil {
		t.Fatal("expected config")
	}
	if doc["model"] != "custom/my-model" {
		t.Fatalf("model=%v", doc["model"])
	}
	prov, _ := doc["provider"].(map[string]any)
	custom, _ := prov["custom"].(map[string]any)
	if custom["npm"] != openCodeCompatibleNPM {
		t.Fatalf("npm=%v", custom["npm"])
	}
	opts, _ := custom["options"].(map[string]any)
	if opts["baseURL"] != "https://llm.example/v1" {
		t.Fatalf("baseURL=%v", opts["baseURL"])
	}
}

func TestMergeAuthEnv_OpenCodeMapsNativeKey(t *testing.T) {
	out, err := MergeAuthEnv(BackendOpenCode, map[string]string{
		EnvApprovingOpenCodeAPIKey: "sk-oc",
		EnvOpenCodeProvider:        "openai",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[EnvOpenCodeAPIKey] != "sk-oc" {
		t.Fatalf("OPENCODE_API_KEY=%q", out[EnvOpenCodeAPIKey])
	}
	if out["OPENAI_API_KEY"] != "sk-oc" {
		t.Fatalf("OPENAI_API_KEY=%q", out["OPENAI_API_KEY"])
	}
	if out[EnvOpenCodeProvider] != "openai" {
		t.Fatalf("provider=%q", out[EnvOpenCodeProvider])
	}
}

func TestPrepareAuthEnv_OpenCodeJSONSkipsKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"model":"openai/gpt-4.1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := PrepareAuthEnv(BackendOpenCode, map[string]string{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if out[EnvOpenCodeAPIKey] != "" {
		t.Fatalf("should not invent key: %#v", out)
	}
}
