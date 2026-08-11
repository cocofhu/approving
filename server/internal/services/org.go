package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const orgFileName = "_org.json"

// Org-related sentinel errors (mapped to HTTP status by handlers).
var (
	ErrOrgConflict   = errors.New("org revision conflict")
	ErrOrgValidation = errors.New("org validation failed")
)

// OrgGroup is a virtual group node in the organization tree.
// Names may duplicate; clients distinguish them via hierarchical path.
type OrgGroup struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ParentGroupID string `json:"parentGroupId,omitempty"`
}

// OrgAgentMembership holds an Agent's group memberships and optional reporting parent.
// ParentAgent is the other Agent's name (skill_profile identity), not a group id.
type OrgAgentMembership struct {
	GroupIDs    []string `json:"groupIds,omitempty"`
	ParentAgent string   `json:"parentAgent,omitempty"`
}

// AgentOrg is the central organization index stored at <profilesRoot>/_org.json.
// "Ungrouped" is a derived sidebar partition (not a persisted group).
type AgentOrg struct {
	Revision int                           `json:"revision"`
	Groups   []OrgGroup                    `json:"groups"`
	Agents   map[string]OrgAgentMembership `json:"agents"`
}

// OrgService manages the central Agent organization index.
type OrgService struct {
	root  string
	skill *SkillService
	mu    sync.Mutex
}

// NewOrgService builds an OrgService sharing the same profiles root as SkillService.
func NewOrgService(root string, skill *SkillService) *OrgService {
	return &OrgService{root: root, skill: skill}
}

func (o *OrgService) path() string {
	return filepath.Join(o.root, orgFileName)
}

// Get returns the organization document, pruning memberships for deleted agents
// and normalizing dangling parentAgent references. Repairs are written back so
// subsequent Put validation does not fail on stale parents.
func (o *OrgService) Get() (AgentOrg, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	org, err := o.loadLocked()
	if err != nil {
		return AgentOrg{}, err
	}
	if o.pruneAgentsLocked(&org) {
		org.Revision++
		if err := o.saveLocked(org); err != nil {
			return AgentOrg{}, err
		}
	}
	return org, nil
}

// Put replaces the organization document after validation.
// expectedRevision must match the on-disk revision (0 allowed when file is missing/empty).
// On success revision is incremented by 1.
func (o *OrgService) Put(incoming AgentOrg, expectedRevision int) (AgentOrg, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	cur, err := o.loadLocked()
	if err != nil {
		return AgentOrg{}, err
	}
	if cur.Revision != expectedRevision {
		return AgentOrg{}, fmt.Errorf("%w: expected %d, got %d", ErrOrgConflict, cur.Revision, expectedRevision)
	}

	normalized, err := o.validateAndNormalize(incoming)
	if err != nil {
		return AgentOrg{}, err
	}
	normalized.Revision = cur.Revision + 1
	if err := o.saveLocked(normalized); err != nil {
		return AgentOrg{}, err
	}
	return normalized, nil
}

// OnRenameAgent cascades membership map keys and parentAgent references.
func (o *OrgService) OnRenameAgent(oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	org, err := o.loadLocked()
	if err != nil {
		return err
	}
	changed := false
	if m, ok := org.Agents[oldName]; ok {
		delete(org.Agents, oldName)
		org.Agents[newName] = m
		changed = true
	}
	for name, m := range org.Agents {
		if m.ParentAgent == oldName {
			m.ParentAgent = newName
			org.Agents[name] = m
			changed = true
		}
	}
	if !changed {
		return nil
	}
	org.Revision++
	return o.saveLocked(org)
}

