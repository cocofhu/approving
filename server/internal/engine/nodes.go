package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/blob"
	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	gatenode "github.com/cocofhu/approving/internal/models/nodereg"
	"github.com/cocofhu/approving/internal/nodereg"
	"github.com/cocofhu/approving/internal/runtime"
	"github.com/cocofhu/approving/internal/services"

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
	// After platform DefaultChecks (including structured gates), optional RPC.
	if agentExecNeedsOutcome(spec.Exec) {
		oc = e.afterDefaultChecks(c, node, oc)
	}
	// Post-run ReAct review phase: when the product contract passed and the
	// node's review control variable requests interaction, seed a review
	// conversation and pause (the live session was already parked by the
	// provider). Skips are zero-overhead — behavior is identical to today.
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
		// No store access here; plan_coverage fail-opens when planJSON is empty.
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

// execOutput is implemented in output.go (multi-card structured output).

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
			v = expr // fall back to literal
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

func (e *Engine) execAgent(c *execCtx, node *models.Node) nodeOutcome {
	req := e.nodeReq(c, node)
	res, err := e.provider.RunAgent(context.Background(), req)
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(),
			outputMd: "Agent 执行失败:" + err.Error(), retryable: true, events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}
	// Drop before withOutcome so a force-cleared zombie cannot TakeOutcome.
	if c.execGen != 0 && !e.isExecOwner(c.run.ID, c.execGen) {
		return nodeOutcome{
			status:   "cancelled",
			err:      "lost exec ownership",
			outputMd: "dropped late outcome: lost exec ownership",
			events:   res.Events,
			usage: res.Usage, usageByModel: res.UsageByModel,
		}
	}
	return e.withOutcome(c, node, res, func(r runtime.NodeResult) nodeOutcome {
		return e.completeProduces(c, node, r)
	})
}

// visualPageName is the reserved single-file deliverable of a visual node.
const visualPageName = "page.html"

