package sandbox

import "testing"

func TestApplyPasswords(t *testing.T) {
	env := map[string]string{}
	ApplyPasswords(env, "  secret  ")
	if env["PASSWORD"] != "secret" || env["ROOT_PASSWORD"] != "secret" {
		t.Fatalf("env=%v", env)
	}
	// Unified auth: every exposed service (incl. the acp-bridge) requires the
	// same token; the platform's ACP client logs in with it before dialing /ws.
	if env["CURSOR_ACP_PASSWORD"] != "secret" {
		t.Fatalf("CURSOR_ACP_PASSWORD must be set: %v", env)
	}
	if env["ACP_BRIDGE_PASSWORD"] != "secret" {
		t.Fatalf("ACP_BRIDGE_PASSWORD must be set: %v", env)
	}
	ApplyPasswords(env, "")
	if env["PASSWORD"] != "secret" {
		t.Fatal("empty password should not clear")
	}
	ApplyPasswords(nil, "x") // no panic
}
