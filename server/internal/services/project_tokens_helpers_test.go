package services

// totalTokens returns the summed project Token total, or nil when no Usage
// has been reported. Test-only thin wrapper over TokenBreakdownByProjectIDs.
func (s *ProjectService) totalTokens(projectID string) *int64 {
	return s.TokenBreakdownByProjectIDs([]string{projectID})[projectID].Total
}

// totalTokensByProjectIDs batch-aggregates project Token totals for tests.
func (s *ProjectService) totalTokensByProjectIDs(projectIDs []string) map[string]*int64 {
	bd := s.TokenBreakdownByProjectIDs(projectIDs)
	out := make(map[string]*int64, len(bd))
	for pid, b := range bd {
		if b.Total != nil {
			out[pid] = b.Total
		}
	}
	return out
}
