package engine

import (
	"context"
	"strings"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
	"github.com/cocofhu/approving/internal/runtime"
)

// consumeNodeOutcome drains the agent's node_complete mark and merges its
// outputs into res. Missing mark or status=failed yields a failed nodeOutcome.
//
// CAPA A7: when the mark is missing, distinguish "MCP tool surface empty /
// unreachable" (expected MCP traffic but zero host calls) from "tools were
// reachable but agent never called node_complete".
func (e *Engine) consumeNodeOutcome(c *execCtx, node *models.Node, res *runtime.NodeResult) (nodeOutcome, bool) {
	o, ok := e.host.TakeOutcome(c.run.ID, node.ID)
	if !ok {
		errMsg := missingOutcomeErr(e.host.PeekMcpCalls(c.run.ID, node.ID))
		return nodeOutcome{
			status:   "failed",
			err:      errMsg,
			outputMd: "节点失败:" + errMsg,
			outputs:  res.Outputs,
			events:   res.Events,
			usage:    res.Usage,
		}, false
	}
	res.Outputs = mcp.MergeOutcomeOutputs(res.Outputs, o)
	if o.Status == mcp.OutcomeFailed {
		errMsg := strings.TrimSpace(o.Error)
		if errMsg == "" {
			errMsg = strings.TrimSpace(o.Summary)
		}
		if errMsg == "" {
			errMsg = "agent reported failure"
		}
		md := "节点失败:" + errMsg
		if strings.TrimSpace(res.OutputMd) != "" {
			md = res.OutputMd + "\n\n---\n**outcome failed**:" + errMsg
		}
		return nodeOutcome{
			status:   "failed",
			err:      errMsg,
			outputMd: md,
			outputs:  res.Outputs,
			events:   res.Events,
			usage:    res.Usage,
		}, false
	}
	return nodeOutcome{}, true
}

// afterDefaultChecks runs the optional business RPC validator only after
// platform DefaultChecks have already passed (artifacts, test/review gates,
// preview ports, etc.). Default failure paths must not call this.
func (e *Engine) afterDefaultChecks(c *execCtx, node *models.Node, oc nodeOutcome) nodeOutcome {
	if oc.status != "completed" && oc.status != "paused" {
		return oc
	}
	status := mcp.OutcomeSuccess
	if s, _ := oc.outputs["outcome_status"].(string); s == mcp.OutcomeFailed {
		status = mcp.OutcomeFailed
	}
	in := mcp.OutcomeValidateIn{
		RunID:    c.run.ID,
		NodeID:   node.ID,
		NodeType: node.Type,
		Outcome: mcp.NodeOutcome{
			Status:  status,
			Summary: str(oc.outputs["outcome_summary"]),
			Outputs: oc.outputs,
		},
	}
	out, err := e.host.ValidateOutcome(context.Background(), in)
	if err != nil {
		oc.status = "failed"
		oc.err = "业务校验失败:" + err.Error()
		oc.outputMd = "业务校验失败:" + err.Error()
		return oc
	}
	if !out.Accept {
		msg := strings.TrimSpace(out.Message)
		if msg == "" {
			msg = "业务校验未通过"
		}
		oc.status = "failed"
		oc.err = msg
		if strings.TrimSpace(oc.outputMd) != "" {
			oc.outputMd += "\n\n---\n**业务校验未通过**:" + msg
		} else {
			oc.outputMd = "业务校验未通过:" + msg
		}
		return oc
	}
	if len(out.OutputsPatch) > 0 {
		if oc.outputs == nil {
			oc.outputs = map[string]any{}
		}
		for k, v := range out.OutputsPatch {
			oc.outputs[k] = v
		}
	}
	return oc
}

// withOutcome consumes node_complete then runs next (platform DefaultChecks).
// Callers that apply additional gates must invoke afterDefaultChecks themselves
// after those gates; otherwise use finishAgentOutcome.
func (e *Engine) withOutcome(c *execCtx, node *models.Node, res runtime.NodeResult, next func(runtime.NodeResult) nodeOutcome) nodeOutcome {
	// Ownership before TakeOutcome: a Cancel/ResumeFrom zombie that returns from
	// RunAgent after a newer driver claimed the slot must not drain the fresh
	// visit's node_complete mark (host outcomes are keyed only by run+node).
	if c.execGen != 0 && !e.isExecOwner(c.run.ID, c.execGen) {
		return nodeOutcome{
			status:   "cancelled",
			err:      "lost exec ownership",
			outputMd: "dropped late outcome: lost exec ownership",
			events:   res.Events,
			usage:    res.Usage,
		}
	}
	if fail, ok := e.consumeNodeOutcome(c, node, &res); !ok {
		return fail
	}
	oc := next(res)
	if oc.usage == nil {
		oc.usage = res.Usage
	}
	return oc
}

// finishAgentOutcome runs withOutcome then afterDefaultChecks (Default→RPC).
func (e *Engine) finishAgentOutcome(c *execCtx, node *models.Node, res runtime.NodeResult, next func(runtime.NodeResult) nodeOutcome) nodeOutcome {
	oc := e.withOutcome(c, node, res, next)
	return e.afterDefaultChecks(c, node, oc)
}

// agentExecNeedsOutcome reports whether this executor kind requires node_complete.
func agentExecNeedsOutcome(k nodereg.ExecKind) bool {
	switch k {
	case nodereg.ExecAgent, nodereg.ExecPlan, nodereg.ExecStructured,
		nodereg.ExecStructuredGated, nodereg.ExecSubmitMR, nodereg.ExecVisual:
		return true
	// ExecAppPreview: 可达 set_preview 即生产相完成，豁免 node_complete 硬门禁。
	default:
		return false
	}
}

// missingOutcomeErr (CAPA A7) picks the failure reason when node_complete is absent.
// Zero MCP host calls ⇒ tool surface empty/unreachable; any traffic ⇒ agent forgot mark.
func missingOutcomeErr(calls []models.McpCall) string {
	if len(calls) == 0 {
		return "MCP 工具面为空/不可达，无法完成 node_complete"
	}
	return "未调用 node_complete 标记完成"
}
