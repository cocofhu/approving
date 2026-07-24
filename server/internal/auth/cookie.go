package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SetSessionCookie writes the cf_session HttpOnly cookie.
func SetSessionCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := isSecureRequest(c)
	c.SetCookie(CookieName, token, CookieMaxAge, "/", "", secure, true)
}

// ClearSessionCookie expires the cf_session cookie.
func ClearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := isSecureRequest(c)
	c.SetCookie(CookieName, "", -1, "/", "", secure, true)
}

// SessionToken reads the cf_session cookie from the request.
func SessionToken(c *gin.Context) string {
	token, err := c.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(token)
}

func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	proto := strings.ToLower(c.GetHeader("X-Forwarded-Proto"))
	return proto == "https"
}
