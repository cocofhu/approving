package channels

import (
	"strings"

	"github.com/cocofhu/approving/internal/services"
)

// LiveIntent is the Live conversation layer's classification of one inbound
// message.
//
// conversation and delegation both need the agent — deciding whether a request
// deserves a background Run is a judgement call, not a keyword match, so the
// router deliberately does not try to separate them. The other three are
// answered from stored task state without opening a sandbox turn, which is what
// keeps the conversation responsive while Runs execute.
type LiveIntent string

const (
	// IntentConversation is ordinary chat: answer directly, never start a Run.
	IntentConversation LiveIntent = "conversation"
	// IntentDelegation is work that belongs in a background Run.
	IntentDelegation LiveIntent = "delegation"
	// IntentTaskQuery asks about an existing task's state.
	IntentTaskQuery LiveIntent = "task_query"
	// IntentTaskControl continues, amends, cancels or approves an existing task.
	IntentTaskControl LiveIntent = "task_control"
	// IntentClarificationReply answers a question the background asked.
	IntentClarificationReply LiveIntent = "clarification_reply"
)

// FastPath reports whether the intent can be served from stored state alone.
func (i LiveIntent) FastPath() bool {
	switch i {
	case IntentTaskQuery, IntentTaskControl, IntentClarificationReply:
		return true
	default:
		return false
	}
}

// highRiskRules map an explicit action verb onto the action it authorizes.
// Order matters: the most specific phrase for an action wins.
var highRiskRules = []struct {
	action string
	verbs  []string
}{
	{"cancel_run", []string{"取消任务", "取消运行", "停止任务", "终止任务", "取消", "停止", "终止",
		"cancel run", "cancel the task", "cancel this task", "cancel", "stop task", "abort"}},
	{"approve_gate", []string{"批准门禁", "同意门禁", "批准", "同意", "通过", "approve gate", "approve"}},
	{"reject_gate", []string{"拒绝门禁", "驳回", "拒绝", "reject gate", "reject"}},
	{"delete_run", []string{"删除任务", "删除", "删掉", "delete task", "delete"}},
}

// negationCues mark a message as commentary rather than a command. A user who
// says 「不要这样啊」 or 「没有实现」 is complaining about what happened, not asking
// to cancel anything — treating that as a destructive intent is how a complaint
// turned into a cancel_run confirmation prompt.
var negationCues = []string{
	"不要这样", "不是这样", "别这样", "不用取消", "不要取消", "别取消", "不想取消",
	"没有实现", "没实现", "没做", "不对", "怎么回事", "为什么", "why", "not implemented",
}

// detectHighRiskIntent returns the destructive action a message explicitly
// authorizes, plus the remaining text used to address a task.
//
// Two conditions must hold, because a bare keyword scan over free-form chat
// produces false positives on ordinary complaints:
//
//  1. the action verb leads the message (a verb buried mid-sentence is
//     commentary, e.g. 「我不想取消」), and
//  2. the message carries no negation cue.
//
// Callers must still resolve the remainder to a real task before acting.
func detectHighRiskIntent(text string) (action, query string) {
	t := strings.TrimSpace(text)
	if t == "" || looksLikeComplaint(t) {
		return "", ""
	}
	lower := strings.ToLower(t)
	for _, rule := range highRiskRules {
		for _, verb := range rule.verbs {
			rest, ok := cutLeadingVerb(lower, t, strings.ToLower(verb))
			if !ok {
				continue
			}
			if rest == "" {
				rest = t
			}
			return rule.action, rest
		}
	}
	return "", ""
}

// cutLeadingVerb reports whether the message opens with verb and returns the
// remainder with connective particles removed.
func cutLeadingVerb(lower, original, verb string) (string, bool) {
	if verb == "" || !strings.HasPrefix(lower, verb) {
		return "", false
	}
	rest := strings.TrimSpace(string([]rune(original)[len([]rune(verb)):]))
	rest = strings.TrimLeft(rest, " \t:：,，。.、")
	for _, particle := range []string{"掉", "了", "吧", "一下", "这个", "那个", "该"} {
		rest = strings.TrimSpace(strings.TrimPrefix(rest, particle))
	}
	return strings.TrimSpace(rest), true
}

func looksLikeComplaint(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, cue := range negationCues {
		if strings.Contains(lower, strings.ToLower(cue)) {
			return true
		}
	}
	return false
}

// amendmentCues introduce a change to an in-flight task's requirements.
var amendmentCues = []string{
	"改成", "改为", "换成", "再加", "追加", "补充", "顺便", "另外还要", "还要加",
	"instead", "also add", "change it to", "make it",
}

// looksLikeAmendment reports whether the user is revising an existing task
// rather than asking for a new one. FR-3 requires these to reach the addressed
// Run instead of silently becoming a fresh delegation.
func looksLikeAmendment(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	for _, cue := range amendmentCues {
		if strings.Contains(lower, strings.ToLower(cue)) {
			return true
		}
	}
	return false
}

// classifyIntent assigns the Live intent for an inbound message. hasPendingRisk
// and taskResolvable are supplied by the Manager because they need database
// state the router itself does not own.
func classifyIntent(text string, hasPendingRisk, taskResolvable bool) LiveIntent {
	t := strings.TrimSpace(text)
	if t == "" {
		return IntentConversation
	}
	if hasPendingRisk && services.ParseRiskDecisionPublic(t) != "" {
		return IntentClarificationReply
	}
	if action, _ := detectHighRiskIntent(t); action != "" && taskResolvable {
		return IntentTaskControl
	}
	if taskResolvable && (isContinuation(t) || looksLikeAmendment(t)) {
		return IntentTaskControl
	}
	if isStatusQuery(t) && taskResolvable {
		return IntentTaskQuery
	}
	// Everything else needs the agent's judgement. Ambiguity resolves toward
	// conversation on purpose: a wrongly created Run is far more disruptive
	// than a clarifying question.
	return IntentConversation
}
