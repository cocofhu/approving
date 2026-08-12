package sandbox

// quoteShellPath validates then POSIX-quotes a remote filesystem path.
// Test-only helper (production paths use shellQuote on trusted fragments /
// newSafeCmd for argv).
func quoteShellPath(path string) (string, error) {
	if err := validateShellArg(path); err != nil {
		return "", err
	}
	return shellQuote(path), nil
}
