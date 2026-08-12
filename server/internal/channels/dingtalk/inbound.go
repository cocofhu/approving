package dingtalk

import (
	"encoding/json"
	"strings"

	"github.com/cocofhu/approving/internal/channels"
)

const (
	unsupportedMediaHint = "不支持解析音频/视频/普通文件/卡片。请改发文本或图片。"
	oversizeHint         = "图片超过 10 MiB 上限，已跳过该附件并保留已收文本。"
	maxInboundImageBytes = 10 << 20
)

type inboundEvent struct {
	MessageID        string
	ConversationID   string
	ConversationType string
	MsgType          string
	Text             string
	Content          any
	SenderStaffID    string
	SenderID         string
	ChatbotUserID    string
	IsInAtList       bool
	AtUsers          []atUser
	SessionWebhook   string
	WebhookExpiredMs int64
}

type atUser struct {
	DingtalkID string
	StaffID    string
}

func sceneOf(conversationType string) (channels.Scene, bool) {
	switch strings.TrimSpace(conversationType) {
	case "1":
		return channels.SceneC2C, true
	case "2":
		return channels.SceneGroup, true
	default:
		return "", false
	}
}

// shouldAccept reports whether this inbound should start an Agent turn.
// c2c is always accepted; group only when the bot is @-mentioned.
func shouldAccept(ev inboundEvent) bool {
	scene, ok := sceneOf(ev.ConversationType)
	if !ok || strings.TrimSpace(ev.ConversationID) == "" {
		return false
	}
	if scene == channels.SceneC2C {
		return true
	}
	if ev.IsInAtList {
		return true
	}
	bot := strings.TrimSpace(ev.ChatbotUserID)
	for _, u := range ev.AtUsers {
		if bot != "" && (u.DingtalkID == bot || u.StaffID == bot) {
			return true
		}
	}
	return false
}

func senderUserID(ev inboundEvent) string {
	if s := strings.TrimSpace(ev.SenderStaffID); s != "" {
		return s
	}
	return strings.TrimSpace(ev.SenderID)
}

func extractText(ev inboundEvent) string {
	msgType := strings.ToLower(strings.TrimSpace(ev.MsgType))
	switch msgType {
	case "text":
		return strings.TrimSpace(stripAtTokens(ev.Text))
	case "richtext":
		return strings.TrimSpace(stripAtTokens(richTextPlain(ev.Content)))
	case "picture", "image":
		return ""
	default:
		if t := strings.TrimSpace(ev.Text); t != "" {
			return strings.TrimSpace(stripAtTokens(t))
		}
		return ""
	}
}

func stripAtTokens(s string) string {
	fields := strings.Fields(s)
	out := fields[:0]
	for _, f := range fields {
		if strings.HasPrefix(f, "@") {
			continue
		}
		out = append(out, f)
	}
	return strings.TrimSpace(strings.Join(out, " "))
}

func richTextPlain(content any) string {
	root := asMap(content)
	if root == nil {
		return ""
	}
	items, _ := root["richText"].([]any)
	if items == nil {
		items, _ = root["richtext"].([]any)
	}
	var parts []string
	for _, item := range items {
		m := asMap(item)
		if m == nil {
			continue
		}
		if t, _ := m["text"].(string); strings.TrimSpace(t) != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "")
}

func imageDownloadCodes(ev inboundEvent) []string {
	msgType := strings.ToLower(strings.TrimSpace(ev.MsgType))
	var codes []string
	switch msgType {
	case "picture", "image":
		codes = append(codes, downloadCodesFromMap(asMap(ev.Content))...)
	case "richtext":
		root := asMap(ev.Content)
		if root == nil {
			return nil
		}
		items, _ := root["richText"].([]any)
		if items == nil {
			items, _ = root["richtext"].([]any)
		}
		for _, item := range items {
			m := asMap(item)
			if m == nil {
				continue
			}
			typ, _ := m["type"].(string)
			if strings.EqualFold(typ, "picture") || strings.EqualFold(typ, "image") {
				codes = append(codes, downloadCodesFromMap(m)...)
			}
		}
	}
	return uniqueNonEmpty(codes)
}

func downloadCodesFromMap(m map[string]any) []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, k := range []string{"downloadCode", "pictureDownloadCode", "download_code"} {
		if v, _ := m[k].(string); strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func isUnsupportedMedia(msgType string) bool {
	switch strings.ToLower(strings.TrimSpace(msgType)) {
	case "audio", "voice", "file", "video", "media", "interactiveCard", "action_card", "actionCard", "richTextCard":
		return true
	default:
		return false
	}
}

func asMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(t), &m) == nil {
			return m
		}
	}
	return nil
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
