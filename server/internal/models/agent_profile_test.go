package models

import "testing"

func TestAgentProfileReadPrefersNewKey(t *testing.T) {
	cfg := map[string]any{
		AgentProfileKey:       "NewAgent",
		LegacyAgentProfileKey: "OldAgent",
	}
	if got := AgentProfile(cfg); got != "NewAgent" {
		t.Fatalf("got %q", got)
	}
}

func TestAgentProfileFallsBackToLegacy(t *testing.T) {
	cfg := map[string]any{LegacyAgentProfileKey: "LegacyAgent"}
	if got := AgentProfile(cfg); got != "LegacyAgent" {
		t.Fatalf("got %q", got)
	}
}

func TestAgentProfileEmptyNewFallsBack(t *testing.T) {
	cfg := map[string]any{
		AgentProfileKey:       "",
		LegacyAgentProfileKey: "LegacyAgent",
	}
	if got := AgentProfile(cfg); got != "LegacyAgent" {
		t.Fatalf("got %q", got)
	}
}

func TestSetAgentProfileDropsLegacy(t *testing.T) {
	cfg := map[string]any{LegacyAgentProfileKey: "Old"}
	SetAgentProfile(cfg, "New")
	if got := cfg[AgentProfileKey]; got != "New" {
		t.Fatalf("new=%v", got)
	}
	if _, ok := cfg[LegacyAgentProfileKey]; ok {
		t.Fatal("legacy should be gone")
	}
}

func TestNormalizeAgentProfileLegacyOnly(t *testing.T) {
	cfg := map[string]any{LegacyAgentProfileKey: "ImplementAgent", "prompt": "x"}
	if !NormalizeAgentProfile(cfg) {
		t.Fatal("expected change")
	}
	if got, _ := cfg[AgentProfileKey].(string); got != "ImplementAgent" {
		t.Fatalf("got %v", cfg[AgentProfileKey])
	}
	if _, ok := cfg[LegacyAgentProfileKey]; ok {
		t.Fatal("legacy remains")
	}
	if NormalizeAgentProfile(cfg) {
		t.Fatal("second pass should be no-op")
	}
}

func TestNormalizeAgentProfileEmptyNewKeepsLegacy(t *testing.T) {
	cfg := map[string]any{
		AgentProfileKey:       "",
		LegacyAgentProfileKey: "LegacyAgent",
	}
	if !NormalizeAgentProfile(cfg) {
		t.Fatal("expected change")
	}
	if got, _ := cfg[AgentProfileKey].(string); got != "LegacyAgent" {
		t.Fatalf("got %v", cfg[AgentProfileKey])
	}
	if _, ok := cfg[LegacyAgentProfileKey]; ok {
		t.Fatal("legacy remains")
	}
}

func TestNormalizeGraphAgentProfiles(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: "n1", Config: map[string]any{LegacyAgentProfileKey: "A"}},
		{ID: "n2", Config: map[string]any{AgentProfileKey: "B"}},
	}}
	if !NormalizeGraphAgentProfiles(&g) {
		t.Fatal("expected change")
	}
	if AgentProfile(g.Nodes[0].Config) != "A" {
		t.Fatalf("n1=%v", g.Nodes[0].Config)
	}
	if _, ok := g.Nodes[0].Config[LegacyAgentProfileKey]; ok {
		t.Fatal("n1 legacy")
	}
	if NormalizeGraphAgentProfiles(&g) {
		t.Fatal("second pass should be no-op")
	}
}
