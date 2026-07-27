package pmmcp

import (
	"errors"
	"strings"

	"github.com/cocofhu/approving/internal/platformmcp"
	"github.com/cocofhu/approving/internal/services"
)

func (h *Host) callAgentFS(projectID, token, name string, args map[string]any) (any, bool) {
	sess, ok := h.SessionFor(projectID, token)
	if !ok {
		return map[string]any{"error": "unauthorized"}, true
	}
	h.mu.RLock()
	org := h.org
	skill := h.skill
	h.mu.RUnlock()
	if org == nil || skill == nil {
		return map[string]any{"error": "org/skill service unavailable"}, true
	}

	switch name {
	case "pm_get_org":
		doc, err := org.Get()
		if err != nil {
			return map[string]any{"error": "failed to load org: " + err.Error()}, true
		}
		allNames := make([]string, 0)
		for _, a := range skill.List() {
			allNames = append(allNames, a.Name)
		}
		view := services.BuildOrgLeaderView(doc, sess.AgentName, allNames)
		return view, false

	case "pm_fs_list", "pm_fs_read", "pm_fs_write", "pm_fs_delete", "pm_fs_mkdir", "pm_fs_rename":
		agentName := strings.TrimSpace(platformmcp.StrArg(args, "agentName"))
		if agentName == "" {
			return map[string]any{"error": "agentName is required"}, true
		}
		if errMsg, deny := h.authorizeAgentFSTarget(sess, skill, org, agentName); deny {
			return map[string]any{"error": errMsg}, true
		}
		path := platformmcp.StrArg(args, "path")
		switch name {
		case "pm_fs_list":
			entries, err := skill.ListWorkspace(agentName, path)
			if err != nil {
				return map[string]any{"error": workspaceFSError(err)}, true
			}
			return map[string]any{"agentName": agentName, "path": path, "entries": entries}, false
		case "pm_fs_read":
			content, err := skill.ReadWorkspaceFile(agentName, path)
			if err != nil {
				return map[string]any{"error": workspaceFSError(err)}, true
			}
			return map[string]any{"agentName": agentName, "path": path, "content": content}, false
		case "pm_fs_write":
			content, _ := args["content"].(string)
			if _, has := args["content"]; !has {
				return map[string]any{"error": "content is required"}, true
			}
			if err := skill.WriteWorkspaceFile(agentName, path, content); err != nil {
				return map[string]any{"error": workspaceFSError(err)}, true
			}
			return map[string]any{
				"ok":        true,
				"agentName": agentName,
				"path":      path,
				"bytes":     len(content),
				"note":      "persisted to host workspace; refresh or reopen Agent Studio to see changes; unsaved Studio drafts may overwrite on Save",
			}, false
		case "pm_fs_delete":
			if err := skill.DeleteWorkspacePath(agentName, path); err != nil {
				return map[string]any{"error": workspaceFSError(err)}, true
			}
			return map[string]any{"ok": true, "agentName": agentName, "path": path}, false
		case "pm_fs_mkdir":
			if err := skill.MkdirWorkspace(agentName, path); err != nil {
				return map[string]any{"error": workspaceFSError(err)}, true
			}
			return map[string]any{"ok": true, "agentName": agentName, "path": path}, false
		case "pm_fs_rename":
			toPath := platformmcp.StrArg(args, "toPath")
			if strings.TrimSpace(toPath) == "" {
				return map[string]any{"error": "toPath is required"}, true
			}
			if err := skill.RenameWorkspace(agentName, path, toPath); err != nil {
				return map[string]any{"error": workspaceFSError(err)}, true
			}
			return map[string]any{"ok": true, "agentName": agentName, "path": path, "toPath": toPath}, false
		}
	}
	return map[string]any{"error": "unknown tool: " + name}, true
}

// authorizeAgentFSTarget enforces reporting closure (incl. self) + same home project.
func (h *Host) authorizeAgentFSTarget(sess *Session, skill *services.SkillService, org *services.OrgService, target string) (errMsg string, deny bool) {
	leader := strings.TrimSpace(sess.AgentName)
	if leader == "" {
		return "leader agent missing from session", true
	}
	ag, ok := skill.Get(target)
	if !ok {
		return "agent not found: " + target, true
	}
	if !services.AgentProjectMatches(ag, sess.ProjectID) {
		return "agent not in project / not home-project bound: " + target, true
	}
	doc, err := org.Get()
	if err != nil {
		return "failed to load org: " + err.Error(), true
	}
	if !services.IsInReportingClosure(doc, leader, target) {
		return "agent is not self or a direct/indirect report of the leader: " + target, true
	}
	return "", false
}

func workspaceFSError(err error) string {
	switch {
	case errors.Is(err, services.ErrWorkspacePathInvalid):
		return "invalid workspace path (escape/symlink rejected): " + err.Error()
	case errors.Is(err, services.ErrWorkspaceFileTooLarge):
		return "file exceeds 1MiB limit"
	case errors.Is(err, services.ErrWorkspaceNotFound):
		return "path not found"
	case errors.Is(err, services.ErrWorkspaceAgentMissing):
		return "agent not found"
	case errors.Is(err, services.ErrWorkspaceIsDir):
		return "path is a directory"
	case errors.Is(err, services.ErrWorkspaceNotDir):
		return "path is not a directory"
	case errors.Is(err, services.ErrWorkspaceExists):
		return "destination already exists"
	default:
		return err.Error()
	}
}
