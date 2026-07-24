package auth

import (
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/config"
)

// RateLimiter tracks per-IP login failures in process memory.
type RateLimiter struct {
	cfg  func() *config.Config
	mu   sync.Mutex
	byIP map[string]*rateEntry
}

type rateEntry struct {
	failures    int
	lockedUntil time.Time
}

// NewRateLimiter constructs a rate limiter backed by the live auth config.
func NewRateLimiter(cfg func() *config.Config) *RateLimiter {
	return &RateLimiter{cfg: cfg, byIP: map[string]*rateEntry{}}
}

// Check returns an error message when the IP is locked.
func (r *RateLimiter) Check(ip string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.byIP[ip]
	if e == nil {
		return "", false
	}
	if time.Now().Before(e.lockedUntil) {
		remaining := time.Until(e.lockedUntil).Round(time.Minute)
		if remaining < time.Minute {
			remaining = time.Minute
		}
		return "登录尝试过于频繁，请 " + remaining.String() + " 后再试", true
	}
	if !e.lockedUntil.IsZero() {
		delete(r.byIP, ip)
	}
	return "", false
}

// RecordFailure increments failures for ip; returns true when locked.
func (r *RateLimiter) RecordFailure(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	max := r.cfg().Auth.MaxFailures
	if max <= 0 {
		max = 5
	}
	e := r.byIP[ip]
	if e == nil {
		e = &rateEntry{}
		r.byIP[ip] = e
	}
	e.failures++
	if e.failures >= max {
		e.lockedUntil = time.Now().Add(r.cfg().Auth.LockDurationDuration())
		return true
	}
	return false
}

// Reset clears failure count for ip after a successful login.
func (r *RateLimiter) Reset(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byIP, ip)
}
