package database

import (
	"strings"
	"testing"
)

func TestGitlabFeatureMergeGateBodyPrefersChangesSummary(t *testing.T) {
	g := gitlabFeatureGraph(seedRepoURL)
	var body string
	var produces string
	found := false
	for _, n := range g.Nodes {
		if n.ID == "implement" {
			produces, _ = n.Config["produces"].(string)
		}
		if n.ID == "done" && n.Type == "human_gate" && n.Label == "合并确认" {
			body, _ = n.Config["body_template"].(string)
			found = true
		}
	}
	if !found {
		t.Fatal("missing 合并确认 human_gate")
	}
	// Inventory: seed human_gate must reference the human-editable produce.
	if !strings.Contains(produces, "changes_summary.md") {
		t.Fatalf("implement produces should include changes_summary.md, got %q", produces)
	}
	if !strings.Contains(body, `{{artifact("changes_summary.md")}}`) {
		t.Fatalf("body_template should reference changes_summary.md, got %q", body)
	}
	if !strings.Contains(body, "{{nodes.implement.outputs.mr_url}}") {
		t.Fatalf("body_template should keep mr_url as link, got %q", body)
	}
	if strings.TrimSpace(body) == "MR: {{nodes.implement.outputs.mr_url}}" {
		t.Fatal("body_template must not be MR-url-only")
	}
}
