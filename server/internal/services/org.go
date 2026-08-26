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

// OrgAgentMembership holds an Agent's virtual-group memberships.
type OrgAgentMembership struct {
	GroupIDs []string `json:"groupIds,omitempty"`
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

// Get returns the organization document, pruning memberships for deleted agents.
// Legacy parentAgent keys in _org.json are stripped on load (idempotent write-back).
func (o *OrgService) Get() (AgentOrg, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	org, legacy, err := o.loadLocked()
	if err != nil {
		return AgentOrg{}, err
	}
	changed := legacy
	if o.pruneAgentsLocked(&org) {
		changed = true
	}
	if changed {
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

	cur, _, err := o.loadLocked()
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

// OnRenameAgent cascades membership map keys when an agent is renamed.
func (o *OrgService) OnRenameAgent(oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	org, _, err := o.loadLocked()
	if err != nil {
		return err
	}
	m, ok := org.Agents[oldName]
	if !ok {
		return nil
	}
	delete(org.Agents, oldName)
	org.Agents[newName] = m
	org.Revision++
	return o.saveLocked(org)
}

// OnDeleteAgent removes the deleted agent's organization membership.
func (o *OrgService) OnDeleteAgent(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	org, _, err := o.loadLocked()
	if err != nil {
		return err
	}
	if _, ok := org.Agents[name]; !ok {
		return nil
	}
	delete(org.Agents, name)
	org.Revision++
	return o.saveLocked(org)
}

// NewGroupID returns a stable unique group id.
func NewGroupID() string {
	return "g_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func (o *OrgService) loadLocked() (AgentOrg, bool, error) {
	p := o.path()
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyOrg(), false, nil
		}
		return AgentOrg{}, false, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return emptyOrg(), false, nil
	}
	hadLegacyParent := strings.Contains(string(b), "parentAgent")
	var org AgentOrg
	if err := json.Unmarshal(b, &org); err != nil {
		return AgentOrg{}, false, fmt.Errorf("parse %s: %w", orgFileName, err)
	}
	if org.Agents == nil {
		org.Agents = map[string]OrgAgentMembership{}
	}
	if org.Groups == nil {
		org.Groups = []OrgGroup{}
	}
	return org, hadLegacyParent, nil
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

// pruneAgentsLocked removes memberships for agents missing on disk.
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
	for name := range org.Agents {
		if !o.skill.Exists(name) {
			delete(org.Agents, name)
			changed = true
		}
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
				continue
			}
		}
		gids := uniqueNonEmpty(m.GroupIDs)
		for _, gid := range gids {
			if _, ok := byID[gid]; !ok {
				return AgentOrg{}, fmt.Errorf("%w: agent %q references unknown group %q", ErrOrgValidation, name, gid)
			}
		}
		nm := OrgAgentMembership{GroupIDs: gids}
		if len(nm.GroupIDs) == 0 {
			continue
		}
		out.Agents[name] = nm
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
