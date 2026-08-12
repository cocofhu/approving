package runtime

import "strings"

// gitRepoScheme classifies a repo URL as https, ssh, or "".
// Test-only helper (production repo URL handling uses gitBaseURL / gitRepoHost).
func gitRepoScheme(repo string) string {
	repo = strings.TrimSpace(repo)
	switch {
	case strings.HasPrefix(repo, "http://"), strings.HasPrefix(repo, "https://"):
		return "https"
	case strings.HasPrefix(repo, "ssh://"):
		return "ssh"
	case strings.Contains(repo, "@") && strings.Contains(repo, ":"):
		return "ssh"
	default:
		return ""
	}
}
