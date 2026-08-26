package services

import (
	"sort"
	"strings"
)

// OrgRelation labels an agent relative to the PM leader in the flat project-member view.
// direct = same project member (not self); indirect is unused (always empty list).
const (
	OrgRelationSelf     = "self"
	OrgRelationDirect   = "direct"
	OrgRelationIndirect = "indirect"
	OrgRelationOther    = "other"
)

// OrgAgentAnnotated is an agent membership plus relation to the Leader.
type OrgAgentAnnotated struct {
	Name     string   `json:"name"`
	GroupIDs []string `json:"groupIds,omitempty"`
	Relation string   `json:"relation"` // self|direct|other
}

// OrgSubtreeNode is one flat project-member node (no nested reporting children).
type OrgSubtreeNode struct {
	Name     string           `json:"name"`
	Relation string           `json:"relation"` // direct for project peers
	Children []OrgSubtreeNode `json:"children,omitempty"`
}

// OrgLeaderView is the read-only org snapshot for pm_get_org.
// Field names are kept for response compatibility; semantics are flat project members.
type OrgLeaderView struct {
	Revision        int                 `json:"revision"`
	Groups          []OrgGroup          `json:"groups"`
	Agents          []OrgAgentAnnotated `json:"agents"`
	Leader          string              `json:"leader"`
	DirectReports   []string            `json:"directReports"`
	IndirectReports []string            `json:"indirectReports"`
	Subtree         []OrgSubtreeNode    `json:"subtree"`
}

// BuildOrgLeaderView builds a flat project-member view for pm_get_org.
// allAgents supplies projectId for scope; when empty, falls back to org.Agents keys + leader.
func BuildOrgLeaderView(org AgentOrg, leader string, allAgents ...[]Agent) OrgLeaderView {
	leader = strings.TrimSpace(leader)
	projectID := ""
	if len(allAgents) > 0 {
		for _, a := range allAgents[0] {
			if strings.TrimSpace(a.Name) == leader {
				projectID = strings.TrimSpace(a.ProjectID)
				break
			}
		}
	}

	projectSet := map[string]struct{}{}
	if projectID != "" && len(allAgents) > 0 {
		for _, a := range allAgents[0] {
			if AgentProjectMatches(a, projectID) {
				projectSet[strings.TrimSpace(a.Name)] = struct{}{}
			}
		}
	}

	names := map[string]struct{}{}
	if len(allAgents) > 0 {
		for _, a := range allAgents[0] {
			n := strings.TrimSpace(a.Name)
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

	sortedNames := make([]string, 0, len(names))
	for n := range names {
		sortedNames = append(sortedNames, n)
	}
	sort.Strings(sortedNames)

	direct := make([]string, 0)
	annotated := make([]OrgAgentAnnotated, 0, len(sortedNames))
	for _, name := range sortedNames {
		m := org.Agents[name]
		rel := OrgRelationOther
		switch {
		case leader != "" && name == leader:
			rel = OrgRelationSelf
		case projectID != "":
			if _, inProject := projectSet[name]; inProject {
				rel = OrgRelationDirect
				if name != leader {
					direct = append(direct, name)
				}
			}
		}
		annotated = append(annotated, OrgAgentAnnotated{
			Name:     name,
			GroupIDs: append([]string(nil), m.GroupIDs...),
			Relation: rel,
		})
	}
	sort.Strings(direct)

	subtree := make([]OrgSubtreeNode, 0, len(direct))
	for _, name := range direct {
		subtree = append(subtree, OrgSubtreeNode{
			Name:     name,
			Relation: OrgRelationDirect,
		})
	}

	return OrgLeaderView{
		Revision:        org.Revision,
		Groups:          append([]OrgGroup(nil), org.Groups...),
		Agents:          annotated,
		Leader:          leader,
		DirectReports:   direct,
		IndirectReports: nil,
		Subtree:         subtree,
	}
}
