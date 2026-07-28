package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"
)

type countingRPC struct {
	calls  int
	accept bool
	msg    string
}

func (c *countingRPC) Validate(ctx context.Context, in mcp.OutcomeValidateIn) (mcp.OutcomeValidateOut, error) {
	c.calls++
	return mcp.OutcomeValidateOut{Accept: c.accept, Message: c.msg}, nil
}

func TestAfterDefaultChecksSkipsRPCOnFailure(t *testing.T) {
	eng, _ := setupEngine(t)
	rpc := &countingRPC{accept: true}
	eng.host.SetRPCOutcomeValidator(rpc)

	oc := eng.afterDefaultChecks(&execCtx{run: &models.Run{ID: "r"}}, &models.Node{ID: "n", Type: "research"},
		nodeOutcome{status: "failed", err: "missing artifact"})
	if oc.status != "failed" {
		t.Fatalf("status = %s", oc.status)
	}
	if rpc.calls != 0 {
		t.Fatalf("RPC called on default failure: %d", rpc.calls)
	}
}

func TestAfterDefaultChecksRunsRPCOnSuccess(t *testing.T) {
	eng, _ := setupEngine(t)
	rpc := &countingRPC{accept: false, msg: "biz no"}
	eng.host.SetRPCOutcomeValidator(rpc)

	oc := eng.afterDefaultChecks(&execCtx{run: &models.Run{ID: "r"}}, &models.Node{ID: "n", Type: "research"},
		nodeOutcome{status: "completed", outputs: map[string]any{"outcome_status": "success"}})
	if oc.status != "failed" || oc.err != "biz no" {
		t.Fatalf("want rpc reject, got status=%s err=%q", oc.status, oc.err)
	}
	if rpc.calls != 1 {
		t.Fatalf("RPC calls = %d", rpc.calls)
	}
}

// TestConsumeNodeOutcomeEmptyMCPSurface (CAPA A7): expected MCP + zero calls +
// missing outcome ⇒ distinct "工具面为空/不可达" failure (not node_complete).
func TestConsumeNodeOutcomeEmptyMCPSurface(t *testing.T) {
	eng, _ := setupEngine(t)
	runID := "r-empty-mcp"
	nodeID := "research"
	eng.host.RegisterRun(runID)
	eng.host.SetActiveNode(runID, nodeID, "research")

	c := &execCtx{run: &models.Run{ID: runID}}
	node := &models.Node{ID: nodeID, Type: "research"}
	res := &runtime.NodeResult{}
	fail, ok := eng.consumeNodeOutcome(c, node, res)
	if ok {
		t.Fatal("want missing outcome failure")
	}
	want := "MCP 工具面为空/不可达，无法完成 node_complete"
	if fail.err != want {
		t.Fatalf("err=%q want %q", fail.err, want)
	}
	if !strings.Contains(fail.outputMd, want) {
		t.Fatalf("outputMd=%q", fail.outputMd)
	}
}

// TestConsumeNodeOutcomeMissingNodeComplete (CAPA A7): MCP traffic present but
// no mark ⇒ keep classic「未调用 node_complete」wording.
func TestConsumeNodeOutcomeMissingNodeComplete(t *testing.T) {
	eng, _ := setupEngine(t)
	runID := "r-has-mcp"
	nodeID := "research"
	tok := eng.host.RegisterRun(runID)
	eng.host.SetActiveNode(runID, nodeID, "research")

	// One built-in MCP call proves the tool surface was reachable.
	st, resp := eng.host.ServeRPC(runID, tok, []byte(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"note.md","content":"x","kind":"markdown"}}}`))
	if st != 200 {
		t.Fatalf("ServeRPC status=%d resp=%s", st, resp)
	}
	if n := len(eng.host.PeekMcpCalls(runID, nodeID)); n != 1 {
		t.Fatalf("PeekMcpCalls=%d want 1", n)
	}

	c := &execCtx{run: &models.Run{ID: runID}}
	node := &models.Node{ID: nodeID, Type: "research"}
	res := &runtime.NodeResult{}
	fail, ok := eng.consumeNodeOutcome(c, node, res)
	if ok {
		t.Fatal("want missing outcome failure")
	}
	want := "未调用 node_complete 标记完成"
	if fail.err != want {
		t.Fatalf("err=%q want %q", fail.err, want)
	}
	if strings.Contains(fail.err, "工具面为空") {
		t.Fatalf("must not use empty-surface wording when MCP traffic exists: %q", fail.err)
	}
}

func TestMissingOutcomeErrBranches(t *testing.T) {
	if got := missingOutcomeErr(nil); got != "MCP 工具面为空/不可达，无法完成 node_complete" {
		t.Fatalf("nil calls: %q", got)
	}
	if got := missingOutcomeErr([]models.McpCall{{Tool: "write_artifact"}}); got != "未调用 node_complete 标记完成" {
		t.Fatalf("with calls: %q", got)
	}
}
