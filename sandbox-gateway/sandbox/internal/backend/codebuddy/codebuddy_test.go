package codebuddy

import (
	"testing"

	"backend/internal/backend/common"
)

func TestBackendBasics(t *testing.T) {
	b := New()
	if b.Name() != common.CodeBuddy || b.Runtime() == "" || b.DefaultConfigRoot() == "" {
		t.Fatal("meta")
	}
	if len(b.Argv("")) == 0 || len(b.Argv("m")) == 0 {
		t.Fatal("argv")
	}
	_, _ = b.OnEvent(nil)
}

func TestAuthEnvRegions(t *testing.T) {
	b := New()
	cases := []struct {
		region, internet, want string
	}{
		{"", "", "public"},
		{"intl", "", "public"},
		{"cn", "", "internal"},
		{"ioa", "", "ioa"},
		{"staging", "", "public"},
		{"custom", "", "custom"},
	}
	for _, tc := range cases {
		t.Setenv("ACP_CODEBUDDY_REGION", tc.region)
		t.Setenv("CODEBUDDY_INTERNET_ENVIRONMENT", "")
		t.Setenv("ACP_CODEBUDDY_API_KEY", "ck")
		env := b.AuthEnv(nil)
		got := common.GetEnvValue(env, "CODEBUDDY_INTERNET_ENVIRONMENT")
		if got != tc.want {
			t.Fatalf("region=%q got=%q want=%q", tc.region, got, tc.want)
		}
		if common.GetEnvValue(env, "CODEBUDDY_API_KEY") != "ck" {
			t.Fatal("api key")
		}
	}
	// Explicit os env: AuthEnv must not Upsert region over it (returned slice may omit it).
	t.Setenv("ACP_CODEBUDDY_REGION", "ioa")
	t.Setenv("CODEBUDDY_INTERNET_ENVIRONMENT", "internal")
	env := b.AuthEnv(nil)
	if got := common.GetEnvValue(env, "CODEBUDDY_INTERNET_ENVIRONMENT"); got == "ioa" {
		t.Fatalf("must not override explicit internet, got %q", got)
	}
}
