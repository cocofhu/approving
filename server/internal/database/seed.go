package database

import (
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
)

// Demo seed content (NOT platform config): a sample GitLab project the example
// workflow points at. Repo/integration credentials are NOT stored here — they
// come from the Agent's meta env (e.g. GITLAB_TOKEN) and the workflow's `repos`
// global variable, injected per run into the kind-agnostic sandbox. The clone
// list reaches the sandbox as GIT_REPOS, wired the same reference way as
// credentials — "GIT_REPOS": "${vars.repos}" in the Agent meta env. The
// platform does no fallback injection: unwired means nothing is cloned.
const (
	seedRepoURL = "https://git.example.com/example/test.git" // clone URL (placeholder)
)

// Seed inserts a sample, runnable workflow on first boot. The platform does not
// depend on GitLab: an empty `repos` yields a pure artifact flow, and any git
// push/MR is driven by credentials configured in the Agent meta (env).
func Seed(db *gorm.DB) error {
	var count int64
	db.Model(&models.WorkflowDef{}).Count(&count)
	if count > 0 {
		return nil
	}
	now := time.Now()

	wf := models.WorkflowDef{
		ID: "gitlab-feature", ProjectID: models.DefaultProjectID,
		Name: "gitlab-feature", Status: "published", Version: 1, NeedsRepo: true,
		Description: "在真实 Git 项目上:Agent 实现功能+测试 → 新分支 push+建 MR → 人工确认合并",
		UpdatedAt:   now, Graph: gitlabFeatureGraph(seedRepoURL),
	}
	if err := db.Create(&wf).Error; err != nil {
		return err
	}
	if err := db.Create(&models.WorkflowVersion{
		WorkflowID: wf.ID, Version: wf.Version, Graph: wf.Graph, PublishedAt: now,
	}).Error; err != nil {
		return err
	}
	return nil
}

func gitlabFeatureGraph(repoURL string) models.Graph {
	return models.Graph{
		Variables: []models.Variable{
			{Name: "feature", Type: "paragraph", Desc: "需求描述", Ask: true, Required: true, Editable: true},
			{Name: "repos", Type: "repos", Desc: "仓库列表(平级,每个 clone 到 /root/workspace/<name>/;留空则纯产物流)", Value: []any{
				map[string]any{"name": "test", "url": repoURL},
			}, Ask: true, Editable: true},
		},
		Nodes: []models.Node{
			{ID: "input", Type: "input", Label: "输入", Position: models.Position{X: 0, Y: 200},
				Config: map[string]any{}},
			{ID: "implement", Type: "agent", Label: "实现 Agent", Position: models.Position{X: 300, Y: 200}, Checkpoint: true,
				Config: map[string]any{
					"skill_profile": "go-backend",
					"detect_push":   true,
					"create_mr":     true,
					"produces":      "changes_summary.md",
					"prompt": "在已 clone 的 Go 项目(位于 `/root/workspace/test/`)实现以下需求并配套测试:\n{{vars.feature}}\n\n" +
						"要求:先 `cd /root/workspace/test`;本地运行 `go test ./... -race -count=1` 必须通过;创建新分支 feature/approving-<时间戳>,提交并 push;" +
						"不要修改 .github/workflows/.gitlab-ci.yml/Dockerfile。最后把改动摘要写入 changes_summary.md。",
				}},
			{ID: "done", Type: "human_gate", Label: "合并确认", Position: models.Position{X: 620, Y: 200},
				Config: map[string]any{
					// Primary whitelist: human-editable produce must be in body_template.
					"title":         "确认合并 MR",
					"body_template": "{{artifact(\"changes_summary.md\")}}\n\nMR: {{nodes.implement.outputs.mr_url}}",
					"actions":       []any{map[string]any{"id": "merge", "label": "合并"}, map[string]any{"id": "close", "label": "关闭"}},
				}},
			{ID: "output", Type: "output", Label: "输出", Position: models.Position{X: 940, Y: 200},
				Config: map[string]any{"result": "流水线完成。\nMR: {{nodes.implement.outputs.mr_url}}"}},
		},
		Edges: []models.Edge{
			{ID: "e1", Source: "input", Target: "implement"},
			{ID: "e2", Source: "implement", Target: "done"},
			{ID: "e3", Source: "done", Target: "output"},
		},
	}
}
