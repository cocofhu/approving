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
	// OnboardingWorkflowName is the fixed published example workflow.
	OnboardingWorkflowName = "快速上手·轻量"
	// DefaultOnboardingRepos is the well-known public Heroku Node getting-started repo.
	DefaultOnboardingRepos = "demo|https://github.com/heroku/nodejs-getting-started.git|main"
	// DefaultOnboardingFeature is a one-line sample feature for the Heroku demo homepage.
	DefaultOnboardingFeature = "把首页欢迎文案与主按钮文案改得更清晰友好"
)

var (
	// ErrOnboardingAPIKeyRequired is returned when bootstrap is called without an API key.
	ErrOnboardingAPIKeyRequired = errors.New("apiKey is required")
	// ErrOnboardingProjectNotFound is returned when the project id does not exist.
	ErrOnboardingProjectNotFound = errors.New("project not found")
	// ErrOnboardingAgentConflict is returned when a fixed-name agent already belongs to another project.
	ErrOnboardingAgentConflict = errors.New("onboarding agent already bound to another project")
)

// OnboardingBootstrapRequest is the body for POST .../bootstrap-onboarding.
type OnboardingBootstrapRequest struct {
	AcpBackend  string `json:"acpBackend"`
	APIKey      string `json:"apiKey"`
	Region      string `json:"region,omitempty"`
	Repos       string `json:"repos,omitempty"`
	FeatureHint string `json:"featureHint,omitempty"`
}

// OnboardingBootstrapResult is returned after a successful (idempotent) bootstrap.
type OnboardingBootstrapResult struct {
	AgentIDs   []string `json:"agentIds"`
	WorkflowID string   `json:"workflowId"`
	Repos       string   `json:"repos"`
	Feature    string   `json:"feature"`
	Published  bool     `json:"published"`
}

// OnboardingService atomically bootstraps project auth + onboarding agents + light workflow.
type OnboardingService struct {
	Projects *ProjectService
	Skills   *SkillService
	WF       *WorkflowService
}

// NewOnboardingService wires dependencies.
func NewOnboardingService(projects *ProjectService, skills *SkillService, wf *WorkflowService) *OnboardingService {
	return &OnboardingService{Projects: projects, Skills: skills, WF: wf}
}

// Bootstrap writes project sandboxEnv auth, saves onboarding agents from embed
// templates, and publishes the light onboarding workflow. It is idempotent for
// the fixed names within the same project. Cross-project name conflicts are
// rejected with ErrOnboardingAgentConflict (no overwrite of another project's
// agents). It never starts a Run. Missing apiKey rejects without creating
// resources. Templates and the light graph are validated before any auth/agent
// writes to reduce mid-flight partial state.
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

	backend := NormalizeAcpBackend(req.AcpBackend)
	repos := strings.TrimSpace(req.Repos)
	if repos == "" {
		repos = DefaultOnboardingRepos
	}
	feature := strings.TrimSpace(req.FeatureHint)
	if feature == "" {
		feature = DefaultOnboardingFeature
	}

	if err := s.checkOnboardingAgentConflicts(projectID); err != nil {
		return OnboardingBootstrapResult{}, err
	}

	region := strings.TrimSpace(req.Region)

	// Preload templates + validate graph before mutating project auth / agents.
	templates := make([]Agent, 0, len(OnboardingAgentNames))
	for _, name := range OnboardingAgentNames {
		tmpl, err := loadOnboardingAgentTemplate(name)
		if err != nil {
			return OnboardingBootstrapResult{}, err
		}
		tmpl.Name = name
		tmpl.ProjectID = projectID
		tmpl.AcpBackend = backend
		// Persist backend-specific layout so Studio opens clean (no dirty from
		// client-side defaultConfigRoot fill).
		tmpl.Layout.ConfigRoot = DefaultConfigRootForBackend(backend)
		if strings.TrimSpace(tmpl.Layout.WorkspaceDir) == "" {
			tmpl.Layout.WorkspaceDir = DefaultWorkspaceDir
		}
		if tmpl.Env == nil {
			tmpl.Env = map[string]string{}
		}
		tmpl.Env["GIT_REPOS"] = "${vars.repos}"
		// Mirror project auth region into Agent.env so Agent Studio's
		// normalizeDraftRegions does not mark every bootstrap agent dirty.
		applyOnboardingAgentRegion(tmpl.Env, backend, region)
		templates = append(templates, tmpl)
	}
	graph := buildOnboardingLightGraph(repos, feature)
	if err := graph.Validate(); err != nil {
		return OnboardingBootstrapResult{}, fmt.Errorf("onboarding graph invalid: %w", err)
	}

	if err := s.writeProjectAuth(projectID, backend, apiKey, region); err != nil {
		return OnboardingBootstrapResult{}, err
	}

	agentIDs := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		if err := s.Skills.Save(tmpl); err != nil {
			return OnboardingBootstrapResult{}, fmt.Errorf("save agent %s: %w", tmpl.Name, err)
		}
		agentIDs = append(agentIDs, tmpl.Name)
	}

	wf, err := s.upsertLightWorkflow(projectID, repos, feature)
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
		Repos:       repos,
		Feature:    feature,
		Published:  published.Status == "published",
	}, nil
}

