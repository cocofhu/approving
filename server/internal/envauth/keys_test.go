package envauth

import "testing"

func TestIsPlatformAuthEnvKey(t *testing.T) {
	for _, k := range []string{
		"CURSOR_API_KEY", "ANTHROPIC_API_KEY", "CODEBUDDY_API_KEY",
		"TRAE_API_KEY", "TRAECLI_PERSONAL_ACCESS_TOKEN",
	} {
		if !IsPlatformAuthEnvKey(k) {
			t.Fatalf("%s should be platform auth key", k)
		}
	}
	for _, k := range []string{
		"GITLAB_TOKEN", "APPROVING_CURSOR_API_KEY", "APPROVING_TRAE_API_KEY",
		"APPROVING_CODEBUDDY_REGION", "APPROVING_TRAE_REGION",
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
		"APPROVING_CODEBUDDY_REGION", "APPROVING_TRAE_REGION", "FEATURE_FLAG",
	} {
		if IsTokenEnvKey(k) {
			t.Fatalf("%s must not be token env key", k)
		}
	}
}
