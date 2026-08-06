package channels

import (
	"regexp"
	"strings"
)

// Everything in this file exists for one reason: a user reading a QQ message
// should never have to know that Approving has Runs, sandboxes, turns or an ACP
// transport. Those words are how the platform talks to itself. The web client
// already enforces the same rule for its copy (see userFacingCopy.test.ts); this
// is the equivalent gate for the channel egress, applied at runtime rather than
// only in tests because outbound text is partly composed by a model.

// identifierToken matches anything shaped like a run or task id. Matching the
// shape rather than a fixed alphabet matters: ids are uuid-derived today, but a
// guard that only knows today's alphabet stops guarding the moment that
// changes.
var identifierToken = regexp.MustCompile(`(?i)\b(?:run|task)[-_ ]?#?[0-9a-z]{4,}\b`)

// internalIDPatterns match identifier spellings that are unambiguous on their
// own, without the digit test isIdentifier applies.
var internalIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brun\s+id\s*[:：]?\s*[0-9a-z-]{4,}\b`),
}

// isIdentifier separates "run-1ca1876f" from "run-optimization": a generated id
// carries a digit, an English phrase usually does not. Erring toward keeping
// real words is deliberate — deleting a word out of a sentence is a worse
// outcome than leaving one that merely looks technical.
func isIdentifier(token string) bool {
	for _, r := range token {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// internalTermReplacements rewrite platform vocabulary into plain language
// rather than deleting it, because a sentence with a hole in it reads worse than
// the jargon did.
var internalTermReplacements = []struct {
	pattern *regexp.Regexp
	with    string
}{
	{regexp.MustCompile(`(?i)\bassistant produced no reply\b`), "暂时没有结论"},
	{regexp.MustCompile(`(?i)\bnode_complete\b`), "阶段完成"},
	{regexp.MustCompile(`(?i)\bsuppressed\b`), "已合并"},
	{regexp.MustCompile(`(?i)\bsendable\b`), "消息"},
	{regexp.MustCompile(`(?i)\bsandbox\b`), "执行环境"},
	{regexp.MustCompile(`沙箱`), "执行环境"},
	{regexp.MustCompile(`(?i)\bacp\b`), "执行环境"},
	{regexp.MustCompile(`(?i)\bworkflow\s+run\b`), "任务"},
	{regexp.MustCompile(`(?i)\bthis turn\b`), "这一轮"},
	{regexp.MustCompile(`本回合`), "这一轮"},
}

// internalMarkerLines are whole lines of machine scaffolding: streaming markers
// and tool traces that were never meant to be read.
var internalMarkerLines = regexp.MustCompile(`(?m)^\s*(\[(摘要|进度|里程碑|阻塞|确认|需确认|失败)\]|【(摘要|进度|里程碑|阻塞|确认|需确认|失败)】)\s*`)

var toolNoiseLine = regexp.MustCompile(`(?im)^\s*(tool_call|function_call|tool result|reasoning_delta|input_tokens|output_tokens)\b.*$`)

// ScrubInternalTerms makes outbound text safe to show a user: internal
// identifiers are removed, platform vocabulary is rewritten in plain language,
// and machine scaffolding lines are dropped. It is deliberately applied to every
// egress path rather than trusted to the composer, because part of that text
// comes from a model.
func ScrubInternalTerms(text string) string {
	out := strings.TrimSpace(text)
	if out == "" {
		return ""
	}
	original := out
	out = toolNoiseLine.ReplaceAllString(out, "")
	out = internalMarkerLines.ReplaceAllString(out, "")
	out = identifierToken.ReplaceAllStringFunc(out, func(token string) string {
		if isIdentifier(token) {
			return ""
		}
		return token
	})
	for _, p := range internalIDPatterns {
		out = p.ReplaceAllString(out, "")
	}
	for _, r := range internalTermReplacements {
		out = r.pattern.ReplaceAllString(out, r.with)
	}
	return tidyWhitespace(out, out != original)
}

// ContainsInternalTerms reports whether text still exposes platform internals.
// Used by tests and by delivery logging to catch regressions at the source.
func ContainsInternalTerms(text string) bool {
	for _, token := range identifierToken.FindAllString(text, -1) {
		if isIdentifier(token) {
			return true
		}
	}
	for _, p := range internalIDPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	for _, r := range internalTermReplacements {
		if r.pattern.MatchString(text) {
			return true
		}
	}
	return internalMarkerLines.MatchString(text) || toolNoiseLine.MatchString(text)
}

// cjkGap matches a space between two Han characters. Chinese does not space its
// characters, so one appearing there is a hole left by a substitution.
var cjkGap = regexp.MustCompile(`(\p{Han})[ \t]+(\p{Han})`)

// tidyWhitespace collapses the gaps scrubbing leaves behind so the result reads
// like it was written that way. changed says whether anything was actually
// removed; when nothing was, the author's own spacing is left alone.
func tidyWhitespace(text string, changed bool) string {
	if changed {
		// Twice: overlapping matches ("字 X 字" leaves "字 字" on the first pass).
		text = cjkGap.ReplaceAllString(text, "$1$2")
		text = cjkGap.ReplaceAllString(text, "$1$2")
	}
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		for strings.Contains(line, "  ") {
			line = strings.ReplaceAll(line, "  ", " ")
		}
		// Scrubbing an id out of 「取消 run-1ca 吧」 leaves a space before the
		// punctuation that follows it.
		for _, punct := range []string{"，", "。", "：", "、", "；", "！", "？"} {
			line = strings.ReplaceAll(line, " "+punct, punct)
		}
		line = strings.TrimSpace(line)
		if line == "" && (len(kept) == 0 || kept[len(kept)-1] == "") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
