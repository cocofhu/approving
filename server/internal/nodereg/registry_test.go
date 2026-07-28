package nodereg

import (
	"testing"

	"github.com/cocofhu/approving/internal/mcp"
)

func TestRegistryStructuredProducts(t *testing.T) {
	cases := map[string]struct{ artifact, tool string }{
		"research": {mcp.ResearchArtifactName, "set_research"},
		"test":     {mcp.TestResultArtifactName, "set_test_result"},
		"review":   {mcp.ReviewArtifactName, "set_review"},
		"plan":     {mcp.PlanArtifactName, "set_plan"},
	}
	for typ, want := range cases {
		a, tool := StructuredProduct(typ)
		if a != want.artifact || tool != want.tool {
			t.Fatalf("%s: got (%q,%q) want (%q,%q)", typ, a, tool, want.artifact, want.tool)
		}
	}
	if _, tool := StructuredProduct("agent"); tool != "" {
		t.Fatalf("agent should have no structured tool, got %q", tool)
	}
}

func TestBuildManifestMatchesRegistry(t *testing.T) {
	m := BuildManifest()
	if m.OutputKeyToArtifact["research"] != mcp.ResearchArtifactName {
		t.Fatal("research mapping")
	}
	if m.ArtifactToOutputJSON[mcp.TestResultArtifactName] != "test_result_json" {
		t.Fatal("test json key")
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("unknown type")
	}
}

func TestEmbeddedRules(t *testing.T) {
	rules := EmbeddedRuleFiles("test")
	if len(rules) != 1 || rules[0] != "rules/test.md" {
		t.Fatalf("test rules: %v", rules)
	}
	if len(EmbeddedRuleFiles("branch")) != 0 {
		t.Fatal("branch should have no embedded rules")
	}
}

// frontendNodeTypes mirrors web/src/lib/types.ts NodeType union.
var frontendNodeTypes = []string{
	"input", "output", "react", "agent", "plan", "implement",
	"research", "test", "review", "proposal", "proposal_select",
	"submit_mr", "visual", "human_gate", "app_preview", "branch", "set_var",
}

func TestRegistryCoversFrontendNodeTypes(t *testing.T) {
	known := map[string]bool{}
	for _, t := range KnownTypes() {
		known[t] = true
	}
	for _, typ := range frontendNodeTypes {
		if !known[typ] {
			t.Fatalf("backend registry missing frontend node type %q", typ)
		}
	}
	if len(KnownTypes()) != len(frontendNodeTypes) {
		t.Fatalf("registry has %d types, frontend expects %d", len(KnownTypes()), len(frontendNodeTypes))
	}
}

func TestStructuredNodesHaveRendererAndTool(t *testing.T) {
	for _, typ := range []string{"react", "research", "test", "review", "proposal", "implement"} {
		s, ok := Get(typ)
		if !ok {
			t.Fatalf("missing %s", typ)
		}
		if s.SetTool == "" || s.ArtifactName == "" || s.OutputKey == "" {
			t.Fatalf("%s missing structured fields: %+v", typ, s)
		}
		if Renderer(s.Render) == nil {
			t.Fatalf("%s missing renderer", typ)
		}
	}
}

func TestGatedNodes(t *testing.T) {
	if s, _ := Get("test"); s.Gate != GateTest {
		t.Fatal("test gate kind")
	}
	if s, _ := Get("review"); s.Gate != GateReview {
		t.Fatal("review gate kind")
	}
}

func TestPromptContractText(t *testing.T) {
	if PromptContractText(nil, "research", "", "") == "" {
		t.Fatal("research contract")
	}
	if PromptContractText(nil, "agent", "", "") != "" {
		t.Fatal("agent should have no fixed contract")
	}
}

func TestReviewCapableDefaults(t *testing.T) {
	// test is intentionally excluded: it already has a structured gate verdict
	// path and is not part of the post-run ReAct review surface.
	for _, typ := range []string{"plan", "implement", "research", "review", "proposal", "visual", "app_preview"} {
		if !ReviewCapable(typ) {
			t.Fatalf("%s should be review-capable", typ)
		}
		if DefaultReviewVar(typ) != "review" {
			t.Fatalf("%s DefaultReviewVar = %q, want review", typ, DefaultReviewVar(typ))
		}
	}
	for _, typ := range []string{"test", "react", "agent", "human_gate", "proposal_select", "input", "nope"} {
		if ReviewCapable(typ) {
			t.Fatalf("%s must not be review-capable", typ)
		}
		if DefaultReviewVar(typ) != "" {
			t.Fatalf("%s DefaultReviewVar should be empty", typ)
		}
	}
}
