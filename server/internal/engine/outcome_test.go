package engine

import (
	"context"
	"testing"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
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
