package channels

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/liveagent"

	"github.com/rs/zerolog/log"
)

// VoicePersonaLead is the single shared opening for every spoken prompt and
// system instruction that addresses the user as the project owner on IM.
// Prompt bodies differ; this lead must not be re-copied elsewhere.
const VoicePersonaLead = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。`

// phrasePromptHeader is the shared role/delivery lead-in for every spoken
// phrase prompt (status + the four acks).
const phrasePromptHeader = VoicePersonaLead

// ComposeVoicePrompt puts the fixed persona in front of a prompt body.
//
// Configurable prompts store the body alone and are assembled here on every
// call. Who the model is, and the fact that its output goes out verbatim, is
// not something an operator can edit away — a prompt that lost the persona
// would have the model introduce itself as an assistant in someone's IM window,
// and a prompt that lost the delivery note would have it write about what it
// was going to say instead of saying it.
func ComposeVoicePrompt(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return VoicePersonaLead
	}
	return VoicePersonaLead + "\n\n" + body
}

// statusWhileRunningPhrasePrompt rewrites a premature "done" claim against the
// live ledger so the user hears what is still in flight. Reuses only the shared
// header; body and finish-tense bans stay local (not the ack「规矩」block).
const statusWhileRunningPhrasePrompt = phrasePromptHeader + `

