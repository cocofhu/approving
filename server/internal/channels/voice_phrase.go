package channels

import (
	"context"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/liveagent"
)

// VoicePersonaLead is the single shared opening for every spoken prompt and
// system instruction that addresses the user as the project owner on IM.
// Prompt bodies differ; this lead must not be re-copied elsewhere.
const VoicePersonaLead = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。`

// phrasePromptHeader is the shared role/delivery lead-in for every spoken
// phrase prompt (status + the four acks).
const phrasePromptHeader = VoicePersonaLead

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

// phraseThroughLive asks the fast model for one short IM line. Empty means the
// caller must not invent a platform sentence to stand in for it.
func (m *Manager) phraseThroughLive(ctx context.Context, system, user string) string {
	if m == nil || m.live == nil || !m.live.Configured() {
		return ""
	}
	system, user = strings.TrimSpace(system), strings.TrimSpace(user)
	if system == "" || user == "" {
		return ""
	}
	// Keep this hop short: it is only one sentence. A stuck Ollama must not
	// burn the full live_timeout before we skip the spoken ack.
	timeout := 20 * time.Second
	if d := m.live.Timeout(); d > 0 && d < timeout {
		timeout = d
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := m.live.Complete(callCtx, liveagent.Request{
		System: system, Messages: []liveagent.Message{{Role: "user", Content: user}},
		MaxTokens: 256,
	})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(res.Text)
}
