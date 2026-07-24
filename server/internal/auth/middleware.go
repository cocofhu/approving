package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/cocofhu/approving/internal/models"

	"github.com/gin-gonic/gin"
)

var apiWhitelist = map[string]struct{}{
	"/api/health":      {},
	"/api/live":        {},
	"/api/auth/login":  {},
	"/api/auth/logout": {},
}

// APIMiddleware validates cf_session for /api/* except whitelisted paths.
func (s *Service) APIMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if _, ok := apiWhitelist[path]; ok {
			c.Next()
			return
		}
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		token := SessionToken(c)
		sess, err := s.ValidateSession(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Set("auth_session", sess)
		c.Set("auth_username", sess.Username)
		c.Next()
	}
}

// SandboxRedirectMiddleware redirects unauthenticated browser requests to /login.
func (s *Service) SandboxRedirectMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := SessionToken(c)
		if _, err := s.ValidateSession(token); err != nil {
			redirect := ValidateRedirect(c.Request.URL.RequestURI())
			c.Redirect(http.StatusFound, "/login?redirect="+url.QueryEscape(redirect))
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireSession aborts with 401 when no valid session (for WebSocket upgrade paths).
func (s *Service) RequireSession(c *gin.Context) (*models.Session, bool) {
	token := SessionToken(c)
	sess, err := s.ValidateSession(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		c.Abort()
		return nil, false
	}
	c.Set("auth_session", sess)
	return sess, true
}
