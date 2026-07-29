package models

import "testing"

func TestIsWeakModelKey(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"", " ", "default", "DEFAULT", "unknown", "Unknown"} {
		if !IsWeakModelKey(k) {
			t.Fatalf("expected weak: %q", k)
		}
	}
	if IsWeakModelKey("claude-sonnet-4") {
		t.Fatal("real modelID must not be weak")
	}
}

func TestAddTokenUsageByModelMergeFilled(t *testing.T) {
	t.Parallel()
	a := TokenUsageByModel{
		"claude-sonnet-4": {InputTokens: 10, Source: TokenUsageSourceUpstream},
	}
	b := TokenUsageByModel{
		"claude-sonnet-4": {InputTokens: 5, Source: TokenUsageSourceBridge, Filled: true},
	}
	out := AddTokenUsageByModel(a, b)
	got := out["claude-sonnet-4"]
	if got.InputTokens != 15 || !got.Filled || got.Source != TokenUsageSourceBridge {
		t.Fatalf("merged = %+v", got)
	}
}

func TestEffectiveUsageByModelLegacy(t *testing.T) {
	t.Parallel()
	if EffectiveUsageByModel(nil, nil) != nil {
		t.Fatal("nil+nil")
	}
	u := &TokenUsage{InputTokens: 3, OutputTokens: 1}
	m := EffectiveUsageByModel(u, nil)
	unk, ok := m[TokenUsageModelUnknown]
	if !ok || unk.InputTokens != 3 || unk.Source != TokenUsageSourceUnknown {
		t.Fatalf("legacy map = %+v", m)
	}
	// Explicit by-model wins even when empty.
	empty := TokenUsageByModel{}
	if EffectiveUsageByModel(u, empty) == nil || len(EffectiveUsageByModel(u, empty)) != 0 {
		t.Fatal("empty by-model must not invent unknown")
	}
}

func TestSumTokenUsageByModel(t *testing.T) {
	t.Parallel()
	if SumTokenUsageByModel(nil) != nil {
		t.Fatal("nil")
	}
	s := SumTokenUsageByModel(TokenUsageByModel{
		"a": {InputTokens: 1, OutputTokens: 2},
		"b": {CacheReadTokens: 3, CacheWriteTokens: 4},
	})
	if s == nil || s.Total() != 10 {
		t.Fatalf("sum = %+v", s)
	}
}
