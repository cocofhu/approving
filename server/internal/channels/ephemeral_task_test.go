package channels

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

// errNoSuchRun is what the engine says when handed an id it has never seen,
// which is exactly what a placeholder key is.
var errNoSuchRun = errors.New("run not found")

// The placeholder key is a shape three packages used to match by hand. Whatever
// it looks like, the row that carries it and the row for a real Run must never
// answer this question the same way.
func TestPlaceholderRowsAreTheOnlyEphemeralOnes(t *testing.T) {
	cases := map[string]bool{
		models.EphemeralRunPrefix + "msg-1":                   true,
		"  " + models.EphemeralRunPrefix + "qq|c2c|user1:170": true,
		"run-abc":          false,
		"":                 false,
		"redispatch:msg-1": false,
	}
	for runID, want := range cases {
		if got := (models.TaskIdentity{RunID: runID}).IsEphemeral(); got != want {
			t.Fatalf("IsEphemeral(%q) = %v, want %v", runID, got, want)
		}
	}
}

// Cancelling a placeholder must not go to the engine. There is no Run behind
// the row, so the engine rejects the id — and the user was told "没能停下来"
// about a turn that had in fact just been stopped.
func TestCancellingAPlaceholderNeverReachesTheEngine(t *testing.T) {
	g := newGPTLive(t)
	stub := g.seedTask(models.EphemeralRunPrefix+"m1", "查错误处理")
	in := InboundMessage{Scene: SceneC2C, ConversationID: "user1", UserID: "u1"}

	var asked []string
	g.m.SetRiskActionExecutor(func(projectID, runID, action string, _ map[string]string) error {
		asked = append(asked, runID)
		return errNoSuchRun
	})

	out := g.m.runCancelWork(context.Background(), g.rc, in, stub.ID)

	if len(asked) != 0 {
		t.Fatalf("the engine was asked to cancel %v; none of those are Runs", asked)
	}
	var res cancelResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("cancel result did not decode: %v (%s)", err, out)
	}
	if res.Failed != "" {
		t.Fatalf("cancel reported failure %q for work that did stop", res.Failed)
	}
	if len(res.Cancelled) != 1 || !strings.Contains(res.Cancelled[0], "错误处理") {
		t.Fatalf("cancelled = %v, want the placeholder's own task", res.Cancelled)
	}

	after, err := g.m.taskContext.IdentityByID(stub.ID, "proj")
	if err != nil || after == nil {
		t.Fatalf("stub row disappeared: %v", err)
	}
	if after.TerminalAt == nil {
		t.Fatal("the placeholder was left running after being cancelled")
	}
}

// A real Run still goes to the engine — the guard above must not have turned
// cancellation into a ledger-only gesture.
func TestCancellingARealRunStillStopsIt(t *testing.T) {
	g := newGPTLive(t)
	task := g.seedTask("run-real", "改登录页")
	in := InboundMessage{Scene: SceneC2C, ConversationID: "user1", UserID: "u1"}

	var asked []string
	g.m.SetRiskActionExecutor(func(projectID, runID, action string, _ map[string]string) error {
		asked = append(asked, runID+"/"+action)
		return nil
	})

	g.m.runCancelWork(context.Background(), g.rc, in, task.ID)

	if len(asked) != 1 || asked[0] != "run-real/cancel_run" {
		t.Fatalf("engine calls = %v, want one cancel for the real Run", asked)
	}
}
