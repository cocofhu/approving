package version

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "whitespace", in: "  \t\n", want: ""},
		{name: "full sha truncated", in: "b01bb39abcdef0123456789", want: "b01bb39"},
		{name: "already short", in: "b01bb39", want: "b01bb39"},
		{name: "uppercase", in: "B01BB39DEADBEEF", want: "b01bb39"},
		{name: "padded", in: "  ABCDEF1  ", want: "abcdef1"},
		{name: "too short", in: "abc123", want: ""},
		{name: "non hex dirty", in: "b01bb39-dirty", want: ""},
		{name: "non hex prefix", in: "commit:b01bb39", want: ""},
		{name: "unknown placeholder", in: "unknown", want: ""},
		{name: "na placeholder", in: "N/A", want: ""},
		{name: "dash placeholder", in: "—", want: ""},
		{name: "semver", in: "v1.2.3", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Normalize(tc.in); got != tc.want {
				t.Fatalf("Normalize(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestShortSHAPrefersLdflagsOverVCS(t *testing.T) {
	prev := commit
	t.Cleanup(func() { commit = prev })

	commit = "DEADBEEFcafe"
	got := ShortSHA()
	if got != "deadbee" {
		t.Fatalf("ldflags ShortSHA=%q want deadbee", got)
	}

	commit = "   "
	// Blank ldflags falls back to BuildInfo; result is either empty or a valid short SHA.
	got = ShortSHA()
	if got == "" {
		return
	}
	if len(got) != 7 || got != strings.ToLower(got) {
		t.Fatalf("VCS fallback ShortSHA=%q; want empty or 7-char lowercase hex", got)
	}
	for _, r := range got {
		if r < '0' || (r > '9' && r < 'a') || r > 'f' {
			t.Fatalf("VCS fallback ShortSHA=%q contains non-hex", got)
		}
	}
}

func TestShortSHARejectsInvalidLdflags(t *testing.T) {
	prev := commit
	t.Cleanup(func() { commit = prev })
	commit = "not-a-sha"
	if got := ShortSHA(); got != "" {
		t.Fatalf("invalid ldflags ShortSHA=%q want empty", got)
	}
}
