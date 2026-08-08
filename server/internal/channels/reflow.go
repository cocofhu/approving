package channels

import (
	"context"
	"errors"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// This file brings a background task's outcome back into the conversation that
// asked for it.
//
// The important part is that the outcome is *rewritten for that conversation*
// before it is sent. Pushing the raw event out as a template is what produced
// 「本回合已结束，请在 Approving 查看完整结果。」— technically a notification, but
// it tells the user nothing and makes them go somewhere else to find out what
// happened. So the event goes to the conversation's own agent first, which
// already holds the context of what was asked, and it answers in that context.
//
// When the agent cannot be reached the message still goes out, as a plain
// statement of what happened and what to do next. Degrading is acceptable;
// dropping the outcome is not, because the user is waiting for it.

// TaskOutcome is a terminal Run result on its way back to a conversation.
type TaskOutcome struct {
	ProjectID string
	RunID     string
	// Status is completed / failed / cancelled.
	Status string
	// FailureReason is the aggregated cause for a failed run.
	FailureReason string
	// ResultSummary is a short digest of what the run produced (findings /
	// summary artifact). Used by both synthesis and the structured fallback so
	// a completed task never reports as an empty "弄完了".
	ResultSummary string
}

// ReflowTaskOutcome delivers a finished task's outcome to its origin
// conversation and records the terminal status on the task, so later questions
// like "how's that one going?" stop answering "in progress".
func (m *Manager) ReflowTaskOutcome(ctx context.Context, outcome TaskOutcome) error {
	runID := strings.TrimSpace(outcome.RunID)
	if runID == "" {
		return errors.New("task outcome requires a real run id")
	}
	if ctx == nil {
		ctx = m.baseCtx
	}
	identity := m.identityForRun(runID, outcome.ProjectID)
	if identity == nil {
		// No task identity means this Run was never started from a
		// conversation; there is nobody waiting on it here.
		return nil
	}
	m.syncTerminalStatus(identity, outcome)

	conv := strings.TrimSpace(identity.OriginConversationID)
	if conv == "" {
		log.Debug().Str("run", runID).Msg("reflow: task has no origin conversation, skipping")
		return nil
	}
	scene := Scene(strings.TrimSpace(identity.OriginScene))
	if scene == "" {
		scene = SceneC2C
	}

	text := m.synthesizeOutcome(ctx, identity, outcome, scene, conv)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: identity.ProjectID, Scene: scene, ConversationID: conv,
		UserID: identity.OriginExternalUserID, RunID: runID,
		Kind: sendable.KindFinal, Reason: "task_outcome",
		Priority: sendable.PriorityCritical,
		// One conclusion per run per conversation, whatever path produced it.
		DedupeKey: strings.Join([]string{"task-outcome", runID, conv}, ":"),
		Text:      text,
	})
	return err
}

// synthesizeOutcome asks the conversation's agent to phrase the outcome in
// context, and falls back to a self-contained statement if that is not
// possible. The fallback is written to be a real answer on its own — it names
// the task, says what happened, and says what comes next — because it is what
// the user actually receives whenever the agent is busy or unavailable.
func (m *Manager) synthesizeOutcome(ctx context.Context, identity *models.TaskIdentity,
	outcome TaskOutcome, scene Scene, conv string) string {
	language := taskLanguage(identity)
	fallback := outcomeFallbackText(identity, outcome, language)
	if m.synthesize == nil {
		return fallback
	}
	// Phrasing happens in the conversation layer, which is not the layer doing
	// the work. That is what lets an outcome be reported while the agent is
	// still busy: synthesis used to borrow the conversation's own agent, so a
	// conversation with a turn in flight got the template instead of a sentence.
	req := SynthesisRequest{
		ProjectID: identity.ProjectID, Scene: scene, ConversationID: conv,
		ExternalUserID: identity.OriginExternalUserID,
		RunID:          identity.RunID, ShortTitle: identity.ShortTitle,
		Language: language, Fallback: fallback,
		Brief: outcomeBrief(identity, outcome, language),
	}
	text, err := m.synthesize(ctx, req)
	if strings.TrimSpace(text) == "" {
		log.Info().Err(err).Str("run", identity.RunID).
			Msg("reflow: natural-language synthesis unavailable, using structured fallback")
		return fallback
	}
	return ScrubInternalTerms(text)
}

// SynthesisRequest asks the conversation's agent to phrase a background event
// for that conversation. Only structured fields are provided — never raw tool
// output or reasoning text — so synthesis cannot become a channel for internal
// content to escape.
type SynthesisRequest struct {
	ProjectID      string
	Scene          Scene
	ConversationID string
	ExternalUserID string
	RunID          string
	ShortTitle     string
	Language       string
	// Brief is the structured description of what happened.
	Brief string
	// Fallback is what will be sent if synthesis does not produce anything.
	Fallback string
}

// SynthesisFunc rewrites a background event as conversational text.
type SynthesisFunc func(ctx context.Context, req SynthesisRequest) (string, error)

