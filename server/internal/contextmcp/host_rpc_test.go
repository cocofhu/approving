package contextmcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextMCPServeRPCMetaMethods(t *testing.T) {
	_, pm, p := setupContextDB(t)
	h := NewHost(pm)
	tok := h.Register(p.ID, "agent-a", "thr", "alice")

	st, _ := h.ServeRPC(p.ID, "bad", []byte(`{}`))
	if st != 401 {
		t.Fatalf("unauth: %d", st)
	}
	st, _ = h.ServeRPC(p.ID, tok, []byte(`{`))
	if st != 400 {
		t.Fatalf("parse: %d", st)
	}

	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
	})
	st, resp := h.ServeRPC(p.ID, tok, initBody)
	if st != 200 || !strings.Contains(string(resp), "context-store") {
		t.Fatalf("init: %d %s", st, resp)
	}
	st, resp = h.ServeRPC(p.ID, tok, []byte(`{"jsonrpc":"2.0","id":2,"method":"ping"}`))
	if st != 200 {
		t.Fatalf("ping: %d %s", st, resp)
	}
	st, resp = h.ServeRPC(p.ID, tok, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if st != 202 {
		t.Fatalf("notification: %d", st)
	}
	st, resp = h.ServeRPC(p.ID, tok, []byte(`{"jsonrpc":"2.0","id":3,"method":"nope"}`))
	if st != 200 || !strings.Contains(string(resp), "method not found") {
		t.Fatalf("nope: %d %s", st, resp)
	}
}
