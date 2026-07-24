// 有界 FIFO + 单 worker 串行处理（promptQueue + pumpPromptQueue）。
package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"backend/internal/acp"
	"backend/internal/correl"
	"backend/internal/provider"
)

func (b *Bridge) clearPromptQueue() {
	b.queueMu.Lock()
	b.promptQueue = nil
	b.queueMu.Unlock()
}

func (b *Bridge) BroadcastQueueState() {
	m := b.queueSnapshot()
	m["op"] = "queue_state"
	b.Broadcast(m)
}

// pumpPromptQueue 在「当前无 activeTurn」时从 FIFO 取一条并启动执行。
// 须在启动 goroutine **之前**同步占用 activeTurn，否则连续 Chat 会在首条 runPrompt 尚未设 activeTurn 时再次入泵，导致多条 session/prompt 并发（顺序错乱）；须保证每会话单 worker 串行消费。
func (b *Bridge) pumpPromptQueue() {
	b.turnMu.Lock()
	if b.activeTurn != nil {
		b.turnMu.Unlock()
		b.BroadcastQueueState()
		return
	}

	b.queueMu.Lock()
	if len(b.promptQueue) == 0 {
		b.queueMu.Unlock()
		b.turnMu.Unlock()
		b.BroadcastQueueState()
		return
	}
	item := b.promptQueue[0]
	b.promptQueue = b.promptQueue[1:]
	b.queueMu.Unlock()

	b.mu.Lock()
	p := b.sess
	ctx := b.agentCtx
	b.mu.Unlock()
	if p == nil {
		b.queueMu.Lock()
		b.promptQueue = append([]queuedPrompt{item}, b.promptQueue...)
		b.queueMu.Unlock()
		b.turnMu.Unlock()
		b.BroadcastQueueState()
		return
	}

	oid := item.OpID
	if oid == "" {
		oid = correl.ID()
	}
	turnCtx, cancelTurn := context.WithCancel(ctx)
	th := &promptTurn{cancel: cancelTurn, opID: oid, userText: item.Text, imageCount: len(item.Images)}
	b.activeTurn = th
	b.turnMu.Unlock()

	log.Printf("prompt %s oid=%s: 开始 session/prompt textLen=%d", b.AgentLogPrefix(), oid, len(item.Text))

	b.BroadcastQueueState()
	// 每轮 prompt 边界：避免上一轮未收到 prompt_done 时前端仍把新正文流式接到同一助手块（见 chat_view appendStreamAgent）
	promptBeginData := map[string]any{
		"type":       "prompt_begin",
		"sessionId":  p.SessionID(),
		"opId":       oid,
		"text":       item.Text,
		"promptText": item.Text,
	}
	// 图片 → data URL 供前端重放气泡预览；非图片只记文件名（附件已统一落盘，不嵌入 prompt）。
	if len(item.Images) > 0 {
		urls := make([]string, 0, len(item.Images))
		fileNames := make([]string, 0, len(item.Images))
		for _, img := range item.Images {
			if img.Data == "" {
				continue
			}
			mime := img.MimeType
			if mime == "" {
				mime = "application/octet-stream"
			}
			if strings.HasPrefix(strings.ToLower(mime), "image/") {
				urls = append(urls, "data:"+mime+";base64,"+img.Data)
				continue
			}
			name := strings.TrimSpace(img.Name)
			if name == "" {
				name = "attachment" + provider.ExtForMIME(mime)
			}
			fileNames = append(fileNames, name)
		}
		if len(urls) > 0 {
			promptBeginData["imageURLs"] = urls
		}
		if len(fileNames) > 0 {
			promptBeginData["fileNames"] = fileNames
		}
	}
	b.Broadcast(map[string]any{
		"op":   "event",
		"data": promptBeginData,
	})

	go b.executePrompt(p, turnCtx, item, th, cancelTurn)
}

