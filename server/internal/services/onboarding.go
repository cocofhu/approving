package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"
	"github.com/cocofhu/approving/internal/sandbox"

	"github.com/google/uuid"
)

const (
	// OnboardingWorkflowName is the first-install published workflow.
	OnboardingWorkflowName = "默认工作流"
	// FirstInstallGroupName is the org folder created on first install.
	FirstInstallGroupName = "综合项目组"
	// FirstInstallGroupID is a stable group id so re-bootstrap is idempotent.
	FirstInstallGroupID = "g_first_install_zonghe"
)

var (
	// ErrOnboardingAPIKeyRequired is returned when bootstrap is called without an API key.
	ErrOnboardingAPIKeyRequired = errors.New("apiKey is required")
	// ErrOnboardingProjectNotFound is returned when the project id does not exist.
	ErrOnboardingProjectNotFound = errors.New("project not found")
	// ErrOnboardingAgentConflict is returned when a fixed-name agent already belongs to another project.
	ErrOnboardingAgentConflict = errors.New("onboarding agent already bound to another project")
	// ErrOnboardingNotDefaultProject is returned when bootstrap is not the default project.
	ErrOnboardingNotDefaultProject = errors.New("first-install onboarding is only allowed on the default project")
)

// OnboardingBootstrapRequest is the body for POST .../bootstrap-onboarding.
type OnboardingBootstrapRequest struct {
	AcpBackend        string `json:"acpBackend"`
	APIKey            string `json:"apiKey"`
	Region            string `json:"region,omitempty"`
	GitCredentialType string `json:"gitCredentialType,omitempty"`
	GitHubToken       string `json:"githubToken,omitempty"`
	GitLabToken       string `json:"gitlabToken,omitempty"`
	GitLabURL         string `json:"gitlabUrl,omitempty"`
	GitSshPrivateKey  string `json:"gitSshPrivateKey,omitempty"`
	GitSshKnownHosts  string `json:"gitSshKnownHosts,omitempty"`
	RepoURL           string `json:"repoUrl,omitempty"`
	RepoBranch        string `json:"repoBranch,omitempty"`
	GitUserName       string `json:"gitUserName,omitempty"`
	GitUserEmail      string `json:"gitUserEmail,omitempty"`
	// VncPreview / BrowserMcp default on when omitted (first-install preview stack).
	VncPreview *bool `json:"vncPreview"`
	BrowserMcp *bool `json:"browserMcp"`
}

// OnboardingBootstrapResult is returned after a successful (idempotent) bootstrap.
type OnboardingBootstrapResult struct {
	AgentIDs   []string `json:"agentIds"`
	WorkflowID string   `json:"workflowId"`
	Published  bool     `json:"published"`
	GroupName  string   `json:"groupName,omitempty"`
}

// OnboardingService bootstraps first-install auth + 综合项目组 + 默认工作流.
type OnboardingService struct {
	Projects    *ProjectService
	Skills      *AgentService
	SharedAgent *SharedAgentService
	WF          *WorkflowService
	Org         *OrgService
}

// NewOnboardingService wires dependencies. org may be nil (agents still saved).
func NewOnboardingService(projects *ProjectService, skills *AgentService, shared *SharedAgentService, wf *WorkflowService, org *OrgService) *OnboardingService {
	return &OnboardingService{Projects: projects, Skills: skills, SharedAgent: shared, WF: wf, Org: org}
}

