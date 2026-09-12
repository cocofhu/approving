package config

import "testing"

func TestImageResolveOrdering(t *testing.T) {
	ic := ImageConfig{Ref: "reg/universal-sandbox:latest"}

	if got := ic.Resolve("reg/custom:tag", "opencode"); got != "reg/custom:tag" {
		t.Fatalf("override: %q", got)
	}
	if got := ic.Resolve("", "opencode"); got != "reg/universal-sandbox:latest" {
		t.Fatalf("default: %q", got)
	}
	if got := ic.Resolve("", ""); got != "reg/universal-sandbox:latest" {
		t.Fatalf("empty provider: %q", got)
	}
}
