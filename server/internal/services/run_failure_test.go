package services

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestAggregateRunFailureFromStateRun(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	db.Create(&models.Run{ID: "rf1", Status: "failed"})
	db.Create(&models.StateRun{RunID: "rf1", NodeID: "research", Status: "failed", Error: "sandbox setup failed: create timeout"})

	info := s.AggregateRunFailure("rf1")
	if info.Reason == "" || !strings.Contains(info.Reason, "sandbox setup failed") {
		t.Fatalf("reason=%q", info.Reason)
	}
	if info.FailedNode != "research" {
		t.Fatalf("failedNode=%q", info.FailedNode)
	}
	if !info.NoSandboxLog {
		t.Fatal("expected noSandboxLog when no archived logs")
	}
	display := info.DisplayReason()
	if !strings.Contains(display, "research") || !strings.Contains(display, NoSandboxLogMarker) {
		t.Fatalf("display=%q", display)
	}
}

func TestAggregateRunFailureLastErrorFallback(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	db.Create(&models.Run{ID: "rf2", Status: "failed"})
	db.Create(&models.RunVariable{RunID: "rf2", Name: "last_error", Type: "string", Value: "加载运行上下文失败: not found"})

	info := s.AggregateRunFailure("rf2")
	if info.Reason != "加载运行上下文失败: not found" {
		t.Fatalf("reason=%q", info.Reason)
	}
	if info.DisplayReason() == "" {
		t.Fatal("display empty")
	}
}

func TestDeliveryURLsCollectsHTTPURLVars(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	db.Create(&models.Run{ID: "du1", Status: "completed"})
	db.Create(&models.RunVariable{RunID: "du1", Name: "mr_url", Type: "string", Value: "https://github.com/o/r/pull/1"})
	db.Create(&models.RunVariable{RunID: "du1", Name: "report_url", Type: "string", Value: "https://example.com/r"})
	db.Create(&models.RunVariable{RunID: "du1", Name: "last_error", Type: "string", Value: "https://not-a-delivery.example/x"})
	db.Create(&models.RunVariable{RunID: "du1", Name: "callback_url", Type: "string", Value: "not-http"})
	got := s.DeliveryURLs("du1")
	if len(got) != 2 {
		t.Fatalf("DeliveryURLs = %v, want 2 http *_url vars", got)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "pull/1") || !strings.Contains(joined, "example.com/r") {
		t.Fatalf("DeliveryURLs = %v", got)
	}
	if strings.Contains(joined, "not-a-delivery") {
		t.Fatalf("non *_url var leaked: %v", got)
	}
}

func TestAggregateRunFailureDefaultFallback(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	db.Create(&models.Run{ID: "rf3", Status: "failed"})
	info := s.AggregateRunFailure("rf3")
	if info.Reason != DefaultRunFailureReason {
		t.Fatalf("reason=%q", info.Reason)
	}
	if info.DisplayReason() == "" {
		t.Fatal("display empty")
	}
}

