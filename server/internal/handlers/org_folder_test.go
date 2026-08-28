package handlers_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/auth"
	"github.com/cocofhu/approving/internal/services"
)

func TestExportImportOrgFolderHTTP(t *testing.T) {
	hn := newHarness(t)
	root := t.TempDir()
	skills := services.NewAgentService(root)
	hn.h.Agents = skills
	hn.h.Org = services.NewOrgService(root, skills)

	if err := skills.Save(services.Agent{
		Name:       "alice",
		AcpBackend: services.AcpBackendClaudeCode,
		Files:      []services.AgentFile{{Path: "rules/a.md", Content: "hello"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := hn.h.Org.Put(services.AgentOrg{
		Groups: []services.OrgGroup{{ID: "g1", Name: "Approving项目组"}},
		Agents: map[string]services.OrgAgentMembership{
			"alice": {GroupIDs: []string{"g1"}},
		},
	}, 0); err != nil {
		t.Fatal(err)
	}

	if w := hn.do(http.MethodGet, "/api/agents/org/export", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("missing groupId: %d %s", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/agents/org/export?groupId=g1", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: hn.cookie})
	w := httptest.NewRecorder()
	hn.r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("export: %d %s", w.Code, w.Body.String())
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="`) || !strings.Contains(cd, "filename*=UTF-8''") {
		t.Fatalf("Content-Disposition missing quote/RFC5987: %s", cd)
	}
	if !strings.Contains(cd, "Approving") {
		t.Fatalf("download name should keep CJK group title: %s", cd)
	}
	zipBytes := append([]byte(nil), w.Body.Bytes()...)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "folder.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(zipBytes); err != nil {
		t.Fatal(err)
	}
	_ = mw.WriteField("mode", "rename")
	_ = mw.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/api/agents/org/import", &buf)
	req2.Header.Set("Content-Type", mw.FormDataContentType())
	req2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: hn.cookie})
	w2 := httptest.NewRecorder()
	hn.r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w2.Code, w2.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["org"]; !ok {
		t.Fatalf("import response missing org: %s", w2.Body.String())
	}

	// Single-agent ZIP must be rejected on group import (zero write beyond rename already done).
	var singleBuf bytes.Buffer
	zw := multipart.NewWriter(&singleBuf)
	sfw, _ := zw.CreateFormFile("file", "agent.zip")
	_, _ = sfw.Write([]byte("PK\x03\x04not-a-folder"))
	_ = zw.WriteField("mode", "rename")
	_ = zw.Close()
	req3 := httptest.NewRequest(http.MethodPost, "/api/agents/org/import", &singleBuf)
	req3.Header.Set("Content-Type", zw.FormDataContentType())
	req3.AddCookie(&http.Cookie{Name: auth.CookieName, Value: hn.cookie})
	w3 := httptest.NewRecorder()
	hn.r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("single/invalid zip: %d %s", w3.Code, w3.Body.String())
	}
}

func TestImportOrgFolderRejectsSingleAgentZIP(t *testing.T) {
	hn := newHarness(t)
	root := t.TempDir()
	skills := services.NewAgentService(root)
	hn.h.Agents = skills
	hn.h.Org = services.NewOrgService(root, skills)
	if err := skills.Save(services.Agent{Name: "solo", Files: []services.AgentFile{{Path: "rules/a.md", Content: "a"}}}); err != nil {
		t.Fatal(err)
	}
	raw, err := skills.ExportZIP("solo")
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "solo.zip")
	_, _ = fw.Write(raw)
	_ = mw.WriteField("mode", "rename")
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/org/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: hn.cookie})
	w := httptest.NewRecorder()
	hn.r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("single agent import: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "单 Agent ZIP") {
		t.Fatalf("error body: %s", w.Body.String())
	}
}
