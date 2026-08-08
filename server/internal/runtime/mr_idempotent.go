package runtime

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/sandbox"
)

// Idempotent MR/PR reuse kinds covered by the platform submit_mr fallback.
const (
	MRIdempotentExists    = "already_exists"
	MRIdempotentMerged    = "already_merged"
	MRIdempotentNoDiff    = "no_diff"
	MRIdempotentDuplicate = "duplicate"
)

// MRReuser is an optional ExecProvider capability: look up an existing PR/MR for
// the submit_mr source branch so a failed/duplicate create can still complete
// with mr_url instead of mis-failing or opening a second PR/MR.
type MRReuser interface {
	LookupExistingMR(ctx context.Context, req NodeReq) (url string, kind string, ok bool)
}

var (
	httpURLRe = regexp.MustCompile(`https?://[^\s"'<>]+`)
	// Message classifiers for GH/GL CLI and common agent narrations.
	mrExistsRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)already exists`),
		regexp.MustCompile(`(?i)pull request.*(exist|open)`),
		regexp.MustCompile(`(?i)merge request.*(exist|open)`),
		regexp.MustCompile(`(?i)a pull request for branch .+ already exists`),
		regexp.MustCompile(`(?i)mr already exists`),
	}
	mrMergedRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)already merged`),
		regexp.MustCompile(`(?i)pull request is (already )?merged`),
		regexp.MustCompile(`(?i)merge request.*(merged|closed)`),
		regexp.MustCompile(`(?i)was already merged`),
	}
	mrNoDiffRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)no commits between`),
		regexp.MustCompile(`(?i)no changes`),
		regexp.MustCompile(`(?i)nothing to (commit|merge|compare)`),
		regexp.MustCompile(`(?i)there are no differences?`),
		regexp.MustCompile(`(?i)no diff(erence)?s?`),
		regexp.MustCompile(`(?i)branches are (up.to.date|identical)`),
	}
	mrDupRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)duplicate`),
		regexp.MustCompile(`(?i)already (have|has) an? (open )?(pr|mr|pull|merge)`),
		regexp.MustCompile(`(?i)another (pull|merge) request`),
	}
)

// ClassifyMRIdempotent inspects agent/CLI failure text for the four reusable
// submit_mr outcomes. ok is false when the text is not a known idempotent case
// (avoids false-positive success on unrelated failures).
func ClassifyMRIdempotent(msg string) (kind string, ok bool) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "", false
	}
	for _, re := range mrMergedRes {
		if re.MatchString(msg) {
			return MRIdempotentMerged, true
		}
	}
	for _, re := range mrNoDiffRes {
		if re.MatchString(msg) {
			return MRIdempotentNoDiff, true
		}
	}
	for _, re := range mrDupRes {
		if re.MatchString(msg) {
			return MRIdempotentDuplicate, true
		}
	}
	for _, re := range mrExistsRes {
		if re.MatchString(msg) {
			return MRIdempotentExists, true
		}
	}
	return "", false
}

// ExtractMRURL picks the first http(s) URL from free text (CLI create output,
// agent summary, error narration).
func ExtractMRURL(msg string) string {
	u := httpURLRe.FindString(msg)
	return strings.TrimRight(u, ".,);]")
}

// ResolveIdempotentMRURL combines message classification with an optional URL
// extracted from the same text. Classified no_diff may succeed with a stable
// synthetic marker when no web URL exists; other kinds require a real URL to
// avoid false-positive success.
func ResolveIdempotentMRURL(msg, urlHint string) (url string, kind string, ok bool) {
	kind, classified := ClassifyMRIdempotent(msg)
	if !classified {
		return "", "", false
	}
	url = strings.TrimSpace(urlHint)
	if url == "" {
		url = ExtractMRURL(msg)
	}
	if url == "" && kind == MRIdempotentNoDiff {
		return "idempotent:no_diff", kind, true
	}
	if url == "" {
		return "", kind, false
	}
	return url, kind, true
}

// LookupExistingMRViaCLI lists open (then merged) PRs/MRs for sourceBranch on
// GitHub (gh) or GitLab (glab). Returns the first usable web URL. Pure helper
// for tests via sandbox.SetExecHook.
func LookupExistingMRViaCLI(ctx context.Context, sb *sandbox.Sandbox, dir, sourceBranch, targetBranch, repoURL, gitlabURL string) (url string, kind string, ok bool) {
	if sb == nil || strings.TrimSpace(sourceBranch) == "" {
		return "", "", false
	}
	cd := "cd " + shellArg(dir) + " && "
	hostGL := isGitLabRepo(repoURL, gitlabURL)
	hostGH := isGitHubRepo(repoURL)

	if hostGH || (!hostGL && !hostGH) {
		// Prefer GitHub when host looks like GH; also try when host is ambiguous
		// (agent may have configured gh on a mirror).
		if u, k, found := lookupGHPR(ctx, sb, cd, sourceBranch, targetBranch); found {
			return u, k, true
		}
	}
	if hostGL || (!hostGL && !hostGH) {
		if u, k, found := lookupGLMR(ctx, sb, cd, sourceBranch, targetBranch); found {
			return u, k, true
		}
	}
	return "", "", false
}

