package sandbox

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed skills_embed
var skillAssets embed.FS

// HomeBaseDir is the host base directory under which BuildConfigHome
// materializes per-sandbox config-home trees. Empty = OS temp dir.
//
// Set once at startup (from config.Sandbox.WorkDir). On the DinD deployment it
// MUST resolve to a volume mounted at the same path in both this process's
// container and the DinD daemon's container, because `docker run -v` resolves
// the bind-mount source from the daemon's filesystem, not ours.
var HomeBaseDir string

// ConfigHomeSpec describes how to assemble a per-node config home (e.g.
// /root/.cursor, /root/.claude) for an ACP backend.
type ConfigHomeSpec struct {
	// BaseWorkDirSrc, when set, is copied first (project shared Agent workspace).
	// Agent WorkDirSrc then overlays same-relative paths.
	BaseWorkDirSrc string
	// WorkDirSrc, when set, is the agent's working directory on the host
	// (<ProfilesRoot>/<agent>/workspace); its whole tree (rules/, skills/,
	// AGENTS.md, scripts, …) is copied verbatim into the config home.
	WorkDirSrc string
	// EmbeddedRules lists platform rule files (under skills_embed/) to embed
	// for this node type. See nodereg.EmbeddedRuleFiles.
	EmbeddedRules []string
	// IncludeArtifactStore writes the artifact-store convention rule. Only set
	// when the Agent has opted into the artifact-store MCP (convention-first):
	// the platform never auto-injects it.
	IncludeArtifactStore bool
	// MCP are the MCP servers (from the Agent config, with the reserved
	// artifact-store entry already resolved to its run-scoped URL+token by the
	// runtime) written into mcp.json.
	MCP []MCPServerSpec
	// Settings, when non-nil, is written as settings.json under the config
	// home (CodeBuddy staging needs envRouteMode+endpoint here; BASE_URL alone
	// hits the wrong chat path).
	Settings map[string]any
	// AgentName is the skill_profile used to resolve per-agent platform-rule
	// overrides under <ProfilesRoot>/<agent>/platform-rules/.
	AgentName string
	// ProfilesRoot is the agents root (e.g. data/profiles).
	ProfilesRoot string
	// GlobalRulesDir is the global platform-rules directory (e.g.
	// data/platform-rules).
	GlobalRulesDir string
}

// MCPServerSpec is one MCP server entry for mcp.json. URL-based (streamable
// HTTP, optional headers) or command-based (stdio).
type MCPServerSpec struct {
	Name    string
	URL     string
	Headers map[string]string
	Command string
	Args    []string
	Env     map[string]string
}

