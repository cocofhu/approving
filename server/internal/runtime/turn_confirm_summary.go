package runtime

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Confirm-time summary turn: once the human clicked「确认并流转」and the agent has
// reconciled its products against the transcript, one extra hidden turn asks it
// for a machine-readable induction of the whole dialogue. The turn's text never
// reaches the transcript, so the whole output is expected to be the JSON payload
// and there is no narration to protect.

// Match fenced JSON objects so the payload is still recovered when the agent
// wraps its answer in a code fence or adds a stray line around it.
var confirmSummaryJSONFence = regexp.MustCompile("(?s)```(?:json)?\\s*\\n(\\{[\\s\\S]*?\\})\\s*\\n```")

type confirmSummaryPayload struct {
	AgentSummary string `json:"agentSummary"`
}

// parseAgentSummary extracts the agentSummary from a confirm-time summary turn.
//
// Accepted shapes (summary only when non-empty after trim):
//  1. A ```json fence holding {"agentSummary":"..."} — the last valid one wins
//  2. A whole-message JSON object with the same key
//
// Returns "" on a miss or an empty value. It never promotes the raw text to a
// summary: an unparseable turn leaves the ledger card's summary section hidden
// rather than showing prose the agent did not mean as an induction.
func parseAgentSummary(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	fences := confirmSummaryJSONFence.FindAllStringSubmatch(raw, -1)
	for i := len(fences) - 1; i >= 0; i-- {
		if s := parseConfirmSummaryJSON(fences[i][1]); s != "" {
			return s
		}
	}
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		return parseConfirmSummaryJSON(raw)
	}
	return ""
}

func parseConfirmSummaryJSON(body string) string {
	var p confirmSummaryPayload
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		return ""
	}
	return strings.TrimSpace(p.AgentSummary)
}
