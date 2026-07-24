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
	agentJSON := `{"env":{"APPROVING_CURSOR_API_KEY":"agent-key","SHARED":"from-agent","AGENT_ONLY":"a1"}}`
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
					"CURSOR_API_KEY":    "should-skip-project",
					"TEMPLATED":         "${vars.region}",
					"ANTHROPIC_API_KEY": "skip-auth",
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
	if spec.Env["ANTHROPIC_API_KEY"] == "skip-auth" {
		t.Fatal("project must not inject ACP auth keys")
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
