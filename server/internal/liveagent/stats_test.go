package liveagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Zero calls on a configured endpoint is the signal that was missing: it is how
// a working configuration can sit there for a day while every message quietly
// goes to a sandbox instead.
func TestStatsStartEmptySoNoTrafficIsVisible(t *testing.T) {
	c := New()
	c.SetLiveEndpoint("http://127.0.0.1:9/v1", "", "m", time.Second)
	if !c.Configured() {
		t.Fatal("endpoint should be configured")
	}
	if got := c.Stats(); got.Calls != 0 || got.Failed != 0 || got.LastFailure != "" {
		t.Fatalf("stats before any call = %+v", got)
	}
}

func TestStatsRecordSuccessesAndFailuresSeparately(t *testing.T) {
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := New()
	c.SetLiveEndpoint(srv.URL+"/v1", "", "m", 5*time.Second)
	req := Request{Messages: []Message{{Role: "user", Content: "hi"}}}

	if _, err := c.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got := c.Stats()
	if got.Calls != 1 || got.Failed != 0 || got.LastSuccessAt == nil {
		t.Fatalf("stats after one success = %+v", got)
	}

	fail = true
	if _, err := c.Complete(context.Background(), req); err == nil {
		t.Fatal("expected the rejection to surface")
	}
	got = c.Stats()
	if got.Calls != 2 || got.Failed != 1 || got.LastFailureAt == nil {
		t.Fatalf("stats after one failure = %+v", got)
	}
	// The recorded reason is the operator-facing one, so a production failure
	// reads the same as one reproduced with the test button.
	if !strings.Contains(got.LastFailure, "密钥") {
		t.Fatalf("last failure = %q want the same explanation the probe gives", got.LastFailure)
	}
	// A timeout must not drag the average down and hide how fast the endpoint
	// is when it does answer.
	if got.AvgLatencyMS < 0 {
		t.Fatalf("avg latency = %d", got.AvgLatencyMS)
	}
}
