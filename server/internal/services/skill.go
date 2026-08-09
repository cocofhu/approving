package services

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
)

// SkillService manages user-defined Agents on the filesystem. Each Agent owns a
// single working directory (a file tree) plus its MCP servers and env vars:
//
//	<root>/<agent>/workspace/<path...>   -- the agent's working dir (file tree)
//	<root>/<agent>/agent.json         -- MCP servers + environment vars
//
// Workflow nodes reference an agent by name via skill_profile. At run time the
// runtime copies the whole workspace/ tree into the sandbox config root (rules,
// skills, AGENTS.md, scripts, …), layers the platform base rules + the resolved
// mcp.json on top, and injects the env vars. Files the agent authors are loaded
// natively by the in-container ACP agent.
type SkillService struct {
	root string
	mu   sync.Mutex
}

// NewSkillService builds the service. It ships no preset agents (users create
// their own); it only migrates any existing agents to the current on-disk layout.
func NewSkillService(root string) *SkillService {
	s := &SkillService{root: root}
	s.seed()
	return s
}

// ArtifactStoreMCP is the conventional name for the platform's run-scoped
// artifact-store. The whole MCP config is user-authored; an Agent wires the
// artifact-store by referencing the run-scoped template vars
// (${APPROVING_ARTIFACT_URL} / ${APPROVING_ARTIFACT_TOKEN}) in its url/headers.
const ArtifactStoreMCP = "artifact-store"

// WorkDirName is the subfolder under each agent that holds its working-dir tree.
const WorkDirName = "workspace"

// legacyWorkDirName is the pre-vendor-neutral layout; kept for dual-read and
// auto-migration during the compatibility window (remove in 0.2.0).
const legacyWorkDirName = "cursor"