下面事实里还有在跑的任务。用一两句人话说清还在做、卡在哪（有阶段就带上）；禁止说已经做完、已经弄完、分析已经做完。
不要把同一件事的标题截断写法说成另一件排队任务。
只输出要发出去的话。`

// Shared spoken-policy atoms used by both phraseAck and operational paths.
// Intentional path differences (conditional naming, 沙箱 vs 执行环境, operational-only
// bans) live beside operational_rules.go — do not silently flatten them here.
const (
	spokenRuleColloquial = `- 像同事当面说，不要工单腔，不要「我这就去确认」「收到」「稍等」。`
	spokenRuleOneLine    = `- 只输出要发给对方的那句话。`
)

// Shared ack「规矩」lines (byte-identical across retry/fallthrough/dispatch/refine).
// Event-specific bullets stay beside each prompt; do not fold approximate wording.
const (
	phraseAckRuleColloquial = spokenRuleColloquial
	phraseAckRuleNoInternal = `- 不要提优先级、任务编号、工作流、沙箱、跟进页面、Approving。`
	phraseAckRuleOneLine    = spokenRuleOneLine

	// phraseAckRuleNameIt is what keeps two acks apart in a chat window. The
	// old rule said the opposite — 「用「那事」「那块」指代即可」 — on the
	// assumption that the user had just named the thing themselves. They often
	// have not: 「修复下」 got back 「好，那事我去弄」, which could have been
	// about anything. A handle costs three characters and settles it.
	//
	// Deliberate vs operational: phrase acks always require naming; operational
	// lines only name when Facts already identified the task (see
	// operationalRuleNameWhenGiven).
	phraseAckRuleNameIt = `- 一句话里要能看出是哪件事：从事情本身取两三个字的自然说法（「CI 那个」「登录页那块」）。不要照抄完整标题，不要用书名号或引号把标题括回去，也不要只说「那事」「那块」——对方手上可能同时有好几件。`
)

// buildPhraseAckPrompt joins header + event body + ordered rule bullets.
// Callers pass event-specific lines in place so differences stay readable;
// there is no event-switch super-template.
func buildPhraseAckPrompt(eventBody string, rules ...string) string {
	var b strings.Builder
	b.WriteString(phrasePromptHeader)
	b.WriteString("\n\n")
	b.WriteString(strings.TrimSpace(eventBody))
	b.WriteString("\n\n规矩：\n")
	for i, r := range rules {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(r)
	}
	return b.String()
}

var retryAckPhrasePrompt = buildPhraseAckPrompt(
	`对方刚明确说要重跑/再试一件刚失败的事。用一两句人话告诉对方：你正派人重新去做那件事——时态是正在重试，不是已经做完。`,
	phraseAckRuleColloquial,
	phraseAckRuleNameIt,
	phraseAckRuleNoInternal,
	`- 禁止「已经重新跑过了 / 已经重试过了 / 重新重试过了 / 已经进到队列」——现在才刚开干，还在进行中。`,
	phraseAckRuleOneLine,
)

var fallthroughAckPhrasePrompt = buildPhraseAckPrompt(
	`你这边还要再查一下才能答，人手马上接着干。用一两句人话接住对方。`,
	phraseAckRuleColloquial,
	`- 不要复述对方原话，也不要用书名号把长标题括回去。`,
	`- 时态是正在查/正在弄，不是已经查完。`,
	phraseAckRuleNoInternal,
	phraseAckRuleOneLine,
)

var dispatchAckPhrasePrompt = buildPhraseAckPrompt(
	`你刚把一件事派人去干了。用一两句人话告诉对方你正让人做这件事——时态是正在做，不是做完了。`,
	phraseAckRuleColloquial,
	phraseAckRuleNameIt,
	phraseAckRuleNoInternal,
	`- 禁止「已经重试过了 / 已经跑完了 / 已经进到队列」。`,
	phraseAckRuleOneLine,
)

var refineAckPhrasePrompt = buildPhraseAckPrompt(
	`对方刚补充/收窄了正在做的事。用一两句人话告诉对方你会按新重点继续——时态是接着做，不是做完了。`,
	phraseAckRuleColloquial,
	phraseAckRuleNameIt,
	phraseAckRuleNoInternal,
	phraseAckRuleOneLine,
)

// reportThroughDirector puts the work layer's conclusion into the voice the
// user has been hearing all along.
//
// This is rephrasing, not rewriting. The facts come from the agent, which is
// the only layer that checked them; the conversation layer is only allowed to
// say them the way it says everything else. When it cannot — no model, a slow
// endpoint, an empty completion — the agent's own words go out and are scrubbed
// at sendOutboundResult. degraded reports which of the two happened.
func (m *Manager) reportThroughDirector(ctx context.Context, rc *runningChannel, in InboundMessage,
	conclusion string) (text string, degraded bool) {
	plain := strings.TrimSpace(conclusion)
	started := time.Now()
	if plain == "" {
		m.appendTraceSpan(in.TraceID, finishSpan("director_report", "skipped", "degraded", started))
		return plain, true
	}
	spoken, err := m.livePhrase(ctx, phraseRequest{
		System: directorReportPrompt,
		Messages: []liveagent.Message{
			{Role: "user", Content: "对方问的是：" + truncateRunes(strings.TrimSpace(in.Text), 200)},
			{Role: "user", Content: "查到的结果：" + truncateRunes(plain, 3000)},
		},
		MaxTokens: directorReportMaxTokens,
		Timeout:   m.liveCallTimeout(directorReportFallbackTimeout),
	})
	if err != nil {
		// An unconfigured model is a deployment choice, not an incident; the
		// rest is worth a line, because an endpoint failing every call reads to
		// users as "the assistant started talking like a robot".
		if errors.Is(err, errNoLiveModel) {
			m.appendTraceSpan(in.TraceID, finishSpan("director_report", "skipped", "degraded", started))
			return plain, true
		}
		log.Info().Err(err).Str("project", rc.cfg.ProjectID).Str("trace", in.TraceID).
			Msg("conclusion reported in the work layer's own words; the conversation model did not phrase it")
		m.appendTraceSpan(in.TraceID, finishSpan("director_report", "error", err.Error(), started))
		return plain, true
	}
	m.appendTraceSpan(in.TraceID, finishSpan("director_report", "ok", "", started))
	return spoken, false
}

// retryAckEchoesBrief is true when the spoken line pastes the ledger title /
// requirement back — the failure mode behind quoting
// 「重新执行上次因服务重启而中断的 Approvin」.
//
// Naming the task is not pasting it, and the difference is length. This used to
// reject any line that reused six characters of the title, which ruled out
// 「CI 那个我去弄」 along with the quoted requirement, and left the pronoun as
// the only thing a model could say. What it rejects now is a run long enough to
// be the requirement read back.
func retryAckEchoesBrief(ack, title, req string) bool {
	ack = strings.TrimSpace(ack)
	if ack == "" {
		return false
	}
	if strings.ContainsAny(ack, "「」《》") {
		return true
	}
	for _, s := range []string{title, req} {
		if pastesSpanOf(ack, strings.TrimSpace(s)) {
			return true
		}
	}
	return false
}

// pasteSpanRunes is where reusing the source stops being a reference to it.
const pasteSpanRunes = 14

// pastesSpanOf reports whether ack quotes any pasteSpanRunes-long run of s.
// Any run, not just the opening one: a model that skips the first few words of
// a requirement and reads back the rest has still read it back.
func pastesSpanOf(ack, s string) bool {
	r := []rune(s)
	for i := 0; i+pasteSpanRunes <= len(r); i++ {
		if strings.Contains(ack, string(r[i:i+pasteSpanRunes])) {
			return true
		}
	}
	return false
}

// phraseRequest is one call to the fast model whose only job is to produce
// something a person will read. The timeout arrives already resolved: an ack
// wants a short ceiling regardless of the configured one, a conclusion wants
// the configured one, and folding both rules in here would make the difference
// invisible at the call sites where it matters.
type phraseRequest struct {
	System    string
	Messages  []liveagent.Message
	MaxTokens int
	Timeout   time.Duration
}

// errNoLiveModel is what an unconfigured or unusable conversation layer looks
// like to a caller. It is not a delivery failure: every caller has a complete
// message to fall back on.
var errNoLiveModel = errors.New("channels: no conversation model available")

// livePhrase makes the call and returns the line. Every phrasing path in this
// package goes through it, so the timeout handling, the empty-answer rule and
// the trimming exist once.
func (m *Manager) livePhrase(ctx context.Context, req phraseRequest) (string, error) {
	live := m.liveModel()
	if live == nil || !live.Configured() {
		return "", errNoLiveModel
	}
	if strings.TrimSpace(req.System) == "" || len(req.Messages) == 0 {
		return "", errNoLiveModel
	}
	callCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	res, err := live.Complete(callCtx, liveagent.Request{
		System:    strings.TrimSpace(req.System),
		Messages:  req.Messages,
		MaxTokens: req.MaxTokens,
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(res.Text)
	if text == "" {
		return "", ErrEmptyPhrase
	}
	return text, nil
}

// ErrEmptyPhrase means the model answered with nothing. Treated as a failure
// rather than as an empty line, because a caller that sent an empty string
// would be delivering silence and calling it a reply.
var ErrEmptyPhrase = errors.New("channels: the conversation model returned nothing")

// ackPhraseTimeout keeps a one-sentence hop short. A stuck Ollama must not burn
// the full live_timeout before we give up on the spoken ack — the reply the
// user is actually waiting for comes after it.
const ackPhraseTimeout = 20 * time.Second

// ackPhraseMaxTokens is sized for one IM line.
const ackPhraseMaxTokens = 256

// phraseThroughLive asks the fast model for one short IM line. Empty means the
// caller must not invent a platform sentence to stand in for it.
func (m *Manager) phraseThroughLive(ctx context.Context, system, user string) string {
	user = strings.TrimSpace(user)
	if user == "" {
		return ""
	}
	timeout := ackPhraseTimeout
	if live := m.liveModel(); live != nil {
		if d := live.Timeout(); d > 0 && d < timeout {
			timeout = d
		}
	}
	text, err := m.livePhrase(ctx, phraseRequest{
		System:    system,
		Messages:  []liveagent.Message{{Role: "user", Content: user}},
		MaxTokens: ackPhraseMaxTokens,
		Timeout:   timeout,
	})
	if err != nil {
		return ""
	}
	return text
}