// BuildConfigHome materializes (rules/, commands/, skills/, mcp.json) to be
// RW bind-mounted into the sandbox at the agent's ConfigRoot, mirroring
// auto-coder's skill_store layout so the in-container ACP agent natively loads
// rules/skills and the Agent's declared MCP servers. Returns the host dir
// (caller removes it).
func BuildConfigHome(spec ConfigHomeSpec) (string, error) {
	if HomeBaseDir != "" {
		if err := os.MkdirAll(HomeBaseDir, 0o755); err != nil {
			return "", fmt.Errorf("create sandbox work dir %q: %w", HomeBaseDir, err)
		}
	}
	dir, err := os.MkdirTemp(HomeBaseDir, "approving-acp-")
	if err != nil {
		return "", err
	}

	// First lay down project-shared workspace (extend), then agent workspace
	// (overlay). Platform base rules + mcp.json layer on top so platform rules
	// can't be silently dropped by the agent.
	if spec.BaseWorkDirSrc != "" {
		if err := copyTree(spec.BaseWorkDirSrc, dir); err != nil {
			return "", err
		}
	}
	if spec.WorkDirSrc != "" {
		if err := copyTree(spec.WorkDirSrc, dir); err != nil {
			return "", err
		}
	}

	if err := os.MkdirAll(filepath.Join(dir, "rules"), 0o755); err != nil {
		return "", err
	}
	// Always-on base rule; artifact-store rule only when the Agent opted in.
	embedded := []string{"rules/base.md"}
	if spec.IncludeArtifactStore {
		embedded = append(embedded, "rules/artifact-store.md")
	}
	embedded = append(embedded, spec.EmbeddedRules...)
	for _, name := range embedded {
		b, err := resolvePlatformRule(name, spec.AgentName, spec.ProfilesRoot, spec.GlobalRulesDir)
		if err != nil {
			return "", fmt.Errorf("resolve platform rule %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return "", err
		}
	}

	servers := map[string]any{}
	for _, m := range spec.MCP {
		if m.Name == "" {
			continue
		}
		entry := map[string]any{}
		if m.URL != "" {
			// Claude Code / CodeBuddy (--strict-mcp-config): a url without type is
			// treated as a broken stdio server and skipped. Cursor accepts url-only;
			// type:http is required for Claude-family HTTP/streamable-http MCP.
			entry["type"] = "http"
			entry["url"] = m.URL
			if len(m.Headers) > 0 {
				entry["headers"] = m.Headers
			}
		} else if m.Command != "" {
			entry["command"] = m.Command
			if len(m.Args) > 0 {
				entry["args"] = m.Args
			}
			if len(m.Env) > 0 {
				entry["env"] = m.Env
			}
		} else {
			continue
		}
		servers[m.Name] = entry
	}
	if len(servers) > 0 {
		b, err := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal mcp.json: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "mcp.json"), b, 0o644); err != nil {
			return "", err
		}
	}
	if len(spec.Settings) > 0 {
		if err := writeMergedSettingsJSON(dir, spec.Settings); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// writeMergedSettingsJSON writes settings.json, merging platform Settings into an
// existing user-authored file (user keys win; nested maps merge recursively).
func writeMergedSettingsJSON(dir string, platform map[string]any) error {
	if len(platform) == 0 {
		return nil
	}
	path := filepath.Join(dir, "settings.json")
	merged := platform
	if b, err := os.ReadFile(path); err == nil {
		var user map[string]any
		if err := json.Unmarshal(b, &user); err == nil {
			merged = mergeSettingsMaps(platform, user)
		}
	}
	b, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	return os.WriteFile(path, b, 0o644)
}

func mergeSettingsMaps(platform, user map[string]any) map[string]any {
	out := make(map[string]any)
	for k, v := range platform {
		out[k] = v
	}
	for k, uv := range user {
		if pv, ok := out[k]; ok {
			pm := asStringAnyMap(pv)
			um := asStringAnyMap(uv)
			if pm != nil && um != nil {
				out[k] = mergeSettingsMaps(pm, um)
				continue
			}
		}
		out[k] = uv
	}
	return out
}

func asStringAnyMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[string]string:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out
	default:
		return nil
	}
}

// EmbeddedRuleBasenames returns sorted basenames of all embedded platform rules
// under skills_embed/rules/.
func EmbeddedRuleBasenames() ([]string, error) {
	entries, err := fs.ReadDir(skillAssets, "skills_embed/rules")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// ReadEmbeddedRule reads an embedded platform rule by relative path such as
// "rules/base.md".
func ReadEmbeddedRule(relPath string) ([]byte, error) {
	return skillAssets.ReadFile("skills_embed/" + relPath)
}

// platformRuleAgentPattern mirrors services path-layer agent names (Unicode L/N + `._-`).
var platformRuleAgentPattern = regexp.MustCompile(`^[\p{L}\p{N}._-]+$`)

func safePlatformRuleAgent(agentName string) string {
	base := filepath.Base(strings.TrimSpace(agentName))
	if base == "" || base == "." || base == ".." {
		return ""
	}
	if !platformRuleAgentPattern.MatchString(base) {
		return ""
	}
	return base
}

func resolvePlatformRule(relPath, agentName, profilesRoot, globalRulesDir string) ([]byte, error) {
	base := filepath.Base(relPath)
	if agentName != "" && profilesRoot != "" {
		agent := safePlatformRuleAgent(agentName)
		if agent != "" {
			p := filepath.Join(profilesRoot, agent, "platform-rules", base)
			if b, err := os.ReadFile(p); err == nil {
				return b, nil
			}
		}
	}
	if globalRulesDir != "" {
		p := filepath.Join(globalRulesDir, base)
		if b, err := os.ReadFile(p); err == nil {
			return b, nil
		}
	}
	return skillAssets.ReadFile("skills_embed/" + relPath)
}

// copyTree recursively copies the host directory src into dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
