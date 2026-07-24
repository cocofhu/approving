package config

import (
	"os"
	"testing"
)

func TestImageResolveOrdering(t *testing.T) {
	ic := ImageConfig{
		Ref:        "reg/universal-sandbox:latest",
		ByProvider: map[string]string{"gemini": "reg/universal-sandbox-gemini:v1"},
		Template:   "reg/universal-sandbox-{provider}:latest",
	}

	// 1. explicit override always wins.
	if got := ic.Resolve("reg/custom:tag", "gemini"); got != "reg/custom:tag" {
		t.Fatalf("override: %q", got)
	}
	// 2. exact per-provider mapping.
	if got := ic.Resolve("", "gemini"); got != "reg/universal-sandbox-gemini:v1" {
		t.Fatalf("byProvider: %q", got)
	}
	// 3. template convention for unmapped providers.
	if got := ic.Resolve("", "codex"); got != "reg/universal-sandbox-codex:latest" {
		t.Fatalf("template: %q", got)
	}
	// 4. default ref when no provider.
	if got := ic.Resolve("", ""); got != "reg/universal-sandbox:latest" {
		t.Fatalf("default: %q", got)
	}

	// no template + unmapped provider => default ref.
	ic2 := ImageConfig{Ref: "reg/base:latest"}
	if got := ic2.Resolve("", "grok"); got != "reg/base:latest" {
		t.Fatalf("no-template fallback: %q", got)
	}
}

func TestImageEnvOverrides(t *testing.T) {
	os.Setenv("SBGW_IMAGE_TEMPLATE", "reg/sb-{provider}:latest")
	os.Setenv("SBGW_IMAGE_MAP", "gemini=reg/sb-gemini:pin, codex = reg/sb-codex:pin ")
	defer func() {
		os.Unsetenv("SBGW_IMAGE_TEMPLATE")
		os.Unsetenv("SBGW_IMAGE_MAP")
	}()

	c := Default()
	applyEnv(c)

	if c.Image.Template != "reg/sb-{provider}:latest" {
		t.Fatalf("template=%q", c.Image.Template)
	}
	if c.Image.ByProvider["gemini"] != "reg/sb-gemini:pin" {
		t.Fatalf("map gemini=%q", c.Image.ByProvider["gemini"])
	}
	if c.Image.ByProvider["codex"] != "reg/sb-codex:pin" {
		t.Fatalf("map codex=%q (trim failed?)", c.Image.ByProvider["codex"])
	}
}
