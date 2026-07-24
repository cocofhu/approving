package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRewriteRedirectLocation(t *testing.T) {
	base := "/sandbox/1"
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"https://ex.com/", base + "/"},
		{"https://ex.com/login?x=1", base + "/login?x=1"},
		{"/dashboard", base + "/dashboard"},
		{"relative", base + "/relative"},
		{base + "/already", base + "/already"},
	}
	for _, tc := range cases {
		resp := &http.Response{Header: http.Header{}}
		if tc.in != "" {
			resp.Header.Set("Location", tc.in)
		}
		rewriteRedirectLocation(resp, base)
		got := resp.Header.Get("Location")
		if got != tc.want {
			t.Errorf("in=%q got=%q want=%q", tc.in, got, tc.want)
		}
	}
	// invalid URL is a no-op
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Location", "://bad")
	rewriteRedirectLocation(resp, base)
	if resp.Header.Get("Location") != "://bad" {
		t.Fatal("invalid URL should stay")
	}
}

func TestRewriteUpstreamSetCookiePaths(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	rewriteUpstreamSetCookiePaths(resp, "/sandbox/1")
	if len(resp.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("empty should no-op")
	}
	resp.Header.Add("Set-Cookie", "a=1; Path=/")
	resp.Header.Add("Set-Cookie", "b=2; Path=/ide")
	resp.Header.Add("Set-Cookie", "c=3")
	rewriteUpstreamSetCookiePaths(resp, "/sandbox/9")
	vals := resp.Header.Values("Set-Cookie")
	joined := strings.Join(vals, "|")
	if !strings.Contains(joined, "Path=/sandbox/9/") || !strings.Contains(joined, "Path=/sandbox/9/ide") {
		t.Fatalf("cookies=%v", vals)
	}
}

func TestMergeUpstreamQueryAndWS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	mergeUpstreamQuery(req, url.Values{"q": {"1"}})
	if !strings.Contains(req.URL.RawQuery, "folder=") || !strings.Contains(req.URL.RawQuery, "q=1") {
		t.Fatalf("query=%q", req.URL.RawQuery)
	}
	req2 := httptest.NewRequest(http.MethodGet, "http://x/path", nil)
	mergeUpstreamQuery(req2, url.Values{"a": {"b"}})
	if strings.Contains(req2.URL.RawQuery, "folder=") {
		t.Fatal("non-root should not force folder")
	}

	ws := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	ws.Header.Set("Connection", "keep-alive, Upgrade")
	ws.Header.Set("Upgrade", "websocket")
	if !isWebSocketUpgrade(ws) {
		t.Fatal("want websocket")
	}
	if isWebSocketUpgrade(httptest.NewRequest(http.MethodGet, "http://x/", nil)) {
		t.Fatal("plain should be false")
	}
}

func TestSandboxMountPrefixAndCookieHelpers(t *testing.T) {
	if got := sandboxMountPrefix(7); got != "/sandbox/7" {
		t.Fatalf("mount=%q", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if requestHasUpstreamCookie(req) {
		t.Fatal("empty")
	}
	req.AddCookie(&http.Cookie{Name: "cf_session", Value: "x"})
	if requestHasUpstreamCookie(req) {
		t.Fatal("session cookie should not count as upstream")
	}
	req.AddCookie(&http.Cookie{Name: "key", Value: "y"})
	if !requestHasUpstreamCookie(req) {
		t.Fatal("upstream cookie")
	}
	c := mountedCookie(&http.Cookie{Name: "k", Value: "v", Path: "/"}, "/sandbox/1")
	if c.Path != "/sandbox/1/" {
		t.Fatalf("path=%q", c.Path)
	}
	if !shouldAutoLoginCodeServer(httptest.NewRequest(http.MethodGet, "/sandbox/1/", nil), 1) {
		t.Fatal("should auto login when no upstream cookie")
	}
	withKey := httptest.NewRequest(http.MethodGet, "/sandbox/1/", nil)
	withKey.AddCookie(&http.Cookie{Name: "key", Value: "y"})
	if shouldAutoLoginCodeServer(withKey, 1) {
		t.Fatal("with session cookie on non-login path should skip")
	}
	login := httptest.NewRequest(http.MethodGet, "/sandbox/1/login", nil)
	login.AddCookie(&http.Cookie{Name: "key", Value: "y"})
	if !shouldAutoLoginCodeServer(login, 1) {
		t.Fatal("login path should still auto login")
	}
}

func TestShouldAutoLoginVibe(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sandbox/1/acp/", nil)
	if !shouldAutoLoginVibe(req, "/sandbox/1/acp") {
		t.Fatal("no cookie should login")
	}
	req.AddCookie(&http.Cookie{Name: "key", Value: "x"})
	if shouldAutoLoginVibe(req, "/sandbox/1/acp") {
		t.Fatal("with cookie on non-login should skip")
	}
	login := httptest.NewRequest(http.MethodGet, "/sandbox/1/acp/login", nil)
	login.AddCookie(&http.Cookie{Name: "key", Value: "x"})
	if !shouldAutoLoginVibe(login, "/sandbox/1/acp") {
		t.Fatal("login path should login")
	}
}
