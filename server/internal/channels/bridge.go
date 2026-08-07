package channels

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sandbox"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// MCPTokenHooks provides platform MCP token binding for channel turns.
// Memory/scheduler WriteAllowed follow SessionCaps from ChannelConfig (default
// off). pm-workflow write access is controlled by whether pm-workflow-write is
// enabled for the project. Injected from main.go so this package stays free of
// MCP hosts.
type MCPTokenHooks struct {
	// Register mints a shared token and returns the platform MCP inject specs
	// for a fresh sandbox open. caps gate memory/scheduler writes for this turn.
	Register func(projectID, threadID, userID, agent string, channel ChannelSessionContext, enabledMcps []string, caps SessionCaps) (token string, specs []sandbox.MCPServerSpec)
	// RestoreOnReuse rebinds the reused sandbox's existing token using the
	// latest caps (so config changes apply on the next turn without reconnect).
	RestoreOnReuse func(projectID, threadID, userID, agent, token string, channel ChannelSessionContext, enabledMcps []string, caps SessionCaps)
	// Unregister drops a token (used when discarding a freshly minted token
	// after a sandbox reuse, or on open failure).
	Unregister func(token string)
}

// ChannelSessionContext carries the concrete inbound identity and destination
// alongside the conversation-scoped thread user id. PM MCP writes use it for
// risk tickets and explicit lifecycle delivery without guessing a cron target.
type ChannelSessionContext struct {
	ChannelType    string
	Scene          Scene
	ConversationID string
	ExternalUserID string
}

// ResolvedChannel is the per-turn channel context passed to the bridge.
type ResolvedChannel struct {
	ID        string
	Type      string
	ProjectID string
	// OpenTimeout bounds getting a sandbox ready. It is separate from
	// TurnTimeout because provisioning a container and thinking about a
	// question are different kinds of waiting: charging cold start to the
	// answer budget means the first message of a conversation can time out
	// before the agent has read it. 0 → fold into TurnTimeout.
	OpenTimeout time.Duration
	// TurnTimeout bounds the agent's own work once the sandbox is ready.
	// 0 → runner default.
	TurnTimeout time.Duration
	Caps        SessionCaps // from latest ChannelConfig; applied each turn
}

// Reply is the bridge's produced answer for one inbound message.
//
// Text is the raw assistant output and is internal-only: it is persisted and
// used for orchestration but must never reach an external channel. Only
// FinalSummary — a server-constructed, deliverable kind=final summary — may
// leave the process. When FinalSummary is empty the Manager takes an
// observable failure path instead of pretending the turn completed for the user.
type Reply struct {
	Text         string
	FinalSummary string
	ImageURLs    []string
}

// ChannelBridge is the Work-side executor for channel turns: resolve thread,
// ensure sandbox, run PmTurnRunner, and return a TurnFinalReport-equivalent
// Reply. It must not call adapter.Send — all QQ egress goes through Manager
// (Reply). Progress is reported via the optional onProgress callback only for
// the three allowed kinds (milestone / blocker / confirm).
type ChannelBridge struct {
	pm    *services.PmService
	sbx   *services.SandboxService
	turns *services.PmTurnRunner
	hooks MCPTokenHooks
}

// NewChannelBridge builds the bridge.
func NewChannelBridge(pm *services.PmService, sbx *services.SandboxService, turns *services.PmTurnRunner, hooks MCPTokenHooks) *ChannelBridge {
	return &ChannelBridge{pm: pm, sbx: sbx, turns: turns, hooks: hooks}
}

// SyntheticUserID derives a stable per-conversation user id, e.g.
// "qq:group:ABC". One conversation maps to exactly one persistent ChatThread.
func SyntheticUserID(channelType string, scene Scene, conversationID string) string {
	return channelType + ":" + string(scene) + ":" + conversationID
}

