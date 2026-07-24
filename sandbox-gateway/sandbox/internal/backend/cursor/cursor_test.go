package cursor

import "testing"

func TestBackend(t *testing.T) {
	b := New()
	if b.Name() != "cursor" || b.DefaultConfigRoot() != "/root/.cursor" {
		t.Fatal(b.Name(), b.DefaultConfigRoot())
	}
	_ = b.Argv("")
	_ = b.Argv("x")
	_ = b.AuthEnv(nil)
	_, _ = b.OnEvent(nil)
}
