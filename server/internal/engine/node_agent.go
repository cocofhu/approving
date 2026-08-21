package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
	"github.com/cocofhu/approving/internal/runtime"
	"github.com/rs/zerolog/log"
)

func (e *Engine) execAgent(c *execCtx, node *models.Node) nodeOutcome {
	req := e.nodeReq(c, node)
	res, err := e.provider.RunAgent(context.Background(), req)
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(),
			outputMd: "Agent 执行失败:" + err.Error(), retryable: true, events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}

	if c.execGen != 0 && !e.isExecOwner(c.run.ID, c.execGen) {
		return nodeOutcome{
			status:   "cancelled",
			err:      "lost exec ownership",
			outputMd: "dropped late outcome: lost exec ownership",
			events:   res.Events,
			usage:    res.Usage, usageByModel: res.UsageByModel,
		}
	}
	return e.withOutcome(c, node, res, func(r runtime.NodeResult) nodeOutcome {
		return e.finalizeAgentProducts(c, node, r)
	})
}

// visualPageName is the reserved single-file deliverable of a visual node.
const visualPageName = "page.html"

// execVisual runs a visual node: an agent that must produce a single
// self-contained HTML page (page.html) via the artifact-store MCP only (never a
// workspace file, so the repo stays clean). It enforces the artifact's
// existence and re-saves it with kind "html" so the run UI previews it in an
// iframe (empty kind on write_artifact now infers html for page.html; re-save
// still normalizes kind for older / mismatched writes).
func (e *Engine) execVisual(c *execCtx, node *models.Node) nodeOutcome {
	req := e.nodeReq(c, node)
	res, err := e.provider.RunAgent(context.Background(), req)
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "视觉网页节点执行失败:" + err.Error(),
			retryable: true, events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}
	return e.withOutcome(c, node, res, func(r runtime.NodeResult) nodeOutcome {
		return e.finalizeVisual(c, node, r)
	})
}

// finalizeVisual enforces the visual node's page.html contract, re-saves it as
// html for iframe preview, and lifts it into outputs. Shared by the initial run
// and the post-run review finish so both paths behave identically.
func (e *Engine) finalizeVisual(c *execCtx, node *models.Node, r runtime.NodeResult) nodeOutcome {
	content, ok := e.store.Get(c.run.ID, visualPageName)
	if !ok {
		return nodeOutcome{status: "failed", err: "网页产物契约未满足:未写出 " + visualPageName,
			outputMd: "网页产物契约未满足:未写出 " + visualPageName, events: r.Events}
	}

	if _, serr := e.store.Save(c.run.ID, node.ID, visualPageName, "html", content); serr != nil {
		log.Warn().Err(serr).Str("node", node.ID).Msg("visual page re-save failed")
	}
	// Node-scoped physical copy so a later visual node cannot overwrite an
	// earlier source when Save keys on (run_id, name). page.html stays the
	// latest/single-visual alias for gates and the Agent contract.
	if _, serr := e.store.Save(c.run.ID, node.ID, visualNodePageName(node.ID), "html", content); serr != nil {
		log.Warn().Err(serr).Str("node", node.ID).Msg("visual node-scoped page save failed")
	}
	outputs := r.Outputs
	if outputs == nil {
		outputs = map[string]any{}
	}
	outputs["page"] = content
	md := r.OutputMd
	if strings.TrimSpace(md) == "" {
		md = "已生成可视化网页 " + visualPageName
	}
	return nodeOutcome{status: "completed", outputMd: md, outputs: outputs, events: r.Events}
}

// execPlan runs a plan node like an agent node, but enforces the plan contract:
// the node must have written the global plan via the set_plan MCP tool. The
// plan (plan.json) is the node's sole deliverable — no generic produces field
// and no auto-captured fallback.
func (e *Engine) execPlan(c *execCtx, node *models.Node) nodeOutcome {
	req := e.nodeReq(c, node)
	res, err := e.provider.RunAgent(context.Background(), req)
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "计划节点执行失败:" + err.Error(),
			retryable: true, events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}
	return e.withOutcome(c, node, res, func(r runtime.NodeResult) nodeOutcome {
		return e.finalizePlan(c, node, r)
	})
}

// finalizePlan enforces the plan node's set_plan contract and lifts the plan
// (rendered markdown + raw JSON) into outputs. Shared by the initial run and
// the post-run review finish.
func (e *Engine) finalizePlan(c *execCtx, node *models.Node, r runtime.NodeResult) nodeOutcome {
	planJSON, ok := e.store.Get(c.run.ID, mcp.PlanArtifactName)
	if !ok {
		return nodeOutcome{status: "failed", err: "计划契约未满足:未调用 set_plan 写入计划",
			outputMd: "计划契约未满足:未调用 set_plan", events: r.Events}
	}

	outputs := r.Outputs
	if outputs == nil {
		outputs = map[string]any{}
	}
	outputs["plan"] = mcp.RenderPlanMarkdown(planJSON)
	outputs["plan_json"] = planJSON
	return nodeOutcome{status: "completed", outputMd: r.OutputMd, outputs: outputs, events: r.Events}
}