// Handle runs one full Work turn and returns the assistant reply for Reply to
// send as the terminal report. onProgress may be nil; when set, only classified
// ProgressEvents are forwarded (tool/token noise suppressed).
func (b *ChannelBridge) Handle(ctx context.Context, rc ResolvedChannel, in InboundMessage, onProgress func(ProgressEvent)) (Reply, error) {
	if b.pm == nil || b.sbx == nil || b.turns == nil {
		return Reply{}, fmt.Errorf("channel bridge unavailable")
	}
	proj, err := b.pm.RequireEnabled(rc.ProjectID)
	if err != nil {
		return Reply{}, err
	}
	agent := proj.PmLeaderAgent
	userID := SyntheticUserID(rc.Type, in.Scene, in.ConversationID)

	thread, err := b.ensureThread(rc.ProjectID, userID, agent, in)
	if err != nil {
		return Reply{}, err
	}

	binding, _ := b.pm.GetBinding(rc.ProjectID)
	channelCtx := ChannelSessionContext{
		ChannelType: rc.Type, Scene: in.Scene,
		ConversationID: in.ConversationID, ExternalUserID: in.UserID,
	}
	// Getting the sandbox up runs on its own clock. A warm conversation passes
	// through here in milliseconds; the first message of a conversation waits
	// for a container, and that wait must not be deducted from the time the
	// agent gets to answer.
	openCtx, closeOpen := ctx, context.CancelFunc(func() {})
	if rc.OpenTimeout > 0 {
		openCtx, closeOpen = context.WithTimeout(ctx, rc.OpenTimeout)
	}
	defer closeOpen()

	token, specs := b.hooks.Register(rc.ProjectID, thread.ID, userID, agent, channelCtx, binding.EnabledMcps, rc.Caps)
	row, reused, err := b.sbx.OpenAgentSandbox(openCtx, services.AgentSandboxOpenOpts{
		Profile:       agent,
		ProjectID:     rc.ProjectID,
		ThreadID:      thread.ID,
		SharedToken:   token,
		PlatformSpecs: specs,
		Reuse:         true,
		RunIDPrefix:   "agent",
	})
	if err != nil {
		if b.hooks.Unregister != nil {
			b.hooks.Unregister(token)
		}
		return Reply{}, err
	}
	if reused {
		if b.hooks.Unregister != nil {
			b.hooks.Unregister(token)
		}
		token = row.Token
		if b.hooks.RestoreOnReuse != nil {
			b.hooks.RestoreOnReuse(rc.ProjectID, thread.ID, userID, agent, token, channelCtx, binding.EnabledMcps, rc.Caps)
		}
	}
	if err := b.pm.BindSandbox(thread.ID, row.ID); err != nil {
		log.Warn().Err(err).Str("thread", thread.ID).Uint("sandbox", row.ID).
			Msg("channel bind sandbox failed")
	}

	if _, err := b.waitReady(openCtx, row.ID); err != nil {
		return Reply{}, err
	}
	closeOpen()

	images := toPromptImages(in.Images)
	userText := formatChannelUserText(in, len(images) > 0)
	userMsg, err := b.pm.AppendMessageSource(thread.ID, "user", userText, "channel", nil, nil, images, nil, nil)
	if err != nil {
		return Reply{}, err
	}

	prompt := userText
	if !reused {
		// First turn of this conversation's sandbox: orient the agent.
		prompt = ChannelPreamble(rc.Type) + "\n\n用户消息：" + userText
	}
	if len(images) > 0 {
		prompt += fmt.Sprintf("\n（本条消息附带 %d 个附件）", len(images))
	}

	if err := b.turns.StartWithTimeout(thread.ID, userMsg.ID, row.ID, prompt, images, rc.TurnTimeout); err != nil {
		return Reply{}, err
	}

	progCtx, progCancel := context.WithCancel(ctx)
	defer progCancel()
	if onProgress != nil {
		go b.forwardProgress(progCtx, thread.ID, onProgress)
	}

	if err := b.waitTurn(ctx, thread.ID, rc.TurnTimeout); err != nil {
		progCancel()
		return Reply{}, err
	}
	progCancel()

	text := b.readReply(thread.ID, userMsg.CreatedAt)
	if strings.TrimSpace(text) == "" {
		return Reply{}, fmt.Errorf("assistant produced no reply")
	}
	stripped, urls := splitImageURLs(text)
	return Reply{
		Text:         stripped,
		FinalSummary: buildDeliverableFinalSummary(stripped),
		ImageURLs:    urls,
	}, nil
}

// forwardProgress subscribes to PmTurnRunner events and forwards only the three
// allowed progress kinds to Reply. ACP agent_message_chunk deltas are coalesced
// before classification (short streaming fragments alone rarely match). Tool
// frames never become QQ text. Status().partial is polled as a backup.
func (b *ChannelBridge) forwardProgress(ctx context.Context, threadID string, onProgress func(ProgressEvent)) {
	if b.turns == nil || onProgress == nil {
		return
	}
	// Brief retry: Start registers the turn just before this goroutine runs.
	var ch <-chan services.PmTurnEvent
	var unsub func()
	var ok bool
	for i := 0; i < 20; i++ {
		ch, unsub, ok = b.turns.Subscribe(threadID, -1)
		if ok {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(25 * time.Millisecond):
		}
	}
	if !ok {
		return
	}
	defer unsub()

	acc := newProgressAccumulator()
	emit := func(events []ProgressEvent) {
		for _, pe := range events {
			onProgress(pe)
		}
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, _, partial, _, _ := b.turns.Status(threadID); partial != "" {
				emit(acc.FeedSnapshot(partial))
			}
			// live_first_sentence opener is hard-disabled for foreground turns:
			// releasing a partial greeting before the final answer caused
			// greeting + answer multi-sends. Minute-scale work must use
			// pm_start_run → RunAcceptanceAck instead.
		case ev, open := <-ch:
			if !open {
				return
			}
			if ev.Type != "acp" || len(ev.Data) == 0 {
				continue
			}
			delta := services.ExtractAgentMessageText(ev.Data)
			if delta == "" {
				continue // tool / non-message frames suppressed
			}
			emit(acc.Feed(delta))
		}
	}
}

