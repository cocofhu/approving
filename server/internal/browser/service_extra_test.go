package browser

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOpenInSandboxDialAndTabFailures(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1})
	s.dial = func(context.Context, string) (Engine, error) {
		return nil, errors.New("dial boom")
	}
	if _, err := s.OpenInSandbox(context.Background(), "sb", "10.0.0.1", "http://x/"); err == nil {
		t.Fatal("expected dial failure")
	}

	s2, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1})
	s2.dial = func(context.Context, string) (Engine, error) {
		return &fakeEngine{failTab: true}, nil
	}
	if _, err := s2.OpenInSandbox(context.Background(), "sb", "10.0.0.1", "http://x/"); err == nil {
		t.Fatal("expected permanent NewTab failure")
	}
}

func TestEnsureSandboxVNCExecFailsStillProbes(t *testing.T) {
	s, d, _ := newFakeService(Config{})
	d.execErr = errors.New("exec failed")
	calls := 0
	s.readyProbe = func(context.Context, string) bool {
		calls++
		return calls >= 3 // first Ensure check false, then after exec loop true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.EnsureSandboxVNC(ctx, "sb", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	n := len(d.execs)
	d.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected one exec despite error, got %d", n)
	}
}

func TestCloseSessionAndTouchMissing(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1})
	s.CloseSession("missing")
	s.touch("missing")
	sess := open(t, s, "sb-a")
	s.CloseSession(sess.ID)
	select {
	case <-sess.Done():
	default:
		t.Fatal("expected closed")
	}
	if sess.Reason() != "closed" {
		t.Fatalf("reason=%q", sess.Reason())
	}
}

func TestStartStopIdempotent(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1, TabIdleTTL: time.Hour, ContainerIdleTTL: time.Hour})
	s.Start()
	sess := open(t, s, "sb-a")
	s.Stop()
	select {
	case <-sess.Done():
	default:
		t.Fatal("expected shutdown close")
	}
	if sess.Reason() != "shutdown" {
		t.Fatalf("reason=%q", sess.Reason())
	}
	s.Stop() // second stop is safe via sync.Once
}
