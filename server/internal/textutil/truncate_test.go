package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const truncatedSuffix = "…(truncated)"

func TestTruncateBytesASCII(t *testing.T) {
	if got := TruncateBytes("abc", 5, truncatedSuffix); got != "abc" {
		t.Errorf("short unchanged = %q", got)
	}
	if got := TruncateBytes("abcdef", 3, truncatedSuffix); got != "abc"+truncatedSuffix {
		t.Errorf("ascii truncate = %q", got)
	}
}

func TestTruncateBytesValidUTF8(t *testing.T) {
	cases := []struct {
		s, suffix string
		maxBytes  int
	}{
		{"abcdef", truncatedSuffix, 3},
		{chineseBoundarySample(), truncatedSuffix, 3998},
		{chineseBoundarySample(), truncatedSuffix, 4000},
		{chineseBoundarySample(), truncatedSuffix, 4001},
		{"hello世界emoji🎉tail", truncatedSuffix, 12},
	}
	for _, tc := range cases {
		got := TruncateBytes(tc.s, tc.maxBytes, tc.suffix)
		if !utf8.ValidString(strings.TrimSuffix(got, tc.suffix)) {
			t.Errorf("TruncateBytes(%q, %d) prefix invalid UTF-8: %q", tc.s, tc.maxBytes, got)
		}
		if !utf8.ValidString(got) {
			t.Errorf("TruncateBytes(%q, %d) result invalid UTF-8: %q", tc.s, tc.maxBytes, got)
		}
	}
}

func TestTruncateBytesChineseBoundary(t *testing.T) {
	sample := chineseBoundarySample()
	for _, n := range []int{3998, 4000, 4001} {
		got := TruncateBytes(sample, n, truncatedSuffix)
		if !utf8.ValidString(got) {
			t.Fatalf("n=%d: invalid UTF-8: %q", n, got)
		}
		if len(sample) > n && !strings.HasSuffix(got, truncatedSuffix) {
			t.Fatalf("n=%d: missing suffix: %q", n, got)
		}
		if len(sample) <= n && got != sample {
			t.Fatalf("n=%d: should be unchanged: %q", n, got)
		}
		prefix := strings.TrimSuffix(got, truncatedSuffix)
		if strings.Contains(prefix, "\uFFFD") {
			t.Fatalf("n=%d: contains replacement char: %q", n, got)
		}
	}
}

func TestTruncateBytesSuffixNotCounted(t *testing.T) {
	got := TruncateBytes("abcdef", 3, truncatedSuffix)
	want := "abc" + truncatedSuffix
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if len(got) > 3+len(truncatedSuffix)+2 {
		t.Errorf("unexpected length %d", len(got))
	}
}

func TestTruncateTailBytes(t *testing.T) {
	const prefix = "…(truncated)…\n"
	long := strings.Repeat("a", 100) + "确保失败信息在末尾"
	tailBudget := 40

	got := TruncateTailBytes(long, tailBudget, prefix)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8: %q", got)
	}
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("missing prefix: %q", got)
	}
	if !strings.Contains(got, "失败信息在末尾") {
		t.Fatalf("tail content lost: %q", got)
	}

	short := "short log"
	if TruncateTailBytes(short, tailBudget, prefix) != short {
		t.Errorf("short string should be unchanged")
	}
}

func TestTruncateTailBytesUTF8Alignment(t *testing.T) {
	// Prefix bytes that would split a multi-byte rune at the start of the tail.
	s := strings.Repeat("x", 20) + "中文结尾"
	maxBytes := len("中文结尾") + 1 // cuts into first byte of 中
	got := TruncateTailBytes(s, maxBytes, "P:")
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8: %q", got)
	}
	if !strings.Contains(got, "结尾") {
		t.Fatalf("lost tail content: %q", got)
	}
}

func chineseBoundarySample() string {
	prefix := strings.Repeat("我需要深入分析这个问题，首先阅读上游产物，然后检查 server/internal/sandbox/events.go 中的 truncateText 实现。关键发现：当 Agent 输出较长中文思考文本时，", 20)
	suffix := "保所有中文字符在截断边界处保持完整，避免出现 U+FFFD 替换字符。接下来将统一替换 4 处风险点并补充单测。"
	return prefix + "确" + suffix
}
