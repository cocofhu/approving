package models

import (
	"strings"
	"testing"
)

func TestAgentPromptsRemainingContracts(t *testing.T) {
	var nilP *AgentPrompts
	if nilP.ClarifiedOpenQuestionsRetryFor([]string{"q1", "q2"}) == "" {
		t.Fatal("nil clarified retry")
	}
	if nilP.MRContractFor("feat", "main") == "" {
		t.Fatal("nil MR")
	}
	if nilP.VisualContractText() == "" || nilP.PreviewContractText() == "" || nilP.PreviewRetryText() == "" {
		t.Fatal("nil visual/preview")
	}

	p := &AgentPrompts{
		ClarifiedOpenQuestionsRetry: "Q:{items}",
		MRContract:                  "from {source} to {target}",
		VisualContract:              "V",
		PreviewContract:             "P",
		PreviewRetry:                "R",
	}
	if got := p.ClarifiedOpenQuestionsRetryFor([]string{"a"}); !strings.Contains(got, "a") {
		t.Fatalf("retry=%q", got)
	}
	if got := p.MRContractFor("s", "t"); got != "from s to t" {
		t.Fatalf("mr=%q", got)
	}
	if p.VisualContractText() != "V" || p.PreviewContractText() != "P" || p.PreviewRetryText() != "R" {
		t.Fatal("overrides")
	}
	if nilP.OutcomeContractText() == "" || nilP.OutcomeRetryText() == "" {
		t.Fatal("nil outcome")
	}
	p.OutcomeContract = "OC"
	p.OutcomeRetry = "OR"
	if p.OutcomeContractText() != "OC" || p.OutcomeRetryText() != "OR" {
		t.Fatal("outcome overrides")
	}
}

func TestDefaultMRContractHostRoutingAndFailurePhrases(t *testing.T) {
	got := (&AgentPrompts{}).MRContractFor("feat/x", "main")
	mustContain := []string{
		"glab mr create --source-branch feat/x --target-branch main --fill --yes",
		"gh pr create --base main --head feat/x --fill",
		"GITLAB_URL",
		"GITHUB_URL",
		"冲突已解决",
		"源分支已推送",
		"推送成功即可 success",
		"outputs.mr_url",
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Fatalf("DefaultMRContract missing %q\n---\n%s", want, got)
		}
	}
}
