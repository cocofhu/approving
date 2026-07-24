package handlers

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/auth"

	"github.com/gin-gonic/gin"
)

func splitTestServerHostPort(t *testing.T, rawURL string) (string, string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split test server host/port: %v", err)
	}
	return host, port
}

func newTestGinContext(path string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	c.Request = req
	return c, w
}

func TestAutoLoginCodeServerWritesMountedCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			t.Fatalf("login path = %q, want /login", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("password"); got != "secret" {
			t.Fatalf("password = %q, want secret", got)
		}
		http.SetCookie(w, &http.Cookie{Name: "key", Value: "abc", Path: "/"})
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	host, port := splitTestServerHostPort(t, upstream.URL)
	upstreamHost := host + ":" + port

	c, w := newTestGinContext("/sandbox/42/")
	c.Request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "jwt"})

	if err := autoLoginCodeServer(c, 42, upstreamHost, "secret"); err != nil {
		t.Fatalf("autoLoginCodeServer() error = %v", err)
	}
	setCookies := w.Result().Cookies()
	if len(setCookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(setCookies))
	}
	if setCookies[0].Name != "key" || setCookies[0].Value != "abc" || setCookies[0].Path != "/sandbox/42/" {
		t.Fatalf("mounted cookie = %#v", setCookies[0])
	}
	if got := c.Request.Header.Get("Cookie"); !strings.Contains(got, "key=abc") {
		t.Fatalf("request Cookie = %q, want key=abc", got)
	}
}

func TestAutoLoginSkipsWhenUpstreamCookieExists(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	host, port := splitTestServerHostPort(t, upstream.URL)
	upstreamHost := host + ":" + port

	c, _ := newTestGinContext("/sandbox/42/")
	c.Request.AddCookie(&http.Cookie{Name: "key", Value: "abc"})

	if err := autoLoginCodeServer(c, 42, upstreamHost, "secret"); err != nil {
		t.Fatalf("autoLoginCodeServer() error = %v", err)
	}
	if called {
		t.Fatal("autoLoginCodeServer() called upstream despite existing upstream cookie")
	}
}

func TestAutoLoginSkipsEmptyPassword(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	host, port := splitTestServerHostPort(t, upstream.URL)
	c, _ := newTestGinContext("/sandbox/1/")
	if err := autoLoginCodeServer(c, 1, host+":"+port, ""); err != nil {
		t.Fatalf("autoLoginCodeServer() error = %v", err)
	}
	if called {
		t.Fatal("autoLoginCodeServer() should no-op when password is empty")
	}
}

func TestAutoLoginVibeCodingWritesMountedCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/login" {
			t.Fatalf("login path = %q", r.URL.Path)
		}
		http.SetCookie(w, &http.Cookie{Name: "key", Value: "vibe", Path: "/"})
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	host, port := splitTestServerHostPort(t, upstream.URL)
	c, w := newTestGinContext("/sandbox-bridge/7/")
	if err := autoLoginVibeCoding(c, 7, "/sandbox-bridge/7", host+":"+port, "secret"); err != nil {
		t.Fatalf("autoLoginVibeCoding: %v", err)
	}
	setCookies := w.Result().Cookies()
	if len(setCookies) != 1 || setCookies[0].Value != "vibe" || setCookies[0].Path != "/sandbox-bridge/7/" {
		t.Fatalf("cookie=%#v", setCookies)
	}
}

func TestMergeUpstreamQueryAddsDefaultFolder(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://upstream/", nil)
	mergeUpstreamQuery(req, url.Values{})
	if got := req.URL.Query().Get("folder"); got != DefaultCodeServerWorkspaceFolder {
		t.Fatalf("folder = %q, want %q", got, DefaultCodeServerWorkspaceFolder)
	}
}
