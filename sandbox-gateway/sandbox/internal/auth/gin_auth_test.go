package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNewGuard(t *testing.T) {
	if NewGuard("") != nil || NewGuard("   ") != nil {
		t.Fatal("empty disables")
	}
	g := NewGuard("secret")
	if !g.Enabled() || g.store == nil {
		t.Fatal("enabled")
	}
}

func TestSafeNextPath(t *testing.T) {
	if safeNextPath("") != "/" || safeNextPath("/login") != "/" {
		t.Fatal("default")
	}
	if safeNextPath("//evil") != "/" || safeNextPath("http://x") != "/" {
		t.Fatal("open redirect")
	}
	if safeNextPath("/foo") != "/foo" {
		t.Fatal("ok path")
	}
}

func TestIsPublicPath(t *testing.T) {
	if !isPublicPath("/login", http.MethodGet) || !isPublicPath("/api/login", http.MethodPost) {
		t.Fatal("public")
	}
	if isPublicPath("/api/x", http.MethodGet) {
		t.Fatal("not public")
	}
}

func TestMiddlewareUnauthed(t *testing.T) {
	g := NewGuard("pw")
	r := gin.New()
	r.Use(g.Middleware())
	loginFile := filepath.Join(t.TempDir(), "login.html")
	_ = os.WriteFile(loginFile, []byte("login"), 0o644)
	r.GET("/login", g.LoginGET(loginFile))
	r.GET("/api/me", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.GET("/ws", func(c *gin.Context) { c.Status(200) })
	r.GET("/page", func(c *gin.Context) { c.String(200, "page") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if w.Code != 401 {
		t.Fatalf("api want 401 got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ws", nil))
	if w.Code != 401 {
		t.Fatalf("ws want 401 got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/page?x=1", nil))
	if w.Code != 302 {
		t.Fatalf("page want 302 got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	if w.Code != 200 {
		t.Fatalf("login get %d", w.Code)
	}
}

func TestLoginFlow(t *testing.T) {
	g := NewGuard("pw")
	r := gin.New()
	r.POST("/api/login", g.LoginPOST())
	r.POST("/api/logout", g.LogoutPOST())
	r.GET("/api/me", g.Middleware(), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("wrong pw %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("bad json %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"pw"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookies[0])
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("authed me %d", w.Code)
	}

	// logged-in LoginGET redirects
	loginFile := filepath.Join(t.TempDir(), "login.html")
	_ = os.WriteFile(loginFile, []byte("x"), 0o644)
	r.GET("/login", g.LoginGET(loginFile))
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/login?next=/page", nil)
	req.AddCookie(cookies[0])
	r.ServeHTTP(w, req)
	if w.Code != 302 {
		t.Fatalf("login redirect %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.AddCookie(cookies[0])
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("logout %d", w.Code)
	}
}

func TestNilGuardHandlers(t *testing.T) {
	var g *Guard
	r := gin.New()
	r.Use(g.Middleware())
	r.GET("/", g.LoginGET("/tmp/x"))
	r.POST("/api/login", g.LoginPOST())
	r.POST("/api/logout", g.LogoutPOST())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != 302 {
		t.Fatalf("nil LoginGET %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/login", nil))
	if w.Code != 400 {
		t.Fatalf("nil LoginPOST %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/logout", nil))
	if w.Code != 200 {
		t.Fatalf("nil Logout %d", w.Code)
	}
}
