package channels

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

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
	Register func(projectID, threadID, userID, agent string, enabledMcps []string, caps SessionCaps) (token string, specs []sandbox.MCPServerSpec)
	// RestoreOnReuse rebinds the reused sandbox's existing token using the
	// latest caps (so config changes apply on the next turn without reconnect).
	RestoreOnReuse func(projectID, threadID, userID, agent, token string, enabledMcps []string, caps SessionCaps)
	// Unregister drops a token (used when discarding a freshly minted token
	// after a sandbox reuse, or on open failure).
	Unregister func(token string)
}

// ResolvedChannel is the per-turn channel context passed to the bridge.
type ResolvedChannel struct {
	ID            string
	Type          string
	ProjectID     string
	TurnTimeout   time.Duration // 0 → runner default
	Caps          SessionCaps   // from latest ChannelConfig; applied each turn
	ReplyMetadata bool
}

// Reply is the bridge's produced answer for one inbound message. Attachments
// live on Final because only a structured final report may authorize them.
type Reply struct {
	Text       string
	RunID      string
	ShortTitle string
	Final      *TurnFinalReport
}

// RunAcceptance is emitted once the inbound orchestration associates a
// conversation with a Run. It carries routing identity only.
type RunAcceptance struct {
	ProjectID      string
	Channel        string
	Scene          Scene
	ConversationID string
	UserID         string
	RunID          string
	ShortTitle     string
	Language       string
}

// RunStateSource exposes engine-persisted Run state. Progress derived from it
// is a server structure, never assistant text.
type RunStateSource interface {
	Get(runID string) (models.Run, bool)
}

// ChannelBridge is the Work-side executor for channel turns: resolve thread,
// ensure sandbox, run PmTurnRunner, and return a TurnFinalReport-equivalent
// Reply. It must not call adapter.Send — all QQ egress goes through Manager
// (Reply). Progress is reported via the optional onProgress callback only for
// the three allowed kinds (milestone / blocker / confirm).
type ChannelBridge struct {
	pm       *services.PmService
	sbx      *services.SandboxService
	turns    *services.PmTurnRunner
	hooks    MCPTokenHooks
	tasks    *services.TaskContextService
	runState RunStateSource
	accepted func(RunAcceptance)
}

// NewChannelBridge builds the bridge.
func NewChannelBridge(pm *services.PmService, sbx *services.SandboxService, turns *services.PmTurnRunner, hooks MCPTokenHooks) *ChannelBridge {
	return &ChannelBridge{pm: pm, sbx: sbx, turns: turns, hooks: hooks}
}

func (b *ChannelBridge) SetTaskContext(tasks *services.TaskContextService) {
	b.tasks = tasks
}

// SetRunState wires the engine-persisted Run state used by the structured
// progress producer.
func (b *ChannelBridge) SetRunState(src RunStateSource) {
	b.runState = src
}

// SetRunAcceptanceHook registers the Reply-side acceptance ACK producer.
func (b *ChannelBridge) SetRunAcceptanceHook(fn func(RunAcceptance)) {
	b.accepted = fn
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
	preflight, err := b.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: in})
	if err != nil {
		return Reply{}, err
	}
	if preflight.Disposition == PreflightRespond {
		return preflight.Reply, nil
	}
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
	if b.tasks != nil {
		// Guarding is unconditional for channel threads: destructive PM MCP
		// mutations are denied unless a confirmed ticket is spendable.
		if guardErr := b.tasks.GuardChannelThread(rc.ProjectID, thread.ID, rc.Type, userID); guardErr != nil {
			return Reply{}, guardErr
		}
		if preflight.AuthorizedAction != "" && preflight.TicketID != "" {
			if err := b.tasks.BindActionGrant(preflight.TicketID, thread.ID); err != nil {
				return Reply{}, err
			}
			// The grant lives only for the turn the user authorized.
			defer func() { _ = b.tasks.ReleaseActionGrant(preflight.TicketID) }()
		}
	}

	binding, _ := b.pm.GetBinding(rc.ProjectID)
	token, specs := b.hooks.Register(rc.ProjectID, thread.ID, userID, agent, binding.EnabledMcps, rc.Caps)
	row, reused, err := b.sbx.OpenAgentSandbox(ctx, services.AgentSandboxOpenOpts{
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
			b.hooks.RestoreOnReuse(rc.ProjectID, thread.ID, userID, agent, token, binding.EnabledMcps, rc.Caps)
		}
	}
	if err := b.pm.BindSandbox(thread.ID, row.ID); err != nil {
		log.Warn().Err(err).Str("thread", thread.ID).Uint("sandbox", row.ID).
			Msg("channel bind sandbox failed")
	}

	if _, err := b.waitReady(ctx, row.ID); err != nil {
		return Reply{}, err
	}

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
	if preflight.Task != nil {
		prompt = fmt.Sprintf(
			"[任务上下文]\nRun: %s\n短标题: %s\n状态: %s\n\n%s",
			preflight.Task.RunID, preflight.Task.ShortTitle, preflight.Task.Status, prompt,
		)
	}
	if preflight.AuthorizedAction != "" {
		prompt = fmt.Sprintf(
			"[已确认的高风险操作]\nTicket: %s\nRun: %s\nAction: %s\n仅允许执行此票据绑定操作。\n\n%s",
			preflight.TicketID, preflight.Task.RunID, preflight.AuthorizedAction, prompt,
		)
	}

	if err := b.turns.StartWithTimeout(thread.ID, userMsg.ID, row.ID, prompt, images, rc.TurnTimeout); err != nil {
		return Reply{}, err
	}

	progCtx, progCancel := context.WithCancel(ctx)
	defer progCancel()
	if onProgress != nil {
		progressRunID := row.RunID
		if preflight.Task != nil {
			progressRunID = preflight.Task.RunID
		}
		go b.forwardProgress(progCtx, thread.ID, progressRunID, onProgress)
		if preflight.Task != nil {
			go b.watchRunState(progCtx, preflight.Task.RunID, onProgress)
		}
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
	stripped, summary, urls := finalFromAssistantText(text)
	replyRunID, shortTitle := row.RunID, ""
	if preflight.Task != nil {
		replyRunID, shortTitle = preflight.Task.RunID, preflight.Task.ShortTitle
	}
	return Reply{
		Text: stripped, RunID: replyRunID, ShortTitle: shortTitle,
		Final: &TurnFinalReport{OK: true, Summary: summary, ImageURLs: urls},
	}, nil
}

