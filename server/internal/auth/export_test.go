package auth

import "github.com/cocofhu/approving/internal/models"

// Test-only helpers (compiled with tests only) so auth_test can mint sessions
// and inspect the login rate limiter without exporting production API surface.

func (s *Service) CreateSession(username string) (*models.Session, error) {
	s.cleanupExpired()
	return s.createSession(username)
}

func (s *Service) RateLimiter() *RateLimiter { return s.limit }
