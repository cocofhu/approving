package channels

import (
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// CronResultKind classifies a deliverToChannel cron outcome for the unified
// Reply egress (priority + merge + templates).
type CronResultKind string

const (
	CronResultChanged   CronResultKind = "changed"
	CronResultUnchanged CronResultKind = "unchanged"
	CronResultFailed    CronResultKind = "failed"
)

const pushQueueDepth = 8

// CronPushItem is a timed push waiting for a conversation idle slot.
type CronPushItem struct {
	ProjectID string
	Scene     Scene
	Conv      string
	Category  string
	Kind      CronResultKind
	Text      string
	Enqueued  time.Time
}

type pushQueue struct {
	mu      sync.Mutex
	pending []CronPushItem
}

func (m *Manager) pushQueueFor(key string) *pushQueue {
	m.pushMu.Lock()
	defer m.pushMu.Unlock()
	q, ok := m.pushQueues[key]
	if !ok {
		q = &pushQueue{}
		m.pushQueues[key] = q
	}
	return q
}

// enqueuePushLocked merges / evicts per product rules. Caller must NOT hold q.mu.
func (m *Manager) enqueuePush(key string, item CronPushItem) {
	if item.Enqueued.IsZero() {
		item.Enqueued = time.Now()
	}
	q := m.pushQueueFor(key)
	q.mu.Lock()
	defer q.mu.Unlock()

	if item.Kind == CronResultUnchanged {
		for i, p := range q.pending {
			if p.Kind == CronResultUnchanged && samePushCategory(p.Category, item.Category) {
				q.pending[i] = item
				return
			}
		}
	}

	if len(q.pending) >= pushQueueDepth {
		if item.Kind == CronResultUnchanged {
			// Prefer replacing an existing unchanged; else drop with log.
			if idx := indexUnchanged(q.pending); idx >= 0 {
				q.pending[idx] = item
				return
			}
			log.Warn().
				Str("project", item.ProjectID).
				Str("category", item.Category).
				Str("kind", string(item.Kind)).
				Msg("cron push queue full; dropping compressible unchanged")
			return
		}
		// Evict unchanged to make room for changed/failed.
		for len(q.pending) >= pushQueueDepth {
			idx := indexUnchanged(q.pending)
			if idx < 0 {
				break
			}
			q.pending = append(q.pending[:idx], q.pending[idx+1:]...)
		}
		if len(q.pending) >= pushQueueDepth {
			log.Error().
				Str("project", item.ProjectID).
				Str("category", item.Category).
				Str("kind", string(item.Kind)).
				Str("text", truncateRunes(item.Text, 80)).
				Msg("cron push queue full of high-priority items; cannot enqueue changed/failed")
			return
		}
	}
	q.pending = append(q.pending, item)
}

func (m *Manager) takePushQueue(key string) []CronPushItem {
	q := m.pushQueueFor(key)
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return nil
	}
	out := append([]CronPushItem(nil), q.pending...)
	q.pending = nil
	// Priority within a flush: changed/failed before compressible unchanged.
	return sortPushByPriority(out)
}

func sortPushByPriority(items []CronPushItem) []CronPushItem {
	if len(items) < 2 {
		return items
	}
	high := make([]CronPushItem, 0, len(items))
	low := make([]CronPushItem, 0, len(items))
	for _, it := range items {
		if it.Kind == CronResultUnchanged {
			low = append(low, it)
		} else {
			high = append(high, it)
		}
	}
	return append(high, low...)
}

func indexUnchanged(items []CronPushItem) int {
	for i, p := range items {
		if p.Kind == CronResultUnchanged {
			return i
		}
	}
	return -1
}

func samePushCategory(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

// ClassifyCronResult maps assistant cron text into changed/unchanged/failed.
func ClassifyCronResult(text string) CronResultKind {
	t := strings.TrimSpace(text)
	if t == "" {
		return CronResultFailed
	}
	lower := strings.ToLower(t)
	switch {
	case containsAny(t, "失败", "错误：", "错误:") ||
		containsAny(lower, "failed", "error:", "failure"):
		return CronResultFailed
	case containsAny(t, "无变化", "无更新", "无新的", "暂无变化") ||
		containsAny(lower, "no change", "unchanged", "no updates", "nothing changed"):
		return CronResultUnchanged
	default:
		return CronResultChanged
	}
}

// FormatCronPush builds the short QQ body for a structured cron result.
// Unchanged uses a minimal template; changed/failed keep (truncated) body.
func FormatCronPush(category string, kind CronResultKind, body string) string {
	body = strings.TrimSpace(body)
	cat := strings.TrimSpace(category)
	switch kind {
	case CronResultUnchanged:
		return unchangedTemplate(cat)
	case CronResultFailed:
		if body == "" {
			return failLabel(cat) + "失败"
		}
		return failLabel(cat) + truncateRunes(body, 120)
	default:
		if body == "" {
			return catLabel(cat) + "有变化"
		}
		return body
	}
}

func unchangedTemplate(category string) string {
	switch cronCategoryClass(category) {
	case "pr":
		return "PR：无变化"
	case "daily":
		return "日报：无变化"
	default:
		if category == "" {
			return "无变化"
		}
		return truncateRunes(category, 20) + "：无变化"
	}
}

func failLabel(category string) string {
	switch cronCategoryClass(category) {
	case "pr":
		return "PR："
	case "daily":
		return "日报："
	default:
		if category == "" {
			return "失败："
		}
		return truncateRunes(category, 20) + "："
	}
}

func catLabel(category string) string {
	switch cronCategoryClass(category) {
	case "pr":
		return "PR："
	case "daily":
		return "日报："
	default:
		if category == "" {
			return ""
		}
		return truncateRunes(category, 20) + "："
	}
}

func cronCategoryClass(category string) string {
	c := strings.ToLower(category)
	switch {
	case strings.Contains(c, "pr") || strings.Contains(category, "拉取请求") || strings.Contains(category, "合并请求"):
		return "pr"
	case strings.Contains(c, "daily") || strings.Contains(category, "日报") || strings.Contains(category, "每日") || strings.Contains(category, "总结"):
		return "daily"
	default:
		return "other"
	}
}
