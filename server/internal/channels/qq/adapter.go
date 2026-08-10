package qq

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/channels"

	"github.com/rs/zerolog/log"
)

const (
	maxOutboundImages = 4
	// maxInboundImages is a historical QQ inbound cap (not a new product limit).
	// Site chat does not add a count gate; QQ retains this existing safety cap.
	maxInboundImages = 4
)

// Adapter is the QQ channel adapter.
type Adapter struct {
	cfg     channels.AdapterConfig
	client  *client
	gateway *gateway
	intents int

	mu     sync.Mutex
	seen   map[string]time.Time // msg_id dedup (replay on reconnect)
	cancel context.CancelFunc
}

// New builds a QQ adapter from a resolved config. Registered as the "qq"
// factory in the channels Manager.
func New(cfg channels.AdapterConfig) (channels.Adapter, error) {
	if strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.AppSecret) == "" {
		return nil, fmt.Errorf("qq: appId 和 appSecret 必填")
	}
	apiBase := prodAPIBase
	if channels.BoolOpt(cfg.Config, "sandbox") {
		apiBase = sandboxAPIBase
	}
	if v := channels.StrOpt(cfg.Config, "apiBase"); v != "" {
		apiBase = v
	}
	intents := defaultIntents
	if n, ok := cfg.Config["intents"].(float64); ok && int(n) > 0 {
		intents = int(n)
	}
	// Native Markdown is on by default; set config "markdown": false to disable.
	markdown := true
	if v, ok := cfg.Config["markdown"].(bool); ok {
		markdown = v
	}
	return &Adapter{
		cfg:     cfg,
		client:  newClient(cfg.AppID, cfg.AppSecret, apiBase, markdown),
		intents: intents,
		seen:    map[string]time.Time{},
	}, nil
}

// Type implements channels.Adapter.
func (a *Adapter) Type() string { return "qq" }

// Start implements channels.Adapter (non-blocking; the gateway runs in a goroutine).
func (a *Adapter) Start(ctx context.Context, onInbound channels.InboundHandler) error {
	runCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	a.gateway = newGateway(a.client, a.intents, func(evtType string, data []byte) {
		a.handleEvent(runCtx, evtType, data, onInbound)
	})
	go a.gateway.run(runCtx)
	return nil
}

// Stop implements channels.Adapter.
func (a *Adapter) Stop() error {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	return nil
}

// Send implements channels.Adapter.
func (a *Adapter) Send(ctx context.Context, out channels.OutboundMessage) error {
	imgs := filterSendableImages(out.ImageURLs)
	switch out.Scene {
	case channels.SceneC2C:
		return a.client.sendC2C(ctx, out.ConversationID, out.ReplyToMessageID, out.Text, imgs)
	case channels.SceneGroup:
		return a.client.sendGroup(ctx, out.ConversationID, out.ReplyToMessageID, out.Text, imgs)
	case channels.SceneGuild:
		return a.client.sendGuild(ctx, out.ConversationID, out.ReplyToMessageID, out.Text, imgs)
	default:
		return fmt.Errorf("qq: 未知会话类型 %q", out.Scene)
	}
}

func (a *Adapter) handleEvent(ctx context.Context, evtType string, data []byte, onInbound channels.InboundHandler) {
	var scene channels.Scene
	switch evtType {
	case evtC2CMessage:
		scene = channels.SceneC2C
	case evtGroupAtMessage:
		scene = channels.SceneGroup
	case evtGuildAtMessage:
		scene = channels.SceneGuild
	default:
		return // unhandled event
	}
	var m inboundMessage
	if err := json.Unmarshal(data, &m); err != nil {
		log.Warn().Err(err).Str("evt", evtType).Msg("qq: decode message failed")
		return
	}
	if m.ID == "" || a.isDuplicate(m.ID) {
		return
	}

	var conversationID, userID string
	switch scene {
	case channels.SceneC2C:
		conversationID = m.Author.UserOpenID
		userID = m.Author.UserOpenID
	case channels.SceneGroup:
		conversationID = m.GroupOpenID
		userID = m.Author.MemberOpenID
	case channels.SceneGuild:
		conversationID = m.ChannelID
		userID = m.Author.ID
	}
	if conversationID == "" {
		log.Warn().Str("evt", evtType).Msg("qq: missing conversation id")
		return
	}

	in := channels.InboundMessage{
		Scene:          scene,
		ConversationID: conversationID,
		UserID:         userID,
		Text:           cleanContent(m.Content),
		MessageID:      m.ID,
		Timestamp:      time.Now(),
	}
	auth, err := a.client.authHeader(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("qq: auth header for inbound download failed; continuing without token")
		auth = ""
	}
	images, hint, oversized := collectInboundAttachments(ctx, m.Attachments, auth, downloadImage)
	in.Images = images
	in.ChannelHint = hint
	if len(oversized) > 0 {
		tip := fmt.Sprintf(
			"附件超过 %d MiB 上限，已拒绝：%s。请压缩后重试。",
			qqAttachMaxMiB, strings.Join(oversized, ", "),
		)
		// Pure oversize with nothing else: reply tip without starting an agent turn.
		if len(in.Images) == 0 && strings.TrimSpace(in.Text) == "" {
			if err := a.Send(ctx, channels.OutboundMessage{
				Scene:            scene,
				ConversationID:   conversationID,
				ReplyToMessageID: m.ID,
				Text:             tip,
			}); err != nil {
				log.Warn().Err(err).Msg("qq: oversized reject reply failed")
			}
			return
		}
		if in.Text != "" {
			in.Text = in.Text + "\n" + tip
		} else {
			in.Text = tip
		}
	}

	// Run the turn asynchronously so the gateway read loop keeps flowing; the
	// Manager serializes per conversation.
	go onInbound(ctx, in)
}

func (a *Adapter) isDuplicate(msgID string) bool {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.seen[msgID]; ok {
		return true
	}
	// Prune entries older than 10 minutes.
	if len(a.seen) > 2048 {
		for k, t := range a.seen {
			if now.Sub(t) > 10*time.Minute {
				delete(a.seen, k)
			}
		}
	}
	a.seen[msgID] = now
	return false
}

var mentionRe = regexp.MustCompile(`<@!?\d+>`)

func cleanContent(s string) string {
	s = mentionRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// filterSendableImages keeps only png/jpg URLs (QQ rich media image support)
// and caps the count.
func filterSendableImages(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		lu := strings.ToLower(u)
		base := lu
		if i := strings.IndexByte(base, '?'); i >= 0 {
			base = base[:i]
		}
		if strings.HasSuffix(base, ".png") || strings.HasSuffix(base, ".jpg") || strings.HasSuffix(base, ".jpeg") {
			out = append(out, u)
		}
		if len(out) >= maxOutboundImages {
			break
		}
	}
	return out
}
