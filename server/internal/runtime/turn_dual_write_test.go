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

func TestSplitTurnDualWriteRecoversFinalValidFenceWithLightweightNoise(t *testing.T) {
	raw := "已完成调整。\n\n```json\n{\"agentSummary\":\"用户要求收紧总结输出契约。\"}\n```\n\n以上为本轮处理说明。"
	narr, sum := splitTurnDualWrite(raw)
	if narr != "已完成调整。\n\n以上为本轮处理说明。" {
		t.Fatalf("narration = %q", narr)
	}
	if sum != "用户要求收紧总结输出契约。" {
		t.Fatalf("summary = %q", sum)
	}
}

func TestSplitTurnDualWriteUsesLastValidSummaryFence(t *testing.T) {
	raw := "先前草稿。\n\n```json\n{\"agentSummary\":\"旧总结\"}\n```\n\n最终回复。\n\n```json\n{\"agentSummary\":\"用户确认最终方案。\"}\n```"
	narr, sum := splitTurnDualWrite(raw)
	if narr != "先前草稿。\n\n```json\n{\"agentSummary\":\"旧总结\"}\n```\n\n最终回复。" {
		t.Fatalf("narration = %q", narr)
	}
	if sum != "用户确认最终方案。" {
		t.Fatalf("summary = %q", sum)
	}
}

func TestSplitTurnDualWriteDoesNotAbsorbMidNarrationFence(t *testing.T) {
	raw := "示例：\n```json\n{\"agentSummary\":\"这不是本轮总结\"}\n```\n" + strings.Repeat("后续叙述内容。", 30)
	narr, sum := splitTurnDualWrite(raw)
	if narr != raw || sum != "" {
		t.Fatalf("mid-narration fence must stay untouched; got narr=%q sum=%q", narr, sum)
	}
}

func TestSplitTurnDualWriteBaselineFixtures(t *testing.T) {
	// This fixture set is the f3/n1 baseline: valid summary recovery must
	// improve without accepting empty keys, unrelated JSON, or raw narration.
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"terminal valid fence", "回复\n```json\n{\"agentSummary\":\"有效总结\"}\n```", true},
		{"whitespace after fence", "回复\n```json\n{\"agentSummary\":\"有效总结\"}\n```\n \t", true},
		{"lightweight postscript", "回复\n```json\n{\"agentSummary\":\"有效总结\"}\n```\n完成。", true},
		{"empty summary", "回复\n```json\n{\"agentSummary\":\" \"}\n```", false},
		{"unrelated fence", "回复\n```json\n{\"foo\":\"bar\"}\n```", false},
		{"no fence", "回复正文不能被当作总结。", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, sum := splitTurnDualWrite(tt.raw)
			if (sum != "") != tt.want {
				t.Fatalf("summary = %q, want present=%v", sum, tt.want)
			}
		})
	}
}

func TestWithDualWriteContractAppends(t *testing.T) {
	got := withDualWriteContract("请改标题")
	for _, part := range []string{"请改标题", "agentSummary", "统一总结契约", "确认并流转", "全部人工反馈", "空字符串"} {
		if !strings.Contains(got, part) {
			t.Fatalf("contract missing %q in: %s", part, got)
		}
	}
	if strings.Contains(got, "本轮输出契约") {
		t.Fatal("unified contract must not use per-turn dual-write wording")
	}
	if strings.Contains(got, "对本轮反馈要点的归纳") {
		t.Fatal("unified contract must summarize all feedback, not only this turn")
	}
}
