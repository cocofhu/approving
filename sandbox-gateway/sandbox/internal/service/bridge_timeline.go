package service

import (
	"strings"
	"sync"
)

// 本 ACP 子进程 + sessionId 生命周期内的用户轮次快照（内存有序字段；刷新时下发供前端重建 user/agent 交替）。
// 不做条数截断，与会话级 eventLog 一致保留完整用户轮次序。

// 仅「已结束」的 session/prompt；进行中与排队由 activeTurn + promptQueue 推导。
type userDoneBuffer struct {
	mu   sync.Mutex
	done []PromptQueueEntry
}

func (r *userDoneBuffer) clear() {
	r.mu.Lock()
	r.done = nil
	r.mu.Unlock()
}

func (r *userDoneBuffer) appendDone(e PromptQueueEntry) {
	r.mu.Lock()
	r.done = append(r.done, e)
	r.mu.Unlock()
}

func (r *userDoneBuffer) snapshotDone() []PromptQueueEntry {
	r.mu.Lock()
	cp := append([]PromptQueueEntry(nil), r.done...)
	r.mu.Unlock()
	return cp
}

// UserTimelineForClient 严格 FIFO：已完成 → 进行中(至多 1) → 排队；供 op:connected 与 HTTP 快照。
func (b *Bridge) UserTimelineForClient() []map[string]any {
	var out []map[string]any
	for _, e := range b.userDoneBuf.snapshotDone() {
		m := timelineEntryMap(e, "done")
		out = append(out, m)
	}
	b.turnMu.Lock()
	t := b.activeTurn
	var runOp, runText string
	var runImageCount int
	if t != nil {
		runOp = t.opID
		runText = t.userText
		runImageCount = t.imageCount
	}
	b.turnMu.Unlock()
	if strings.TrimSpace(runText) != "" || runImageCount > 0 {
		e := promptQueueEntryFromQueued(queuedPrompt{Text: strings.TrimSpace(runText), OpID: runOp, Action: "chat"})
		e.ImageCount = runImageCount
		out = append(out, timelineEntryMap(e, "running"))
	}
	b.queueMu.Lock()
	q := append([]queuedPrompt(nil), b.promptQueue...)
	b.queueMu.Unlock()
	for _, p := range q {
		out = append(out, timelineEntryMap(promptQueueEntryFromQueued(p), "queued"))
	}
	return out
}

func timelineEntryMap(e PromptQueueEntry, phase string) map[string]any {
	m := map[string]any{
		"id":      e.ID,
		"opId":    e.OpID,
		"action":  e.Action,
		"content": e.Content,
		"text":    e.Text,
		"phase":   phase,
	}
	if e.ImageCount > 0 {
		m["imageCount"] = e.ImageCount
	}
	return m
}

func (b *Bridge) recordUserTurnDone(item queuedPrompt) {
	b.userDoneBuf.appendDone(promptQueueEntryFromQueued(item))
}

func (b *Bridge) clearUserTurnHistory() {
	b.userDoneBuf.clear()
}
