package engine

import (
	"fmt"
	"strings"

	"github.com/cocofhu/approving/internal/blob"
	"github.com/cocofhu/approving/internal/models"
)

// whenLooksLikeFailAction reports whether an edge when-guard is a fail/reject
// action match (used to keep app_preview firstSuccess fallback off fail edges).
func whenLooksLikeFailAction(when string) bool {
	when = strings.TrimSpace(when)
	if when == "" {
		return false
	}
	for _, id := range []string{"fail", "reject", "revise"} {
		if strings.Contains(when, "action") && (strings.Contains(when, "'"+id+"'") || strings.Contains(when, `"`+id+`"`)) {
			return true
		}
	}
	return false
}

// routeSuccess selects the next state after a node succeeds.
func (e *Engine) routeSuccess(c *execCtx, node *models.Node, outcome nodeOutcome) string {
	if e.IsHalted() {
		e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "exit", Detail: "shutdown: scheduler halted"})
		e.finish(c.run.ID, "cancelled")
		return ""
	}

	if node.Type == "branch" {
		if outcome.goto_ != "" {
			e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "transition", To: outcome.goto_, Kind: models.EdgeSuccess})
			return outcome.goto_
		}
		return ""
	}

	if outcome.goto_ != "" {
		e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "transition", To: outcome.goto_, Kind: models.EdgeSuccess})
		return outcome.goto_
	}

	extra := map[string]any{}
	if a, ok := outcome.outputs["action"]; ok {
		extra["action"] = a
	}
	ec := e.evalContext(c, extra)
	edges := c.graph.OutEdges(node.ID)
	var firstSuccess *models.Edge
	for i := range edges {
		ed := edges[i]
		if ed.KindOrDefault() == models.EdgeSuccess && firstSuccess == nil {

			if node.Type == "app_preview" && whenLooksLikeFailAction(ed.When) {

			} else {
				firstSuccess = &edges[i]
			}
		}
		if !guardPasses(ed.When, ec) {
			continue
		}
		if ed.KindOrDefault() == models.EdgeRollback {
			if target, ok := e.doRollback(c, ed); ok {
				return target
			}
			continue
		}
		e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "transition", To: ed.Target, Kind: ed.KindOrDefault()})
		return ed.Target
	}

	if firstSuccess != nil {
		e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "transition", To: firstSuccess.Target, Kind: models.EdgeSuccess})
		return firstSuccess.Target
	}
	return ""
}

// routeFailure selects the next state after a node fails. When the outcome
// carries a structured-gate goto or action, goto is preferred, then when-guarded
// success edges from the bottom outlet; otherwise legacy rollback/failure edges.
func (e *Engine) routeFailure(c *execCtx, node *models.Node, outcome nodeOutcome) string {
	if outcome.goto_ != "" {
		e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "transition", To: outcome.goto_, Kind: models.EdgeFailure})
		return outcome.goto_
	}
	if a, ok := outcome.outputs["action"]; ok {
		ec := e.evalContext(c, map[string]any{"action": a})
		edges := c.graph.OutEdges(node.ID)
		for i := range edges {
			ed := edges[i]
			if ed.KindOrDefault() != models.EdgeSuccess || strings.TrimSpace(ed.When) == "" {
				continue
			}
			if !guardPasses(ed.When, ec) {
				continue
			}
			e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "transition", To: ed.Target, Kind: models.EdgeSuccess})
			return ed.Target
		}
	}
	edges := c.graph.OutEdges(node.ID)
	for i := range edges {
		if edges[i].KindOrDefault() == models.EdgeRollback {
			if target, ok := e.doRollback(c, edges[i]); ok {
				return target
			}
		}
	}
	for i := range edges {
		if edges[i].KindOrDefault() == models.EdgeFailure {
			e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "transition", To: edges[i].Target, Kind: models.EdgeFailure})
			return edges[i].Target
		}
	}
	return ""
}

// doRollback performs a rollback transition: enforce the attempt cap, restore
// the target checkpoint's variable snapshot, and inject carried error context.
func (e *Engine) doRollback(c *execCtx, ed models.Edge) (string, bool) {
	c.run.Attempt++
	if ed.MaxAttempts > 0 && c.run.Attempt > ed.MaxAttempts {
		e.appendTrace(c, models.TraceEntry{NodeID: ed.Source, Event: "exit", Detail: "rollback attempts exhausted"})
		c.run.Attempt--
		return "", false
	}
	logDB(e.db.Model(&models.Run{}).Where("id = ?", c.run.ID).UpdateColumn("attempt", c.run.Attempt), c.run.ID, "rollback attempt")

	if snap, ok := c.run.Checkpoints[ed.Target]; ok {
		for k, v := range snap {
			c.setVar(k, v)
			e.persistVar(c.run.ID, k, v)
		}
	}

	for _, name := range ed.Carry {
		if _, ok := c.vars[name]; !ok {
			c.setVar(name, "")
			e.persistVar(c.run.ID, name, "")
		}
	}
	e.appendTrace(c, models.TraceEntry{NodeID: ed.Source, Event: "rollback", To: ed.Target, Kind: models.EdgeRollback,
		Detail: fmt.Sprintf("attempt=%d 携带 %v 回滚到 checkpoint", c.run.Attempt, ed.Carry)})
	return ed.Target, true
}

func (e *Engine) snapshotCheckpoint(c *execCtx, nodeID string) {
	if c.run.Checkpoints == nil {
		c.run.Checkpoints = map[string]map[string]any{}
	}
	snap := map[string]any{}
	for k, v := range c.vars {
		snap[k] = blob.StripDataInValue(v)
	}
	c.run.Checkpoints[nodeID] = snap

	logDB(e.db.Model(&models.Run{}).Where("id = ?", c.run.ID).
		Select("Checkpoints").Updates(&models.Run{Checkpoints: c.run.Checkpoints}), c.run.ID, "snapshot checkpoint")
}
