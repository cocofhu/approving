package channels

import (
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
)

// Recording what the conversation layer decided is a feature, not telemetry.
//
// Four rounds of this design were corrected the same way: someone reported a
// bad reply, a phrase went on a ban list, and nobody could say whether routing
// had improved or merely moved. A decision that is not recorded can only be
// argued about. One that is — with the briefing the model saw, what it
// produced, what the tools returned, and what the user actually received — can
// be replayed, counted, and eventually trained on.

// Route names a decision in one word so samples can be counted without parsing.
const (
	routeReply       = "reply"
	routeDispatch    = "dispatch"
	routeRefine      = "refine"
	routeFallthrough = "fallthrough"
	routeDirect      = "direct"
)

// Egress names which layer spoke to the user. The direct case is the
// degradation path, and it is recorded rather than assumed rare.
const (
	egressDirector = "director"
	egressPMDirect = "pm_direct"
)

// sampleRecorder accumulates one turn's decision. A nil recorder is usable and
// does nothing, so call sites do not branch on whether sampling is configured.
type sampleRecorder struct {
	m      *Manager
	sample models.LiveDecisionSample
	started time.Time

	completions []map[string]any
	toolResults []map[string]any
	actions     []map[string]any
	spans       []TraceSpan
	flags       []string
}

func (m *Manager) newSampleRecorder(rc *runningChannel, in InboundMessage) *sampleRecorder {
	ensureTraceID(&in)
	return &sampleRecorder{
		m:       m,
		started: time.Now(),
		sample: models.LiveDecisionSample{
			ProjectID: rc.cfg.ProjectID, Channel: rc.cfg.Type, Scene: string(in.Scene),
			ConversationID: in.ConversationID,
			TurnID:         turnScope(rc, in),
			UserMessageID:  strings.TrimSpace(in.RecordedMessageID),
			UserText:       strings.TrimSpace(in.Text),
			TraceID:        strings.TrimSpace(in.TraceID),
			Egress:         egressDirector,
			Model:          liveModelName(m.live),
		},
	}
}

func liveModelName(model LiveModel) string {
	type named interface{ ModelName() string }
	if n, ok := model.(named); ok {
		return strings.TrimSpace(n.ModelName())
	}
	return ""
}

func (r *sampleRecorder) briefedWith(dc directorContext) {
	if r == nil {
		return
	}
	r.sample.DirectorContext = encodeToolResult(dc)
}

func (r *sampleRecorder) shown(messages []liveagent.Message) {
	if r == nil {
		return
	}
	window := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		window = append(window, map[string]any{"role": msg.Role, "content": msg.Content})
	}
	r.sample.Transcript = encodeToolResult(window)
}

func (r *sampleRecorder) completed(res liveagent.Result) {
	if r == nil {
		return
	}
	r.completions = append(r.completions, map[string]any{
		"text": res.Text, "tool": res.ToolName, "args": res.Args,
	})
}

func (r *sampleRecorder) failed(err error) {
	if r == nil || err == nil {
		return
	}
	r.completions = append(r.completions, map[string]any{"error": err.Error()})
	r.flags = append(r.flags, "model_unavailable")
}

func (r *sampleRecorder) toolReturned(tool string, args map[string]string, result string) {
	if r == nil {
		return
	}
	r.toolResults = append(r.toolResults, map[string]any{
		"tool": tool, "args": args, "result": result,
	})
	r.spans = append(r.spans, finishSpan("tool:"+tool, "ok", truncateRunes(result, 200), time.Now()))
}

// acted records something the platform tried to send, delivered or not. A
// suppressed message is as interesting as a sent one: it is usually where a
// turn went quiet.
func (r *sampleRecorder) acted(reason, text string, result DeliveryResult) {
	if r == nil {
		return
	}
	r.actions = append(r.actions, map[string]any{
		"reason": reason, "text": text,
		"sent": result.Sent, "decision": result.Decision.Reason,
	})
	status := "ok"
	if !result.Sent {
		status = "suppressed"
	}
	r.spans = append(r.spans, finishSpan("outbound:"+reason, status, truncateRunes(text, 120), time.Now()))
}

func (r *sampleRecorder) span(name, status, detail string, started time.Time) {
	if r == nil {
		return
	}
	r.spans = append(r.spans, finishSpan(name, status, detail, started))
}

func (r *sampleRecorder) flag(flags ...string) {
	if r == nil {
		return
	}
	for _, f := range flags {
		if strings.TrimSpace(f) != "" {
			r.flags = append(r.flags, f)
		}
	}
}

func (r *sampleRecorder) degraded() {
	if r == nil {
		return
	}
	r.sample.Degraded = true
	r.sample.Egress = egressPMDirect
}

// commit writes the sample and returns its id, which links a decision made now
// to a conclusion that arrives minutes later.
func (r *sampleRecorder) commit(route string) string {
	if r == nil || r.m == nil || r.m.samples == nil {
		return ""
	}
	r.sample.Route = route
	r.sample.RawCompletion = encodeToolResult(r.completions)
	r.sample.ToolResults = encodeToolResult(r.toolResults)
	r.sample.Actions = encodeToolResult(r.actions)
	r.sample.QualityFlags = r.flags
	if !r.started.IsZero() {
		r.sample.LatencyMs = int(time.Since(r.started).Milliseconds())
	}
	// live_route is the root span for every recorded decision. Tool/outbound
	// spans nest under it chronologically; prepend so readers see the route first.
	routeStatus := "ok"
	for _, f := range r.flags {
		switch f {
		case "model_unavailable", "tool_loop_exhausted":
			routeStatus = "error"
		case "live_not_configured":
			routeStatus = "skipped"
		}
	}
	spans := append([]TraceSpan{finishSpan("live_route", routeStatus, route, r.started)}, r.spans...)
	r.sample.Spans = encodeToolResult(spans)
	id, err := r.m.samples.Record(r.sample)
	if err != nil {
		log.Debug().Err(err).Str("project", r.sample.ProjectID).Str("trace", r.sample.TraceID).
			Msg("conversation decision not recorded")
		return ""
	}
	return id
}

// attachSampleOutcome completes a delegation's record once the work layer has
// something to say. Best-effort: a lost sample costs a training example, never
// a reply.
func (m *Manager) attachSampleOutcome(sampleID, outcome, egress string) {
	if m.samples == nil || strings.TrimSpace(sampleID) == "" {
		return
	}
	if err := m.samples.AttachOutcome(sampleID, truncateRunes(outcome, 2000), egress); err != nil {
		log.Debug().Err(err).Msg("work outcome not attached to its decision sample")
	}
}

// appendTraceSpan records a late step (sandbox turn, synthesis) onto the sample
// identified by TraceID.
func (m *Manager) appendTraceSpan(traceID string, span TraceSpan) {
	if m.samples == nil || strings.TrimSpace(traceID) == "" {
		return
	}
	if err := m.samples.AppendSpanByTrace(traceID, span); err != nil {
		log.Debug().Err(err).Str("trace", traceID).Str("span", span.Name).
			Msg("trace span not appended")
	}
}
