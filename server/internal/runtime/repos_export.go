package runtime

import (
	"strings"

	"github.com/cocofhu/approving/internal/sandbox"
)

// ResolveReposFromVars returns the clone list derived from vars.repos, using
// the same rules as the provider's resolveRepos (name+url required; branch
// overridden by vars.branches[name] when set). Nil/empty when unusable.
func ResolveReposFromVars(vars map[string]any) []sandbox.RepoSpec {
	return resolveRepos(NodeReq{Vars: vars})
}

// RepoNamesFromVars returns vars.repos entry names in list order (parseReposVar
// rules: safe names only). Used for ∈repos membership checks that should not
// require a clone URL.
func RepoNamesFromVars(vars map[string]any) []string {
	entries := parseReposVar(vars["repos"])
	if len(entries) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := strings.TrimSpace(e.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
