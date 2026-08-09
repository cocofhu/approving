package gateshare

import (
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

// ParseTTLTier returns the duration for a product TTL tier. Empty → 24h default.
func ParseTTLTier(tier string) (string, time.Duration, bool) {
	t := strings.TrimSpace(tier)
	if t == "" {
		t = models.ShareTTLTier24h
	}
	switch t {
	case models.ShareTTLTier1h:
		return t, time.Hour, true
	case models.ShareTTLTier8h:
		return t, 8 * time.Hour, true
	case models.ShareTTLTier24h:
		return t, 24 * time.Hour, true
	case models.ShareTTLTier72h:
		return t, 72 * time.Hour, true
	case models.ShareTTLTier7d:
		return t, 7 * 24 * time.Hour, true
	default:
		return "", 0, false
	}
}

// IsPassAction is a standard positive human_gate action.
func IsPassAction(id string) bool {
	return id == "approve" || id == "pass"
}

// IsFailAction is a standard negative human_gate action.
func IsFailAction(id string) bool {
	return id == "revise" || id == "fail"
}

// HasStandardAction reports whether approve|pass or revise|fail is present.
func HasStandardAction(actions []models.GateAction) bool {
	for _, a := range actions {
		if IsPassAction(a.ID) || IsFailAction(a.ID) {
			return true
		}
	}
	return false
}

// ResolvePassAction returns the first standard pass action id, or "".
func ResolvePassAction(actions []models.GateAction) string {
	for _, a := range actions {
		if IsPassAction(a.ID) {
			return a.ID
		}
	}
	return ""
}

// ResolveFailAction returns the first standard fail action id, or "".
func ResolveFailAction(actions []models.GateAction) string {
	for _, a := range actions {
		if IsFailAction(a.ID) {
			return a.ID
		}
	}
	return ""
}

// IsWhitelistedExternalAction reports whether action may be submitted externally.
func IsWhitelistedExternalAction(action string, actions []models.GateAction) bool {
	action = strings.TrimSpace(action)
	if !IsPassAction(action) && !IsFailAction(action) {
		return false
	}
	for _, a := range actions {
		if a.ID == action {
			return true
		}
	}
	return false
}
