package channels

import (
	"encoding/json"
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

// ClassifyProgressFromACP extracts agent_message_chunk text from a raw ACP
// frame and classifies a single chunk. Prefer progressAccumulator for live
// streaming paths where chunks are short deltas.
func ClassifyProgressFromACP(raw json.RawMessage, extract func(json.RawMessage) string) (ProgressEvent, bool) {
	if extract == nil || len(raw) == 0 {
		return ProgressEvent{}, false
	}
	return ClassifyProgressText(extract(raw))
}

// progressAccumulator coalesces ACP agent_message_chunk deltas (or Status
// partial snapshots) before classification so short streaming fragments still
// yield marker/keyword milestones.
//
// Marker events are keyed by byte offset in the buffer (not by growing summary),
// so streaming "[进"+"度] 已打开…" emits once. Keyword heuristics only run on
// completed lines (newline-terminated) to avoid mid-sentence false positives.
type progressAccumulator struct {
	buf            string
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
	a.buf += delta
	return a.emitNew()
}

// FeedSnapshot replaces the buffer with an authoritative partial (e.g.
// PmTurnRunner.Status) and returns newly forwardable events.
func (a *progressAccumulator) FeedSnapshot(partial string) []ProgressEvent {
	if a == nil {
		return nil
	}
	partial = strings.TrimRight(partial, "\x00")
	if partial == "" || partial == a.buf {
		return nil
	}
	a.buf = partial
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
