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
// joined by OriginTraceID on TaskIdentity.
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
		OriginTraceID:       traceID,
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
	for _, want := range []string{"live_route", "tool:dispatch_pm", "sandbox_turn", "synthesis", "outbound:task_outcome"} {
		if !containsString(names, want) {
			t.Fatalf("missing span %q in %v (raw=%s)", want, names, sample.Spans)
		}
	}
	if sample.Route != routeDispatch {
		t.Fatalf("route = %q want dispatch", sample.Route)
	}
}

// TestConcurrentReflowUsesOriginTraceIDNotNewestSample proves two tasks in the
// same origin conversation keep their own inbound TraceID when both reflow —
// the old newest-sample best-effort would cross-wire them.
func TestConcurrentReflowUsesOriginTraceIDNotNewestSample(t *testing.T) {
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
	tasks := services.NewTaskContextService(db)
	m.SetTaskContextService(tasks)

	type job struct {
		runID, traceID, title, summary string
	}
	jobs := []job{
		{"run-concurrent-a", "tr-concurrent-a", "错误处理", "超时策略已对齐。"},
		{"run-concurrent-b", "tr-concurrent-b", "登录性能", "首屏降到 1.1s。"},
	}
	for _, j := range jobs {
		if _, err := tasks.EnsureIdentity(services.EnsureTaskIdentityInput{
			RunID: j.runID, ProjectID: "proj",
			UserID:     services.SyntheticQQUserID("u1"),
			ShortTitle: j.title, Status: "running",
			OriginChannel: "qq", OriginScene: string(SceneC2C),
			OriginConversationID: "user-shared", OriginExternalUserID: "u1",
			OriginTraceID: j.traceID, Language: "zh-CN",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Record(models.LiveDecisionSample{
			ProjectID: "proj", Channel: "qq", Scene: string(SceneC2C),
			ConversationID: "user-shared", TraceID: j.traceID, Route: routeDispatch,
			Spans: `[{"name":"live_route","status":"ok"},{"name":"tool:dispatch_pm","status":"ok"}]`,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Newest-first sample list would prefer B; A must still join its own OriginTraceID.
	for _, j := range jobs {
		if err := m.ReflowTaskOutcome(context.Background(), TaskOutcome{
			ProjectID: "proj", RunID: j.runID, Status: "completed",
			ResultSummary: j.summary,
		}); err != nil {
			t.Fatalf("reflow %s: %v", j.runID, err)
		}
	}
	for _, j := range jobs {
		sample, err := svc.GetByTrace("proj", j.traceID)
		if err != nil || sample == nil {
			t.Fatalf("sample %s: %+v err=%v", j.traceID, sample, err)
		}
		var spans []TraceSpan
		if err := json.Unmarshal([]byte(sample.Spans), &spans); err != nil {
			t.Fatal(err)
		}
		names := spanNames(spans)
		for _, want := range []string{"synthesis", "outbound:task_outcome"} {
			if !containsString(names, want) {
				t.Fatalf("%s missing %q in %v (would indicate cross-wired reflow)", j.traceID, want, names)
			}
		}
		var outcomeDetail string
		for _, sp := range spans {
			if sp.Name == "outbound:task_outcome" {
				outcomeDetail = sp.Detail
			}
		}
		if outcomeDetail == "" {
			t.Fatalf("%s has no outbound:task_outcome detail", j.traceID)
		}
		for _, other := range jobs {
			if other.traceID == j.traceID {
				continue
			}
			// Fallback names the task; the other title must not appear on this chain.
			if strings.Contains(outcomeDetail, other.title) {
				t.Fatalf("%s outcome cross-wired to %s: %q", j.traceID, other.title, outcomeDetail)
			}
		}
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
