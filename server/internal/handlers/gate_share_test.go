package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/gateshare"
	"github.com/cocofhu/approving/internal/models"
)

func seedHumanGate(t *testing.T, h *harness, runID, nodeID string, actions []models.GateAction) {
	t.Helper()
	now := time.Now()
	if actions == nil {
		actions = []models.GateAction{
			{ID: "approve", Label: "批准"},
			{ID: "revise", Label: "驳回", RequireForm: true},
		}
	}
	h.db.Create(&models.WorkflowDef{
		ID: "wf-" + runID, ProjectID: models.DefaultProjectID, Name: "share-" + runID,
		Status: "published", Version: 1,
	})
	h.db.Create(&models.Run{
		ID: runID, WorkflowID: "wf-" + runID, WorkflowName: "share-" + runID, Status: "waiting_human",
		StartedAt: now,
		Graph: models.Graph{Nodes: []models.Node{{
			ID: nodeID, Type: "human_gate", Label: "审",
			Config: map[string]any{"title": "审阅", "body_template": "请审阅", "actions": []any{
				map[string]any{"id": "approve", "label": "批准"},
				map[string]any{"id": "revise", "label": "驳回", "requireForm": true},
			}},
		}}},
	})
	h.db.Create(&models.Gate{
		RunID: runID, NodeID: nodeID, Iteration: 1, WorkflowID: "wf-" + runID, WorkflowName: "share-" + runID,
		Title: "审阅视觉稿", BodyMd: "请审阅 page.html，勿泄露内部 URL http://10.1.2.3/api/runs/x",
		Actions: actions, Form: []models.GateField{{Key: "comment", Label: "意见"}},
		RequestedAt: now,
	})
	h.h.Arts.Save(runID, "visual", "page.html", "html", `<html><body><a href="/api/blobs/abc">x</a><p>ok</p></body></html>`)
	h.h.Arts.Save(runID, "visual", "clarified_requirement.json", "json", `{"title":"外部一次审批","goals":["g1"],"runId":"should-hide","projectId":"p"}`)
}

func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("json: %v body=%s", err, w.Body.String())
	}
	return m
}

func TestGateShareCreateRegenRevokeAndInboxStatus(t *testing.T) {
	h := newHarness(t)
	seedHumanGate(t, h, "run-share-1", "hg1", nil)

	w := h.do(http.MethodPost, "/api/runs/run-share-1/gates/hg1/share-link", map[string]any{"ttlTier": "24h"})
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	created := parseJSON(t, w)
	url, _ := created["url"].(string)
	if !strings.Contains(url, "/public/gate-approvals#t=") {
		t.Fatalf("url fragment: %s", url)
	}
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")
	if !gateshare.ValidTokenShape(token) {
		t.Fatalf("token shape: %s", token)
	}
	if strings.Contains(w.Body.String(), `"token"`) && !strings.Contains(url, token) {
		t.Fatal("unexpected token field")
	}

	st := parseJSON(t, h.do(http.MethodGet, "/api/runs/run-share-1/gates/hg1/share-link", nil))
	if st["state"] != models.ShareLinkStateActive {
		t.Fatalf("status: %+v", st)
	}
	if _, ok := st["url"]; ok {
		t.Fatalf("status leaked url: %+v", st)
	}

	list := h.do(http.MethodGet, "/api/gates", nil)
	if list.Code != 200 {
		t.Fatalf("list: %d", list.Code)
	}
	if !bytes.Contains(list.Body.Bytes(), []byte(`"shareLink"`)) {
		t.Fatalf("inbox missing shareLink: %s", list.Body.String())
	}
	if bytes.Contains(list.Body.Bytes(), []byte(token)) {
		t.Fatal("inbox leaked plaintext token")
	}

	regen := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-share-1/gates/hg1/share-link/regen", nil))
	newURL, _ := regen["url"].(string)
	if newURL == url {
		t.Fatal("regen returned same url")
	}
	oldToken := token
	newToken := strings.TrimPrefix(newURL[strings.Index(newURL, "#t="):], "#t=")

	// Old token must show revoked on public preview.
	prev := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: oldToken})
	if parseJSON(t, prev)["status"] != models.ShareLinkStateRevoked {
		t.Fatalf("old token after regen: %s", prev.Body.String())
	}
	prev2 := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: newToken})
	p2 := parseJSON(t, prev2)
	if p2["status"] != models.ShareLinkStateActive {
		t.Fatalf("new token preview: %s", prev2.Body.String())
	}
	if strings.Contains(prev2.Body.String(), "run-share-1") || strings.Contains(prev2.Body.String(), "should-hide") || strings.Contains(prev2.Body.String(), "/api/blobs") {
		t.Fatalf("preview leak: %s", prev2.Body.String())
	}
	if p2["visualHtml"] == nil || p2["nonce"] == "" {
		t.Fatalf("preview missing visual/nonce: %+v", p2)
	}

	if w := h.do(http.MethodPost, "/api/runs/run-share-1/gates/hg1/share-link/revoke", nil); w.Code != 200 {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	prev3 := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: newToken})
	if parseJSON(t, prev3)["status"] != models.ShareLinkStateRevoked {
		t.Fatalf("after revoke: %s", prev3.Body.String())
	}

	// Recreate after revoke while still pending.
	w = h.do(http.MethodPost, "/api/runs/run-share-1/gates/hg1/share-link", map[string]any{"ttlTier": "1h"})
	if w.Code != 200 {
		t.Fatalf("recreate: %d %s", w.Code, w.Body.String())
	}
}

