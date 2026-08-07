package liveagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// probeServer answers the reachability call and then the tool-call call, so a
// test can decide each independently.
func probeServer(t *testing.T, handler func(w http.ResponseWriter, hasTools bool)) Endpoint {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []any `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		handler(w, len(body.Tools) > 0)
	}))
	t.Cleanup(srv.Close)
	return Endpoint{BaseURL: srv.URL + "/v1", Model: "m", Timeout: 5 * time.Second}
}

func checkByName(t *testing.T, report ProbeReport, name string) ProbeCheck {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("report has no %q check: %+v", name, report.Checks)
	return ProbeCheck{}
}

func TestProbePassesWhenTheEndpointAnswersAndCallsTools(t *testing.T) {
	ep := probeServer(t, func(w http.ResponseWriter, hasTools bool) {
		if hasTools {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[
				{"function":{"name":"report_ready","arguments":"{\"status\":\"ready\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})

	report := Probe(context.Background(), ep)
	if !report.Configured || !report.OK {
		t.Fatalf("report = %+v", report)
	}
	if !checkByName(t, report, CheckReachable).OK || !checkByName(t, report, CheckToolCalls).OK {
		t.Fatalf("checks = %+v", report.Checks)
	}
	if report.Sample != "ok" {
		t.Fatalf("sample = %q want what the model actually said", report.Sample)
	}
}

// A model that answers but cannot call tools is the failure worth catching:
// it looks healthy and silently never escalates anything to the project.
func TestProbeReportsAModelThatCannotCallTools(t *testing.T) {
	ep := probeServer(t, func(w http.ResponseWriter, _ bool) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"我不会调工具"}}]}`))
	})

	report := Probe(context.Background(), ep)
	if !checkByName(t, report, CheckReachable).OK {
		t.Fatalf("a reachable endpoint was reported unreachable: %+v", report.Checks)
	}
	tools := checkByName(t, report, CheckToolCalls)
	if tools.OK {
		t.Fatal("a text-only answer counted as tool-call support")
	}
	if !strings.Contains(tools.Reason, "function calling") {
		t.Fatalf("tool-call failure does not say what to do: %q", tools.Reason)
	}
	if report.OK {
		t.Fatal("report is OK despite a failed check")
	}
}

// Each rejection points at the field that most likely caused it. A settings
// page that only says "it failed" leaves the operator guessing between the
// address, the key and the model name.
func TestProbeBlamesTheFieldBehindEachRejection(t *testing.T) {
	cases := []struct {
		status int
		expect string
	}{
		{http.StatusUnauthorized, "密钥"},
		{http.StatusForbidden, "密钥"},
		{http.StatusNotFound, "/v1"},
		{http.StatusBadRequest, "模型名称"},
		{http.StatusTooManyRequests, "限流"},
		{http.StatusInternalServerError, "端点自己出错"},
	}
	for _, tc := range cases {
		ep := probeServer(t, func(w http.ResponseWriter, _ bool) {
			// The body is deliberately something that must never be shown.
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":"echo of the prompt: sk-secret"}`))
		})
		report := Probe(context.Background(), ep)
		reason := checkByName(t, report, CheckReachable).Reason
		if !strings.Contains(reason, tc.expect) {
			t.Fatalf("HTTP %d reason = %q want it to mention %q", tc.status, reason, tc.expect)
		}
		if strings.Contains(reason, "sk-secret") || strings.Contains(reason, "echo") {
			t.Fatalf("HTTP %d leaked the response body: %q", tc.status, reason)
		}
	}
}

func TestProbeRefusesAnIncompleteForm(t *testing.T) {
	report := Probe(context.Background(), Endpoint{BaseURL: "http://x/v1"})
	if report.Configured || report.OK {
		t.Fatalf("a form with no model was probed anyway: %+v", report)
	}
	if len(report.Checks) != 1 || checkByName(t, report, CheckReachable).OK {
		t.Fatalf("checks = %+v", report.Checks)
	}
}

func TestProbeReportsAnUnreachableAddress(t *testing.T) {
	// Port 0 is never listening, so this exercises the connection-error path
	// rather than any HTTP status.
	report := Probe(context.Background(), Endpoint{
		BaseURL: "http://127.0.0.1:1/v1", Model: "m", Timeout: 2 * time.Second,
	})
	if report.OK {
		t.Fatal("an unreachable endpoint passed")
	}
	if reason := checkByName(t, report, CheckReachable).Reason; reason == "" {
		t.Fatal("an unreachable endpoint gave no reason at all")
	}
	// The tool-call check must be skipped: asking twice would cost a second
	// full timeout for no new information.
	if len(report.Checks) != 1 {
		t.Fatalf("checks = %+v want only reachability", report.Checks)
	}
}

// A probe must not look like traffic, or the runtime stats stop answering
// "is this layer actually being used" — which is the one thing they exist for.
func TestProbeDoesNotCountAsTraffic(t *testing.T) {
	ep := probeServer(t, func(w http.ResponseWriter, _ bool) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	c := New()
	c.SetLiveEndpoint(ep.BaseURL, "", ep.Model, ep.Timeout)

	Probe(context.Background(), ep)
	if got := c.Stats(); got.Calls != 0 {
		t.Fatalf("stats after a probe = %+v want no recorded calls", got)
	}
}
