package feishu

import (
	"encoding/json"
	"strings"

	"github.com/cocofhu/approving/internal/channels"
)

const (
	unsupportedMediaHint = "不支持解析音频/视频/普通文件。请改发文本或图片。"
	oversizeHint         = "图片超过 10 MiB 上限，已跳过该附件并保留已收文本。"
	maxInboundImageBytes = 10 << 20
)

type inboundEvent struct {
	MessageID   string
	ChatID      string
	ChatType    string
	MessageType string
	Content     string
	Mentions    []inboundMention
	SenderID    string
}

type inboundMention struct {
	Key    string
	OpenID string
	Name   string
}

func sceneOf(chatType string) (channels.Scene, bool) {
	switch strings.ToLower(strings.TrimSpace(chatType)) {
	case "p2p":
		return channels.SceneC2C, true
	case "group", "topic_group":
		return channels.SceneGroup, true
	default:
		return "", false
	}
}

// shouldAccept reports whether this inbound should start an Agent turn.
// p2p is always accepted; group/topic_group only when the bot is @-mentioned
// and the mention is not solely @all.
func shouldAccept(ev inboundEvent, botOpenID string) bool {
	scene, ok := sceneOf(ev.ChatType)
	if !ok || strings.TrimSpace(ev.ChatID) == "" {
		return false
	}
	if scene == channels.SceneC2C {
		return true
	}
	return mentionedBot(ev.Mentions, botOpenID)
}

func mentionedBot(mentions []inboundMention, botOpenID string) bool {
	botOpenID = strings.TrimSpace(botOpenID)
	for _, m := range mentions {
		if isAtAll(m) {
			continue
		}
		if botOpenID != "" {
			if strings.TrimSpace(m.OpenID) == botOpenID {
				return true
			}
			continue
		}
		// Bot open id unknown: any non-@all mention counts (group_at subscription).
		return true
	}
	return false
}

func isAtAll(m inboundMention) bool {
	key := strings.ToLower(strings.TrimSpace(m.Key))
	name := strings.TrimSpace(m.Name)
	return key == "@_all" || name == "所有人" || strings.EqualFold(name, "all")
}

func extractText(msgType, content string) string {
	msgType = strings.ToLower(strings.TrimSpace(msgType))
	raw := strings.TrimSpace(content)
	if raw == "" {
		return ""
	}
	switch msgType {
	case "text":
		var body struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(raw), &body) == nil {
			return strings.TrimSpace(stripMentionTokens(body.Text))
		}
		return stripMentionTokens(raw)
	case "post":
		return strings.TrimSpace(postPlainText(raw))
	case "image":
		return ""
	default:
		return ""
	}
}

func stripMentionTokens(s string) string {
	// Feishu text mentions look like @_user_1 / @_all
	fields := strings.Fields(s)
	out := fields[:0]
	for _, f := range fields {
		if strings.HasPrefix(f, "@_") {
			continue
		}
		out = append(out, f)
	}
	return strings.TrimSpace(strings.Join(out, " "))
}

func imageKeys(msgType, content string) []string {
	msgType = strings.ToLower(strings.TrimSpace(msgType))
	raw := strings.TrimSpace(content)
	if raw == "" {
		return nil
	}
	if msgType == "image" {
		var body struct {
			ImageKey string `json:"image_key"`
		}
		if json.Unmarshal([]byte(raw), &body) == nil && strings.TrimSpace(body.ImageKey) != "" {
			return []string{body.ImageKey}
		}
		return nil
	}
	if msgType == "post" {
		return postImageKeys(raw)
	}
	return nil
}

func isUnsupportedMedia(msgType string) bool {
	switch strings.ToLower(strings.TrimSpace(msgType)) {
	case "file", "audio", "media", "sticker":
		return true
	default:
		return false
	}
}

func postPlainText(raw string) string {
	var root map[string]any
	if json.Unmarshal([]byte(raw), &root) != nil {
		return ""
	}
	var parts []string
	for _, loc := range []string{"zh_cn", "zh_hk", "zh_tw", "en_us", "ja_jp"} {
		block, _ := root[loc].(map[string]any)
		if block == nil {
			continue
		}
		if title, _ := block["title"].(string); strings.TrimSpace(title) != "" {
			parts = append(parts, strings.TrimSpace(title))
		}
		content, _ := block["content"].([]any)
		for _, line := range content {
			row, _ := line.([]any)
			var lineParts []string
			for _, cell := range row {
				m, _ := cell.(map[string]any)
				if m == nil {
					continue
				}
				if t, _ := m["text"].(string); strings.TrimSpace(t) != "" {
					lineParts = append(lineParts, t)
				}
			}
			if s := strings.TrimSpace(strings.Join(lineParts, "")); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			break
		}
	}
	return strings.Join(parts, "\n")
}

func postImageKeys(raw string) []string {
	var root map[string]any
	if json.Unmarshal([]byte(raw), &root) != nil {
		return nil
	}
	var keys []string
	walk := func(v any) {
		m, _ := v.(map[string]any)
		if m == nil {
			return
		}
		tag, _ := m["tag"].(string)
		if tag != "img" && tag != "image" {
			return
		}
		if k, _ := m["image_key"].(string); strings.TrimSpace(k) != "" {
			keys = append(keys, k)
		}
	}
	for _, loc := range root {
		block, _ := loc.(map[string]any)
		if block == nil {
			continue
		}
		content, _ := block["content"].([]any)
		for _, line := range content {
			row, _ := line.([]any)
			for _, cell := range row {
				walk(cell)
			}
		}
	}
	return keys
}
