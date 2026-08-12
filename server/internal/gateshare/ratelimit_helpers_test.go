package gateshare

// LenForTest returns the number of tracked IP+bucket keys (tests / diagnostics).
func (l *IPLimiter) LenForTest() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.byKey)
}
