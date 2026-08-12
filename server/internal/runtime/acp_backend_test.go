package runtime

import (
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/envauth"
)

func TestNormalizeBackend(t *testing.T) {
	cases := []struct {
		in   string
		want AcpBackend
	}{
		{"cursor", BackendCursor},
		{"claude_code", BackendClaudeCode},
		{"codebuddy", BackendCodeBuddy},
		{"trae", BackendTrae},
		{"", BackendCursor},
		{" unknown ", BackendCursor},
		{"CURSOR", BackendCursor}, // case-sensitive; unknown → cursor
	}
	for _, tc := range cases {
		if got := NormalizeBackend(tc.in); got != tc.want {
			t.Fatalf("NormalizeBackend(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestDefaultConfigRootAndResolve(t *testing.T) {
	if got := DefaultConfigRoot(BackendCodeBuddy); got != "/root/.codebuddy" {
		t.Fatalf("codebuddy root=%q", got)
	}
	if got := DefaultConfigRoot(BackendTrae); got != "/root/.trae" {
		t.Fatalf("trae root=%q", got)
	}
	if got := DefaultConfigRoot(BackendClaudeCode); got != "/root/.claude" {
		t.Fatalf("claude root=%q", got)
	}
	if got := DefaultConfigRoot(BackendCursor); got != "/root/.cursor" {
		t.Fatalf("cursor root=%q", got)
	}
	if got := ResolveConfigRoot(BackendTrae, "  /custom  "); got != "/custom" {
		t.Fatalf("explicit root=%q", got)
	}
	if got := ResolveConfigRoot(BackendTrae, ""); got != "/root/.trae" {
		t.Fatalf("default root=%q", got)
	}
}

func TestAgentRuntimeLabel(t *testing.T) {
	cases := map[AcpBackend]string{
		BackendCursor:     "cursor-agent",
		BackendClaudeCode: "claude-code-acp",
		BackendCodeBuddy:  "codebuddy-acp",
		BackendTrae:       "trae-acp",
	}
	for b, want := range cases {
		if got := AgentRuntimeLabel(b); got != want {
			t.Fatalf("label(%s)=%q want %q", b, got, want)
		}
	}
}

func TestMergeAuthEnv_TraeAliases(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"APPROVING", map[string]string{"APPROVING_TRAE_API_KEY": "trae-lt-a"}, "trae-lt-a"},
		{"legacy TRAE_API_KEY", map[string]string{"TRAE_API_KEY": "trae-lt-b"}, "trae-lt-b"},
		{"official token", map[string]string{EnvTraeCLIToken: "trae-lt-c"}, "trae-lt-c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := MergeAuthEnv(BackendTrae, tc.env)
			if err != nil {
				t.Fatal(err)
			}
			if got := out[EnvTraeCLIToken]; got != tc.want {
				t.Fatalf("%s=%q want %q (env=%#v)", EnvTraeCLIToken, got, tc.want, out)
			}
		})
	}
}

