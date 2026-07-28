package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSetPreviewGateAndUpsert(t *testing.T) {
	h := NewHost(&memStore{})
	h.SetPreviewBaseURL("http://app.example.com")
	h.SetPreviewSandboxOps(&fakePreviewOps{name: "sb", ok: true, healthy: true, up: "http://10.0.0.1:5173"})
	tok := h.RegisterRun("r1")
	h.SetActiveNode("r1", "preview1", "app_preview")

	resp := callPreviewTool(t, h, "r1", tok, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_preview","arguments":{"port":5173,"label":"前端"}}}`)
	text := previewResultText(t, resp)
	if text == "" || previewTextIsError(text) {
		t.Fatalf("set_preview failed: %s", text)
	}
	if !h.HasPreviewPorts("r1", "preview1") {
		t.Fatal("expected preview port registered")
	}
	ports := h.ListPreviewPorts("r1", "preview1")
	if len(ports) != 1 || ports[0].Port != 5173 || ports[0].Label != "前端" {
		t.Fatalf("unexpected ports: %+v", ports)
	}
	if !ports[0].Healthy || ports[0].KeepalivePID != 4242 {
		t.Fatalf("want healthy+keepalive pid, got %+v", ports[0])
	}
	wantURL := "/preview/r1/preview1/5173/"
	if ports[0].ProxyURL != wantURL {
		t.Fatalf("proxy url = %q, want %q", ports[0].ProxyURL, wantURL)
	}
	if strings.Contains(ports[0].ProxyURL, "://") {
		t.Fatalf("proxy url must be relative, got %q", ports[0].ProxyURL)
	}

	resp2 := callPreviewTool(t, h, "r1", tok, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"set_preview","arguments":{"port":5173,"label":"Web"}}}`)
	if previewTextIsError(previewResultText(t, resp2)) {
		t.Fatal("upsert failed")
	}
	ports = h.ListPreviewPorts("r1", "preview1")
	if len(ports) != 1 || ports[0].Label != "Web" {
		t.Fatalf("upsert label: %+v", ports)
	}

	h.SetActiveNode("r1", "preview1", "implement")
	bad := callPreviewTool(t, h, "r1", tok, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"set_preview","arguments":{"port":8080}}}`)
	if !previewResultIsError(bad) {
		t.Fatal("expected set_preview rejected on implement node")
	}
}

func TestSetPreviewUnreachableFails(t *testing.T) {
	h := NewHost(&memStore{})
	h.SetPreviewSandboxOps(&fakePreviewOps{name: "sb", ok: true, healthy: false, up: "http://10.0.0.1:9"})
	tok := h.RegisterRun("r1")
	h.SetActiveNode("r1", "preview1", "app_preview")
	resp := callPreviewTool(t, h, "r1", tok, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_preview","arguments":{"port":9}}}`)
	if !previewResultIsError(resp) {
		t.Fatal("expected unreachable set_preview to fail")
	}
	if h.HasHealthyPreviewPorts("r1", "preview1") {
		t.Fatal("unreachable port must not count as healthy preview")
	}
}

func callPreviewTool(t *testing.T, h *Host, runID, tok, body string) map[string]any {
	t.Helper()
	_, resp := h.ServeRPC(runID, tok, []byte(body))
	var out map[string]any
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func previewResultText(t *testing.T, resp map[string]any) string {
	t.Helper()
	r, _ := resp["result"].(map[string]any)
	if r == nil {
		return ""
	}
	content, _ := r["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	m, _ := content[0].(map[string]any)
	return m["text"].(string)
}

func previewResultIsError(resp map[string]any) bool {
	r, _ := resp["result"].(map[string]any)
	return r != nil && r["isError"] == true
}

func previewTextIsError(s string) bool {
	return len(s) >= 5 && (s[:5] == "error" || len(s) >= 18 && s[:18] == "set_preview failed")
}
