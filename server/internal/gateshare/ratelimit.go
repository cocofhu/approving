package gateshare

import (
	"sync"
	"time"
)

const (
	publicRateWindow = time.Minute
	publicRateMax    = 30
)

type ipWindow struct {
	start time.Time
	count int
}

// IPLimiter is a per-IP sliding-window QPS limiter (not login lockout).
type IPLimiter struct {
	mu   sync.Mutex
	byIP map[string]*ipWindow
}

// NewIPLimiter builds a public-endpoint limiter.
func NewIPLimiter() *IPLimiter {
	return &IPLimiter{byIP: map[string]*ipWindow{}}
}

// Allow reports whether ip may proceed. false → over limit.
func (l *IPLimiter) Allow(ip string) bool {
	if ip == "" {
		ip = "unknown"
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.byIP[ip]
	if w == nil || now.Sub(w.start) >= publicRateWindow {
		l.byIP[ip] = &ipWindow{start: now, count: 1}
		return true
	}
	if w.count >= publicRateMax {
		return false
	}
	w.count++
	return true
}