// execStructuredAgent runs an autonomous framework-card node (research / test /
// review / proposal) like an agent node, then enforces its structured-product
// contract: the reserved JSON must have been written via the node's set_* MCP
// tool. The rendered markdown and raw JSON are exposed as outputs so downstream
// references (human_gate bodies, the UI) render the real content.
func (e *Engine) execStructuredAgent(c *execCtx, node *models.Node, artifactName, outKey string, render func(string) string) nodeOutcome {
	req := e.nodeReq(c, node)
	res, err := e.provider.RunAgent(context.Background(), req)
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(),
			outputMd: node.Label + " 执行失败:" + err.Error(), retryable: true, events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}
	return e.withOutcome(c, node, res, func(r runtime.NodeResult) nodeOutcome {
		return e.finalizeStructured(c, node, r, artifactName, outKey, render)
	})
}

// finalizeStructured enforces that a node wrote its reserved structured
// product and lifts it into the node outputs (rendered markdown at outKey, raw
// JSON at outKey+"_json"). A missing artifact yields a failed outcome.
func (e *Engine) finalizeStructured(c *execCtx, node *models.Node, res runtime.NodeResult, artifactName, outKey string, render func(string) string) nodeOutcome {
	content, ok := e.store.Get(c.run.ID, artifactName)
	if !ok {
		return nodeOutcome{status: "failed", err: "结构化产物契约未满足:" + artifactName,
			outputMd: "结构化产物契约未满足:未写入 " + artifactName, events: res.Events}
	}
	outputs := res.Outputs
	if outputs == nil {
		outputs = map[string]any{}
	}
	rendered := render(content)
	outputs[outKey] = rendered
	outputs[outKey+"_json"] = content
	e.setRunBranch(c, res.Git)
	return nodeOutcome{status: "completed", outputMd: res.OutputMd, outputs: outputs, events: res.Events}
}

// finalizeProducts enforces required reserved artifacts then lifts required and
// any present optional products into node outputs. Only artifacts last written
// by this node count — a same-named upstream product (e.g. plan.json from a
// prior plan node) must not satisfy Approve's contract or be re-attributed.
func (e *Engine) finalizeProducts(c *execCtx, node *models.Node, res runtime.NodeResult, required, optional []nodereg.ProductRef) nodeOutcome {
	outputs := res.Outputs
	if outputs == nil {
		outputs = map[string]any{}
	}
	for _, p := range required {
		content, ok := e.artifactOwnedByNode(c.run.ID, node.ID, p.ArtifactName)
		if !ok {
			return nodeOutcome{status: "failed", err: "结构化产物契约未满足:" + p.ArtifactName,
				outputMd: "结构化产物契约未满足:未由本节点写入 " + p.ArtifactName, events: res.Events}
		}
		e.liftProduct(c, node, outputs, p, content)
	}
	for _, p := range optional {
		content, ok := e.artifactOwnedByNode(c.run.ID, node.ID, p.ArtifactName)
		if !ok {
			continue
		}
		e.liftProduct(c, node, outputs, p, content)
	}
	e.setRunBranch(c, res.Git)
	return nodeOutcome{status: "completed", outputMd: res.OutputMd, outputs: outputs, events: res.Events}
}

// artifactOwnedByNode returns content only when the named artifact exists and
// its last writer (store List Node) is nodeID. Upstream leftovers are ignored.
func (e *Engine) artifactOwnedByNode(runID, nodeID, name string) (string, bool) {
	content, ok := e.store.Get(runID, name)
	if !ok {
		return "", false
	}
	owner := ""
	found := false
	for _, info := range e.store.List(runID) {
		if info.Name != name {
			continue
		}
		owner = info.Node
		found = true
		break
	}
	if !found || owner != nodeID {
		return "", false
	}
	return content, true
}

func (e *Engine) liftProduct(c *execCtx, node *models.Node, outputs map[string]any, p nodereg.ProductRef, content string) {
	if p.ArtifactName == visualPageName {
		if _, serr := e.store.Save(c.run.ID, node.ID, visualPageName, "html", content); serr != nil {
			log.Warn().Err(serr).Str("node", node.ID).Msg("approve visual page re-save failed")
		}
		if _, serr := e.store.Save(c.run.ID, node.ID, visualNodePageName(node.ID), "html", content); serr != nil {
			log.Warn().Err(serr).Str("node", node.ID).Msg("approve visual node-scoped page save failed")
		}
		outputs[p.OutputKey] = content
		return
	}
	if render := nodereg.Renderer(p.Render); render != nil {
		outputs[p.OutputKey] = render(content)
		outputs[p.OutputKey+"_json"] = content
		return
	}
	outputs[p.OutputKey] = content
}

