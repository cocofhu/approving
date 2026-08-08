package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// The conversation layer is a person reporting to the user, not a switchboard.
//
// That distinction decides what it is allowed to know. A reporter who invents a
// status is worse than one who has none, so everything it says about work in
// flight comes from here: the task ledger, the progress notes recorded as they
// arrive, and the tools that read them. The model supplies the wording; this
// file supplies the facts.

// ledgerLimit bounds how many live tasks the director is shown at once. A
// conversation with more than a handful in flight has a different problem, and
// listing them all would push the actual question out of the prompt.
const ledgerLimit = 5

// maxConcurrentWork caps the tasks one conversation may have running. The limit
// is the director's to explain, not a silent rejection: past this point it says
// so and offers to cancel something.
const maxConcurrentWork = 3

// workNote is the most recent thing the platform actually observed about a
// task. It is written when progress arrives and read when the user asks, which
// is what makes "还在跑，现在在查代码" a fact rather than a guess.
type workNote struct {
	Stage   string
	Blocked bool
	At      time.Time
}

// ledgerEntry is one task as the director is allowed to describe it.
type ledgerEntry struct {
	TaskID       string `json:"task_id"`
	RunID        string `json:"run_id,omitempty"`
	ShortTitle   string `json:"short_title"`
	Status       string `json:"status"`
	Stage        string `json:"stage,omitempty"`
	Blocked      bool   `json:"blocked,omitempty"`
	LastProgress string `json:"last_progress,omitempty"`
	// ResultSummary is the completed-run digest (findings, links, etc.).
	// Empty for in-flight work.
	ResultSummary string `json:"result_summary,omitempty"`
	UpdatedAgo    string `json:"updated_ago,omitempty"`
}

// directorContext is what the conversation layer knows before it speaks.
//
// It is a briefing, not an authority: the model may read it to decide what to
// do, but a status it reports to the user must come back from get_status. The
// two are usually the same, and when they are not it is because something
// finished while the model was thinking — exactly the case where guessing from
// a snapshot is wrong.
type directorContext struct {
	ConversationBusy  bool            `json:"conversation_busy"`
	FocusTaskID       string          `json:"focus_task_id,omitempty"`
	Tasks             []ledgerEntry   `json:"tasks,omitempty"`
	RecentTerminal    []ledgerEntry   `json:"recent_terminal,omitempty"`
	RecentAttachments []AttachmentRef `json:"recent_attachments,omitempty"`
	LastOutbound      string          `json:"last_outbound,omitempty"`
	UserMessageID     string          `json:"user_message_id,omitempty"`
}

// recentTerminalWindow is how far back get_status / briefing still surface
// finished work so "什么情况了" cannot invent success from an empty active list.
const recentTerminalWindow = 24 * time.Hour

