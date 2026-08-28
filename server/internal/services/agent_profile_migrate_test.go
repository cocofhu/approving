package services

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestMigrateAgentProfileInGraph_legacyOnly(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "n1", Type: "implement", Config: map[string]any{"skill_profile": "ImplementAgent", "prompt": "x"}},
			{ID: "n2", Type: "output", Config: map[string]any{"results": []any{"a"}}},
		},
	}
	if !MigrateAgentProfileInGraph(&g) {
		t.Fatal("expected change")
	}
	cfg := g.Nodes[0].Config
	if _, ok := cfg["skill_profile"]; ok {
		t.Fatal("legacy key should be removed")
	}
	if got, _ := cfg["agent_profile"].(string); got != "ImplementAgent" {
		t.Fatalf("agent_profile=%v", cfg["agent_profile"])
	}
	if MigrateAgentProfileInGraph(&g) {
		t.Fatal("reentrant migrate should be no-op")
	}
}

func TestMigrateAgentProfileInGraph_bothKeysPreferNew(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "n1", Type: "plan", Config: map[string]any{
				"skill_profile": "OldAgent",
				"agent_profile": "NewAgent",
			}},
		},
	}
	if !MigrateAgentProfileInGraph(&g) {
		t.Fatal("expected change (delete legacy)")
	}
	cfg := g.Nodes[0].Config
	if _, ok := cfg["skill_profile"]; ok {
		t.Fatal("legacy key should be removed")
	}
	if got, _ := cfg["agent_profile"].(string); got != "NewAgent" {
		t.Fatalf("should keep agent_profile value, got %v", cfg["agent_profile"])
	}
}

func TestMigrateAgentProfileInGraph_alreadyNew(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "n1", Type: "react", Config: map[string]any{"agent_profile": "ClarifyAgent"}},
		},
	}
	if MigrateAgentProfileInGraph(&g) {
		t.Fatal("already-new graph must not change")
	}
	if got, _ := g.Nodes[0].Config["agent_profile"].(string); got != "ClarifyAgent" {
		t.Fatalf("value mutated: %v", g.Nodes[0].Config["agent_profile"])
	}
}

func TestMigrateAgentProfileInGraph_emptyLegacy(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "n1", Type: "agent", Config: map[string]any{"skill_profile": ""}},
		},
	}
	if !MigrateAgentProfileInGraph(&g) {
		t.Fatal("expected change")
	}
	got, _ := g.Nodes[0].Config["agent_profile"].(string)
	if got != "" {
		t.Fatalf("empty legacy should become empty agent_profile, got %q", got)
	}
	if _, ok := g.Nodes[0].Config["skill_profile"]; ok {
		t.Fatal("legacy key should be removed")
	}
}
