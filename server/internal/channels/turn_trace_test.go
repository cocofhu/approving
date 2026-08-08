package channels

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

func TestInboundTurnGetsATraceableCallChain(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveDispatch("查错误处理", "错误处理"),
	}})

	g.say("m1", "项目的错误处理完整吗")

	if len(g.agent) != 1 || strings.TrimSpace(g.agent[0].TraceID) == "" {
		t.Fatalf("turn reached the agent without a trace id: %+v", g.agent)
	}
	traceID := g.agent[0].TraceID
	if !strings.HasPrefix(traceID, "tr-") {
		t.Fatalf("trace id = %q", traceID)
	}

	var samples []models.LiveDecisionSample
	if err := g.db.Where("trace_id = ?", traceID).Find(&samples).Error; err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d want 1 for trace %s", len(samples), traceID)
	}
	s := samples[0]
	if s.Route != routeDispatch {
		t.Fatalf("route = %q", s.Route)
	}
	if s.ConversationID != "user1" {
		t.Fatalf("conversation = %q", s.ConversationID)
	}
	var spans []TraceSpan
	if err := json.Unmarshal([]byte(s.Spans), &spans); err != nil || len(spans) == 0 {
		t.Fatalf("spans = %q err=%v", s.Spans, err)
	}
	if spans[0].Name != "live_route" {
		t.Fatalf("first span = %+v want live_route", spans[0])
	}
	// Sandbox turn appends after the sample is committed.
	foundSandbox := false
	for _, sp := range spans {
		if sp.Name == "sandbox_turn" {
			foundSandbox = true
		}
	}
	if !foundSandbox {
		// Allow a short moment for append; re-read.
		_ = g.db.Where("id = ?", s.ID).First(&s).Error
		_ = json.Unmarshal([]byte(s.Spans), &spans)
		for _, sp := range spans {
			if sp.Name == "sandbox_turn" {
				foundSandbox = true
			}
		}
	}
	if !foundSandbox {
		t.Fatalf("sandbox_turn span missing: %s", s.Spans)
	}

	listed, err := g.m.samples.List(services.SampleQuery{
		ProjectID: "proj", ConversationID: "user1", Limit: 10,
	})
	if err != nil || len(listed) == 0 || listed[0].TraceID != traceID {
		t.Fatalf("list by conversation = %+v err=%v", listed, err)
	}
}

func TestNoLiveModelStillRecordsADirectTrace(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: false})
	g.say("m1", "在吗")
	if len(g.agent) != 1 || g.agent[0].TraceID == "" {
		t.Fatalf("direct path lost the trace: %+v", g.agent)
	}
	got, err := g.m.samples.GetByTrace("proj", g.agent[0].TraceID)
	if err != nil || got == nil || got.Route != routeDirect {
		t.Fatalf("direct sample = %+v err=%v", got, err)
	}
	_ = context.Background()
}

// TestDispatchReflowOutboundTraceChain covers the observable handoff:
// live_route → tool:dispatch_pm → sandbox_turn → synthesis → outbound:task_outcome
// joined by one TraceID (reflow best-effort matches the origin conversation).
func TestDispatchReflowOutboundTraceChain(t *testing.T) {
	g := newGPTLive(t)
	// handleInbound uses the standalone rc; DeliverSendable resolves via m.running.
	g.m.mu.Lock()
	if g.m.running == nil {
		g.m.running = map[string]*runningChannel{}
	}
	g.m.running[g.rc.cfg.ID] = g.rc
	g.m.mu.Unlock()

	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveDispatch("查错误处理", "错误处理"),
	}})
	g.say("m1", "项目的错误处理完整吗")
	if len(g.agent) != 1 || strings.TrimSpace(g.agent[0].TraceID) == "" {
		t.Fatalf("dispatch lost the trace: %+v", g.agent)
	}
	traceID := g.agent[0].TraceID

	if _, err := g.m.taskContext.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-trace-reflow1", ProjectID: "proj",
		UserID:     services.SyntheticQQUserID("u1"),
		ShortTitle: "错误处理", Status: "running",
		OriginChannel: "qq", OriginScene: string(SceneC2C),
		OriginConversationID: "user1", OriginExternalUserID: "u1",
		Language:            "zh-CN",
		OriginalRequirement: "查错误处理",
	}); err != nil {
		t.Fatal(err)
	}

	if err := g.m.ReflowTaskOutcome(context.Background(), TaskOutcome{
		ProjectID: "proj", RunID: "run-trace-reflow1", Status: "completed",
		ResultSummary: "超时与重试策略已对齐。\n交付链接：https://github.com/org/repo/pull/42",
	}); err != nil {
		t.Fatalf("reflow: %v", err)
	}

	sample, err := g.m.samples.GetByTrace("proj", traceID)
	if err != nil || sample == nil {
		t.Fatalf("sample by trace: %+v err=%v", sample, err)
	}
	var spans []TraceSpan
	if err := json.Unmarshal([]byte(sample.Spans), &spans); err != nil {
		t.Fatal(err)
	}
	names := spanNames(spans)
	for _, want := range []string{"live_route", "sandbox_turn", "synthesis", "outbound:task_outcome"} {
		if !containsString(names, want) {
			t.Fatalf("missing span %q in %v (raw=%s)", want, names, sample.Spans)
		}
	}
	// Successful dispatch returns before toolReturned; the observable handoff is
	// the live_ack (and route=dispatch on the sample), not tool:dispatch_pm.
	if sample.Route != routeDispatch {
		t.Fatalf("route = %q want dispatch", sample.Route)
	}
	if !containsString(names, "outbound:live_ack") && !containsString(names, "tool:dispatch_pm") {
		t.Fatalf("dispatch handoff missing (want live_ack or tool:dispatch_pm): %v", names)
	}
}

// TestLateOutboundWithTraceIDAppendsSpan covers external/late delivery (e.g.
// pm_reply via DeliverConversationReply) when Envelope.TraceID is present.
func TestLateOutboundWithTraceIDAppendsSpan(t *testing.T) {
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "c2c:cron-target",
	}})
	t.Cleanup(m.StopAll)
	m.mu.Lock()
	for _, rc := range m.running {
		rc.adapter = fa
	}
	m.mu.Unlock()
	svc := services.NewLiveSampleService(db)
	m.SetLiveSampleService(svc)

	traceID := "tr-lateoutbound1"
	if _, err := svc.Record(models.LiveDecisionSample{
		ProjectID: "proj", Channel: "qq", Scene: string(SceneC2C),
		ConversationID: "user-late", TraceID: traceID, Route: routeDirect,
		Spans: `[{"name":"live_route","status":"ok"}]`,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := m.DeliverConversationReply(context.Background(), ConversationReply{
		ProjectID: "proj", Scene: SceneC2C, ConversationID: "user-late",
		UserID: "u1", TraceID: traceID, Text: "这边看完了，超时策略没问题。",
	})
	if err != nil || !result.Sent {
		t.Fatalf("deliver = %+v err=%v", result, err)
	}

	sample, err := svc.GetByTrace("proj", traceID)
	if err != nil || sample == nil {
		t.Fatalf("sample = %+v err=%v", sample, err)
	}
	var spans []TraceSpan
	if err := json.Unmarshal([]byte(sample.Spans), &spans); err != nil {
		t.Fatal(err)
	}
	if !containsString(spanNames(spans), "outbound:pm_reply") {
		t.Fatalf("outbound:pm_reply missing: %v", spanNames(spans))
	}
}

func spanNames(spans []TraceSpan) []string {
	out := make([]string, 0, len(spans))
	for _, sp := range spans {
		out = append(out, sp.Name)
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

