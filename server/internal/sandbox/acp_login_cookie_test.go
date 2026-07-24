package sandbox

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestBridgeLoginAcceptsAgentchatSessionCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/login" {
			http.NotFound(w, r)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "agentchat_session", Value: "tok123", Path: "/"})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	cookie, err := bridgeLogin(context.Background(), host, port, "secret")
	if err != nil {
		t.Fatalf("bridgeLogin: %v", err)
	}
	if cookie != "agentchat_session=tok123" {
		t.Fatalf("cookie = %q", cookie)
	}
}

func TestBridgeLoginAcceptsLegacyCursorCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "cursor_acp_session", Value: "legacy", Path: "/"})
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	cookie, err := bridgeLogin(context.Background(), host, port, "secret")
	if err != nil || cookie != "cursor_acp_session=legacy" {
		t.Fatalf("cookie=%q err=%v", cookie, err)
	}
}