// MCPServer describes one MCP server an agent can talk to. A server is either
// URL-based (streamable HTTP, optional headers) or command-based (stdio).
type MCPServer struct {
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// AgentFile is one file in the agent's working directory (relative path +
// content). Paths use forward slashes and may be nested (e.g. rules/identity.md,
// skills/gitlab/SKILL.md).
type AgentFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Default sandbox-injection layout (sandbox protocol convention). Used when an
// Agent does not pin its own ConfigRoot / WorkspaceDir.
const (
	DefaultConfigRoot   = "/root/.cursor"
	DefaultWorkspaceDir = "/root/workspace"
)

// AgentLayout is the sandbox-injection layout for an Agent: where, inside the
// sandbox container, the platform mounts this Agent's config and clones the
// repo. Persisted per-agent (agent.json) and consumed at sandbox creation —
// the executor drives the bind-mount target and WORKSPACE_DIR from these,
// instead of a hardcoded path. mcp.json / rules/ / skills/ live under
// ConfigRoot as protocol-fixed sub-paths (derived, not stored).
type AgentLayout struct {
	// ConfigRoot is the container path the Agent's working dir (rules/, skills/,
	// mcp.json) is RW-mounted at. Empty → DefaultConfigRoot.
	ConfigRoot string `json:"configRoot,omitempty"`
	// WorkspaceDir is the container path the repo is cloned into / code runs in
	// (WORKSPACE_DIR). Empty → DefaultWorkspaceDir.
	WorkspaceDir string `json:"workspaceDir,omitempty"`
}

// withDefaults returns the layout with empty fields filled by protocol defaults.
func (l AgentLayout) withDefaults() AgentLayout {
	if strings.TrimSpace(l.ConfigRoot) == "" {
		l.ConfigRoot = DefaultConfigRoot
	}
	if strings.TrimSpace(l.WorkspaceDir) == "" {
		l.WorkspaceDir = DefaultWorkspaceDir
	}
	return l
}

// AcpBackend values bind a skill_profile to a sandbox ACP bridge.
const (
	AcpBackendCursor     = "cursor"
	AcpBackendClaudeCode = "claude_code"
	AcpBackendCodeBuddy  = "codebuddy"
	AcpBackendTrae       = "trae"
)

// NormalizeAcpBackend coerces unknown/empty values to cursor.
func NormalizeAcpBackend(raw string) string {
	switch strings.TrimSpace(raw) {
	case AcpBackendCursor, AcpBackendClaudeCode, AcpBackendCodeBuddy, AcpBackendTrae:
		return strings.TrimSpace(raw)
	default:
		return AcpBackendCursor
	}
}

// Allowed Agent-level git credential contracts (Studio UI metadata).
const (
	GitCredentialGitHubHTTPS = "github_https"
	GitCredentialGitLabHTTPS = "gitlab_https"
	GitCredentialSSH         = "ssh"
)

// normalizeGitCredentialType keeps only known values; unknown input is cleared
// so dirty agent.json / ZIP imports cannot leak misleading credential checks.
func normalizeGitCredentialType(raw string) string {
	v := strings.TrimSpace(raw)
	switch v {
	case "", GitCredentialGitHubHTTPS, GitCredentialGitLabHTTPS, GitCredentialSSH:
		return v
	default:
		log.Warn().Str("gitCredentialType", v).Msg("clearing invalid agent gitCredentialType")
		return ""
	}
}

// DefaultConfigRootForBackend returns the protocol default config root.
func DefaultConfigRootForBackend(backend string) string {
	switch NormalizeAcpBackend(backend) {
	case AcpBackendClaudeCode:
		return "/root/.claude"
	case AcpBackendCodeBuddy:
		return "/root/.codebuddy"
	case AcpBackendTrae:
		return "/root/.trae"
	default:
		return DefaultConfigRoot
	}
}

// Agent is a reusable, user-defined Agent identity: a working directory (file
// tree) plus the MCP servers and environment variables it runs with.
type Agent struct {
	Name string `json:"name"`
	// ProjectID is the Agent's single home project. Empty means unbound. When
	// unbound the Agent may only use the run-scoped artifact-store; the
	// project-scoped platform MCPs (memory-store / context-store /
	// task-scheduler) are rejected at save time and never injected at runtime.
	// Switching or clearing this field purges the Agent's data under the old
	// project (see PmService.PurgeAgentProjectData).
	ProjectID string `json:"projectId,omitempty"`
	// AcpBackend selects the ACP bridge (cursor | claude_code | codebuddy | trae).
	// Empty defaults to cursor for backward compatibility.
	AcpBackend string `json:"acpBackend,omitempty"`
	// GitCredentialType is the Agent-level credential contract selected in Studio.
	// Runtime credential injection continues to use Env; this field is UI metadata.
	GitCredentialType string `json:"gitCredentialType,omitempty"`
	// Files is the agent's working directory, copied into ConfigRoot at run.
	Files []AgentFile `json:"files"`
	// MCP lists the MCP servers wired into the sandbox for this agent.
	MCP []MCPServer `json:"mcp"`
	// Env are environment variables injected into the sandbox for this agent.
	Env map[string]string `json:"env"`
	// Layout is the sandbox-injection layout (config root + workspace dir).
	// Always returned with defaults applied so callers can use it verbatim.
	Layout AgentLayout `json:"layout"`
	// Prompts optionally overrides the platform-injected prompt text and rule
	// files for this Agent. Nil = use platform defaults for everything.
	Prompts *models.AgentPrompts `json:"prompts,omitempty"`
}

// agentConfig is the on-disk shape of agent.json (working-dir files live under
// the workspace/ subfolder).
type agentConfig struct {
	ProjectID         string               `json:"projectId,omitempty"`
	AcpBackend        string               `json:"acpBackend,omitempty"`
	GitCredentialType string               `json:"gitCredentialType,omitempty"`
	MCP               []MCPServer          `json:"mcp,omitempty"`
	Env               map[string]string    `json:"env,omitempty"`
	Layout            *AgentLayout         `json:"layout,omitempty"`
	Prompts           *models.AgentPrompts `json:"prompts,omitempty"`
}

// DefaultPlatformMCP returns the platform's built-in MCP server (the run-scoped
// artifact-store) wired via template vars — the run URL/token are resolved per
// run (see runtime.mcpVars), so no secret is persisted. New agents get this by
// default so they can read/write artifacts and use plan/ask tools out of the box.
func DefaultPlatformMCP() []MCPServer {
	return []MCPServer{{
		Name:    ArtifactStoreMCP,
		URL:     "${APPROVING_ARTIFACT_URL}",
		Headers: map[string]string{"Authorization": "Bearer ${APPROVING_ARTIFACT_TOKEN}"},
	}}
}

// seed no longer ships any preset agents — users create their own. It only runs
// on-disk layout migrations for any agents that already exist.
func (s *SkillService) seed() {
	s.migrateLegacy()
	s.migrateAllWorkDirs()
}

func (s *SkillService) migrateAllWorkDirs() {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s.migrateCursorWorkDir(e.Name())
	}
}

// migrateCursorWorkDir upgrades cursor/ → workspace/ when only the legacy dir
// exists. If workspace/ already exists, migration is skipped (workspace wins).
func (s *SkillService) migrateCursorWorkDir(name string) {
	dir := filepath.Join(s.root, sanitize(name))
	workspace := filepath.Join(dir, WorkDirName)
	legacy := filepath.Join(dir, legacyWorkDirName)
	if fi, err := os.Stat(workspace); err == nil && fi.IsDir() {
		return // canonical dir already present
	}
	if fi, err := os.Stat(legacy); err != nil || !fi.IsDir() {
		return // nothing to migrate
	}
	if err := os.Rename(legacy, workspace); err != nil {
		return
	}
	_ = os.RemoveAll(legacy)
}