// execVisual runs a visual node: an agent that must produce a single
// self-contained HTML page (page.html) via the artifact-store MCP only (never a
// workspace file, so the repo stays clean). It enforces the artifact's
// existence and re-saves it with kind "html" so the run UI previews it in an
// iframe (a page written via write_artifact would default to markdown).
func (e *Engine) execVisual(c *execCtx, node *models.Node) nodeOutcome {
	req := e.nodeReq(c, node)
	res, err := e.provider.RunAgent(context.Background(), req)
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "视觉网页节点执行失败:" + err.Error(), events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
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
	// Guarantee the artifact kind is html so the UI previews it in an iframe.
	if _, serr := e.store.Save(c.run.ID, node.ID, visualPageName, "html", content); serr != nil {
		log.Warn().Err(serr).Str("node", node.ID).Msg("visual page re-save failed")
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
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "计划节点执行失败:" + err.Error(), events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
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
	// Expose the plan as the node's `plan` output (declared in the node
	// registry): a readable markdown checklist so downstream references like
	// {{nodes.<id>.outputs.plan}} — e.g. a human_gate body — render the actual
	// plan for review instead of a blank/opaque value.
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

// setRunBranch records the run's working branch in both the in-memory execCtx
// and its own DB column. Keeping c.run.Branch in sync lets later reads of the
// context see it; the branch is persisted via a scoped column update. An empty
// branch is ignored so it never wipes a branch a prior node already recorded.
func (e *Engine) setRunBranch(c *execCtx, git *runtime.GitInfo) {
	if git == nil || strings.TrimSpace(git.Branch) == "" {
		return
	}
	c.run.Branch = git.Branch
	logDB(e.db.Model(&models.Run{}).Where("id = ?", c.run.ID).UpdateColumn("branch", git.Branch), c.run.ID, "set run branch")
}

// execSubmitMR runs the submit_mr node: an agent (LLM) node whose contract is
// to resolve conflicts against the target branch, push the source branch, and
// open a merge request, then mark completion via node_complete. Platform
// DefaultChecks require the outcome mark only — they do NOT verify git push /
// MR existence / conflicts (those are agent-attested; optional business RPC
// may validate later). On success, outputs.mr_url is exported as the global
// `mr_url` variable for downstream references (e.g. a human_gate body).
//
// config.repo supports three modes (see submit_mr_interp.go):
//   - blank: keep legacy Agent-guided / single-repo fallback (no engine loop)
//   - single: strict-interpolate → ∈repos check → one RunAgent
//   - list ({{vars.repos}}): engine iterates vars.repos by name, fail-fast
func (e *Engine) execSubmitMR(c *execCtx, node *models.Node) nodeOutcome {
	rawRepo := strings.TrimSpace(str(node.Config["repo"]))
	rawSrc := strings.TrimSpace(str(node.Config["source_branch"]))
	rawTgt := strings.TrimSpace(str(node.Config["target_branch"]))

	repoR := e.strictInterpolate(c, rawRepo)
	mode := detectSubmitMRRepoMode(rawRepo, repoR)

	sourceBranch, err := e.resolveStrictBranchField(c, rawSrc, "源分支")
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "提交 MR 失败:" + err.Error()}
	}
	targetBranch, err := e.resolveStrictBranchField(c, rawTgt, "目标分支")
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "提交 MR 失败:" + err.Error()}
	}

	switch mode {
	case submitMRModeBlank:
		// Legacy path: do not enter the engine per-repo loop.
		return e.runSubmitMROnce(c, node, "", sourceBranch, targetBranch)

	case submitMRModeList:
		if !repoR.ok {
			return nodeOutcome{status: "failed", err: repoR.err, outputMd: "提交 MR 失败:" + repoR.err}
		}
		repos := runtime.ResolveReposFromVars(c.vars)
		if len(repos) == 0 {
			msg := "vars.repos 列表为空或无法解析"
			return nodeOutcome{status: "failed", err: msg, outputMd: "提交 MR 失败:" + msg}
		}
		var (
			lastURL   string
			lastOut   map[string]any
			allEvents []models.AcpEvent
			summaries []string
			lastGit   *runtime.GitInfo
		)
		for _, r := range repos {
			oc, git := e.runSubmitMROnceWithGit(c, node, r.Name, sourceBranch, targetBranch)
			if oc.events != nil {
				allEvents = append(allEvents, oc.events...)
			}
			if oc.status == "failed" {
				errMsg := fmt.Sprintf("仓 %s: %s", r.Name, oc.err)
				if oc.err == "" {
					errMsg = fmt.Sprintf("仓 %s: 提交 MR 失败", r.Name)
				}
				return nodeOutcome{
					status:   "failed",
					err:      errMsg,
					outputMd: "提交 MR 失败:" + errMsg,
					outputs:  oc.outputs,
					events:   allEvents,
				}
			}
			lastOut = oc.outputs
			lastGit = git
			lastURL = strings.TrimSpace(str(oc.outputs["mr_url"]))
			summaries = append(summaries, fmt.Sprintf("- %s: %s", r.Name, lastURL))
		}
		if lastGit != nil {
			e.setRunBranch(c, lastGit)
		}
		c.setVar("mr_url", lastURL)
		e.persistVar(c.run.ID, "mr_url", lastURL)
		md := "已按 vars.repos 逐仓提交 MR:\n" + strings.Join(summaries, "\n")
		return nodeOutcome{status: "completed", outputMd: md, outputs: lastOut, events: allEvents}

	default: // single
		if !repoR.ok {
			return nodeOutcome{status: "failed", err: repoR.err, outputMd: "提交 MR 失败:" + repoR.err}
		}
		repoName := strings.TrimSpace(repoR.value)
		if repoName == "" || repoName == reposListSentinel {
			msg := "目标仓:原配置非空但插值结果为空"
			return nodeOutcome{status: "failed", err: msg, outputMd: "提交 MR 失败:" + msg}
		}
		if !repoNameInVars(repoName, c.vars) {
			msg := fmt.Sprintf("仓名 %q 不在 vars.repos 中", repoName)
			return nodeOutcome{status: "failed", err: msg, outputMd: "提交 MR 失败:" + msg}
		}
		return e.runSubmitMROnce(c, node, repoName, sourceBranch, targetBranch)
	}
}

// runSubmitMROnce pins repo/source/target on the node request, runs the agent,
// and requires node_complete (no platform verifyMR gate).
func (e *Engine) runSubmitMROnce(c *execCtx, node *models.Node, repo, sourceBranch, targetBranch string) nodeOutcome {
	oc, git := e.runSubmitMROnceWithGit(c, node, repo, sourceBranch, targetBranch)
	if oc.status == "completed" {
		e.setRunBranch(c, git)
		mrURL := strings.TrimSpace(str(oc.outputs["mr_url"]))
		c.setVar("mr_url", mrURL)
		e.persistVar(c.run.ID, "mr_url", mrURL)
	}
	return oc
}

func (e *Engine) runSubmitMROnceWithGit(c *execCtx, node *models.Node, repo, sourceBranch, targetBranch string) (nodeOutcome, *runtime.GitInfo) {
	req := e.nodeReq(c, node)
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	req.Config["repo"] = repo
	req.Config["source_branch"] = sourceBranch
	req.Config["target_branch"] = targetBranch

	res, err := e.provider.RunAgent(context.Background(), req)
	if err != nil {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "提交 MR 失败:" + err.Error()}, nil
	}
	oc := e.withOutcome(c, node, res, func(r runtime.NodeResult) nodeOutcome {
		outputs := r.Outputs
		if outputs == nil {
			outputs = map[string]any{}
		}
		mrURL := strings.TrimSpace(str(outputs["mr_url"]))
		md := "已提交 MR"
		if mrURL != "" {
			md = "已提交 MR:" + mrURL
		} else if sum := strings.TrimSpace(str(outputs["outcome_summary"])); sum != "" {
			md = sum
		}
		return nodeOutcome{status: "completed", outputMd: md, outputs: outputs, events: r.Events}
	})
	return oc, res.Git
}