func TestMergeAuthEnv_TraeKeyPreference(t *testing.T) {
	// agentKeys order: APPROVING → TRAE_API_KEY → TRAECLI token
	out, err := MergeAuthEnv(BackendTrae, map[string]string{
		"APPROVING_TRAE_API_KEY": "trae-lt-first",
		"TRAE_API_KEY":           "trae-lt-second",
		EnvTraeCLIToken:          "trae-lt-third",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[EnvTraeCLIToken] != "trae-lt-first" {
		t.Fatalf("preference=%q want trae-lt-first", out[EnvTraeCLIToken])
	}
}

func TestMergeAuthEnv_CodeBuddyAliases(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{"APPROVING", map[string]string{"APPROVING_CODEBUDDY_API_KEY": "ck_a"}},
		{"official", map[string]string{"CODEBUDDY_API_KEY": "ck_b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := MergeAuthEnv(BackendCodeBuddy, tc.env)
			if err != nil {
				t.Fatal(err)
			}
			if out["CODEBUDDY_API_KEY"] == "" {
				t.Fatalf("CODEBUDDY_API_KEY unset: %#v", out)
			}
		})
	}
}

func TestMergeAuthEnv_MissingKey(t *testing.T) {
	for _, b := range []AcpBackend{BackendCursor, BackendClaudeCode, BackendCodeBuddy, BackendTrae} {
		t.Run(string(b), func(t *testing.T) {
			_, err := MergeAuthEnv(b, map[string]string{})
			if err == nil {
				t.Fatal("expected error when key missing")
			}
			if !strings.Contains(err.Error(), "鉴权未配置") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestMergeAuthEnv_CursorNoRegionMutation(t *testing.T) {
	out, err := MergeAuthEnv(BackendCursor, map[string]string{
		"CURSOR_API_KEY":   "ck",
		EnvCodeBuddyRegion: "staging",
		EnvTraeRegion:      "intl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[EnvCodeBuddyInternet] != "" {
		t.Fatalf("cursor must not set codebuddy internet: %q", out[EnvCodeBuddyInternet])
	}
	if out[EnvTraeCLIHost] != "" {
		t.Fatalf("cursor must not set trae host: %q", out[EnvTraeCLIHost])
	}
	if CodeBuddySettingsForEnv(BackendCursor, out) != nil {
		t.Fatal("cursor must not produce codebuddy settings")
	}
}

func TestMergeRegionEnv_CodeBuddy(t *testing.T) {
	t.Run("default public", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
			"CODEBUDDY_API_KEY": "ck_x",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvCodeBuddyInternet] != "public" {
			t.Fatalf("internet=%q want public", out[EnvCodeBuddyInternet])
		}
		if out[EnvCodeBuddyBaseURL] != "" {
			t.Fatalf("unexpected base url %q", out[EnvCodeBuddyBaseURL])
		}
	})
	t.Run("internal via region alias", func(t *testing.T) {
		for _, alias := range []string{"cn", "china", "internal"} {
			out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
				"CODEBUDDY_API_KEY": "ck_x",
				EnvCodeBuddyRegion:  alias,
			})
			if err != nil {
				t.Fatal(err)
			}
			if out[EnvCodeBuddyInternet] != "internal" {
				t.Fatalf("alias %q → internet=%q want internal", alias, out[EnvCodeBuddyInternet])
			}
		}
	})
	t.Run("intl aliases map to public", func(t *testing.T) {
		for _, alias := range []string{"public", "intl", "international"} {
			out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
				"CODEBUDDY_API_KEY": "ck_x",
				EnvCodeBuddyRegion:  alias,
			})
			if err != nil {
				t.Fatal(err)
			}
			if out[EnvCodeBuddyInternet] != "public" {
				t.Fatalf("alias %q → internet=%q want public", alias, out[EnvCodeBuddyInternet])
			}
		}
	})
	t.Run("ioa via region", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
			"CODEBUDDY_API_KEY": "ck_x",
			EnvCodeBuddyRegion:  "ioa",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvCodeBuddyInternet] != "ioa" {
			t.Fatalf("internet=%q want ioa", out[EnvCodeBuddyInternet])
		}
	})
	t.Run("internet-only without region", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
			"CODEBUDDY_API_KEY":  "ck_x",
			EnvCodeBuddyInternet: "internal",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvCodeBuddyInternet] != "internal" {
			t.Fatalf("internet=%q want internal", out[EnvCodeBuddyInternet])
		}
		if CodeBuddySettingsForEnv(BackendCodeBuddy, out) != nil {
			t.Fatal("non-staging must not write settings")
		}
	})
	t.Run("unknown region passthrough", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
			"CODEBUDDY_API_KEY": "ck_x",
			EnvCodeBuddyRegion:  "custom-site",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvCodeBuddyInternet] != "custom-site" {
			t.Fatalf("internet=%q want custom-site", out[EnvCodeBuddyInternet])
		}
	})
	t.Run("staging sets region for settings.json", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
			"APPROVING_CODEBUDDY_API_KEY": "ck_x",
			EnvCodeBuddyRegion:            "staging",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvCodeBuddyInternet] != "public" {
			t.Fatalf("internet=%q want public", out[EnvCodeBuddyInternet])
		}
		if out[EnvCodeBuddyRegion] != "staging" {
			t.Fatalf("region=%q want staging", out[EnvCodeBuddyRegion])
		}
		if out[EnvCodeBuddyBaseURL] != "" {
			t.Fatalf("staging must not invent BASE_URL, got %q", out[EnvCodeBuddyBaseURL])
		}
		settings := CodeBuddySettingsForEnv(BackendCodeBuddy, out)
		if settings == nil {
			t.Fatal("expected settings for staging")
		}
		if settings["endpoint"] != CodeBuddyStagingEndpoint {
			t.Fatalf("endpoint=%v", settings["endpoint"])
		}
		if settings["envRouteMode"] != "staging" {
			t.Fatalf("envRouteMode=%v", settings["envRouteMode"])
		}
		envNested, _ := settings["env"].(map[string]string)
		if envNested[EnvCodeBuddyInternet] != "public" {
			t.Fatalf("settings.env=%v", settings["env"])
		}
	})
	t.Run("staging keeps custom BASE_URL for settings endpoint", func(t *testing.T) {
		custom := "https://staging.example.com"
		out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
			"CODEBUDDY_API_KEY": "ck_x",
			EnvCodeBuddyRegion:  "staging",
			EnvCodeBuddyBaseURL: custom,
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvCodeBuddyBaseURL] != custom {
			t.Fatalf("base url mutated: %q", out[EnvCodeBuddyBaseURL])
		}
		settings := CodeBuddySettingsForEnv(BackendCodeBuddy, out)
		if settings == nil || settings["endpoint"] != custom {
			t.Fatalf("settings=%v want endpoint %q", settings, custom)
		}
	})
	t.Run("explicit official vars win", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendCodeBuddy, map[string]string{
			"CODEBUDDY_API_KEY":  "ck_x",
			EnvCodeBuddyRegion:   "cn",
			EnvCodeBuddyInternet: "ioa",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvCodeBuddyInternet] != "ioa" {
			t.Fatalf("internet=%q want ioa (explicit)", out[EnvCodeBuddyInternet])
		}
		if CodeBuddySettingsForEnv(BackendCodeBuddy, out) != nil {
			t.Fatal("cn should not write staging settings")
		}
	})
}

