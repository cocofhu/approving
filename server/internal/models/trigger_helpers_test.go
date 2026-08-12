package models

// validTrigger reports whether s is one of the three allowed codes.
// Test-only helper (production parsing uses ParseTrigger / NormalizeTrigger).
func validTrigger(s string) bool {
	switch s {
	case TriggerManual, TriggerAPI, TriggerPMMCP:
		return true
	default:
		return false
	}
}
