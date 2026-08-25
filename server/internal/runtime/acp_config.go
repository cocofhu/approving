package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
	"github.com/cocofhu/approving/internal/sandbox"
	"github.com/rs/zerolog/log"
)

func (c *acpProvider) agentLayout(profile string, cfg agentFile) agentLayout {
	backend := c.backend
	if b := NormalizeBackend(cfg.AcpBackend); cfg.AcpBackend != "" {
		backend = b
	}
	root := cfg.Layout.ConfigRoot
	ws := cfg.Layout.WorkspaceDir
	if strings.TrimSpace(root) == "" {
		root = ResolveConfigRoot(backend, "")
	}
	if strings.TrimSpace(ws) == "" {
		ws = "/root/workspace"
	}
	return agentLayout{ConfigRoot: root, WorkspaceDir: ws}
}

// buildConfigHome materializes the per-node config home (rules/skills + mcp.json).
func (c *acpProvider) buildConfigHome(req NodeReq, env map[string]string) string {
	profile := str2(req.Config["skill_profile"])
	specs := c.resolvedMCPSpecs(req)
	home, err := sandbox.BuildConfigHome(sandbox.ConfigHomeSpec{
		BaseWorkDirSrc:       c.sharedWorkDir(req),
		WorkDirSrc:           c.workDir(profile),
		EmbeddedRules:        nodereg.EmbeddedRuleFiles(req.NodeType),
		IncludeArtifactStore: hasArtifactStore(specs),
		MCP:                  specs,
		Settings:             CodeBuddySettingsForEnv(c.backend, env),
		AgentName:            profile,
		ProfilesRoot:         c.opts.ProfilesRoot,
		GlobalRulesDir:       c.opts.PlatformRulesRoot,
	})
	if err != nil {
		log.Warn().Err(err).Str("node", req.NodeID).Msg("build cursor home failed; running without /root/.cursor mount")
		return ""
	}
	return home
}

const (
	agentWorkDirName       = "workspace"
	legacyAgentWorkDirName = "cursor" // compatibility window; remove in 0.2.0
)