// finalFromAssistantText derives the terminal report. Attachments are returned
// only when the assistant used the explicit structured final marker: unmarked
// terminal output stays internal, and so do the URLs embedded in it.
func finalFromAssistantText(text string) (stripped, summary string, images []string) {
	stripped, urls := splitImageURLs(text)
	summary, ok := ExtractStructuredFinalSummary(stripped)
	if !ok {
		return stripped, "处理完成，请在项目中查看结果。", nil
	}
	return stripped, summary, urls
}

// watchRunState is the production structured progress producer. It reads the
// engine-persisted Run row — never assistant output — and emits an authorized
// event on each server-observed transition.
func (b *ChannelBridge) watchRunState(ctx context.Context, runID string, onProgress func(ProgressEvent)) {
	if b.runState == nil || onProgress == nil || strings.TrimSpace(runID) == "" {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	last := runStateSnapshot{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, ok := b.runState.Get(runID)
			if !ok {
				continue
			}
			ev, next, changed := runStateProgress(run, last)
			last = next
			if changed {
				onProgress(NewSendableProgressEvent(ev.Kind, ev.Summary, runID))
			}
		}
	}
}

// runStateSnapshot is the last server state a producer reported on.
type runStateSnapshot struct {
	status string
	step   int
	seen   bool
}

// runStateProgress maps engine Run state to a substantive progress event.
// Progress is bucketed to 10% so routine ticks do not become messages.
func runStateProgress(run models.Run, last runStateSnapshot) (ProgressEvent, runStateSnapshot, bool) {
	step := int(run.Progress * 10)
	if step < 0 {
		step = 0
	}
	next := runStateSnapshot{status: run.Status, step: step, seen: true}
	if !last.seen {
		// Baseline only: the first observation is the state the user already
		// asked about, not a change.
		return ProgressEvent{}, next, false
	}
	if run.Status != last.status {
		switch run.Status {
		case "waiting_human":
			return ProgressEvent{Kind: ProgressConfirm, Summary: "运行等待人工决策"}, next, true
		case "failed":
			return ProgressEvent{Kind: ProgressBlocker, Summary: "运行失败"}, next, true
		case "cancelled":
			return ProgressEvent{Kind: ProgressBlocker, Summary: "运行已取消"}, next, true
		case "completed":
			return ProgressEvent{Kind: ProgressMilestone, Summary: "运行已完成"}, next, true
		default:
			return ProgressEvent{
				Kind: ProgressMilestone, Summary: "运行状态：" + run.Status,
			}, next, true
		}
	}
	if step > last.step {
		return ProgressEvent{
			Kind:    ProgressMilestone,
			Summary: fmt.Sprintf("运行进度 %d%%", step*10),
		}, next, true
	}
	return ProgressEvent{}, next, false
}

// forwardProgress subscribes to PmTurnRunner events and forwards only the three
// allowed progress kinds to Reply. ACP agent_message_chunk deltas are coalesced
// before classification (short streaming fragments alone rarely match). Tool
// frames never become QQ text. Status().partial is polled as a backup.
func (b *ChannelBridge) forwardProgress(ctx context.Context, threadID, runID string, onProgress func(ProgressEvent)) {
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
			pe.RunID = runID
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
	mdImageRe   = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)
	bareImageRe = regexp.MustCompile(`https?://[^\s)]+\.(?:png|jpe?g|gif|webp)(?:\?[^\s)]*)?`)
)

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
