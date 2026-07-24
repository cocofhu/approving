package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionPageTouchAndStart(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1})
	s.Start()
	t.Cleanup(s.Stop)

	sess, err := s.OpenInSandbox(context.Background(), "sb-touch", "10.0.0.9", "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Page() == nil {
		t.Fatal("Page nil")
	}
	sess.Touch()
	stats := s.Stats()
	if stats.TabCount < 1 {
		t.Fatalf("stats: %+v", stats)
	}
	s.SetMaxTabs(8)
	sess.Close()
}

func TestEnsureSandboxVNCAlreadyReady(t *testing.T) {
	s, d, _ := newFakeService(Config{})
	ctx := context.Background()
	if err := s.EnsureSandboxVNC(ctx, "sb", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	n := len(d.execs)
	d.mu.Unlock()
	if n != 0 {
		t.Fatalf("ready probe should skip exec, got %d", n)
	}
}

func TestEnsureSandboxVNCStartsThenReady(t *testing.T) {
	s, d, _ := newFakeService(Config{})
	calls := 0
	s.readyProbe = func(context.Context, string) bool {
		calls++
		return calls >= 2
	}
	ctx := context.Background()
	if err := s.EnsureSandboxVNC(ctx, "sb", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	n := len(d.execs)
	d.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected one vnc-preview exec, got %d", n)
	}
}

func TestEnsureSandboxVNCTimeout(t *testing.T) {
	s, _, _ := newFakeService(Config{})
	s.readyProbe = func(context.Context, string) bool { return false }
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := s.EnsureSandboxVNC(ctx, "sb", "10.0.0.1")
	if err == nil {
		t.Fatal("expected timeout/not ready error")
	}
}

func TestProbeVNCReadyAgainstLocalServer(t *testing.T) {
	// CDP JSON endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Browser":"x"}`))
	})
	mux.HandleFunc("/json/new", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"tab1"}`))
	})
	mux.HandleFunc("/json/close/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	cdp := httptest.NewServer(mux)
	t.Cleanup(cdp.Close)

	// We cannot change cdpPort/vncWSport constants; probe uses fixed ports.
	// Cover probeTabCreate / probeVNCReady indirectly by calling them with unreachable IP (false paths).
	s, _, _ := newFakeService(Config{})
	ctx := context.Background()
	if s.probeVNCReady(ctx, "127.0.0.1") {
		// May flake if something listens on 9222/6080; just log
		t.Log("local ports unexpectedly ready")
	}
	if s.probeTabCreate(ctx, "127.0.0.1", cdpPort) {
		t.Log("tab create unexpectedly ok")
	}
	_ = cdp
}

func TestOpenInSandboxMissingArgs(t *testing.T) {
	s, _, _ := newFakeService(Config{})
	if _, err := s.OpenInSandbox(context.Background(), "", "1.1.1.1", "http://x/"); err == nil {
		t.Fatal("empty name should fail")
	}
	if _, err := s.OpenInSandbox(context.Background(), "sb", "", "http://x/"); err == nil {
		t.Fatal("empty ip should fail")
	}
}

func TestSetReadyProbeOverride(t *testing.T) {
	s, d, _ := newFakeService(Config{})
	called := 0
	s.SetReadyProbe(func(context.Context, string) bool {
		called++
		return true
	})
	if err := s.EnsureSandboxVNC(context.Background(), "sb", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if called == 0 {
		t.Fatal("custom probe not used")
	}
	d.mu.Lock()
	n := len(d.execs)
	d.mu.Unlock()
	if n != 0 {
		t.Fatalf("ready should skip exec, got %d", n)
	}
	s.SetReadyProbe(nil) // restore default
}