// workDir returns the agent's on-disk working directory (workspace/ or legacy
// cursor/) if it exists, for verbatim copy into the sandbox config root.
func (c *acpProvider) workDir(profile string) string {
	if profile == "" || c.opts.ProfilesRoot == "" {
		return ""
	}
	base, err := profileDir(c.opts.ProfilesRoot, profile)
	if err != nil {
		return ""
	}
	for _, sub := range []string{agentWorkDirName, legacyAgentWorkDirName} {
		d := filepath.Join(base, sub)
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	return ""
}

// agentMCP mirrors one MCP server entry of agent.json.
type agentMCP struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// agentLayout mirrors the sandbox-injection layout in agent.json (config root +
// workspace dir). Empty fields fall back to protocol defaults at use site.
type agentLayout struct {
	ConfigRoot   string `json:"configRoot"`
	WorkspaceDir string `json:"workspaceDir"`
}

// agentFile mirrors <ProfilesRoot>/<profile>/agent.json (mcp + env + layout +
// per-Agent prompt overrides).
type agentFile struct {
	AcpBackend string               `json:"acpBackend"`
	MCP        []agentMCP           `json:"mcp"`
	Env        map[string]string    `json:"env"`
	Layout     agentLayout          `json:"layout"`
	Prompts    *models.AgentPrompts `json:"prompts"`
}

// agentConfig reads the Agent's agent.json (best effort; empty on miss).
func (c *acpProvider) agentConfig(profile string) agentFile {
	var f agentFile
	if profile == "" || c.opts.ProfilesRoot == "" {
		return f
	}
	dir, err := profileDir(c.opts.ProfilesRoot, profile)
	if err != nil {
		return f
	}
	b, err := os.ReadFile(filepath.Join(dir, "agent.json"))
	if err != nil {
		return f
	}
	_ = json.Unmarshal(b, &f)
	return f
}

// projectIDForReq resolves the extend source project for a workflow node.
func (c *acpProvider) projectIDForReq(req NodeReq) string {
	if c.opts.ProjectIDForWorkflow == nil || req.WorkflowID == "" {
		return ""
	}
	return strings.TrimSpace(c.opts.ProjectIDForWorkflow(req.WorkflowID))
}

func (c *acpProvider) sharedView(req NodeReq) (SharedAgentView, bool) {
	pid := c.projectIDForReq(req)
	if pid == "" || c.opts.SharedAgentForProject == nil {
		return SharedAgentView{}, false
	}
	return c.opts.SharedAgentForProject(pid), true
}

func (c *acpProvider) sharedWorkDir(req NodeReq) string {
	if v, ok := c.sharedView(req); ok {
		return v.WorkDir
	}
	return ""
}

// effectiveAgent returns extend(shared) → overlay(agent) for this node.
func (c *acpProvider) effectiveAgent(req NodeReq) agentFile {
	agent := c.agentConfig(str2(req.Config["skill_profile"]))
	shared, ok := c.sharedView(req)
	if !ok {
		return agent
	}
	return overlayAgentFile(shared, agent)
}

func overlayAgentFile(shared SharedAgentView, agent agentFile) agentFile {
	out := agent
	// Env: shared base, agent keys win.
	env := map[string]string{}
	for k, v := range shared.Env {
		if strings.TrimSpace(k) == "" {
			continue
		}
		env[k] = v
	}
	for k, v := range agent.Env {
		if strings.TrimSpace(k) == "" {
			continue
		}
		env[k] = v
	}
	out.Env = env
	// MCP by name
	byName := map[string]agentMCP{}
	order := make([]string, 0, len(shared.MCP)+len(agent.MCP))
	addMCP := func(m agentMCP) {
		n := strings.TrimSpace(m.Name)
		if n == "" {
			return
		}
		m.Name = n
		if _, ok := byName[n]; !ok {
			order = append(order, n)
		}
		byName[n] = m
	}
	for _, m := range shared.MCP {
		addMCP(agentMCP{Name: m.Name, URL: m.URL, Headers: m.Headers, Command: m.Command, Args: m.Args, Env: m.Env})
	}
	for _, m := range agent.MCP {
		addMCP(m)
	}
	out.MCP = make([]agentMCP, 0, len(order))
	for _, n := range order {
		out.MCP = append(out.MCP, byName[n])
	}
	// Meta / layout: non-empty agent wins
	if strings.TrimSpace(agent.AcpBackend) == "" && strings.TrimSpace(shared.AcpBackend) != "" {
		out.AcpBackend = shared.AcpBackend
	}
	if strings.TrimSpace(agent.Layout.ConfigRoot) == "" && strings.TrimSpace(shared.Layout.ConfigRoot) != "" {
		out.Layout.ConfigRoot = shared.Layout.ConfigRoot
	}
	if strings.TrimSpace(agent.Layout.WorkspaceDir) == "" && strings.TrimSpace(shared.Layout.WorkspaceDir) != "" {
		out.Layout.WorkspaceDir = shared.Layout.WorkspaceDir
	}
	out.Prompts = mergePromptPtrs(shared.Prompts, agent.Prompts)
	return out
}

func mergePromptPtrs(base, overlay *models.AgentPrompts) *models.AgentPrompts {
	if base == nil && overlay == nil {
		return nil
	}
	bm := map[string]string{}
	om := map[string]string{}
	if base != nil {
		b, _ := json.Marshal(base)
		_ = json.Unmarshal(b, &bm)
	}
	if overlay != nil {
		b, _ := json.Marshal(overlay)
		_ = json.Unmarshal(b, &om)
	}
	out := map[string]string{}
	for k, v := range bm {
		out[k] = v
	}
	for k, v := range om {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	b, err := json.Marshal(out)
	if err != nil {
		return overlay
	}
	var p models.AgentPrompts
	if err := json.Unmarshal(b, &p); err != nil {
		return overlay
	}
	return &p
}

// reservedArtifactStore is the conventional name for the platform's run-scoped
// artifact-store MCP. It is NOT special-cased in mcp.json: the whole MCP config
// is user-authored. The run-scoped endpoint + token are exposed only as
// template vars (see mcpVars) that the user references inside their config,
// e.g. url "${APPROVING_ARTIFACT_URL}" + header "Bearer ${APPROVING_ARTIFACT_TOKEN}".
// The name is used only to gate the convention doc rule and as the UI default.
const reservedArtifactStore = "artifact-store"

// mcpVars are the run-scoped template variables substituted into the Agent's
// MCP config and env values at runtime. They are the only way the dynamic
// artifact-store URL/token reach the user-authored mcp.json — so the token is
// never persisted in config and stays bound to this run (per-run isolation).
// gitToken resolves GITLAB_TOKEN from the platform env and the Agent-meta env
// (with ${...} substitution), mirroring how spec() builds the sandbox env. It
// gates optional MR creation; empty means "no credentials, skip MR".
func (c *acpProvider) gitToken(req NodeReq) string {
	vars := c.mcpVars(req)
	if v := substVars(c.effectiveAgent(req).Env["GITLAB_TOKEN"], vars); v != "" {
		return v
	}
	return c.opts.Env["GITLAB_TOKEN"]
}

// gitLabURL resolves GITLAB_URL for GitLab detection and MR gating. Explicit
// agent GITLAB_URL wins; otherwise derive from the node's repo URL only when
// GITLAB_TOKEN is configured and the repo is not GitHub (avoids a misconfigured
// token on GitHub).
func (c *acpProvider) gitLabURL(req NodeReq) string {
	vars := c.mcpVars(req)
	if v := substVars(c.effectiveAgent(req).Env["GITLAB_URL"], vars); v != "" {
		return v
	}
	repo := c.nodeRepoURL(req)
	host := gitRepoHost(repo)
	if host == "github.com" {
		return ""
	}
	if c.gitToken(req) != "" {
		return gitBaseURL(repo)
	}
	return ""
}

func (c *acpProvider) mcpVars(req NodeReq) map[string]string {
	m := map[string]string{
		"APPROVING_ARTIFACT_URL":   c.mcpURL(req),
		"APPROVING_ARTIFACT_TOKEN": req.Token,
		"APPROVING_RUN_ID":         req.RunID,
		"APPROVING_NODE_ID":        req.NodeID,
	}

	for k, v := range req.Vars {
		if k == "" {
			continue
		}
		m["vars."+k] = str2(v)
	}

	m["vars.repos"] = sandbox.EncodeRepos(resolveRepos(req))
	return m
}

func substVars(s string, vars map[string]string) string {
	if s == "" {
		return s
	}
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

func substMap(m, vars map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = substVars(v, vars)
	}
	return out
}

func substSlice(s []string, vars map[string]string) []string {
	if len(s) == 0 {
		return nil
	}
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = substVars(v, vars)
	}
	return out
}

// resolvedMCPSpecs maps the Agent's user-authored MCP servers to sandbox specs,
// substituting the run-scoped template vars into every string field. A server
// whose URL/command resolves to empty (e.g. artifact-store while the MCP
// endpoint is disabled) is dropped.
//
// Workflow ACP never auto-injects live memory/context/scheduler endpoints;
// declared entries for those names (or URL paths) are dropped here.
func (c *acpProvider) resolvedMCPSpecs(req NodeReq) []sandbox.MCPServerSpec {
	mcp := c.effectiveAgent(req).MCP
	if len(mcp) == 0 {
		return nil
	}
	vars := c.mcpVars(req)
	out := make([]sandbox.MCPServerSpec, 0, len(mcp))
	for _, m := range mcp {
		if m.Name == "" {
			continue
		}

		if isWorkflowDroppedProjectPlatformMCP(m.Name, m.URL) {
			continue
		}
		url := config.RewriteMisconfiguredMCPAdvertise(substVars(m.URL, vars))
		cmd := substVars(m.Command, vars)
		if url == "" && cmd == "" {
			continue
		}
		out = append(out, sandbox.MCPServerSpec{
			Name:    m.Name,
			URL:     url,
			Headers: substMap(m.Headers, vars),
			Command: cmd,
			Args:    substSlice(m.Args, vars),
			Env:     substMap(m.Env, vars),
		})
	}
	return out
}

func isWorkflowDroppedProjectPlatformMCP(name, url string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "memory-store", "context-store", "task-scheduler":
		return true
	}
	return strings.Contains(url, "/mcp/memory-store/") ||
		strings.Contains(url, "/mcp/context-store/") ||
		strings.Contains(url, "/mcp/task-scheduler/")
}

