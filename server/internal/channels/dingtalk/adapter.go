package dingtalk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	dingclient "github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/rs/zerolog/log"

	"github.com/cocofhu/approving/internal/channels"
)

// Adapter is the DingTalk Stream channel adapter.
// Inbound uses Stream long-connection; outbound prefers sessionWebhook and
// falls back to OpenAPI (groupMessages / oToMessages) when webhook is absent
// or expired. robotCode defaults to AppID (ClientID).
type Adapter struct {
	cfg       channels.AdapterConfig
	robotCode string
	tokens    tokenCache
	webhooks  *webhookCache
	hasSpoken func(scene channels.Scene, conversationID string) bool

	mu      sync.Mutex
	seen    map[string]time.Time
	cancel  context.CancelFunc
	onState func(state, detail string)
	stream  *dingclient.StreamClient
}

// New builds a DingTalk adapter from a resolved config.
func New(cfg channels.AdapterConfig) (channels.Adapter, error) {
	if strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.AppSecret) == "" {
		return nil, fmt.Errorf("dingtalk: appId 和 appSecret 必填")
	}
	return &Adapter{
		cfg:       cfg,
		robotCode: strings.TrimSpace(cfg.AppID),
		webhooks:  newWebhookCache(),
		hasSpoken: cfg.HasSpoken,
		seen:      map[string]time.Time{},
	}, nil
}

// Type implements channels.Adapter.
func (a *Adapter) Type() string { return "dingtalk" }

// SetStateHandler implements channels.StatefulAdapter.
func (a *Adapter) SetStateHandler(fn func(state, detail string)) {
	a.mu.Lock()
	a.onState = fn
	a.mu.Unlock()
}

func (a *Adapter) report(state, detail string) {
	a.mu.Lock()
	fn := a.onState
	a.mu.Unlock()
	if fn != nil {
		fn(state, detail)
	}
}

// Start probes access_token then starts the Stream client with reconnect.
func (a *Adapter) Start(ctx context.Context, onInbound channels.InboundHandler) error {
	probeCtx, cancelProbe := context.WithTimeout(ctx, 12*time.Second)
	_, err := a.tokens.get(probeCtx, a.cfg.AppID, a.cfg.AppSecret)
	cancelProbe()
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()

	cli := dingclient.NewStreamClient(
		dingclient.WithAppCredential(dingclient.NewAppCredentialConfig(a.cfg.AppID, a.cfg.AppSecret)),
	)
	cli.RegisterChatBotCallbackRouter(func(cbCtx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		go a.handleReceive(runCtx, data, onInbound)
		return []byte(""), nil
	})
	a.mu.Lock()
	a.stream = cli
	a.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		backoff := time.Second
		for {
			if runCtx.Err() != nil {
				return
			}
			err := cli.Start(runCtx)
			select {
			case errCh <- err:
			default:
			}
			if runCtx.Err() != nil {
				return
			}
			if err != nil {
				a.report(channels.ConnStateDisconnected,
					"钉钉 Stream 长连接已断开，正在重连。请确认应用在线且凭据有效。")
				log.Warn().Err(err).Dur("backoff", backoff).Msg("dingtalk: stream disconnected; retrying")
			}
			select {
			case <-runCtx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}()

	select {
	case err := <-errCh:
		if err != nil {
			cancel()
			cli.Close()
			if isAuthish(err) {
				return fmt.Errorf("%w: %v", channels.ErrAdapterAuth, err)
			}
			return err
		}
		a.report(channels.ConnStateConnected, "钉钉 Stream 已连接")
		return nil
	case <-time.After(5 * time.Second):
		a.report(channels.ConnStateConnected, "钉钉 Stream 已连接")
		return nil
	case <-ctx.Done():
		cancel()
		cli.Close()
		return ctx.Err()
	}
}

func isAuthish(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "invalid") ||
		strings.Contains(s, "unauthorized") ||
		strings.Contains(s, "appkey") ||
		strings.Contains(s, "appsecret") ||
		strings.Contains(s, "client_id") ||
		strings.Contains(s, "clientid") ||
		strings.Contains(s, "credential") ||
		strings.Contains(s, "401") ||
		strings.Contains(s, "403")
}

