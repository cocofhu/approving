package services

// setMaxParallel updates concurrency (1–16). Test-only setter.
func (s *CronScheduler) setMaxParallel(n int) {
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	s.parallel.Store(int64(n))
}

// setClaimStaleMinutes updates reclaim window (30–1440). Test-only setter.
func (s *CronScheduler) setClaimStaleMinutes(n int) {
	if n < 30 {
		n = 30
	}
	if n > 1440 {
		n = 1440
	}
	s.staleMin.Store(int64(n))
}
