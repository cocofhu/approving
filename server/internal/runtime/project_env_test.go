package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpecMergesProjectEnvBetweenPlatformAndAgent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	agentJSON := `{"env":{"SHARED":"from-agent","AGENT_ONLY":"a1","CURSOR_API_KEY":"agent-cursor"}}`
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), []byte(agentJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &acpProvider{
		opts: Options{
			Env: map[string]string{
				"PLATFORM_KEY":   "plat",
				"CURSOR_API_KEY": "should-skip-platform",
				"SHARED":         "from-platform",
			},
			ProjectEnvForWorkflow: func(workflowID string) map[string]string {
				if workflowID != "wf-1" {
					t.Fatalf("workflowID = %q", workflowID)
				}
				return map[string]string{
					"SHARED":            "from-project",
					"PROJECT_ONLY":      "p1",
					"CURSOR_API_KEY":    "project-cursor",
					"TEMPLATED":         "${vars.region}",
					"ANTHROPIC_API_KEY": "project-anthropic",
				}
			},
			ProfilesRoot: root,
		},
		backend: BackendCursor,
	}
	req := NodeReq{
		WorkflowID: "wf-1",
		NodeType:   "agent",
		Token:      "tok",
		Config:     map[string]any{"skill_profile": "demo"},
		Vars:       map[string]any{"region": "cn-east"},
	}
	spec, err := c.spec(req)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Env["PLATFORM_KEY"] != "plat" {
		t.Fatalf("platform = %q", spec.Env["PLATFORM_KEY"])
	}
	if spec.Env["PROJECT_ONLY"] != "p1" {
		t.Fatalf("project only = %q", spec.Env["PROJECT_ONLY"])
	}
	// Agent overlays project for SHARED.
	if spec.Env["SHARED"] != "from-agent" {
		t.Fatalf("shared (agent wins) = %q", spec.Env["SHARED"])
	}
	if spec.Env["AGENT_ONLY"] != "a1" {
		t.Fatalf("agent only = %q", spec.Env["AGENT_ONLY"])
	}
	if spec.Env["TEMPLATED"] != "cn-east" {
		t.Fatalf("templated = %q", spec.Env["TEMPLATED"])
	}
	// Project injects official ACP auth keys; Agent same-name key wins for Cursor.
	if spec.Env["ANTHROPIC_API_KEY"] != "project-anthropic" {
		t.Fatalf("project anthropic = %q", spec.Env["ANTHROPIC_API_KEY"])
	}
	if spec.Env["CURSOR_API_KEY"] != "agent-cursor" {
		t.Fatalf("agent cursor wins = %q", spec.Env["CURSOR_API_KEY"])
	}
	// Platform opts.Env auth keys remain skipped.
	if spec.Env["CURSOR_API_KEY"] == "should-skip-platform" {
		t.Fatal("platform CURSOR_API_KEY must stay skipped")
	}
}

func TestSpecProjectAuthKeyAloneSucceeds(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Agent has no Cursor auth key — project baseline must satisfy mergeAuthEnv.
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), []byte(`{"env":{"AGENT_ONLY":"a1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &acpProvider{
		opts: Options{
			Env: map[string]string{
				"CURSOR_API_KEY": "platform-must-skip",
			},
			ProjectEnvForWorkflow: func(string) map[string]string {
				return map[string]string{"CURSOR_API_KEY": "project-only-key"}
			},
			ProfilesRoot: root,
		},
		backend: BackendCursor,
	}
	spec, err := c.spec(NodeReq{
		WorkflowID: "wf-1",
		NodeType:   "agent",
		Token:      "tok",
		Config:     map[string]any{"skill_profile": "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Env["CURSOR_API_KEY"] != "project-only-key" {
		t.Fatalf("project-only auth = %q", spec.Env["CURSOR_API_KEY"])
	}
	if _, ok := spec.Env["AGENT_ONLY"]; !ok {
		t.Fatal("expected agent non-auth env")
	}
}

func TestSpecSkipsProjectEnvWithoutLookup(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), []byte(`{"env":{"APPROVING_CURSOR_API_KEY":"k"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &acpProvider{
		opts: Options{
			Env:          map[string]string{"P": "1"},
			ProfilesRoot: root,
		},
		backend: BackendCursor,
	}
	spec, err := c.spec(NodeReq{
		WorkflowID: "wf-1",
		NodeType:   "agent",
		Token:      "t",
		Config:     map[string]any{"skill_profile": "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := spec.Env["PROJECT_ONLY"]; ok {
		t.Fatal("unexpected project env")
	}
}