// OnDeleteAgent reparents direct reports to the deleted agent's parent (or clears),
// then removes the deleted agent's organization membership.
func (o *OrgService) OnDeleteAgent(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	org, err := o.loadLocked()
	if err != nil {
		return err
	}
	deletedParent := ""
	if m, ok := org.Agents[name]; ok {
		deletedParent = m.ParentAgent
	}
	changed := false
	for n, m := range org.Agents {
		if n == name {
			continue
		}
		if m.ParentAgent == name {
			m.ParentAgent = deletedParent
			org.Agents[n] = m
			changed = true
		}
	}
	if _, ok := org.Agents[name]; ok {
		delete(org.Agents, name)
		changed = true
	}
	if !changed {
		return nil
	}
	org.Revision++
	return o.saveLocked(org)
}

// NewGroupID returns a stable unique group id.
func NewGroupID() string {
	return "g_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func (o *OrgService) loadLocked() (AgentOrg, error) {
	p := o.path()
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyOrg(), nil
		}
		return AgentOrg{}, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return emptyOrg(), nil
	}
	var org AgentOrg
	if err := json.Unmarshal(b, &org); err != nil {
		return AgentOrg{}, fmt.Errorf("parse %s: %w", orgFileName, err)
	}
	if org.Agents == nil {
		org.Agents = map[string]OrgAgentMembership{}
	}
	if org.Groups == nil {
		org.Groups = []OrgGroup{}
	}
	return org, nil
}

func emptyOrg() AgentOrg {
	return AgentOrg{
		Revision: 0,
		Groups:   []OrgGroup{},
		Agents:   map[string]OrgAgentMembership{},
	}
}