// Stop implements channels.Adapter.
func (a *Adapter) Stop() error {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	cli := a.stream
	a.stream = nil
	a.mu.Unlock()
	if cli != nil {
		cli.Close()
	}
	return nil
}

// Send implements channels.Adapter with sessionWebhook-first dual path.
func (a *Adapter) Send(ctx context.Context, out channels.OutboundMessage) error {
	if strings.TrimSpace(out.ConversationID) == "" {
		return fmt.Errorf("dingtalk: missing conversation id")
	}
	msgType, title, text := a.chooseOutbound(out.Text)

	if entry, ok := a.webhooks.get(out.Scene, out.ConversationID); ok && entry.URL != "" {
		if err := replySessionWebhook(ctx, entry.URL, msgType, title, text); err != nil {
			log.Warn().Err(err).Msg("dingtalk: sessionWebhook send failed; falling back to OpenAPI")
		} else {
			a.sendOutboundImages(ctx, out, entry.StaffID, true)
			return nil
		}
	}

	// Active / fallback OpenAPI path.
	if out.ReplyToMessageID == "" && a.hasSpoken != nil && !a.hasSpoken(out.Scene, out.ConversationID) {
		return ErrUnspoken
	}
	token, err := a.tokens.get(ctx, a.cfg.AppID, a.cfg.AppSecret)
	if err != nil {
		return err
	}
	staffID := a.webhooks.staffID(out.Scene, out.ConversationID)
	msgKey, msgParam := openAPIPayload(msgType, title, text)
	if err := sendOpenAPI(ctx, token, a.robotCode, out.Scene, out.ConversationID, staffID, msgKey, msgParam); err != nil {
		return err
	}
	a.sendOutboundImages(ctx, out, staffID, false)
	return nil
}

func openAPIPayload(msgType, title, text string) (msgKey, msgParam string) {
	if msgType == "markdown" {
		return "sampleMarkdown", markdownMsgParam(title, text)
	}
	return "sampleText", textMsgParam(text)
}

func (a *Adapter) chooseOutbound(text string) (msgType, title, body string) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "text", "Approving", ""
	}
	if isProgressOrAck(t) {
		return "text", "Approving", t
	}
	title = "Approving"
	for _, line := range strings.Split(t, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		title = truncateRunes(strings.TrimLeft(line, "# "), 40)
		break
	}
	return "markdown", title, t
}

func isProgressOrAck(t string) bool {
	prefixes := []string{
		"已收到，正在处理", "收到，正在处理", "已收到，排队中",
		"[进度]", "[阻塞]", "[确认]", "进度：", "阻塞：", "需确认：",
		"处理失败：", "队列已满", unsupportedMediaHint,
	}
	for _, p := range prefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

func (a *Adapter) sendOutboundImages(ctx context.Context, out channels.OutboundMessage, staffID string, viaWebhookOK bool) {
	if len(out.ImageURLs) == 0 {
		return
	}
	token, err := a.tokens.get(ctx, a.cfg.AppID, a.cfg.AppSecret)
	if err != nil {
		log.Warn().Err(err).Msg("dingtalk: token for outbound image failed")
		return
	}
	for _, u := range out.ImageURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		// Prefer public URL as sampleImageMsg.photoURL — never stuff media_id into photoURL.
		if !isPublicHTTPURL(u) {
			log.Warn().Str("url", u).Msg("dingtalk: outbound image requires public http(s) photoURL")
			continue
		}
		if viaWebhookOK {
			if entry, ok := a.webhooks.get(out.Scene, out.ConversationID); ok && entry.URL != "" {
				if err := replySessionWebhookImage(ctx, entry.URL, u); err == nil {
					continue
				}
				log.Warn().Err(err).Str("url", u).Msg("dingtalk: webhook image failed; trying OpenAPI")
			}
		}
		if err := sendOpenAPI(ctx, token, a.robotCode, out.Scene, out.ConversationID, staffID,
			"sampleImageMsg", imageMsgParam(u)); err != nil {
			md := fmt.Sprintf("![](%s)", u)
			if mdErr := sendOpenAPI(ctx, token, a.robotCode, out.Scene, out.ConversationID, staffID,
				"sampleMarkdown", markdownMsgParam("image", md)); mdErr != nil {
				log.Warn().Err(err).Err(mdErr).Str("url", u).Msg("dingtalk: outbound image send failed")
			} else {
				log.Warn().Err(err).Str("url", u).Msg("dingtalk: sampleImageMsg failed; used markdown url fallback")
			}
		}
	}
}

