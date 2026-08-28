package services

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cocofhu/approving/internal/envauth"
)

// SensitiveKeyHit is one Token-class key found in a group subtree (no values).
type SensitiveKeyHit struct {
	Key        string `json:"key"`
	AgentCount int    `json:"agentCount"`
}

// StripGroupSensitiveResult reports bulk strip outcome (no key values).
type StripGroupSensitiveResult struct {
	Cleared      int      `json:"cleared"`
	Failed       []string `json:"failed,omitempty"`
	StrippedKeys []string `json:"strippedKeys"`
	AgentNames   []string `json:"agentNames"`
}

// ScanGroupSensitiveKeys lists Token-class keys that actually appear in the
// group subtree (CollectGroupSubtree). Values are never returned.
func (o *OrgService) ScanGroupSensitiveKeys(groupID string) ([]SensitiveKeyHit, error) {
	if o == nil || o.skill == nil {
		return nil, fmt.Errorf("org service unavailable")
	}
	o.skill.mu.Lock()
	defer o.skill.mu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()

	org, _, err := o.loadLocked()
	if err != nil {
		return nil, err
	}
	sub, err := CollectGroupSubtree(org, groupID)
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	for _, name := range sub.AgentNames {
		a, ok := o.skill.Get(name)
		if !ok {
			continue
		}
		seen := map[string]struct{}{}
		for k := range a.Env {
			k = strings.TrimSpace(k)
			if k == "" || !envauth.IsTokenEnvKey(k) {
				continue
			}
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			counts[k]++
		}
	}
	hits := make([]SensitiveKeyHit, 0, len(counts))
	for k, n := range counts {
		hits = append(hits, SensitiveKeyHit{Key: k, AgentCount: n})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Key < hits[j].Key })
	return hits, nil
}

// StripGroupSensitiveKeys removes the selected Token-class keys from every
// Agent in the group subtree and persists immediately. Shared Agent config is
// never modified. keys must be non-empty and Token-class only.
func (o *OrgService) StripGroupSensitiveKeys(groupID string, keys []string) (StripGroupSensitiveResult, error) {
	if o == nil || o.skill == nil {
		return StripGroupSensitiveResult{}, fmt.Errorf("org service unavailable")
	}
	want, err := normalizeStripTokenKeys(keys)
	if err != nil {
		return StripGroupSensitiveResult{}, err
	}

	o.skill.mu.Lock()
	defer o.skill.mu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()

	org, _, err := o.loadLocked()
	if err != nil {
		return StripGroupSensitiveResult{}, err
	}
	sub, err := CollectGroupSubtree(org, groupID)
	if err != nil {
		return StripGroupSensitiveResult{}, err
	}

	out := StripGroupSensitiveResult{
		StrippedKeys: append([]string(nil), want...),
		AgentNames:   append([]string(nil), sub.AgentNames...),
	}
	for _, name := range sub.AgentNames {
		a, ok := o.skill.Get(name)
		if !ok {
			out.Failed = append(out.Failed, name)
			continue
		}
		changed := false
		if a.Env == nil {
			a.Env = map[string]string{}
		}
		for _, k := range want {
			if _, ok := a.Env[k]; ok {
				delete(a.Env, k)
				changed = true
			}
		}
		if !changed {
			out.Cleared++
			continue
		}
		if err := o.skill.saveUnlocked(a); err != nil {
			out.Failed = append(out.Failed, name)
			continue
		}
		out.Cleared++
	}
	return out, nil
}

func normalizeStripTokenKeys(keys []string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range keys {
		k := strings.TrimSpace(raw)
		if k == "" {
			continue
		}
		if !envauth.IsTokenEnvKey(k) {
			return nil, fmt.Errorf("%w: %s is not a token env key", ErrOrgValidation, k)
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: keys is required", ErrOrgValidation)
	}
	sort.Strings(out)
	return out, nil
}

// stripTokenKeysFromEnvMap drops Token-class keys so individual Agent.env does
// not store API/Git tokens (shared / project layer remains the write surface).
func stripTokenKeysFromEnvMap(env map[string]string) map[string]string {
	if env == nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for k, v := range env {
		k = strings.TrimSpace(k)
		if k == "" || envauth.IsTokenEnvKey(k) {
			continue
		}
		out[k] = v
	}
	return out
}
