package channels

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

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

	traceID := m.traceIDForReflow(identity)
	text := m.synthesizeOutcome(ctx, identity, outcome, scene, conv, traceID)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: identity.ProjectID, Scene: scene, ConversationID: conv,
		UserID: identity.OriginExternalUserID, RunID: runID,
		TraceID: traceID,
		Kind:    sendable.KindFinal, Reason: "task_outcome",
		Priority: sendable.PriorityCritical,
		// One conclusion per run per conversation, whatever path produced it.
		DedupeKey: strings.Join([]string{"task-outcome", runID, conv}, ":"),
		Text:      text,
	})
	return err
}

// TaskPause is a Run that has stopped and cannot continue without a person, on
// its way back to the conversation that asked for the work.
type TaskPause struct {
	ProjectID string
	RunID     string
	NodeID    string
	Iteration int
	// Ask is what the run stopped to hear, as the node phrased it. It is raw
	// working-layer output, so it is filtered before any of it is quoted.
	Ask string
}

// ReflowTaskPaused tells the origin conversation that its task has stopped and
// is waiting on a person, and records that state on the task itself.
//
// Both halves matter. Without the message the user hears nothing at all between
// dispatch and completion, so a run that stops for input is indistinguishable
// from one that hung. Without the status write the ledger keeps reporting the
// status the task had when it was created, which is how a run that had been
// waiting for a human for half an hour still read as queued.
func (m *Manager) ReflowTaskPaused(ctx context.Context, pause TaskPause) error {
	runID := strings.TrimSpace(pause.RunID)
	if runID == "" {
		return errors.New("task pause requires a real run id")
	}
	if ctx == nil {
		ctx = m.baseCtx
	}
	identity := m.identityForRun(runID, pause.ProjectID)
	if identity == nil {
		// Nobody dispatched this from a conversation; the project-level notify
		// is the only audience it has.
		return nil
	}
	// A pause event that arrives after the run already ended is stale. Acting
	// on it would reopen a settled task and ask the user about work that is
	// over.
	if services.IsTerminalTaskStatus(identity.Status) {
		return nil
	}
	m.markTaskWaitingHuman(identity)

	conv := strings.TrimSpace(identity.OriginConversationID)
	if conv == "" {
		log.Debug().Str("run", runID).Msg("reflow: paused task has no origin conversation, skipping")
		return nil
	}
	scene := Scene(strings.TrimSpace(identity.OriginScene))
	if scene == "" {
		scene = SceneC2C
	}

	traceID := m.traceIDForReflow(identity)
	language := taskLanguage(identity)
	text := m.synthesizeForTask(ctx, identity, scene, conv, traceID,
		pauseBrief(identity, pause, language),
		pauseFallbackText(identity, pause, language))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: identity.ProjectID, Scene: scene, ConversationID: conv,
		UserID: identity.OriginExternalUserID, RunID: runID,
		TraceID: traceID,
		Kind:    sendable.KindActionRequired, Reason: "task_paused",
		Priority: sendable.PriorityCritical,
		// One nudge per pause. A run can stop at several nodes, and the same
		// node can pause again on a later iteration, so both are in the key —
		// otherwise the second stop would be silently deduped away.
		DedupeKey: strings.Join([]string{
			"task-paused", runID, pause.NodeID, strconv.Itoa(pause.Iteration), conv,
		}, ":"),
		Text: text,
	})
	return err
}

// markTaskWaitingHuman records the pause on the task ledger so status questions
// stop answering with whatever the run's status happened to be at creation.
func (m *Manager) markTaskWaitingHuman(identity *models.TaskIdentity) {
	if m.taskContext == nil || identity == nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(identity.Status), taskStatusWaitingHuman) {
		return
	}
	_, err := m.taskContext.UpdateIdentity(services.EnsureTaskIdentityInput{
		RunID: identity.RunID, ProjectID: identity.ProjectID, UserID: identity.UserID,
		Status: taskStatusWaitingHuman,
	})
	if err != nil {
		log.Warn().Err(err).Str("run", identity.RunID).
			Msg("reflow: writing waiting_human task status failed")
	}
}

// taskStatusWaitingHuman matches the Run status vocabulary so the ledger and
// the run list read the same way.
const taskStatusWaitingHuman = "waiting_human"

