package services

import (
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

type fakeAgentLookup map[string]Agent

func (f fakeAgentLookup) Get(name string) (Agent, bool) {
	a, ok := f[name]
	return a, ok
}

func TestValidateSkillProfilesProject(t *testing.T) {
	skills := fakeAgentLookup{
		"ok-agent":     {Name: "ok-agent", ProjectID: "alpha"},
		"other-agent":  {Name: "other-agent", ProjectID: "beta"},
		"unbound-agent": {Name: "unbound-agent", ProjectID: ""},
	}

	t.Run("empty skill_profile skipped", func(t *testing.T) {
		g := models.Graph{Nodes: []models.Node{
			{ID: "n1", Type: "research", Label: "调研", Config: map[string]any{"skill_profile": ""}},
			{ID: "n2", Type: "human_gate", Label: "门禁", Config: map[string]any{}},
		}}
		if err := ValidateSkillProfilesProject(skills, "alpha", g); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("same project ok", func(t *testing.T) {
		g := models.Graph{Nodes: []models.Node{
			{ID: "n1", Type: "implement", Label: "实现", Config: map[string]any{"skill_profile": "ok-agent"}},
			{ID: "n2", Type: "app_preview", Label: "预览", Config: map[string]any{"skill_profile": "ok-agent"}},
		}}
		if err := ValidateSkillProfilesProject(skills, "alpha", g); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("rejects cross project unbound and deleted", func(t *testing.T) {
		g := models.Graph{Nodes: []models.Node{
			{ID: "a", Type: "agent", Label: "执行", Config: map[string]any{"skill_profile": "other-agent"}},
			{ID: "b", Type: "react", Label: "澄清", Config: map[string]any{"skill_profile": "unbound-agent"}},
			{ID: "c", Type: "plan", Label: "计划", Config: map[string]any{"skill_profile": "ghost"}},
		}}
		err := ValidateSkillProfilesProject(skills, "alpha", g)
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		for _, want := range []string{"非本项目", "未绑定", "已删除", "执行 → other-agent", "澄清 → unbound-agent", "计划 → ghost"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("msg %q missing %q", msg, want)
			}
		}
	})

	t.Run("covers all skill_profile node types not only agent", func(t *testing.T) {
		types := []string{"react", "agent", "approve", "plan", "implement", "research", "test", "review", "proposal", "submit_mr", "visual", "app_preview"}
		for _, typ := range types {
			g := models.Graph{Nodes: []models.Node{
				{ID: "n", Type: typ, Label: typ, Config: map[string]any{"skill_profile": "other-agent"}},
			}}
			if err := ValidateSkillProfilesProject(skills, "alpha", g); err == nil {
				t.Fatalf("type %s: expected rejection", typ)
			}
		}
	})
}

func TestWorkflowServiceSavePublishSkillProfileGate(t *testing.T) {
	db := newTestDB(t)
	s := NewWorkflowService(db)
	s.SetSkills(fakeAgentLookup{
		"ok-agent":    {Name: "ok-agent", ProjectID: models.DefaultProjectID},
		"other-agent": {Name: "other-agent", ProjectID: "beta"},
	})

	wf := &models.WorkflowDef{
		ID:        "wf-skill-1",
		ProjectID: models.DefaultProjectID,
		Name:      "skill-gate",
		Status:    "draft",
		Version:   1,
		Graph: models.Graph{Nodes: []models.Node{
			{ID: "in", Type: "input", Label: "输入", Position: models.Position{}, Config: map[string]any{}},
			{ID: "ag", Type: "agent", Label: "执行", Position: models.Position{}, Config: map[string]any{"skill_profile": "other-agent"}},
			{ID: "out", Type: "output", Label: "输出", Position: models.Position{}, Config: map[string]any{}},
		}, Edges: []models.Edge{
			{ID: "e1", Source: "in", Target: "ag"},
			{ID: "e2", Source: "ag", Target: "out"},
		}},
	}
	if err := s.Save(wf); err == nil || !strings.Contains(err.Error(), "非本项目") {
		t.Fatalf("Save want foreign reject, got %v", err)
	}

	wf.Graph.Nodes[1].Config["skill_profile"] = "ok-agent"
	if err := s.Save(wf); err != nil {
		t.Fatalf("Save ok: %v", err)
	}

	// poison then publish
	wf.Graph.Nodes[1].Config["skill_profile"] = "other-agent"
	if err := s.db.Save(wf).Error; err != nil {
		t.Fatalf("direct save: %v", err)
	}
	if _, err := s.Publish(wf.ID); err == nil || !strings.Contains(err.Error(), "非本项目") {
		t.Fatalf("Publish want foreign reject, got %v", err)
	}

	wf.Graph.Nodes[1].Config["skill_profile"] = "ok-agent"
	if err := s.db.Save(wf).Error; err != nil {
		t.Fatalf("fix: %v", err)
	}
	if _, err := s.Publish(wf.ID); err != nil {
		t.Fatalf("Publish ok: %v", err)
	}
}