func lookupGHPR(ctx context.Context, sb *sandbox.Sandbox, cd, source, target string) (string, string, bool) {
	openCmd := cd + "gh pr list --state open --head " + shellArg(source) + " --json url,state,headRefName,baseRefName 2>/dev/null || true"
	out, err := sb.ExecScript(ctx, 25*time.Second, "bash", openCmd)
	if err == nil {
		if u, st := firstGHPRURL(out, target); u != "" {
			return u, kindFromGHState(st), true
		}
	}
	mergedCmd := cd + "gh pr list --state merged --head " + shellArg(source) + " --json url,state,headRefName,baseRefName --limit 5 2>/dev/null || true"
	out, err = sb.ExecScript(ctx, 25*time.Second, "bash", mergedCmd)
	if err == nil {
		if u, st := firstGHPRURL(out, target); u != "" {
			return u, kindFromGHState(st), true
		}
	}
	// Closed/all as last resort for "already exists" / duplicate narratives.
	allCmd := cd + "gh pr list --state all --head " + shellArg(source) + " --json url,state,headRefName,baseRefName --limit 5 2>/dev/null || true"
	out, err = sb.ExecScript(ctx, 25*time.Second, "bash", allCmd)
	if err == nil {
		if u, st := firstGHPRURL(out, target); u != "" {
			return u, kindFromGHState(st), true
		}
	}
	return "", "", false
}

func lookupGLMR(ctx context.Context, sb *sandbox.Sandbox, cd, source, target string) (string, string, bool) {
	openCmd := cd + "glab mr list --source-branch " + shellArg(source) + " -F json 2>/dev/null || true"
	out, err := sb.ExecScript(ctx, 25*time.Second, "bash", openCmd)
	if err == nil {
		if u, st := firstGLMRURL(out, target); u != "" {
			return u, kindFromGLState(st), true
		}
	}
	mergedCmd := cd + "glab mr list --merged --source-branch " + shellArg(source) + " -F json 2>/dev/null || true"
	out, err = sb.ExecScript(ctx, 25*time.Second, "bash", mergedCmd)
	if err == nil {
		if u, st := firstGLMRURL(out, target); u != "" {
			return u, kindFromGLState(st), true
		}
	}
	return "", "", false
}

func kindFromGHState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "MERGED":
		return MRIdempotentMerged
	default:
		return MRIdempotentExists
	}
}

func kindFromGLState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "merged":
		return MRIdempotentMerged
	default:
		return MRIdempotentExists
	}
}

func firstGHPRURL(jsonOut, target string) (url, state string) {
	var arr []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &arr); err != nil || len(arr) == 0 {
		return "", ""
	}
	target = strings.TrimSpace(target)
	for _, m := range arr {
		if target != "" {
			if base, _ := m["baseRefName"].(string); base != "" && base != target {
				continue
			}
		}
		u, _ := m["url"].(string)
		st, _ := m["state"].(string)
		if strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u), st
		}
	}
	return "", ""
}

func firstGLMRURL(jsonOut, target string) (url, state string) {
	var arr []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &arr); err != nil || len(arr) == 0 {
		return "", ""
	}
	target = strings.TrimSpace(target)
	for _, m := range arr {
		if target != "" {
			if base, _ := m["target_branch"].(string); base != "" && base != target {
				continue
			}
		}
		u, _ := m["web_url"].(string)
		st, _ := m["state"].(string)
		if strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u), st
		}
	}
	return "", ""
}

// isGitHubRepo reports whether repo_url points at github.com (or github Enterprise
// hosts that still use the `gh` CLI). Empty repo → false.
func isGitHubRepo(repo string) bool {
	r := strings.ToLower(strings.TrimSpace(repo))
	if r == "" {
		return false
	}
	return strings.Contains(r, "github.com") || strings.Contains(r, "github.")
}

// LookupExistingMR implements MRReuser for the ACP provider: inspect the node
// sandbox (if still registered live) for an existing GH/GL PR/MR on the source branch.
func (c *acpProvider) LookupExistingMR(ctx context.Context, req NodeReq) (url string, kind string, ok bool) {
	source, target := mrBranches(req)
	if source == "" {
		source = strings.TrimSpace(str2(req.Config["source_branch"]))
	}
	if source == "" {
		return "", "", false
	}
	dir, repo := c.nodeRepo(req)
	c.mu.Lock()
	sb := c.live[reactKey(req)]
	if sb == nil {
		// Fall back to any live sandbox for this run (submit_mr may have torn
		// down the exact key already during late outcome handling).
		prefix := req.RunID + "|"
		for k, v := range c.live {
			if strings.HasPrefix(k, prefix) && v != nil {
				sb = v
				break
			}
		}
	}
	c.mu.Unlock()
	if sb == nil {
		return "", "", false
	}
	return LookupExistingMRViaCLI(ctx, sb, dir, source, target, repo, c.gitLabURL(req))
}