func (b *Bridge) executePrompt(p provider.Session, turnCtx context.Context, item queuedPrompt, th *promptTurn, cancelTurn context.CancelFunc) {
	oid := th.opID
	defer func() {
		b.turnMu.Lock()
		if b.activeTurn == th {
			b.activeTurn = nil
		}
		b.turnMu.Unlock()
		cancelTurn()
		b.recordUserTurnDone(item)
		b.pumpPromptQueue()
	}()

	// 统一附件策略：图片/文件一律落盘到 /tmp，prompt 里只引用绝对路径。
	// 这样 cursor / gemini / ACP 等都走同一套「读本地文件」能力，不再依赖各 CLI 的原生 image block。
	text := item.Text
	images := item.Images
	if len(images) > 0 {
		dir, paths, merr := provider.MaterializeAttachments(images)
		if merr != nil {
			log.Printf("prompt %s oid=%s: 附件落盘失败: %v", b.AgentLogPrefix(), oid, merr)
			b.Broadcast(map[string]any{"op": "error", "message": "附件保存失败: " + merr.Error()})
			b.Broadcast(map[string]any{
				"op": "event",
				"data": map[string]any{
					"type":       "prompt_done",
					"sessionId":  p.SessionID(),
					"stopReason": "failed",
				},
			})
			return
		}
		defer os.RemoveAll(dir)
		log.Printf("prompt %s oid=%s: 已将 %d 个附件落到 %s", b.AgentLogPrefix(), oid, len(paths), dir)
		text = provider.AppendAttachmentRefs(text, paths)
		images = nil
	}

	res, err := p.Prompt(turnCtx, text, images)
	stopReason := res.StopReason
	if err == nil {
		if sr := strings.ToLower(strings.TrimSpace(stopReason)); sr == "refusal" {
			log.Printf("prompt %s oid=%s: 轮次结束 stopReason=refusal（多为鉴权失败：检查 CODEBUDDY_API_KEY / 区域 ACP_CODEBUDDY_REGION，或执行 codebuddy login）", b.AgentLogPrefix(), oid)
			b.Broadcast(map[string]any{
				"op": "error",
				"message": "Agent 拒绝本轮请求（refusal）。CodeBuddy 多为 API Key 无效或区域不匹配：" +
					"请确认 CODEBUDDY_API_KEY（iOA 站：https://tencent.sso.copilot.tencent.com/profile/keys），" +
					"并设置 ACP_CODEBUDDY_REGION=ioa；也可 unset CODEBUDDY_API_KEY 后执行 codebuddy login。",
			})
		}
		return
	}
	log.Printf("prompt %s oid=%s: 结束 err=%v", b.AgentLogPrefix(), oid, err)
	if errors.Is(err, context.Canceled) {
		// Session already emitted prompt_done (e.g. oneshot); only synthesize
		// for transports that return cancel without a stop reason (ACP Call).
		if stopReason == "" && th.fromUserStop.Load() {
			b.Broadcast(map[string]any{
				"op": "event",
				"data": map[string]any{
					"type":       "prompt_done",
					"sessionId":  p.SessionID(),
					"stopReason": "cancelled",
				},
			})
		}
		return
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "connection closed") || strings.Contains(low, "broken pipe") ||
		strings.Contains(low, "write |1:") || errors.Is(err, syscall.EPIPE) {
		// Pipe death usually means the agent process exited; surface to the UI
		// so the client is not left waiting on an open turn.
		b.Broadcast(map[string]any{"op": "error", "message": acp.UserFacingAny(err)})
		return
	}
	b.Broadcast(map[string]any{"op": "error", "message": acp.UserFacingAny(err)})
}

func (b *Bridge) CancelPrompt() {
	b.mu.Lock()
	p := b.sess
	b.mu.Unlock()

	var stopTurn context.CancelFunc
	b.turnMu.Lock()
	if t := b.activeTurn; t != nil {
		t.fromUserStop.Store(true)
		stopTurn = t.cancel
	}
	b.turnMu.Unlock()

	// 先通知 Agent（session/cancel + $/cancel_request），再取消本地 wait，避免只断客户端、子进程仍在跑
	if p != nil {
		if err := p.Cancel(); err != nil {
			log.Printf("ws→acp %s: Stop 通知 Agent 失败: %v", b.AgentLogPrefix(), err)
		}
	}
	if stopTurn != nil {
		stopTurn()
	}
	log.Printf("ws→acp %s: CancelPrompt 已执行", b.AgentLogPrefix())
	// 取消当前轮次并丢弃尚未开始处理的排队消息
	b.clearPromptQueue()
	b.BroadcastQueueState()
}

// ChatWithOpID 与 WebSocket 单次 chat 帧共用 oid（与 InMessage.ID 一致）；action 默认 chat。
// 无 oid 时自动生成（非 WS 入口时仍可有可搜日志键）。
func (b *Bridge) ChatWithOpID(text, opID, action string, images []PromptImage) error {
	t := strings.TrimSpace(text)
	if t == "" && len(images) == 0 {
		return errors.New("empty message")
	}
	if opID == "" {
		opID = correl.ID()
	}
	action = strings.TrimSpace(action)
	if action == "" {
		action = "chat"
	}
	b.mu.Lock()
	ok := b.sess != nil
	b.mu.Unlock()
	if !ok {
		return errors.New("not connected; send connect first")
	}

	b.enqueueMu.Lock()
	defer b.enqueueMu.Unlock()

	b.queueMu.Lock()
	if len(b.promptQueue) >= MaxPromptQueueItems {
		b.queueMu.Unlock()
		return fmt.Errorf("消息队列已满（最多 %d 条），请等待当前回复结束后再发", MaxPromptQueueItems)
	}
	b.promptQueue = append(b.promptQueue, queuedPrompt{Text: t, OpID: opID, Action: action, Images: images})
	b.queueMu.Unlock()
	b.BroadcastQueueState()
	b.pumpPromptQueue()
	return nil
}
