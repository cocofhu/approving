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
	if nilP.ReviewCommitWrapUpFor("a.go") == "" {
		t.Fatal("nil review commit wrap-up")
	}
	p.ReviewCommitWrapUp = "FILES:{files}"
	if got := p.ReviewCommitWrapUpFor("- untracked tmp.log"); got != "FILES:- untracked tmp.log" {
		t.Fatalf("wrap-up=%q", got)
	}
	got := (&AgentPrompts{}).ReviewCommitWrapUpFor("- untracked tmp.log")
	for _, want := range []string{"禁止 `git add -A`", "tmp.log", "临时文件", "git status"} {
		if !strings.Contains(got, want) {
			t.Fatalf("DefaultReviewCommitWrapUp missing %q\n%s", want, got)
		}
	}
}

func TestDefaultOutcomeContractDetachesLongRunningServices(t *testing.T) {
	got := DefaultOutcomeContract
	for _, want := range []string{
		"setsid",
		"nohup",
		"禁止前台或未脱钩的命令占住 Agent 回合",
		"不要为收尾杀掉这些进程",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("DefaultOutcomeContract missing %q\n---\n%s", want, got)
		}
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
		// list-first + idempotent success (Demo s1/s2/s3)
		"list open",
		"不得直接判 failed",
		"already exists",
		"No commits between",
		"跳过新建",
		"已合并无新提交",
		"无历史单已同步",
		"无差异且无历史单可复用",
		// closed + true failures must stay failed (Demo s4 / s5)
		"closed 未合并",
		"不得仅因存在 closed URL 而 success",
		"无法 push",
		"鉴权/权限失败",
		"冲突未解决",
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Fatalf("DefaultMRContract missing %q\n---\n%s", want, got)
		}
	}
}
