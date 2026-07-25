package channels

import (
	"encoding/json"
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

// ClassifyProgressText maps assistant narration into one of the three
// forwardable progress kinds. Empty / tool-noise / heartbeat-like text is
// rejected (ok=false) so Reply does not spam QQ.
func ClassifyProgressText(text string) (ProgressEvent, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ProgressEvent{}, false
	}
	if isProgressNoise(text) {
		return ProgressEvent{}, false
	}

	// Explicit markers from Work (preferred).
	switch {
	case hasAnyPrefix(text, "[确认]", "【确认】", "[需确认]", "【需确认】"):
		return ProgressEvent{Kind: ProgressConfirm, Summary: truncateRunes(stripProgressMarker(text), progressSummaryRunes), At: time.Now()}, true
	case hasAnyPrefix(text, "[阻塞]", "【阻塞】", "[失败]", "【失败】"):
		return ProgressEvent{Kind: ProgressBlocker, Summary: truncateRunes(stripProgressMarker(text), progressSummaryRunes), At: time.Now()}, true
	case hasAnyPrefix(text, "[进度]", "【进度】", "[里程碑]", "【里程碑】"):
		return ProgressEvent{Kind: ProgressMilestone, Summary: truncateRunes(stripProgressMarker(text), progressSummaryRunes), At: time.Now()}, true
	}

	lower := strings.ToLower(text)
	switch {
	case containsAny(text, "请确认", "需要确认", "是否同意", "要不要", "请选择") ||
		containsAny(lower, "please confirm", "awaiting confirmation"):
		return ProgressEvent{Kind: ProgressConfirm, Summary: truncateRunes(text, progressSummaryRunes), At: time.Now()}, true
	case containsAny(text, "阻塞", "失败", "错误", "无法继续", "超时", "权限不足") ||
		containsAny(lower, "blocked", "failed", "error:", "timeout"):
		return ProgressEvent{Kind: ProgressBlocker, Summary: truncateRunes(text, progressSummaryRunes), At: time.Now()}, true
	case containsAny(text, "已完成", "完成：", "已提交", "已推送", "PR #", "pr #", "检查通过", "开始处理", "里程碑") ||
		containsAny(lower, "opened pr", "pushed", "merged", "milestone"):
		return ProgressEvent{Kind: ProgressMilestone, Summary: truncateRunes(text, progressSummaryRunes), At: time.Now()}, true
	default:
		return ProgressEvent{}, false
	}
}

// ClassifyProgressFromACP extracts agent_message_chunk text from a raw ACP
// frame and classifies it. Non-message / tool frames are suppressed.
func ClassifyProgressFromACP(raw json.RawMessage, extract func(json.RawMessage) string) (ProgressEvent, bool) {
	if extract == nil || len(raw) == 0 {
		return ProgressEvent{}, false
	}
	return ClassifyProgressText(extract(raw))
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

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
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

func stripProgressMarker(s string) string {
	markers := []string{"[确认]", "【确认】", "[需确认]", "【需确认】", "[阻塞]", "【阻塞】", "[失败]", "【失败】", "[进度]", "【进度】", "[里程碑]", "【里程碑】"}
	for _, m := range markers {
		if strings.HasPrefix(s, m) {
			return strings.TrimSpace(strings.TrimPrefix(s, m))
		}
	}
	return s
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
