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
	for _, k := range []string{
		"GIT_REPOS", "GITHUB_URL", "GITLAB_URL", "GIT_SSH_KNOWN_HOSTS",
		"GRASP_CODEBUDDY_REGION", "GRASP_TRAE_REGION", "FEATURE_FLAG",
	} {
		if IsTokenEnvKey(k) {
			t.Fatalf("%s must not be token env key", k)
		}
	}
}
