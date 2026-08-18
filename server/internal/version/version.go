package version

import (
	"runtime/debug"
	"strings"
)

// commit is the VCS revision stamped at link time:
//
//	-ldflags "-X github.com/cocofhu/approving/internal/version.commit=<sha>"
//
// Empty means "not injected"; ShortSHA then falls back to BuildInfo vcs.revision.
var commit string

// skipVCS is set by Override so tests can pin an empty stamp without the
// local checkout's vcs.revision leaking into assertions.
var skipVCS bool

const shortLen = 7

// ShortSHA returns a 7-char lowercase hex commit, or "" when unavailable.
// Prefer the ldflags value; if blank, use runtime/debug.ReadBuildInfo vcs.revision.
// Whitespace-only or non-hex input is treated as unavailable.
func ShortSHA() string {
	raw := strings.TrimSpace(commit)
	if raw == "" && !skipVCS {
		raw = vcsRevision()
	}
	return Normalize(raw)
}

// Override pins the link-time stamp and disables VCS fallback until restore.
// Pass "" to simulate a binary with no usable revision (health omits commit).
func Override(raw string) (restore func()) {
	prevCommit := commit
	prevSkip := skipVCS
	commit = raw
	skipVCS = true
	return func() {
		commit = prevCommit
		skipVCS = prevSkip
	}
}

// Normalize trims, lowercases, and truncates a revision to 7 hex chars.
// Returns "" when the value is empty, shorter than 7 hex digits, or contains
// non-hex characters (after trim). Longer SHA strings are truncated, not padded.
func Normalize(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return ""
	}
	for _, r := range s {
		if !isHex(r) {
			return ""
		}
	}
	if len(s) < shortLen {
		return ""
	}
	return s[:shortLen]
}

func isHex(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}

func vcsRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}
