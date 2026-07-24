package config

import (
	"testing"
	"time"
)

func TestTTLHelpersAndMergeAcpEnv(t *testing.T) {
	c := &Config{}
	setDefaults(c)
	if c.TabIdleTTL() <= 0 || c.ContainerIdleTTL() <= 0 || c.SandboxCreateTimeout() <= 0 {
		t.Fatalf("defaults: tab=%v cont=%v create=%v", c.TabIdleTTL(), c.ContainerIdleTTL(), c.SandboxCreateTimeout())
	}
	c.Browser.TabIdleTTLSeconds = 120
	c.Browser.ContainerIdleTTLSeconds = 300
	if c.TabIdleTTL() != 120*time.Second || c.ContainerIdleTTL() != 300*time.Second {
		t.Fatalf("parsed: %v %v", c.TabIdleTTL(), c.ContainerIdleTTL())
	}
	c.Sandbox.CreateTimeoutSeconds = 45
	if c.SandboxCreateTimeout() != 45*time.Second {
		t.Fatalf("create timeout: %v", c.SandboxCreateTimeout())
	}

	c.Sandbox.AcpEnv = map[string]string{"A": "1", "B": "2", "": "skip"}
	c.Sandbox.Env = nil
	mergeAcpEnv(c)
	if c.Sandbox.Env["A"] != "1" || c.Sandbox.Env["B"] != "2" {
		t.Fatalf("mergeAcpEnv: %+v", c.Sandbox.Env)
	}
	mergeEnvList(c, "C=3, junk, D=4")
	if c.Sandbox.Env["C"] != "3" || c.Sandbox.Env["D"] != "4" {
		t.Fatalf("mergeEnvList: %+v", c.Sandbox.Env)
	}
}
