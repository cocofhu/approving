package services

import (
	"strings"

	"github.com/cocofhu/approving/internal/models"
)

// ResolveNotifyEvents returns the effective P0 event set for a workflow under
// a project policy. Aligns with Demo page.html resolveEvents:
//
//	enabled=false → empty (hard close)
//	mode=off      → empty
//	mode=custom   → workflow events (may be empty)
//	inherit/else  → project defaultEvents
//
// Only waiting_human and failed participate in P0 delivery; completed (and any
// other kind) is filtered out even if present in stored policy.
func ResolveNotifyEvents(project models.ProjectNotifyPolicy, workflow models.WorkflowNotifyPolicy) []string {
	if !project.IsEnabled() {
		return nil
	}
	var raw []string
	switch workflow.EffectiveMode() {
	case models.NotifyModeOff:
		return nil
	case models.NotifyModeCustom:
		raw = append([]string(nil), workflow.Events...)
	default: // inherit
		raw = project.EffectiveDefaultEvents()
	}
	return filterP0NotifyEvents(raw)
}

// NotifyEventAllowed reports whether kind is in the resolved event set.
func NotifyEventAllowed(events []string, kind string) bool {
	kind = strings.TrimSpace(kind)
	for _, e := range events {
		if e == kind {
			return true
		}
	}
	return false
}

func filterP0NotifyEvents(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 2)
	for _, e := range in {
		switch strings.TrimSpace(e) {
		case models.NotifyKindWaitingHuman, models.NotifyKindFailed:
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// NormalizeProjectNotifyPolicy sanitizes a project policy for persistence.
// Nil Enabled defaults to true; nil DefaultEvents defaults to waiting_human+failed.
// completed may be stored for schema foresight but is stripped from delivery set
// at resolve time; we still allow it in DefaultEvents for UI grey-state roundtrip
// only when explicitly present — P0 UI never writes it as selectable, so strip
// unknown kinds except the known three for forward-compat storage foresight.
func NormalizeProjectNotifyPolicy(p models.ProjectNotifyPolicy) models.ProjectNotifyPolicy {
	if p.Enabled == nil {
		on := true
		p.Enabled = &on
	}
	if p.DefaultEvents == nil {
		p.DefaultEvents = []string{models.NotifyKindWaitingHuman, models.NotifyKindFailed}
	} else {
		p.DefaultEvents = normalizeStoredEvents(p.DefaultEvents)
	}
	return p
}

// NormalizeWorkflowNotifyPolicy sanitizes a workflow override for persistence.
func NormalizeWorkflowNotifyPolicy(w models.WorkflowNotifyPolicy) models.WorkflowNotifyPolicy {
	switch w.Mode {
	case models.NotifyModeOff, models.NotifyModeInherit, models.NotifyModeCustom:
		// ok
	case "":
		w.Mode = models.NotifyModeInherit
	default:
		w.Mode = models.NotifyModeInherit
	}
	if w.Mode != models.NotifyModeCustom {
		// Keep events for round-trip when switching back to custom, but still
		// normalize kinds so junk does not accumulate.
		w.Events = normalizeStoredEvents(w.Events)
	} else {
		w.Events = normalizeStoredEvents(w.Events)
	}
	return w
}

func normalizeStoredEvents(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, e := range in {
		e = strings.TrimSpace(e)
		switch e {
		case models.NotifyKindWaitingHuman, models.NotifyKindFailed, models.NotifyKindCompleted:
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// NotifyPoliciesEqual compares two project policies for change detection.
func NotifyPoliciesEqual(a, b models.ProjectNotifyPolicy) bool {
	a = NormalizeProjectNotifyPolicy(a)
	b = NormalizeProjectNotifyPolicy(b)
	if a.IsEnabled() != b.IsEnabled() {
		return false
	}
	return stringSlicesEqual(a.DefaultEvents, b.DefaultEvents)
}

// WorkflowNotifyPoliciesEqual compares workflow overrides.
func WorkflowNotifyPoliciesEqual(a, b models.WorkflowNotifyPolicy) bool {
	a = NormalizeWorkflowNotifyPolicy(a)
	b = NormalizeWorkflowNotifyPolicy(b)
	if a.Mode != b.Mode {
		return false
	}
	return stringSlicesEqual(a.Events, b.Events)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
