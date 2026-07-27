package services

import (
	"sort"
	"strings"
)

// OrgRelation labels an agent relative to a Leader in the reporting tree.
const (
	OrgRelationSelf     = "self"
	OrgRelationDirect   = "direct"
	OrgRelationIndirect = "indirect"
	OrgRelationOther    = "other"
)

// OrgAgentAnnotated is an agent membership plus relation to the Leader.
type OrgAgentAnnotated struct {
	Name        string   `json:"name"`
	GroupIDs    []string `json:"groupIds,omitempty"`
	ParentAgent string   `json:"parentAgent,omitempty"`
	Relation    string   `json:"relation"` // self|direct|indirect|other
}

// OrgSubtreeNode is one node in the Leader-rooted reporting subtree.
type OrgSubtreeNode struct {
	Name     string           `json:"name"`
	Relation string           `json:"relation"` // direct|indirect (root omitted; children only)
	Children []OrgSubtreeNode `json:"children,omitempty"`
}

// OrgLeaderView is the read-only org snapshot for pm_get_org.
type OrgLeaderView struct {
	Revision       int                 `json:"revision"`
	Groups         []OrgGroup          `json:"groups"`
	Agents         []OrgAgentAnnotated `json:"agents"`
	Leader         string              `json:"leader"`
	DirectReports  []string            `json:"directReports"`
	IndirectReports []string           `json:"indirectReports"`
	Subtree        []OrgSubtreeNode    `json:"subtree"`
}

// BuildOrgLeaderView annotates agents relative to leader and builds the
// reporting subtree. allAgentNames should be the full on-disk agent set
// (e.g. SkillService.List); when empty, falls back to org.Agents keys + leader.
// Cycles / dangling parents must already be pruned by Get(); this helper still
// guards BFS with a visited set so a corrupt in-memory map cannot expand access.
func BuildOrgLeaderView(org AgentOrg, leader string, allAgentNames ...[]string) OrgLeaderView {
	leader = strings.TrimSpace(leader)
	children := childrenByParent(org.Agents)
	direct, indirect := reportingClosure(leader, children)

	directSet := map[string]struct{}{}
	for _, n := range direct {
		directSet[n] = struct{}{}
	}
	indirectSet := map[string]struct{}{}
	for _, n := range indirect {
		indirectSet[n] = struct{}{}
	}

	names := map[string]struct{}{}
	if len(allAgentNames) > 0 {
		for _, n := range allAgentNames[0] {
			n = strings.TrimSpace(n)
			if n != "" {
				names[n] = struct{}{}
			}
		}
	}
	for name := range org.Agents {
		names[name] = struct{}{}
	}
	if leader != "" {
		names[leader] = struct{}{}
	}
	for _, m := range org.Agents {
		if p := strings.TrimSpace(m.ParentAgent); p != "" {
			names[p] = struct{}{}
		}
	}

	sortedNames := make([]string, 0, len(names))
	for n := range names {
		sortedNames = append(sortedNames, n)
	}
	sort.Strings(sortedNames)

	annotated := make([]OrgAgentAnnotated, 0, len(sortedNames))
	for _, name := range sortedNames {
		m := org.Agents[name]
		rel := OrgRelationOther
		switch {
		case leader != "" && name == leader:
			rel = OrgRelationSelf
		default:
			if _, ok := directSet[name]; ok {
				rel = OrgRelationDirect
			} else if _, ok := indirectSet[name]; ok {
				rel = OrgRelationIndirect
			}
		}
		annotated = append(annotated, OrgAgentAnnotated{
			Name:        name,
			GroupIDs:    append([]string(nil), m.GroupIDs...),
			ParentAgent: m.ParentAgent,
			Relation:    rel,
		})
	}

	subtree := buildSubtree(leader, children, directSet, indirectSet)
	return OrgLeaderView{
		Revision:        org.Revision,
		Groups:          append([]OrgGroup(nil), org.Groups...),
		Agents:          annotated,
		Leader:          leader,
		DirectReports:   direct,
		IndirectReports: indirect,
		Subtree:         subtree,
	}
}

// IsInReportingClosure reports whether target is leader itself or a descendant
// under parentAgent edges (direct or indirect).
func IsInReportingClosure(org AgentOrg, leader, target string) bool {
	leader = strings.TrimSpace(leader)
	target = strings.TrimSpace(target)
	if leader == "" || target == "" {
		return false
	}
	if leader == target {
		return true
	}
	children := childrenByParent(org.Agents)
	direct, indirect := reportingClosure(leader, children)
	for _, n := range direct {
		if n == target {
			return true
		}
	}
	for _, n := range indirect {
		if n == target {
			return true
		}
	}
	return false
}

func childrenByParent(agents map[string]OrgAgentMembership) map[string][]string {
	out := map[string][]string{}
	for name, m := range agents {
		parent := strings.TrimSpace(m.ParentAgent)
		if parent == "" {
			continue
		}
		out[parent] = append(out[parent], name)
	}
	for p := range out {
		sort.Strings(out[p])
	}
	return out
}

// reportingClosure returns sorted direct and indirect descendants of leader.
// Visited guards against cycles so we never traverse infinitely or grant
// extra agents outside the reachable tree from leader.
func reportingClosure(leader string, children map[string][]string) (direct, indirect []string) {
	leader = strings.TrimSpace(leader)
	if leader == "" {
		return nil, nil
	}
	visited := map[string]struct{}{leader: {}}
	var queue []string
	for _, c := range children[leader] {
		if _, seen := visited[c]; seen {
			continue
		}
		visited[c] = struct{}{}
		direct = append(direct, c)
		queue = append(queue, c)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range children[cur] {
			if _, seen := visited[c]; seen {
				continue
			}
			visited[c] = struct{}{}
			indirect = append(indirect, c)
			queue = append(queue, c)
		}
	}
	sort.Strings(direct)
	sort.Strings(indirect)
	return direct, indirect
}

func buildSubtree(leader string, children map[string][]string, direct, indirect map[string]struct{}) []OrgSubtreeNode {
	leader = strings.TrimSpace(leader)
	if leader == "" {
		return nil
	}
	var walk func(parent string, depth int, path map[string]struct{}) []OrgSubtreeNode
	walk = func(parent string, depth int, path map[string]struct{}) []OrgSubtreeNode {
		kids := children[parent]
		if len(kids) == 0 {
			return nil
		}
		out := make([]OrgSubtreeNode, 0, len(kids))
		for _, name := range kids {
			if _, inPath := path[name]; inPath {
				continue // cycle guard
			}
			rel := OrgRelationIndirect
			if depth == 0 {
				if _, ok := direct[name]; ok {
					rel = OrgRelationDirect
				}
			} else if _, ok := indirect[name]; ok {
				rel = OrgRelationIndirect
			} else if _, ok := direct[name]; ok {
				rel = OrgRelationDirect
			}
			nextPath := map[string]struct{}{}
			for k := range path {
				nextPath[k] = struct{}{}
			}
			nextPath[name] = struct{}{}
			node := OrgSubtreeNode{
				Name:     name,
				Relation: rel,
				Children: walk(name, depth+1, nextPath),
			}
			out = append(out, node)
		}
		return out
	}
	return walk(leader, 0, map[string]struct{}{leader: {}})
}
