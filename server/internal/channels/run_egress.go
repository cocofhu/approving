package channels

// Reason strings for every Run-lifecycle message this package can put in front
// of a user.
//
// They are constants rather than inline literals because they are the join key
// between the code and services.EventRoutes — the table that answers "which Run
// events reach a conversation". Before that table existed the answer could only
// be reconstructed by reading four SetXxxObserver calls in main.go plus three
// MCP entry points, and nobody could see it from the product at all.
//
// A new Run egress path must add its Reason here and a row to that table;
// TestEventRoutingRegistryMatchesRunEgressReasons fails otherwise. That failure
// is the point: an egress nobody declared is an egress nobody can switch off.
const (
	// ReasonRunAccepted is the once-per-run acknowledgement that a dispatched
	// Run was picked up.
	ReasonRunAccepted = "run_accepted"
	// ReasonPMNotifyProgress carries a fact the worker reported about a Run in
	// flight — progress, blocked, needs-confirmation, or a final conclusion.
	ReasonPMNotifyProgress = "pm_notify_progress"
	// ReasonPMReply is an answer the worker explicitly submitted for a turn.
	ReasonPMReply = "pm_reply"
	// ReasonTaskPaused tells the origin conversation its Run stopped and needs
	// a person.
	ReasonTaskPaused = "task_paused"
	// ReasonTaskOutcome is the conclusion of a finished Run.
	ReasonTaskOutcome = "task_outcome"
	// ReasonRunNotification is the project-level template push. It is the one
	// Run egress addressed to the project's ops target rather than to whoever
	// asked for the work.
	ReasonRunNotification = "run_notification"
	// ReasonOriginBinding is the notice that a Run is being detached from this
	// conversation, or reconnected to it. It travels the guarded path like
	// everything else; it escapes its own guard only by ordering, since the
	// detach mark is written after the goodbye has been sent.
	ReasonOriginBinding = "origin_binding"
)

// RunEgressReasons is the closed set of Reasons above. Order is not meaningful;
// callers compare it as a set.
func RunEgressReasons() []string {
	return []string{
		ReasonRunAccepted,
		ReasonPMNotifyProgress,
		ReasonPMReply,
		ReasonTaskPaused,
		ReasonTaskOutcome,
		ReasonRunNotification,
		ReasonOriginBinding,
	}
}