// migrateLegacy upgrades any agent still using the old on-disk layout
// (rules.md + skills/<name>/) to the unified workspace/ working-dir tree, so both
// the UI and the runtime read a single consistent representation.
func (s *SkillService) migrateLegacy() {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(s.root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, WorkDirName)); err == nil {
			continue // already migrated
		}
		if _, err := os.Stat(filepath.Join(dir, legacyWorkDirName)); err == nil {
			continue // cursor/ layout handled by migrateCursorWorkDir
		}
		if _, err := os.Stat(filepath.Join(dir, "rules.md")); err != nil {
			if _, err := os.Stat(filepath.Join(dir, "skills")); err != nil {
				continue // nothing legacy to migrate
			}
		}
		if a, ok := s.Get(e.Name()); ok {
			if err := s.Save(a); err != nil { // Save writes workspace/ and removes the legacy files
				log.Warn().Err(err).Str("agent", e.Name()).Msg("legacy skill migrate save failed")
			}
		}
	}
}

// List returns all agents, sorted by name.
func (s *SkillService) List() []Agent {
	out := []Agent{}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if a, ok := s.Get(e.Name()); ok {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns one agent (working-dir files + mcp + env).
func (s *SkillService) Get(name string) (Agent, bool) {
	dir := filepath.Join(s.root, sanitize(name))
	if _, err := os.Stat(dir); err != nil {
		return Agent{}, false
	}
	cfg := s.readConfig(name)
	layout := AgentLayout{}
	if cfg.Layout != nil {
		layout = *cfg.Layout
	}
	backend := NormalizeAcpBackend(cfg.AcpBackend)
	layout = layout.withDefaults()
	if strings.TrimSpace(layout.ConfigRoot) == DefaultConfigRoot && backend != AcpBackendCursor {
		layout.ConfigRoot = DefaultConfigRootForBackend(backend)
	}
	return Agent{
		Name:              name,
		ProjectID:         strings.TrimSpace(cfg.ProjectID),
		AcpBackend:        backend,
		GitCredentialType: normalizeGitCredentialType(cfg.GitCredentialType),
		Files:             s.readFiles(name),
		MCP:               cfg.MCP,
		Env:               cfg.Env,
		Layout:            layout,
		Prompts:           cfg.Prompts,
	}, true
}

// readFiles loads the agent's working-dir tree from workspace/, falling back to
// legacy cursor/ during the compatibility window. If the agent still uses the
// oldest layout it synthesizes the tree from rules.md + skills/.
func (s *SkillService) readFiles(name string) []AgentFile {
	dir := filepath.Join(s.root, sanitize(name))
	if files := readTreeIfDir(filepath.Join(dir, WorkDirName)); len(files) > 0 {
		return files
	}
	if files := readTreeIfDir(filepath.Join(dir, legacyWorkDirName)); len(files) > 0 {
		return files
	}
	return s.legacyFiles(dir, name)
}

func readTreeIfDir(dir string) []AgentFile {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return nil
	}
	return readTree(dir)
}

// legacyFiles maps the old rules.md + skills/<name>/ layout to working-dir paths.
func (s *SkillService) legacyFiles(dir, name string) []AgentFile {
	var out []AgentFile
	if b, err := os.ReadFile(filepath.Join(dir, "rules.md")); err == nil {
		body := string(b)
		if !strings.HasPrefix(strings.TrimSpace(body), "---") {
			body = "---\ndescription: " + name + " 身份\nalwaysApply: true\n---\n\n" + body
		}
		out = append(out, AgentFile{Path: "rules/" + sanitize(name) + ".md", Content: body})
	}
	skills := filepath.Join(dir, "skills")
	for _, f := range readTree(skills) {
		out = append(out, AgentFile{Path: "skills/" + f.Path, Content: f.Content})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// readTree walks dir and returns every file as a slash-relative AgentFile.
func readTree(dir string) []AgentFile {
	var out []AgentFile
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		out = append(out, AgentFile{Path: filepath.ToSlash(rel), Content: string(b)})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// safeRel cleans a working-dir-relative path and rejects traversal/absolute paths.
func safeRel(p string) string {
	p = strings.TrimSpace(filepath.ToSlash(p))
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return ""
	}
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	if p == "" || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "..") {
		return ""
	}
	return p
}

// underRoot joins root/rel and asserts the result stays within root (Zip Slip /
// path-traversal barrier for CodeQL #17/#18/#19).
func underRoot(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return "", fmt.Errorf("invalid path %q", rel)
	}
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("non-local path %q", rel)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(absRoot, rel)
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	sep := string(os.PathSeparator)
	if absFull != absRoot && !strings.HasPrefix(absFull, absRoot+sep) {
		return "", fmt.Errorf("path %q escapes root", rel)
	}
	return absFull, nil
}

func (s *SkillService) readConfig(name string) agentConfig {
	var cfg agentConfig
	b, err := os.ReadFile(filepath.Join(s.root, sanitize(name), "agent.json"))
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg
}

// UpdateProjectID writes only agent.json.projectId. Unlike Save it does not
// RemoveAll(workspace), rewrite files, or touch MCP / env / layout / prompts.
// Used by group-level assign so unrelated drafts and workspace stay intact.
func (s *SkillService) UpdateProjectID(name, projectID string) error {
	n := sanitize(name)
	if n == "" {
		return fmt.Errorf("invalid agent name")
	}
	if !s.Exists(n) {
		return fmt.Errorf("agent %q not found", name)
	}
	dir := filepath.Join(s.root, n)
	cfg := s.readConfig(n)
	cfg.ProjectID = strings.TrimSpace(projectID)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "agent.json"), b, 0o644)
}