// pauseAskRunes bounds how much of the node's own words may be quoted. The
// pause detail is written by the working agent for the platform, so the same
// per-sentence filter as a findings digest applies: quote the part that is a
// question to the user, drop the part that is a work log.
const pauseAskRunes = 160

// pauseBrief states what the conversation's agent may say. The task has not
// ended, so the one thing the message must do is make clear that it is the
// user's turn now.
func pauseBrief(identity *models.TaskIdentity, pause TaskPause, language string) string {
	var b strings.Builder
	b.WriteString("后台任务停下来了，需要用户拍板才能继续，请用一到两句话说清楚。\n")
	if title := services.SanitizeShortTitle(identity.ShortTitle); title != "" {
		b.WriteString("任务：" + title + "\n")
	}
	if req := strings.TrimSpace(identity.OriginalRequirement); req != "" {
		b.WriteString("用户当初的要求：" + truncateRunes(req, 200) + "\n")
	}
	if ask := leadingConclusion(pause.Ask, pauseAskRunes); ask != "" {
		b.WriteString("停下来是想问：" + ask + "\n")
	} else {
		b.WriteString("这一轮没有留下可读的提问内容。如实说需要对方确认才能继续，问对方要怎么走；禁止编造具体选项。\n")
	}
	b.WriteString("要求：说人话，像同事当面问一句；先说卡在哪需要什么，再问对方怎么定；")
	b.WriteString("不要出现任务编号、工作流名、节点名、执行环境、工具名；不要说「请前往 Approving 查看」；")
	b.WriteString("不要说任务已经完成或失败——它还在等。")
	if services.NormalizeLanguage(language) == "en" {
		b.WriteString("\n用英文回答。")
	}
	return b.String()
}

// pauseFallbackText is what goes out when phrasing is unavailable. Like the
// outcome fallback it has to stand on its own, and it must not send the user
// somewhere else to find out what is being asked.
func pauseFallbackText(identity *models.TaskIdentity, pause TaskPause, language string) string {
	en := services.NormalizeLanguage(language) == "en"
	title := services.SanitizeShortTitle(identity.ShortTitle)
	subject := title
	if subject == "" {
		if en {
			subject = "That one"
		} else {
			subject = "刚才那件事"
		}
	} else if en {
		subject = "\"" + title + "\""
	}
	ask := leadingConclusion(pause.Ask, pauseAskRunes)
	if ask != "" {
		if en {
			return subject + " is waiting on you: " + ask + " Tell me how you want to go and I'll carry on."
		}
		return subject + "停下来等你拿主意：" + ask + "你说怎么走，我就接着做。"
	}
	if en {
		return subject + " has stopped and needs your call before it can go further. Tell me how you want to handle it."
	}
	return subject + "做到一半停下了，得你确认才能往下走。你说说想怎么处理。"
}

// traceIDForReflow joins a terminal outcome to the inbound turn that dispatched
// it via the write-once OriginTraceID on TaskIdentity. An empty result is fine —
// delivery still proceeds without a span join (including historical rows that
// predate OriginTraceID persistence).
func (m *Manager) traceIDForReflow(identity *models.TaskIdentity) string {
	if identity == nil {
		return ""
	}
	return strings.TrimSpace(identity.OriginTraceID)
}

// synthesizeOutcome asks the conversation's agent to phrase the outcome in
// context, and falls back to a self-contained statement if that is not
// possible. The fallback is written to be a real answer on its own — it names
// the task, says what happened, and says what comes next — because it is what
// the user actually receives whenever the agent is busy or unavailable.
func (m *Manager) synthesizeOutcome(ctx context.Context, identity *models.TaskIdentity,
	outcome TaskOutcome, scene Scene, conv, traceID string) string {
	language := taskLanguage(identity)
	return m.synthesizeForTask(ctx, identity, scene, conv, traceID,
		outcomeBrief(identity, outcome, language),
		outcomeFallbackText(identity, outcome, language))
}

