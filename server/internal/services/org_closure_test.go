package services

import (
	"testing"
)

func TestBuildOrgLeaderViewRelations(t *testing.T) {
	org := AgentOrg{
		Revision: 3,
		Groups:   []OrgGroup{{ID: "g1", Name: "Eng"}},
		Agents: map[string]OrgAgentMembership{
			"leader": {GroupIDs: []string{"g1"}},
			"alice":  {GroupIDs: []string{"g1"}, ParentAgent: "leader"},
			"bob":    {ParentAgent: "alice"},
			"carol":  {ParentAgent: "bob"},
			"zoe":    {GroupIDs: []string{"g1"}}, // peer / other
		},
	}
	view := BuildOrgLeaderView(org, "leader")
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
		"bob":    OrgRelationIndirect,
		"carol":  OrgRelationIndirect,
		"zoe":    OrgRelationOther,
	}
	for name, w := range want {
		if rel[name] != w {
			t.Fatalf("%s relation=%q want %q (all=%v)", name, rel[name], w, rel)
		}
	}
	if len(view.DirectReports) != 1 || view.DirectReports[0] != "alice" {
		t.Fatalf("direct=%v", view.DirectReports)
	}
	if len(view.IndirectReports) != 2 || view.IndirectReports[0] != "bob" || view.IndirectReports[1] != "carol" {
		t.Fatalf("indirect=%v", view.IndirectReports)
	}
	if len(view.Subtree) != 1 || view.Subtree[0].Name != "alice" || view.Subtree[0].Relation != OrgRelationDirect {
		t.Fatalf("subtree=%+v", view.Subtree)
	}
	if len(view.Subtree[0].Children) != 1 || view.Subtree[0].Children[0].Name != "bob" {
		t.Fatalf("alice children=%+v", view.Subtree[0].Children)
	}
}

func TestIsInReportingClosure(t *testing.T) {
	org := AgentOrg{
		Agents: map[string]OrgAgentMembership{
			"pm":    {},
			"alice": {ParentAgent: "pm"},
			"bob":   {ParentAgent: "alice"},
			"other": {},
		},
	}
	cases := []struct {
		target string
		want   bool
	}{
		{"pm", true},
		{"alice", true},
		{"bob", true},
		{"other", false},
		{"ghost", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsInReportingClosure(org, "pm", tc.target); got != tc.want {
			t.Fatalf("target %q: got %v want %v", tc.target, got, tc.want)
		}
	}
	if IsInReportingClosure(org, "", "alice") {
		t.Fatal("empty leader must deny")
	}
}

func TestReportingClosureIgnoresCycle(t *testing.T) {
	// Corrupt in-memory cycle must not hang or grant unrelated agents.
	org := AgentOrg{
		Agents: map[string]OrgAgentMembership{
			"a": {ParentAgent: "b"},
			"b": {ParentAgent: "a"},
			"c": {ParentAgent: "a"},
		},
	}
	view := BuildOrgLeaderView(org, "a")
	// From a: child is whoever lists parent a → c (and possibly b if parent is a — here b's parent is a? No b's parent is a in this map? Wait: a->b, b->a, c->a
	// children of a: those with ParentAgent==a → b? No: a has parent b, b has parent a, c has parent a.
	// children[a] = [b? no - b's parent is a, yes b; and c] → b, c
	// From a: direct b,c; then from b: child a (cycle, visited) → no new; from c: none.
	if !containsStr(view.DirectReports, "b") || !containsStr(view.DirectReports, "c") {
		t.Fatalf("direct=%v", view.DirectReports)
	}
	if len(view.IndirectReports) != 0 {
		t.Fatalf("indirect should be empty under cycle guard: %v", view.IndirectReports)
	}
	if IsInReportingClosure(org, "a", "ghost") {
		t.Fatal("must not grant unrelated")
	}
}

func TestReportingClosureDanglingParentDoesNotWiden(t *testing.T) {
	org := AgentOrg{
		Agents: map[string]OrgAgentMembership{
			"leader": {},
			"alice":  {ParentAgent: "leader"},
			// dave points at missing parent — not reachable from leader
			"dave": {ParentAgent: "missing"},
		},
	}
	if IsInReportingClosure(org, "leader", "dave") {
		t.Fatal("dangling parent agent must not enter leader closure")
	}
	view := BuildOrgLeaderView(org, "leader")
	for _, a := range view.Agents {
		if a.Name == "dave" && a.Relation != OrgRelationOther {
			t.Fatalf("dave should be other, got %q", a.Relation)
		}
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
