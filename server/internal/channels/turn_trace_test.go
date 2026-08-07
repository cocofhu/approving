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