// checkOnboardingAgentConflicts refuses to overwrite a fixed-name agent that is
// already bound to a different project. Same-project overwrite (idempotent
// re-bootstrap) and unbound agents (empty projectId) are allowed.
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

func (s *OnboardingService) writeProjectAuth(projectID, backend, apiKey, region string) error {
	p, ok := s.Projects.Get(projectID)
	if !ok {
		return ErrOnboardingProjectNotFound
	}
	byKey := make(map[string]models.EnvEntry, len(p.SandboxEnv))
	order := make([]string, 0, len(p.SandboxEnv)+2)
	for _, e := range p.SandboxEnv {
		k := strings.TrimSpace(e.Key)
		if k == "" {
			continue
		}
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = e
	}
	upsert := func(key, value string, secret bool) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = models.EnvEntry{Key: key, Value: value, Secret: secret || runtime.IsPlatformAuthEnvKey(key)}
	}
	upsert(primaryAuthEnvKey(backend), apiKey, true)
	switch backend {
	case AcpBackendCodeBuddy:
		if region == "" {
			region = "public"
		}
		upsert(runtime.EnvCodeBuddyRegion, region, false)
	case AcpBackendTrae:
		if region == "" {
			region = "intl"
		}
		upsert(runtime.EnvTraeRegion, region, false)
	}
	out := make([]models.EnvEntry, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	_, err := s.Projects.Update(projectID, nil, nil, &out, nil, nil)
	return err
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

// onboardingReposVarValue converts a GIT_REPOS wire literal into the structured
// []any form expected by workflow variables of Type "repos" (and by parseReposVar).
func onboardingReposVarValue(wire string) any {
	specs := sandbox.DecodeRepos(wire)
	if len(specs) == 0 {
		specs = sandbox.DecodeRepos(DefaultOnboardingRepos)
	}
	out := make([]any, 0, len(specs))
	for _, r := range specs {
		m := map[string]any{"name": r.Name, "url": r.URL}
		if r.Branch != "" {
			m["branch"] = r.Branch
		}
		out = append(out, m)
	}
	return out
}

func (s *OnboardingService) upsertLightWorkflow(projectID, repos, feature string) (models.WorkflowDef, error) {
	graph := buildOnboardingLightGraph(repos, feature)
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
	if existing != nil {
		existing.Description = "空项目快速上手轻量示例（clarify→visual→gate→implement→test→review→preview）"
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
		Description: "空项目快速上手轻量示例（clarify→visual→gate→implement→test→review→preview）",
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

// BuildOnboardingLightGraphForTest exposes the light graph builder for unit tests.
func BuildOnboardingLightGraphForTest(repos, feature string) models.Graph {
	return buildOnboardingLightGraph(repos, feature)
}

func buildOnboardingLightGraph(repos, feature string) models.Graph {
	implementPrompt := "轻量链路：用 get_clarified_requirement 读取澄清结论，并结合视觉产物 page.html 与 {{vars.preview_issues}} / {{vars.reason}}（如有）实现改动。" +
		"勿依赖实施计划。在仓库中完成需求后提交推送，并调用 set_implementation_result。"
	visualPrompt := "根据澄清后的需求做一个简洁美观的可视化网页 demo（原型），用 write_artifact 写入 page.html。勿依赖 plan。"
	previewPrompt := "在沙箱内启动 Heroku Node Getting Started 示例（或工作流 repos 指向的公开仓）：PORT=5006，监听 0.0.0.0，调用 set_preview(5006)。"
	reviewPrompt := "轻量评审上游实现：只拦正确性/回归/明显安全问题；文案与风格非阻塞。用 set_review 写入结论。无 plan 时可省略计划覆盖。"

	// Positions: never use (0,0) — the canvas treats that as "invalid" and
	// session-layouts the node to the right of all others (input appeared last).
	const y = 200
	step := 220
	x := func(i int) float64 { return float64(40 + i*step) }

	return models.Graph{
		Variables: []models.Variable{
			{
				Name: "feature", Type: "paragraph", Value: feature,
				Desc: "示例需求", Ask: true, Required: true, Editable: true,
			},
			{
				// Must be Type "repos" (structured [{name,url,branch}]) — not a
				// string wire literal. Wire form does not render the repos editor
				// and previously left GIT_REPOS empty after parseReposVar.
				Name: "repos", Type: "repos", Value: onboardingReposVarValue(repos),
				Desc: "仓库列表(平级,每个 clone 到 /root/workspace/<name>/)", Ask: true, Required: true, Editable: true,
			},
		},
		Nodes: []models.Node{
			{ID: "input", Type: "input", Label: "输入", Position: models.Position{X: x(0), Y: y}, Config: map[string]any{}},
			{
				ID: "clarify", Type: "react", Label: "澄清", Position: models.Position{X: x(1), Y: y},
				Config: map[string]any{
					"skill_profile": "ClarifyAgent",
					"max_rounds":    6,
					"prompt":        "针对以下需求提出澄清问题,直到信息充分,再调用 set_clarified_requirement 写入结构化需求:\n{{vars.feature}}",
				},
			},
			{
				ID: "visual", Type: "visual", Label: "视觉", Position: models.Position{X: x(2), Y: y},
				Config: map[string]any{
					"skill_profile": "VisualAgent",
					"prompt":        visualPrompt,
				},
			},
			{
				ID: "gate", Type: "human_gate", Label: "视觉确认", Position: models.Position{X: x(3), Y: y},
				Config: map[string]any{
					"title":         "确认视觉原型",
					"body_template": "{{nodes.visual.outputs.page}}",
					"output_var":    "action",
					"form":          []any{},
					// goto on actions is the primary canvas/engine route; When
					// edges below remain as fallback for older clients.
					"actions": []any{
						map[string]any{"id": "approve", "label": "批准", "goto": "implement"},
						map[string]any{"id": "revise", "label": "退回修改", "goto": "visual"},
					},
				},
			},
			{
				ID: "implement", Type: "implement", Label: "实现", Position: models.Position{X: x(4), Y: y},
				Config: map[string]any{
					"skill_profile": "ImplementAgent",
					"max_rounds":    3,
					"prompt":        implementPrompt,
					"conditional_prompt": map[string]any{
						"when_var": "reason",
						"text":     "测试/评审/预览未通过：{{vars.reason}}\n按反馈最小修复后重新 PUSH。",
					},
				},
			},
			{
				ID: "test", Type: "test", Label: "测试", Position: models.Position{X: x(5), Y: y},
				Config: map[string]any{
					"skill_profile":    "TestAgent",
					"reason_var":       "reason",
					"repoScope":        "all",
					"block_on_skipped": false,
					"prompt":           "对上游实现执行测试并如实记录结果,用 set_test_result 写入测试总结。无 plan 叶子时可省略 plan_coverage。",
					"exits": map[string]any{
						"pass": map[string]any{"goto": "review"},
						"fail": map[string]any{"goto": "implement"},
					},
				},
			},
			{
				ID: "review", Type: "review", Label: "复审", Position: models.Position{X: x(6), Y: y},
				Config: map[string]any{
					"skill_profile": "ReviewAgent",
					"reason_var":    "reason",
					"prompt":        reviewPrompt,
					"exits": map[string]any{
						"pass": map[string]any{"goto": "preview"},
						"fail": map[string]any{"goto": "implement"},
					},
				},
			},
			{
				ID: "preview", Type: "app_preview", Label: "预览", Position: models.Position{X: x(7), Y: y},
				Config: map[string]any{
					"skill_profile": "PreviewAgent",
					"max_rounds":    3,
					"title":         "应用预览",
					"output_var":    "action",
					"prompt":        previewPrompt,
					"form":          []any{},
					"actions": []any{
						map[string]any{"id": "pass", "label": "通过", "goto": "output"},
						map[string]any{"id": "fail", "label": "退回", "goto": "implement"},
					},
				},
			},
			{ID: "output", Type: "output", Label: "输出", Position: models.Position{X: x(8), Y: y}, Config: map[string]any{}},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "clarify"},
			{ID: "e2", Source: "clarify", Target: "visual"},
			{ID: "e3", Source: "visual", Target: "gate"},
			{ID: "e4", Source: "gate", Target: "implement", When: "action == 'approve'", Kind: models.EdgeSuccess},
			{ID: "e5", Source: "gate", Target: "visual", When: "action == 'revise'", Kind: models.EdgeSuccess},
			{ID: "e6", Source: "implement", Target: "test"},
			{ID: "e7", Source: "test", Target: "review", Kind: models.EdgeSuccess},
			{ID: "e8", Source: "test", Target: "implement", Kind: models.EdgeFailure},
			{ID: "e9", Source: "review", Target: "preview", Kind: models.EdgeSuccess},
			{ID: "e10", Source: "review", Target: "implement", Kind: models.EdgeFailure},
			{ID: "e11", Source: "preview", Target: "output", When: "action == 'pass'", Kind: models.EdgeSuccess},
			{ID: "e12", Source: "preview", Target: "implement", When: "action == 'fail'", Kind: models.EdgeSuccess},
			{ID: "e13", Source: "preview", Target: "implement", Kind: models.EdgeFailure},
		},
	}
}
