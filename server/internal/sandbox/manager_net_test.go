package sandbox

import (
	"context"
	"testing"
)

func TestManagerNetworkHelpers(t *testing.T) {
	gw, fg := newInlineGW(t)
	fg.seed("net-1", "running")
	// Seed a numeric app-port endpoint so HostForPort hits the endpoints map
	// before the /hosts/ API (not implemented on inlineGW).
	fg.mu.Lock()
	if rec := fg.recs["net-1"]; rec != nil {
		eps := map[string]string{
			"session": "10.0.0.3:34567",
			"ide":     "10.0.0.3:34568",
			"ssh":     "10.0.0.3:2222",
			"3000":    "10.0.0.3:3000",
		}
		rec["endpoints"] = eps
	}
	fg.mu.Unlock()
	m := NewManager(gw, ManagerOptions{WorkspaceDir: "/root/workspace"})
	ctx := context.Background()

	if m.Gateway() == nil {
		t.Fatal("Gateway nil")
	}

	ip, err := m.ContainerIP(ctx, "net-1")
	if err != nil || ip == "" {
		t.Fatalf("ContainerIP: %q %v", ip, err)
	}
	addr, err := m.HostForPort(ctx, "net-1", 3000)
	if err != nil || addr == "" {
		t.Fatalf("HostForPort: %q %v", addr, err)
	}
	sess, err := m.EndpointAddr(ctx, "net-1", "session")
	if err != nil || sess == "" {
		t.Fatalf("EndpointAddr: %q %v", sess, err)
	}
	if _, err := m.EndpointAddr(ctx, "net-1", ""); err == nil {
		t.Fatal("empty key")
	}
	if _, err := m.EndpointAddr(ctx, "missing", "session"); err == nil {
		t.Fatal("missing endpoint")
	}
	if _, err := m.ContainerIP(ctx, "missing"); err == nil {
		t.Fatal("missing ip")
	}
	logs, err := m.Logs(ctx, "net-1", 100)
	if err != nil || logs != "" {
		t.Fatalf("Logs: %q %v", logs, err)
	}
	fg.mu.Lock()
	fg.recs["net-1"]["logs"] = "live-from-gw"
	fg.mu.Unlock()
	logs, err = m.Logs(ctx, "net-1", 100)
	if err != nil || logs != "live-from-gw" {
		t.Fatalf("Logs content: %q %v", logs, err)
	}

	m2 := NewManager(nil, ManagerOptions{})
	if _, err := m2.HostForPort(ctx, "x", 1); err == nil {
		t.Fatal("nil gw HostForPort")
	}
	if _, err := m2.EndpointAddr(ctx, "x", "session"); err == nil {
		t.Fatal("nil gw EndpointAddr")
	}
	if _, err := m2.Logs(ctx, "x", 10); err == nil {
		t.Fatal("nil gw Logs should error")
	}

	// HostForPort falls back to /hosts/:port when numeric endpoint is absent.
	gw3, fg3 := newInlineGW(t)
	fg3.seed("net-2", "running")
	m3 := NewManager(gw3, ManagerOptions{WorkspaceDir: "/root/workspace"})
	addr2, err := m3.HostForPort(ctx, "net-2", 5173)
	if err != nil || addr2 != "127.0.0.1:5173" {
		t.Fatalf("hosts fallback: %q %v", addr2, err)
	}
}
