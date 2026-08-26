package services

import (
	"testing"
)

func TestBuildOrgLeaderViewFlatProjectMembers(t *testing.T) {
	projectID := "proj-1"
	org := AgentOrg{
		Revision: 3,
		Groups:   []OrgGroup{{ID: "g1", Name: "Eng"}},
		Agents: map[string]OrgAgentMembership{
			"leader": {GroupIDs: []string{"g1"}},
			"alice":  {GroupIDs: []string{"g1"}},
			"bob":    {},
			"zoe":    {GroupIDs: []string{"g1"}},
		},
	}
	allAgents := []Agent{
		{Name: "leader", ProjectID: projectID},
		{Name: "alice", ProjectID: projectID},
		{Name: "bob", ProjectID: projectID},
		{Name: "zoe", ProjectID: "other"},
	}
	view := BuildOrgLeaderView(org, "leader", allAgents)
	if view.Leader != "leader" || view.Revision != 3 {
		t.Fatalf("meta: %+v", view)
	}
	rel := map[string]string{}
	for _, a := range view.Agents {
		rel[a.Name] = a.Relation
	}
	want := map[string]string{
		"leader": OrgRelationSelf,
		"alice":  OrgRelationDirect,
		"bob":    OrgRelationDirect,
		"zoe":    OrgRelationOther,
	}
	for name, w := range want {
		if rel[name] != w {
			t.Fatalf("%s relation=%q want %q (all=%v)", name, rel[name], w, rel)
		}
	}
	if len(view.DirectReports) != 2 {
		t.Fatalf("direct=%v want alice+bob", view.DirectReports)
	}
	if len(view.IndirectReports) != 0 {
		t.Fatalf("indirect should be empty: %v", view.IndirectReports)
	}
	if len(view.Subtree) != 2 {
		t.Fatalf("subtree=%+v", view.Subtree)
	}
	for _, n := range view.Subtree {
		if n.Relation != OrgRelationDirect || len(n.Children) != 0 {
			t.Fatalf("flat subtree node: %+v", n)
		}
	}
}

func TestBuildOrgLeaderViewNoProjectBinding(t *testing.T) {
	org := AgentOrg{
		Agents: map[string]OrgAgentMembership{
			"leader": {},
			"alice":  {},
		},
	}
	allAgents := []Agent{
		{Name: "leader", ProjectID: ""},
		{Name: "alice", ProjectID: "p1"},
	}
	view := BuildOrgLeaderView(org, "leader", allAgents)
	for _, a := range view.Agents {
		if a.Name == "leader" && a.Relation != OrgRelationSelf {
			t.Fatalf("leader rel=%q", a.Relation)
		}
		if a.Name == "alice" && a.Relation != OrgRelationOther {
			t.Fatalf("unbound leader: alice should be other, got %q", a.Relation)
		}
	}
	if len(view.DirectReports) != 0 {
		t.Fatalf("direct=%v", view.DirectReports)
	}
}
