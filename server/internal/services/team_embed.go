package services

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed all:team_embed
var teamEmbedFS embed.FS

const teamEmbedRoot = "team_embed"

// TeamRoleTemplate describes one engineer role in the reference roster (1 PM + 9).
type TeamRoleTemplate struct {
	ID          string `json:"id"`
	EmbedName   string `json:"embedName"`
	RoleLabelZH string `json:"roleLabelZh"`
	Summary     string `json:"summary"`
}

// TeamEngineerTemplates is the fixed 9-role pipeline roster.
var TeamEngineerTemplates = []TeamRoleTemplate{
	{ID: "research", EmbedName: "ResearchAgent", RoleLabelZH: "调研工程师", Summary: "技术与竞品调研"},
	{ID: "plan", EmbedName: "PlanAgent", RoleLabelZH: "计划工程师", Summary: "拆解实现计划"},
	{ID: "proposal", EmbedName: "ProposalAgent", RoleLabelZH: "方案工程师", Summary: "方案提案"},
	{ID: "clarify", EmbedName: "ClarifyAgent", RoleLabelZH: "澄清工程师", Summary: "需求澄清"},
	{ID: "visual", EmbedName: "VisualAgent", RoleLabelZH: "视觉原型工程师", Summary: "可预览视觉原型"},
	{ID: "implement", EmbedName: "ImplementAgent", RoleLabelZH: "实现工程师", Summary: "代码实现与推送"},
	{ID: "test", EmbedName: "TestAgent", RoleLabelZH: "测试工程师", Summary: "测试验证"},
	{ID: "review", EmbedName: "ReviewAgent", RoleLabelZH: "代码Review工程师", Summary: "代码复核"},
	{ID: "preview", EmbedName: "PreviewAgent", RoleLabelZH: "变更摘要视觉工程师", Summary: "变更摘要视觉预览"},
}

// TeamRoleByID returns a template by id.
func TeamRoleByID(id string) (TeamRoleTemplate, bool) {
	id = strings.TrimSpace(id)
	for _, t := range TeamEngineerTemplates {
		if t.ID == id {
			return t, true
		}
	}
	return TeamRoleTemplate{}, false
}

// EngineerDisplayName builds "{prefix}{roleLabel}".
func EngineerDisplayName(prefix, roleLabelZH string) string {
	return strings.TrimSpace(prefix) + strings.TrimSpace(roleLabelZH)
}

// PMDisplayName builds "{prefix}项目经理".
func PMDisplayName(prefix string) string {
	return strings.TrimSpace(prefix) + "项目经理"
}

func loadTeamAgentTemplate(embedName string) (Agent, error) {
	embedName = strings.TrimSpace(embedName)
	if embedName == "" {
		return Agent{}, fmt.Errorf("empty team template name")
	}
	cfgPath := path.Join(teamEmbedRoot, embedName, "agent.json")
	raw, err := teamEmbedFS.ReadFile(cfgPath)
	if err != nil {
		return Agent{}, fmt.Errorf("read team embed agent.json for %s: %w", embedName, err)
	}
	var cfg agentConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Agent{}, fmt.Errorf("parse team embed agent.json for %s: %w", embedName, err)
	}
	files, err := readTeamEmbedWorkspaceFiles(path.Join(teamEmbedRoot, embedName, WorkDirName))
	if err != nil {
		return Agent{}, err
	}
	env := map[string]string{}
	for k, v := range cfg.Env {
		env[k] = v
	}
	if _, ok := env["GIT_REPOS"]; !ok {
		env["GIT_REPOS"] = "${vars.repos}"
	}
	mcp := cfg.MCP
	if len(mcp) == 0 {
		mcp = DefaultPlatformMCP()
	}
	layout := AgentLayout{}
	if cfg.Layout != nil {
		layout = *cfg.Layout
	}
	return Agent{
		Name:       embedName,
		AcpBackend: NormalizeAcpBackend(cfg.AcpBackend),
		Files:      files,
		MCP:        mcp,
		Env:        env,
		Layout:     layout,
		Prompts:    cfg.Prompts,
	}, nil
}

func readTeamEmbedWorkspaceFiles(root string) ([]AgentFile, error) {
	if _, err := fs.Stat(teamEmbedFS, root); err != nil {
		return nil, fmt.Errorf("team embed workspace missing %s: %w (rebuild image with team_embed/*.md included)", root, err)
	}
	var out []AgentFile
	prefix := strings.TrimSuffix(root, "/") + "/"
	err := fs.WalkDir(teamEmbedFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, prefix)
		rel = path.Clean(rel)
		if rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
			return nil
		}
		b, err := teamEmbedFS.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, AgentFile{Path: rel, Content: string(b)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk team embed workspace %s: %w", root, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("team embed workspace empty %s (check Docker .md allowlist for team_embed)", root)
	}
	return out, nil
}
