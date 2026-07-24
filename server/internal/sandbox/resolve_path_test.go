package sandbox

import (
	"testing"
)

func TestSandboxResolvePathAndWorkspace(t *testing.T) {
	s := &Sandbox{WorkspaceDir: "/root/workspace"}
	if got := s.resolvePath("/abs"); got != "/abs" {
		t.Fatalf("abs=%q", got)
	}
	if got := s.resolvePath("rel"); got != "/root/workspace/rel" {
		t.Fatalf("rel=%q", got)
	}
	if s.workspaceDir() != "/root/workspace" {
		t.Fatal("workspaceDir")
	}
	s2 := &Sandbox{mgr: &Manager{WorkspaceDir: "/from-mgr"}}
	if s2.workspaceDir() != "/from-mgr" {
		t.Fatalf("mgr workspace=%q", s2.workspaceDir())
	}
	creds := s.creds()
	if creds.user != "root" {
		t.Fatalf("detached creds user=%q", creds.user)
	}
}
