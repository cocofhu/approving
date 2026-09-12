package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentService_MigratesApprovingEnvKeys(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "legacy-cursor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"acpBackend":"cursor","env":{"APPROVING_CURSOR_API_KEY":"crsr_old","FEATURE_FLAG":"1"}}`)
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewAgentService(root)
	got, ok := s.Get("legacy-cursor")
	if !ok {
		t.Fatal("agent not found")
	}
	if got.Env["GRASP_CURSOR_API_KEY"] != "crsr_old" {
		t.Fatalf("env = %#v", got.Env)
	}
	if _, ok := got.Env["APPROVING_CURSOR_API_KEY"]; ok {
		t.Fatalf("legacy key still present: %#v", got.Env)
	}

	disk, err := os.ReadFile(filepath.Join(dir, "agent.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(disk, &cfg); err != nil {
		t.Fatal(err)
	}
	env, _ := cfg["env"].(map[string]any)
	if env["GRASP_CURSOR_API_KEY"] != "crsr_old" {
		t.Fatalf("disk env = %#v", env)
	}
	if _, ok := env["APPROVING_CURSOR_API_KEY"]; ok {
		t.Fatalf("disk still has legacy key: %#v", env)
	}
}

func TestAgentService_MigratesApprovingMCPInterpolations(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "legacy-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"mcp":[{"name":"artifact-store","url":"${APPROVING_ARTIFACT_URL}","headers":{"Authorization":"Bearer ${APPROVING_ARTIFACT_TOKEN}"}}]}`)
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewAgentService(root)
	got, ok := s.Get("legacy-mcp")
	if !ok || len(got.MCP) != 1 {
		t.Fatalf("agent=%#v", got)
	}
	if got.MCP[0].URL != "${GRASP_ARTIFACT_URL}" {
		t.Fatalf("url = %q", got.MCP[0].URL)
	}
	if got.MCP[0].Headers["Authorization"] != "Bearer ${GRASP_ARTIFACT_TOKEN}" {
		t.Fatalf("headers = %#v", got.MCP[0].Headers)
	}
}

func TestAgentService_SaveRewritesApprovingEnvKeys(t *testing.T) {
	s := NewAgentService(t.TempDir())
	if err := s.Save(Agent{
		Name:       "demo",
		AcpBackend: AcpBackendCursor,
		Env:        map[string]string{"APPROVING_CURSOR_API_KEY": "k"},
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get("demo")
	if !ok {
		t.Fatal("missing")
	}
	if got.Env["GRASP_CURSOR_API_KEY"] != "k" {
		t.Fatalf("env = %#v", got.Env)
	}
}
