package gateshare

import (
	"sync"
	"time"
)

const (
	publicRateWindow = time.Minute
	publicRateMax    = 30
	ipLimiterGCAt    = 64
)

type ipWindow struct {
	start time.Time
	count int
}

// IPLimiter is a per-IP sliding-window QPS limiter (not login lockout).
// In-process only; expired IP entries are garbage-collected.
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
	if len(l.byIP) >= ipLimiterGCAt {
		l.gcExpiredLocked(now)
	}
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

func (l *IPLimiter) gcExpiredLocked(now time.Time) {
	for k, w := range l.byIP {
		if w == nil || now.Sub(w.start) >= publicRateWindow {
			delete(l.byIP, k)
		}
	}
}

// LenForTest returns the number of tracked IPs (tests / diagnostics).
func (l *IPLimiter) LenForTest() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.byIP)
}
