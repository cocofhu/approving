package auth

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultSessionTTL = 7 * 24 * time.Hour

// Guard 封装口令保护与会话 Cookie。
type Guard struct {
	passwordHash []byte
	store        *Store
	ttl          time.Duration
}

// NewGuard 在 password 非空时启用；password 已 trim，启动时哈希为 bcrypt。
func NewGuard(password string) *Guard {
	password = strings.TrimSpace(password)
	if password == "" {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil
	}
	return &Guard{
		passwordHash: hash,
		store:        NewStore(defaultSessionTTL),
		ttl:          defaultSessionTTL,
	}
}

// Enabled 是否启用了访问控制。
func (g *Guard) Enabled() bool { return g != nil }

// sessionFromRequest 从 Cookie 读取会话令牌。
func sessionFromRequest(c *gin.Context) string {
	v, err := c.Cookie(cookieName)
	if err != nil || v == "" {
		return ""
	}
	return v
}

// Middleware 未登录时：/api/* 返回 401，/ws 返回 401，其余 GET 重定向到登录页。
func (g *Guard) Middleware() gin.HandlerFunc {
	if g == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if isPublicPath(path, c.Request.Method) {
			c.Next()
			return
		}
		if g.store.Valid(sessionFromRequest(c)) {
			c.Next()
			return
		}
		respondUnauthorized(c)
	}
}

func isPublicPath(path, method string) bool {
	if path == "/login" && method == http.MethodGet {
		return true
	}
	if path == "/api/login" && method == http.MethodPost {
		return true
	}
	if path == "/api/logout" && method == http.MethodPost {
		return true
	}
	return false
}

func respondUnauthorized(c *gin.Context) {
	path := c.Request.URL.Path
	if path == "/ws" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if strings.HasPrefix(path, "/api/") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	reqPath := c.Request.URL.Path
	if rq := c.Request.URL.RawQuery; rq != "" {
		reqPath = reqPath + "?" + rq
	}
	next := safeNextPath(reqPath)
	q := url.Values{}
	q.Set("next", next)
	c.Redirect(http.StatusFound, "/login?"+q.Encode())
	c.Abort()
}

func safeNextPath(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "/login" {
		return "/"
	}
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return "/"
	}
	return s
}

// LoginGET 已登录则跳回 next 或首页。
func (g *Guard) LoginGET(loginFile string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil {
			c.Redirect(http.StatusFound, "/")
			return
		}
		if g.store.Valid(sessionFromRequest(c)) {
			c.Redirect(http.StatusFound, safeNextPath(c.Query("next")))
			return
		}
		c.File(loginFile)
	}
}

type loginBody struct {
	Password string `json:"password"`
}

// LoginPOST 校验口令并下发 HttpOnly Cookie。
func (g *Guard) LoginPOST() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "auth disabled"})
			return
		}
		var body loginBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
			return
		}
		if !PasswordMatch(g.passwordHash, body.Password) {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "wrong password"})
			return
		}
		token, err := g.store.Issue()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "session error"})
			return
		}
		setSessionCookie(c, token, g.ttl)
		c.JSON(http.StatusOK, gin.H{"ok": true, "next": safeNextPath(c.Query("next"))})
	}
}

// LogoutPOST 清除 Cookie 并作废服务端会话。
func (g *Guard) LogoutPOST() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil {
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
		tok := sessionFromRequest(c)
		if tok != "" {
			g.store.Revoke(tok)
		}
		clearSessionCookie(c)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func setSessionCookie(c *gin.Context, token string, ttl time.Duration) {
	maxAge := int(ttl.Seconds())
	if maxAge < 1 {
		maxAge = 3600
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}

func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}
