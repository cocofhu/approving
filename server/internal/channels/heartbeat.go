package channels

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"
	"github.com/rs/zerolog/log"
)

// RunHeartbeat is a long-running Run the platform decided to speak about,
// carrying only what the engine could see for itself.
type RunHeartbeat struct {
	ProjectID  string
	RunID      string
	NodeLabel  string
	RunningFor time.Duration
}

// DefaultHeartbeatInterval is how long a task may go without the platform
// saying anything.
//
// Half an hour is a compromise between the two ways this gets annoying. Much
// shorter and a normal task produces a stream of "还在跑" that people learn to
// ignore, at which point the real update is ignored too. Much longer and the
// silence is indistinguishable from a hang, which is the thing this exists to
// fix.
const DefaultHeartbeatInterval = 30 * time.Minute

// SetHeartbeatInterval sets the minimum gap between volunteered updates about
// one task. Zero switches them off.
func (m *Manager) SetHeartbeatInterval(d time.Duration) {
	m.mu.Lock()
	m.heartbeatInterval = d
	m.mu.Unlock()
}

func (m *Manager) heartbeatEvery() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.heartbeatInterval
}

// ReportRunHeartbeat tells the origin conversation that its task is still
// going, when enough time has passed that saying nothing would read as a hang.
//
// The facts are assembled in two layers. The engine supplies what it can see —
// which node, for how long — and everything about who is waiting, what they
// asked for, and what was last reported comes from the task ledger here. That
// split is what keeps the engine from needing to know about conversations, and
// keeps this from having to guess at run internals.
func (m *Manager) ReportRunHeartbeat(ctx context.Context, hb RunHeartbeat) error {
	every := m.heartbeatEvery()
	if every <= 0 {
		return nil
	}
	runID := strings.TrimSpace(hb.RunID)
	if runID == "" {
		return nil
	}
	if ctx == nil {
		ctx = m.baseCtx
	}
	origin := m.resolveRunOrigin(hb.ProjectID, runID)
	identity := origin.Identity
	if identity == nil || !origin.Speaks() {
		return nil
	}
	// A run the engine still lists as running can already be settled in the
	// ledger — a cancel, say. Volunteering an update about it would reopen
	// something the user has already been told is over.
	if services.IsTerminalTaskStatus(identity.Status) {
		return nil
	}
	if !heartbeatDue(identity, every) {
		return nil
	}
	// Marked before delivery, not after. A conversation that is busy or rate
	// limited suppresses the message, and treating that as "not yet reported"
	// would retry on every sweep and then deliver a burst the moment it opened
	// up — the exact spam this interval exists to prevent.
	if err := m.taskContext.MarkHeartbeat(identity.ProjectID, runID); err != nil {
		log.Warn().Err(err).Str("run", runID).Msg("heartbeat not marked; skipping to avoid a burst later")
		return err
	}

	traceID := m.traceIDForReflow(identity)
	language := taskLanguage(identity)
	text := m.synthesizeForTask(ctx, identity, origin.Scene, origin.Conv, traceID,
		heartbeatBrief(identity, hb, language), heartbeatFallbackText(identity, hb, language))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: identity.ProjectID, Scene: origin.Scene, ConversationID: origin.Conv,
		UserID: identity.OriginExternalUserID, RunID: runID,
		TraceID: traceID,
		Kind:    sendable.KindProgress, Reason: ReasonRunHeartbeat,
		Priority: sendable.PriorityNormal,
		// The egress policy refuses a progress message with nothing in it, and
		// it is right to: an update that carries no fact is noise. "Still
		// running" is the fact here, so it is stated rather than left blank.
		Progress: sendable.ProgressFields{Stage: heartbeatStage(identity, hb)},
		// Bucketed by interval so a duplicate sweep in the same window
		// collapses, while the next window's update is a different message.
		DedupeKey: strings.Join([]string{
			"run-heartbeat", runID, strconv.FormatInt(time.Now().Unix()/int64(every/time.Second), 10),
		}, ":"),
		Text: text,
	})
	return err
}

// heartbeatStage is the fact this update carries, for the egress policy's
// digest rather than for the user. Its wording never reaches anyone.
func heartbeatStage(identity *models.TaskIdentity, hb RunHeartbeat) string {
	for _, candidate := range []string{identity.LastStage, hb.NodeLabel} {
		if s := strings.TrimSpace(candidate); s != "" {
			return s
		}
	}
	return "still running"
}

// heartbeatDue answers whether enough time has passed. A task that has never
// been spoken about since it started is measured from how long it has been
// running, which is what the engine already told us.
func heartbeatDue(identity *models.TaskIdentity, every time.Duration) bool {
	if identity.LastHeartbeatAt != nil {
		return time.Since(*identity.LastHeartbeatAt) >= every
	}
	// Anything the task itself reported counts as having been heard from, so a
	// chatty task is not immediately followed by the platform saying the same
	// thing in its own words.
	if identity.LastStageAt != nil && time.Since(*identity.LastStageAt) < every {
		return false
	}
	return true
}

// heartbeatBrief states what the conversation layer may say. The one thing it
// must not do is imply the task is finished or stuck.
func heartbeatBrief(identity *models.TaskIdentity, hb RunHeartbeat, language string) string {
	var b strings.Builder
	b.WriteString("后台任务还在跑，已经跑了一阵子了，主动跟用户说一句免得他以为卡死了。用一句话，别写成汇报。\n")
	if title := services.SanitizeShortTitle(identity.ShortTitle); title != "" {
		b.WriteString("任务：" + title + "\n")
	}
	b.WriteString("已经跑了：" + humanizeAge(hb.RunningFor) + "\n")
	if stage := strings.TrimSpace(identity.LastStage); stage != "" {
		b.WriteString("执行方最后报的是：" + truncateRunes(stage, 120) + "\n")
	} else if label := strings.TrimSpace(hb.NodeLabel); label != "" {
		b.WriteString("现在在这一步：" + truncateRunes(label, 60) + "\n")
	} else {
		b.WriteString("没有更细的进展可说。就说还在跑，别编具体在做什么。\n")
	}
	b.WriteString("要求：说人话，像同事路过顺口说一句；不要出现任务编号、工作流名、节点名、执行环境、工具名；")
	b.WriteString("不要说「请前往 Approving 查看」；不要说已经完成或失败；不要问对方要不要继续——没人要求你确认。")
	if services.NormalizeLanguage(language) == "en" {
		b.WriteString("\n用英文回答。")
	}
	return b.String()
}

// heartbeatFallbackText is what goes out when phrasing is unavailable. It has
// to stand on its own and must not read like a status template.
func heartbeatFallbackText(identity *models.TaskIdentity, hb RunHeartbeat, language string) string {
	en := services.NormalizeLanguage(language) == "en"
	title := services.SanitizeShortTitle(identity.ShortTitle)
	detail := strings.TrimSpace(identity.LastStage)
	if detail == "" {
		detail = strings.TrimSpace(hb.NodeLabel)
	}
	if en {
		subject := "that one"
		if title != "" {
			subject = "\"" + title + "\""
		}
		if detail != "" {
			return "Still on " + subject + " — currently " + truncateRunes(detail, 60) + ". I'll come back when it's done."
		}
		return "Still working on " + subject + ", nothing to report yet. I'll come back when it's done."
	}
	subject := "刚才那件事"
	if title != "" {
		subject = title
	}
	if detail != "" {
		return subject + "还在跑，现在在" + truncateRunes(detail, 60) + "，完了我告诉你。"
	}
	return subject + "还在跑，暂时没什么可说的，完了我告诉你。"
}
