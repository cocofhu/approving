package models

import (
	"fmt"
	"strings"
)

// AgentProfileKey is the node config key for the Agent identity reference.
const AgentProfileKey = "agent_profile"

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
func AgentProfile(cfg map[string]any) string {
	if cfg == nil {
		return ""
	}
	return profileString(cfg[AgentProfileKey])
}

// SetAgentProfile writes agent_profile.
func SetAgentProfile(cfg map[string]any, name string) {
	if cfg == nil {
		return
	}
	cfg[AgentProfileKey] = strings.TrimSpace(name)
}
