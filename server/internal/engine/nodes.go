package engine

import (
	"fmt"
	"time"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"

	"github.com/rs/zerolog/log"
)

// nodeOutcome is the result of executing a single state.
type nodeOutcome struct {
	status   string // completed | failed | paused
	outputMd string
	outputs  map[string]any
	events   []models.AcpEvent
	// usage is this save's token delta (added onto StateRun.Usage). nil skips.
	usage *models.TokenUsage
	// usageByModel is this save's per-model delta (added onto StateRun.UsageByModel).
	usageByModel models.TokenUsageByModel
	err          string
	goto_        string // branch target
	// sandboxSetup marks a react node's sandbox/ACP infrastructure failure
	// (distinct from a normal clarify pause or agent execution fault).
	sandboxSetup bool
	// retryable marks a failure the engine may auto-retry from the failure
	// position (see isAutoRetryable / tryAutoRetry). Zero value false means
	// not auto-retryable; only failure construction sites that opt in set true.
	// Display text (err/outputMd) is intentionally decoupled from retryability.
	retryable bool
}

// executeNode dispatches to the per-type executor via the node registry.
func (e *Engine) executeNode(c *execCtx, node *models.Node) nodeOutcome {
	start := time.Now()
	defer func() {
		log.Info().Str("run_id", c.run.ID).Str("node_id", node.ID).
			Str("node_type", node.Type).Int("cost_ms", int(time.Since(start).Milliseconds())).
			Msg("node executed")
	}()

	if err := e.checkSkillProfileProject(c, node); err != nil {
		msg := err.Error()
		return nodeOutcome{status: "failed", err: msg, outputMd: msg}
	}

	spec, ok := nodereg.Get(node.Type)
	if !ok {
		return nodeOutcome{
			status:   "failed",
			err:      fmt.Sprintf("未知节点类型 %q", node.Type),
			outputMd: fmt.Sprintf("执行失败:未知节点类型 %q", node.Type),
		}
	}
	var oc nodeOutcome
	switch spec.Exec {
	case nodereg.ExecInput:
		return e.execInput(c, node)
	case nodereg.ExecOutput:
		return e.execOutput(c, node)
	case nodereg.ExecSetVar:
		return e.execSetVar(c, node)
	case nodereg.ExecBranch:
		return e.execBranch(c, node)
	case nodereg.ExecAgent:
		oc = e.execAgent(c, node)
	case nodereg.ExecPlan:
		oc = e.execPlan(c, node)
	case nodereg.ExecReact:
		return e.execReactEnter(c, node)
	case nodereg.ExecStructured:
		oc = e.execStructuredFromSpec(c, node, spec)
	case nodereg.ExecStructuredGated:
		oc = e.execStructuredFromSpec(c, node, spec)
		var gate func(string) (bool, string)
		switch spec.Gate {
		case nodereg.GateTest:
			blockOnSkipped := truthy(node.Config["block_on_skipped"])
			runID := c.run.ID
			gate = func(content string) (bool, string) {
				planJSON, _ := e.store.Get(runID, mcp.PlanArtifactName)
				return testGate(content, blockOnSkipped, planJSON)
			}
		default:
			gate = gateFor(spec.Gate)
		}
		oc = e.applyStructuredGate(c, oc, spec.ArtifactName, gate)
		oc = e.finalizeStructuredGate(c, node, oc, spec.Gate)
	case nodereg.ExecProposalSelect:
		return e.execProposalSelect(c, node)
	case nodereg.ExecSubmitMR:
		oc = e.execSubmitMR(c, node)
	case nodereg.ExecVisual:
		oc = e.execVisual(c, node)
	case nodereg.ExecHumanGate:
		return e.execGate(c, node)
	case nodereg.ExecAppPreview:
		oc = e.execAppPreview(c, node)
	default:
		return nodeOutcome{
			status:   "failed",
			err:      fmt.Sprintf("节点类型 %q 未配置执行器", node.Type),
			outputMd: fmt.Sprintf("执行失败:节点类型 %q 未配置执行器", node.Type),
		}
	}

	if agentExecNeedsOutcome(spec.Exec) {
		oc = e.afterDefaultChecks(c, node, oc)
	}

	if oc.status == "completed" {
		oc = e.maybeEnterReview(c, node, oc)
	}
	return oc
}

func (e *Engine) execStructuredFromSpec(c *execCtx, node *models.Node, spec nodereg.Spec) nodeOutcome {
	render := nodereg.Renderer(spec.Render)
	if render == nil {
		return nodeOutcome{status: "failed", err: "structured renderer missing",
			outputMd: "执行失败:结构化渲染器缺失"}
	}
	return e.execStructuredAgent(c, node, spec.ArtifactName, spec.OutputKey, render)
}

func gateFor(kind nodereg.GateKind) func(string) (bool, string) {
	switch kind {
	case nodereg.GateTest:

		return func(content string) (bool, string) { return testGate(content, false, "") }
	case nodereg.GateReview:
		return reviewGate
	default:
		return func(string) (bool, string) { return true, "" }
	}
}

func (e *Engine) execInput(c *execCtx, node *models.Node) nodeOutcome {
	out := map[string]any{"validated": true}
	for k, v := range c.vars {
		out[k] = v
	}
	return nodeOutcome{status: "completed", outputMd: "输入校验通过。", outputs: out}
}

func (e *Engine) execSetVar(c *execCtx, node *models.Node) nodeOutcome {
	assignments, _ := node.Config["assignments"].([]any)
	for _, a := range assignments {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["var"].(string)
		expr, _ := m["expr"].(string)
		if name == "" {
			continue
		}
		v, err := evalExpr(expr, e.evalContext(c, nil))
		if err != nil {
			v = expr
		}
		c.setVar(name, v)
		e.persistVar(c.run.ID, name, v)
	}
	snap := map[string]any{}
	for k, v := range c.vars {
		snap[k] = v
	}
	return nodeOutcome{status: "completed", outputMd: fmt.Sprintf("赋值完成:%v", snap), outputs: map[string]any{"vars": snap}}
}

func (e *Engine) execBranch(c *execCtx, node *models.Node) nodeOutcome {
	cases, _ := node.Config["cases"].([]any)
	ec := e.evalContext(c, nil)
	for i, ci := range cases {
		m, ok := ci.(map[string]any)
		if !ok {
			continue
		}
		when, _ := m["when"].(string)
		goto_, _ := m["goto"].(string)
		if guardPasses(when, ec) {
			return nodeOutcome{status: "completed",
				outputMd: fmt.Sprintf("命中分支 #%d → %s", i+1, goto_),
				outputs:  map[string]any{"matched": i, "goto": goto_}, goto_: goto_}
		}
	}
	return nodeOutcome{status: "completed", outputMd: "无分支命中", outputs: map[string]any{"matched": -1}}
}
