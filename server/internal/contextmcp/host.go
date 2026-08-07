// Package contextmcp implements the agent-scoped context-store MCP host.
// Conversation SoT is ChatThread / ChatMessage (no second dialogue store).
package contextmcp

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/platformmcp"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// Session binds a context-store token.
type Session struct {
	Token     string
	ProjectID string
	AgentName string
	ThreadID  string
	UserID    string
	Attached  *models.AttachedContext
}

// Host manages context-store sessions.
type Host struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	pm       *services.PmService
	audit    func(services.AuditRecord)
}

// NewHost builds a context-store host.
func NewHost(pm *services.PmService) *Host {
	return &Host{sessions: map[string]*Session{}, pm: pm}
}

// SetAuditRecorder wires project-scoped audit recording for context-store tools.
func (h *Host) SetAuditRecorder(fn func(services.AuditRecord)) {
	h.mu.Lock()
	h.audit = fn
	h.mu.Unlock()
}

// Register creates a session token.
func (h *Host) Register(projectID, agentName, threadID, userID string) string {
	tok := platformmcp.NewToken()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[tok] = &Session{
		Token: tok, ProjectID: projectID, AgentName: agentName,
		ThreadID: threadID, UserID: userID,
	}
	return tok
}

// Restore rebinds a token.
func (h *Host) Restore(tok, projectID, agentName, threadID, userID string) {
	if strings.TrimSpace(tok) == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[tok] = &Session{
		Token: tok, ProjectID: projectID, AgentName: agentName,
		ThreadID: threadID, UserID: userID,
	}
}

// Unregister drops a token.
func (h *Host) Unregister(token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, token)
}

// SetAttached updates attached context on a session.
func (h *Host) SetAttached(token string, attached *models.AttachedContext) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.sessions[token]; ok {
		s.Attached = attached
	}
}

// Authorize checks token for project.
func (h *Host) Authorize(projectID, token string) (*Session, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[token]
	if !ok || s.ProjectID != projectID {
		return nil, false
	}
	cp := *s
	return &cp, true
}

// ServeRPC handles one JSON-RPC message.
func (h *Host) ServeRPC(projectID, token string, body []byte) (int, []byte) {
	sess, ok := h.Authorize(projectID, token)
	if !ok {
		return platformmcp.Unauthorized()
	}
	var req platformmcp.RPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("context-store rpc parse error")
		return platformmcp.ParseError()
	}
	switch req.Method {
	case "initialize":
		ver := platformmcp.ProtocolVersion
		var ip struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(req.Params, &ip) == nil && ip.ProtocolVersion != "" {
			ver = ip.ProtocolVersion
		}
		return platformmcp.Ok(req, map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "context-store", "version": "1.0.0"},
		})
	case "notifications/initialized", "notifications/cancelled":
		return 202, nil
	case "ping":
		return platformmcp.Ok(req, map[string]any{})
	case "tools/list":
		return platformmcp.Ok(req, map[string]any{"tools": toolSchemas()})
	case "tools/call":
		var p platformmcp.ToolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return platformmcp.Fail(req, -32602, "invalid tools/call params")
		}
		result, isErr := h.callTool(sess, p.Name, p.Arguments)
		h.auditToolCall(sess, p.Name, p.Arguments, result, isErr)
		return platformmcp.Ok(req, platformmcp.ToolResult(result, isErr))
	default:
		if platformmcp.IsNotification(req) {
			return 202, nil
		}
		return platformmcp.Fail(req, -32601, "method not found: "+req.Method)
	}
}

