package envauth

import "testing"

func TestIsPlatformAuthEnvKey(t *testing.T) {
	for _, k := range []string{
		"CURSOR_API_KEY", "ANTHROPIC_API_KEY", "CODEBUDDY_API_KEY",
		"TRAE_API_KEY", "TRAECLI_PERSONAL_ACCESS_TOKEN", "OPENCODE_API_KEY",
	} {
		if !IsPlatformAuthEnvKey(k) {
			t.Fatalf("%s should be platform auth key", k)
		}
	}
	for _, k := range []string{
		"GITLAB_TOKEN", "GRASP_CURSOR_API_KEY", "GRASP_TRAE_API_KEY",
		"GRASP_CODEBUDDY_REGION", "GRASP_TRAE_REGION",
	} {
		if IsPlatformAuthEnvKey(k) {
			t.Fatalf("%s must not be filtered as platform auth", k)
		}
	}
}

func TestIsTokenEnvKey(t *testing.T) {
	for _, k := range TokenEnvKeys() {
		if !IsTokenEnvKey(k) {
			t.Fatalf("%s should be token env key", k)
		}
	}
	if IsTokenEnvKey("APPROVING_CURSOR_API_KEY") {
		t.Fatal("APPROVING_CURSOR_API_KEY is not a token key")
	}
	for _, k := range []string{
		"GIT_REPOS", "GITHUB_URL", "GITLAB_URL", "GIT_SSH_KNOWN_HOSTS",
		"GRASP_CODEBUDDY_REGION", "GRASP_TRAE_REGION", "FEATURE_FLAG",
	} {
		if IsTokenEnvKey(k) {
			t.Fatalf("%s must not be token env key", k)
		}
	}
}

func TestMergeEnvSharedTokenPriority(t *testing.T) {
	got := MergeEnvSharedTokenPriority(
		map[string]string{"GRASP_CURSOR_API_KEY": "shared", "FEATURE": "s"},
		map[string]string{"GRASP_CURSOR_API_KEY": "agent", "FEATURE": "a", "GITLAB_TOKEN": "gl"},
	)
	if got["GRASP_CURSOR_API_KEY"] != "shared" {
		t.Fatalf("token shared wins: %#v", got)
	}
	if got["FEATURE"] != "a" {
		t.Fatalf("non-token agent wins: %#v", got)
	}
	if got["GITLAB_TOKEN"] != "gl" {
		t.Fatalf("token only on agent kept: %#v", got)
	}
}
