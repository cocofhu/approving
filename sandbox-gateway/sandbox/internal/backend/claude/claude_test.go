package claude

import "testing"

func TestBackend(t *testing.T) {
	b := New()
	if b.Name() != "claude_code" || b.Runtime() == "" || b.DefaultConfigRoot() == "" {
		t.Fatal(b)
	}
	if len(b.Argv("")) < 2 || len(b.Argv("m")) < 2 {
		t.Fatal("argv")
	}
	_ = b.AuthEnv([]string{})
	_, keep := b.OnEvent(nil)
	if !keep {
		t.Fatal("OnEvent")
	}
}
