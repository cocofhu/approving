package channels

import (
	"context"
	"errors"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"
	"github.com/rs/zerolog/log"
)

// errOriginUnbound is internal to target resolution. It never reaches a caller
// of DeliverSendable: the delivery path turns it into a suppressed result.
var errOriginUnbound = errors.New("run detached from its origin conversation")

// ReasonOriginUnbound is why an unsolicited Run message was not delivered.
//
// It is a suppression reason, not a failure: pm_notify_progress promises the
// worker that only a real transport failure returns an error, so an unbound Run
// that answered with one would be retried forever — the detach would make the
// system louder instead of quieter.
const ReasonOriginUnbound = "origin_unbound"

// originState says whether a Run is allowed to speak to the conversation that
// created it.
type originState int

const (
	// originNone: the Run never came from a conversation. Web-triggered runs
	// and cron runs land here, and there is simply nobody waiting on them.
	originNone originState = iota
	// originActive: the Run reports back to its origin conversation.
	originActive
	// originDetached: somebody unbound this Run on purpose. Distinct from
	// originNone because "no target" falls back to the project push target and
	// "detached" must not — falling back would deliver the message to an
	// unrelated ops session, which is the opposite of what was asked for.
	originDetached
)

// runOrigin is the resolved answer to "where is this Run allowed to talk".
type runOrigin struct {
	// Identity is present whenever the Run has a task ledger row, including
	// when it is detached. Detaching stops the talking, not the tracking: the
	// caller still writes status through this.
	Identity *models.TaskIdentity
	Scene    Scene
	Conv     string
	State    originState
}

// Speaks reports whether an unsolicited message for this Run may go out.
func (o runOrigin) Speaks() bool { return o.State == originActive }

// normalizeScene defaults a blank scene to C2C. Seven call sites used to carry
// their own copy of this line.
func normalizeScene(scene Scene) Scene {
	if scene == "" {
		return SceneC2C
	}
	return scene
}

// resolveRunOrigin is the single place that decides where a Run's unsolicited
// messages go, and the only place the unbind mark is read.
//
// It replaced two functions that queried the same table for the same thing
// (reflow's identity lookup and delivery's origin lookup). Keeping them apart
// meant a detach had to be enforced in two places, and the delivery side would
// have missed it entirely: every worker-originated path passes an explicit
// ConversationID, and that side only consulted the ledger when the conversation
// was blank.
func (m *Manager) resolveRunOrigin(projectID, runID string) runOrigin {
	if m.taskContext == nil || strings.TrimSpace(runID) == "" || strings.TrimSpace(projectID) == "" {
		return runOrigin{}
	}
	identity, err := m.taskContext.IdentityForRun(runID, projectID)
	if err != nil {
		log.Warn().Err(err).Str("run", runID).Msg("reflow: loading task identity failed")
		return runOrigin{}
	}
	if identity == nil {
		return runOrigin{}
	}
	origin := runOrigin{Identity: identity}
	conv := strings.TrimSpace(identity.OriginConversationID)
	switch {
	case conv == "":
		origin.State = originNone
	case identity.OriginUnboundAt != nil:
		origin.State = originDetached
	default:
		origin.State = originActive
		origin.Scene = normalizeScene(Scene(strings.TrimSpace(identity.OriginScene)))
		origin.Conv = conv
	}
	return origin
}

// identityForRun loads a Run's task ledger row regardless of whether it still
// talks to its origin. Status writes use this; anything that speaks must go
// through resolveRunOrigin.
func (m *Manager) identityForRun(runID, projectID string) *models.TaskIdentity {
	return m.resolveRunOrigin(projectID, runID).Identity
}

// AnnounceOriginBinding tells a Run's origin conversation that it is being
// detached, or that it is being reconnected, and reports whether the notice
// actually went out.
//
// Detaching without a word is the failure mode this exists to avoid: the person
// who asked for the work would sit waiting for an update that is never coming
// and has no way to know why. The caller sends this before writing the detach
// mark — afterwards the guard would swallow the farewell itself.
//
// The wording is the conversation model's, like every other line the platform
// speaks; the fixed strings are only what a missing or slow model falls back to.
func (m *Manager) AnnounceOriginBinding(ctx context.Context, projectID, runID string, bound bool) bool {
	origin := m.resolveRunOrigin(projectID, runID)
	identity := origin.Identity
	if identity == nil {
		return false
	}
	// On detach the mark is not written yet, so the origin still reads active.
	// On re-attach it has just been cleared. Either way an origin that cannot
	// speak has nobody to tell.
	if !origin.Speaks() {
		return false
	}
	if ctx == nil {
		ctx = m.baseCtx
	}
	language := taskLanguage(identity)
	title := strings.TrimSpace(identity.ShortTitle)
	line := originUnboundLine(title, language)
	verb := "unbound"
	if bound {
		line = originReboundLine(title, language)
		verb = "rebound"
	}
	text := m.speakOperationalLine(ctx, line)
	if strings.TrimSpace(text) == "" {
		return false
	}
	result, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: identity.ProjectID, Scene: origin.Scene, ConversationID: origin.Conv,
		UserID: identity.OriginExternalUserID, RunID: identity.RunID,
		Kind: sendable.KindFinal, Reason: ReasonOriginBinding,
		Priority: sendable.PriorityCritical,
		// One notice per transition. Without the verb a detach and a later
		// re-attach would collapse into each other and the second would be
		// silently deduped away.
		DedupeKey: strings.Join([]string{"origin-binding", verb, runID, origin.Conv}, ":"),
		Text:      text,
	})
	if err != nil {
		log.Warn().Err(err).Str("run", runID).Bool("bound", bound).
			Msg("origin binding notice did not reach the conversation")
		return false
	}
	return result.Sent
}

// originUnboundLine is the goodbye. It says where to look instead, because
// "you will not hear from me again" without an alternative is worse than
// silence.
func originUnboundLine(title, language string) operationalLine {
	fallback := "这件事后面不在这儿同步了，进展去页面上看。"
	if services.NormalizeLanguage(language) == "en" {
		fallback = "I won't be posting updates on this one here any more — check the run page for progress."
	}
	return operationalLine{
		Situation: "这件事以后不在这个会话里自动同步了，对方要看进展得去页面上看。" +
			"说清楚不是任务停了，也不是不能再问你——他随时可以问，只是不会再自动收到消息。",
		Facts:    title,
		Avoid:    title,
		Language: language,
		Fallback: fallback,
	}
}

// originReboundLine is the symmetric notice when the Run is reconnected.
func originReboundLine(title, language string) operationalLine {
	fallback := "这件事又回到这儿同步了，后面有进展我照常告诉你。"
	if services.NormalizeLanguage(language) == "en" {
		fallback = "This one is back on here — I'll keep you posted as it moves."
	}
	return operationalLine{
		Situation: "这件事重新回到这个会话里同步了，后面有进展会照常说。" +
			"不要复述中间错过了什么，就说恢复了。",
		Facts:    title,
		Avoid:    title,
		Language: language,
		Fallback: fallback,
	}
}
