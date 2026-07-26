package sandbox

import (
	"fmt"
	"strings"
)

// safeCmd is a remote shell command that can only be built by constructors that
// validate every argv fragment (CodeQL #11). Unvalidated strings cannot be
// converted to safeCmd, so they cannot reach Session.Start/Run.
type safeCmd struct {
	argv []string
}

// newSafeCmd validates each argv fragment with validateShellArg and stores a
// copy. render() (called at the Start/Run sink) re-validates and quotes.
func newSafeCmd(argv ...string) (safeCmd, error) {
	if len(argv) == 0 {
		return safeCmd{}, fmt.Errorf("empty command")
	}
	out := make([]string, len(argv))
	for i, a := range argv {
		if err := validateShellArg(a); err != nil {
			return safeCmd{}, fmt.Errorf("argv[%d]: %w", i, err)
		}
		out[i] = a
	}
	return safeCmd{argv: out}, nil
}

// render re-validates and POSIX-quotes argv at the execution sink so CodeQL
// sees the allowlist check adjacent to Session.Start/Run.
func (c safeCmd) render() (string, error) {
	if len(c.argv) == 0 {
		return "", fmt.Errorf("empty command")
	}
	parts := make([]string, len(c.argv))
	for i, a := range c.argv {
		if err := validateShellArg(a); err != nil {
			return "", fmt.Errorf("argv[%d]: %w", i, err)
		}
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " "), nil
}

// String returns the rendered command line, or "" if render fails.
func (c safeCmd) String() string {
	s, err := c.render()
	if err != nil {
		return ""
	}
	return s
}
