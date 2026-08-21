package runtime

import "testing"

func TestParseAgentSummaryFence(t *testing.T) {
	raw := "```json\n{\"agentSummary\":\"用户要求补充竞品对比证据。\"}\n```"
	if got := parseAgentSummary(raw); got != "用户要求补充竞品对比证据。" {
		t.Fatalf("summary = %q", got)
	}
}

func TestParseAgentSummaryWholeJSON(t *testing.T) {
	if got := parseAgentSummary(`{"agentSummary":"用户确认缓存用 Redis。"}`); got != "用户确认缓存用 Redis。" {
		t.Fatalf("summary = %q", got)
	}
}

func TestParseAgentSummaryToleratesSurroundingProse(t *testing.T) {
	raw := "好的,总结如下:\n\n```json\n{\"agentSummary\":\"用户收紧了流转期摘要契约。\"}\n```\n\n以上。"
	if got := parseAgentSummary(raw); got != "用户收紧了流转期摘要契约。" {
		t.Fatalf("summary = %q", got)
	}
}

func TestParseAgentSummaryUsesLastValidFence(t *testing.T) {
	raw := "```json\n{\"agentSummary\":\"旧总结\"}\n```\n\n更正:\n\n```json\n{\"agentSummary\":\"用户确认最终方案。\"}\n```"
	if got := parseAgentSummary(raw); got != "用户确认最终方案。" {
		t.Fatalf("summary = %q", got)
	}
}

func TestParseAgentSummarySkipsEmptyAndUnrelated(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty value", "```json\n{\"agentSummary\":\"  \"}\n```"},
		{"missing key", "```json\n{\"foo\":\"bar\"}\n```"},
		{"invalid json", "```json\n{not json}\n```"},
		{"plain prose", "本轮没有可归纳的内容。"},
		{"blank", "   \n "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseAgentSummary(tt.raw); got != "" {
				t.Fatalf("must not invent a summary; got %q", got)
			}
		})
	}
}

func TestParseAgentSummaryFallsBackToEarlierValidFence(t *testing.T) {
	raw := "```json\n{\"agentSummary\":\"用户确认视觉跟截图。\"}\n```\n\n```json\n{\"agentSummary\":\"\"}\n```"
	if got := parseAgentSummary(raw); got != "用户确认视觉跟截图。" {
		t.Fatalf("summary = %q", got)
	}
}
