// Package channels is a platform-agnostic abstraction for external IM channels
// (QQ today; Slack/Discord/Feishu later). Concrete transports implement the
// Adapter interface and normalize their protocol into InboundMessage /
// OutboundMessage.
//
// Reply/Work equivalent orchestration (验收口径，不强制物理双容器):
//
//   - Manager = Reply：唯一对 QQ 的发言出口（即时 ACK、排队 ACK、按需进度、
//     终态、定时推送队列与优先级）。
//   - ChannelBridge / PmTurnRunner / cron 沙箱 = Work：执行用户回合与定时任务，
//     只产出内部 ProgressEvent / TurnFinalReport / CronDelivery，不得旁路
//     adapter.Send 抢发 IM。ACK 在派发 Work 之前由 Manager 发出。
package channels

import (
	"context"
	"time"

	"github.com/cocofhu/approving/internal/sendable"
)

// Scene classifies a conversation surface. Adapters map their native scopes
// onto these so the bridge and thread naming stay platform-neutral.
type Scene string

const (
	// SceneC2C is a 1:1 private chat.
	SceneC2C Scene = "c2c"
	// SceneGroup is a group chat.
	SceneGroup Scene = "group"
	// SceneGuild is a guild/server channel (QQ 频道).
	SceneGuild Scene = "guild"
)

// Image is a normalized file/image attachment. Inbound attachments carry raw
// bytes (downloaded by the adapter); outbound images are referenced by URL.
// Filename is the original name when the channel provides one; empty means
// callers should use a distinguishable fallback before PromptImage.Name.
type Image struct {
	Data     []byte
	MimeType string
	URL      string
	Filename string
}

// SafetyNotice is an adapter-detected notice about an inbound message (e.g. a
// rejected oversized attachment). Adapters must not send it themselves: they
// attach it to the inbound message so it leaves through the Manager's single
// egress and shares its dedupe receipt, retries and audit.
type SafetyNotice struct {
	Text   string
	Reason string
	// DedupeKey keeps the notice idempotent across gateway reconnects that
	// replay the same inbound message.
	DedupeKey string
	// Only marks an inbound message that carries no user content: deliver the
	// notice and do not start a turn.
	Only bool
}

// InboundMessage is a normalized user message received from a channel.
type InboundMessage struct {
	Scene          Scene
	ConversationID string // openid / group_openid / channel_id
	UserID         string // author id (openid/union), for display/attribution
	Text           string
	Images         []Image
	MessageID      string // platform message id (passive reply + dedup)
	Timestamp      time.Time
	Raw            map[string]any
	Safety         *SafetyNotice
	// RecordedMessageID is this message's row in the canonical transcript. The
	// Manager writes the message before routing it, so the turn that eventually
	// runs refers to that row instead of storing the user's words a second time.
	RecordedMessageID string
	// EscalationReason is why the conversation model handed this turn to the
	// agent. Empty when no conversation model was involved.
	EscalationReason string
	// Dispatch is what the director decided about this turn. Empty when the
	// turn reached the agent without one (no conversation model, or a failure
	// that fell through to it).
	Dispatch *WorkDispatch
	// DecisionSampleID links this turn back to the recorded routing decision,
	// so the work layer's eventual conclusion lands on the choice that produced
	// it rather than in a separate row nothing joins.
	DecisionSampleID string
	// TraceID joins Live routing, sandbox work, MCP, synthesis, and outbound
	// delivery for this one user message. Minted at inbound entry.
	TraceID string
}

// WorkDispatch is the director's delegation: what the agent is being asked to
// do, and the pointers it needs to find the rest for itself.
//
// It deliberately carries no conversation replay. The agent reads history
// through context-store when it needs it, so a long-running conversation costs
// a prompt that stays the same size instead of one that grows with every turn.
type WorkDispatch struct {
	// Brief is one sentence stating what the user wants, written for the agent.
	Brief string
	// Difficulty is the director's estimate: DifficultyLookup for a question
	// that needs a real check, DifficultyHeavy for work that outlives a chat
	// turn.
	Difficulty Difficulty
	// TaskID / ShortTitle identify this work in the conversation's ledger.
	TaskID     string
	ShortTitle string
	// Attachments are files already in this conversation that the agent may
	// need. Pointers only: the bytes stay in context-store until it asks.
	Attachments []AttachmentRef
}