func TestGateShareNoStandardActionAndUsedCannotRecreate(t *testing.T) {
	h := newHarness(t)
	seedHumanGate(t, h, "run-share-2", "hg2", []models.GateAction{{ID: "custom", Label: "自定义"}})
	w := h.do(http.MethodPost, "/api/runs/run-share-2/gates/hg2/share-link", map[string]any{"ttlTier": "24h"})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "标准") {
		t.Fatalf("no standard action: %d %s", w.Code, w.Body.String())
	}

	h2 := newHarness(t)
	seedHumanGate(t, h2, "run-share-3", "hg3", nil)
	created := parseJSON(t, h2.do(http.MethodPost, "/api/runs/run-share-3/gates/hg3/share-link", map[string]any{"ttlTier": "24h"}))
	url, _ := created["url"].(string)
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")
	nonce := publicPreviewNonce(t, h2, token)

	dec := h2.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "approve", "comment": "", "name": "Jordan", "nonce": nonce,
	}, map[string]string{
		headerShareRequest: "1",
		"Origin":           "http://" + publicHost,
	})
	if dec.Code != 200 {
		t.Fatalf("decide: %d %s", dec.Code, dec.Body.String())
	}
	body := parseJSON(t, dec)
	if body["status"] != "approved" {
		t.Fatalf("decide status: %+v", body)
	}

	w = h2.do(http.MethodPost, "/api/runs/run-share-3/gates/hg3/share-link", map[string]any{"ttlTier": "24h"})
	if w.Code != http.StatusConflict {
		t.Fatalf("used recreate: %d %s", w.Code, w.Body.String())
	}

	prev := h2.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	if parseJSON(t, prev)["status"] != models.ShareLinkStateUsed {
		t.Fatalf("used preview: %s", prev.Body.String())
	}

	// Same action retry → already processed
	nonce2 := publicPreviewNonce(t, h2, token)
	dec2 := h2.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "approve", "nonce": nonce2,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	b2 := parseJSON(t, dec2)
	if dec2.Code != 200 || b2["alreadyProcessed"] != true {
		t.Fatalf("idempotent: %d %s", dec2.Code, dec2.Body.String())
	}

	nonce3 := publicPreviewNonce(t, h2, token)
	dec3 := h2.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "revise", "comment": "nope", "nonce": nonce3,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	if dec3.Code != http.StatusConflict {
		t.Fatalf("conflict: %d %s", dec3.Code, dec3.Body.String())
	}

	var evs []models.ProjectAuditEvent
	h2.db.Where("action IN ?", []string{models.AuditActionGateShareCreate, models.AuditActionGateShareUse, models.AuditActionGateDecide}).Find(&evs)
	raw, _ := json.Marshal(evs)
	if bytes.Contains(raw, []byte(token)) {
		t.Fatal("audit leaked plaintext token")
	}
	foundExternal := false
	for _, ev := range evs {
		if ev.Action == models.AuditActionGateShareUse && ev.CallerKind == models.CallerKindExternal {
			foundExternal = true
			if strings.Contains(fmtPayload(ev.Payload), token) {
				t.Fatal("use payload leaked token")
			}
		}
	}
	if !foundExternal {
		t.Fatal("missing gate.share.use callerKind=external")
	}
}

