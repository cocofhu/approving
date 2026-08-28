package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateProjectIDDoesNotTouchWorkspaceOrMCP(t *testing.T) {
	root := t.TempDir()
	s := NewAgentService(root)
	if err := s.Save(Agent{
		Name:      "keep-ws",
		ProjectID: "old-proj",
		MCP:       []MCPServer{{Name: "keep-me", URL: "http://x"}},
		Env:       map[string]string{"FOO": "bar"},
		Files:     []AgentFile{{Path: "AGENTS.md", Content: "# keep me\n"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteWorkspaceFile("keep-ws", "notes/a.md", "hello"); err != nil {
		t.Fatal(err)
	}
	beforeJSON, err := os.ReadFile(filepath.Join(root, "keep-ws", "agent.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateProjectID("keep-ws", "new-proj"); err != nil {
		t.Fatal(err)
	}

	got, ok := s.Get("keep-ws")
	if !ok {
		t.Fatal("agent missing")
	}
	if got.ProjectID != "new-proj" {
		t.Fatalf("projectId=%q", got.ProjectID)
	}
	if len(got.MCP) != 1 || got.MCP[0].Name != "keep-me" {
		t.Fatalf("mcp mutated: %+v", got.MCP)
	}
	if got.Env["FOO"] != "bar" {
		t.Fatalf("env mutated: %+v", got.Env)
	}
	note, err := s.ReadWorkspaceFile("keep-ws", "notes/a.md")
	if err != nil || note != "hello" {
		t.Fatalf("workspace cleared: %q err=%v", note, err)
	}
	md, err := s.ReadWorkspaceFile("keep-ws", "AGENTS.md")
	if err != nil || md != "# keep me\n" {
		t.Fatalf("AGENTS.md lost: %q err=%v", md, err)
	}
	afterJSON, err := os.ReadFile(filepath.Join(root, "keep-ws", "agent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(afterJSON), `"projectId": "new-proj"`) {
		t.Fatalf("agent.json missing new projectId: %s", afterJSON)
	}
	if !strings.Contains(string(afterJSON), `"keep-me"`) {
		t.Fatalf("agent.json mcp lost:\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
}

func TestUpdateProjectIDMissingAgent(t *testing.T) {
	s := NewAgentService(t.TempDir())
	if err := s.UpdateProjectID("ghost", "p1"); err == nil {
		t.Fatal("expected not found")
	}
}