func (b *ChannelBridge) ensureThread(projectID, userID, agent string, in InboundMessage) (models.ChatThread, error) {
	threads, err := b.pm.ListThreads(projectID, userID)
	if err != nil {
		return models.ChatThread{}, err
	}
	if len(threads) > 0 {
		return threads[0], nil // ListThreads is ordered updated_at desc
	}
	title := channelThreadTitle(in)
	return b.pm.CreateThread(projectID, userID, title, agent, models.ChatThreadKindUser)
}

func (b *ChannelBridge) waitReady(ctx context.Context, id uint) (*models.Sandbox, error) {
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		row, err := b.sbx.Get(id)
		if err == nil {
			if row.Status == "running" {
				return row, nil
			}
			if row.Status == "error" {
				return nil, fmt.Errorf("沙箱启动失败：%s", row.Error)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("沙箱未就绪")
}

func (b *ChannelBridge) waitTurn(ctx context.Context, threadID string, turnTimeout time.Duration) error {
	max := turnTimeout
	if max <= 0 {
		// Derive from the runner default so we never cancel a turn that the
		// runner itself still considers live.
		max = b.turns.DefaultDeadline()
	}
	max += time.Minute // allow finalize/persist after the chat stream ends
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if !b.turns.Active(threadID) {
			return nil
		}
		select {
		case <-ctx.Done():
			b.turns.Cancel(threadID)
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	b.turns.Cancel(threadID)
	return fmt.Errorf("回合超时")
}

func (b *ChannelBridge) readReply(threadID string, after time.Time) string {
	msgs, err := b.pm.ListMessages(threadID)
	if err != nil {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == "assistant" && !m.CreatedAt.Before(after) {
			return m.Content
		}
	}
	return ""
}

func channelThreadTitle(in InboundMessage) string {
	t := strings.TrimSpace(in.Text)
	if t == "" {
		t = "渠道会话"
	}
	if len([]rune(t)) > 30 {
		t = string([]rune(t)[:30]) + "…"
	}
	return t
}

// formatChannelUserText builds the persisted/prompt user text. Group and guild
// threads are keyed by conversation (so the room shares one PM thread); speaker
// identity is prefixed so the agent can attribute multi-user turns.
func formatChannelUserText(in InboundMessage, hasAttachments bool) string {
	text := strings.TrimSpace(in.Text)
	if text == "" && hasAttachments {
		text = "(用户发送了附件)"
	}
	speaker := strings.TrimSpace(in.UserID)
	if speaker == "" || (in.Scene != SceneGroup && in.Scene != SceneGuild) {
		return text
	}
	if text == "" {
		return "[来自 " + speaker + "]"
	}
	return "[来自 " + speaker + "] " + text
}

func toPromptImages(in []Image) []models.PromptImage {
	if len(in) == 0 {
		return nil
	}
	out := make([]models.PromptImage, 0, len(in))
	for i, img := range in {
		if len(img.Data) == 0 {
			continue
		}
		mime := img.MimeType
		if mime == "" {
			mime = "image/png"
		}
		name := strings.TrimSpace(img.Filename)
		if name == "" {
			name = fmt.Sprintf("attachment-%d%s", i+1, extForMime(mime))
		}
		out = append(out, models.PromptImage{
			Data:     base64.StdEncoding.EncodeToString(img.Data),
			MimeType: mime,
			Name:     name,
		})
	}
	return out
}

func extForMime(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/png"):
		return ".png"
	case strings.HasPrefix(mime, "image/jpeg"), strings.HasPrefix(mime, "image/jpg"):
		return ".jpg"
	case strings.HasPrefix(mime, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mime, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mime, "application/pdf"):
		return ".pdf"
	case strings.Contains(mime, "zip"):
		return ".zip"
	default:
		return ""
	}
}

var (
	mdImageRe           = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)
	bareImageRe         = regexp.MustCompile(`https?://[^\s)]+\.(?:png|jpe?g|gif|webp)(?:\?[^\s)]*)?`)
	finalSummaryMarkers = []string{"[摘要]", "【摘要】", "[最终]", "【最终】", "[Final]", "[Summary]"}
)

// buildDeliverableFinalSummary constructs the only text allowed into
// Reply.FinalSummary. Marker lines ([摘要]/…) win; otherwise a constrained
// conversational summary is derived from user-visible assistant lines.
// Empty means the Manager must fail observably — never leak Reply.Text whole.
func buildDeliverableFinalSummary(text string) string {
	if summary := extractStructuredFinalSummary(text); summary != "" {
		return summary
	}
	return constrainedConversationalSummary(text)
}

// extractStructuredFinalSummary pulls an orchestration-authored structured
// summary line. Unmarked narration never becomes a FinalSummary here; the
// conversational fallback lives in constrainedConversationalSummary.
func extractStructuredFinalSummary(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, marker := range finalSummaryMarkers {
			if strings.HasPrefix(trimmed, marker) {
				return truncateRunes(strings.TrimSpace(strings.TrimPrefix(trimmed, marker)), 240)
			}
		}
	}
	return ""
}

