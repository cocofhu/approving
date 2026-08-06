package qq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/sendable"

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

// Send implements channels.Adapter. The returned SendResult carries the real QQ
// message id when the API reported one; it is left empty rather than filled with
// a locally derived value.
func (a *Adapter) Send(ctx context.Context, out channels.OutboundMessage) (channels.SendResult, error) {
	// Second gate: the Manager already evaluated the delivery policy, this
	// fail-closed check keeps a bypassing caller from reaching QQ.
	if reason := sendable.GateReason(out.Envelope, sendable.ChannelQQ,
		out.Text+"\n"+strings.Join(out.ImageURLs, "\n")); reason != "" {
		return channels.SendResult{}, fmt.Errorf("qq: outbound suppressed by delivery policy: %s", reason)
	}
	imgs := filterSendableImages(out.ImageURLs)
	var messageID string
	var err error
	switch out.Scene {
	case channels.SceneC2C:
		messageID, err = a.client.sendC2C(ctx, out.ConversationID, out.ReplyToMessageID, out.Text, imgs)
	case channels.SceneGroup:
		messageID, err = a.client.sendGroup(ctx, out.ConversationID, out.ReplyToMessageID, out.Text, imgs)
	case channels.SceneGuild:
		messageID, err = a.client.sendGuild(ctx, out.ConversationID, out.ReplyToMessageID, out.Text, imgs)
	default:
		return channels.SendResult{}, fmt.Errorf("qq: 未知会话类型 %q", out.Scene)
	}
	if err != nil {
		return channels.SendResult{}, err
	}
	return channels.SendResult{MessageID: messageID}, nil
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
	var oversized []string
	for _, att := range m.Attachments {
		// Accept any type (PDF/zip/etc.); only the historical count cap applies.
		if len(in.Images) >= maxInboundImages {
			break
		}
		img, err := downloadImage(ctx, att)
		if err != nil {
			if errors.Is(err, errInboundTooLarge) {
				name := strings.TrimSpace(att.Filename)
				if name == "" {
					name = "附件"
				}
				oversized = append(oversized, name)
				continue
			}
			log.Warn().Err(err).Msg("qq: inbound attachment download failed; skipping")
			continue
		}
		in.Images = append(in.Images, img)
	}
	in = applyOversizedNotice(in, oversized)

	// Run the turn asynchronously so the gateway read loop keeps flowing; the
	// Manager serializes per conversation.
	go onInbound(ctx, in)
}

// applyOversizedNotice routes the rejected-attachment tip. An inbound that has
// nothing but rejected attachments carries the tip as a SafetyNotice, so the
// Manager delivers it through the single egress (policy, dedupe receipt, retry
// and audit) instead of the adapter sending it directly. When there is other
// content, the tip rides along with the user text and the turn runs normally.
func applyOversizedNotice(in channels.InboundMessage, oversized []string) channels.InboundMessage {
	if len(oversized) == 0 {
		return in
	}
	tip := fmt.Sprintf(
		"附件超过 %d MiB 上限，已拒绝：%s。请压缩后重试。",
		qqAttachMaxMiB, strings.Join(oversized, ", "),
	)
	if len(in.Images) == 0 && strings.TrimSpace(in.Text) == "" {
		in.Safety = &channels.SafetyNotice{
			Text: tip, Reason: "oversized_attachment",
			// Same key across gateway reconnects that replay this message.
			DedupeKey: in.MessageID + ":oversize", Only: true,
		}
		return in
	}
	if in.Text != "" {
		in.Text = in.Text + "\n" + tip
	} else {
		in.Text = tip
	}
	return in
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
