package channels

import (
	"strings"
	"testing"
)

// TestOperationalPhraseAckAlignmentLock nails the SSOT contract:
// shared colloquial/one-line atoms are byte-identical across paths, and
// intentional operational-only differences remain present (not flattened).
func TestOperationalPhraseAckAlignmentLock(t *testing.T) {
	if spokenRuleColloquial == "" || spokenRuleOneLine == "" {
		t.Fatal("shared spoken rule atoms must be non-empty")
	}
	if phraseAckRuleColloquial != spokenRuleColloquial {
		t.Fatal("phraseAckRuleColloquial must alias spokenRuleColloquial")
	}
	if phraseAckRuleOneLine != spokenRuleOneLine {
		t.Fatal("phraseAckRuleOneLine must alias spokenRuleOneLine")
	}
	if !strings.Contains(operationalLineRules, spokenRuleColloquial) {
		t.Fatal("operationalLineRules must embed spokenRuleColloquial")
	}
	if !strings.Contains(operationalLineRules, spokenRuleOneLine) {
		t.Fatal("operationalLineRules must embed spokenRuleOneLine")
	}
	if strings.Count(operationalLineRules, spokenRuleColloquial) != 1 {
		t.Fatal("operationalLineRules must embed spokenRuleColloquial exactly once")
	}
	if strings.Count(operationalLineRules, spokenRuleOneLine) != 1 {
		t.Fatal("operationalLineRules must embed spokenRuleOneLine exactly once")
	}

	// Intentional differences must remain (see operational_rules.go comments).
	if !strings.Contains(operationalLineRules, operationalRuleNameWhenGiven) {
		t.Fatal("operational conditional naming rule missing")
	}
	if strings.Contains(phraseAckRuleNameIt, "内部参考里给了是哪件事的") {
		t.Fatal("phraseAck naming must stay unconditional; do not copy operational conditional wording")
	}
	if !strings.Contains(operationalLineRules, operationalRuleNoTeachUser) {
		t.Fatal("operational exclusive no-teach-user ban missing")
	}
	if !strings.Contains(operationalLineRules, operationalRuleNoFinishedClaim) {
		t.Fatal("operational exclusive no-finished-claim ban missing")
	}
	if !strings.Contains(operationalRuleNoInternalExecEnv, "执行环境") {
		t.Fatal("operational internal-term ban must use 执行环境")
	}
	if strings.Contains(operationalRuleNoInternalExecEnv, "沙箱") {
		t.Fatal("operational internal-term ban must not use 沙箱")
	}
	if !strings.Contains(phraseAckRuleNoInternal, "沙箱") {
		t.Fatal("phraseAck internal-term ban must keep 沙箱")
	}
	if strings.Contains(phraseAckRuleNoInternal, "执行环境") {
		t.Fatal("phraseAck internal-term ban must not use 执行环境")
	}
	if !strings.Contains(operationalLineRules, operationalRuleNoInternalExecEnv) {
		t.Fatal("operationalLineRules must embed exec-env internal ban")
	}
	if strings.Contains(operationalLineRules, "沙箱") {
		t.Fatal("operationalLineRules must not introduce 沙箱 wording")
	}

	// Ack prompts still reuse the shared atoms exactly once each.
	acks := []string{
		retryAckPhrasePrompt,
		fallthroughAckPhrasePrompt,
		dispatchAckPhrasePrompt,
		refineAckPhrasePrompt,
	}
	for i, p := range acks {
		if strings.Count(p, spokenRuleColloquial) != 1 {
			t.Fatalf("ack %d must embed spokenRuleColloquial exactly once", i)
		}
		if strings.Count(p, spokenRuleOneLine) != 1 {
			t.Fatalf("ack %d must embed spokenRuleOneLine exactly once", i)
		}
	}

	// Rebuilding operational rules must stay byte-stable (builder is the SSOT).
	if got := buildOperationalLineRules(); got != operationalLineRules {
		t.Fatalf("buildOperationalLineRules drifted from operationalLineRules:\n%s\n---\n%s", got, operationalLineRules)
	}
}
