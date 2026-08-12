package services

import "github.com/cocofhu/approving/internal/models"

// FormatRunDeepLinkForTest exposes runDeepLink for unit tests.
func FormatRunDeepLinkForTest(base, runID string) string {
	return runDeepLink(base, runID, "", "")
}

// FormatCompletedRunDeepLinkForTest exposes completed deep links for unit tests.
func FormatCompletedRunDeepLinkForTest(base, runID, nodeID string) string {
	return runDeepLink(base, runID, models.NotifyKindCompleted, nodeID)
}
