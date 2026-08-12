package services

import "github.com/cocofhu/approving/internal/models"

// notifyPoliciesEqual compares two project policies for change detection.
// Test-only helper (production persists via Normalize + field writes).
func notifyPoliciesEqual(a, b models.ProjectNotifyPolicy) bool {
	a = NormalizeProjectNotifyPolicy(a)
	b = NormalizeProjectNotifyPolicy(b)
	if a.IsEnabled() != b.IsEnabled() {
		return false
	}
	if !stringSlicesEqual(a.DefaultEvents, b.DefaultEvents) {
		return false
	}
	if !stringSlicesEqual(NormalizeNotifyChannelIDs(a.ChannelIDs), NormalizeNotifyChannelIDs(b.ChannelIDs)) {
		return false
	}
	return a.WaitingHumanTemplate == b.WaitingHumanTemplate &&
		a.FailedTemplate == b.FailedTemplate &&
		a.CompletedTemplate == b.CompletedTemplate
}
