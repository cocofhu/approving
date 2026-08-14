package previewinject

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func scriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	p := filepath.Join(filepath.Dir(file), "..", "..", "scripts", "preview-inject.sh")
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func runScript(t *testing.T, env []string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", scriptPath(t))
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestPreviewInjectSh_NoDirectExits0(t *testing.T) {
	out, err := runScript(t, nil)
	if err != nil {
		t.Fatalf("exit: %v out=%s", err, out)
	}
	if !strings.Contains(out, "PREVIEW_DIRECT not set") {
		t.Fatalf("out=%s", out)
	}
}

func TestPreviewInjectSh_MissingPortExits0(t *testing.T) {
	out, err := runScript(t, []string{
		"PREVIEW_DIRECT=1",
		"PREVIEW_PICK_SCRIPT_URL=http://x/preview-pick.js",
	})
	if err != nil {
		t.Fatalf("exit: %v out=%s", err, out)
	}
	if !strings.Contains(out, "missing") {
		t.Fatalf("out=%s", out)
	}
}

func TestPreviewInjectSh_DryRunRules(t *testing.T) {
	out, err := runScript(t, []string{
		"PREVIEW_DIRECT=1",
		"PREVIEW_PORT=18080",
		"PREVIEW_PICK_SCRIPT_URL=http://x/preview-pick.js",
		"PREVIEW_INJECT_DRY_RUN=1",
	})
	if err != nil {
		t.Fatalf("exit: %v out=%s", err, out)
	}
	for _, want := range []string{
		"APPROVING-PREVIEW",
		"--dport 18080",
		"--to-ports 17980",
		"REDIRECT",
		"approving-preview-inject",
		"PREROUTING",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestPreviewInjectSh_DryRunIdempotent(t *testing.T) {
	env := []string{
		"PREVIEW_DIRECT=1",
		"PREVIEW_PORT=18111",
		"PREVIEW_PICK_SCRIPT_URL=http://x/preview-pick.js",
		"PREVIEW_INJECT_DRY_RUN=1",
	}
	a, err := runScript(t, env)
	if err != nil {
		t.Fatal(err)
	}
	b, err := runScript(t, env)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("dry-run not stable:\n%s\n---\n%s", a, b)
	}
}

func TestPreviewInjectSh_AutoInjectOff(t *testing.T) {
	out, err := runScript(t, []string{
		"PREVIEW_DIRECT=1",
		"PREVIEW_AUTO_INJECT=0",
		"PREVIEW_PORT=18080",
		"PREVIEW_PICK_SCRIPT_URL=http://x/preview-pick.js",
	})
	if err != nil {
		t.Fatalf("exit: %v out=%s", err, out)
	}
	if !strings.Contains(out, "PREVIEW_AUTO_INJECT off") {
		t.Fatalf("out=%s", out)
	}
}

func TestPreviewInjectSh_SamePortSkip(t *testing.T) {
	out, err := runScript(t, []string{
		"PREVIEW_DIRECT=1",
		"PREVIEW_PORT=17980",
		"PREVIEW_PICK_SCRIPT_URL=http://x/preview-pick.js",
	})
	if err != nil {
		t.Fatalf("exit: %v out=%s", err, out)
	}
	if !strings.Contains(out, "must not equal") {
		t.Fatalf("out=%s", out)
	}
}
