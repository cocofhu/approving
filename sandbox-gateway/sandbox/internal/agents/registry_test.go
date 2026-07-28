package agents

import (
	"testing"

	"backend/internal/provider"
)

// allNames is every agent the sandbox advertises support for.
var allNames = []provider.Name{
	provider.Cursor, provider.ClaudeCode, provider.CodeBuddy, provider.Trae,
	provider.Kiro, provider.Qoder, provider.Grok, provider.Kimi, provider.Hermes,
	provider.Codex, provider.ClaudeStream, provider.Gemini, provider.OpenCode,
	provider.DevEco, provider.Copilot, provider.OpenClaw, provider.Antigravity, provider.Pi,
}

func TestEveryNameResolves(t *testing.T) {
	for _, n := range allNames {
		p, ok := registry[n]
		if !ok {
			t.Fatalf("unregistered agent: %s", n)
		}
		if p.Name() != n {
			t.Fatalf("agent %s reports Name()=%s", n, p.Name())
		}
		if p.Runtime() == "" || p.DefaultConfigRoot() == "" {
			t.Fatalf("agent %s missing runtime/configRoot", n)
		}
		// argv/handshake construction must not panic (long-lived vs one-shot).
		_ = p.Transport()
	}
}

func TestTransportSplit(t *testing.T) {
	// ACP-native CLIs run long-lived (persistent JSON-RPC over stdio). Every
	// other agent is one-shot (fresh process per turn + resume). cursor /
	// claude_code / codebuddy default to one-shot stream-json; their *_acp
	// aliases are the opt-in long-lived fallbacks.
	long := map[provider.Name]bool{
		provider.Trae: true, provider.Kiro: true, provider.Qoder: true,
		provider.Grok: true, provider.Kimi: true, provider.Hermes: true,
		provider.CursorACP: true, provider.ClaudeCodeACP: true, provider.CodeBuddyACP: true,
	}
	for n, p := range registry {
		want := provider.OneShot
		if long[n] {
			want = provider.LongLived
		}
		if p.Transport() != want {
			t.Fatalf("agent %s transport=%v want %v", n, p.Transport(), want)
		}
	}
}

func TestCodeBuddyKeepsStrictMcpIsolation(t *testing.T) {
	// Regression lock: codebuddy must keep --strict-mcp-config (isolation) and
	// a ConfigRoot so streamjson can attach --mcp-config <root>/mcp.json.
	// Without that pair, artifact-store from seeded mcp.json never reaches -p.
	p := registry[provider.CodeBuddy]
	if p.DefaultConfigRoot() != "/root/.codebuddy" {
		t.Fatalf("codebuddy ConfigRoot=%q", p.DefaultConfigRoot())
	}
	if p.Transport() != provider.OneShot {
		t.Fatalf("codebuddy transport=%v want oneshot", p.Transport())
	}
}

func TestClaudeFamilyDoesNotUseBareStrictMcp(t *testing.T) {
	// Only codebuddy opts into --strict-mcp-config (with streamjson auto --mcp-config).
	// Claude loads ~/.claude/mcp.json by default — do not add bare strict here.
	for _, n := range []provider.Name{provider.ClaudeCode, provider.ClaudeStream} {
		if registry[n].DefaultConfigRoot() == "" {
			t.Fatalf("%s missing ConfigRoot", n)
		}
	}
}

func TestFromEnvSelection(t *testing.T) {
	t.Setenv("AGENT_PROVIDER", "gemini")
	t.Setenv("ACP_BACKEND", "")
	if FromEnv() != provider.Gemini {
		t.Fatalf("AGENT_PROVIDER ignored: %s", FromEnv())
	}

	t.Setenv("AGENT_PROVIDER", "")
	t.Setenv("ACP_BACKEND", "claude_code")
	if FromEnv() != provider.ClaudeCode {
		t.Fatalf("ACP_BACKEND fallback broken: %s", FromEnv())
	}

	t.Setenv("AGENT_PROVIDER", "does-not-exist")
	t.Setenv("ACP_BACKEND", "")
	if FromEnv() != provider.Cursor {
		t.Fatalf("unknown provider should default to cursor: %s", FromEnv())
	}
}
