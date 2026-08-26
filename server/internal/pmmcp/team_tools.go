package pmmcp

import (
	"strings"

	"github.com/cocofhu/approving/internal/platformmcp"
	"github.com/cocofhu/approving/internal/services"
)

func (h *Host) callTeamTools(sess *Session, skill *services.SkillService, name string, args map[string]any) (any, bool) {
	h.mu.RLock()
	team := h.team
	h.mu.RUnlock()
	if team == nil {
		return map[string]any{"error": "team service unavailable"}, true
	}
	if strings.TrimSpace(sess.ProjectID) == "" || strings.TrimSpace(sess.AgentName) == "" {
		return map[string]any{"error": "session missing project or agent"}, true
	}

	switch name {
	case "pm_list_agent_templates":
		return map[string]any{"items": team.ListTemplates()}, false

	case "pm_create_agent_from_template":
		templateID := strings.TrimSpace(platformmcp.StrArg(args, "templateId"))
		agentName := strings.TrimSpace(platformmcp.StrArg(args, "name"))
		if templateID == "" || agentName == "" {
			return map[string]any{"error": "templateId and name are required"}, true
		}
		leader, ok := skill.Get(sess.AgentName)
		if !ok {
			return map[string]any{"error": "leader agent not found"}, true
		}
		if !services.AgentProjectMatches(leader, sess.ProjectID) {
			return map[string]any{"error": "leader not bound to session project"}, true
		}
		created, err := team.CreateAgentFromTemplate(services.CreateFromTemplateArgs{
			TemplateID:  templateID,
			Name:        agentName,
			ProjectID:   sess.ProjectID,
			AcpBackend:  leader.AcpBackend,
			GitCredType: leader.GitCredentialType,
			MCP:         leader.MCP,
			Env:         leader.Env,
		})
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"ok": true, "agentName": created.Name, "templateId": templateID}, false

	case "pm_set_org_membership":
		agentName := strings.TrimSpace(platformmcp.StrArg(args, "agentName"))
		groupIDs := strSliceArg(args, "groupIds")
		if agentName == "" || len(groupIDs) == 0 {
			return map[string]any{"error": "agentName and groupIds are required"}, true
		}
		ag, ok := skill.Get(agentName)
		if !ok {
			return map[string]any{"error": "agent not found: " + agentName}, true
		}
		if !services.AgentProjectMatches(ag, sess.ProjectID) {
			return map[string]any{"error": "agent not in session project"}, true
		}
		if err := team.SetOrgMembership(services.SetOrgMembershipArgs{
			AgentName: agentName,
			GroupIDs:  groupIDs,
		}); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"ok": true, "agentName": agentName, "groupIds": groupIDs}, false

	case "pm_ensure_child_group":
		gName := strings.TrimSpace(platformmcp.StrArg(args, "name"))
		parentID := strings.TrimSpace(platformmcp.StrArg(args, "parentGroupId"))
		if gName == "" {
			return map[string]any{"error": "name is required"}, true
		}
		h.mu.RLock()
		orgSvc := h.org
		h.mu.RUnlock()
		if orgSvc == nil {
			return map[string]any{"error": "org service unavailable"}, true
		}
		doc, err := orgSvc.Get()
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		if parentID == "" {
			// Prefer a root group that already contains the leader.
			mem := doc.Agents[sess.AgentName]
			for _, gid := range mem.GroupIDs {
				for _, g := range doc.Groups {
					if g.ID == gid && strings.TrimSpace(g.ParentGroupID) == "" {
						parentID = g.ID
						break
					}
				}
				if parentID != "" {
					break
				}
			}
		}
		if parentID == "" {
			return map[string]any{"error": "parentGroupId required (leader has no root group)"}, true
		}
		for _, g := range doc.Groups {
			if g.ParentGroupID == parentID && g.Name == gName {
				return map[string]any{"ok": true, "group": g, "existed": true}, false
			}
		}
		doc.Groups = append(doc.Groups, services.OrgGroup{
			ID: services.NewGroupID(), Name: gName, ParentGroupID: parentID,
		})
		created := doc.Groups[len(doc.Groups)-1]
		if _, err := orgSvc.Put(doc, doc.Revision); err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"ok": true, "group": created, "existed": false}, false
	}
	return map[string]any{"error": "unknown tool: " + name}, true
}

func strSliceArg(args map[string]any, key string) []string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	default:
		return nil
	}
}
