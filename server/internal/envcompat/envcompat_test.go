package envcompat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyKey(t *testing.T) {
	if got := LegacyKey("GRASP_PORT"); got != "APPROVING_PORT" {
		t.Fatalf("LegacyKey = %q", got)
	}
	if got := LegacyKey("CURSOR_API_KEY"); got != "" {
		t.Fatalf("unexpected alias %q", got)
	}
}

func TestLookupPrefersGrasp(t *testing.T) {
	t.Setenv("GRASP_PORT", "8081")
	t.Setenv("APPROVING_PORT", "8082")
	if got := Lookup("GRASP_PORT"); got != "8081" {
		t.Fatalf("Lookup = %q, want 8081", got)
	}
}

func TestLookupFallsBackToApproving(t *testing.T) {
	t.Setenv("GRASP_PORT", "")
	t.Setenv("APPROVING_PORT", "7000")
	if got := Lookup("GRASP_PORT"); got != "7000" {
		t.Fatalf("Lookup = %q, want 7000", got)
	}
}

func TestRewriteEnvFileContent(t *testing.T) {
	in := "APPROVING_PORT=8080\nexport APPROVING_DB=/data/approving.db\n# APPROVING_IMAGE=old\nKEEP=has APPROVING_ in value\nGRASP_PORT=9\n"
	out, changed := RewriteEnvFileContent(in)
	if !changed {
		t.Fatal("expected change")
	}
	want := "GRASP_PORT=8080\nexport GRASP_DB=/data/approving.db\n# GRASP_IMAGE=old\nKEEP=has APPROVING_ in value\nGRASP_PORT=9\n"
	if out != want {
		t.Fatalf("rewrite:\n%s\nwant:\n%s", out, want)
	}
	again, changed := RewriteEnvFileContent(out)
	if changed || again != out {
		t.Fatal("rewrite must be idempotent")
	}
}

func TestRewriteEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("APPROVING_FOO=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := RewriteEnvFile(path)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "GRASP_FOO=1\n" {
		t.Fatalf("file = %q", data)
	}
}

func TestRewriteEnvFileSkipsDirectory(t *testing.T) {
	dir := t.TempDir()
	changed, err := RewriteEnvFile(dir)
	if err != nil || changed {
		t.Fatalf("dir rewrite: changed=%v err=%v", changed, err)
	}
}

func TestRewriteEnvFilePreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("APPROVING_FOO=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RewriteEnvFile(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestPromoteProcessEnv(t *testing.T) {
	t.Setenv("APPROVING_PORT", "7000")
	t.Setenv("GRASP_PORT", "")
	t.Setenv("APPROVING_DB", "legacy.db")
	t.Setenv("GRASP_DB", "grasp.db")
	n := PromoteProcessEnv()
	if n < 1 {
		t.Fatalf("expected at least one promotion, got %d", n)
	}
	if got := os.Getenv("GRASP_PORT"); got != "7000" {
		t.Fatalf("GRASP_PORT = %q", got)
	}
	if got := os.Getenv("GRASP_DB"); got != "grasp.db" {
		t.Fatalf("existing GRASP_DB must win: %q", got)
	}
}

func TestRewriteNearbyEnvFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(".env", []byte("APPROVING_PORT=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RewriteNearbyEnvFiles()
	data, err := os.ReadFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "GRASP_PORT=1\n" {
		t.Fatalf(".env = %q", data)
	}
}

func TestRewriteNearbyEnvFilesViaGraspEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.env")
	if err := os.WriteFile(path, []byte("APPROVING_DB=old.db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRASP_ENV_FILE", path)
	t.Chdir(t.TempDir())
	RewriteNearbyEnvFiles()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "GRASP_DB=old.db\n" {
		t.Fatalf("custom.env = %q", data)
	}
}

func TestMigrateLegacyPath(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "approving.db")
	newPath := filepath.Join(dir, "grasp.db")
	if err := os.WriteFile(oldPath, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := MigrateLegacyPath(oldPath, newPath)
	if err != nil || !ok {
		t.Fatalf("migrate: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("old path should be gone")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new path: %v", err)
	}

	ok, err = MigrateLegacyPath(oldPath, newPath)
	if err != nil || ok {
		t.Fatalf("second migrate should no-op: ok=%v err=%v", ok, err)
	}

	clobberOld := filepath.Join(dir, "approving.db")
	if err := os.WriteFile(clobberOld, []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = MigrateLegacyPath(clobberOld, newPath)
	if err != nil || ok {
		t.Fatalf("must not clobber: ok=%v err=%v", ok, err)
	}
}

func TestMigrateLegacyWorkspaceDir(t *testing.T) {
	dir := t.TempDir()
	oldDir := filepath.Join(dir, ".approving")
	newDir := filepath.Join(dir, ".grasp")
	if err := os.Mkdir(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ok, err := MigrateLegacyPath(oldDir, newDir)
	if err != nil || !ok {
		t.Fatalf("dir migrate: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Fatal(err)
	}
}