// constrainedConversationalSummary builds a short user-facing final from
// assistant body text: drop tool/reasoning/progress-marker noise, then truncate.
// It is still a server-owned summary, not a raw Reply.Text passthrough.
func constrainedConversationalSummary(text string) string {
	var keep []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isFinalSummaryNoiseLine(trimmed) {
			continue
		}
		keep = append(keep, trimmed)
	}
	joined := strings.TrimSpace(strings.Join(keep, "\n"))
	if joined == "" {
		return ""
	}
	return truncateRunes(joined, 240)
}

func isFinalSummaryNoiseLine(line string) bool {
	if isProgressNoise(line) {
		return true
	}
	if isDeliveryReceiptOrProcessLine(line) {
		return true
	}
	for _, m := range progressMarkers {
		if strings.HasPrefix(line, m.prefix) {
			return true
		}
	}
	for _, marker := range finalSummaryMarkers {
		if strings.HasPrefix(line, marker) {
			return true
		}
	}
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(line, "内部推理"),
		strings.HasPrefix(lower, "reasoning:"),
		strings.HasPrefix(lower, "thinking:"):
		return true
	case strings.Contains(lower, "tool_call"),
		strings.Contains(lower, "tool call"),
		strings.Contains(lower, "function_call"),
		strings.Contains(lower, "reasoning_delta"):
		return true
	}
	return false
}

// isDeliveryReceiptOrProcessLine matches model asides that restate delivery
// results or fixed wait/ack templates. These must never become user-visible
// FinalSummary content (e.g. "已发送。" after pm_reply status=sent).
func isDeliveryReceiptOrProcessLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	// Strip trailing punctuation for exact-phrase matching.
	core := strings.TrimRight(trimmed, "。.!！?？…")
	core = strings.TrimSpace(core)
	exact := []string{
		"已发送",
		"已通过 QQ 回复用户",
		"已通过QQ回复用户",
		"稍等，我看一下",
		"稍等我看一下",
		"Give me a moment on this one",
		"已开始处理",
		"任务已启动",
		"正在处理",
		"收到，正在处理",
	}
	for _, p := range exact {
		if strings.EqualFold(core, p) {
			return true
		}
	}
	lower := strings.ToLower(trimmed)
	// Soft contains for short receipt asides that include channel names.
	soft := []string{
		"已通过 qq 回复用户",
		"已通过qq回复用户",
		"已发送。",
		"give me a moment on this one",
	}
	for _, p := range soft {
		if strings.Contains(lower, p) && utf8.RuneCountInString(trimmed) <= 40 {
			return true
		}
	}
	return false
}

// isReceiptOrProcessOnlyBody reports whether assistant text has no user-facing
// content and includes delivery-receipt / fixed-process asides. Such turns must
// produce 0 user-visible sends. Pure tool/reasoning noise (no receipts) still
// takes the missing-answer fallback path so failures stay observable.
func isReceiptOrProcessOnlyBody(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	sawReceipt := false
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isDeliveryReceiptOrProcessLine(line) {
			sawReceipt = true
			continue
		}
		if isFinalSummaryNoiseLine(line) {
			continue
		}
		return false
	}
	return sawReceipt
}

// splitImageURLs extracts shareable image URLs (markdown or bare) from an
// assistant reply and returns the text with markdown image syntax removed.
func splitImageURLs(text string) (string, []string) {
	seen := map[string]bool{}
	var urls []string
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		urls = append(urls, u)
	}
	for _, m := range mdImageRe.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}
	stripped := mdImageRe.ReplaceAllString(text, "")
	for _, u := range bareImageRe.FindAllString(stripped, -1) {
		add(u)
	}
	stripped = strings.TrimSpace(stripped)
	if len(urls) > 4 {
		log.Warn().Int("count", len(urls)).Msg("channel reply had many image urls; capping to 4")
		urls = urls[:4]
	}
	return stripped, urls
}
