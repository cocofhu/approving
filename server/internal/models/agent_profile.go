package models

import (
	"fmt"
	"strings"
)

// Node config keys for the Agent identity reference.
const (
	AgentProfileKey       = "agent_profile"
	LegacyAgentProfileKey = "skill_profile"
)

func profileString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// AgentProfile reads the Agent name from a node config.
// Non-empty agent_profile wins; otherwise skill_profile is used.
func AgentProfile(cfg map[string]any) string {
	if cfg == nil {
		return ""
	}
	if s := profileString(cfg[AgentProfileKey]); s != "" {
		return s
	}
	return profileString(cfg[LegacyAgentProfileKey])
}

// SetAgentProfile writes agent_profile and drops the legacy key.
func SetAgentProfile(cfg map[string]any, name string) {
	if cfg == nil {
		return
	}
	cfg[AgentProfileKey] = strings.TrimSpace(name)
	delete(cfg, LegacyAgentProfileKey)
}

// NormalizeAgentProfile folds skill_profile into agent_profile and deletes the
// legacy key. When both exist, agent_profile is kept. Empty values are
// preserved. Returns whether the map changed.
func NormalizeAgentProfile(cfg map[string]any) bool {
	if cfg == nil {
		return false
	}
	oldRaw, hasOld := cfg[LegacyAgentProfileKey]
	if !hasOld {
		return false
	}
	// Empty agent_profile is treated as unset so a leftover skill_profile is not dropped.
	if profileString(cfg[AgentProfileKey]) == "" {
		cfg[AgentProfileKey] = profileString(oldRaw)
	}
	delete(cfg, LegacyAgentProfileKey)
	return true
}

// NormalizeGraphAgentProfiles normalizes every node config. Idempotent.
func NormalizeGraphAgentProfiles(g *Graph) bool {
	if g == nil {
		return false
	}
	changed := false
	for i := range g.Nodes {
		if NormalizeAgentProfile(g.Nodes[i].Config) {
			changed = true
		}
	}
	return changed
}
