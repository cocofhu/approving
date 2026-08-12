package services

import (
	"fmt"
	"strings"
)

// applyDeleteGroup returns a copy of org with groupID removed and cascade rules applied:
// children groups promoted to the deleted group's parent; members lose that group
// (and gain the parent group if the deleted group had a parent).
// Test-only helper (production org mutations go through OrgService HTTP paths).
func applyDeleteGroup(org AgentOrg, groupID string) (AgentOrg, error) {
	groupID = strings.TrimSpace(groupID)
	var deleted *OrgGroup
	for i := range org.Groups {
		if org.Groups[i].ID == groupID {
			deleted = &org.Groups[i]
			break
		}
	}
	if deleted == nil {
		return org, fmt.Errorf("%w: group %q not found", ErrOrgValidation, groupID)
	}
	parentID := deleted.ParentGroupID
	groups := make([]OrgGroup, 0, len(org.Groups)-1)
	for _, g := range org.Groups {
		if g.ID == groupID {
			continue
		}
		if g.ParentGroupID == groupID {
			g.ParentGroupID = parentID
		}
		groups = append(groups, g)
	}
	agents := map[string]OrgAgentMembership{}
	for name, m := range org.Agents {
		gids := make([]string, 0, len(m.GroupIDs))
		had := false
		for _, gid := range m.GroupIDs {
			if gid == groupID {
				had = true
				continue
			}
			gids = append(gids, gid)
		}
		if had && parentID != "" {
			gids = append(gids, parentID)
		}
		m.GroupIDs = uniqueNonEmpty(gids)
		if len(m.GroupIDs) == 0 && m.ParentAgent == "" {
			continue
		}
		agents[name] = m
	}
	org.Groups = groups
	org.Agents = agents
	return org, nil
}

// applyMoveAgent applies sidebar drag move semantics: remove sourceGroupID (if any),
// add targetGroupID (if any). Empty target clears all groupIds (ungrouped drop).
// Test-only helper.
func applyMoveAgent(org AgentOrg, agentName, sourceGroupID, targetGroupID string) (AgentOrg, error) {
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return org, fmt.Errorf("%w: agent name required", ErrOrgValidation)
	}
	if org.Agents == nil {
		org.Agents = map[string]OrgAgentMembership{}
	}
	m := org.Agents[agentName]
	target := strings.TrimSpace(targetGroupID)
	source := strings.TrimSpace(sourceGroupID)

	if target == "" {
		// Drop on ungrouped: clear all memberships.
		m.GroupIDs = nil
	} else {
		gids := make([]string, 0, len(m.GroupIDs)+1)
		for _, gid := range m.GroupIDs {
			if gid == source {
				continue
			}
			gids = append(gids, gid)
		}
		gids = append(gids, target)
		m.GroupIDs = uniqueNonEmpty(gids)
	}
	if len(m.GroupIDs) == 0 && m.ParentAgent == "" {
		delete(org.Agents, agentName)
	} else {
		org.Agents[agentName] = m
	}
	return org, nil
}