// SetSynthesizer wires natural-language reflow (nil keeps structured fallbacks).
func (m *Manager) SetSynthesizer(fn SynthesisFunc) { m.synthesize = fn }

// captureAgentReplies diverts this conversation's agent replies into a buffer
// instead of the channel, and returns a function that ends the capture and
// yields whatever was collected.
//
// This is what makes one voice possible. The agent answers through pm_reply,
// which used to go straight to the channel — so the user heard the work layer
// directly, in its own register, sometimes twice in a turn, and the
// conversation layer had no idea what had been said. Capturing turns that
// answer into an internal result the conversation layer can report.
//
// It reports false when the conversation is already under capture, because two
// captures would split one answer between them.
func (m *Manager) captureAgentReplies(projectID string, scene Scene, conv string) (take func() string, ok bool) {
	key := convKey(projectID, scene, conv)
	m.captureMu.Lock()
	if m.captured == nil {
		m.captured = map[string]*string{}
	}
	if _, exists := m.captured[key]; exists {
		m.captureMu.Unlock()
		return nil, false
	}
	buf := new(string)
	m.captured[key] = buf
	m.captureMu.Unlock()

	return func() string {
		m.captureMu.Lock()
		defer m.captureMu.Unlock()
		delete(m.captured, key)
		return *buf
	}, true
}

// captureReply records an agent reply when the conversation is under capture.
func (m *Manager) captureReply(projectID string, scene Scene, conv, text string) bool {
	m.captureMu.Lock()
	defer m.captureMu.Unlock()
	buf, ok := m.captured[convKey(projectID, scene, conv)]
	if !ok {
		return false
	}
	if *buf == "" {
		*buf = text
	} else {
		*buf += "\n" + text
	}
	return true
}

// outcomeBrief states the facts the agent may use. It is deliberately terse and
// structured: the agent's job is to phrase it for this conversation, not to
// discover new information.
func outcomeBrief(identity *models.TaskIdentity, outcome TaskOutcome, language string) string {
	var b strings.Builder
	b.WriteString("后台任务已结束，请用一到两句话把结果告诉用户。\n")
	if title := services.SanitizeShortTitle(identity.ShortTitle); title != "" {
		b.WriteString("任务：" + title + "\n")
	}
	if req := strings.TrimSpace(identity.OriginalRequirement); req != "" {
		b.WriteString("用户当初的要求：" + truncateRunes(req, 200) + "\n")
	}
	if recent := strings.TrimSpace(identity.RecentContext); recent != "" {
		b.WriteString("最近一次相关对话：" + truncateRunes(recent, 200) + "\n")
	}
	b.WriteString("不要把截断的任务名或半截英文词贴进回复；有关键发现就直接讲发现。\n")
	switch strings.ToLower(strings.TrimSpace(outcome.Status)) {
	case "completed":
		b.WriteString("结果：做完了。\n")
		if facts := strings.TrimSpace(outcome.ResultSummary); facts != "" {
			b.WriteString("关键发现（必须写进回复，写出具体点，不要藏到「想看细节」后面；禁止只说「有办法 / 可以精简」这种空结论）：\n")
			b.WriteString(truncateRunes(facts, 800) + "\n")
		} else {
			b.WriteString("这一轮没有留下可读结论摘要。如实说做完了但还没整理出要点，问对方要不要接着补查；")
			b.WriteString("禁止编造「确实有办法 / 可以精简」这类没有依据的实质结论；")
			b.WriteString("禁止空说「弄完了，想看细节跟我说」。\n")
		}
	case "cancelled":
		b.WriteString("结果：被取消了。\n")
	default:
		b.WriteString("结果：没做成。\n")
		if reason := strings.TrimSpace(outcome.FailureReason); reason != "" {
			b.WriteString("原因（请翻译成用户能懂的话，不要照抄）：" + truncateRunes(reason, 200) + "\n")
		}
	}
	b.WriteString("要求：说人话，像同事汇报一样；先给结论再带关键发现；不要出现任务编号、工作流名、执行环境、工具名；")
	b.WriteString("不要说「请前往 Approving 查看」；不要把实质内容推到下一轮；结论要能独立看懂。")
	if services.NormalizeLanguage(language) == "en" {
		b.WriteString("\n用英文回答。")
	}
	return b.String()
}