// Bootstrap writes shared-agent env auth, saves the 综合项目组 agents, and publishes
// 默认工作流. It is idempotent for the fixed names within the default project.
// Cross-project name conflicts are rejected with ErrOnboardingAgentConflict.
// It never starts a Run. Missing apiKey rejects without creating resources.
func (s *OnboardingService) Bootstrap(projectID string, req OnboardingBootstrapRequest) (OnboardingBootstrapResult, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return OnboardingBootstrapResult{}, ErrOnboardingProjectNotFound
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		return OnboardingBootstrapResult{}, ErrOnboardingAPIKeyRequired
	}
	if _, ok := s.Projects.Get(projectID); !ok {
		return OnboardingBootstrapResult{}, ErrOnboardingProjectNotFound
	}
	defID := ""
	if s.Projects != nil {
		defID = strings.TrimSpace(s.Projects.DefaultProjectID())
	}
	if defID == "" {
		defID = models.DefaultProjectID
	}
	if projectID != defID {
		return OnboardingBootstrapResult{}, ErrOnboardingNotDefaultProject
	}

	backend := NormalizeAcpBackend(req.AcpBackend)
	region := strings.TrimSpace(req.Region)

	if err := s.checkOnboardingAgentConflicts(projectID); err != nil {
		return OnboardingBootstrapResult{}, err
	}

	envelope, err := loadFirstInstallWorkflowEnvelope()
	if err != nil {
		return OnboardingBootstrapResult{}, err
	}

	templates := make([]Agent, 0, len(OnboardingAgentNames))
	for _, name := range OnboardingAgentNames {
		tmpl, err := loadFirstInstallAgentTemplate(name)
		if err != nil {
			return OnboardingBootstrapResult{}, err
		}
		tmpl.Name = name
		tmpl.ProjectID = projectID
		tmpl.AcpBackend = backend
		tmpl.Layout.ConfigRoot = DefaultConfigRootForBackend(backend)
		if strings.TrimSpace(tmpl.Layout.WorkspaceDir) == "" {
			tmpl.Layout.WorkspaceDir = DefaultWorkspaceDir
		}
		if tmpl.Env == nil {
			tmpl.Env = map[string]string{}
		}
		tmpl.Env = stripTokenKeysFromEnvMap(tmpl.Env)
		delete(tmpl.Env, runtime.EnvCodeBuddyRegion)
		delete(tmpl.Env, runtime.EnvTraeRegion)
		templates = append(templates, tmpl)
	}

	if err := s.writeProjectAuth(projectID, backend, apiKey, region, req); err != nil {
		return OnboardingBootstrapResult{}, err
	}

	agentIDs := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		if err := s.Skills.Save(tmpl); err != nil {
			return OnboardingBootstrapResult{}, fmt.Errorf("save agent %s: %w", tmpl.Name, err)
		}
		agentIDs = append(agentIDs, tmpl.Name)
	}

	if err := s.ensureFirstInstallOrg(agentIDs); err != nil {
		return OnboardingBootstrapResult{}, err
	}

	applyOnboardingRepo(&envelope.Graph, req.RepoURL, req.RepoBranch)

	wf, err := s.upsertDefaultWorkflow(projectID, envelope)
	if err != nil {
		return OnboardingBootstrapResult{}, err
	}
	published, err := s.WF.Publish(wf.ID)
	if err != nil {
		return OnboardingBootstrapResult{}, fmt.Errorf("publish workflow: %w", err)
	}

	return OnboardingBootstrapResult{
		AgentIDs:   agentIDs,
		WorkflowID: published.ID,
		Published:  published.Status == "published",
		GroupName:  FirstInstallGroupName,
	}, nil
}

func (s *OnboardingService) checkOnboardingAgentConflicts(projectID string) error {
	for _, name := range OnboardingAgentNames {
		existing, ok := s.Skills.Get(name)
		if !ok {
			continue
		}
		owner := strings.TrimSpace(existing.ProjectID)
		if owner != "" && owner != projectID {
			return fmt.Errorf("%w: %s owned by project %s", ErrOnboardingAgentConflict, name, owner)
		}
	}
	return nil
}

func (s *OnboardingService) writeProjectAuth(projectID, backend, apiKey, region string, req OnboardingBootstrapRequest) error {
	if s.SharedAgent == nil {
		return fmt.Errorf("shared agent service unavailable")
	}
	cfg := s.SharedAgent.Get(projectID)
	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	cfg.ProjectID = projectID
	cfg.AcpBackend = backend
	cfg.Env[primaryAuthEnvKey(backend)] = apiKey
	switch backend {
	case AcpBackendCodeBuddy:
		if region == "" {
			region = "public"
		}
		cfg.Env[runtime.EnvCodeBuddyRegion] = region
	case AcpBackendTrae:
		if region == "" {
			region = "intl"
		}
		cfg.Env[runtime.EnvTraeRegion] = region
	}
	cred := strings.TrimSpace(req.GitCredentialType)
	if cred != "" {
		cfg.GitCredentialType = cred
	}
	if v := strings.TrimSpace(req.GitHubToken); v != "" {
		cfg.Env["GITHUB_TOKEN"] = v
	}
	if v := strings.TrimSpace(req.GitLabToken); v != "" {
		cfg.Env["GITLAB_TOKEN"] = v
	}
	if v := strings.TrimSpace(req.GitLabURL); v != "" {
		cfg.Env["GITLAB_URL"] = v
	}
	if err := ValidateAgentSSHMeta(req.GitSshKnownHosts, req.GitSshPrivateKey); err != nil {
		return err
	}
	if v := strings.TrimSpace(req.GitSshPrivateKey); v != "" {
		cfg.GitSshPrivateKey = v
	}
	if v := strings.TrimSpace(req.GitSshKnownHosts); v != "" {
		cfg.GitSshKnownHosts = v
	}
	if v := strings.TrimSpace(req.GitUserName); v != "" {
		cfg.Env["GIT_USER_NAME"] = v
	}
	if v := strings.TrimSpace(req.GitUserEmail); v != "" {
		cfg.Env["GIT_USER_EMAIL"] = v
	}
	if boolOrDefault(req.VncPreview, true) {
		cfg.Env["VNC_PREVIEW"] = "1"
	} else {
		cfg.Env["VNC_PREVIEW"] = "0"
	}
	if boolOrDefault(req.BrowserMcp, true) {
		cfg.Env["BROWSER_MCP"] = "1"
	} else {
		cfg.Env["BROWSER_MCP"] = "0"
	}
	return s.SharedAgent.Save(cfg)
}

func boolOrDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func primaryAuthEnvKey(backend string) string {
	switch NormalizeAcpBackend(backend) {
	case AcpBackendClaudeCode:
		return "APPROVING_CLAUDE_API_KEY"
	case AcpBackendCodeBuddy:
		return "APPROVING_CODEBUDDY_API_KEY"
	case AcpBackendTrae:
		return "APPROVING_TRAE_API_KEY"
	default:
		return "APPROVING_CURSOR_API_KEY"
	}
}

func (s *OnboardingService) ensureFirstInstallOrg(agentNames []string) error {
	if s.Org == nil {
		return nil
	}
	org, err := s.Org.Get()
	if err != nil {
		return err
	}
	gid := FirstInstallGroupID
	found := false
	for _, g := range org.Groups {
		if g.ID == FirstInstallGroupID || (g.Name == FirstInstallGroupName && strings.TrimSpace(g.ParentGroupID) == "") {
			gid = g.ID
			found = true
			break
		}
	}
	if !found {
		org.Groups = append(org.Groups, OrgGroup{ID: FirstInstallGroupID, Name: FirstInstallGroupName})
		gid = FirstInstallGroupID
	}
	if org.Agents == nil {
		org.Agents = map[string]OrgAgentMembership{}
	}
	for _, name := range agentNames {
		org.Agents[name] = OrgAgentMembership{GroupIDs: []string{gid}}
	}
	_, err = s.Org.Put(org, org.Revision)
	return err
}

func (s *OnboardingService) upsertDefaultWorkflow(projectID string, envelope models.ExportEnvelope) (models.WorkflowDef, error) {
	graph := envelope.Graph
	LiftInputVariables(&graph)
	MigrateOutputNodes(&graph)
	MigrateAgentProfileInGraph(&graph)
	if err := graph.Validate(); err != nil {
		return models.WorkflowDef{}, fmt.Errorf("default workflow graph invalid: %w", err)
	}

	var existing *models.WorkflowDef
	for _, wf := range s.WF.List(projectID) {
		if wf.Name == OnboardingWorkflowName {
			full, ok := s.WF.Get(wf.ID)
			if !ok {
				continue
			}
			existing = &full
			break
		}
	}
	desc := strings.TrimSpace(envelope.Description)
	if desc == "" {
		desc = "第一次安装默认工作流。仓库与凭据在运行时 / 共享 Agent 配置中填写。"
	}
	if existing != nil {
		existing.Description = desc
		existing.NeedsRepo = true
		existing.Graph = graph
		if err := s.WF.Save(existing); err != nil {
			return models.WorkflowDef{}, err
		}
		return *existing, nil
	}
	wf := models.WorkflowDef{
		ID:          uuid.NewString(),
		ProjectID:   projectID,
		Name:        OnboardingWorkflowName,
		Description: desc,
		Status:      "draft",
		Version:     1,
		NeedsRepo:   true,
		Graph:       graph,
	}
	if err := s.WF.Save(&wf); err != nil {
		return models.WorkflowDef{}, err
	}
	return wf, nil
}

// applyOnboardingRepo fills the default workflow's `repos` variable from the
// wizard. An empty URL leaves the shipped blank row so the launcher still asks
// for a repo at run start; the name is derived the same way the sandbox does.
func applyOnboardingRepo(graph *models.Graph, url, branch string) {
	url = strings.TrimSpace(url)
	if url == "" || graph == nil {
		return
	}
	name := sandbox.RepoNameFromURL(url)
	if name == "" {
		name = "repo"
	}
	row := map[string]any{"url": url, "name": name, "branch": strings.TrimSpace(branch)}
	for i := range graph.Variables {
		if graph.Variables[i].Name != "repos" {
			continue
		}
		graph.Variables[i].Value = []any{row}
		return
	}
}

// applyOnboardingAgentRegion writes the Studio-managed region env key into an
// Agent env map for backends that require it (CodeBuddy / Trae). Empty region
// falls back to the same defaults as writeProjectAuth / web regionPolicy.
func applyOnboardingAgentRegion(env map[string]string, backend, region string) {
	if env == nil {
		return
	}
	switch NormalizeAcpBackend(backend) {
	case AcpBackendCodeBuddy:
		if region == "" {
			region = "public"
		}
		env[runtime.EnvCodeBuddyRegion] = region
	case AcpBackendTrae:
		if region == "" {
			region = "intl"
		}
		env[runtime.EnvTraeRegion] = region
	}
}