// Difficulty is how much work the director thinks a turn needs. It decides
// whether the user is answered now and told later, or simply waited with.
type Difficulty string

const (
	// DifficultyLookup is a real check the user can reasonably wait a moment
	// for. The director may hold the turn briefly and answer once.
	DifficultyLookup Difficulty = "lookup"
	// DifficultyHeavy is work measured in minutes. The user is answered first
	// and the work is delegated behind the conversation.
	DifficultyHeavy Difficulty = "heavy"
)

// AttachmentRef points at one file already stored in the conversation.
//
// It exists so an attachment can be named without being carried: the bytes of
// an image sent five turns ago do not belong in every later prompt, but
// "there is a screenshot, here is where it lives" does.
type AttachmentRef struct {
	MessageID string
	Index     int
	Name      string
	MimeType  string
	Bytes     int
	// Role is who sent it ("user" / "assistant"), so the agent can tell a file
	// it was given from one it produced.
	Role string
}

// OutboundMessage is a normalized reply/push to a channel conversation.
type OutboundMessage struct {
	Scene            Scene
	ConversationID   string
	ReplyToMessageID string // passive reply id (empty → active push)
	Text             string
	ImageURLs        []string // shareable image URLs to attach
	Envelope         sendable.DeliveryEnvelope
}

// InboundHandler is invoked by an adapter for each received message.
type InboundHandler func(ctx context.Context, in InboundMessage)

// SendResult is what a channel reports back about a delivered message.
// MessageID is the channel's own message identifier and must only be set from a
// real transport response — it is never derived from an inbound message id, a
// dedupe key, or any other local value. Channels without such a response leave
// it empty.
type SendResult struct {
	MessageID string
}

// Adapter is a transport for one external channel account. Implementations are
// created per ChannelConfig and are responsible only for receiving/sending;
// PM orchestration lives in ChannelBridge.
type Adapter interface {
	// Type returns the channel type (e.g. "qq").
	Type() string
	// Start connects and begins delivering inbound messages to onInbound until
	// the ctx is cancelled or Stop is called. It should return once the receive
	// loop is running (non-blocking) or on a fatal startup error.
	Start(ctx context.Context, onInbound InboundHandler) error
	// Send delivers an outbound message and reports the channel's message id
	// when the transport returns one.
	Send(ctx context.Context, out OutboundMessage) (SendResult, error)
	// Stop tears down the connection and releases resources.
	Stop() error
}

// AdapterConfig is the resolved, decrypted configuration handed to a factory.
type AdapterConfig struct {
	ID        string
	Type      string
	Name      string
	ProjectID string
	AppID     string
	AppSecret string // decrypted
	Config    map[string]any
}

// AdapterFactory builds an Adapter for a resolved config. Registered in the
// Manager (from main.go) so the channels package never imports concrete
// adapters, keeping the dependency graph acyclic.
type AdapterFactory func(cfg AdapterConfig) (Adapter, error)

// StrOpt reads a string option from an adapter Config map.
func StrOpt(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	if v, ok := cfg[key].(string); ok {
		return v
	}
	return ""
}

// BoolOpt reads a bool option from an adapter Config map.
func BoolOpt(cfg map[string]any, key string) bool {
	if cfg == nil {
		return false
	}
	if v, ok := cfg[key].(bool); ok {
		return v
	}
	return false
}

// SessionCaps are per-channel write gates for platform MCP tools on external
// Channel turns. Missing keys default to false (compatible with existing
// channels that never set them).
type SessionCaps struct {
	AllowMemoryWrite    bool
	AllowSchedulerWrite bool
}

// SessionCapsFromConfig parses allowMemoryWrite / allowSchedulerWrite from a
// ChannelConfig.Config JSON map. BoolOpt defaults missing keys to false.
func SessionCapsFromConfig(cfg map[string]any) SessionCaps {
	return SessionCaps{
		AllowMemoryWrite:    BoolOpt(cfg, "allowMemoryWrite"),
		AllowSchedulerWrite: BoolOpt(cfg, "allowSchedulerWrite"),
	}
}
