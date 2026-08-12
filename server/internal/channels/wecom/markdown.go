package wecom

import "unicode/utf8"

// MarkdownMaxBytes is the official WeCom AI-bot markdown content limit (UTF-8).
const MarkdownMaxBytes = 20480

const truncateSuffix = "已截断"

// TruncateMarkdown cuts s to MarkdownMaxBytes UTF-8 bytes and appends 「已截断」
// when the original exceeds the limit. It never splits into multiple messages.
func TruncateMarkdown(s string) string {
	if len(s) <= MarkdownMaxBytes {
		return s
	}
	keep := MarkdownMaxBytes - len(truncateSuffix)
	if keep <= 0 {
		return truncateSuffix
	}
	for keep > 0 && !utf8.ValidString(s[:keep]) {
		keep--
	}
	return s[:keep] + truncateSuffix
}