func (h *Host) auditToolCall(sess *Session, tool string, args map[string]any, result any, isErr bool) {
	h.mu.RLock()
	fn := h.audit
	h.mu.RUnlock()
	if fn == nil || sess == nil || sess.ProjectID == "" || tool == "" {
		return
	}
	outcome := models.AuditOutcomeOK
	if isErr {
		outcome = models.AuditOutcomeFail
	}
	fn(services.AuditRecord{
		ProjectID:    sess.ProjectID,
		Actor:        services.ActorFromUsername(sess.UserID),
		Action:       models.AuditActionMCPCall,
		ResourceType: "mcp",
		ResourceID:   tool,
		Outcome:      outcome,
		Summary:      "mcp context-store/" + tool,
		Payload: map[string]any{
			"mcp": "context-store", "tool": tool,
			"arguments": args, "result": result, "isError": isErr,
		},
	})
}

func (h *Host) callTool(sess *Session, name string, args map[string]any) (any, bool) {
	if args == nil {
		args = map[string]any{}
	}
	switch name {
	case "list_conversations":
		threads, err := h.pm.ListConversationsForAgent(sess.ProjectID, sess.AgentName, sess.UserID)
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		ids := make([]string, len(threads))
		for i, t := range threads {
			ids[i] = t.ID
		}
		counts, _ := h.pm.CountMessagesByThreads(ids)
		rows := make([]map[string]any, 0, len(threads))
		for _, t := range threads {
			kind := t.Kind
			if kind == "" {
				kind = models.ChatThreadKindUser
			}
			rows = append(rows, map[string]any{
				"id": t.ID, "title": t.Title, "kind": kind,
				"updatedAt": t.UpdatedAt, "messageCount": counts[t.ID],
			})
		}
		return map[string]any{"conversations": rows, "count": len(rows)}, false
	case "get_messages":
		cid := platformmcp.StrArg(args, "conversationId")
		if cid == "" {
			cid = sess.ThreadID
		}
		if cid == "" {
			return map[string]any{"error": "conversationId required"}, true
		}
		if !h.threadVisible(sess, cid) {
			return map[string]any{"error": "conversation not found"}, true
		}
		msgs, total, err := h.pm.GetMessagesPage(cid, platformmcp.IntArg(args, "limit", 20), platformmcp.IntArg(args, "offset", 0))
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		rows := make([]any, 0, len(msgs))
		includeImages := platformmcp.BoolArg(args, "includeImages")
		for _, msg := range msgs {
			rows = append(rows, messageRow(msg, includeImages))
		}
		return map[string]any{"conversationId": cid, "messages": rows, "total": total, "count": len(rows)}, false
	case "get_attachment":
		mid := platformmcp.StrArg(args, "messageId")
		if mid == "" {
			return map[string]any{"error": "messageId required"}, true
		}
		cid := platformmcp.StrArg(args, "conversationId")
		if cid == "" {
			cid = sess.ThreadID
		}
		if cid == "" || !h.threadVisible(sess, cid) {
			return map[string]any{"error": "conversation not found"}, true
		}
		msg, err := h.pm.GetMessage(cid, mid)
		if err != nil {
			return map[string]any{"error": "message not found"}, true
		}
		idx := platformmcp.IntArg(args, "index", 0)
		if idx < 0 || idx >= len(msg.Images) {
			return map[string]any{
				"error": "attachment index out of range", "attachmentCount": len(msg.Images),
			}, true
		}
		img := msg.Images[idx]
		return map[string]any{
			"messageId": msg.ID, "index": idx, "name": img.Name,
			"mimeType": img.MimeType, "bytes": len(img.Data), "data": img.Data,
		}, false
	case "search_messages":
		hits, err := h.pm.SearchMessages(sess.ProjectID, sess.AgentName, sess.UserID,
			platformmcp.StrArg(args, "query"), platformmcp.IntArg(args, "limit", 20))
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"hits": hits, "count": len(hits)}, false
	case "get_current_conversation":
		if sess.ThreadID == "" {
			return map[string]any{"empty": true, "message": "当前会话未绑定 thread"}, false
		}
		t, err := h.pm.GetThreadByID(sess.ThreadID)
		if err != nil || !h.threadVisible(sess, t.ID) {
			return map[string]any{"empty": true, "message": "当前会话不可用"}, false
		}
		return map[string]any{
			"empty": false, "id": t.ID, "title": t.Title, "kind": t.Kind, "updatedAt": t.UpdatedAt,
		}, false
	case "get_attached_context":
		if sess.Attached == nil {
			return map[string]any{"empty": true, "message": "当前未附加 Run/工作流上下文"}, false
		}
		return map[string]any{"empty": false, "attached": sess.Attached}, false
	default:
		return map[string]any{"error": "unknown tool: " + name}, true
	}
}