// outcomeFallbackText is the self-contained version sent when synthesis is
// unavailable. It never tells the user to go look somewhere else, and it must
// carry ResultSummary when present — an empty "弄完了" is what produced the
// hollow IM replies this path exists to avoid.
func outcomeFallbackText(identity *models.TaskIdentity, outcome TaskOutcome, language string) string {
	title := services.SanitizeShortTitle(identity.ShortTitle)
	en := services.NormalizeLanguage(language) == "en"
	facts := ScrubInternalTerms(strings.TrimSpace(outcome.ResultSummary))
	// Keep the same budget as fireRunTerminal's digest — a second hard cut at
	// 400 is what produced mid-word endings like「findi…」in IM.
	facts = truncateRunes(facts, 800)
	switch strings.ToLower(strings.TrimSpace(outcome.Status)) {
	case "completed":
		return completedOutcomeFallback(title, facts, en)
	case "cancelled":
		if en {
			if title == "" {
				return "That one's been cancelled, so I've stopped work on it. Tell me if you want it picked back up."
			}
			return "\"" + title + "\" has been cancelled, so I've stopped work on it. Tell me if you want it picked back up."
		}
		if title == "" {
			return "刚才那个取消了，我停下了。要重新做的话说一声。"
		}
		return title + "取消了，我停下了。要重新做的话说一声。"
	default:
		reason := humanizeFailureReason(outcome.FailureReason, en)
		if en {
			if title == "" {
				return "That one didn't go through: " + reason + " Want me to retry, change the approach, or leave it for now?"
			}
			return "\"" + title + "\" didn't go through: " + reason + " Want me to retry, change the approach, or leave it for now?"
		}
		if title == "" {
			return "刚才那个没做成：" + reason + "你看是重试、换个做法，还是先搁置？"
		}
		return title + "没做成：" + reason + "你看是重试、换个做法，还是先搁置？"
	}
}

func completedOutcomeFallback(title, facts string, en bool) string {
	facts = strings.TrimSpace(facts)
	if facts != "" {
		// Findings already carry the substance. Gluing ShortTitle produced
		// 「调研 … 和 wo弄完了」when the ledger title had been mid-token cut.
		if en {
			return "Done. " + facts
		}
		return "弄完了。" + facts
	}
	if en {
		if title == "" {
			return "That one's done, but this turn didn't leave a readable summary. Want me to dig in again?"
		}
		return "\"" + title + "\" is done, but this turn didn't leave a readable summary. Want me to dig in again?"
	}
	if title == "" {
		return "刚才那个弄完了，但这一轮没留下可读结论。要我接着补查可以说。"
	}
	return title + "弄完了，但这一轮没留下可读结论。要我接着补查可以说。"
}

// humanizeFailureReason turns an aggregated failure cause into something a user
// can act on. The raw reason is diagnostic text and often mentions internals.
func humanizeFailureReason(reason string, en bool) string {
	lower := strings.ToLower(strings.TrimSpace(reason))
	switch {
	case lower == "":
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "超时"),
		strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "timed out"):
		if en {
			return "it ran out of time."
		}
		return "跑太久超时了。"
	case strings.Contains(lower, "permission"), strings.Contains(lower, "权限"),
		strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"):
		if en {
			return "it didn't have the access it needed."
		}
		return "权限不够。"
	case strings.Contains(lower, "sandbox"), strings.Contains(lower, "沙箱"):
		if en {
			return "the execution environment couldn't start."
		}
		return "执行环境没起来。"
	case strings.Contains(lower, "network"), strings.Contains(lower, "connection"),
		strings.Contains(lower, "网络"):
		if en {
			return "a network call kept failing."
		}
		return "网络一直连不上。"
	}
	if en {
		return "something went wrong partway through."
	}
	return "中途出了问题。"
}

// syncTerminalStatus writes the outcome onto the task so status questions stop
// answering "in progress" forever. Without this the task table is the only
// place a user's question is answered from, and it would never be updated.
//
// For completed work, ResultSummary (findings / delivery URLs) is persisted as
// RecentContext so get_status / briefing can answer follow-ups from facts.
func (m *Manager) syncTerminalStatus(identity *models.TaskIdentity, outcome TaskOutcome) {
	if m.taskContext == nil || identity == nil {
		return
	}
	in := services.EnsureTaskIdentityInput{
		RunID: identity.RunID, ProjectID: identity.ProjectID, UserID: identity.UserID,
		Status: strings.TrimSpace(outcome.Status),
	}
	if strings.EqualFold(strings.TrimSpace(outcome.Status), "completed") {
		if summary := strings.TrimSpace(outcome.ResultSummary); summary != "" {
			in.RecentContext = summary
		}
	}
	if _, err := m.taskContext.UpdateIdentity(in); err != nil {
		log.Warn().Err(err).Str("run", identity.RunID).
			Msg("reflow: writing terminal task status failed")
	}
}

func (m *Manager) identityForRun(runID, projectID string) *models.TaskIdentity {
	if m.taskContext == nil {
		return nil
	}
	identity, err := m.taskContext.IdentityForRun(runID, projectID)
	if err != nil {
		log.Warn().Err(err).Str("run", runID).Msg("reflow: loading task identity failed")
		return nil
	}
	return identity
}

// taskLanguage returns the language the task has been conducted in. Following
// the task rather than the individual message keeps a conversation from
// switching languages mid-task just because one update happened to be phrased
// differently.
func taskLanguage(identity *models.TaskIdentity) string {
	if identity == nil {
		return ""
	}
	if lang := services.NormalizeLanguage(identity.Language); lang != "" {
		return lang
	}
	return services.DetectLanguage(identity.OriginalRequirement, "")
}
