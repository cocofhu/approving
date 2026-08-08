package channels

import (
	"regexp"
	"strings"

	"github.com/cocofhu/approving/internal/textutil"
)

// Everything in this file exists for one reason: a user reading a QQ message
// should never have to know that Approving has Runs, sandboxes, turns or an ACP
// transport. Those words are how the platform talks to itself. The web client
// already enforces the same rule for its copy (see userFacingCopy.test.ts); this
// is the equivalent gate for the channel egress, applied at runtime rather than
// only in tests because outbound text is partly composed by a model.

// identifierToken matches anything shaped like a run or task id. It matches the
// shape rather than a fixed alphabet on purpose: ids are uuid-derived today, so
// a guard that only knows hex would stop guarding the moment that changes.
//
// The separator must be "-" or "_", which is how an id is always written. An
// earlier version also accepted a space, and that swallowed ordinary prose:
// "we run 5000 iterations" matched as a whole and left " iterations" behind.
var identifierToken = regexp.MustCompile(`(?i)\b(?:run|task)[-_][0-9a-z]{4,}\b`)

// internalIDPatterns match identifier spellings that are unambiguous on their
// own, without the digit test isIdentifier applies.
var internalIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brun\s*#\s*[0-9a-z]{4,}\b`),
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

// repoCoordinates match the way a working agent cites where it looked: a commit
// hash, a merge commit subject, a HEAD snapshot. They are precise and useful in
// a report, and meaningless to someone reading QQ — 「对照基线（git: 90713d62
// Merge #177），之后到 HEAD（652b0d68 Merge #178）」 tells the user nothing about
// what was decided.
//
// Each pattern is anchored on its label (git:, Merge #, HEAD) rather than on the
// hash alone: a bare hex run also matches ordinary numbers and ids, and deleting
// those would eat real content.
var repoCoordinates = []*regexp.Regexp{
	regexp.MustCompile(`(?i)[（(]\s*git\s*[:：]\s*[0-9a-f]{7,40}[^)）]*[)）]`),
	regexp.MustCompile(`(?i)\bgit\s*[:：]\s*[0-9a-f]{7,40}\b`),
	regexp.MustCompile(`(?i)\bHEAD\s*[（(][0-9a-f]{7,40}[^)）]*[)）]`),
	regexp.MustCompile(`(?i)\bMerge\s*#\s*\d+\b`),
	regexp.MustCompile(`(?i)\b[0-9a-f]{7,40}\s+Merge\s*#\s*\d+\b`),
}

// verdictScaffolding matches the headings a working agent puts on a written
// report. The sentence after the heading is the actual finding and is kept; only
// the label is dropped, because 「最终判定：需局部修订」 read aloud to a colleague
// is just 「需局部修订」.
var verdictScaffolding = regexp.MustCompile(`(?m)(^|[。；;\n])\s*(最终判定|结论状态|判定结果|建议下一步|下一步建议|置信度|证据链)\s*[:：]\s*`)

// deliveryReceiptLine matches whole-line model asides that restate a successful
// pm_reply / channel delivery. These leaked through FinalSummary after #161.
var deliveryReceiptLine = regexp.MustCompile(`(?im)^\s*(已发送|已通过\s*QQ\s*回复用户|稍等，?我看一下|Give me a moment on this one|已开始处理|任务已启动|收到，正在处理)\s*[。.!！…]*\s*$`)

// ScrubForOutbound is the single policy entry for user-visible channel text.
// Channel egress (capture/report/delivery/reflow) passes text through and
// sendOutboundResult applies this as the final gate before transport.
func ScrubForOutbound(text string) string {
	return ScrubInternalTerms(text)
}

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
	out = deliveryReceiptLine.ReplaceAllString(out, "")
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
	for _, p := range repoCoordinates {
		out = p.ReplaceAllString(out, "")
	}
	out = verdictScaffolding.ReplaceAllString(out, "$1")
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
	for _, p := range repoCoordinates {
		if p.MatchString(text) {
			return true
		}
	}
	if verdictScaffolding.MatchString(text) {
		return true
	}
	for _, r := range internalTermReplacements {
		if r.pattern.MatchString(text) {
			return true
		}
	}
	return internalMarkerLines.MatchString(text) || toolNoiseLine.MatchString(text) || deliveryReceiptLine.MatchString(text)
}

// quotedSpan matches the ways outbound copy quotes a task title. A title is
// the one fragment in a message that was shortened somewhere else and then
// pasted into a sentence, so it is where a bad cut surfaces.
var quotedSpan = regexp.MustCompile(`「[^」]*」|『[^』]*』|“[^”]*”`)

// RepairTruncatedCopy is the last line of defence against text that was cut
// badly before it got here — a title stored as 「快模型和 wo」 under the old
// hard cut, or a payload byte-sliced through a Chinese character. The cut sites
// themselves are fixed, but records written before that are still in the
// database and still get quoted into new messages, and a user reading
// 「快模型和 wo」已经跑完了 has no way to know which task that was.
//
// It only repairs; it never shortens. Anything it cannot recognise as debris is
// passed through untouched.
func RepairTruncatedCopy(text string) string {
	out := strings.ReplaceAll(text, "\uFFFD", "")
	out = quotedSpan.ReplaceAllStringFunc(out, func(span string) string {
		r := []rune(span)
		inner := string(r[1 : len(r)-1])
		healed := textutil.HealBrokenTail(inner)
		if healed == inner {
			return span
		}
		return string(r[0]) + healed + string(r[len(r)-1])
	})
	return textutil.HealBrokenTail(out)
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
