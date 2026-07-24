package qqbot

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"
)

func (s *Service) handleBridgeEvent(raw json.RawMessage) {
	var ev map[string]any
	if err := json.Unmarshal(raw, &ev); err != nil {
		return
	}
	t, _ := ev["type"].(string)
	switch t {
	case "prompt_begin":
		opID := firstString(ev["opId"], ev["opID"], ev["id"])
		if opID == "" {
			return
		}
		s.mu.Lock()
		if tr := s.pending[opID]; tr != nil {
			s.current = tr
			delete(s.pending, opID)
		}
		s.mu.Unlock()
	case "session_update":
		text := agentMessageText(ev)
		if text == "" {
			return
		}
		s.mu.Lock()
		if s.current != nil {
			_, _ = s.current.buf.WriteString(text)
		}
		s.mu.Unlock()
	case "prompt_done":
		s.mu.Lock()
		tr := s.current
		s.current = nil
		if tr != nil {
			tr.finishedAt = time.Now()
		}
		s.mu.Unlock()
		if tr == nil {
			return
		}
		msg := strings.TrimSpace(tr.buf.String())
		if msg == "" {
			msg = "（本轮没有可发送的文本回复）"
		}
		go func() {
			if err := s.SendMarkdown(context.Background(), tr.target, msg); err != nil {
				log.Printf("qqbot: 发送回复失败 opID=%s: %v", tr.opID, err)
			}
		}()
	}
}

// ─── 消息发送 (REST API) ───

func agentMessageText(ev map[string]any) string {
	upd, ok := ev["update"]
	if !ok || upd == nil {
		return ""
	}
	if s, ok := upd.(string); ok {
		var parsed any
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			upd = parsed
		}
	}
	m, ok := upd.(map[string]any)
	if !ok {
		return ""
	}
	merged := mergeSessionUpdateEnvelope(m)
	kind := normalizeKind(merged)
	if kind != "agent_message_chunk" {
		return ""
	}
	return extractText(merged["content"])
}

func mergeSessionUpdateEnvelope(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+4)
	for k, v := range in {
		out[k] = v
	}
	if nested, ok := out["sessionUpdate"].(map[string]any); ok {
		for k, v := range nested {
			out[k] = v
		}
		delete(out, "sessionUpdate")
	}
	if nested, ok := out["session_update"].(map[string]any); ok {
		for k, v := range nested {
			out[k] = v
		}
		delete(out, "session_update")
	}
	return out
}

func normalizeKind(m map[string]any) string {
	raw := firstString(m["sessionUpdate"], m["session_update"])
	if raw == "" {
		if nested, ok := m["sessionUpdate"].(map[string]any); ok {
			raw = firstString(nested["type"], nested["kind"], nested["sessionUpdate"])
		}
	}
	if raw == "" {
		raw = firstString(m["type"], m["kind"])
	}
	return toSnakeLower(raw)
}

func toSnakeLower(s string) string {
	var b strings.Builder
	var prevLower bool
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r == '-':
			b.WriteByte('_')
			prevLower = false
		case r >= 'A' && r <= 'Z':
			if prevLower {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
			prevLower = false
		default:
			b.WriteRune(r)
			prevLower = (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		}
	}
	return strings.ToLower(b.String())
}

func extractText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []any:
		var b strings.Builder
		for _, item := range x {
			b.WriteString(extractText(item))
		}
		return b.String()
	case map[string]any:
		if s := firstString(x["text"]); s != "" {
			return s
		}
		if x["type"] == "text" {
			return firstString(x["content"])
		}
		if parts, ok := x["parts"].([]any); ok {
			return extractText(parts)
		}
	}
	return ""
}
