package sandbox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GitChanges computes a VCS-neutral change report for a single git working
// tree at dir, entirely over the SSH data plane (no in-sandbox HTTP service).
// It replaces the universal-sandbox's missing GET /api/changes: approving runs
// git itself via one batched shell script and parses the result. ok=false when
// dir is not a git repository. Best-effort: a partial/empty report degrades
// gracefully rather than failing the caller.
//
// The session baseline is derived from the default branch (merge-base with
// origin/HEAD, then @{upstream}); the universal image writes no /root/.sandbox
// baseline files, so "changes" means "commits + working-tree edits since this
// branch diverged from the default branch".
func (s *Sandbox) GitChanges(ctx context.Context, dir string) (*Changes, bool) {
	if err := validateShellArg(dir); err != nil {
		return nil, false
	}
	out, err := s.Exec(ctx, 40*time.Second, "bash", "-lc", gitChangesScript(dir))
	if strings.TrimSpace(out) == "" {
		if err != nil {
			return nil, false
		}
		return nil, false
	}
	return parseGitChanges(out)
}

// gitChangesScript builds the one-shot shell script (run over SSH) that emits
// tab-separated records describing dir's git state. It never exits non-zero.
// dir must already pass validateShellArg.
func gitChangesScript(dir string) string {
	return `d=` + shellQuote(dir) + `
cd "$d" 2>/dev/null || { echo NONE; exit 0; }
git rev-parse --git-dir >/dev/null 2>&1 || { echo NONE; exit 0; }
g(){ git -c core.quotepath=false "$@" 2>/dev/null; }
head=$(g rev-parse HEAD)
branch=$(g rev-parse --abbrev-ref HEAD)
basebranch=$(g symbolic-ref --short refs/remotes/origin/HEAD); basebranch=${basebranch#origin/}
base=$(g merge-base origin/HEAD HEAD)
[ -z "$base" ] && base=$(g rev-parse @{upstream})
[ -n "$base" ] && base=$(g rev-parse --verify --quiet "${base}^{commit}")
spec="$base"; [ -z "$spec" ] && spec=HEAD
[ -n "$(g status --porcelain)" ] && dirty=1 || dirty=0
pushed=0; remotesha=; unpushed=0
if [ -n "$branch" ] && [ "$branch" != HEAD ] && g remote | grep -qx origin; then
  ls=$(timeout 15 git ls-remote --heads origin "$branch" 2>/dev/null)
  if [ -n "$ls" ]; then
    remotesha=$(printf '%s\n' "$ls" | awk 'NR==1{print $1}')
    [ "$remotesha" = "$head" ] && pushed=1
    unpushed=$(g rev-list --count "${remotesha}..HEAD"); [ -z "$unpushed" ] && unpushed=0
  fi
fi
ahead=0; [ -n "$base" ] && { ahead=$(g rev-list --count "${base}..HEAD"); [ -z "$ahead" ] && ahead=0; }
printf 'VCS\tgit\n'
printf 'HEAD\t%s\n' "$head"
printf 'BRANCH\t%s\n' "$branch"
printf 'BASEBRANCH\t%s\n' "$basebranch"
printf 'BASE\t%s\n' "$base"
printf 'DIRTY\t%s\n' "$dirty"
printf 'PUSHED\t%s\n' "$pushed"
printf 'REMOTESHA\t%s\n' "$remotesha"
printf 'UNPUSHED\t%s\n' "$unpushed"
printf 'AHEAD\t%s\n' "$ahead"
[ -n "$base" ] && g log --format='COMMIT%x09%H%x09%an%x09%aI%x09%s' "${base}..HEAD"
g diff --numstat --no-renames "$spec" | awk -F'\t' 'NF>=3{printf "NUM\t%s\t%s\t%s\n",$1,$2,$3}'
g diff --name-status --no-renames "$spec" | awk -F'\t' 'NF>=2{printf "NAME\t%s\t%s\n",$1,$2}'
g ls-files --others --exclude-standard | awk 'NF{printf "UNTRACKED\t%s\n",$0}'
exit 0`
}

// parseGitChanges turns the script's tab-separated records into a *Changes.
// ok=false only when the tree is not a git repo (first line "NONE" or missing
// the "VCS git" marker).
func parseGitChanges(out string) (*Changes, bool) {
	ch := &Changes{}
	byPath := map[string]*ChangedFile{}
	var order []string
	seenGit := false

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if line == "NONE" {
			return nil, false
		}
		key, rest, _ := strings.Cut(line, "\t")
		switch key {
		case "VCS":
			seenGit = rest == "git"
			ch.VCS = rest
		case "HEAD":
			ch.HeadSHA = rest
		case "BRANCH":
			ch.Branch = rest
		case "BASEBRANCH":
			ch.BaseBranch = rest
		case "BASE":
			ch.BaseSHA = rest
		case "DIRTY":
			ch.Dirty = rest == "1"
		case "PUSHED":
			ch.Pushed = rest == "1"
		case "REMOTESHA":
			ch.RemoteSHA = rest
		case "UNPUSHED":
			ch.Unpushed, _ = strconv.Atoi(rest)
		case "AHEAD":
			ch.Ahead, _ = strconv.Atoi(rest)
		case "COMMIT":
			cols := strings.SplitN(rest, "\t", 4)
			if len(cols) == 4 {
				ch.Commits = append(ch.Commits, Commit{SHA: cols[0], Author: cols[1], At: cols[2], Subject: cols[3]})
			}
		case "NUM":
			cols := strings.SplitN(rest, "\t", 3)
			if len(cols) == 3 {
				added, _ := strconv.Atoi(cols[0]) // binary -> "-" -> 0
				deleted, _ := strconv.Atoi(cols[1])
				path := cols[2]
				byPath[path] = &ChangedFile{Path: path, Status: "modified", Added: added, Deleted: deleted}
				order = append(order, path)
			}
		case "NAME":
			cols := strings.SplitN(rest, "\t", 2)
			if len(cols) == 2 {
				status, path := mapGitStatus(cols[0]), cols[1]
				if cf, ok := byPath[path]; ok {
					cf.Status = status
				} else {
					byPath[path] = &ChangedFile{Path: path, Status: status}
					order = append(order, path)
				}
			}
		case "UNTRACKED":
			if _, ok := byPath[rest]; !ok {
				byPath[rest] = &ChangedFile{Path: rest, Status: "untracked"}
				order = append(order, rest)
			}
		}
	}

	if !seenGit {
		return nil, false
	}

	added, deleted := 0, 0
	for _, p := range order {
		cf := byPath[p]
		ch.ChangedFiles = append(ch.ChangedFiles, *cf)
		added += cf.Added
		deleted += cf.Deleted
	}
	if len(ch.ChangedFiles) > 0 {
		ch.DiffStat = fmt.Sprintf("%d file(s) changed, +%d -%d", len(ch.ChangedFiles), added, deleted)
	}
	ch.NewBranch = ch.BaseBranch != "" && ch.Branch != "" && ch.Branch != "HEAD" && ch.Branch != ch.BaseBranch
	return ch, true
}

// mapGitStatus maps a git name-status code to a VCS-neutral status word.
func mapGitStatus(code string) string {
	if code == "" {
		return "modified"
	}
	switch code[0] {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'M':
		return "modified"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "typechange"
	default:
		return "modified"
	}
}
