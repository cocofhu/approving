package engine

import (
	"context"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"
)

func (e *Engine) execReactEnter(c *execCtx, node *models.Node) nodeOutcome {

	iter := c.iter[node.ID]
	var conv models.ReactConversation
	err := e.db.Where("run_id = ? AND node_id = ? AND iteration = ?", c.run.ID, node.ID, iter).First(&conv).Error
	if err == nil && conv.Done {
		return nodeOutcome{status: "completed", outputMd: "澄清已完成"}
	}
	if err != nil {

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

	if err != nil && !e.host.HasHealthyPreviewPorts(c.run.ID, node.ID) {
		return nodeOutcome{status: "failed", err: err.Error(), outputMd: "应用预览执行失败:" + err.Error(), events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
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
	if !e.host.HasHealthyPreviewPorts(c.run.ID, node.ID) {
		return nodeOutcome{status: "failed", err: "预览契约未满足:未成功 set_preview(可达)",
			outputMd: "应用预览失败:未成功注册可达预览端口", events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}

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

	iter := c.iter[node.ID]
	var conv models.ReactConversation
	if err := e.db.Where("run_id = ? AND node_id = ? AND iteration = ?", c.run.ID, node.ID, iter).First(&conv).Error; err == nil && conv.Done {
		return nodeOutcome{status: "completed", outputMd: "预览复审已完成",
			outputs: map[string]any{"resolved": true, "preview_ready": true}, events: res.Events, usage: res.Usage, usageByModel: res.UsageByModel}
	}
	paused := nodeOutcome{status: "paused", outputMd: "等待人工预览复审…", events: res.Events, outputs: res.Outputs, usage: res.Usage, usageByModel: res.UsageByModel}
	return e.enterReview(c, node, paused)
}
