// Package qq implements a QQ Bot (official OpenAPI v2) channel adapter over
// WebSocket. It supports C2C (private), group, and guild-channel messages,
// including inbound and outbound images. It implements channels.Adapter.
package qq

import (
	"encoding/json"
	"strings"
)

// Gateway opcodes (QQ bot WebSocket).
const (
	opDispatch     = 0
	opHeartbeat    = 1
	opIdentify     = 2
	opResume       = 6
	opReconnect    = 7
	opInvalidSess  = 9
	opHello        = 10
	opHeartbeatACK = 11
)

// Intent bits. Defaults cover group/C2C events and public guild @ messages.
const (
	intentGuilds        = 1 << 0
	intentPublicGuildAt = 1 << 30
	intentGroupAndC2C   = 1 << 25
)

const defaultIntents = intentGuilds | intentPublicGuildAt | intentGroupAndC2C

// Dispatch event type names we handle.
const (
	evtReady          = "READY"
	evtResumed        = "RESUMED"
	evtC2CMessage     = "C2C_MESSAGE_CREATE"
	evtGroupAtMessage = "GROUP_AT_MESSAGE_CREATE"
	evtGuildAtMessage = "AT_MESSAGE_CREATE"
)

// gatewayFrame is a raw WS frame envelope.
type gatewayFrame struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  int64           `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
	ID string          `json:"id,omitempty"`
}

type helloData struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

type identifyData struct {
	Token      string            `json:"token"`
	Intents    int               `json:"intents"`
	Shard      []int             `json:"shard"`
	Properties map[string]string `json:"properties"`
}

type resumeData struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
}

type readyData struct {
	SessionID string `json:"session_id"`
	User      struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"user"`
}

// author shapes across scenes.
type msgAuthor struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	UserOpenID   string `json:"user_openid"`
	MemberOpenID string `json:"member_openid"`
	UnionOpenID  string `json:"union_openid"`
}

type attachment struct {
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	Filename    string `json:"filename"`
}

// UnmarshalJSON accepts both QQ snake_case content_type and camelCase contentType.
func (a *attachment) UnmarshalJSON(b []byte) error {
	var raw struct {
		ContentType  string `json:"content_type"`
		ContentType2 string `json:"contentType"`
		URL          string `json:"url"`
		Filename     string `json:"filename"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	a.ContentType = strings.TrimSpace(raw.ContentType)
	if a.ContentType == "" {
		a.ContentType = strings.TrimSpace(raw.ContentType2)
	}
	a.URL = raw.URL
	a.Filename = raw.Filename
	return nil
}

// inboundMessage is the union of C2C/group/guild message payloads.
type inboundMessage struct {
	ID          string       `json:"id"`
	Content     string       `json:"content"`
	Timestamp   string       `json:"timestamp"`
	Author      msgAuthor    `json:"author"`
	GroupOpenID string       `json:"group_openid"`
	ChannelID   string       `json:"channel_id"`
	GuildID     string       `json:"guild_id"`
	Attachments []attachment `json:"attachments"`
}

// tokenResponse is the app access token endpoint reply.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   any    `json:"expires_in"` // string or number depending on env
}

// gatewayResponse is the /gateway endpoint reply.
type gatewayResponse struct {
	URL string `json:"url"`
}

// fileInfoResponse is the rich-media upload reply (C2C/group).
type fileInfoResponse struct {
	FileUUID string `json:"file_uuid"`
	FileInfo string `json:"file_info"`
	TTL      int    `json:"ttl"`
}