func (o *OrgService) saveLocked(org AgentOrg) error {
	if err := os.MkdirAll(o.root, 0o755); err != nil {
		return err
	}
	if org.Agents == nil {
		org.Agents = map[string]OrgAgentMembership{}
	}
	if org.Groups == nil {
		org.Groups = []OrgGroup{}
	}
	b, err := json.MarshalIndent(org, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := o.path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, o.path()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// pruneAgentsLocked removes memberships for agents missing on disk and
// reparents/clears dangling parentAgent refs (same rules as OnDeleteAgent).
// Returns true when org was mutated.
func (o *OrgService) pruneAgentsLocked(org *AgentOrg) bool {
	if org.Agents == nil {
		org.Agents = map[string]OrgAgentMembership{}
		return false
	}
	if o.skill == nil {
		return false
	}
	changed := false
	for {
		var missing string
		var deletedParent string
		for name, m := range org.Agents {
			if !o.skill.Exists(name) {
				missing = name
				deletedParent = m.ParentAgent
				break
			}
		}
		if missing == "" {
			break
		}
		for n, m := range org.Agents {
			if n == missing {
				continue
			}
			if m.ParentAgent == missing {
				m.ParentAgent = deletedParent
				org.Agents[n] = m
			}
		}
		delete(org.Agents, missing)
		changed = true
	}
	for name, m := range org.Agents {
		if m.ParentAgent == "" || o.skill.Exists(m.ParentAgent) {
			continue
		}
		m.ParentAgent = ""
		if len(m.GroupIDs) == 0 {
			delete(org.Agents, name)
		} else {
			org.Agents[name] = m
		}
		changed = true
	}
	return changed
}

func (o *OrgService) validateAndNormalize(incoming AgentOrg) (AgentOrg, error) {
	out := AgentOrg{
		Groups: make([]OrgGroup, 0, len(incoming.Groups)),
		Agents: map[string]OrgAgentMembership{},
	}

	seenIDs := map[string]struct{}{}
	byID := map[string]OrgGroup{}
	for _, g := range incoming.Groups {
		id := strings.TrimSpace(g.ID)
		name := strings.TrimSpace(g.Name)
		parent := strings.TrimSpace(g.ParentGroupID)
		if id == "" {
			return AgentOrg{}, fmt.Errorf("%w: group id is required", ErrOrgValidation)
		}
		if name == "" {
			return AgentOrg{}, fmt.Errorf("%w: group %q name is required", ErrOrgValidation, id)
		}
		if _, dup := seenIDs[id]; dup {
			return AgentOrg{}, fmt.Errorf("%w: duplicate group id %q", ErrOrgValidation, id)
		}
		seenIDs[id] = struct{}{}
		ng := OrgGroup{ID: id, Name: name, ParentGroupID: parent}
		out.Groups = append(out.Groups, ng)
		byID[id] = ng
	}

	for id, g := range byID {
		if g.ParentGroupID == "" {
			continue
		}
		if _, ok := byID[g.ParentGroupID]; !ok {
			return AgentOrg{}, fmt.Errorf("%w: group %q parent %q not found", ErrOrgValidation, id, g.ParentGroupID)
		}
	}
	if err := detectGroupCycles(byID); err != nil {
		return AgentOrg{}, err
	}

	// Stable group order: roots first by name, then DFS.
	sort.SliceStable(out.Groups, func(i, j int) bool {
		return out.Groups[i].ID < out.Groups[j].ID
	})

	existingAgents := map[string]struct{}{}
	if o.skill != nil {
		for _, a := range o.skill.List() {
			existingAgents[a.Name] = struct{}{}
		}
	}

	for name, m := range incoming.Agents {
		name = strings.TrimSpace(name)
		if name == "" {
			return AgentOrg{}, fmt.Errorf("%w: empty agent name in memberships", ErrOrgValidation)
		}
		if o.skill != nil {
			if _, ok := existingAgents[name]; !ok {
				// Ignore memberships for agents that no longer exist (import/cleanup).
				continue
			}
		}
		parent := strings.TrimSpace(m.ParentAgent)
		if parent == name {
			return AgentOrg{}, fmt.Errorf("%w: agent %q cannot be its own parent", ErrOrgValidation, name)
		}
		if parent != "" && o.skill != nil {
			if _, ok := existingAgents[parent]; !ok {
				return AgentOrg{}, fmt.Errorf("%w: parent agent %q does not exist", ErrOrgValidation, parent)
			}
		}
		gids := uniqueNonEmpty(m.GroupIDs)
		for _, gid := range gids {
			if _, ok := byID[gid]; !ok {
				return AgentOrg{}, fmt.Errorf("%w: agent %q references unknown group %q", ErrOrgValidation, name, gid)
			}
		}
		nm := OrgAgentMembership{GroupIDs: gids, ParentAgent: parent}
		if len(nm.GroupIDs) == 0 && nm.ParentAgent == "" {
			// No org data — omit to keep file compact (ungrouped + no parent).
			continue
		}
		out.Agents[name] = nm
	}

	if err := detectReportingCycles(out.Agents); err != nil {
		return AgentOrg{}, err
	}
	return out, nil
}

func uniqueNonEmpty(ids []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func detectGroupCycles(byID map[string]OrgGroup) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var visit func(id string) error
	visit = func(id string) error {
		switch color[id] {
		case gray:
			return fmt.Errorf("%w: group cycle involving %q", ErrOrgValidation, id)
		case black:
			return nil
		}
		color[id] = gray
		g := byID[id]
		if g.ParentGroupID != "" {
			if err := visit(g.ParentGroupID); err != nil {
				return err
			}
		}
		color[id] = black
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func detectReportingCycles(agents map[string]OrgAgentMembership) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var visit func(name string) error
	visit = func(name string) error {
		switch color[name] {
		case gray:
			return fmt.Errorf("%w: reporting cycle involving %q", ErrOrgValidation, name)
		case black:
			return nil
		}
		color[name] = gray
		if m, ok := agents[name]; ok && m.ParentAgent != "" {
			if err := visit(m.ParentAgent); err != nil {
				return err
			}
		}
		color[name] = black
		return nil
	}
	for name := range agents {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

// ApplyDeleteGroup returns a copy of org with groupID removed and cascade rules applied:
// children groups promoted to the deleted group's parent; members lose that group
// (and gain the parent group if the deleted group had a parent).
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

// ApplyMoveAgent applies sidebar drag move semantics: remove sourceGroupID (if any),
// add targetGroupID (if any). Empty target clears all groupIds (ungrouped drop).
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
