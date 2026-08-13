package runtime

import (
	"strings"
	"testing"
)

func TestSplitTurnDualWriteTrailingFence(t *testing.T) {
	raw := "已按标注改了摘要。\n\n```json\n{\"agentSummary\":\"用户要求补充竞品对比证据。\"}\n```"
	narr, sum := splitTurnDualWrite(raw)
	if narr != "已按标注改了摘要。" {
		t.Fatalf("narration = %q", narr)
	}
	if sum != "用户要求补充竞品对比证据。" {
		t.Fatalf("summary = %q", sum)
	}
}

func TestSplitTurnDualWriteWholeJSON(t *testing.T) {
	raw := `{"narration":"继续澄清缓存选型。","agentSummary":"用户确认缓存用 Redis。"}`
	narr, sum := splitTurnDualWrite(raw)
	if narr != "继续澄清缓存选型。" || sum != "用户确认缓存用 Redis。" {
		t.Fatalf("got narr=%q sum=%q", narr, sum)
	}
}

func TestSplitTurnDualWriteEmptyOmitsSummary(t *testing.T) {
	raw := "普通回复,无总结代码块。"
	narr, sum := splitTurnDualWrite(raw)
	if narr != raw || sum != "" {
		t.Fatalf("got narr=%q sum=%q", narr, sum)
	}
}

func TestSplitTurnDualWriteEmptySummaryKeyNoFallback(t *testing.T) {
	raw := "气泡原文\n\n```json\n{\"agentSummary\":\"  \"}\n```"
	narr, sum := splitTurnDualWrite(raw)
	if narr != "气泡原文" {
		t.Fatalf("narration = %q", narr)
	}
	if sum != "" {
		t.Fatalf("empty summary must not fall back; got %q", sum)
	}
}

func TestSplitTurnDualWriteUnrelatedFenceKept(t *testing.T) {
	raw := "说明如下\n\n```json\n{\"foo\":1}\n```"
	narr, sum := splitTurnDualWrite(raw)
	if narr != raw || sum != "" {
		t.Fatalf("unrelated fence must stay in narration; got narr=%q sum=%q", narr, sum)
	}
}

func TestWithDualWriteContractAppends(t *testing.T) {
	got := withDualWriteContract("请改标题")
	for _, part := range []string{"请改标题", "agentSummary", "本轮输出契约"} {
		if !strings.Contains(got, part) {
			t.Fatalf("contract missing %q in: %s", part, got)
		}
	}
}
