package services

import "github.com/cocofhu/approving/internal/models"

// EventRoute is one Run event and where it is allowed to speak.
//
// This table exists because that question had no answer anywhere. The wiring
// lives in four SetXxxObserver calls in main.go plus three MCP entry points,
// and a single waiting_human pause quietly fans out to three different side
// effects. Operators could not see any of it, so they could not reason about
// why a Run was loud, quiet, or talking to the wrong audience.
//
// Key is the sendable Reason the path uses (channels.Reason*), which is what
// keeps the table honest: a lock test in the channels package fails when the
// two sets drift apart.
type EventRoute struct {
	Key string `json:"key"`
	// ToLive is whether this event reaches the conversation that asked for the
	// work, phrased by the fast model.
	ToLive bool `json:"toLive"`
	// ToTemplate is whether this event reaches the project's ops push target
	// through the notify template. A route can do both; they are different
	// audiences, not duplicates.
	ToTemplate bool `json:"toTemplate"`
	// Unbindable is whether a single Run can be detached from this event.
	//
	// It is not a free-form promise: it is true exactly when the path goes
	// through Manager.resolveSendableTarget carrying a RunID, which is where
	// the unbind guard sits. The project template push enqueues on its own
	// route and can never be unbound per Run — switch it off in the notify
	// policy instead.
	Unbindable bool `json:"unbindable"`
	// NotifyKinds are the project notify policy kinds this route delivers.
	// Only meaningful when ToTemplate.
	NotifyKinds []string `json:"notifyKinds,omitempty"`
	// NoEgress marks a side effect that never reaches IM at all. It has no
	// sendable Reason, so it is exempt from the key lock test — but it belongs
	// in the table, because "this pause also queues an automatic PM turn" is
	// exactly the kind of invisible fan-out the table is for.
	NoEgress bool `json:"noEgress,omitempty"`
}

// GateAutoInvokeRoute is the one row with no sendable Reason behind it.
const GateAutoInvokeRoute = "gate_auto_invoke"

// EventRoutes returns the routing table. Keys mirror channels.Reason*; the
// literals are repeated here rather than imported because channels depends on
// services and not the other way round. The lock test is what keeps the
// duplication from rotting.
func EventRoutes() []EventRoute {
	return []EventRoute{
		{
			Key: "run_accepted", ToLive: true, Unbindable: true,
		},
		{
			// One Reason, two behaviours: the worker's `progress` facts and its
			// blocked / needs-confirmation / final facts travel the same path
			// and are told apart by kind. Splitting this into two rows would
			// break the key lock, so the split is described rather than modelled.
			Key: "pm_notify_progress", ToLive: true, Unbindable: true,
		},
		{
			Key: "pm_reply", ToLive: true, Unbindable: true,
		},
		{
			// A pause is the one event with two audiences at once.
			Key: "task_paused", ToLive: true, Unbindable: true,
		},
		{
			Key: "task_outcome", ToLive: true, Unbindable: true,
		},
		{
			// The goodbye (and the matching hello) when somebody detaches a Run
			// from this conversation or puts it back.
			Key: "origin_binding", ToLive: true, Unbindable: true,
		},
		{
			Key: "run_notification", ToTemplate: true,
			NotifyKinds: []string{models.NotifyKindWaitingHuman, models.NotifyKindFailed},
		},
		{
			Key: GateAutoInvokeRoute, NoEgress: true,
		},
	}
}

// EventRouteStatus is a route resolved against one project's notify policy, so
// the table shows what is switched on right now rather than what the code is
// capable of.
type EventRouteStatus struct {
	EventRoute
	// TemplateActive is whether the project template push would actually fire
	// for this route today. False when the route never uses it, and false when
	// the project turned the channel or every one of its kinds off.
	TemplateActive bool `json:"templateActive"`
	// ActiveKinds are the NotifyKinds the project currently has switched on.
	ActiveKinds []string `json:"activeKinds,omitempty"`
}

// ResolveEventRoutes overlays a project's notify policy on the routing table.
func ResolveEventRoutes(policy models.ProjectNotifyPolicy) []EventRouteStatus {
	enabled := policy.IsEnabled()
	defaults := make(map[string]bool)
	for _, kind := range policy.EffectiveDefaultEvents() {
		defaults[kind] = true
	}
	routes := EventRoutes()
	out := make([]EventRouteStatus, 0, len(routes))
	for _, route := range routes {
		status := EventRouteStatus{EventRoute: route}
		if route.ToTemplate && enabled {
			for _, kind := range route.NotifyKinds {
				if defaults[kind] {
					status.ActiveKinds = append(status.ActiveKinds, kind)
				}
			}
			status.TemplateActive = len(status.ActiveKinds) > 0
		}
		out = append(out, status)
	}
	return out
}
