package sandbox

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/config"
)

func TestLiveStartWithPasswordsAndInject(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("APPROVING_LIVE_GATEWAY"))
	if base == "" {
		t.Skip("set APPROVING_LIVE_GATEWAY")
	}
	store := NewBundleStore()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sandbox-inject/", func(w http.ResponseWriter, r *http.Request) {
		store.ServeHTTP(w, r, strings.TrimPrefix(r.URL.Path, "/sandbox-inject/"))
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close() })

	advertise := strings.TrimSpace(os.Getenv("APPROVING_LIVE_INJECT_ADVERTISE"))
	if advertise == "" {
		advertise = liveReachableAdvertise(t, ln.Addr().(*net.TCPAddr).Port)
	}
	t.Log("advertise", advertise)
	prev := config.GetConfig()
	t.Cleanup(func() { config.StoreConfig(prev) })
	config.StoreConfig(&config.Config{Server: config.ServerConfig{MCPAdvertise: advertise}})

	home := t.TempDir()
	_ = os.MkdirAll(filepath.Join(home, "rules"), 0755)
	_ = os.WriteFile(filepath.Join(home, "mcp.json"), []byte(`{"mcpServers":{}}`), 0644)
	_ = os.WriteFile(filepath.Join(home, "rules", "base.md"), []byte("x"), 0644)

	m := NewManager(NewGatewayClient(base, os.Getenv("APPROVING_SANDBOX_GATEWAY_API_KEY")), ManagerOptions{
		WorkspaceDir: "/root/workspace", InstallHelpers: true,
		InjectStore: store, InjectAdvertise: advertise,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	env := map[string]string{"SKIP_INNER_DOCKER": "1"}
	ApplyPasswords(env, "probe-token-xyz")
	t0 := time.Now()
	sb, err := m.Create(ctx, Spec{
		Name: NewContainerName(), ConfigHome: home, ConfigRoot: "/root/.cursor", Env: env,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = m.DestroyByName(context.Background(), sb.ID) })
	t.Logf("create_ok id=%s elapsed=%s session=%s:%d", sb.ID, time.Since(t0), sb.Host, sb.Port)

	if sb.Password != "probe-token-xyz" {
		t.Fatalf("sb.Password not propagated: %q", sb.Password)
	}
	// Unified auth (WS layer): the bridge must reject an unauthenticated /ws
	// dial and accept one carrying the login cookie. This is exactly what the
	// platform's ACP client + WaitForACPReady rely on. (The in-container
	// cursor-agent's own session/new login needs a real CURSOR_API_KEY, which a
	// bare probe sandbox lacks, so we assert the bridge auth, not agent auth.)
	if err := WaitForACPReady(ctx, sb.Host, sb.Port, "", 15*time.Second); err == nil {
		t.Fatal("bridge accepted unauthenticated /ws dial; expected auth to be enforced")
	}
	t.Log("unauthenticated /ws dial rejected (auth enforced)")
	if err := WaitForACPReady(ctx, sb.Host, sb.Port, sb.Password, 2*time.Minute); err != nil {
		t.Fatalf("WaitForACPReady (authed): %v", err)
	}
	t.Log("authenticated /ws dial accepted (acp ready)")

	out, err := sb.creds().run(ctx, 20*time.Second,
		`tr '\0' '\n' < /proc/1/environ | grep -E '^(PASSWORD|ROOT_PASSWORD|ACP_BRIDGE_PASSWORD|CURSOR_ACP_PASSWORD|SANDBOX_INJECT)=' | sort`)
	if err != nil {
		t.Fatalf("environ: %v", err)
	}
	envOut := string(out)
	t.Log(envOut)
	for _, k := range []string{"PASSWORD=probe-token-xyz", "ROOT_PASSWORD=probe-token-xyz", "ACP_BRIDGE_PASSWORD=probe-token-xyz", "CURSOR_ACP_PASSWORD=probe-token-xyz"} {
		if !strings.Contains(envOut, k) {
			t.Fatalf("missing %s in:\n%s", k, envOut)
		}
	}
	if !strings.Contains(envOut, "SANDBOX_INJECT=") {
		t.Fatalf("missing SANDBOX_INJECT:\n%s", envOut)
	}
	raw, err := sb.creds().run(ctx, 20*time.Second, "test -f /root/.cursor/mcp.json && echo OK")
	if err != nil || !strings.Contains(string(raw), "OK") {
		t.Fatalf("mcp.json missing after inject: %v %s", err, raw)
	}
}
