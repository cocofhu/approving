package runtime

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Dual-write turn contract: review/clarify human turns ask the agent to append
// a trailing ```json fence with {"agentSummary":"..."} after the user-facing
// narration. The summary is persisted on the feedback round product; Msg keeps
// only the narration for the transcript bubble.
const dualWriteTurnContract = `

## 本轮输出契约(强制)
处理本轮人工反馈时,请同时产出两段内容:
1. **叙述回复**(正文):面向对话气泡,说明你如何理解/处理本轮反馈。
2. **文末 JSON 代码块**(机器可读):在叙述之后追加且仅追加一个 fenced JSON,格式严格为:
` + "```json\n" + `{"agentSummary":"对本轮反馈要点的归纳"}
` + "```" + `
规则:
- agentSummary 是供反馈账本卡片最前展示的「Agent 总结」,须归纳本轮反馈意图/要点,与叙述分离。
- 禁止把索引规则 gist、正文首行或对话气泡原文原样当作 agentSummary。
- 若无法归纳,可省略该 JSON 代码块(平台将隐藏总结区);不要写空字符串占位。
- JSON 只出现在文末代码块中,不要把 JSON 写进叙述正文。
`

// Match the last fenced code block (optional language tag) at end of text.
var dualWriteJSONFence = regexp.MustCompile("(?s)```(?:json)?\\s*\\n(\\{[\\s\\S]*?\\})\\s*\\n```\\s*$")

type dualWritePayload struct {
	AgentSummary string `json:"agentSummary"`
	Narration    string `json:"narration"`
}

// withDualWriteContract appends the dual-write output contract to a human
// prompt sent on review/clarify feedback turns.
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

	if loc := dualWriteJSONFence.FindStringSubmatchIndex(raw); len(loc) >= 4 {
		jsonBody := raw[loc[2]:loc[3]]
		prefix := strings.TrimSpace(raw[:loc[0]])
		if sum, narr, ok := parseDualWriteJSON(jsonBody); ok {
			if narr != "" {
				return narr, sum
			}
			return prefix, sum
		}
		// Fence present but not our dual-write shape: keep raw intact.
		return raw, ""
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
func applyDualWrite(raw string) (msg, agentSummary string) {
	return splitTurnDualWrite(raw)
}
