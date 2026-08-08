package channels

import (
	"sort"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

// TestEventRoutingRegistryMatchesRunEgressReasons is the join that keeps
// services.EventRoutes from turning into a stale document. The two sets are
// declared in different packages (channels depends on services, so the table
// cannot import these constants), and this test is the only thing that notices
// when one of them moves.
//
// It is deliberately bidirectional. A missing row means a Run egress nobody
// declared, which is an egress nobody can switch off; a leftover row means the
// product promises an event that no longer exists.
func TestEventRoutingRegistryMatchesRunEgressReasons(t *testing.T) {
	declared := map[string]bool{}
	for _, reason := range RunEgressReasons() {
		if declared[reason] {
			t.Fatalf("duplicate reason %q in RunEgressReasons", reason)
		}
		declared[reason] = true
	}

	routed := map[string]bool{}
	for _, route := range services.EventRoutes() {
		if route.NoEgress {
			// Exempt on purpose: it never reaches IM, so it has no Reason.
			continue
		}
		if routed[route.Key] {
			t.Fatalf("duplicate route key %q in services.EventRoutes", route.Key)
		}
		routed[route.Key] = true
	}

	for reason := range declared {
		if !routed[reason] {
			t.Errorf("Run egress %q has no row in services.EventRoutes; "+
				"an egress nobody declared is an egress nobody can switch off", reason)
		}
	}
	for key := range routed {
		if !declared[key] {
			t.Errorf("services.EventRoutes lists %q but no Run egress uses it", key)
		}
	}
}

// TestGateAutoInvokeIsListedWithoutEgress locks the one row that exists purely
// so operators can see it: a waiting_human pause also queues an automatic PM
// turn, and that fan-out is invisible everywhere else.
func TestGateAutoInvokeIsListedWithoutEgress(t *testing.T) {
	for _, route := range services.EventRoutes() {
		if route.Key != services.GateAutoInvokeRoute {
			continue
		}
		if !route.NoEgress {
			t.Fatal("gate auto invoke does not reach IM; it must stay marked NoEgress")
		}
		if route.ToLive || route.ToTemplate || route.Unbindable {
			t.Fatal("gate auto invoke has no IM egress, so it cannot be routed or unbound")
		}
		return
	}
	t.Fatal("gate auto invoke is missing from services.EventRoutes")
}

// TestUnbindableMatchesTheGuardedPaths keeps the Unbindable column honest. It
// is a promise about what the aliasing guard in resolveSendableTarget can
// actually stop, not a free-form label: everything that reaches a conversation
// through a RunID-carrying sendable can be unbound, and the project template
// push — which enqueues on its own route — never can.
func TestUnbindableMatchesTheGuardedPaths(t *testing.T) {
	for _, route := range services.EventRoutes() {
		switch {
		case route.NoEgress:
			continue
		case route.Key == ReasonRunNotification:
			if route.Unbindable {
				t.Errorf("%s bypasses resolveSendableTarget, so per-Run unbind cannot stop it", route.Key)
			}
		case route.ToLive:
			if !route.Unbindable {
				t.Errorf("%s reaches the origin conversation with a RunID, so the guard does stop it", route.Key)
			}
		}
	}
}

func TestResolveEventRoutesReflectsProjectPolicy(t *testing.T) {
	templateStatus := func(policy models.ProjectNotifyPolicy) EventRouteStatusView {
		for _, status := range services.ResolveEventRoutes(policy) {
			if status.Key == ReasonRunNotification {
				return EventRouteStatusView{status.TemplateActive, status.ActiveKinds}
			}
		}
		t.Fatal("run_notification missing from resolved routes")
		return EventRouteStatusView{}
	}

	t.Run("default project pushes both kinds", func(t *testing.T) {
		got := templateStatus(models.DefaultProjectNotifyPolicy())
		if !got.Active {
			t.Fatal("a default project does push run notifications")
		}
		sort.Strings(got.Kinds)
		if len(got.Kinds) != 2 {
			t.Fatalf("want waiting_human and failed, got %v", got.Kinds)
		}
	})

	t.Run("kill switch reads as off", func(t *testing.T) {
		off := false
		got := templateStatus(models.ProjectNotifyPolicy{Enabled: &off})
		if got.Active || len(got.Kinds) > 0 {
			t.Fatalf("a disabled project pushes nothing, got %+v", got)
		}
	})

	t.Run("dropping every kind reads as off", func(t *testing.T) {
		got := templateStatus(models.ProjectNotifyPolicy{DefaultEvents: []string{}})
		if got.Active {
			t.Fatal("no kinds selected means the template channel is not actually live")
		}
	})

	t.Run("one kind reads as partially on", func(t *testing.T) {
		got := templateStatus(models.ProjectNotifyPolicy{
			DefaultEvents: []string{models.NotifyKindFailed},
		})
		if !got.Active || len(got.Kinds) != 1 || got.Kinds[0] != models.NotifyKindFailed {
			t.Fatalf("want failed only, got %+v", got)
		}
	})
}

// EventRouteStatusView is a test-local shorthand for the two fields under test.
type EventRouteStatusView struct {
	Active bool
	Kinds  []string
}