func TestCodeBuddySettingsForEnv(t *testing.T) {
	staging := map[string]string{EnvCodeBuddyRegion: "staging"}
	if CodeBuddySettingsForEnv(BackendCodeBuddy, nil) != nil {
		t.Fatal("nil env")
	}
	if CodeBuddySettingsForEnv(BackendCodeBuddy, map[string]string{EnvCodeBuddyRegion: "public"}) != nil {
		t.Fatal("public must not write settings")
	}
	if CodeBuddySettingsForEnv(BackendCodeBuddy, map[string]string{EnvCodeBuddyRegion: "STAGING"}) == nil {
		t.Fatal("region compare is case-insensitive")
	}
	// Stray CodeBuddy REGION on other backends must not materialize settings.json.
	for _, b := range []AcpBackend{BackendCursor, BackendClaudeCode, BackendTrae} {
		if CodeBuddySettingsForEnv(b, staging) != nil {
			t.Fatalf("backend %s must ignore codebuddy staging region", b)
		}
	}
}

func TestMergeRegionEnv_Trae(t *testing.T) {
	t.Run("cn default no host", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendTrae, map[string]string{
			EnvTraeCLIToken: "trae-lt-x",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvTraeCLIHost] != "" {
			t.Fatalf("unexpected host %q", out[EnvTraeCLIHost])
		}
	})
	t.Run("cn aliases leave host unset", func(t *testing.T) {
		for _, alias := range []string{"cn", "china", "internal"} {
			out, err := MergeAuthEnv(BackendTrae, map[string]string{
				EnvTraeCLIToken: "trae-lt-x",
				EnvTraeRegion:   alias,
			})
			if err != nil {
				t.Fatal(err)
			}
			if out[EnvTraeCLIHost] != "" {
				t.Fatalf("alias %q set host %q", alias, out[EnvTraeCLIHost])
			}
		}
	})
	t.Run("intl sets host", func(t *testing.T) {
		for _, alias := range []string{"intl", "international", "public", "ai"} {
			out, err := MergeAuthEnv(BackendTrae, map[string]string{
				"APPROVING_TRAE_API_KEY": "trae-lt-x",
				EnvTraeRegion:            alias,
			})
			if err != nil {
				t.Fatal(err)
			}
			if out[EnvTraeCLIHost] != TraeIntlHost {
				t.Fatalf("alias %q host=%q want %q", alias, out[EnvTraeCLIHost], TraeIntlHost)
			}
			if out[EnvTraeCLIToken] != "trae-lt-x" {
				t.Fatalf("token not mapped: %q", out[EnvTraeCLIToken])
			}
		}
	})
	t.Run("unknown region no host mutation", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendTrae, map[string]string{
			EnvTraeCLIToken: "trae-lt-x",
			EnvTraeRegion:   "corp-custom",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvTraeCLIHost] != "" {
			t.Fatalf("unknown region must not invent host, got %q", out[EnvTraeCLIHost])
		}
	})
	t.Run("explicit host wins", func(t *testing.T) {
		out, err := MergeAuthEnv(BackendTrae, map[string]string{
			EnvTraeCLIToken: "trae-lt-x",
			EnvTraeRegion:   "intl",
			EnvTraeCLIHost:  "https://corp.example",
		})
		if err != nil {
			t.Fatal(err)
		}
		if out[EnvTraeCLIHost] != "https://corp.example" {
			t.Fatalf("host=%q want corp", out[EnvTraeCLIHost])
		}
	})
}

func TestIsPlatformAuthEnvKey(t *testing.T) {
	for _, k := range []string{"CURSOR_API_KEY", "ANTHROPIC_API_KEY", "CODEBUDDY_API_KEY", "TRAE_API_KEY", EnvTraeCLIToken} {
		if !envauth.IsPlatformAuthEnvKey(k) {
			t.Fatalf("%s should be platform auth key", k)
		}
	}
	for _, k := range []string{"GITLAB_TOKEN", "APPROVING_CURSOR_API_KEY", "APPROVING_TRAE_API_KEY", EnvCodeBuddyRegion, EnvTraeRegion} {
		if envauth.IsPlatformAuthEnvKey(k) {
			t.Fatalf("%s must not be filtered as platform auth", k)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("  ", "", "x", "y"); got != "x" {
		t.Fatalf("got %q", got)
	}
	if got := firstNonEmpty("", "  "); got != "" {
		t.Fatalf("got %q", got)
	}
}
