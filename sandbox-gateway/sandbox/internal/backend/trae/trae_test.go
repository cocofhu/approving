package trae

import (
	"testing"

	"backend/internal/backend/common"
)

func TestBackend(t *testing.T) {
	b := New()
	if b.Name() != common.Trae || b.DefaultConfigRoot() != "/root/.trae" {
		t.Fatal(b.Name())
	}
	_ = b.Argv("")
	_ = b.Argv("x")
	_, _ = b.OnEvent(nil)
}

func TestAuthEnv(t *testing.T) {
	b := New()
	t.Setenv("ACP_TRAE_API_KEY", "tok")
	t.Setenv("ACP_TRAE_REGION", "intl")
	t.Setenv("TRAECLI_HOST", "")
	env := b.AuthEnv(nil)
	if common.GetEnvValue(env, "TRAECLI_PERSONAL_ACCESS_TOKEN") != "tok" {
		t.Fatal("token")
	}
	if common.GetEnvValue(env, "TRAECLI_HOST") != "https://www.trae.ai" {
		t.Fatal("host intl")
	}
	t.Setenv("ACP_TRAE_REGION", "cn")
	t.Setenv("TRAECLI_HOST", "")
	env = b.AuthEnv(nil)
	if common.GetEnvValue(env, "TRAECLI_HOST") != "" {
		t.Fatal("cn must not set host")
	}
	t.Setenv("ACP_TRAE_REGION", "intl")
	t.Setenv("TRAECLI_HOST", "https://custom.example")
	env = b.AuthEnv(nil)
	// Explicit host in os env → AuthEnv must not Upsert the intl default.
	if common.GetEnvValue(env, "TRAECLI_HOST") == "https://www.trae.ai" {
		t.Fatal("must not override explicit host")
	}
}
