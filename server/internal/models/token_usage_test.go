package models

import "testing"

func TestAddTokenUsagePresentSemantics(t *testing.T) {
	t.Parallel()
	if AddTokenUsage(nil, nil) != nil {
		t.Fatal("nil+nil")
	}
	z := AddTokenUsage(nil, &TokenUsage{})
	if z == nil || z.Total() != 0 {
		t.Fatalf("explicit zero: %+v", z)
	}
	a := AddTokenUsage(nil, &TokenUsage{InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 4})
	b := AddTokenUsage(a, &TokenUsage{InputTokens: 10})
	if b.InputTokens != 11 || b.Total() != 1+2+3+4+10 {
		t.Fatalf("sum: %+v total=%d", b, b.Total())
	}
	if CloneTokenUsage(nil) != nil {
		t.Fatal("clone nil")
	}
	cp := CloneTokenUsage(b)
	cp.InputTokens = 0
	if b.InputTokens != 11 {
		t.Fatal("clone must not alias")
	}
}
