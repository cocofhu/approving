package backend

import (
	"os"
	"testing"

	"backend/internal/backend/common"
)

// env var names under test (kept as literals so the test also guards against
// accidental renames in the per-backend packages).
const (
	envCodeBuddyRegion   = "ACP_CODEBUDDY_REGION"
	envCodeBuddyInternet = "CODEBUDDY_INTERNET_ENVIRONMENT"
	envCodeBuddyAPIKey   = "CODEBUDDY_API_KEY"
	envAcpCodeBuddyKey   = "ACP_CODEBUDDY_API_KEY"
	envCursorAPIKey      = "CURSOR_API_KEY"
	envAcpCursorKey      = "ACP_CURSOR_API_KEY"
	envTraeCLIToken      = "TRAECLI_PERSONAL_ACCESS_TOKEN"
	envAcpTraeKey        = "ACP_TRAE_API_KEY"
	envTraeRegion        = "ACP_TRAE_REGION"
	envTraeCLIHost       = "TRAECLI_HOST"
)

func getEnvValue(env []string, key string) string { return common.GetEnvValue(env, key) }

func authEnv(n Name) []string { return Get(n).AuthEnv(os.Environ()) }

func TestAgentEnv_CodeBuddyRegionIOA(t *testing.T) {
	t.Setenv(envCodeBuddyRegion, "ioa")
	t.Setenv(envCodeBuddyInternet, "")
	t.Setenv(envAcpCodeBuddyKey, "ck_test")
	t.Setenv(envCodeBuddyAPIKey, "")

	env := authEnv(CodeBuddy)
	if got := getEnvValue(env, envCodeBuddyInternet); got != "ioa" {
		t.Fatalf("CODEBUDDY_INTERNET_ENVIRONMENT=%q want ioa", got)
	}
	if got := getEnvValue(env, envCodeBuddyAPIKey); got != "ck_test" {
		t.Fatalf("CODEBUDDY_API_KEY=%q want ck_test", got)
	}
}

func TestAgentEnv_CodeBuddyExplicitInternetWins(t *testing.T) {
	t.Setenv(envCodeBuddyRegion, "ioa")
	t.Setenv(envCodeBuddyInternet, "internal")

	env := authEnv(CodeBuddy)
	if got := getEnvValue(env, envCodeBuddyInternet); got != "internal" {
		t.Fatalf("explicit internet=%q want internal", got)
	}
}

func TestAgentEnv_CursorNoMutation(t *testing.T) {
	t.Setenv(envCodeBuddyRegion, "")
	t.Setenv(envCodeBuddyInternet, "")
	t.Setenv(envAcpCursorKey, "cur_test")
	t.Setenv(envCursorAPIKey, "")

	env := authEnv(Cursor)
	if got := getEnvValue(env, envCursorAPIKey); got != "cur_test" {
		t.Fatalf("CURSOR_API_KEY=%q want cur_test", got)
	}
	if got := getEnvValue(env, envCodeBuddyInternet); got != "" {
		t.Fatalf("cursor must not set codebuddy internet: %q", got)
	}
}

func TestAgentEnv_PreservesUnrelatedVars(t *testing.T) {
	t.Setenv("HOME", "/tmp/test-home")
	t.Setenv(envCodeBuddyRegion, "public")

	env := authEnv(CodeBuddy)
	if got := getEnvValue(env, "HOME"); got != "/tmp/test-home" {
		t.Fatalf("HOME=%q want /tmp/test-home", got)
	}
}

func TestUpsertEnv(t *testing.T) {
	base := []string{"A=1", "B=2"}
	out := common.UpsertEnv(base, "B", "9")
	if getEnvValue(out, "B") != "9" {
		t.Fatalf("upsert existing failed")
	}
	out = common.UpsertEnv(out, "C", "3")
	if getEnvValue(out, "C") != "3" {
		t.Fatalf("upsert new failed")
	}
}

func TestAgentEnv_TraeCnNoHost(t *testing.T) {
	t.Setenv(envTraeRegion, "cn")
	t.Setenv(envTraeCLIHost, "")
	t.Setenv(envAcpTraeKey, "tk_test")

	env := authEnv(Trae)
	if got := getEnvValue(env, envTraeCLIHost); got != "" {
		t.Fatalf("TRAECLI_HOST=%q want empty for cn", got)
	}
	if got := getEnvValue(env, envTraeCLIToken); got != "tk_test" {
		t.Fatalf("TRAECLI_PERSONAL_ACCESS_TOKEN=%q want tk_test", got)
	}
}

func TestAgentEnv_TraeIntlHost(t *testing.T) {
	t.Setenv(envTraeRegion, "intl")
	t.Setenv(envTraeCLIHost, "")

	env := authEnv(Trae)
	if got := getEnvValue(env, envTraeCLIHost); got != "https://www.trae.ai" {
		t.Fatalf("TRAECLI_HOST=%q want https://www.trae.ai", got)
	}
}

func TestAgentEnv_TraeExplicitHostWins(t *testing.T) {
	t.Setenv(envTraeRegion, "intl")
	t.Setenv(envTraeCLIHost, "https://custom.trae.example")

	env := authEnv(Trae)
	if got := getEnvValue(env, envTraeCLIHost); got != "https://custom.trae.example" {
		t.Fatalf("explicit TRAECLI_HOST=%q want https://custom.trae.example", got)
	}
}

func TestFirstNonEmptyEnv(t *testing.T) {
	os.Unsetenv("X_TEST_A")
	os.Unsetenv("X_TEST_B")
	t.Setenv("X_TEST_B", "b")
	if got := common.FirstNonEmptyEnv("X_TEST_A", "X_TEST_B"); got != "b" {
		t.Fatalf("got %q want b", got)
	}
}
