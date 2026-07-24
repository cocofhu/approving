package backend

import (
	"testing"

	"backend/internal/backend/common"
)

func TestGet(t *testing.T) {
	if Get(CodeBuddy).Name() != CodeBuddy {
		t.Fatal("Get codebuddy")
	}
	if Get(Name("nope")).Name() != Cursor {
		t.Fatal("Get fallback")
	}
}

func TestConcreteBackends(t *testing.T) {
	for _, n := range []Name{Cursor, ClaudeCode, CodeBuddy, Trae} {
		be := Get(n)
		if be.Name() != n {
			t.Fatalf("%s Name", n)
		}
		if be.DefaultConfigRoot() == "" || be.Runtime() == "" {
			t.Fatalf("%s root/runtime", n)
		}
		if len(be.Argv("")) == 0 || len(be.Argv("x")) == 0 {
			t.Fatalf("%s Argv", n)
		}
		_, keep := be.OnEvent(nil)
		if !keep {
			t.Fatalf("%s OnEvent", n)
		}
		_ = be.AuthEnv([]string{"HOME=/tmp"})
	}
	_ = common.Cursor // keep import used via reexport
}
