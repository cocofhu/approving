package services

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/sandbox"
	"github.com/cocofhu/approving/internal/sandbox/sandboxtest"
)

func TestParseKeepalivePID(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"OK pid=4242 detached=1", 4242},
		{"noise\nOK pid=9 detached=1\n", 9},
		{"OK pid=abc", 0},
		{"", 0},
		{"nope", 0},
	}
	for _, tc := range cases {
		if got := parseKeepalivePID(tc.in); got != tc.want {
			t.Fatalf("parseKeepalivePID(%q)=%d want %d", tc.in, got, tc.want)
		}
	}
}

func TestKeepalivePortSourceUsesSetsid(t *testing.T) {
	// Pin KeepalivePort implementation contract: setsid + pidfile + logfile,
	// not a nohup watch-only loop (regression vs the broken KeepalivePort).
	src, err := os.ReadFile("preview_keepalive.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, needle := range []string{"setsid", "pidfile", "logfile", "OK pid="} {
		if !strings.Contains(body, needle) {
			t.Fatalf("preview_keepalive.go missing %q", needle)
		}
	}
	if strings.Contains(body, "while kill -0") {
		t.Fatal("keepalive must not fall back to nohup watch-only loop")
	}
}

func TestKeepalivePortExecUsesSetsidScript(t *testing.T) {
	db := newTestDB(t)
	fg := sandboxtest.New(t)
	fg.Seed("sb-ka")
	mgr := sandbox.NewManager(fg.Client(), sandbox.ManagerOptions{WorkspaceDir: "/root/workspace"})
	svc := NewPreviewService(db, mgr)

	var sawScript string
	restore := sandbox.SetExecHook(func(_ context.Context, _ string, _ int, command string, stdin io.Reader) ([]byte, error) {
		if stdin != nil {
			b, _ := io.ReadAll(stdin)
			sawScript = string(b)
		}
		return []byte("OK pid=7777 detached=1\n"), nil
	})
	t.Cleanup(restore)

	pid, err := svc.KeepalivePort(context.Background(), "sb-ka", 8080)
	if err != nil {
		t.Fatalf("KeepalivePort: %v", err)
	}
	if pid != 7777 {
		t.Fatalf("pid=%d want 7777", pid)
	}
	if !strings.Contains(sawScript, "setsid") || !strings.Contains(sawScript, "pidfile") {
		t.Fatalf("script missing setsid/pidfile: %s", sawScript[:min(200, len(sawScript))])
	}
	if strings.Contains(sawScript, "while kill -0") {
		t.Fatal("must not use watch-only loop")
	}
}
