package channels

import (
	"context"
	"strings"

	"github.com/cocofhu/approving/internal/services"
)

// operationalLine is a situation the platform has to tell the user about that
// no model decided: the backlog is full, a turn ran out of time, a confirmed
// action did not go through, the process restarted mid-turn.
//
// Each of these used to be a fixed string, and a fixed string is how copy
// nobody chose ends up in someone's chat window — 「你可以接着问别的」 was one
// of them. The situation is a fact the platform owns; the wording is the
// conversation model's job, exactly as it is for every other line it speaks.
// Fallback is what a missing or slow model falls back to, and nothing else.
type operationalLine struct {
	// Situation describes what happened, addressed to the model as the person
	// who has to say it.
	Situation string
	// Facts are internal details the model may use but must not quote back.
	Facts string
	// Avoid is a title or requirement the reply may name but must not paste
	// back wholesale.
	Avoid    string
	Language string
	Fallback string
}

// Intentional differences vs phraseAck (kept deliberately; do not flatten):
//  1. Naming is conditional on Facts (phraseAckRuleNameIt is unconditional).
//  2. Internal-term ban says「执行环境」not「沙箱」(phraseAck keeps 沙箱).
//  3. Exclusive bans: do not teach the user what to do; do not claim finished.
const (
	operationalRuleNameWhenGiven = `- 内部参考里给了是哪件事的，就用两三个字的自然说法点出来（「CI 那个」「登录页那块」）；不要照抄完整标题，不要用书名号或引号括回去，也不要只说「那事」「那块」——对方手上可能同时有好几件。`
	operationalRuleNoTeachUser   = `- 不要交代对方可以做什么（「你可以接着问别的」「有事随时叫我」这类都不要）——他本来就知道。`
	// Deliberate wording vs phraseAckRuleNoInternal (沙箱): operational lines
	// talk about the worker side, so the ban uses「执行环境」.
	operationalRuleNoInternalExecEnv = `- 不要提优先级、任务编号、工作流、执行环境、跟进页面、Approving。`
	operationalRuleNoFinishedClaim   = `- 不要说已经做完、已经跑完、已经处理好。`
)

// operationalLineRules are the constraints every spoken platform line shares.
// Shared colloquial / one-line atoms come from spokenRule* (SSOT with phraseAck);
// path-specific bullets above stay exclusive to this builder.
var operationalLineRules = buildOperationalLineRules()

func buildOperationalLineRules() string {
	rules := []string{
		spokenRuleColloquial,
		operationalRuleNameWhenGiven,
		operationalRuleNoTeachUser,
		operationalRuleNoInternalExecEnv,
		operationalRuleNoFinishedClaim,
		spokenRuleOneLine,
	}
	var b strings.Builder
	b.WriteString("\n\n规矩：\n")
	for i, r := range rules {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(r)
	}
	return b.String()
}

const operationalLinePersona = VoicePersonaLead + `

`

// speakOperationalLine phrases one platform notice, falling back to fixed copy.
func (m *Manager) speakOperationalLine(ctx context.Context, line operationalLine) string {
	fallback := strings.TrimSpace(line.Fallback)
	situation := strings.TrimSpace(line.Situation)
	if situation == "" {
		return fallback
	}
	user := "用一两句人话把这个情况告诉对方。"
	if facts := strings.TrimSpace(line.Facts); facts != "" {
		user = "（内部参考，勿原样复述）" + truncateRunes(facts, 160) + "\n" + user
	}
	if services.NormalizeLanguage(line.Language) == "en" {
		user += "\n对方说英文，用英文回一句。"
	}
	out := strings.TrimSpace(m.phraseThroughLive(ctx,
		operationalLinePersona+situation+operationalLineRules, user))
	if out == "" || spokenLineSoundsFinished(out) || retryAckEchoesBrief(out, line.Avoid, "") {
		return fallback
	}
	return out
}

// phraseRunAccepted asks the conversation model for the acceptance line.
func (m *Manager) phraseRunAccepted(ctx context.Context, shortTitle, language string) string {
	return m.speakOperationalLine(ctx, operationalLine{
		Situation: "对方要的事你已经安排下去了，现在只需要接一句：你去弄，完了回他。时态是正在做，不是做完了。",
		Facts:     shortTitle,
		Avoid:     shortTitle,
		Language:  language,
	})
}
