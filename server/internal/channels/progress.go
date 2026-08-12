package channels

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ProgressKind is the only progress class Reply may forward to QQ.
type ProgressKind string

const (
	// ProgressMilestone is a substantive milestone (e.g. PR opened, checks done).
	ProgressMilestone ProgressKind = "milestone"
	// ProgressBlocker is a block / failure risk that the user should know.
	ProgressBlocker ProgressKind = "blocker"
	// ProgressConfirm means Work needs an explicit user decision.
	ProgressConfirm ProgressKind = "confirm"
)

// ProgressEvent is an internal Work→Reply progress signal. Tool-level / token
// noise must not become a ProgressEvent.
type ProgressEvent struct {
	Kind    ProgressKind
	Summary string
	At      time.Time
}

// TurnFinalReport is the Work turn outcome consumed by Reply for the required
// terminal QQ message (success or failure/interrupt).
type TurnFinalReport struct {
	OK      bool
	Summary string
}

const progressSummaryRunes = 120

var progressMarkers = []struct {
	prefix string
	kind   ProgressKind
}{
	{"[确认]", ProgressConfirm},
	{"【确认】", ProgressConfirm},
	{"[需确认]", ProgressConfirm},
	{"【需确认】", ProgressConfirm},
	{"[阻塞]", ProgressBlocker},
	{"【阻塞】", ProgressBlocker},
	{"[失败]", ProgressBlocker},
	{"【失败】", ProgressBlocker},
	{"[进度]", ProgressMilestone},
	{"【进度】", ProgressMilestone},
	{"[里程碑]", ProgressMilestone},
	{"【里程碑】", ProgressMilestone},
}

// ClassifyProgressText maps assistant narration into one of the three
// forwardable progress kinds. Markers are authoritative; bare keywords are
// conservative to avoid false positives on casual chat. Empty / tool-noise /
// heartbeat-like text is rejected (ok=false) so Reply does not spam QQ.
func ClassifyProgressText(text string) (ProgressEvent, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ProgressEvent{}, false
	}
	if isProgressNoise(text) {
		return ProgressEvent{}, false
	}

	// Explicit markers from Work (preferred) — prefix or mid-buffer after coalesce.
	if pe, ok := classifyProgressMarker(text); ok {
		return pe, true
	}

	lower := strings.ToLower(text)
	switch {
	case containsAny(text, "请确认", "需要确认", "是否同意", "请选择") ||
		containsAny(lower, "please confirm", "awaiting confirmation"):
		return ProgressEvent{Kind: ProgressConfirm, Summary: truncateRunes(text, progressSummaryRunes), At: time.Now()}, true
	// Stronger blocker phrases only — bare 「错误」「失败」 alone is too noisy.
	case containsAny(text, "无法继续", "权限不足", "检查失败", "失败：", "失败:", "阻塞：", "阻塞:") ||
		containsAny(lower, "blocked:", "failed:", "error:", "timeout"):
		return ProgressEvent{Kind: ProgressBlocker, Summary: truncateRunes(text, progressSummaryRunes), At: time.Now()}, true
	case containsAny(text, "已完成", "已提交", "已推送", "PR #", "pr #", "检查通过", "已打开") ||
		containsAny(lower, "opened pr", "pushed", "merged", "milestone"):
		return ProgressEvent{Kind: ProgressMilestone, Summary: truncateRunes(text, progressSummaryRunes), At: time.Now()}, true
	default:
		return ProgressEvent{}, false
	}
}

func classifyProgressMarker(text string) (ProgressEvent, bool) {
	for _, m := range progressMarkers {
		if strings.HasPrefix(text, m.prefix) {
			rest := strings.TrimSpace(strings.TrimPrefix(text, m.prefix))
			return ProgressEvent{Kind: m.kind, Summary: truncateRunes(rest, progressSummaryRunes), At: time.Now()}, true
		}
	}
	// Mid-buffer markers after streaming coalesce (e.g. "...\n[进度] opened").
	for _, m := range progressMarkers {
		idx := strings.Index(text, m.prefix)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(text[idx+len(m.prefix):])
		if nl := strings.IndexAny(rest, "\n\r"); nl >= 0 {
			rest = strings.TrimSpace(rest[:nl])
		}
		return ProgressEvent{Kind: m.kind, Summary: truncateRunes(rest, progressSummaryRunes), At: time.Now()}, true
	}
	return ProgressEvent{}, false
}

// progressAccumulator coalesces ACP agent_message_chunk deltas (or Status
// partial snapshots) before classification so short streaming fragments still
// yield marker/keyword milestones.
//
// Marker events are keyed by byte offset in the buffer (not by growing summary),
// so streaming "[进"+"度] 已打开…" emits once. Keyword heuristics only run on
// completed lines (newline-terminated) to avoid mid-sentence false positives.
//
// Feed (Subscribe deltas) and FeedSnapshot (Status.partial) are dual inputs for
// the same cumulative stream. PmTurnRunner updates partial before fanout, so a
// ticker may snapshot the full text before Subscribe delivers the same delta.
// Both paths merge via adoptStream: only a strict prefix-extension advances
// buf, so Snapshot→Feed of the same content never double-buffers or re-emits.
type progressAccumulator struct {
	buf            string // merged cumulative stream used for emit
	feedBuf        string // concatenation of Feed deltas only (may lag snapshot)
	seen           map[string]bool
	keywordLineIdx int // next complete line index to scan for keyword heuristics
}