func TestGateSharePublicSecurityHeadersCSRFAndRateLimit(t *testing.T) {
	h := newHarness(t)
	seedHumanGate(t, h, "run-share-4", "hg4", nil)
	created := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-share-4/gates/hg4/share-link", map[string]any{"ttlTier": "8h"}))
	url, _ := created["url"].(string)
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")

	page := h.doPublic(http.MethodGet, "/public/gate-approvals", nil, nil)
	if page.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("page cache: %s", page.Header().Get("Cache-Control"))
	}
	if page.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("referrer: %s", page.Header().Get("Referrer-Policy"))
	}
	if !strings.Contains(page.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("csp: %s", page.Header().Get("Content-Security-Policy"))
	}
	if acao := page.Header().Get("Access-Control-Allow-Origin"); acao != "" {
		t.Fatalf("page ACAO=%q", acao)
	}

	prev := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	if prev.Header().Get("Cache-Control") != "no-store" || prev.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("preview headers %+v", prev.Header())
	}
	if prev.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("preview ACAO leaked")
	}

	// CSRF: missing custom header
	bad := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "approve", "nonce": "x",
	}, map[string]string{"Origin": "http://" + publicHost})
	if bad.Code != http.StatusForbidden {
		t.Fatalf("csrf missing header: %d %s", bad.Code, bad.Body.String())
	}
	// CSRF: bad origin
	bad2 := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "approve", "nonce": "x",
	}, map[string]string{headerShareRequest: "1", "Origin": "https://evil.example"})
	if bad2.Code != http.StatusForbidden {
		t.Fatalf("csrf bad origin: %d %s", bad2.Code, bad2.Body.String())
	}

	// Cross-origin GET preview must not include ACAO (browser hides body).
	cross := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/public/gate-approvals/preview", nil)
	req.Header.Set(headerShareToken, token)
	req.Header.Set("Origin", "https://evil.example")
	h.r.ServeHTTP(cross, req)
	if cross.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("cross ACAO=%q", cross.Header().Get("Access-Control-Allow-Origin"))
	}

	// Unknown token → invalid, no run leak
	unk := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{
		headerShareToken: strings.Repeat("ab", 32),
	})
	u := parseJSON(t, unk)
	if u["status"] != "invalid" && u["status"] != models.ShareLinkStateNone {
		t.Fatalf("unknown: %+v", u)
	}
	if strings.Contains(unk.Body.String(), "run-share") || strings.Contains(unk.Body.String(), "wf-share") {
		t.Fatalf("unknown leak: %s", unk.Body.String())
	}

	// Rate limit
	limited := false
	for i := 0; i < 40; i++ {
		w := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
		if w.Code == http.StatusTooManyRequests {
			limited = true
			if strings.Contains(w.Body.String(), token) {
				t.Fatal("429 leaked token")
			}
			break
		}
	}
	if !limited {
		t.Fatal("expected rate limit")
	}
}

func TestGateShareConcurrentDecideIdempotent(t *testing.T) {
	h := newHarness(t)
	seedHumanGate(t, h, "run-share-5", "hg5", nil)
	created := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-share-5/gates/hg5/share-link", map[string]any{"ttlTier": "24h"}))
	url, _ := created["url"].(string)
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")

	var wg sync.WaitGroup
	results := make([]int, 8)
	wg.Add(8)
	for i := 0; i < 8; i++ {
		go func(i int) {
			defer wg.Done()
			nonce := publicPreviewNonce(t, h, token)
			w := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
				"token": token, "action": "approve", "nonce": nonce,
			}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
			results[i] = w.Code
		}(i)
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range results {
		if c == 200 {
			ok++
		} else if c == http.StatusConflict || c == http.StatusForbidden {
			conflict++
		}
	}
	if ok < 1 {
		t.Fatalf("no success among %v", results)
	}
	var gate models.Gate
	h.db.Where("run_id = ? AND node_id = ?", "run-share-5", "hg5").First(&gate)
	if !gate.Resolved {
		t.Fatal("gate not resolved after concurrent decide")
	}
}

func TestGateShareUnauthorizedCannotCreate(t *testing.T) {
	h := newHarness(t)
	seedHumanGate(t, h, "run-share-6", "hg6", nil)
	w := h.doWithCookie(http.MethodPost, "/api/runs/run-share-6/gates/hg6/share-link", map[string]any{"ttlTier": "24h"}, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth create: %d %s", w.Code, w.Body.String())
	}
}

const (
	headerShareToken   = "X-Gate-Share-Token"
	headerShareRequest = "X-Gate-Share-Requested"
	publicHost         = "example.test"
)

func (hn *harness) doPublic(method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Host = publicHost
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	hn.r.ServeHTTP(w, req)
	return w
}

func publicPreviewNonce(t *testing.T, h *harness, token string) string {
	t.Helper()
	w := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	m := parseJSON(t, w)
	n, _ := m["nonce"].(string)
	if n == "" && m["status"] == models.ShareLinkStateUsed {
		return "unused"
	}
	if n == "" {
		t.Fatalf("no nonce: %s", w.Body.String())
	}
	return n
}

func fmtPayload(p map[string]any) string {
	b, _ := json.Marshal(p)
	return string(b)
}
