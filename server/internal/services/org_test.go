package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupOrg(t *testing.T) (*AgentService, *OrgService) {
	t.Helper()
	root := t.TempDir()
	skill := NewAgentService(root)
	for _, name := range []string{"alice", "bob", "carol", "dave"} {
		if err := skill.Save(Agent{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	return skill, NewOrgService(root, skill)
}

func TestNewGroupID(t *testing.T) {
	id := NewGroupID()
	if id == "" || id == NewGroupID() {
		t.Fatalf("expected unique group id, got %q", id)
	}
}

func TestOrgLoadCorruptFile(t *testing.T) {
	root := t.TempDir()
	skill := NewAgentService(root)
	orgSvc := NewOrgService(root, skill)
	if err := os.WriteFile(filepath.Join(root, orgFileName), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := orgSvc.Get(); err == nil {
		t.Fatal("expected parse error for corrupt org file")
	}
}

func TestOrgGetEmpty(t *testing.T) {
	_, orgSvc := setupOrg(t)
	org, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if org.Revision != 0 || len(org.Groups) != 0 || len(org.Agents) != 0 {
		t.Fatalf("expected empty org, got %+v", org)
	}
}

func TestOrgPutAndGet(t *testing.T) {
	_, orgSvc := setupOrg(t)
	g1 := OrgGroup{ID: "g_eng", Name: "工程部"}
	g2 := OrgGroup{ID: "g_des", Name: "设计组", ParentGroupID: "g_eng"}
	put, err := orgSvc.Put(AgentOrg{
		Groups: []OrgGroup{g1, g2},
		Agents: map[string]OrgAgentMembership{
			"alice": {GroupIDs: []string{"g_eng"}},
			"bob":   {GroupIDs: []string{"g_des", "g_eng"}},
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if put.Revision != 1 {
		t.Fatalf("revision: %d", put.Revision)
	}
	got, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 1 || len(got.Groups) != 2 {
		t.Fatalf("got %+v", got)
	}
	if len(got.Agents["bob"].GroupIDs) != 2 {
		t.Fatalf("bob groups: %+v", got.Agents["bob"])
	}
}

func TestOrgRevisionConflict(t *testing.T) {
	_, orgSvc := setupOrg(t)
	if _, err := orgSvc.Put(AgentOrg{Groups: []OrgGroup{{ID: "g1", Name: "A"}}}, 0); err != nil {
		t.Fatal(err)
	}
	_, err := orgSvc.Put(AgentOrg{Groups: []OrgGroup{{ID: "g1", Name: "B"}}}, 0)
	if !errors.Is(err, ErrOrgConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestOrgRejectGroupCycle(t *testing.T) {
	_, orgSvc := setupOrg(t)
	_, err := orgSvc.Put(AgentOrg{
		Groups: []OrgGroup{
			{ID: "a", Name: "A", ParentGroupID: "b"},
			{ID: "b", Name: "B", ParentGroupID: "a"},
		},
	}, 0)
	if !errors.Is(err, ErrOrgValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}

func TestOrgRenameCascade(t *testing.T) {
	skill, orgSvc := setupOrg(t)
	if _, err := orgSvc.Put(AgentOrg{
		Groups: []OrgGroup{{ID: "g1", Name: "G"}},
		Agents: map[string]OrgAgentMembership{
			"alice": {GroupIDs: []string{"g1"}},
			"bob":   {GroupIDs: []string{"g1"}},
		},
	}, 0); err != nil {
		t.Fatal(err)
	}
	if err := skill.Rename("alice", "alice2"); err != nil {
		t.Fatal(err)
	}
	if err := orgSvc.OnRenameAgent("alice", "alice2"); err != nil {
		t.Fatal(err)
	}
	got, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Agents["alice"]; ok {
		t.Fatal("old name still present")
	}
	if got.Agents["alice2"].GroupIDs[0] != "g1" {
		t.Fatalf("membership not moved: %+v", got.Agents["alice2"])
	}
}

func TestOrgDeleteRemovesMembership(t *testing.T) {
	skill, orgSvc := setupOrg(t)
	if _, err := orgSvc.Put(AgentOrg{
		Groups: []OrgGroup{{ID: "g1", Name: "G"}},
		Agents: map[string]OrgAgentMembership{
			"alice": {GroupIDs: []string{"g1"}},
			"bob":   {GroupIDs: []string{"g1"}},
			"carol": {GroupIDs: []string{"g1"}},
		},
	}, 0); err != nil {
		t.Fatal(err)
	}
	if err := skill.Delete("bob"); err != nil {
		t.Fatal(err)
	}
	if err := orgSvc.OnDeleteAgent("bob"); err != nil {
		t.Fatal(err)
	}
	got, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Agents["bob"]; ok {
		t.Fatal("bob still in org")
	}
	if got.Agents["carol"].GroupIDs[0] != "g1" {
		t.Fatalf("carol membership: %+v", got.Agents["carol"])
	}
}

func TestApplyDeleteGroup(t *testing.T) {
	org := AgentOrg{
		Groups: []OrgGroup{
			{ID: "root", Name: "Root"},
			{ID: "child", Name: "Child", ParentGroupID: "root"},
			{ID: "leaf", Name: "Leaf", ParentGroupID: "child"},
		},
		Agents: map[string]OrgAgentMembership{
			"alice": {GroupIDs: []string{"leaf"}},
			"bob":   {GroupIDs: []string{"child"}},
		},
	}
	out, err := applyDeleteGroup(org, "child")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Groups) != 2 {
		t.Fatalf("groups=%d", len(out.Groups))
	}
	if out.Agents["bob"].GroupIDs[0] != "root" {
		t.Fatalf("bob promoted to root: %+v", out.Agents["bob"])
	}
	if out.Agents["alice"].GroupIDs[0] != "leaf" {
		t.Fatalf("alice still in leaf: %+v", out.Agents["alice"])
	}
}

func TestOrgDoesNotAffectSkillList(t *testing.T) {
	skill, orgSvc := setupOrg(t)
	if _, err := orgSvc.Put(AgentOrg{Groups: []OrgGroup{{ID: "g1", Name: "G"}}}, 0); err != nil {
		t.Fatal(err)
	}
	list := skill.List()
	for _, a := range list {
		if a.Name == "_org.json" || a.Name == orgFileName {
			t.Fatal("org file leaked into agent list")
		}
	}
	if len(list) != 4 {
		t.Fatalf("list len %d", len(list))
	}
}

func TestOrgGetPrunesMissingAgent(t *testing.T) {
	skill, orgSvc := setupOrg(t)
	if _, err := orgSvc.Put(AgentOrg{
		Groups: []OrgGroup{{ID: "g1", Name: "G"}},
		Agents: map[string]OrgAgentMembership{
			"alice": {GroupIDs: []string{"g1"}},
			"bob":   {GroupIDs: []string{"g1"}},
			"carol": {GroupIDs: []string{"g1"}},
		},
	}, 0); err != nil {
		t.Fatal(err)
	}
	if err := skill.Delete("bob"); err != nil {
		t.Fatal(err)
	}
	got, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Agents["bob"]; ok {
		t.Fatal("bob membership should be pruned")
	}
	if got.Agents["carol"].GroupIDs[0] != "g1" {
		t.Fatalf("carol membership: %+v", got.Agents["carol"])
	}
	again, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if again.Revision != got.Revision {
		t.Fatalf("revision should be stable after repair writeback: %d vs %d", again.Revision, got.Revision)
	}
}

func TestOrgLegacyParentAgentStrippedOnGet(t *testing.T) {
	root := t.TempDir()
	skill := NewAgentService(root)
	for _, name := range []string{"pm", "eng1", "eng2", "eng3"} {
		if err := skill.Save(Agent{Name: name, ProjectID: "proj-1"}); err != nil {
			t.Fatal(err)
		}
	}
	orgSvc := NewOrgService(root, skill)
	legacy := `{
  "revision": 2,
  "groups": [{"id": "g1", "name": "Pipeline"}],
  "agents": {
    "pm": {"groupIds": ["g1"]},
    "eng1": {"groupIds": ["g1"], "parentAgent": "pm"},
    "eng2": {"groupIds": ["g1"], "parentAgent": "eng1"},
    "eng3": {"groupIds": ["g1"], "parentAgent": "eng2"}
  }
}`
	if err := os.WriteFile(filepath.Join(root, orgFileName), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 3 {
		t.Fatalf("revision want 3 after strip writeback, got %d", got.Revision)
	}
	raw, err := os.ReadFile(filepath.Join(root, orgFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "parentAgent") {
		t.Fatalf("persisted file still has parentAgent: %s", raw)
	}
	again, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if again.Revision != got.Revision {
		t.Fatalf("second get should be idempotent: %d vs %d", again.Revision, got.Revision)
	}
}