func TestAggregateRunFailureWithSandboxLog(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	db.Create(&models.Run{ID: "rf4", Status: "failed"})
	db.Create(&models.StateRun{RunID: "rf4", NodeID: "research", Status: "failed", Error: "agent exit 1"})
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "log-line-" + string(rune('A'+i%26))
	}
	db.Create(&models.SandboxLog{
		Name: "sbx-rf4", RunID: "rf4", NodeID: "research",
		Content:   strings.Join(lines, "\n") + "\nTAIL_MARKER",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	info := s.AggregateRunFailure("rf4")
	if info.NoSandboxLog {
		t.Fatal("expected sandbox log present")
	}
	if !strings.Contains(info.LogSummaryOrRef, "TAIL_MARKER") {
		t.Fatalf("log summary missing tail: %q", info.LogSummaryOrRef)
	}
	if strings.Contains(info.DisplayReason(), NoSandboxLogMarker) {
		t.Fatalf("display should not claim no log: %q", info.DisplayReason())
	}
}

func TestMarshalRunErrorJSON(t *testing.T) {
	body, err := MarshalRunErrorJSON(RunFailureInfo{
		Reason: "boom", FailedNode: "research", NoSandboxLog: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["failedNode"] != "research" {
		t.Fatalf("parsed=%v", parsed)
	}
	reason, _ := parsed["reason"].(string)
	if reason == "" || !strings.Contains(reason, "boom") {
		t.Fatalf("reason=%q", reason)
	}
	if parsed["noSandboxLog"] != true {
		t.Fatalf("noSandboxLog=%v", parsed["noSandboxLog"])
	}
}

func TestTruncateLogSummary(t *testing.T) {
	if TruncateLogSummary("") != "" {
		t.Fatal("empty")
	}
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line\n")
	}
	out := TruncateLogSummary(b.String())
	if strings.Count(out, "\n")+1 > maxLogSummaryLines {
		t.Fatalf("too many lines: %d", strings.Count(out, "\n")+1)
	}
}

// fake secret fixtures are built by prefix+suffix at runtime so source never
// contains contiguous ghp_/sk- literals that trip gitleaks (CI scans PR diffs).
func testFixtureGitHubPAT() string {
	return "ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789"
}

func testFixtureOpenAIKey() string {
	return "sk-" + "abcdefghijklmnopqrstuvwxyz0123"
}

func testFixtureShortPAT() string {
	return "ghp_" + "abcdefghijklmnopqrstuvwxyz01"
}

// TestRunErrorRedactsSecretsRegression: run_error summaries must not leak
// token/Bearer/key prefixes from reason or sandbox log tails.
func TestRunErrorRedactsSecretsRegression(t *testing.T) {
	ghp := testFixtureGitHubPAT()
	sk := testFixtureOpenAIKey()
	apiVal := "supersecretvalue99"
	bearerTok := "abcdefghijklmnopqr"
	secretLine := "Authorization: Bearer " + ghp + "\n" +
		"cursor key=" + sk + "\n" +
		"api_key=" + apiVal + "\n" +
		"TAIL_OK"
	out := TruncateLogSummary(secretLine)
	for _, leak := range []string{
		ghp,
		sk,
		apiVal,
		"Bearer ghp_",
	} {
		if strings.Contains(out, leak) {
			t.Fatalf("log summary leaked %q in %q", leak, out)
		}
	}
	if !strings.Contains(out, SecretMask) {
		t.Fatalf("expected mask in %q", out)
	}
	if !strings.Contains(out, "TAIL_OK") {
		t.Fatalf("expected non-secret tail kept: %q", out)
	}

	body, err := MarshalRunErrorJSON(RunFailureInfo{
		Reason:          "agent failed with token=leakytoken123 and Bearer " + bearerTok,
		FailedNode:      "implement",
		LogSummaryOrRef: TruncateLogSummary(secretLine),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"leakytoken123", bearerTok, ghp, apiVal} {
		if strings.Contains(body, leak) {
			t.Fatalf("run_error.json leaked %q in %s", leak, body)
		}
	}
}

func TestRedactSensitiveStringExported(t *testing.T) {
	bearerTok := "abcdefghijklmnopqr"
	ghp := testFixtureShortPAT()
	in := "Bearer " + bearerTok + " and " + ghp
	out := RedactSensitiveString(in)
	if strings.Contains(out, bearerTok) || strings.Contains(out, ghp) {
		t.Fatalf("not redacted: %q", out)
	}
}

func TestArtifactSummaryFailedEmptyStillReturnsError(t *testing.T) {
	p, db, pid, wfID := setupPmProgressFixtures(t)
	db.Create(&models.Run{
		ID: "run-empty-fail", WorkflowID: wfID, WorkflowName: "Pipeline",
		Status: "failed", Progress: 0.05, Title: "Early fail", StartedAt: time.Now(),
	})
	db.Create(&models.StateRun{
		RunID: "run-empty-fail", NodeID: "research", Status: "failed",
		Error: "sandbox setup failed: create timeout",
	})

	got := p.ArtifactSummary(pid, "run-empty-fail", 5)
	if got["empty"] != true {
		t.Fatalf("expected empty artifacts: %+v", got)
	}
	errMsg, _ := got["error"].(string)
	if errMsg == "" {
		t.Fatalf("expected non-empty error on empty failed run: %+v", got)
	}
	if !strings.Contains(errMsg, "sandbox setup failed") {
		t.Fatalf("error=%q", errMsg)
	}
}

func TestArtifactSummarySuccessOmitsFailureFields(t *testing.T) {
	p, _, pid, _ := setupPmProgressFixtures(t)
	got := p.ArtifactSummary(pid, "run-a", 5)
	if got["empty"] != false {
		t.Fatalf("artifacts: %+v", got)
	}
	if _, ok := got["error"]; ok {
		t.Fatalf("success run must not expose error: %+v", got)
	}
}