// render turns the briefing into the lines the model reads.
//
// An empty ledger is stated outright while every other empty section is simply
// left out. The asymmetry is deliberate: "nothing is running" is the fact that
// stops the model from reporting progress on work that does not exist, whereas
// "there are no attachments" is a sentence that only invites it to say so.
func (dc directorContext) render() string {
	var b strings.Builder
	b.WriteString("【当前情况】\n")
	if len(dc.Tasks) == 0 {
		b.WriteString("现在没有在跑的任务。这不等于「做完了」——先看刚结束的任务状态。\n")
	} else {
		b.WriteString("在跑的任务：\n")
		for _, t := range dc.Tasks {
			line := fmt.Sprintf("- %s（taskId=%s，状态=%s", t.ShortTitle, t.TaskID, t.Status)
			if t.Stage != "" {
				line += "，阶段=" + t.Stage
			}
			if t.Blocked {
				line += "，被阻塞"
			}
			if t.UpdatedAgo != "" {
				line += "，" + t.UpdatedAgo + "有更新"
			}
			line += "）"
			if t.LastProgress != "" {
				line += "\n  最近进展：" + t.LastProgress
			}
			b.WriteString(line + "\n")
		}
		if dc.FocusTaskID != "" {
			b.WriteString("你们刚才在聊的是 taskId=" + dc.FocusTaskID + "。\n")
			b.WriteString("对方若是补充/收窄这件事，用 refine_work 挂到这个 taskId，不要新开任务，也不要把队列甩给对方选。\n")
		}
	}
	if len(dc.RecentTerminal) > 0 {
		b.WriteString("刚结束的任务（务必按 status 原样说，failed≠完成）：\n")
		for _, t := range dc.RecentTerminal {
			line := fmt.Sprintf("- %s（taskId=%s，状态=%s", t.ShortTitle, t.TaskID, t.Status)
			if t.UpdatedAgo != "" {
				line += "，" + t.UpdatedAgo
			}
			line += "）"
			b.WriteString(line + "\n")
			if t.ResultSummary != "" {
				b.WriteString("  结论摘要：" + t.ResultSummary + "\n")
			}
		}
		if hasFailedOrCancelled(dc.RecentTerminal) {
			b.WriteString("其中有失败或取消：先说清状况，再让对方选下一步（重试 / 换范围或改方向 / 先搁置）。不要擅自说接着做。\n")
		}
		if hasCompletedWithSummary(dc.RecentTerminal) {
			b.WriteString("对方若追问刚结束任务里的细节或交付物：用结论摘要和上一条汇报作答；没有的事实如实说没有，不要抛开摘要做名词百科。\n")
		}
	}
	if dc.ConversationBusy {
		b.WriteString("现在有一件事正在前台执行。\n")
	}
	if len(dc.RecentAttachments) > 0 {
		b.WriteString("这个会话里已有的附件（你看不到内容，但可以在 dispatch_pm 时把它们指给干活的人）：\n")
		for _, ref := range dc.RecentAttachments {
			b.WriteString(fmt.Sprintf("- %s（%s，messageId=%s，index=%d）\n",
				ref.Name, ref.MimeType, ref.MessageID, ref.Index))
		}
	}
	if dc.LastOutbound != "" {
		b.WriteString("你上一条对他说的是：" + dc.LastOutbound + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildDirectorContext assembles the briefing for one inbound message.
func (m *Manager) buildDirectorContext(rc *runningChannel, in InboundMessage) directorContext {
	dc := directorContext{
		ConversationBusy: m.IsConversationBusy(rc.cfg.ProjectID, in.Scene, in.ConversationID),
		UserMessageID:    strings.TrimSpace(in.RecordedMessageID),
		Tasks:            m.taskLedger(rc, in),
		RecentTerminal:   m.recentTerminalLedger(rc, in),
	}
	if focus := m.focusTaskID(rc, in); focus != "" {
		dc.FocusTaskID = focus
	}
	if b, ok := m.transcript.(*ChannelBridge); ok && b != nil {
		dc.RecentAttachments = b.RecentAttachments(conversationRefFor(rc, in))
	}
	dc.LastOutbound = m.lastOutboundText(rc, in)
	return dc
}

// taskLedger lists this conversation's live tasks with whatever the platform
// has actually observed about them.
func (m *Manager) taskLedger(rc *runningChannel, in InboundMessage) []ledgerEntry {
	if m.taskContext == nil {
		return nil
	}
	tasks, err := m.taskContext.ActiveTasksForConversation(m.taskScopeFor(rc, in), ledgerLimit)
	if err != nil {
		log.Warn().Err(err).Str("project", rc.cfg.ProjectID).
			Msg("task ledger unavailable; the conversation layer speaks without it")
		return nil
	}
	return m.entriesFromIdentities(rc.cfg.ProjectID, tasks)
}

func (m *Manager) recentTerminalLedger(rc *runningChannel, in InboundMessage) []ledgerEntry {
	if m.taskContext == nil {
		return nil
	}
	tasks, err := m.taskContext.RecentTerminalTasksForConversation(
		m.taskScopeFor(rc, in), ledgerLimit, recentTerminalWindow)
	if err != nil {
		log.Debug().Err(err).Str("project", rc.cfg.ProjectID).
			Msg("recent terminal ledger unavailable")
		return nil
	}
	return m.entriesFromIdentities(rc.cfg.ProjectID, tasks)
}

func (m *Manager) entriesFromIdentities(projectID string, tasks []models.TaskIdentity) []ledgerEntry {
	out := make([]ledgerEntry, 0, len(tasks))
	for _, t := range tasks {
		entry := ledgerEntry{
			TaskID: t.ID, RunID: t.RunID,
			ShortTitle: services.SanitizeShortTitle(t.ShortTitle),
			Status:     t.Status,
			UpdatedAgo: humanizeAge(time.Since(t.UpdatedAt)),
		}
		if note, ok := m.workNoteFor(projectID, t.RunID); ok {
			entry.Stage, entry.Blocked = note.Stage, note.Blocked
			entry.LastProgress = note.Stage
		}
		if services.IsTerminalTaskStatus(t.Status) {
			entry.ResultSummary = truncateRunes(strings.TrimSpace(t.RecentContext), 400)
		}
		out = append(out, entry)
	}
	return out
}

func (m *Manager) focusTaskID(rc *runningChannel, in InboundMessage) string {
	if id := m.focusIdentity(rc, in); id != nil {
		return id.ID
	}
	return ""
}

// focusShortTitle is for fallthrough acks when Live failed: name the open work
// without inventing a title from the user's latest sentence.
func (m *Manager) focusShortTitle(rc *runningChannel, in InboundMessage) string {
	if id := m.focusIdentity(rc, in); id != nil {
		return id.ShortTitle
	}
	return ""
}

func (m *Manager) focusIdentity(rc *runningChannel, in InboundMessage) *models.TaskIdentity {
	if m.taskContext == nil {
		return nil
	}
	scope := m.taskScopeFor(rc, in)
	focus, err := m.taskContext.GetFocus(scope, false)
	if err != nil || focus == nil {
		return nil
	}
	identity, err := m.taskContext.IdentityByID(focus.TaskIdentityID, rc.cfg.ProjectID)
	if err != nil || identity == nil || identity.TerminalAt != nil ||
		services.IsTerminalTaskStatus(identity.Status) {
		// Pointing at finished work makes the next follow-up refine a ghost.
		_ = m.taskContext.ExpireFocus(scope)
		return nil
	}
	return identity
}

func (m *Manager) taskScopeFor(rc *runningChannel, in InboundMessage) services.TaskScope {
	return services.TaskScope{
		ProjectID: rc.cfg.ProjectID, UserID: services.SyntheticQQUserID(in.UserID),
		Channel: rc.cfg.Type, ConversationID: in.ConversationID,
	}
}

// lastOutboundText is the most recent thing this conversation was told, so the
// director does not greet someone it just greeted.
func (m *Manager) lastOutboundText(rc *runningChannel, in InboundMessage) string {
	if m.transcript == nil {
		return ""
	}
	entries, err := m.transcript.Window(conversationRefFor(rc, in), 6)
	if err != nil {
		return ""
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Role == "assistant" {
			// Keep room for links / findings in the last report so follow-ups
			// can be answered from the prior outbound when get_status is thin.
			return truncateRunes(strings.TrimSpace(entries[i].Text), 400)
		}
	}
	return ""
}

// noteWorkProgress records what was just observed about a task so a later
// "how's it going" can be answered from fact.
func (m *Manager) noteWorkProgress(projectID, runID, stage string, blocked bool) {
	runID = strings.TrimSpace(runID)
	stage = strings.TrimSpace(ScrubInternalTerms(stage))
	if runID == "" || stage == "" {
		return
	}
	m.noteMu.Lock()
	defer m.noteMu.Unlock()
	if m.workNotes == nil {
		m.workNotes = map[string]workNote{}
	}
	m.workNotes[projectID+"|"+runID] = workNote{
		Stage: truncateRunes(stage, 120), Blocked: blocked, At: time.Now(),
	}
}

func (m *Manager) workNoteFor(projectID, runID string) (workNote, bool) {
	if strings.TrimSpace(runID) == "" {
		return workNote{}, false
	}
	m.noteMu.Lock()
	defer m.noteMu.Unlock()
	note, ok := m.workNotes[projectID+"|"+runID]
	return note, ok
}

// labelProgress names which task an update is about, but only when the answer
// is not obvious.
//
// With one task in flight the conversation already says what "还在跑" refers to,
// and a title in front of it reads like a ticket header. With two, an unlabelled
// update is genuinely ambiguous — the user has to guess which of the things
// they asked for just moved.
func (m *Manager) labelProgress(rc *runningChannel, in InboundMessage, ev ProgressEvent, text string) string {
	tasks := m.taskLedger(rc, in)
	if len(tasks) < 2 {
		return text
	}
	runID := strings.TrimSpace(ev.RunID)
	for _, t := range tasks {
		if t.RunID == runID && t.ShortTitle != "" {
			return "「" + t.ShortTitle + "」" + text
		}
	}
	return text
}

// statusResult is what get_status hands back to the model. It is JSON on
// purpose: a sentence would invite the model to copy it verbatim, and the point
// of this layer is that the model does the phrasing.
type statusResult struct {
	Tasks          []ledgerEntry `json:"tasks"`
	RecentTerminal []ledgerEntry `json:"recent_terminal,omitempty"`
	Note           string        `json:"note,omitempty"`
}

// runGetStatus answers "where is it" from the ledger, never from memory.
func (m *Manager) runGetStatus(rc *runningChannel, in InboundMessage, taskID string) string {
	tasks := m.taskLedger(rc, in)
	recent := m.recentTerminalLedger(rc, in)
	taskID = strings.TrimSpace(taskID)
	if taskID != "" {
		for _, t := range tasks {
			if t.TaskID == taskID || t.RunID == taskID {
				return encodeToolResult(statusResult{Tasks: []ledgerEntry{t}, RecentTerminal: recent})
			}
		}
		for _, t := range recent {
			if t.TaskID == taskID || t.RunID == taskID {
				note := "这个任务已经结束，状态见 recent_terminal，不要说成还在跑或笼统「做完了」。"
				if isFailedOrCancelledStatus(t.Status) {
					note += " " + failedStatusChoiceNote
				} else if strings.TrimSpace(t.ResultSummary) != "" {
					note += " " + deliveryFollowupNote
				}
				return encodeToolResult(statusResult{
					Tasks: nil, RecentTerminal: []ledgerEntry{t},
					Note: note,
				})
			}
		}
		return encodeToolResult(statusResult{
			Tasks: tasks, RecentTerminal: recent,
			Note: "没有这个 taskId；下面是在跑的和刚结束的。",
		})
	}
	res := statusResult{Tasks: tasks, RecentTerminal: recent}
	switch {
	case len(tasks) == 0 && len(recent) > 0:
		res.Note = "现在没有在跑的任务。刚结束的在 recent_terminal：必须按每条的 status 说（failed=失败，cancelled=取消，completed=完成）。禁止把空的在跑列表说成「都做完了」。"
		if hasFailedOrCancelled(recent) {
			res.Note += " " + failedStatusChoiceNote
		} else if hasCompletedWithSummary(recent) {
			res.Note += " " + deliveryFollowupNote
		}
	case len(tasks) == 0:
		res.Note = "现在没有在跑的任务，也没有刚结束的记录。如果用户问的事情还没开始，用 dispatch_pm 派下去；不要编造完成或失败。"
	default:
		if hasFailedOrCancelled(recent) {
			res.Note = failedStatusChoiceNote
		}
	}
	if m.IsConversationBusy(rc.cfg.ProjectID, in.Scene, in.ConversationID) {
		res.Note = strings.TrimSpace(res.Note + " 现在有一件事正在前台执行。")
	}
	return encodeToolResult(res)
}

// failedStatusChoiceNote steers the conversation model away from unilaterally
// restarting failed work. The user should pick the next step.
const failedStatusChoiceNote = "有失败或取消时：先如实说状况，再明确让对方选——重试、换范围/改方向、还是先搁置。不要擅自派活或说「我接着做」。"

// deliveryFollowupNote steers clarifying follow-ups after a completed delivery
// toward result_summary facts, instead of glossary answers that ignore them.
const deliveryFollowupNote = "对方若追问刚结束任务里的细节或交付物：用 result_summary 和上一条汇报作答；有就直接给，没有就如实说这轮没留下；禁止抛开摘要做名词百科。"

func hasFailedOrCancelled(entries []ledgerEntry) bool {
	for _, e := range entries {
		if isFailedOrCancelledStatus(e.Status) {
			return true
		}
	}
	return false
}

func hasCompletedWithSummary(entries []ledgerEntry) bool {
	for _, e := range entries {
		if strings.EqualFold(strings.TrimSpace(e.Status), "completed") &&
			strings.TrimSpace(e.ResultSummary) != "" {
			return true
		}
	}
	return false
}

func isFailedOrCancelledStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

// cancelResult reports what actually stopped.
type cancelResult struct {
	Cancelled  []string      `json:"cancelled,omitempty"`
	Ambiguous  []ledgerEntry `json:"ambiguous,omitempty"`
	NothingRun bool          `json:"nothing_running,omitempty"`
	Failed     string        `json:"failed,omitempty"`
}

// runCancelWork stops work the user has asked to stop.
//
// It refuses to guess. With two tasks in flight and no pointer to either, the
// result says so and the director asks which one — cancelling the wrong task is
// not a wording mistake that can be apologised for afterwards.
func (m *Manager) runCancelWork(ctx context.Context, rc *runningChannel, in InboundMessage, taskID string) string {
	tasks := m.taskLedger(rc, in)
	taskID = strings.TrimSpace(taskID)

	var target *ledgerEntry
	switch {
	case taskID != "":
		for i := range tasks {
			if tasks[i].TaskID == taskID || tasks[i].RunID == taskID {
				target = &tasks[i]
				break
			}
		}
	case len(tasks) == 1:
		target = &tasks[0]
	case len(tasks) > 1:
		if focus := m.focusTaskID(rc, in); focus != "" {
			for i := range tasks {
				if tasks[i].TaskID == focus {
					target = &tasks[i]
					break
				}
			}
		}
		if target == nil {
			return encodeToolResult(cancelResult{Ambiguous: tasks})
		}
	}

	// A foreground turn is work too, and it is the one the user is watching.
	stoppedForeground := m.cancelForegroundTurn(rc, in)

	if target == nil {
		if stoppedForeground {
			return encodeToolResult(cancelResult{Cancelled: []string{"正在处理的这件事"}})
		}
		return encodeToolResult(cancelResult{NothingRun: true})
	}
	if runID := strings.TrimSpace(target.RunID); runID != "" && m.riskExecutor != nil {
		if err := m.riskExecutor(rc.cfg.ProjectID, runID, "cancel_run", map[string]string{}); err != nil {
			log.Warn().Err(err).Str("run", runID).Msg("director cancel_work failed")
			return encodeToolResult(cancelResult{Failed: "没能停下来，状态没变。"})
		}
	}
	m.clearWorkNote(rc.cfg.ProjectID, target.RunID)
	m.retireTask(rc, in, target)
	_ = ctx
	return encodeToolResult(cancelResult{Cancelled: []string{target.ShortTitle}})
}

// retireTask writes the cancellation into the ledger and drops the focus if it
// pointed here.
//
// Without this the next "how's it going" reads a task that is still marked
// running, and the conversation layer — which is required to answer from the
// ledger — reports progress on work that was stopped minutes ago.
func (m *Manager) retireTask(rc *runningChannel, in InboundMessage, target *ledgerEntry) {
	if m.taskContext == nil || target == nil {
		return
	}
	scope := m.taskScopeFor(rc, in)
	focused := false
	if focus, err := m.taskContext.GetFocus(scope, false); err == nil && focus != nil {
		focused = focus.TaskIdentityID == target.TaskID
	}
	if _, err := m.taskContext.UpdateIdentity(services.EnsureTaskIdentityInput{
		RunID: target.RunID, ProjectID: rc.cfg.ProjectID, UserID: scope.UserID,
		Status: "cancelled",
	}); err != nil {
		log.Warn().Err(err).Str("task", target.TaskID).
			Msg("cancelled task still reads as running in the ledger")
	}
	if focused {
		if err := m.taskContext.ExpireFocus(scope); err != nil {
			log.Debug().Err(err).Msg("conversation focus still points at a cancelled task")
		}
	}
}

// completeEphemeralDispatch closes a dispatch:* ledger row when the foreground
// turn that created it is over. Real Runs are closed by ReflowTaskOutcome; the
// temporary dispatch key is not a Run, so without this every answered lookup
// stays in the "在跑的任务" list forever.
func (m *Manager) completeEphemeralDispatch(rc *runningChannel, in InboundMessage, status string) {
	if m.taskContext == nil || in.Dispatch == nil {
		return
	}
	taskID := strings.TrimSpace(in.Dispatch.TaskID)
	if taskID == "" {
		return
	}
	identity, err := m.taskContext.IdentityByID(taskID, rc.cfg.ProjectID)
	if err != nil || identity == nil {
		return
	}
	if !strings.HasPrefix(strings.TrimSpace(identity.RunID), "dispatch:") {
		return
	}
	if status = strings.TrimSpace(status); status == "" {
		status = "completed"
	}
	scope := m.taskScopeFor(rc, in)
	if _, err := m.taskContext.UpdateIdentity(services.EnsureTaskIdentityInput{
		RunID: identity.RunID, ProjectID: rc.cfg.ProjectID, UserID: scope.UserID,
		Status: status,
	}); err != nil {
		log.Debug().Err(err).Str("task", taskID).Msg("ephemeral dispatch not closed in the ledger")
	}
}

func (m *Manager) clearWorkNote(projectID, runID string) {
	m.noteMu.Lock()
	delete(m.workNotes, projectID+"|"+runID)
	m.noteMu.Unlock()
}

// cancelForegroundTurn stops an in-flight sandbox turn for this conversation.
func (m *Manager) cancelForegroundTurn(rc *runningChannel, in InboundMessage) bool {
	b, ok := m.transcript.(*ChannelBridge)
	if !ok || b == nil {
		return false
	}
	return b.CancelConversationTurn(conversationRefFor(rc, in))
}

func encodeToolResult(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"result could not be encoded"}`
	}
	return string(b)
}

// humanizeAge renders an age the way a colleague would say it.
func humanizeAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
}

// ensureTaskIdentity records a dispatched task in the ledger so the next "how's
// it going" has something real to read. A dispatch that cannot be recorded
// still runs: losing the ledger entry costs context, not the work.
func (m *Manager) ensureTaskIdentity(rc *runningChannel, in InboundMessage, d *WorkDispatch) {
	if m.taskContext == nil || d == nil {
		return
	}
	title := strings.TrimSpace(d.ShortTitle)
	if title == "" {
		title = truncateRunes(strings.TrimSpace(in.Text), 40)
	}
	if title == "" {
		return
	}
	scope := m.taskScopeFor(rc, in)
	identity, err := m.taskContext.EnsureIdentity(services.EnsureTaskIdentityInput{
		ProjectID: rc.cfg.ProjectID, UserID: scope.UserID,
		RunID:                dispatchLedgerRunID(rc, in),
		ShortTitle:           title,
		OriginalRequirement:  strings.TrimSpace(in.Text),
		RecentContext:        strings.TrimSpace(d.Brief),
		Status:               "running",
		OriginChannel:        rc.cfg.Type,
		OriginScene:          string(in.Scene),
		OriginConversationID: in.ConversationID,
		OriginExternalUserID: in.UserID,
		Language:             services.DetectLanguage(in.Text, ""),
	})
	if err != nil || identity == nil {
		log.Debug().Err(err).Str("project", rc.cfg.ProjectID).
			Msg("dispatched task not recorded in the ledger")
		return
	}
	d.TaskID = identity.ID
	if _, err := m.taskContext.SetFocus(scope, identity, identity.Language); err != nil {
		log.Debug().Err(err).Msg("conversation focus not updated for dispatched task")
	}
	if msgID := strings.TrimSpace(in.MessageID); msgID != "" {
		if err := m.taskContext.BindMessage(scope, msgID, identity); err != nil {
			log.Debug().Err(err).Msg("dispatched task not bound to the inbound message")
		}
	}
}

// dispatchLedgerRunID keys a dispatched task before a Run exists.
//
// A ledger row needs an identifier and a real Run id is not available until the
// agent starts one, so the turn itself is the key. It is replaced by the Run's
// own identity once pm_start_run creates it.
func dispatchLedgerRunID(rc *runningChannel, in InboundMessage) string {
	if id := strings.TrimSpace(in.RecordedMessageID); id != "" {
		return "dispatch:" + id
	}
	return fmt.Sprintf("dispatch:%s:%d",
		convKey(rc.cfg.ProjectID, in.Scene, in.ConversationID), time.Now().UnixNano())
}