// synthesizeForTask is the shared phrasing path: hand the conversation layer a
// structured brief and the message that goes out if phrasing is unavailable.
func (m *Manager) synthesizeForTask(ctx context.Context, identity *models.TaskIdentity,
	scene Scene, conv, traceID, brief, fallback string) string {
	started := time.Now()
	language := taskLanguage(identity)
	status, detail := "ok", "synthesized"
	text := ""
	if m.synthesize == nil {
		status, detail = "fallback", "no_synthesizer"
		text = fallback
	} else {
		// Phrasing happens in the conversation layer, which is not the layer doing
		// the work. That is what lets an outcome be reported while the agent is
		// still busy: synthesis used to borrow the conversation's own agent, so a
		// conversation with a turn in flight got the template instead of a sentence.
		req := SynthesisRequest{
			ProjectID: identity.ProjectID, Scene: scene, ConversationID: conv,
			ExternalUserID: identity.OriginExternalUserID,
			RunID:          identity.RunID, ShortTitle: identity.ShortTitle,
			Language: language, Fallback: fallback,
			Brief: brief,
		}
		var err error
		text, err = m.synthesize(ctx, req)
		if strings.TrimSpace(text) == "" {
			log.Info().Err(err).Str("run", identity.RunID).
				Msg("reflow: natural-language synthesis unavailable, using structured fallback")
			status, detail = "fallback", "unavailable"
			text = fallback
		}
	}
	m.appendTraceSpan(traceID, finishSpan("synthesis", status, detail, started))
	// Pass through; sendOutboundResult is the sole ScrubForOutbound gate.
	return text
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
	// The digest goes to completedOutcomeFallback unscrubbed on purpose: it
	// chooses which sentences to quote by asking whether each one is clean, and
	// a pre-scrubbed digest looks clean everywhere. Outbound scrubbing happens
	// only in sendOutboundResult.
	facts := strings.TrimSpace(outcome.ResultSummary)
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

// outcomeFallbackFactsRunes bounds what the degraded path is allowed to quote.
// ResultSummary is written by the working agent for the platform, not for the
// user: a full one is a report with commit hashes, module names and headings.
// Quoting its opening conclusion is useful; pasting the whole thing is how a
// finished task arrived in QQ as 「弄完了。对照…基线（git: 90713d62 Merge #177）…」.
const outcomeFallbackFactsRunes = 160

// deliveryLine matches the link row the platform itself appends to a digest
// (services.AppendRunDeliveryURL). Lifting it out is what lets the link end the
// message instead of trailing a wall of text nobody read that far into.
var deliveryLine = regexp.MustCompile(`(?m)^[^\S\n]*(?:交付链接|Delivery)[：:][^\S\n]*(\S+)[^\S\n]*$`)

func completedOutcomeFallback(title, facts string, en bool) string {
	link := ""
	if m := deliveryLine.FindStringSubmatch(facts); m != nil {
		link = m[1]
		facts = deliveryLine.ReplaceAllString(facts, "")
	}
	// leadingConclusion already drops sentences with internal terms; final
	// ScrubForOutbound runs in sendOutboundResult.
	body := leadingConclusion(facts, outcomeFallbackFactsRunes)
	// The task has to be named, first thing. Several jobs can be in flight at
	// once, and two pushes that both open with 「弄完了」 are indistinguishable
	// in a chat window — which is the state this message arrived in.
	subject := title
	if subject == "" {
		if en {
			subject = "That one"
		} else {
			subject = "刚才那件事"
		}
	} else if en {
		subject = "\"" + title + "\""
	}

	switch {
	case body != "" && link != "":
		if en {
			return subject + " is done: " + body + " The change is at " + link
		}
		return subject + "跑完了：" + body + "改动在 " + link
	case body != "":
		if en {
			return subject + " is done: " + body
		}
		return subject + "跑完了：" + body
	case link != "":
		// Nothing in the digest was worth quoting, but the link is the whole
		// point of the task and is enough on its own.
		if en {
			return subject + " is done — the change is at " + link
		}
		return subject + "跑完了，改动在 " + link
	}
	if en {
		return subject + " is done, but this turn didn't leave a readable summary. Want me to dig in again?"
	}
	return subject + "跑完了，但这一轮没留下可读结论。要我接着补查可以说。"
}

// leadingConclusion picks the part of a findings digest that can be said out
// loud, up to limit runes.
//
// Selection is per sentence, and a sentence that carries internal vocabulary is
// dropped whole rather than cleaned. Scrubbing words out of it leaves a hole —
// 「对照基线（git: 90713d62 Merge #177），之后到 HEAD（652b0d68）只有文案改动」
// becomes 「对照基线，之后到 只有文案改动」, which is not better, just broken.
// The sentence next to it — 「原架构判断大体仍成立，但清单需要局部修订」— is the
// conclusion, and it was already fine.
//
// Everything clean is dropped only for length, and then whole sentences at a
// time, because the point of this path is that it still reads like a finished
// thought. An empty result is honest: this digest was written for the platform,
// and the caller says so instead of quoting it.
func leadingConclusion(facts string, limit int) string {
	facts = strings.TrimSpace(facts)
	if facts == "" || limit <= 0 {
		return ""
	}
	var kept strings.Builder
	for _, sentence := range splitSentences(facts) {
		if ContainsInternalTerms(sentence) || readsLikeWorkLog(sentence) {
			continue
		}
		if kept.Len() == 0 && len([]rune(sentence)) > limit {
			// A single clean sentence past the budget: shortened beats absent.
			return services.SoftTruncateRunes(strings.TrimSpace(sentence), limit)
		}
		if kept.Len() > 0 && len([]rune(kept.String()))+len([]rune(sentence)) > limit {
			break
		}
		kept.WriteString(sentence)
	}
	return strings.TrimSpace(kept.String())
}

// urlSpan matches a link so it can be lifted out of a digest, or excluded when
// judging whether the prose around it is a work log.
var urlSpan = regexp.MustCompile(`https?://\S+`)

// workLogShapes are how an agent writes down what it did rather than what it
// found: branch refs, source paths, CLI flags, API fields echoed verbatim. A
// sentence built out of these is a work log entry —
// 「在现有源分支 feat/live-context-settings 上重生 server/CONFIGURATION.md…」—
// and no amount of word-level cleaning turns it into something to say to
// somebody. It is dropped whole.
//
// This test is deliberately local to quoting a findings digest. It is not part
// of ScrubInternalTerms, which rewrites every outbound message including the
// conversation model's own: a branch name or file path is often exactly what a
// user asked for, and deleting it there would answer the wrong question.
var workLogShapes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:feat|feature|fix|hotfix|chore|refactor|docs|release)/[a-z0-9][\w.-]*`),
	regexp.MustCompile(`(?i)\borigin/[\w.-]+`),
	regexp.MustCompile(`(?i)\b[\w.-]+(?:/[\w.-]+)+\.(?:go|ts|tsx|vue|md|json|ya?ml|sql|sh)\b`),
	regexp.MustCompile(`\b[a-z][\w]*\s*=\s*[A-Z][A-Z_]{2,}\b`),
	regexp.MustCompile(`(?i)\bHEAD\s+[0-9a-f]{7,40}\b`),
	regexp.MustCompile(`\s--?[a-z][\w-]{2,}\b`),
}

// readsLikeWorkLog reports whether this sentence is an account of the work
// rather than of the outcome. Links are excluded from the judgement: a delivery
// URL is the one piece of a digest that is always worth keeping.
func readsLikeWorkLog(sentence string) bool {
	probe := urlSpan.ReplaceAllString(sentence, " ")
	for _, p := range workLogShapes {
		if p.MatchString(probe) {
			return true
		}
	}
	return false
}

// splitSentences cuts after CJK/latin sentence enders and newlines, keeping the
// terminator (and any spacing that follows) with the sentence it ends, so the
// pieces can be re-joined without losing the gaps between them.
//
// An ASCII period or semicolon only ends a sentence when whitespace or the end
// of the text follows it. Inside a token it belongs to the token: breaking at
// every dot cut CONFIGURATION.md in half — and the half that was left no longer
// looked like a file path, so it passed the work-log filter and went out. It
// does the same to version numbers, decimals and URLs.
func splitSentences(text string) []string {
	var out []string
	var cur strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		cur.WriteRune(r)
		switch r {
		case '。', '！', '？', '；', '\n':
		case '.', '!', '?', ';':
			if i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
				continue
			}
		default:
			continue
		}
		if strings.TrimSpace(cur.String()) != "" {
			out = append(out, cur.String())
		}
		cur.Reset()
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
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
