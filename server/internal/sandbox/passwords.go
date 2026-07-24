package sandbox

import "strings"

// ApplyPasswords sets the password env vars the universal-sandbox image
// recognizes so direct host:port access (not via the platform proxy) requires
// the same secret everywhere:
//
//   - PASSWORD              → code-server (port 8744) login (shell alias of ROOT_PASSWORD)
//   - ROOT_PASSWORD         → root shell / SSH password fallback
//   - ACP_BRIDGE_PASSWORD   → acp-bridge (port 8765) /ws + UI login (canonical)
//   - CURSOR_ACP_PASSWORD   → deprecated alias for ACP_BRIDGE_PASSWORD (remove 0.2.0)
//
// Auth is unified: every exposed service requires the sandbox token. The
// platform's own ACP client (see ACPClient.WithPassword) and WaitForACPReady
// log in with this same token (POST /api/login → session cookie)
// before dialing /ws, so enabling ACP auth no longer locks the platform out.
func ApplyPasswords(env map[string]string, password string) {
	if env == nil {
		return
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return
	}
	env["PASSWORD"] = password
	env["ROOT_PASSWORD"] = password
	env["ACP_BRIDGE_PASSWORD"] = password
	env["CURSOR_ACP_PASSWORD"] = password
}
