package services

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed all:onboarding_embed
var onboardingEmbedFS embed.FS

const onboardingEmbedRoot = "onboarding_embed"

// OnboardingAgentNames is the fixed ordered set of agents created by bootstrap.
var OnboardingAgentNames = []string{
	"ClarifyAgent",
	"VisualAgent",
	"ImplementAgent",
	"TestAgent",
	"ReviewAgent",
	"PreviewAgent",
}

// loadOnboardingAgentTemplate reads one agent template from the embedded FS.
func loadOnboardingAgentTemplate(name string) (Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Agent{}, fmt.Errorf("empty agent template name")
	}
	cfgPath := path.Join(onboardingEmbedRoot, name, "agent.json")
	raw, err := onboardingEmbedFS.ReadFile(cfgPath)
	if err != nil {
		return Agent{}, fmt.Errorf("read embed agent.json for %s: %w", name, err)
	}
	var cfg agentConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Agent{}, fmt.Errorf("parse embed agent.json for %s: %w", name, err)
	}
	files, err := readEmbedWorkspaceFiles(path.Join(onboardingEmbedRoot, name, WorkDirName))
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
		Name:       name,
		AcpBackend: NormalizeAcpBackend(cfg.AcpBackend),
		Files:      files,
		MCP:        mcp,
		Env:        env,
		Layout:     layout,
		Prompts:    cfg.Prompts,
	}, nil
}

func readEmbedWorkspaceFiles(root string) ([]AgentFile, error) {
	var out []AgentFile
	prefix := strings.TrimSuffix(root, "/") + "/"
	err := fs.WalkDir(onboardingEmbedFS, root, func(p string, d fs.DirEntry, err error) error {
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
		b, err := onboardingEmbedFS.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, AgentFile{Path: rel, Content: string(b)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk embed workspace %s: %w", root, err)
	}
	return out, nil
}
