package services_test

import (
	"strings"
	"testing"

	"github.com/cocofhu/grasp/internal/models"
	"github.com/cocofhu/grasp/internal/services"
)

func TestSanitizeOnboardingPrefix(t *testing.T) {
	got, err := services.SanitizeOnboardingPrefix(" 中国 象棋 ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "中国象棋" {
		t.Fatalf("got %q", got)
	}
	_, err = services.SanitizeOnboardingPrefix("   ")
	if err == nil {
		t.Fatal("expected empty name error")
	}
	_, err = services.SanitizeOnboardingPrefix("!!!")
	if err == nil {
		t.Fatal("expected invalid name error")
	}
}

func TestBuildOnboardingNamePlan_defaultAndDerived(t *testing.T) {
	def, err := services.BuildOnboardingNamePlan(models.DefaultProjectID, "默认项目", models.DefaultProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if def.GroupID != services.FirstInstallGroupID || def.GroupName != services.FirstInstallGroupName {
		t.Fatalf("default group = %s/%s", def.GroupID, def.GroupName)
	}
	for _, n := range def.AgentNames {
		if !strings.HasPrefix(n, "综合") {
			t.Fatalf("default agent %q", n)
		}
	}

	plan, err := services.BuildOnboardingNamePlan("proj-abc", "Acme Corp", models.DefaultProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Prefix != "AcmeCorp" {
		t.Fatalf("prefix = %q", plan.Prefix)
	}
	if plan.GroupID != "g_onb_proj-abc" {
		t.Fatalf("groupID = %q", plan.GroupID)
	}
	if plan.GroupName != "Acme Corp项目组" {
		t.Fatalf("groupName = %q", plan.GroupName)
	}
	if plan.NameMap["综合研发工程师"] != "AcmeCorp研发工程师" {
		t.Fatalf("map = %#v", plan.NameMap)
	}
}

func TestRemapOnboardingAgentProfiles(t *testing.T) {
	g := &models.Graph{
		Nodes: []models.Node{
			{ID: "n1", Type: "implement", Config: map[string]any{"agent_profile": "综合研发工程师"}},
			{ID: "n2", Type: "test", Config: map[string]any{"agent_profile": "综合测试工程师"}},
		},
	}
	services.RemapOnboardingAgentProfiles(g, map[string]string{
		"综合研发工程师": "中国象棋研发工程师",
		"综合测试工程师": "中国象棋测试工程师",
	})
	if got, _ := g.Nodes[0].Config["agent_profile"].(string); got != "中国象棋研发工程师" {
		t.Fatalf("n1 = %q", got)
	}
	if got, _ := g.Nodes[1].Config["agent_profile"].(string); got != "中国象棋测试工程师" {
		t.Fatalf("n2 = %q", got)
	}
}
