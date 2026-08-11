package services

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setupOrg(t *testing.T) (*SkillService, *OrgService) {
	t.Helper()
	root := t.TempDir()
	skill := NewSkillService(root)
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
	skill := NewSkillService(root)
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
			"alice": {GroupIDs: []string{"g_eng"}, ParentAgent: ""},
			"bob":   {GroupIDs: []string{"g_des", "g_eng"}, ParentAgent: "alice"},
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
	if got.Agents["bob"].ParentAgent != "alice" {
		t.Fatalf("bob parent: %q", got.Agents["bob"].ParentAgent)
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

func TestOrgRejectReportingCycle(t *testing.T) {
	_, orgSvc := setupOrg(t)
	_, err := orgSvc.Put(AgentOrg{
		Agents: map[string]OrgAgentMembership{
			"alice": {ParentAgent: "bob"},
			"bob":   {ParentAgent: "alice"},
		},
	}, 0)
	if !errors.Is(err, ErrOrgValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}

func TestOrgRejectSelfParent(t *testing.T) {
	_, orgSvc := setupOrg(t)
	_, err := orgSvc.Put(AgentOrg{
		Agents: map[string]OrgAgentMembership{
			"alice": {ParentAgent: "alice"},
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
			"bob":   {GroupIDs: []string{"g1"}, ParentAgent: "alice"},
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
	if got.Agents["bob"].ParentAgent != "alice2" {
		t.Fatalf("parent not renamed: %q", got.Agents["bob"].ParentAgent)
	}
}

func TestOrgDeleteCascade(t *testing.T) {
	skill, orgSvc := setupOrg(t)
	if _, err := orgSvc.Put(AgentOrg{
		Agents: map[string]OrgAgentMembership{
			"alice": {ParentAgent: ""},
			"bob":   {ParentAgent: "alice"},
			"carol": {ParentAgent: "bob"},
		},
	}, 0); err != nil {
		t.Fatal(err)
	}
	// alice has no parent; deleting bob should reparent carol -> alice
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
	if got.Agents["carol"].ParentAgent != "alice" {
		t.Fatalf("carol parent want alice, got %q", got.Agents["carol"].ParentAgent)
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
			"alice": {GroupIDs: []string{"child"}},
			"bob":   {GroupIDs: []string{"child", "root"}},
			"carol": {GroupIDs: []string{"leaf"}},
		},
	}
	out, err := applyDeleteGroup(org, "child")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, g := range out.Groups {
		ids[g.ID] = g.ParentGroupID
	}
	if _, ok := ids["child"]; ok {
		t.Fatal("child still present")
	}
	if ids["leaf"] != "root" {
		t.Fatalf("leaf parent want root, got %q", ids["leaf"])
	}
	// alice was only in child → moves to root
	if len(out.Agents["alice"].GroupIDs) != 1 || out.Agents["alice"].GroupIDs[0] != "root" {
		t.Fatalf("alice: %+v", out.Agents["alice"])
	}
	// bob keeps root (child removed, parent added but unique)
	if len(out.Agents["bob"].GroupIDs) != 1 || out.Agents["bob"].GroupIDs[0] != "root" {
		t.Fatalf("bob: %+v", out.Agents["bob"])
	}
}

func TestApplyDeleteRootGroup(t *testing.T) {
	org := AgentOrg{
		Groups: []OrgGroup{
			{ID: "root", Name: "Root"},
			{ID: "child", Name: "Child", ParentGroupID: "root"},
		},
		Agents: map[string]OrgAgentMembership{
			"alice": {GroupIDs: []string{"root"}},
		},
	}
	out, err := applyDeleteGroup(org, "root")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != "child" || out.Groups[0].ParentGroupID != "" {
		t.Fatalf("child should become root: %+v", out.Groups)
	}
	if _, ok := out.Agents["alice"]; ok {
		t.Fatalf("alice should be ungrouped: %+v", out.Agents)
	}
}

func TestApplyMoveAgent(t *testing.T) {
	org := AgentOrg{
		Groups: []OrgGroup{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}},
		Agents: map[string]OrgAgentMembership{
			"alice": {GroupIDs: []string{"a", "c"}},
		},
	}
	out, err := applyMoveAgent(org, "alice", "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	gids := out.Agents["alice"].GroupIDs
	if len(gids) != 2 || gids[0] != "b" || gids[1] != "c" {
		// sorted: b, c
		if !(len(gids) == 2 && ((gids[0] == "b" && gids[1] == "c") || (gids[0] == "c" && gids[1] == "b"))) {
			t.Fatalf("move result: %+v", gids)
		}
	}
	out, err = applyMoveAgent(out, "alice", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.Agents["alice"]; ok {
		t.Fatalf("ungrouped should clear: %+v", out.Agents["alice"])
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

func TestOrgGetPrunesAndReparentsDanglingParent(t *testing.T) {
	skill, orgSvc := setupOrg(t)
	if _, err := orgSvc.Put(AgentOrg{
		Groups: []OrgGroup{{ID: "g1", Name: "G"}},
		Agents: map[string]OrgAgentMembership{
			"alice": {GroupIDs: []string{"g1"}},
			"bob":   {GroupIDs: []string{"g1"}, ParentAgent: "alice"},
			"carol": {GroupIDs: []string{"g1"}, ParentAgent: "bob"},
		},
	}, 0); err != nil {
		t.Fatal(err)
	}
	// Simulate skipped OnDeleteAgent: remove bob's directory only.
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
	if got.Agents["carol"].ParentAgent != "alice" {
		t.Fatalf("carol parent want alice after prune, got %q", got.Agents["carol"].ParentAgent)
	}
	// Repair must persist so a second Get stays clean at the new revision.
	again, err := orgSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if again.Revision != got.Revision {
		t.Fatalf("revision should be stable after repair writeback: %d vs %d", again.Revision, got.Revision)
	}
	if again.Agents["carol"].ParentAgent != "alice" {
		t.Fatalf("persisted parent want alice, got %q", again.Agents["carol"].ParentAgent)
	}
}
