package platformmcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRPCHelpers(t *testing.T) {
	req := RPCRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "x"}
	st, body := Ok(req, map[string]any{"ok": true})
	if st != 200 || !strings.Contains(string(body), `"ok"`) {
		t.Fatalf("Ok: %d %s", st, body)
	}
	st, body = Fail(req, -1, "nope")
	if st != 200 || !strings.Contains(string(body), "nope") {
		t.Fatalf("Fail: %s", body)
	}
	st, body = Unauthorized()
	if st != 401 {
		t.Fatalf("Unauthorized: %d", st)
	}
	st, body = ParseError()
	if st != 400 {
		t.Fatalf("ParseError: %d", st)
	}

	if MustJSON(map[string]any{"a": 1}) == nil {
		t.Fatal("MustJSON")
	}
	if MustString("hi") != "hi" || !strings.Contains(MustString(map[string]any{"k": "v"}), "k") {
		t.Fatal("MustString")
	}
	tool := Tool("t", "desc", map[string]any{"x": map[string]any{"type": "string"}})
	if tool["name"] != "t" {
		t.Fatalf("Tool=%v", tool)
	}
	res := ToolResult("out", true)
	if res["isError"] != true {
		t.Fatalf("ToolResult=%v", res)
	}

	args := map[string]any{"s": "v", "n": float64(3), "b": true, "m": map[string]any{"k": "v"}}
	if StrArg(args, "s") != "v" || StrArg(nil, "s") != "" {
		t.Fatal("StrArg")
	}
	if IntArg(args, "n", 0) != 3 || IntArg(args, "missing", 7) != 7 {
		t.Fatal("IntArg")
	}
	if !BoolArg(args, "b") || BoolArg(args, "missing") {
		t.Fatal("BoolArg")
	}
	if MapArg(args, "m")["k"] != "v" || MapArg(args, "missing") != nil {
		t.Fatal("MapArg")
	}
	tok := NewToken()
	if len(tok) != 32 {
		t.Fatalf("NewToken len=%d", len(tok))
	}
	if !IsNotification(RPCRequest{}) || IsNotification(req) {
		t.Fatal("IsNotification")
	}
}
