package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// APIKeyAuth returns a middleware that enforces `Authorization: Bearer <key>`
// against the configured keys. When keys is empty, auth is disabled (local
// testing) and all requests pass.
func APIKeyAuth(keys []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			allowed[k] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		if len(allowed) == 0 {
			c.Next()
			return
		}
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" || !matchAny(token, allowed) {
			log.Warn().
				Str("path", c.FullPath()).
				Str("raw_path", c.Request.URL.Path).
				Str("client_ip", c.ClientIP()).
				Msg("api auth rejected")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
func bearerToken(h string) string {
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// matchAny compares in constant time against each allowed key.
func matchAny(token string, allowed map[string]struct{}) bool {
	tb := []byte(token)
	for k := range allowed {
		if subtle.ConstantTimeCompare(tb, []byte(k)) == 1 {
			return true
		}
	}
	return false
}
