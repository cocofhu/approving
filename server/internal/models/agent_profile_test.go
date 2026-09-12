package models

import "testing"

func TestAgentProfileReadsNewKey(t *testing.T) {
	cfg := map[string]any{AgentProfileKey: "NewAgent"}
	if got := AgentProfile(cfg); got != "NewAgent" {
		t.Fatalf("got %q", got)
	}
	if AgentProfile(nil) != "" {
		t.Fatal("nil config")
	}
	if AgentProfile(map[string]any{}) != "" {
		t.Fatal("empty config")
	}
}

func TestSetAgentProfile(t *testing.T) {
	cfg := map[string]any{}
	SetAgentProfile(cfg, "New")
	if got := cfg[AgentProfileKey]; got != "New" {
		t.Fatalf("new=%v", got)
	}
	SetAgentProfile(nil, "x")
}
