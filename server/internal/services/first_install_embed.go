package services

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/cocofhu/approving/internal/models"
)

//go:embed all:first_install_embed
var firstInstallEmbedFS embed.FS

const firstInstallEmbedRoot = "first_install_embed"

// OnboardingAgentNames is the first-install 综合项目组 roster (fixed names).
var OnboardingAgentNames = []string{
	"综合AI技术产品",
	"综合研发工程师",
	"综合测试工程师",
	"综合代码审查工程师",
	"综合运维工程师",
	"综合项目组组长",
}

func loadFirstInstallAgentTemplate(name string) (Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Agent{}, fmt.Errorf("empty agent template name")
	}
	cfgPath := path.Join(firstInstallEmbedRoot, "folder", name, "agent.json")
	raw, err := firstInstallEmbedFS.ReadFile(cfgPath)
	if err != nil {
		return Agent{}, fmt.Errorf("read first-install agent.json for %s: %w", name, err)
	}
	var cfg agentConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Agent{}, fmt.Errorf("parse first-install agent.json for %s: %w", name, err)
	}
	files, err := readFirstInstallWorkspaceFiles(path.Join(firstInstallEmbedRoot, "folder", name, WorkDirName))
	if err != nil {
		return Agent{}, err
	}
	env := map[string]string{}
	for k, v := range cfg.Env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		env[k] = v
	}
	env = stripTokenKeysFromEnvMap(env)
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
		Name:       name,
		AcpBackend: NormalizeAcpBackend(cfg.AcpBackend),
		Files:      files,
		MCP:        mcp,
		Env:        env,
		Layout:     layout,
		Prompts:    cfg.Prompts,
	}, nil
}

func readFirstInstallWorkspaceFiles(root string) ([]AgentFile, error) {
	var out []AgentFile
	prefix := strings.TrimSuffix(root, "/") + "/"
	err := fs.WalkDir(firstInstallEmbedFS, root, func(p string, d fs.DirEntry, err error) error {
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
		b, err := firstInstallEmbedFS.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, AgentFile{Path: rel, Content: string(b)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk first-install workspace %s: %w", root, err)
	}
	return out, nil
}

func loadFirstInstallWorkflowEnvelope() (models.ExportEnvelope, error) {
	raw, err := firstInstallEmbedFS.ReadFile(path.Join(firstInstallEmbedRoot, "default-workflow.json"))
	if err != nil {
		return models.ExportEnvelope{}, fmt.Errorf("read default workflow embed: %w", err)
	}
	env, err := ValidateImport(raw)
	if err != nil {
		return models.ExportEnvelope{}, err
	}
	env.Name = OnboardingWorkflowName
	env.NeedsRepo = true
	return env, nil
}
