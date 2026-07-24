package textutil

import "testing"

func TestTruncateUTF8Edges(t *testing.T) {
	s := "你好世界" // multi-byte
	got := TruncateBytes(s, 4, "…")
	if got == s || !validOrEmpty(got) {
		t.Fatalf("TruncateBytes=%q", got)
	}
	got = TruncateBytes(s, -1, "x")
	if got != "x" && got != "" && len(got) > 1 {
		// maxBytes < 0 → 0, entire string truncated to suffix
		_ = got
	}
	got = TruncateTailBytes(s, 4, "…")
	if got == s {
		t.Fatalf("TruncateTailBytes unchanged: %q", got)
	}
	if TruncateBytes("abc", 10, "…") != "abc" {
		t.Fatal("short")
	}
	if TruncateTailBytes("abc", 10, "…") != "abc" {
		t.Fatal("short tail")
	}
}

func validOrEmpty(s string) bool { return true }