func newProgressAccumulator() *progressAccumulator {
	return &progressAccumulator{seen: map[string]bool{}}
}

// Feed appends an incremental agent_message delta and returns newly forwardable events.
func (a *progressAccumulator) Feed(delta string) []ProgressEvent {
	if a == nil || delta == "" {
		return nil
	}
	a.feedBuf += delta
	return a.adoptStream(a.feedBuf)
}

// FeedSnapshot merges an authoritative Status.partial and returns newly
// forwardable events. Stale/behind snapshots are ignored; only strict
// extensions of the merged buffer emit.
func (a *progressAccumulator) FeedSnapshot(partial string) []ProgressEvent {
	if a == nil {
		return nil
	}
	partial = strings.TrimRight(partial, "\x00")
	if partial == "" {
		return nil
	}
	return a.adoptStream(partial)
}

// adoptStream advances buf only when next strictly extends the current merged
// view (or replaces on true divergence). Equal / behind inputs are no-ops.
func (a *progressAccumulator) adoptStream(next string) []ProgressEvent {
	if next == a.buf {
		return nil
	}
	// Already have this content or more (Feed ahead of Snapshot, or duplicate).
	if strings.HasPrefix(a.buf, next) {
		return nil
	}
	// Normal growth: next is a prefix-extension of merged buf.
	if strings.HasPrefix(next, a.buf) {
		a.buf = next
		return a.emitNew()
	}
	// True divergence (rare encoding glitch). Prefer longer authoritative text
	// and reset emit state so marker offsets stay consistent with the new buf.
	if len(next) < len(a.buf) {
		return nil
	}
	a.buf = next
	a.seen = map[string]bool{}
	a.keywordLineIdx = 0
	return a.emitNew()
}

func (a *progressAccumulator) emitNew() []ProgressEvent {
	var out []ProgressEvent
	out = append(out, a.emitMarkers()...)
	out = append(out, a.emitKeywordLines()...)
	return out
}

func (a *progressAccumulator) emitMarkers() []ProgressEvent {
	var out []ProgressEvent
	for _, m := range progressMarkers {
		start := 0
		for {
			rel := strings.Index(a.buf[start:], m.prefix)
			if rel < 0 {
				break
			}
			abs := start + rel
			key := fmt.Sprintf("m|%d|%s", abs, m.kind)
			contentStart := abs + len(m.prefix)
			start = contentStart
			if a.seen[key] {
				continue
			}
			rest := a.buf[contentStart:]
			closed := false
			if nl := strings.IndexAny(rest, "\n\r"); nl >= 0 {
				rest = rest[:nl]
				closed = true
			}
			rest = strings.TrimSpace(rest)
			// Wait until the marker line closes or summary is substantive.
			if !closed && utf8.RuneCountInString(rest) < 4 {
				continue
			}
			a.seen[key] = true
			out = append(out, ProgressEvent{
				Kind: m.kind, Summary: truncateRunes(rest, progressSummaryRunes), At: time.Now(),
			})
		}
	}
	return out
}

func (a *progressAccumulator) emitKeywordLines() []ProgressEvent {
	parts := strings.Split(a.buf, "\n")
	nComplete := len(parts) - 1
	if nComplete < 0 {
		nComplete = 0
	}
	var out []ProgressEvent
	for i := a.keywordLineIdx; i < nComplete; i++ {
		line := strings.TrimSpace(parts[i])
		if line == "" {
			continue
		}
		// Markers already handled by emitMarkers.
		if _, ok := classifyProgressMarker(line); ok {
			continue
		}
		pe, ok := ClassifyProgressText(line)
		if !ok {
			continue
		}
		key := fmt.Sprintf("k|%d|%s|%s", i, pe.Kind, pe.Summary)
		if a.seen[key] {
			continue
		}
		a.seen[key] = true
		out = append(out, pe)
	}
	if nComplete > a.keywordLineIdx {
		a.keywordLineIdx = nComplete
	}
	return out
}

func FormatProgressText(ev ProgressEvent) string {
	sum := strings.TrimSpace(ev.Summary)
	if sum == "" {
		return ""
	}
	switch ev.Kind {
	case ProgressBlocker:
		return "阻塞：" + sum
	case ProgressConfirm:
		return "需确认：" + sum
	default:
		return "进度：" + sum
	}
}

// FormatProgressTextFor forks Feishu Demo copy ([进度] prefix) without
// changing existing QQ wording.
func FormatProgressTextFor(channelType string, ev ProgressEvent) string {
	if channelType != "feishu" {
		return FormatProgressText(ev)
	}
	sum := strings.TrimSpace(ev.Summary)
	if sum == "" {
		return ""
	}
	switch ev.Kind {
	case ProgressBlocker:
		return "[阻塞] " + sum
	case ProgressConfirm:
		return "[确认] " + sum
	default:
		return "[进度] " + sum
	}
}

func isProgressNoise(text string) bool {
	// Extremely short token-like fragments and tool dump markers.
	if utf8.RuneCountInString(text) < 4 {
		return true
	}
	lower := strings.ToLower(text)
	noise := []string{
		"tool_call", "tool call", "function_call", "token",
		"input_tokens", "output_tokens", "thinking", "reasoning_delta",
	}
	for _, n := range noise {
		if strings.Contains(lower, n) && utf8.RuneCountInString(text) < 40 {
			return true
		}
	}
	return false
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
