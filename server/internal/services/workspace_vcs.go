package services

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	workspaceVcsDirName = ".workspace-vcs"
	vcsCommitPrefix     = "approving-vcs:"

	VcsSourceSystem      = "system"
	VcsSourceStudio      = "studio"
	VcsSourcePmMCP       = "pm-mcp"
	VcsSourceExternalMCP = "external-mcp"
	VcsSourceRestore     = "restore"
)

var (
	ErrVcsReasonRequired = errors.New("变更原因不能为空")
	ErrVcsRevisionMiss   = errors.New("目标版本不存在")
	ErrVcsGitFailed      = errors.New("版本记录失败")
)

// VcsCommitMeta is metadata stored in each workspace revision.
type VcsCommitMeta struct {
	SHA       string           `json:"sha"`
	ParentSHA string           `json:"parentSha,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
	Author    string           `json:"author"`
	Source    string           `json:"source"`
	Reason    string           `json:"reason"`
	Changes   []VcsPathChange  `json:"changes"`
}

// VcsPathChange describes one path operation in a revision.
type VcsPathChange struct {
	Path     string `json:"path"`
	Op       string `json:"op"` // write, delete, mkdir, rename, restore
	FromPath string `json:"fromPath,omitempty"`
}

// WorkspaceVcsService manages sidecar Git repos for agent workspace trees.
type WorkspaceVcsService struct {
	agentsRoot string
}

// NewWorkspaceVcsService creates the sidecar VCS service rooted beside agents.
func NewWorkspaceVcsService(agentsRoot string) *WorkspaceVcsService {
	return &WorkspaceVcsService{agentsRoot: agentsRoot}
}

func (v *WorkspaceVcsService) gitDir(agent string) string {
	return filepath.Join(v.agentsRoot, sanitize(agent), workspaceVcsDirName)
}

func (v *WorkspaceVcsService) workTree(agent string) (string, error) {
	s := &SkillService{root: v.agentsRoot}
	return s.ensureWorkspaceRoot(agent)
}

// vcsRestoreRef is a constant ref used so restore never puts a user SHA on argv.
const vcsRestoreRef = "refs/approving-restore"

// git runs git with a fixed argv list. User-controlled values must go through
// stdin (commit -F -, pathspec-from-file, rev-parse/log --stdin), never args.
func (v *WorkspaceVcsService) git(agent, stdin string, args ...string) (string, error) {
	gitDir := v.gitDir(agent)
	workTree, err := v.workTree(agent)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Env = append(os.Environ(),
		"GIT_DIR="+gitDir,
		"GIT_WORK_TREE="+workTree,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if stdout.Len() > 0 {
			msg = strings.TrimSpace(msg + " " + stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%w: %s", ErrVcsGitFailed, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func parseCatFileCommit(out string) (string, bool) {
	out = strings.TrimSpace(out)
	if out == "" || strings.HasSuffix(out, " missing") {
		return "", false
	}
	fields := strings.Fields(out)
	if len(fields) < 2 || fields[1] != "commit" {
		return "", false
	}
	return fields[0], true
}

func (v *WorkspaceVcsService) resolveCommit(agent, rev string) (string, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return "", ErrVcsRevisionMiss
	}
	out, err := v.git(agent, rev+"\n", "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return "", ErrVcsRevisionMiss
	}
	sha, ok := parseCatFileCommit(out)
	if !ok {
		return "", ErrVcsRevisionMiss
	}
	return sha, nil
}

func (v *WorkspaceVcsService) ensureRepo(agent string) error {
	gitDir := v.gitDir(agent)
	if fi, err := os.Stat(filepath.Join(gitDir, "HEAD")); err == nil && !fi.IsDir() {
		return nil
	}
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		return err
	}
	if _, err := v.git(agent, "", "init"); err != nil {
		return err
	}
	_, _ = v.git(agent, "", "config", "user.email", "approving-vcs@local")
	_, _ = v.git(agent, "", "config", "user.name", "Approving VCS")
	return nil
}

func (v *WorkspaceVcsService) hasCommits(agent string) bool {
	if _, err := os.Stat(v.gitDir(agent)); err != nil {
		return false
	}
	out, err := v.git(agent, "", "rev-parse", "HEAD")
	return err == nil && strings.TrimSpace(out) != ""
}

func (v *WorkspaceVcsService) ensureBaseline(agent string) error {
	if err := v.ensureRepo(agent); err != nil {
		return err
	}
	if v.hasCommits(agent) {
		return nil
	}
	if _, err := v.git(agent, "", "add", "-A"); err != nil {
		return err
	}
	msg := formatVcsMessage(VcsCommitMeta{
		Author:  "system",
		Source:  VcsSourceSystem,
		Reason:  "基线",
		Changes: []VcsPathChange{{Path: ".", Op: "baseline"}},
	})
	if _, err := v.git(agent, msg, "commit", "-F", "-"); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			if _, err2 := v.git(agent, msg, "commit", "--allow-empty", "-F", "-"); err2 != nil {
				return err2
			}
		} else {
			return err
		}
	}
	return nil
}

func vcsField(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), " ", "_")
}

func vcsFieldDecode(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}

func formatVcsMessage(meta VcsCommitMeta) string {
	subject := vcsCommitPrefix + " source=" + vcsField(meta.Source) +
		" author=" + vcsField(meta.Author) +
		" reason=" + vcsField(meta.Reason)
	for _, ch := range meta.Changes {
		if ch.Op == "rename" && ch.FromPath != "" {
			subject += fmt.Sprintf(" change=%s:%s->%s", ch.Op, ch.FromPath, ch.Path)
		} else {
			subject += fmt.Sprintf(" change=%s:%s", ch.Op, ch.Path)
		}
	}
	return subject
}

func parseVcsMessage(sha, subject string) VcsCommitMeta {
	meta := VcsCommitMeta{SHA: sha, Changes: []VcsPathChange{}}
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, vcsCommitPrefix) {
		meta.Reason = subject
		return meta
	}
	rest := strings.TrimSpace(strings.TrimPrefix(subject, vcsCommitPrefix))
	if i := strings.Index(rest, "source="); i >= 0 {
		seg := rest[i+7:]
		if j := strings.Index(seg, " "); j >= 0 {
			meta.Source = vcsFieldDecode(seg[:j])
		} else {
			meta.Source = vcsFieldDecode(seg)
		}
	}
	if i := strings.Index(rest, "author="); i >= 0 {
		seg := rest[i+7:]
		if j := strings.Index(seg, " reason="); j >= 0 {
			meta.Author = vcsFieldDecode(seg[:j])
		} else if j := strings.Index(seg, " change="); j >= 0 {
			meta.Author = vcsFieldDecode(seg[:j])
		} else {
			meta.Author = vcsFieldDecode(seg)
		}
	}
	if i := strings.Index(rest, "reason="); i >= 0 {
		seg := rest[i+7:]
		if j := strings.Index(seg, " change="); j >= 0 {
			meta.Reason = vcsFieldDecode(seg[:j])
		} else {
			meta.Reason = vcsFieldDecode(seg)
		}
	}
	for _, tok := range strings.Split(rest, " change=") {
		if tok == rest || !strings.Contains(tok, ":") {
			continue
		}
		if j := strings.Index(tok, " change="); j >= 0 {
			tok = tok[:j]
		}
		body := tok
		if k := strings.Index(body, ":"); k >= 0 {
			op := body[:k]
			pathPart := body[k+1:]
			ch := VcsPathChange{Op: op}
			if j := strings.Index(pathPart, "->"); j >= 0 {
				ch.FromPath = pathPart[:j]
				ch.Path = pathPart[j+2:]
			} else {
				ch.Path = pathPart
			}
			meta.Changes = append(meta.Changes, ch)
		}
	}
	return meta
}

// EnsureBaseline records the current workspace tree before the first mutating operation.
func (v *WorkspaceVcsService) EnsureBaseline(agent string) error {
	return v.ensureBaseline(agent)
}

// CommitChange records one workspace mutation as a single revision.
func (v *WorkspaceVcsService) CommitChange(agent string, meta VcsCommitMeta) (string, error) {
	if strings.TrimSpace(meta.Reason) == "" && meta.Source != VcsSourceSystem {
		return "", ErrVcsReasonRequired
	}
	if err := v.ensureRepo(agent); err != nil {
		return "", err
	}
	for _, ch := range meta.Changes {
		switch ch.Op {
		case "write", "mkdir", "rename":
			rel := strings.TrimSpace(ch.Path)
			if rel != "" && rel != "." {
				if _, err := v.git(agent, rel+"\n", "add", "--pathspec-from-file=-"); err != nil {
					return "", err
				}
			}
			if ch.Op == "rename" && ch.FromPath != "" {
				if _, err := v.git(agent, ch.FromPath+"\n", "add", "--pathspec-from-file=-"); err != nil {
					return "", err
				}
			}
		case "delete":
			rel := strings.TrimSpace(ch.Path)
			if rel != "" {
				if _, err := v.git(agent, rel+"\n", "rm", "-r", "--ignore-unmatch", "--pathspec-from-file=-"); err != nil {
					return "", err
				}
			}
		case "restore":
			if _, err := v.git(agent, "", "add", "-A"); err != nil {
				return "", err
			}
		case "baseline":
			if _, err := v.git(agent, "", "add", "-A"); err != nil {
				return "", err
			}
		}
	}
	msg := formatVcsMessage(meta)
	out, err := v.git(agent, msg, "commit", "-F", "-")
	if err != nil {
		return "", err
	}
	sha, _ := v.git(agent, "", "rev-parse", "HEAD")
	if sha == "" {
		// Parse from commit output: [branch abc1234]
		if i := strings.LastIndex(out, " "); i >= 0 {
			sha = strings.TrimRight(out[i+1:], "]")
		}
	}
	if sha == "" {
		sha, _ = v.git(agent, "", "rev-parse", "HEAD")
	}
	return sha, nil
}

// ListRevisions returns workspace revisions newest-first.
func (v *WorkspaceVcsService) ListRevisions(agent string, limit int) ([]VcsCommitMeta, error) {
	if !v.hasCommits(agent) {
		return []VcsCommitMeta{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	shasOut, err := v.git(agent, "", "log", "-n", "100", "--pretty=format:%H")
	if err != nil {
		return nil, err
	}
	var revs []VcsCommitMeta
	n := 0
	for _, sha := range strings.Split(strings.TrimSpace(shasOut), "\n") {
		sha = strings.TrimSpace(sha)
		if sha == "" {
			continue
		}
		if n >= limit {
			break
		}
		n++
		parent, _ := v.resolveCommit(agent, sha+"^")
		tsOut, _ := v.git(agent, sha+"\n", "log", "-1", "--pretty=format:%at", "--no-walk", "--stdin")
		subject, _ := v.git(agent, sha+"\n", "log", "-1", "--pretty=format:%s", "--no-walk", "--stdin")
		meta := parseVcsMessage(sha, subject)
		meta.SHA = sha
		meta.ParentSHA = parent
		if ts, err := parseInt64(strings.TrimSpace(tsOut)); err == nil {
			meta.CreatedAt = time.Unix(ts, 0).UTC()
		}
		revs = append(revs, meta)
	}
	return revs, nil
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// DiffRevision returns unified diff vs parent for one revision.
func (v *WorkspaceVcsService) DiffRevision(agent, sha string) (string, error) {
	if !v.hasCommits(agent) {
		return "", ErrVcsRevisionMiss
	}
	full, err := v.resolveCommit(agent, sha)
	if err != nil {
		return "", err
	}
	out, err := v.git(agent, full+"\n", "log", "-1", "--pretty=format:", "-p", "--stdin")
	if err != nil {
		return "", err
	}
	return out, nil
}

// RestoreRevision restores workspace to target sha and appends a restore revision.
func (v *WorkspaceVcsService) RestoreRevision(agent, sha, author, reason string) (string, error) {
	if strings.TrimSpace(reason) == "" {
		return "", ErrVcsReasonRequired
	}
	if err := v.ensureBaseline(agent); err != nil {
		return "", err
	}
	full, err := v.resolveCommit(agent, sha)
	if err != nil {
		return "", err
	}
	workTree, err := v.workTree(agent)
	if err != nil {
		return "", err
	}
	// Clean untracked except sidecar placeholder files
	if _, err := v.git(agent, "", "clean", "-fd"); err != nil {
		return "", err
	}
	if _, err := v.git(agent, "update "+vcsRestoreRef+" "+full+"\n", "update-ref", "--stdin"); err != nil {
		return "", err
	}
	if _, err := v.git(agent, "", "checkout", vcsRestoreRef, "--", "."); err != nil {
		return "", err
	}
	// Remove files added after target snapshot
	listOut, _ := v.git(agent, "", "ls-tree", "-r", "--name-only", vcsRestoreRef)
	keep := map[string]bool{"": true}
	for _, p := range strings.Split(listOut, "\n") {
		keep[strings.TrimSpace(p)] = true
	}
	_ = filepath.WalkDir(workTree, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(workTree, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !keep[rel] {
			_ = os.Remove(path)
		}
		return nil
	})
	return v.CommitChange(agent, VcsCommitMeta{
		Author:  author,
		Source:  VcsSourceRestore,
		Reason:  reason,
		Changes: []VcsPathChange{{Op: "restore", Path: full}},
	})
}

// RenameAgent moves the sidecar repo when an agent is renamed.
func (v *WorkspaceVcsService) RenameAgent(old, newName string) error {
	oldDir := v.gitDir(old)
	newDir := v.gitDir(newName)
	if _, err := os.Stat(oldDir); err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("target vcs dir already exists")
	}
	return os.Rename(oldDir, newDir)
}

// DeleteAgent removes the sidecar repo for an agent.
func (v *WorkspaceVcsService) DeleteAgent(agent string) error {
	return os.RemoveAll(v.gitDir(agent))
}

// DiffAgentFiles compares two file snapshots and returns path changes.
func DiffAgentFiles(before, after []AgentFile) []VcsPathChange {
	bm := map[string]string{}
	for _, f := range before {
		if p := safeRel(f.Path); p != "" {
			bm[p] = f.Content
		}
	}
	am := map[string]string{}
	for _, f := range after {
		if p := safeRel(f.Path); p != "" {
			am[p] = f.Content
		}
	}
	var changes []VcsPathChange
	for p, content := range am {
		if old, ok := bm[p]; !ok {
			changes = append(changes, VcsPathChange{Op: "write", Path: p})
		} else if old != content {
			changes = append(changes, VcsPathChange{Op: "write", Path: p})
		}
	}
	for p := range bm {
		if _, ok := am[p]; !ok {
			changes = append(changes, VcsPathChange{Op: "delete", Path: p})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Op != changes[j].Op {
			return changes[i].Op < changes[j].Op
		}
		return changes[i].Path < changes[j].Path
	})
	return changes
}

// CommitStudioSave records each changed path as its own revision with shared reason.
func (v *WorkspaceVcsService) CommitStudioSave(agent string, before, after []AgentFile, author, reason string) ([]string, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "Studio 保存"
	}
	changes := DiffAgentFiles(before, after)
	if len(changes) == 0 {
		return nil, nil
	}
	var shas []string
	for _, ch := range changes {
		sha, err := v.CommitChange(agent, VcsCommitMeta{
			Author:  author,
			Source:  VcsSourceStudio,
			Reason:  reason,
			Changes: []VcsPathChange{ch},
		})
		if err != nil {
			return shas, err
		}
		shas = append(shas, sha)
	}
	return shas, nil
}

// PlaceholderFileName is used to track empty directories in git.
const PlaceholderFileName = ".approving-dir-placeholder"

// EnsureDirTracked adds a placeholder so mkdir can be versioned.
func EnsureDirTracked(agent, rel string) error {
	// noop at service layer; mkdir in skill adds placeholder
	return nil
}
