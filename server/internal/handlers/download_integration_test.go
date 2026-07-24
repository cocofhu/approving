package handlers_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestDownloadArtifactImageDecode(t *testing.T) {
	h := newHarness(t)
	pngMagic := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	b64 := base64.StdEncoding.EncodeToString(pngMagic)
	h.db.Create(&models.Artifact{
		ID: "img1", RunID: "r1", Name: "shot.png", Kind: "image",
		Content: b64, CreatedAt: time.Now(),
	})
	h.db.Create(&models.Artifact{
		ID: "md1", RunID: "r1", Name: "doc.md", Kind: "markdown",
		Content: "hello", CreatedAt: time.Now(),
	})

	w := h.do("GET", "/api/artifacts/img1/download", nil)
	if w.Code != 200 {
		t.Fatalf("download image: %d %s", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if got := w.Body.Bytes(); len(got) < 8 || string(got[:8]) != string(pngMagic) {
		t.Fatalf("response is not PNG binary: %v", got)
	}

	w = h.do("GET", "/api/artifacts/md1/download", nil)
	if w.Code != 200 || w.Body.String() != "hello" {
		t.Fatalf("download markdown: %d %s", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("markdown Content-Type = %q", ct)
	}
}

func TestArtifactContentDoesNotHydrateTestResult(t *testing.T) {
	h := newHarness(t)
	shotB64 := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47})
	h.db.Create(&models.Artifact{
		ID: "shot", RunID: "r1", Name: "capture.png", Kind: "image", Content: shotB64,
	})
	raw := `{"summary":"ok","screenshots":[{"artifact":"capture.png","caption":"home"}]}`
	h.db.Create(&models.Artifact{
		ID: "tr", RunID: "r1", Name: "test_result.json", Kind: "json", Content: raw,
	})

	w := h.do("GET", "/api/artifacts/tr/content", nil)
	if w.Code != 200 {
		t.Fatalf("content: %d %s", w.Code, w.Body)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	content, _ := resp["content"].(string)
	var doc map[string]any
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("doc: %v", err)
	}
	shots, _ := doc["screenshots"].([]any)
	if len(shots) != 1 {
		t.Fatalf("screenshots = %d", len(shots))
	}
	shot, _ := shots[0].(map[string]any)
	if shot["artifact"] != "capture.png" {
		t.Fatalf("artifact should be preserved for lazy-load, got %v", shot["artifact"])
	}
	if shot["data"] != nil && shot["data"] != "" {
		t.Fatal("ArtifactContent must not inject inline data")
	}

	var stored models.Artifact
	h.db.First(&stored, "id = ?", "tr")
	if stored.Content != raw {
		t.Fatalf("stored content was modified")
	}
}