func (a *Adapter) handleReceive(ctx context.Context, data *chatbot.BotCallbackDataModel, onInbound channels.InboundHandler) {
	if data == nil {
		return
	}
	ev := inboundEvent{
		MessageID:        strings.TrimSpace(data.MsgId),
		ConversationID:   strings.TrimSpace(data.ConversationId),
		ConversationType: strings.TrimSpace(data.ConversationType),
		MsgType:          strings.TrimSpace(data.Msgtype),
		Text:             data.Text.Content,
		Content:          data.Content,
		SenderStaffID:    strings.TrimSpace(data.SenderStaffId),
		SenderID:         strings.TrimSpace(data.SenderId),
		ChatbotUserID:    strings.TrimSpace(data.ChatbotUserId),
		IsInAtList:       data.IsInAtList,
		SessionWebhook:   strings.TrimSpace(data.SessionWebhook),
		WebhookExpiredMs: data.SessionWebhookExpiredTime,
	}
	for _, u := range data.AtUsers {
		ev.AtUsers = append(ev.AtUsers, atUser{DingtalkID: u.DingtalkId, StaffID: u.StaffId})
	}
	if ev.MessageID == "" || a.isDuplicate(ev.MessageID) {
		return
	}
	if !shouldAccept(ev) {
		return
	}
	scene, ok := sceneOf(ev.ConversationType)
	if !ok {
		return
	}

	peer := senderUserID(ev)
	convID := conversationRef(scene, ev.ConversationID, peer)
	a.webhooks.put(scene, convID, ev.SessionWebhook, ev.WebhookExpiredMs, peer)
	if peer != "" {
		a.webhooks.rememberStaff(scene, convID, peer)
	}

	in := channels.InboundMessage{
		Scene:          scene,
		ConversationID: convID,
		UserID:         peer,
		Text:           extractText(ev),
		MessageID:      ev.MessageID,
		Timestamp:      time.Now(),
	}
	if isUnsupportedMedia(ev.MsgType) {
		in.ChannelHint = unsupportedMediaHint
		if in.Text == "" {
			in.Text = unsupportedMediaHint
		}
	}

	var oversized bool
	codes := imageDownloadCodes(ev)
	if len(codes) > 0 {
		token, err := a.tokens.get(ctx, a.cfg.AppID, a.cfg.AppSecret)
		if err != nil {
			log.Warn().Err(err).Msg("dingtalk: token for inbound image failed")
		} else {
			for _, code := range codes {
				data, mime, err := downloadByCode(ctx, token, a.robotCode, code)
				if err != nil {
					if err == errTooLarge {
						oversized = true
						continue
					}
					log.Warn().Err(err).Str("code", code).Msg("dingtalk: download image failed")
					continue
				}
				in.Images = append(in.Images, channels.Image{Data: data, MimeType: mime, Filename: code})
			}
		}
	}
	if oversized {
		tip := oversizeHint
		if in.Text != "" {
			in.Text = in.Text + "\n" + tip
		} else {
			in.Text = tip
		}
		if len(in.Images) == 0 && strings.TrimSpace(extractText(ev)) == "" {
			if err := a.Send(ctx, channels.OutboundMessage{
				Scene: scene, ConversationID: convID, ReplyToMessageID: ev.MessageID, Text: tip,
			}); err != nil {
				log.Warn().Err(err).Msg("dingtalk: oversize tip failed")
			}
			return
		}
	}

	if onInbound != nil {
		onInbound(ctx, in)
	}
}

func (a *Adapter) isDuplicate(msgID string) bool {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.seen[msgID]; ok {
		return true
	}
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

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
