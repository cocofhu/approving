package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/config"
)

func TestPackConfigHomeTarGz(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"mcpServers":{}}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "rules", "base.md"), []byte("# base"), 0o644)

	gz, err := PackConfigHomeTarGz(dir)
	if err != nil {
		t.Fatal(err)
	}
	gr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	names := map[string]bool{}
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[strings.TrimSuffix(hdr.Name, "/")] = true
	}
	if !names["mcp.json"] || !names["rules/base.md"] {
		t.Fatalf("names=%v", names)
	}
}

func TestBundleStoreAuth(t *testing.T) {
	s := NewBundleStore()
	id, token := s.Put([]byte("hello-tgz"), DefaultInjectBundleTTL)
	if id == "" || token == "" {
		t.Fatal("empty id/token")
	}
	if _, ok := s.Get(id, "wrong"); ok {
		t.Fatal("wrong token accepted")
	}
	data, ok := s.Get(id+".tgz", token)
	if !ok || string(data) != "hello-tgz" {
		t.Fatalf("get = %q ok=%v", data, ok)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/sandbox-inject/", func(w http.ResponseWriter, r *http.Request) {
		s.ServeHTTP(w, r, strings.TrimPrefix(r.URL.Path, "/sandbox-inject/"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/sandbox-inject/"+id+".tgz", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello-tgz" {
		t.Fatalf("body=%q", body)
	}
}

func TestManagerCreateUsesBundleURLInject(t *testing.T) {
	prev := config.GetConfig()
	t.Cleanup(func() { config.StoreConfig(prev) })
	config.StoreConfig(&config.Config{Server: config.ServerConfig{MCPAdvertise: "http://api.example.com"}})

	gw, fg := newInlineGW(t)
	store := NewBundleStore()
	m := NewManager(gw, ManagerOptions{
		Image:           "img:test",
		WorkspaceDir:    "/root/workspace",
		InstallHelpers:  false, // no SSH; inject only
		InjectStore:     store,
		InjectAdvertise: "http://api.example.com",
	})

	home := t.TempDir()
	_ = os.MkdirAll(filepath.Join(home, "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(home, "mcp.json"), []byte(`{"mcpServers":{"a":{"url":"http://x"}}}`), 0o644)
	_ = os.WriteFile(filepath.Join(home, "rules", "base.md"), []byte("rule"), 0o644)

	sb, err := m.Create(context.Background(), Spec{
		Name:       "approving-sb-inj",
		ConfigHome: home,
		ConfigRoot: "/root/.cursor",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sb.ConfigRoot != "/root/.cursor" {
		t.Errorf("ConfigRoot=%q", sb.ConfigRoot)
	}
	cfg, _ := fg.lastCreate["config"].(map[string]any)
	if cfg == nil {
		t.Fatalf("create missing config: %+v", fg.lastCreate)
	}
	if cfg["configRoot"] != "/root/.cursor" {
		t.Errorf("configRoot=%v", cfg["configRoot"])
	}
	bundleURL, _ := cfg["bundleUrl"].(string)
	if !strings.HasPrefix(bundleURL, "http://api.example.com/sandbox-inject/") || !strings.HasSuffix(bundleURL, ".tgz") {
		t.Fatalf("bundleUrl=%q", bundleURL)
	}
	if cfg["hostPath"] != nil && cfg["hostPath"] != "" {
		t.Fatalf("hostPath must be empty for remote inject, got %v", cfg["hostPath"])
	}
	hdr, _ := cfg["headers"].(string)
	if !strings.HasPrefix(hdr, "Authorization: Bearer ") {
		t.Fatalf("headers=%q", hdr)
	}
}
