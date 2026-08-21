package runtime

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Dual-write turn contract: only 「确认并流转」(force) turns ask the agent to
// append a trailing ```json fence with {"agentSummary":"..."} after the
// user-facing narration. The summary covers all human feedback in this node
// execution (unified), and is persisted on the confirm-round product; Msg keeps
// only the narration for the transcript bubble. Middle turns must not inject
// this contract.
const dualWriteTurnContract = `

## 统一总结契约(强制)
处理「确认并流转」时,请同时产出两段内容:
1. **叙述回复**(正文):面向对话气泡,说明你如何收束并完成流转。
2. **文末 JSON 代码块**(机器可读):在叙述之后追加且仅追加一个 fenced JSON,格式严格为:
` + "```json\n" + `{"agentSummary":"对本节点本次执行全部人工反馈要点的统一归纳"}
` + "```" + `
规则:
- agentSummary 是供反馈账本确认轮卡片最前展示的「Agent 总结」,须覆盖本节点本次执行收到的全部人工反馈要点(多轮合并为一份统一总结),与叙述分离。
- 禁止把索引规则 gist、正文首行或对话气泡原文原样当作 agentSummary。
- 除非确实无法归纳,请始终产出一段非空的 agentSummary；它应归纳全部反馈意图,而非复述叙述回复或仅复述最后一轮。
- 仅在确实无法归纳时才省略整个 JSON 代码块(平台将隐藏总结区);禁止输出空键、空字符串或模板占位。
- JSON 只出现在文末代码块中,不要把 JSON 写进叙述正文。
`

// Match fenced JSON objects so we can recover the final valid dual-write
// payload when an agent appends harmless prose after its contract fence.
var dualWriteJSONFence = regexp.MustCompile("(?s)```(?:json)?\\s*\\n(\\{[\\s\\S]*?\\})\\s*\\n```")

type dualWritePayload struct {
	AgentSummary string `json:"agentSummary"`
	Narration    string `json:"narration"`
}

// withDualWriteContract appends the unified-summary output contract to a
// confirm-and-advance (force) prompt.
func withDualWriteContract(human string) string {
	return strings.TrimRight(human, "\n") + dualWriteTurnContract
}

// splitTurnDualWrite extracts an optional agentSummary from a dual-write turn
// output and returns the narration for the transcript bubble.
//
// Accepted shapes (summary only when non-empty after trim):
//  1. Trailing ```json fence with {"agentSummary":"..."} (optional narration key)
//  2. Whole-message JSON object with the same keys
//
// On miss or empty summary: narration is the original raw text (trimmed of the
// fence when a fence was present but summary empty), agentSummary is "".
// Never invents a summary from gist / first line / bubble text.
func splitTurnDualWrite(raw string) (narration, agentSummary string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	matches := dualWriteJSONFence.FindAllStringSubmatchIndex(raw, -1)
	var emptyNarration string
	for i := len(matches) - 1; i >= 0; i-- {
		loc := matches[i]
		sum, narr, ok := parseDualWriteJSON(raw[loc[2]:loc[3]])
		if !ok || !hasLightweightTrailingNoise(raw[loc[1]:]) {
			continue
		}
		if sum == "" {
			if emptyNarration == "" {
				emptyNarration = withoutDualWriteFence(raw, loc)
			}
			continue
		}
		if narr != "" {
			return narr, sum
		}
		return withoutDualWriteFence(raw, loc), sum
	}
	if emptyNarration != "" {
		return emptyNarration, ""
	}

	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		if sum, narr, ok := parseDualWriteJSON(raw); ok {
			if narr != "" {
				return narr, sum
			}
			// Whole-message JSON with only agentSummary: no separate narration.
			return "", sum
		}
	}
	return raw, ""
}

// hasLightweightTrailingNoise permits a short, plain-text postscript after a
// valid final fence. It deliberately rejects another fence or a long body so
// an unrelated JSON example in the middle of narration cannot be absorbed.
func hasLightweightTrailingNoise(suffix string) bool {
	suffix = strings.TrimSpace(suffix)
	return len(suffix) <= 160 && !strings.Contains(suffix, "```")
}

func withoutDualWriteFence(raw string, loc []int) string {
	prefix := strings.TrimSpace(raw[:loc[0]])
	suffix := strings.TrimSpace(raw[loc[1]:])
	if prefix == "" {
		return suffix
	}
	if suffix == "" {
		return prefix
	}
	return prefix + "\n\n" + suffix
}

func parseDualWriteJSON(body string) (summary, narration string, ok bool) {
	var p dualWritePayload
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		return "", "", false
	}
	// Require the agentSummary key to have been present in some form: empty
	// string after trim means "omit field" (ok=true with empty summary only when
	// the key existed). Distinguishing absent vs empty via RawMessage.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &probe); err != nil {
		return "", "", false
	}
	rawSum, hasSum := probe["agentSummary"]
	if !hasSum {
		return "", "", false
	}
	_ = rawSum
	summary = strings.TrimSpace(p.AgentSummary)
	narration = strings.TrimSpace(p.Narration)
	return summary, narration, true
}

// applyDualWrite parses raw agent narration into ReactTurn Msg + AgentSummary.
// Callers on middle (!force) turns should keep the stripped narration but
// discard AgentSummary so spontaneous fences never land on intermediate cards.
func applyDualWrite(raw string) (msg, agentSummary string) {
	return splitTurnDualWrite(raw)
}