// Save writes an agent's working-dir tree + config, creating it if needed. The
// workspace/ tree is fully rewritten so removed files disappear from disk; the
// legacy rules.md / skills/ / cursor/ are dropped on save.
func (s *SkillService) Save(a Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnlocked(a)
}

func (s *SkillService) saveUnlocked(a Agent) error {
	name := sanitize(a.Name)
	if name == "" {
		return fmt.Errorf("invalid agent name")
	}
	s.migrateCursorWorkDir(name)
	dir := filepath.Join(s.root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	layout := a.Layout.withDefaults()
	backend := NormalizeAcpBackend(a.AcpBackend)
	if strings.TrimSpace(a.Layout.ConfigRoot) == "" {
		layout.ConfigRoot = DefaultConfigRootForBackend(backend)
	}
	cfg := agentConfig{
		ProjectID:         strings.TrimSpace(a.ProjectID),
		AcpBackend:        backend,
		GitCredentialType: normalizeGitCredentialType(a.GitCredentialType),
		MCP:               a.MCP,
		Env:               a.Env,
		Layout:            &layout,
		Prompts:           a.Prompts,
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), b, 0o644); err != nil {
		return err
	}
	work := filepath.Join(dir, WorkDirName)
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	for _, f := range a.Files {
		rel := safeRel(f.Path)
		if rel == "" {
			continue
		}
		full, err := underRoot(work, rel)
		if err != nil {
			return fmt.Errorf("file %q: %w", f.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			return err
		}
	}
	// Drop legacy layouts now that the working dir is authoritative.
	_ = os.Remove(filepath.Join(dir, "rules.md"))
	_ = os.RemoveAll(filepath.Join(dir, "skills"))
	_ = os.RemoveAll(filepath.Join(dir, legacyWorkDirName))
	return nil
}

// Delete removes an agent and all its files.
func (s *SkillService) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteUnlocked(name)
}

func (s *SkillService) deleteUnlocked(name string) error {
	n := sanitize(name)
	if n == "" {
		return fmt.Errorf("invalid agent name")
	}
	return os.RemoveAll(filepath.Join(s.root, n))
}

// Rename atomically renames an agent directory (old -> newName) via os.Rename,
// so the change either fully succeeds or leaves the old agent untouched (no
// half-copied intermediate state).
func (s *SkillService) Rename(old, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, n := sanitize(old), sanitize(newName)
	if o == "" || n == "" {
		return fmt.Errorf("invalid agent name")
	}
	if o == n {
		return nil
	}
	if !s.Exists(o) {
		return fmt.Errorf("agent %q not found", old)
	}
	if s.Exists(n) {
		return fmt.Errorf("agent %q already exists", newName)
	}
	return os.Rename(filepath.Join(s.root, o), filepath.Join(s.root, n))
}

// Exists reports whether an agent directory is present.
func (s *SkillService) Exists(name string) bool {
	n := sanitize(name)
	if n == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(s.root, n))
	return err == nil
}

// WorkDir returns the agent's on-disk working directory (workspace/ or legacy
// cursor/) if it exists, for verbatim copy into the sandbox config root.
func (s *SkillService) WorkDir(name string) string {
	dir := filepath.Join(s.root, sanitize(name))
	for _, sub := range []string{WorkDirName, legacyWorkDirName} {
		d := filepath.Join(dir, sub)
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	return ""
}

// sanitize prevents path traversal in agent names (no ReplaceAll("..","") incomplete sanitization).
// Path layer accepts Unicode L/N + `._-` so legacy dotted names (e.g. clarify.v1) still resolve.
// Write-identity rules (Create/Rename targets) live in NormalizeAndValidateAgentName.
func sanitize(name string) string {
	return sanitizeAgentPath(name)
}