// hasArtifactStore reports whether a server named artifact-store survived
// resolution (used only to gate its convention doc rule).
func hasArtifactStore(specs []sandbox.MCPServerSpec) bool {
	for _, s := range specs {
		if s.Name == reservedArtifactStore {
			return true
		}
	}
	return false
}

// mcpURL is the run-scoped artifact-store MCP endpoint reachable from inside
// the sandbox (matches router path /mcp/runs/:runId). Empty when unconfigured.
// Prefers live config (hot-reload) over the boot-time Options snapshot.
func (c *acpProvider) mcpURL(req NodeReq) string {
	base := config.ResolveMCPAdvertise(c.opts.MCPEndpoint)
	if base == "" {
		return ""
	}
	return base + "/mcp/runs/" + req.RunID
}

// mcpServers builds the ACP mcpServers array injected at session/new from the
// Agent's declared MCP servers (with artifact-store resolved to its run-scoped
// URL+token). Returns nil when the Agent declares none.
func (c *acpProvider) mcpServers(req NodeReq) json.RawMessage {
	specs := c.resolvedMCPSpecs(req)
	if len(specs) == 0 {
		return nil
	}
	servers := make([]map[string]any, 0, len(specs))
	for _, m := range specs {
		switch {
		case m.URL != "":

			entry := map[string]any{"name": m.Name, "type": "http", "url": m.URL}
			if len(m.Headers) > 0 {
				hs := make([]map[string]string, 0, len(m.Headers))
				for k, v := range m.Headers {
					hs = append(hs, map[string]string{"name": k, "value": v})
				}
				entry["headers"] = hs
			}
			servers = append(servers, entry)
		case m.Command != "":
			entry := map[string]any{"name": m.Name, "command": m.Command}
			if len(m.Args) > 0 {
				entry["args"] = m.Args
			}
			if len(m.Env) > 0 {
				es := make([]map[string]string, 0, len(m.Env))
				for k, v := range m.Env {
					es = append(es, map[string]string{"name": k, "value": v})
				}
				entry["env"] = es
			}
			servers = append(servers, entry)
		}
	}
	if len(servers) == 0 {
		return nil
	}
	b, _ := json.Marshal(servers)
	return b
}