// applyStructuredGate turns a framework node into a quality gate driven by its
// structured product's conclusion. It runs only after the node already
// completed and wrote its artifact (so the rendered report stays visible); when
// the conclusion is negative or the artifact is malformed it flips the outcome
// to failed so the FSM routes a failure/rollback edge (e.g. review reject →
// back to implement) — or fails the run when no such edge exists. A passing
// conclusion (or a missing artifact) leaves the outcome untouched.
func (e *Engine) applyStructuredGate(c *execCtx, oc nodeOutcome, artifactName string, gate func(string) (bool, string)) nodeOutcome {
	if oc.status != "completed" {
		return oc
	}
	content, ok := e.store.Get(c.run.ID, artifactName)
	if !ok {
		return oc
	}
	if pass, reason := gate(content); !pass {
		oc.status = "failed"
		oc.err = reason
		if strings.TrimSpace(oc.outputMd) != "" {
			oc.outputMd += "\n\n---\n**门禁未通过**:" + reason
		} else {
			oc.outputMd = "门禁未通过:" + reason
		}
	}
	return oc
}

// finalizeStructuredGate runs after applyStructuredGate: it persists the
// configurable reason variable and, when the node declares config.exits,
// injects a temporary action plus per-exit goto for structured routing.
// action is never persisted to vars — only outcome.outputs for when guards.
func (e *Engine) finalizeStructuredGate(c *execCtx, node *models.Node, oc nodeOutcome, gateKind nodereg.GateKind) nodeOutcome {
	if oc.outputs == nil {
		oc.outputs = map[string]any{}
	}
	reasonVar := firstNonEmptyStr(str(node.Config["reason_var"]), "reason")
	var reasonText string
	switch oc.status {
	case "completed":
		switch gateKind {
		case nodereg.GateTest:
			reasonText = "测试全部通过"
		case nodereg.GateReview:
			reasonText = "评审已通过"
		}
	case "failed":
		reasonText = oc.err
	}
	if reasonText != "" {
		c.setVar(reasonVar, reasonText)
		e.persistVar(c.run.ID, reasonVar, reasonText)
	}
	if !hasStructuredExits(node) {
		return oc
	}
	var action string
	if oc.status == "completed" {
		action = "pass"
	} else if gateKind == nodereg.GateReview {
		action = "reject"
	} else {
		action = "fail"
	}
	oc.outputs["action"] = action
	if goto_ := structuredExitGoto(node, oc.status == "completed"); goto_ != "" {
		oc.goto_ = goto_
	}
	return oc
}

func hasStructuredExits(node *models.Node) bool {
	_, ok := node.Config["exits"]
	return ok
}

func structuredExitGoto(node *models.Node, pass bool) string {
	exits, ok := node.Config["exits"].(map[string]any)
	if !ok {
		return ""
	}
	key := "fail"
	if pass {
		key = "pass"
	}
	side, ok := exits[key].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str(side["goto"]))
}

// testGate blocks the flow when any test case failed or test_result.json is
// malformed. When blockOnSkipped is true, skipped cases also fail the gate.
// When planJSON has leaves, plan_coverage must fully cover them with passed
// evidence (AND with the cases gate). Empty/leaf-less plan fail-opens.
func testGate(content string, blockOnSkipped bool, planJSON string) (bool, string) {
	n := mcp.TestFailedCount(content)
	if n < 0 {
		return false, "测试结果解析失败:无法读取 test_result.json"
	}
	if n > 0 {
		return false, fmt.Sprintf("测试未通过:%d 个用例失败,需修复后重新测试", n)
	}
	if blockOnSkipped {
		skipped := mcp.TestSkippedCount(content)
		if skipped < 0 {
			return false, "测试结果解析失败:无法读取 test_result.json"
		}
		if skipped > 0 {
			return false, fmt.Sprintf("测试未通过:%d 个用例被跳过,需修复后重新测试", skipped)
		}
	}
	if ok, reason := mcp.PlanCoverageOK(content, planJSON); !ok {
		return false, reason
	}
	return true, ""
}

// reviewGate blocks the flow when the review verdict is request_changes or
// reject, or when review.json is malformed. approve / approve_with_comments
// pass.
func reviewGate(content string) (bool, string) {
	verdict, ok := mcp.ReviewVerdictOK(content)
	if !ok {
		return false, "评审结果无效:无法读取 review.json 或 verdict 不合法"
	}
	switch verdict {
	case "request_changes":
		return false, "评审结论为 request_changes:需按评审意见修改后重新评审"
	case "reject":
		return false, "评审结论为 reject:方案/实现被否决,需整改后重新评审"
	default:
		return true, ""
	}
}
