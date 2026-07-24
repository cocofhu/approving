package sandbox

import (
	"encoding/json"

	"github.com/cocofhu/approving/internal/models"
)

// parsePromptDoneUsage flattens prompt_done.usage (map[modelID]TokenUsage) into
// a single TokenUsage by summing each component across model buckets. Returns a
// non-nil pointer whenever the usage field is present and parseable (including
// `{}` or all-zero counters), so callers can distinguish "reported 0" from
// "not reported". Returns nil when raw is empty/null or unparseable.
func parsePromptDoneUsage(raw json.RawMessage) *models.TokenUsage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var byModel map[string]models.TokenUsage
	if err := json.Unmarshal(raw, &byModel); err != nil {
		return nil
	}
	// Explicit usage object (even empty) means "reported".
	out := &models.TokenUsage{}
	for _, u := range byModel {
		out.InputTokens += u.InputTokens
		out.OutputTokens += u.OutputTokens
		out.CacheReadTokens += u.CacheReadTokens
		out.CacheWriteTokens += u.CacheWriteTokens
	}
	return out
}