// execProposalSelect resolves a single final proposal from the upstream
// proposals.json. When the configured auto var is truthy it auto-selects the
// recommended option and continues; otherwise it pauses on a human_gate whose
// actions are the proposals, and ResumeGate finalizes the choice.
func (e *Engine) execProposalSelect(c *execCtx, node *models.Node) nodeOutcome {
	from := firstNonEmptyStr(str(node.Config["from"]), mcp.ProposalsArtifactName)
	content, ok := e.store.Get(c.run.ID, from)
	if !ok {
		return nodeOutcome{status: "failed", err: "未找到上游方案 " + from,
			outputMd: "方案确认失败:未找到上游方案 " + from}
	}
	autoVar := firstNonEmptyStr(str(node.Config["auto_var"]), "auto_confirm")
	outVar := firstNonEmptyStr(str(node.Config["output_var"]), "selected_proposal")
	if truthy(c.vars[autoVar]) {
		final, id, ok := mcp.SelectProposal(content, "")
		if !ok {
			return nodeOutcome{status: "failed", err: "方案解析失败", outputMd: "方案确认失败:方案解析失败"}
		}
		oc := e.finalizeProposal(c, node, final, id, outVar)
		// Align with ResumeGate: once the choice is final, release the upstream
		// ProposalAgent park kept alive for potential ReAct reject.
		if oc.status == "completed" {
			e.retireGateUpstreamSession(c, node)
		}
		return oc
	}
	// Human selection: reuse the gate pause/resume machinery (per-visit, so a
	// loop-back re-opens the choice — see execGate).
	iter := c.iter[node.ID]
	var gate models.Gate
	err := e.db.Where("run_id = ? AND node_id = ? AND iteration = ?", c.run.ID, node.ID, iter).First(&gate).Error
	if err == nil && gate.Resolved {
		return nodeOutcome{status: "completed", outputMd: "方案已选择", outputs: map[string]any{"resolved": true}}
	}
	if err != nil {
		var actions []models.GateAction
		for _, ch := range mcp.ProposalChoices(content) {
			actions = append(actions, models.GateAction{ID: ch.ID, Label: ch.Title})
		}
		gate = models.Gate{RunID: c.run.ID, NodeID: node.ID, Iteration: iter, WorkflowID: c.run.WorkflowID, WorkflowName: c.run.WorkflowName,
			Title:       firstNonEmptyStr(str(node.Config["title"]), "选择方案"),
			BodyMd:      mcp.RenderProposalsMarkdown(content),
			Actions:     actions,
			RequestedAt: time.Now()}
		logDB(e.db.Create(&gate), c.run.ID, "create proposal_select gate")
	}
	return nodeOutcome{status: "paused", outputMd: "等待人工选择方案…"}
}

// finalizeProposal writes the chosen proposal as proposal.json, assigns the
// selected id to the output variable, and returns a completed outcome.
func (e *Engine) finalizeProposal(c *execCtx, node *models.Node, finalJSON, id, outVar string) nodeOutcome {
	if _, err := e.store.Save(c.run.ID, node.ID, mcp.ProposalArtifactName, "json", finalJSON); err != nil {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "写入最终方案失败:" + err.Error()}
	}
	c.setVar(outVar, id)
	e.persistVar(c.run.ID, outVar, id)
	outputs := map[string]any{
		"proposal":          mcp.RenderProposalMarkdown(finalJSON),
		"proposal_json":     finalJSON,
		"selected_proposal": id,
		outVar:              id,
	}
	return nodeOutcome{status: "completed", outputMd: "已选定方案 " + id, outputs: outputs}
}