func (h *Host) threadVisible(sess *Session, threadID string) bool {
	t, err := h.pm.GetThreadByID(threadID)
	if err != nil {
		return false
	}
	if t.ProjectID != sess.ProjectID || t.AgentName != sess.AgentName {
		return false
	}
	kind := t.Kind
	if kind == "" {
		kind = models.ChatThreadKindUser
	}
	if kind == models.ChatThreadKindCron {
		return true
	}
	if sess.UserID == "cron" {
		return false
	}
	return t.UserID == sess.UserID
}

// messageRow renders one stored message for the agent.
//
// Attachments are described, not shipped. A conversation with a handful of
// screenshots holds tens of megabytes of base64, and this tool used to return
// the rows verbatim — so reading twenty messages could bury the agent's whole
// context in image bytes it never asked for. The manifest says what is there;
// get_attachment fetches the one that matters.
func messageRow(msg models.ChatMessage, includeImages bool) any {
	if includeImages || len(msg.Images) == 0 {
		return msg
	}
	manifest := make([]map[string]any, 0, len(msg.Images))
	for i, img := range msg.Images {
		manifest = append(manifest, map[string]any{
			"index": i, "name": img.Name, "mimeType": img.MimeType, "bytes": len(img.Data),
		})
	}
	row := map[string]any{
		"id": msg.ID, "threadId": msg.ThreadID, "role": msg.Role, "content": msg.Content,
		"createdAt": msg.CreatedAt, "attachments": manifest,
	}
	if msg.Status != "" {
		row["status"] = msg.Status
	}
	if msg.Source != "" {
		row["source"] = msg.Source
	}
	if len(msg.Citations) > 0 {
		row["citations"] = msg.Citations
	}
	if msg.AttachedContext != nil {
		row["attachedContext"] = msg.AttachedContext
	}
	return row
}

func toolSchemas() []map[string]any {
	return []map[string]any{
		platformmcp.Tool("list_conversations", "列出当前用户可见的会话索引（本人用户会话 + Agent 定时会话；不含消息全文）。", nil),
		platformmcp.Tool("get_messages",
			"分页读取某会话消息（默认 limit=20）。默认只返回附件清单（文件名/类型/字节数/序号），"+
				"不返回附件内容；需要内容用 get_attachment 单取。", map[string]any{
				"conversationId": map[string]any{"type": "string", "description": "可选；默认当前会话"},
				"limit":          map[string]any{"type": "number"},
				"offset":         map[string]any{"type": "number"},
				"includeImages": map[string]any{
					"type":        "boolean",
					"description": "默认 false。设为 true 会把附件的完整 base64 一并返回，可能非常大。",
				},
			}),
		platformmcp.Tool("get_attachment",
			"按消息 id 和序号取回单个附件的内容（base64）。序号来自 get_messages 返回的附件清单。",
			map[string]any{
				"messageId":      map[string]any{"type": "string"},
				"index":          map[string]any{"type": "number", "description": "附件序号，从 0 开始"},
				"conversationId": map[string]any{"type": "string", "description": "可选；默认当前会话"},
			}),
		platformmcp.Tool("search_messages", "在当前用户可见的历史消息中关键词搜索（返回摘要）。", map[string]any{
			"query": map[string]any{"type": "string"},
			"limit": map[string]any{"type": "number"},
		}),
		platformmcp.Tool("get_current_conversation", "读取当前绑定会话的元数据。", nil),
		platformmcp.Tool("get_attached_context", "读取用户当次附加的 Run/工作流上下文。", nil),
	}
}