// exportBranchVar publishes an implement node's per-repo working branches to
// the global variable `branches` (JSON name→branch map) so downstream nodes
// can consume it — both in templates ({{vars.branches}}) and, crucially, to
// check each repo out in their fresh sandbox clones (resolveRepos injects
// the branch via GIT_REPOS). Without this a downstream node clones the
// default branch and never sees the implementation.
func (e *Engine) exportBranchVar(c *execCtx, outputs map[string]any) {
	br := strings.TrimSpace(str(outputs["branches"]))
	if br == "" {
		return
	}
	c.setVar("branches", br)
	e.persistVar(c.run.ID, "branches", br)
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// completeProduces finalizes a successful agent/react node: it enforces the
// produces contract when one is declared, auto-captures the deliverable when
// one is not, and records any branch the node pushed. A declared-but-missing
// produces artifact yields a failed outcome — this is a last-resort guard; the
// react provider already re-prompts the agent to write it before finishing.
// When no produces is declared the check is skipped entirely and the node just
// flows through.
func (e *Engine) completeProduces(c *execCtx, node *models.Node, res runtime.NodeResult) nodeOutcome {
	// Framework nodes with a dedicated structured deliverable enforce their
	// reserved JSON (written via the node's set_* MCP tool) instead of the
	// generic produces contract.
	if spec, ok := nodereg.Get(node.Type); ok && spec.Render != nodereg.RenderNone {
		oc := e.finalizeStructured(c, node, res, spec.ArtifactName, spec.OutputKey, nodereg.Renderer(spec.Render))
		if spec.Type == "implement" && oc.status == "completed" {
			e.exportBranchVar(c, oc.outputs)
		}
		return oc
	}
	if produces := str(node.Config["produces"]); produces != "" {
		if _, ok := e.store.Get(c.run.ID, produces); !ok {
			return nodeOutcome{status: "failed", err: "produces contract not satisfied: " + produces,
				outputMd: "产物契约未满足:" + produces, events: res.Events}
		}
	}
	e.captureDeliverable(c, node, res)
	e.setRunBranch(c, res.Git)
	return nodeOutcome{status: "completed", outputMd: res.OutputMd, outputs: res.Outputs, events: res.Events}
}

func (e *Engine) execReactEnter(c *execCtx, node *models.Node) nodeOutcome {
	// Scope the conversation to THIS visit's execution index: a loop-back onto
	// the same react node opens a fresh dialogue instead of passing through the
	// previous (done) one. Only a conversation done *for this iteration* passes
	// through (defensive; the resume path continues from the next node).
	iter := c.iter[node.ID]
	var conv models.ReactConversation
	err := e.db.Where("run_id = ? AND node_id = ? AND iteration = ?", c.run.ID, node.ID, iter).First(&conv).Error
	if err == nil && conv.Done {
		return nodeOutcome{status: "completed", outputMd: "澄清已完成"}
	}
	if err != nil {
		// First entry: open the dialogue. The opening turn may already conclude
		// the clarification (agent asked nothing) — then finish the node now.
		req := e.nodeReq(c, node)
		t := e.provider.ReactOpen(context.Background(), req)
		if t.SetupErr != nil {
			fullErr := ""
			if t.SetupErr != nil {
				fullErr = t.SetupErr.Error()
			}
			if fullErr == "" {
				fullErr = t.Msg
			}
			return nodeOutcome{
				status: "failed", err: fullErr, outputMd: "沙箱启动失败",
				events: t.Events, usage: t.Usage, usageByModel: t.UsageByModel, sandboxSetup: true,
			}
		}
		conv = models.ReactConversation{RunID: c.run.ID, NodeID: node.ID, Iteration: iter, Done: t.Done,
			Messages: []models.ReactMessage{{Role: "agent", Text: t.Msg,
				At: time.Now().Format(time.RFC3339), Questions: t.Questions}}}
		logDB(e.db.Create(&conv), c.run.ID, "create react conversation")
		// Auto-clarify: when the node's auto var is truthy, drive the dialogue
		// without pausing for a human — each round is answered with the
		// recommended option set (all recommended for multi-select, or the first
		// option as fallback) until the agent concludes or the round cap is hit.
		if !t.Done && len(t.Questions) > 0 && e.autoReactEnabled(c, node) {
			t = e.autoAdvanceReact(c, node, &conv, req, t)
		}
		if t.Done {
			if t.Err != nil {
				return nodeOutcome{status: "failed", err: t.Err.Error(), outputMd: t.Msg, events: t.Events, usage: t.Usage, usageByModel: t.UsageByModel}
			}
			return e.finishAgentOutcome(c, node, t.Result, func(r runtime.NodeResult) nodeOutcome {
				return e.completeProduces(c, node, r)
			})
		}
		return nodeOutcome{status: "paused", outputMd: "等待人工回复(ReAct 澄清)…", events: t.Events, usage: t.Usage, usageByModel: t.UsageByModel}
	}
	return nodeOutcome{status: "paused", outputMd: "等待人工回复(ReAct 澄清)…"}
}

// autoReactEnabled reports whether this react node should self-answer without
// waiting for a human, i.e. its configured auto var resolves truthy. The var
// name is optional; when unset the node is always interactive.
func (e *Engine) autoReactEnabled(c *execCtx, node *models.Node) bool {
	autoVar := strings.TrimSpace(str(node.Config["auto_var"]))
	if autoVar == "" {
		return false
	}
	return truthy(c.vars[autoVar])
}

// autoAdvanceReact drives a react dialogue to completion without human input:
// every round it replies with FormatChoiceReply (recommended option set —
// multi-select may pick several, joined with "、" — or the first as fallback)
// and appends both turns to conv, stopping when the agent concludes (Done),
// stops asking questions, or the max_rounds cap is reached.
// It persists conv as it goes so the transcript reflects every auto round, and
// returns the final ReactTurn for the caller to finalize (Done) or pause on.
func (e *Engine) autoAdvanceReact(c *execCtx, node *models.Node, conv *models.ReactConversation, req runtime.NodeReq, t runtime.ReactTurn) runtime.ReactTurn {
	// Keep every auto round's token delta (plus the seed turn) so a single
	// subsequent saveState/flush does not drop earlier usage.
	acc := models.CloneTokenUsage(t.Usage)
	accBy := models.CloneTokenUsageByModel(t.UsageByModel)
	for !t.Done && len(t.Questions) > 0 {
		if runtime.ReactCapReached(req, conv.Messages) {
			break
		}
		humanText := models.FormatChoiceReply(t.Questions)
		if humanText == "" {
			break
		}
		conv.Messages = append(conv.Messages, models.ReactMessage{Role: "human", Text: humanText,
			At: time.Now().Format(time.RFC3339)})
		t = e.provider.ReactReply(context.Background(), req, conv.Messages, humanText, nil, false)
		acc = models.AddTokenUsage(acc, t.Usage)
		accBy = models.AddTokenUsageByModel(accBy, t.UsageByModel)
		conv.Messages = append(conv.Messages, models.ReactMessage{Role: "agent", Text: t.Msg,
			At: time.Now().Format(time.RFC3339), Questions: t.Questions})
		conv.Done = t.Done
		logDB(e.db.Save(conv), c.run.ID, "auto react round")
	}
	t.Usage = acc
	t.UsageByModel = accBy
	if t.Done {
		t.Result.Usage = models.CloneTokenUsage(acc)
		t.Result.UsageByModel = models.CloneTokenUsageByModel(accBy)
	}
	return t
}

// execAppPreview runs an agent to build/start the app and register preview ports.
// A healthy set_preview ends the production phase (no node_complete required) and
// enters pure ReAct review (no Gate / Inbox row). Confirm finishes via
// finalizeAppPreview + routeSuccess with an internal action=pass for legacy edges.
func (e *Engine) execAppPreview(c *execCtx, node *models.Node) nodeOutcome {
	req := e.nodeReq(c, node)
	e.host.ResetPreviewReady(c.run.ID, node.ID)
	res, err := e.provider.RunAgent(context.Background(), req)
	// ACP/WS 断连兜底:已有可达预览则仍进入复审,禁止无限 busy。
	if err != nil && !e.host.HasHealthyPreviewPorts(c.run.ID, node.ID) {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "应用预览执行失败:" + err.Error(), events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}
	if c.execGen != 0 && !e.isExecOwner(c.run.ID, c.execGen) {
		return nodeOutcome{
			status:   "cancelled",
			err:      "lost exec ownership",
			outputMd: "dropped late outcome: lost exec ownership",
			events:   res.Events,
			usage: res.Usage, usageByModel: res.UsageByModel,
		}
	}
	if !e.host.HasHealthyPreviewPorts(c.run.ID, node.ID) {
		return nodeOutcome{status: "failed", err: "预览契约未满足:未成功 set_preview(可达)",
			outputMd: "应用预览失败:未成功注册可达预览端口", events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}
	// Soft-consume node_complete when present; never fail closed on its absence.
	if o, ok := e.host.TakeOutcome(c.run.ID, node.ID); ok {
		res.Outputs = mcp.MergeOutcomeOutputs(res.Outputs, o)
		if o.Status == mcp.OutcomeFailed {
			errMsg := strings.TrimSpace(o.Error)
			if errMsg == "" {
				errMsg = strings.TrimSpace(o.Summary)
			}
			if errMsg == "" {
				errMsg = "agent reported failure"
			}
			return nodeOutcome{status: "failed", err: errMsg, outputMd: "节点失败:" + errMsg,
				outputs: res.Outputs, events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
		}
	}
	// Idempotent re-entry: if this visit's review conversation already concluded,
	// treat the node as completed (e.g. late RunAgent after confirm).
	iter := c.iter[node.ID]
	var conv models.ReactConversation
	if err := e.db.Where("run_id = ? AND node_id = ? AND iteration = ?", c.run.ID, node.ID, iter).First(&conv).Error; err == nil && conv.Done {
		return nodeOutcome{status: "completed", outputMd: "预览复审已完成",
			outputs: map[string]any{"resolved": true, "preview_ready": true}, events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}
	paused := nodeOutcome{status: "paused", outputMd: "等待人工预览复审…", events: res.Events, outputs: res.Outputs, usage: res.Usage, usageByModel: res.UsageByModel}
	return e.enterReview(c, node, paused)
}

func (e *Engine) execGate(c *execCtx, node *models.Node) nodeOutcome {
	// Look up the gate for THIS visit's execution index. A loop-back re-enters
	// with a higher iteration, so a prior resolved gate never short-circuits the
	// new visit — the gate re-opens and is re-approvable. Only a gate already
	// resolved *for this same iteration* passes through (defensive: the resume
	// path normally continues from the next node, not back into the gate).
	iter := c.iter[node.ID]
	var gate models.Gate
	err := e.db.Where("run_id = ? AND node_id = ? AND iteration = ?", c.run.ID, node.ID, iter).First(&gate).Error
	if err == nil && gate.Resolved {
		return nodeOutcome{status: "completed", outputMd: "审批已完成", outputs: map[string]any{"resolved": true}}
	}
	if err != nil {
		upNodeID, upIter := e.resolveGateUpstreamPointer(c, node)
		gate = models.Gate{RunID: c.run.ID, NodeID: node.ID, Iteration: iter, WorkflowID: c.run.WorkflowID, WorkflowName: c.run.WorkflowName,
			Title:             str(node.Config["title"]),
			BodyMd:            e.interpolate(c, str(node.Config["body_template"])),
			Actions:           parseActions(node.Config["actions"]),
			Form:              parseForm(node.Config["form"]),
			UpstreamNodeID:    upNodeID,
			UpstreamIteration: upIter,
			RequestedAt:       time.Now()}
		logDB(e.db.Create(&gate), c.run.ID, "create gate")
		if e.shareRevoker != nil {
			e.shareRevoker.RevokeUnusedForNode(c.run.ID, node.ID)
		}
	}
	return nodeOutcome{status: "paused", outputMd: "等待人工门禁审批…"}
}

// resolveGateUpstreamPointer picks the main upstream node (page preferred) and
// the latest completed iteration for that node at gate-create time. Returns
// empty values when the template has no upstream refs or no iteration is known.
func (e *Engine) resolveGateUpstreamPointer(c *execCtx, node *models.Node) (string, int) {
	upNodeID := gatenode.GatePrimaryUpstreamNodeID(node)
	if upNodeID == "" {
		return "", 0
	}
	var sr models.StateRun
	err := e.db.Where("run_id = ? AND node_id = ? AND status = ?", c.run.ID, upNodeID, "completed").
		Order("iteration desc").First(&sr).Error
	if err == nil && sr.Iteration > 0 {
		return upNodeID, sr.Iteration
	}
	if it := c.iter[upNodeID]; it > 0 {
		return upNodeID, it
	}
	return "", 0
}

// --- helpers --------------------------------------------------------------

// captureDeliverable persists an agent/react node's primary textual output as a
// run artifact when the node does not declare an explicit produces contract and
// has not already written one via the artifact-store MCP. This guarantees that
// a node which produced a document is reflected in the run's artifacts even if
// the author did not wire up produces / write_artifact.
func (e *Engine) captureDeliverable(c *execCtx, node *models.Node, res runtime.NodeResult) {
	if str(node.Config["produces"]) != "" {
		return
	}
	content := pickDeliverable(res)
	if strings.TrimSpace(content) == "" {
		return
	}
	for _, a := range e.store.List(c.run.ID) {
		if a.Node == node.ID && a.Name != mcp.NodeOutcomeArtifactName {
			return // node already produced a deliverable artifact; don't duplicate
		}
	}
	if _, err := e.store.Save(c.run.ID, node.ID, node.ID+".md", "markdown", content); err != nil {
		log.Warn().Err(err).Str("node", node.ID).Msg("auto-capture deliverable failed")
	}
}

// pickDeliverable selects the node's primary human-facing output.
func pickDeliverable(res runtime.NodeResult) string {
	for _, k := range []string{"clarified_requirement", "content"} {
		if s, ok := res.Outputs[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return res.OutputMd
}

// checkSkillProfileProject enforces same-project Agent usage before any
// agent-class execution. Empty skill_profile is allowed and skipped.
func (e *Engine) checkSkillProfileProject(c *execCtx, node *models.Node) error {
	if e == nil || node == nil || node.Config == nil {
		return nil
	}
	raw, _ := node.Config["skill_profile"].(string)
	profile := strings.TrimSpace(raw)
	if profile == "" {
		return nil
	}
	label := strings.TrimSpace(node.Label)
	if label == "" {
		label = node.ID
	}
	// Gate is opt-in: unit tests and hosts without SkillService skip.
	if e.skills == nil {
		return nil
	}
	ag, ok := e.skills.Get(profile)
	if !ok {
		return fmt.Errorf("节点「%s」的 Agent「%s」不可用：已删除", label, profile)
	}
	projectID := services.ResolveProjectIDForRun(e.db, c.run.ID)
	if strings.TrimSpace(ag.ProjectID) == "" {
		return fmt.Errorf("节点「%s」的 Agent「%s」不可用：未绑定", label, profile)
	}
	if !services.AgentProjectMatches(ag, projectID) {
		return fmt.Errorf("节点「%s」的 Agent「%s」不可用：非本项目", label, profile)
	}
	return nil
}

func (e *Engine) nodeReq(c *execCtx, node *models.Node) runtime.NodeReq {
	cfg := map[string]any{}
	for k, v := range node.Config {
		cfg[k] = v
	}
	if p, ok := cfg["prompt"].(string); ok {
		cfg["prompt"] = e.interpolate(c, p)
	}
	// Interpolate the conditional-injection text up front so {{vars.x}} refs in
	// it resolve; the provider decides whether to append it based on when_var.
	if cp, ok := cfg["conditional_prompt"].(map[string]any); ok {
		merged := map[string]any{}
		for k, v := range cp {
			merged[k] = v
		}
		if txt, ok := merged["text"].(string); ok {
			merged["text"] = e.interpolate(c, txt)
		}
		cfg["conditional_prompt"] = merged
	}
	// Attribute any artifact-store MCP writes during this node to this node,
	// and record its type so clarify-only tools (ask_question) can be gated.
	e.host.SetActiveNode(c.run.ID, node.ID, node.Type)
	// Drop a leftover node_complete from a prior failed attempt (RunAgent error
	// before withOutcome/TakeOutcome). Auto-retry and ResumeFrom must require a
	// fresh attestation for this visit.
	e.host.ClearOutcome(c.run.ID, node.ID)
	if node.Type == "app_preview" {
		e.host.ResetPreviewReady(c.run.ID, node.ID)
	}
	promptImages := collectPromptVarImages(c, promptScanTemplates(node.Config)...)
	req := runtime.NodeReq{RunID: c.run.ID, WorkflowID: c.run.WorkflowID, WorkflowName: c.run.WorkflowName,
		Token: c.token, NodeID: node.ID, NodeType: node.Type, Config: cfg, Vars: c.vars,
		PromptImages: promptImages}
	// Park the live session after a successful run when this node will enter an
	// inline ReAct review, or a downstream approval gate referencing its product
	// may issue a ReAct reject — either needs the same sandbox kept alive.
	// app_preview always parks (reviewEnabled always true).
	req.KeepAliveForReview = e.reviewEnabled(c, node) || e.hasDownstreamReactGate(c, node)
	return req
}

var varsRefRE = regexp.MustCompile(`\{\{\s*vars\.(\w+)\s*\}\}`)

// promptScanTemplates lists config fields that may contain {{vars.xxx}} refs and
// drive first-turn prompt image attachment collection.
func promptScanTemplates(cfg map[string]any) []string {
	var out []string
	if p, ok := cfg["prompt"].(string); ok {
		out = append(out, p)
	}
	if cp, ok := cfg["conditional_prompt"].(map[string]any); ok {
		if txt, ok := cp["text"].(string); ok {
			out = append(out, txt)
		}
	}
	if bt, ok := cfg["body_template"].(string); ok {
		out = append(out, bt)
	}
	return out
}

// collectPromptVarImages scans templates for {{vars.xxx}} refs and merges the
// referenced variables' images in first-seen order (deduped by var name).
func collectPromptVarImages(c *execCtx, templates ...string) []models.PromptImage {
	seen := map[string]bool{}
	var out []models.PromptImage
	for _, tmpl := range templates {
		if tmpl == "" {
			continue
		}
		for _, m := range varsRefRE.FindAllStringSubmatch(tmpl, -1) {
			name := m[1]
			if seen[name] {
				continue
			}
			seen[name] = true
			if v, ok := c.vars[name]; ok {
				out = append(out, models.ExtractImages(v)...)
			}
		}
	}
	return out
}

// interpolate resolves {{vars.x}} / {{nodes.x.outputs.y}} references in a
// template string. Unknown refs render empty.
func (e *Engine) interpolate(c *execCtx, tmpl string) string {
	if tmpl == "" {
		return ""
	}
	out := tmpl
	ec := e.evalContext(c, nil)
	for {
		i := strings.Index(out, "{{")
		if i < 0 {
			break
		}
		j := strings.Index(out[i:], "}}")
		if j < 0 {
			break
		}
		expr := strings.TrimSpace(out[i+2 : i+j])
		repl := ""
		// skip handlebars-style blocks like {{#if ...}}
		if !strings.HasPrefix(expr, "#") && !strings.HasPrefix(expr, "/") {
			if v, err := evalExpr(expr, ec); err == nil && v != nil {
				repl = models.VarDisplayText(v)
			}
		}
		out = out[:i] + repl + out[i+j+2:]
	}
	return out
}

func (e *Engine) saveState(c *execCtx, node *models.Node, o nodeOutcome) {
	status := o.status
	if status == "paused" {
		status = "waiting_human"
	}
	// Update the StateRun for THIS visit's execution index (opened by
	// startNodeRun on enter). Falling back to a create keeps resume paths robust
	// even if the running row is somehow missing. Never touch earlier iterations
	// so their output/events/duration stay traceable.
	iter := c.iter[node.ID]
	if iter < 1 {
		iter = 1
	}
	var sr models.StateRun
	now := time.Now()
	err := e.db.Where("run_id = ? AND node_id = ? AND iteration = ?", c.run.ID, node.ID, iter).First(&sr).Error
	if err != nil {
		sr = models.StateRun{RunID: c.run.ID, NodeID: node.ID, NodeType: node.Type, Iteration: iter, StartedAt: &now}
	}
	// A concurrent ResumeGate may have already finalized this visit between
	// gate creation and the pause saveState. Do not downgrade terminal status
	// back to waiting_human.
	if status == "waiting_human" &&
		(sr.Status == "completed" || sr.Status == "failed" || sr.Status == "cancelled") {
		return
	}
	// Cancel/fail finalizes in-flight StateRuns while RunAgent is still blocked.
	// When the late provider returns completed/running, refuse to revive the
	// visit — otherwise the UI shows a cancelled run whose implement node is
	// still "running" / flips back to completed and blocks ResumeFrom.
	if (sr.Status == "cancelled" || sr.Status == "failed") &&
		(status == "completed" || status == "running" || status == "waiting_human" || status == "paused") {
		return
	}
	sr.Status = status
	sr.OutputMd = o.outputMd
	if o.outputs != nil {
		sr.Outputs = o.outputs
	}
	// Snapshot the global variables as they stand after this execution, so the
	// timeline can surface each card's vars-at-that-moment for debugging.
	snap := map[string]any{}
	for k, v := range c.vars {
		snap[k] = blob.StripDataInValue(v)
	}
	sr.VarsSnapshot = snap
	// Accumulate this execution's built-in MCP tool-call trace so the timeline
	// card shows what the stage asked the platform to do (debugging). Append
	// (not replace) because a react node persists across several turns — the
	// opening turn saves on pause, later reply turns flush incrementally, and
	// completion appends the final turn; overwriting would drop earlier turns.
	if calls := e.host.TakeMcpCalls(c.run.ID, node.ID); len(calls) > 0 {
		sr.McpCalls = append(sr.McpCalls, calls...)
	}
	if len(o.events) > 0 {
		sr.Events = o.events
	}
	// Merge this save's token delta into the StateRun total. Deltas (not
	// cumulative snapshots) let react mid-turns and the final save accumulate
	// without double-counting prior pauses.
	if o.usage != nil {
		sr.Usage = models.AddTokenUsage(sr.Usage, o.usage)
	}
	if o.usageByModel != nil {
		sr.UsageByModel = models.AddTokenUsageByModel(sr.UsageByModel, o.usageByModel)
	}
	sr.Error = o.err
	sr.Attempt = c.run.Attempt
	if sr.StartedAt != nil {
		sr.DurationSec = int(now.Sub(*sr.StartedAt).Seconds())
	}
	logDB(e.db.Save(&sr), c.run.ID, "save state_run")
}

// flushMcpCalls appends the buffered built-in MCP tool-call trace onto a node's
// latest StateRun without ending the execution. Used between react turns (which
// pause without a saveState) so each round's tool calls land on the timeline as
// they happen rather than being stranded in the buffer until completion.
func (e *Engine) flushMcpCalls(runID, nodeID string) {
	calls := e.host.TakeMcpCalls(runID, nodeID)
	if len(calls) == 0 {
		return
	}
	var sr models.StateRun
	if err := e.db.Where("run_id = ? AND node_id = ?", runID, nodeID).
		Order("iteration desc, id desc").First(&sr).Error; err != nil {
		log.Warn().Str("run_id", runID).Str("node_id", nodeID).Err(err).Msg("flushMcpCalls: no state_run to attach calls to")
		return
	}
	sr.McpCalls = append(sr.McpCalls, calls...)
	logDB(e.db.Save(&sr), runID, "flush mcp calls")
}

// flushTokenUsage merges a react mid-turn token delta onto the latest StateRun
// without ending the execution (paired with flushMcpCalls while the node stays
// paused).
func (e *Engine) flushTokenUsage(runID, nodeID string, delta *models.TokenUsage, byModel models.TokenUsageByModel) {
	if delta == nil && byModel == nil {
		return
	}
	var sr models.StateRun
	if err := e.db.Where("run_id = ? AND node_id = ?", runID, nodeID).
		Order("iteration desc, id desc").First(&sr).Error; err != nil {
		log.Warn().Str("run_id", runID).Str("node_id", nodeID).Err(err).Msg("flushTokenUsage: no state_run to attach usage to")
		return
	}
	sr.Usage = models.AddTokenUsage(sr.Usage, delta)
	sr.UsageByModel = models.AddTokenUsageByModel(sr.UsageByModel, byModel)
	logDB(e.db.Save(&sr), runID, "flush token usage")
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func parseActions(v any) []models.GateAction {
	var out []models.GateAction
	arr, _ := v.([]any)
	for _, a := range arr {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		reqForm, _ := m["requireForm"].(bool)
		out = append(out, models.GateAction{ID: str(m["id"]), Label: str(m["label"]), Goto: str(m["goto"]), RequireForm: reqForm})
	}
	return out
}

func parseForm(v any) []models.GateField {
	var out []models.GateField
	arr, _ := v.([]any)
	for _, a := range arr {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		req, _ := m["required"].(bool)
		out = append(out, models.GateField{Key: str(m["key"]), Label: str(m["label"]), Required: req})
	}
	return out
}
