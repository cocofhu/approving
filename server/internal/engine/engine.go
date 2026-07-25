// Package engine implements the finite-state-machine orchestrator: nodes
// are states, edges are transitions (success / failure / rollback). It
// drives node executors, evaluates guards, persists per-node StateRuns and
// the run-level state trace, and on failure can roll back to a checkpoint
// (restoring the variable snapshot and injecting error context).
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"
	"github.com/cocofhu/approving/internal/services"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Engine orchestrates workflow runs.
type Engine struct {
	db       *gorm.DB
	provider runtime.ExecProvider
	host     *mcp.Host
	store    mcp.Store
	broker   *Broker
	// sem bounds the number of runs actively executing (in the "running"
	// state). Its capacity is the effective max_concurrent_runs and can be
	// changed at runtime via SetMaxConcurrent (settings page).
	sem *sema
	// wake signals the dispatcher to (re)drain the queued-run backlog. Buffered
	// size 1 so signals coalesce and senders never block.
	wake chan struct{}
	// stop closes to end the dispatcher goroutine (tests / process shutdown).
	stop     chan struct{}
	stopOnce sync.Once

	// autoRetryMax caps how many times a node that fails with retryable=true
	// (and has no explicit failure/rollback edge) is auto-retried from the
	// failure position before the run is failed. 0 disables auto-retry.
	// Live-tunable from the settings page via SetAutoRetryMax; the raw engine
	// defaults to disabled so tests that don't wire settings keep the old
	// fail-fast behavior (production enables it via SettingsService.ApplyOnBoot,
	// whose config default is 3).
	autoRetryMax atomic.Int32

	mu     sync.Mutex
	tokens map[string]string // runID -> MCP token (in-memory; same process)

	// resumeMu guards resumeLocks; resumeLocks serializes human-resume
	// operations (gate resume / react reply) per "runID:nodeID" so a
	// double-submit (e.g. a page refresh re-enabling the confirm button while
	// the first reply is still being processed by the slow sandbox agent)
	// cannot both pass the "already done" guard and advance the FSM twice.
	resumeMu    sync.Mutex
	resumeLocks map[string]*sync.Mutex

	// execMu guards execRuns / execGen. execRuns is the set of runs that
	// currently have a live execute() goroutine. The FSM keeps exactly one
	// active node per run, so a run must be driven by at most one goroutine.
	// execGen is a per-run ownership token: Cancel/ResumeFrom may force-clear a
	// zombie slot (agent still blocked in RunAgent after the run was cancelled)
	// by bumping the generation so the late driver's deferred endExecute cannot
	// delete a newer driver's claim.
	execMu   sync.Mutex
	execRuns map[string]bool
	execGen  map[string]uint64

	// haltMu guards halted: when true the dispatcher stops admitting work and
	// agent/react nodes finish as cancelled instead of advancing the FSM.
	haltMu sync.RWMutex
	halted bool

	// projectVarsLookup, when set, returns project-level workflow variables for
	// a workflow so StartRun can seed them into vars.* before Graph.Variables
	// and launch inputs overlay. Optional (nil = no project seed).
	projectVarsLookup func(workflowID string) []models.ProjectVariable

	// issues resolves preview-issue lifecycle on gate resume. Optional in tests
	// (nil falls back to a short-lived IssueService on e.db).
	issues *services.IssueService
}

// New builds an engine.
func New(db *gorm.DB, provider runtime.ExecProvider, host *mcp.Host, store mcp.Store, maxRuns int) *Engine {
	if maxRuns <= 0 {
		maxRuns = 5
	}
	e := &Engine{
		db: db, provider: provider, host: host, store: store,
		broker: NewBroker(), sem: newSema(maxRuns),
		wake: make(chan struct{}, 1), stop: make(chan struct{}),
		tokens: map[string]string{}, resumeLocks: map[string]*sync.Mutex{},
		execRuns: map[string]bool{}, execGen: map[string]uint64{},
	}
	// Wire live ACP event streaming (optional provider capability) so a running
	// agent node's events reach the run detail UI as they happen.
	if sink, ok := provider.(interface {
		SetEventSink(func(runID, nodeID string, events []models.AcpEvent, busy bool))
	}); ok {
		sink.SetEventSink(e.publishAcp)
	}
	// When MCP WriteArtifact persists a mapped primary product (e.g. page.html)
	// during review / waiting_human, sync StateRun.outputs + pending BodyMd so
	// clarify/复审 paths match SaveGateArtifact.
	host.SetAfterWriteArtifact(e.syncAfterPrimaryArtifactWrite)
	e.reconcileInterrupted()
	// Start the priority-then-FIFO dispatcher: it admits queued runs (including
	// any left queued across a restart) up to the concurrency limit.
	go e.dispatch()
	return e
}

// SetMaxConcurrent changes the live cap on simultaneously executing runs and
// nudges the dispatcher so a raised limit immediately admits more queued runs.
func (e *Engine) SetMaxConcurrent(n int) {
	e.sem.SetLimit(n)
	e.signalDispatch()
}

// MaxConcurrent returns the current concurrency cap.
func (e *Engine) MaxConcurrent() int { return e.sem.Limit() }

// SetProjectVarsLookup wires the lookup used by StartRun to seed project-level
// workflow variables into the run's vars.* namespace.
func (e *Engine) SetProjectVarsLookup(fn func(workflowID string) []models.ProjectVariable) {
	e.projectVarsLookup = fn
}

// SetIssueService wires the preview-issue service used on gate resume.
func (e *Engine) SetIssueService(s *services.IssueService) {
	e.issues = s
}

func (e *Engine) issueService() *services.IssueService {
	if e.issues != nil {
		return e.issues
	}
	return services.NewIssueService(e.db)
}

// SetAutoRetryMax changes the live cap on how many times a failed node is
// auto-retried from its failure position (see autoRetryMax). Negative clamps to
// 0 (disabled). Driven by the settings page / config.
func (e *Engine) SetAutoRetryMax(n int) {
	if n < 0 {
		n = 0
	}
	e.autoRetryMax.Store(int32(n))
}

// AutoRetryMax returns the current per-node auto-retry cap (0 = disabled).
func (e *Engine) AutoRetryMax() int { return int(e.autoRetryMax.Load()) }

// isAutoRetryable reports whether a failed node's outcome may be auto-retried
// from the failure position. It reads only oc.retryable — callers (execAgent /
// execStructuredAgent) set that flag on RunAgent failures. Contract finalize
// misses, structured-gate rejects, and other deterministic faults leave the
// zero value false and are not auto-retried. Note: "计划未全部完成" returned as a
// RunAgent error still goes through execAgent and is therefore retryable.
func isAutoRetryable(oc nodeOutcome) bool {
	return oc.retryable
}

// shortReason renders a failure message as a single trimmed line, capped in
// length, for a compact auto-retry trace detail.
func shortReason(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}

// autoRetryBackoff is the pause before re-entering a node on auto-retry, giving
// a transient fault (e.g. a flaky CI push, a sandbox/registry hiccup) a moment
// to clear before the fresh attempt. A package var (not const) so tests can
// zero it out.
var autoRetryBackoff = 5 * time.Second

// tryAutoRetry decides whether a failed node (with no explicit failure/rollback
// edge) should be re-run from its failure position, and if so records the
// attempt. It returns true when the caller should re-enter node.ID; false when
// the budget is exhausted, the fault is deterministic, or auto-retry is off.
// The failed StateRun was already persisted by the caller, so it stays as
// history and the re-entry opens a fresh visit (like a manual ResumeFrom).
func (e *Engine) tryAutoRetry(c *execCtx, node *models.Node, outcome nodeOutcome) bool {
	max := e.AutoRetryMax()
	if max <= 0 || !isAutoRetryable(outcome) || c.autoRetries[node.ID] >= max {
		return false
	}
	c.autoRetries[node.ID]++
	reason := outcome.err
	if strings.TrimSpace(reason) == "" {
		reason = outcome.outputMd
	}
	e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "resume",
		Detail: fmt.Sprintf("自动从失败位置重试 %d/%d:%s", c.autoRetries[node.ID], max, shortReason(reason))})
	log.Info().Str("run_id", c.run.ID).Str("node_id", node.ID).
		Int("attempt", c.autoRetries[node.ID]).Int("max", max).Str("err", outcome.err).
		Msg("auto-retrying failed node from failure position")
	if autoRetryBackoff > 0 {
		time.Sleep(autoRetryBackoff)
	}
	return true
}

// signalDispatch wakes the dispatcher without blocking (buffered, coalescing).
func (e *Engine) signalDispatch() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// reconcileInterrupted cleans up runs whose in-memory execution goroutine was
// lost on a process restart. A node's sandbox is a separate container that can
// keep running independently, but nothing in this fresh process will finalize
// it — leaving the run in a zombie "running" (or orphaned "waiting_human")
// state that never advances. We mark those runs (and their mid-flight node
// states) failed so they surface clearly and can be re-run. Runs legitimately
// paused at a human gate or react dialogue have no in-flight node state and are
// left untouched so they stay resumable.
//
// Runs still "queued" are also left untouched: they had not started executing,
// so they are safe to recover — the dispatcher (started right after this in
// New) admits them by priority then FIFO once slots are free. This is the
// durable-queue guarantee: a backlog survives a restart instead of being lost.
func (e *Engine) reconcileInterrupted() {
	orphans := map[string]bool{}

	// Node states caught mid-execution cannot resume: their driving goroutine
	// died with the old process.
	var running []models.StateRun
	if err := e.db.Where("status = ?", "running").Find(&running).Error; err != nil {
		log.Error().Err(err).Msg("startup reconciliation: query running node states failed")
	} else {
		for _, sr := range running {
			logDB(e.db.Model(&models.StateRun{}).Where("id = ?", sr.ID).Updates(map[string]any{
				"status": "failed",
				"error":  "服务重启中断,执行协程丢失,节点未收尾",
			}), sr.RunID, "reconcile state_run")
			orphans[sr.RunID] = true
		}
	}

	// Runs still flagged running had no live goroutine after restart.
	var runs []models.Run
	if err := e.db.Where("status = ?", "running").Find(&runs).Error; err != nil {
		log.Error().Err(err).Msg("startup reconciliation: query running runs failed")
	} else {
		for _, r := range runs {
			orphans[r.ID] = true
		}
	}

	for id := range orphans {
		var r models.Run
		if e.db.First(&r, "id = ?", id).Error != nil {
			continue
		}
		if r.Status == "completed" || r.Status == "failed" || r.Status == "cancelled" {
			continue
		}
		logDB(e.db.Model(&models.Run{}).Where("id = ?", id).UpdateColumn("status", "failed"), id, "reconcile run")
		e.host.UnregisterRun(id)
		log.Warn().Str("run_id", id).Msg("reconciled interrupted run on startup -> failed")
	}
	if len(orphans) > 0 {
		log.Info().Int("count", len(orphans)).Msg("startup reconciliation complete")
	}
}

// Broker exposes the run-update pub/sub for the WS handler.
func (e *Engine) Broker() *Broker { return e.broker }

// LiveNodeEvents reads a running node's event log directly from its live
// sandbox (via the provider), if the node is currently executing here. ok is
// false otherwise, so the caller falls back to the persisted StateRun snapshot.
// A non-nil err means a live sandbox was found but the bridge read failed.
func (e *Engine) LiveNodeEvents(ctx context.Context, runID, nodeID string) ([]models.AcpEvent, bool, error) {
	src, ok := e.provider.(runtime.LiveEventSource)
	if !ok {
		return nil, false, nil
	}
	return src.LiveNodeEvents(ctx, runID, nodeID)
}

// LiveNodeEventsPage reads a page of events from a running node's live sandbox.
// A non-nil err means a live sandbox was found but the bridge read failed —
// callers must not treat that as an empty live page.
func (e *Engine) LiveNodeEventsPage(ctx context.Context, runID, nodeID, cursor string, limit int) ([]models.AcpEvent, string, bool, bool, error) {
	src, ok := e.provider.(runtime.LiveEventPageSource)
	if !ok {
		return nil, "", false, false, nil
	}
	return src.LiveNodeEventsPage(ctx, runID, nodeID, cursor, limit)
}

// publishAcp streams a running node's in-progress ACP events to subscribers of
// the run's WebSocket (live agent preview). It is best-effort: marshal failures
// or absent subscribers are silently dropped. The authoritative event log lives
// in the sandbox (queried on demand via LiveNodeEvents) and the final snapshot
// is persisted by saveState — this is purely the live push channel.
func (e *Engine) publishAcp(runID, nodeID string, events []models.AcpEvent, busy bool) {
	msg, err := json.Marshal(map[string]any{
		"type": "acp", "runId": runID, "nodeId": nodeID, "events": events, "busy": busy,
	})
	if err != nil {
		return
	}
	e.broker.Publish(runID, msg)
}

// startNodeRun appends a fresh in-flight StateRun (status=running) for the
// node's current execution index the moment the FSM enters it — for every node
// type, not just streaming ones. This makes each (re-)visit its own row so a
// loop-back / rollback retry never overwrites the previous execution's output,
// events, or duration, and a long-running node's streamed events (persisted via
// publishAcp) are queryable on refresh even before it completes.
func (e *Engine) startNodeRun(c *execCtx, node *models.Node) {
	now := time.Now()
	// Snapshot the variables as they stand on entry so a later manual resume can
	// restore this node's "state at that time" (see ResumeFrom / restoreVars).
	before := map[string]any{}
	for k, v := range c.vars {
		before[k] = v
	}
	logDB(e.db.Create(&models.StateRun{
		RunID: c.run.ID, NodeID: node.ID, NodeType: node.Type,
		Iteration: c.iter[node.ID], Status: "running", StartedAt: &now, Attempt: c.run.Attempt,
		VarsBefore: before,
	}), c.run.ID, "startNodeRun")
}

// StartRun creates a run pinned to an immutable snapshot of the workflow's
// current graph (Run.Graph) and launches asynchronous execution from the start
// node. The snapshot makes historical runs immune to later edits / deletion of
// the live workflow definition. Priority defaults to normal.
func (e *Engine) StartRun(workflowID string, inputs map[string]any, trigger string) (*models.Run, error) {
	return e.StartRunWithPriority(workflowID, inputs, trigger, "")
}

// StartRunWithPriority is like StartRun but accepts a priority label
// (high|normal|low). Empty string defaults to normal; invalid values error.
func (e *Engine) StartRunWithPriority(workflowID string, inputs map[string]any, trigger, priorityLabel string) (*models.Run, error) {
	if e.IsHalted() {
		return nil, fmt.Errorf("server is shutting down")
	}
	pri, err := models.ParsePriorityLabel(priorityLabel)
	if err != nil {
		return nil, err
	}
	var def models.WorkflowDef
	if err := e.db.First(&def, "id = ?", workflowID).Error; err != nil {
		return nil, fmt.Errorf("workflow not found: %w", err)
	}
	// Snapshot the workflow's current graph head and run against it. Every run
	// freezes its own immutable copy into Run.Graph (below), so later edits /
	// re-publishes never change what a historical run executed or displays.
	//
	// We deliberately use the live definition head (def.Graph) rather than an
	// archived WorkflowVersion keyed by def.Version: after a publish → edit(draft)
	// cycle, Save overwrites def.Graph but leaves def.Version pointing at the old
	// published snapshot, so keying off it would silently run (and snapshot) the
	// stale graph — the "改了之后历史流水线对不上" bug. def.Graph is always the
	// graph the user just saved; for an unedited published head it equals the
	// published snapshot anyway.
	return e.startRun(def, def.Graph, inputs, trigger, pri)
}

// StartRunFromPublished creates a run using the published WorkflowVersion
// snapshot. Only workflows with status=published are accepted. Empty trigger
// defaults to api; explicit values must be whitelist codes (manual|api|pm_mcp).
// Used exclusively by /v1 external API.
// Priority is always normal (non-UI paths cannot set priority this period).
func (e *Engine) StartRunFromPublished(workflowID string, inputs map[string]any, trigger string) (*models.Run, error) {
	if e.IsHalted() {
		return nil, fmt.Errorf("server is shutting down")
	}
	resolved, err := models.ResolveTrigger(trigger, models.TriggerAPI)
	if err != nil {
		return nil, err
	}
	var def models.WorkflowDef
	if err := e.db.First(&def, "id = ?", workflowID).Error; err != nil {
		return nil, fmt.Errorf("workflow not found: %w", err)
	}
	if def.Status != "published" {
		return nil, fmt.Errorf("workflow not published")
	}
	var snap models.WorkflowVersion
	if err := e.db.Where("workflow_id = ? AND version = ?", def.ID, def.Version).First(&snap).Error; err != nil {
		return nil, fmt.Errorf("published version not found: %w", err)
	}
	return e.startRun(def, snap.Graph, inputs, resolved, models.PriorityNormal)
}

func (e *Engine) startRun(def models.WorkflowDef, graph models.Graph, inputs map[string]any, trigger string, priority int) (*models.Run, error) {
	// The pipeline must start at an input and end at an output.
	if err := graph.Validate(); err != nil {
		return nil, err
	}
	if !models.ValidPriorityInt(priority) {
		priority = models.PriorityNormal
	}

	runID := "run-" + uuid.NewString()[:8]
	if inputs == nil {
		inputs = map[string]any{}
	}
	// Resolve the live value of every global variable: project defaults first,
	// then Graph.Variables + launcher-submitted Ask values (latter wins on name).
	var projectVars []models.ProjectVariable
	if e.projectVarsLookup != nil {
		projectVars = e.projectVarsLookup(def.ID)
	}
	seeded, err := resolveStartVars(graph, inputs, projectVars)
	if err != nil {
		return nil, err
	}
	title := computeRunTitle(graph, seeded)
	// Enqueue as "queued": the priority dispatcher promotes it to "running" (and
	// stamps StartedAt) when a concurrency slot is free. Persisting the queued
	// state — rather than blocking a goroutine on a slot — is what makes the
	// backlog visible and durable across restarts. StartedAt is deliberately
	// left zero here so 耗时 reflects actual execution time, not queue wait.
	run := models.Run{
		ID: runID, WorkflowID: def.ID, WorkflowName: def.Name, WorkflowVersion: def.Version,
		Status: "queued", Trigger: trigger, Inputs: inputs, Graph: graph, Title: title,
		Priority: priority,
		Trace:    []models.TraceEntry{}, Checkpoints: map[string]map[string]any{},
	}
	// Provision the per-run scoped MCP endpoint + token (injected to sandboxes).
	// Register it before publishing a queued row: the dispatcher can observe a
	// committed queued run immediately, so registering afterward races its
	// first node and intermittently fails MCP writes as unauthorized.
	tok := e.host.RegisterRun(runID)
	run.McpToken = tok
	e.mu.Lock()
	e.tokens[runID] = tok
	e.mu.Unlock()

	now := time.Now()
	if err := e.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		// Persist live global variables in the same transaction so the
		// dispatcher cannot execute a partially initialized run.
		for _, rv := range seeded {
			rv.RunID = runID
			if err := tx.Create(&rv).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.WorkflowDef{}).Where("id = ?", def.ID).Update("last_run_at", now).Error
	}); err != nil {
		e.host.UnregisterRun(runID)
		e.mu.Lock()
		delete(e.tokens, runID)
		e.mu.Unlock()
		return nil, err
	}

	start := graph.StartNode()
	if start == nil {
		e.finish(runID, "failed")
		return &run, fmt.Errorf("workflow has no nodes")
	}
	log.Info().Str("run_id", runID).Str("node_id", start.ID).Int("priority", priority).Msg("run queued")
	// Hand off to the dispatcher, which admits by priority then FIFO when a slot is free.
	e.signalDispatch()
	return &run, nil
}

// UpdateRunPriority sets the admission priority of a non-terminal run.
// Allowed statuses: queued, running, waiting_human. Terminal runs
// (completed/failed/cancelled) are rejected. Changing priority never
// preempts a running run; it only affects subsequent claim ordering.
func (e *Engine) UpdateRunPriority(runID, priorityLabel string) (*models.Run, error) {
	pri, err := models.ParsePriorityLabel(priorityLabel)
	if err != nil {
		return nil, err
	}
	var run models.Run
	if err := e.db.First(&run, "id = ?", runID).Error; err != nil {
		return nil, fmt.Errorf("run not found")
	}
	switch run.Status {
	case "queued", "running", "waiting_human":
		// ok
	default:
		return nil, fmt.Errorf("cannot change priority of run in status %q", run.Status)
	}
	if err := e.db.Model(&models.Run{}).Where("id = ?", runID).Update("priority", pri).Error; err != nil {
		return nil, err
	}
	run.Priority = pri
	log.Info().Str("run_id", runID).Str("status", run.Status).Int("priority", pri).Msg("run priority updated")
	return &run, nil
}

// resolveStartVars computes the initial live value of every declared global
// variable. Project variables are seeded first; Graph.Variables then overlay
// (and Ask vars take submitted values). Required Ask variables must end up
// non-blank. Submitted values are coerced to the variable's type so
// guards/templates see numbers and bools, not strings from the launch form.
// Every variable is persisted under the single {{vars.x}} namespace. Project
// seed alone does not inject values into the container OS environ.
func resolveStartVars(g models.Graph, submitted map[string]any, projectVars []models.ProjectVariable) ([]models.RunVariable, error) {
	byName := make(map[string]models.RunVariable, len(projectVars)+len(g.Variables))
	order := make([]string, 0, len(projectVars)+len(g.Variables))
	for _, v := range projectVars {
		if v.Name == "" {
			continue
		}
		typ := v.Type
		if typ == "" {
			typ = "string"
		}
		byName[v.Name] = models.RunVariable{Name: v.Name, Type: typ, Value: coerceVar(v.Value, typ)}
		order = append(order, v.Name)
	}
	for _, v := range g.Variables {
		if v.Name == "" {
			continue
		}
		val := v.Value
		if v.Ask {
			if sv, ok := submitted[v.Name]; ok && !isBlank(sv) {
				val = sv
			}
			if isBlank(val) && v.Required {
				label := v.Desc
				if label == "" {
					label = v.Name
				}
				return nil, fmt.Errorf("缺少必填项: %s", label)
			}
		}
		if _, exists := byName[v.Name]; !exists {
			order = append(order, v.Name)
		}
		byName[v.Name] = models.RunVariable{Name: v.Name, Type: v.Type, Value: coerceVar(val, v.Type)}
	}
	out := make([]models.RunVariable, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out, nil
}

// computeRunTitle returns the string title for a run: the coerced value of the
// first ask=true global variable (by Graph.Variables order), or "" if none.
func computeRunTitle(g models.Graph, seeded []models.RunVariable) string {
	byName := make(map[string]models.RunVariable, len(seeded))
	for _, rv := range seeded {
		byName[rv.Name] = rv
	}
	for _, v := range g.Variables {
		if v.Name == "" || !v.Ask {
			continue
		}
		rv, ok := byName[v.Name]
		if !ok || isBlank(rv.Value) {
			return ""
		}
		return varValueToTitleString(rv.Value, v.Type)
	}
	return ""
}

func varValueToTitleString(val any, typ string) string {
	switch typ {
	case "bool":
		if b, ok := val.(bool); ok {
			return strconv.FormatBool(b)
		}
	case "number":
		switch val.(type) {
		case float64, int, int64:
			return fmt.Sprint(val)
		}
	}
	if models.IsCompositeText(val) {
		ct := models.AsCompositeText(val)
		t := strings.TrimSpace(ct.Text)
		n := len(ct.Images)
		if t != "" && n > 0 {
			return fmt.Sprintf("%s · %d图", t, n)
		}
		if n > 0 {
			return fmt.Sprintf("%d张图", n)
		}
		return t
	}
	return fmt.Sprint(val)
}

func isBlank(v any) bool {
	return models.IsBlankVar(v)
}

// coerceVar normalizes a raw value (often a string from the launch form) to the
// variable's declared type so numeric/boolean guards compare correctly.
func coerceVar(v any, t string) any {
	switch t {
	case "number":
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f
			}
		}
	case "bool":
		switch b := v.(type) {
		case bool:
			return b
		case string:
			return strings.EqualFold(strings.TrimSpace(b), "true")
		}
	case "string":
		if models.IsCompositeText(v) {
			return v
		}
	}
	return v
}

// execCtx is the in-memory execution context, rebuildable from the DB so a
// run can resume after a human gate / react pause.
type execCtx struct {
	run         *models.Run
	graph       models.Graph
	vars        map[string]any
	nodeOutputs map[string]map[string]any
	// iter tracks the current per-node execution index (1-based). Seeded from
	// the DB in loadCtx so it survives a pause/resume, then bumped on each
	// enter. Every node write (StateRun / Gate / conversation) is stamped with
	// it, so a loop-back onto a node produces a fresh, separately-traceable
	// execution instead of overwriting the previous one.
	iter map[string]int
	// autoRetries tracks how many times each node has been auto-retried from
	// its failure position within THIS driver's lifetime (see tryAutoRetry).
	// In-memory only: a manual ResumeFrom starts a fresh driver and thus a
	// fresh budget, which is the intended "manual retry is separate" behavior.
	autoRetries map[string]int
	token       string
	// execGen is this driver's ownership token from beginExecute. After Cancel
	// force-clears the slot and ResumeFrom admits a newer driver, the zombie
	// must not TakeOutcome (node_complete is keyed only by run+node) or the
	// fresh visit loses its mark and fails with "未调用 node_complete".
	execGen uint64
}

// lockResume serializes resume operations for a given key (runID:nodeID),
// returning the unlock func. Concurrent callers (e.g. a re-submitted confirm
// after a page refresh) block until the in-flight one finishes, after which
// they observe the persisted Done/Resolved state and are rejected.
func (e *Engine) lockResume(key string) func() {
	e.resumeMu.Lock()
	m, ok := e.resumeLocks[key]
	if !ok {
		m = &sync.Mutex{}
		e.resumeLocks[key] = m
	}
	e.resumeMu.Unlock()
	m.Lock()
	return m.Unlock
}

func (e *Engine) loadCtx(runID string) (*execCtx, error) {
	var run models.Run
	if err := e.db.First(&run, "id = ?", runID).Error; err != nil {
		return nil, err
	}
	c := &execCtx{run: &run, graph: run.Graph,
		vars: map[string]any{}, nodeOutputs: map[string]map[string]any{},
		iter: map[string]int{}, autoRetries: map[string]int{}}
	var vars []models.RunVariable
	if err := e.db.Where("run_id = ?", runID).Find(&vars).Error; err != nil {
		return nil, fmt.Errorf("load run variables: %w", err)
	}
	for _, v := range vars {
		c.vars[v.Name] = v.Value
	}
	// Load the per-node execution history. nodeOutputs must reflect the LATEST
	// execution of each node, and c.iter must resume from the highest recorded
	// index so the next enter bumps it correctly (rather than re-numbering from
	// 1 after a pause/resume).
	var states []models.StateRun
	if err := e.db.Where("run_id = ?", runID).Order("iteration asc, id asc").Find(&states).Error; err != nil {
		return nil, fmt.Errorf("load run states: %w", err)
	}
	for _, s := range states {
		if s.Iteration > c.iter[s.NodeID] {
			c.iter[s.NodeID] = s.Iteration
		}
		if s.Outputs != nil {
			c.nodeOutputs[s.NodeID] = s.Outputs // later (higher iteration) wins
		}
	}
	e.mu.Lock()
	c.token = e.tokens[runID]
	e.mu.Unlock()
	// After a server restart the in-memory token map is empty; re-register the
	// token persisted on the Run so a resumed run's sandbox MCP calls authorize
	// again (otherwise every write_artifact/set_* fails with ErrUnauthorized).
	if c.token == "" && run.McpToken != "" {
		c.token = run.McpToken
		e.mu.Lock()
		e.tokens[runID] = run.McpToken
		e.mu.Unlock()
		e.host.RestoreRun(runID, run.McpToken)
	}
	return c, nil
}

func (e *Engine) evalContext(c *execCtx, extra map[string]any) EvalContext {
	return EvalContext{
		Vars: c.vars, Nodes: c.nodeOutputs, Extra: extra,
		Artifact: func(name string) (string, bool) { return e.store.Get(c.run.ID, name) },
	}
}

// beginExecute claims the sole execution slot for a run. It returns ok=false
// when another goroutine is already driving the run, so the caller must bail
// rather than run a second concurrent FSM loop over the same run (which would
// race on its persisted state and double-advance transitions). The returned gen
// must be passed to endExecute so a force-cleared zombie driver cannot drop a
// newer owner's claim.
func (e *Engine) beginExecute(runID string) (gen uint64, ok bool) {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	if e.execRuns[runID] {
		return 0, false
	}
	e.execGen[runID]++
	e.execRuns[runID] = true
	return e.execGen[runID], true
}

func (e *Engine) endExecute(runID string, gen uint64) {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	if e.execGen[runID] == gen {
		delete(e.execRuns, runID)
	}
}

// forceEndExecute drops a zombie exec slot (and bumps generation) so ResumeFrom
// can admit a fresh driver after Cancel left an agent call still blocked.
func (e *Engine) forceEndExecute(runID string) {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	if e.execRuns[runID] {
		e.execGen[runID]++
		delete(e.execRuns, runID)
	}
}

// isExecOwner reports whether gen still owns runID's execute slot. After
// forceEndExecute + a newer beginExecute, the late zombie driver must abandon
// without routing — even if ResumeFrom has already flipped DB status back to
// running.
func (e *Engine) isExecOwner(runID string, gen uint64) bool {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	return e.execRuns[runID] && e.execGen[runID] == gen
}

// acquireExecuteSlot polls beginExecute for up to ~2s. The only expected
// contention is the brief teardown window of a just-paused/failed driver (see
// execute's comment); a slot still held past that window means a genuinely
// live driver (e.g. a long node call), and the caller should bail.
func (e *Engine) acquireExecuteSlot(runID string) (gen uint64, ok bool) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		if gen, ok = e.beginExecute(runID); ok {
			return gen, true
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// runAdmitted drives an already-admitted run (its concurrency slot is held by
// the caller — the dispatcher via TryAcquire, or a resume via Acquire). It runs
// the FSM loop, then releases the slot and wakes the dispatcher so the next
// queued run can be admitted. This centralizes the "release + wake" so the
// invariant holds: a run in the running state holds exactly one slot.
func (e *Engine) runAdmitted(runID, fromNodeID string) {
	defer func() {
		e.sem.Release()
		e.signalDispatch()
	}()
	e.execute(runID, fromNodeID)
}

// resumeAdmitted continues a paused or terminated run from fromNodeID via the
// same TryAcquire + queued fallback path as the priority-then-FIFO dispatcher.
// DB status is only promoted to running once a slot is actually held.
func (e *Engine) resumeAdmitted(runID, fromNodeID string) {
	e.scheduleRunAdmission(runID, fromNodeID)
}

// execute drives the FSM loop from the given node id. The caller must already
// hold a concurrency slot (see runAdmitted / resumeAdmitted).
func (e *Engine) execute(runID, fromNodeID string) {
	// Guarantee a single driver per run: a stray second execute() (double
	// resume, re-trigger, restart race) becomes a no-op instead of racing.
	//
	// A departing driver becomes externally actionable (Gate row committed,
	// ReactConversation opened) *before* it finishes unwinding (saveState /
	// appendTrace / finish + the deferred endExecute below) — so a human
	// resume landing in that narrow window would otherwise see the slot as
	// still taken and get silently dropped, stranding the run. Retry briefly
	// rather than bailing immediately: the departing driver never blocks on
	// external work past this point, so it releases almost immediately.
	gen, ok := e.acquireExecuteSlot(runID)
	if !ok {
		log.Warn().Str("run_id", runID).Str("from", fromNodeID).Msg("execute skipped: run driver did not release its slot in time")
		e.requeueRun(runID, fromNodeID)
		return
	}
	defer e.endExecute(runID, gen)
	// A node executor panic (e.g. a nil-deref in a provider/normalizer) would
	// otherwise kill this goroutine silently, stranding the run in "running"
	// forever with no driver. Recover, log with a stack, and fail the run so it
	// surfaces and can be re-run.
	defer func() {
		if r := recover(); r != nil {
			log.Error().Str("run_id", runID).Interface("panic", r).
				Bytes("stack", debug.Stack()).Msg("execute goroutine panicked; marking run failed")
			e.finish(runID, "failed")
		}
	}()

	c, err := e.loadCtx(runID)
	if err != nil {
		log.Error().Str("run_id", runID).Err(err).Msg("load run context failed")
		e.finish(runID, "failed")
		return
	}
	c.execGen = gen
	// Cancel/fail may race admission: refuse to drive a run that is already
	// terminal (e.g. Cancel landed between scheduleRunAdmission and here).
	if st := e.runStatus(runID); st == "cancelled" || st == "failed" || st == "completed" {
		log.Info().Str("run_id", runID).Str("status", st).Msg("execute aborted: run already terminal")
		return
	}

	cur := fromNodeID
	for cur != "" {
		// Cancel/ResumeFrom may have force-cleared our slot and admitted a newer
		// driver while we were blocked in RunAgent. Stop immediately — do not
		// start another node under a stolen identity.
		if !e.isExecOwner(runID, gen) {
			log.Info().Str("run_id", runID).Str("from", cur).
				Msg("execute abandoned: lost exec ownership")
			return
		}
		node := c.graph.FindNode(cur)
		if node == nil {
			e.appendTrace(c, models.TraceEntry{NodeID: cur, Event: "exit", Detail: "node not found"})
			e.finish(runID, "failed")
			return
		}
		// Snapshot variables on entering a checkpoint, for rollback restore.
		if node.Checkpoint {
			e.snapshotCheckpoint(c, node.ID)
		}
		// A new visit to this node: bump its per-node execution index and open a
		// fresh running StateRun row for it. Every subsequent write for this
		// visit (saveState / gate / conversation) is stamped with c.iter[node].
		c.iter[node.ID]++
		e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "enter", Iteration: c.iter[node.ID]})
		e.startNodeRun(c, node)

		outcome := e.executeNode(c, node)

		// Ownership first: ResumeFrom may have revived DB status to running while
		// this zombie was still inside RunAgent — gen mismatch is the SSOT.
		if !e.isExecOwner(runID, gen) {
			e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "exit", Detail: "lost exec ownership; drop late outcome"})
			log.Info().Str("run_id", runID).Str("node_id", node.ID).
				Msg("execute stopped: lost exec ownership while node was in flight")
			return
		}
		// Cancel during a long RunAgent: finish() already finalized StateRuns and
		// flipped the run terminal, but this driver was blocked in the provider.
		// Do not save completed / route onward — that would revive a cancelled run.
		if st := e.runStatus(runID); st == "cancelled" || st == "failed" {
			e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "exit", Detail: "run " + st + " during node; drop late outcome"})
			log.Info().Str("run_id", runID).Str("node_id", node.ID).Str("status", st).
				Msg("execute stopped: run became terminal while node was in flight")
			return
		}

		switch outcome.status {
		case "paused":
			// Reflect the pause on this visit's StateRun (running → waiting_human)
			// so the node reads as awaiting input, not stuck "running".
			e.saveState(c, node, outcome)
			e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "pause", Detail: outcome.outputMd})
			// Gate/react rows are visible to ResumeGate/ReactReply before this
			// unwind finishes. If a concurrent resume already resolved the
			// pause, skip waiting_human — otherwise we can clobber a queued /
			// re-admitted / completed run and strand it with no driver.
			if e.pauseStillPending(runID, node) {
				e.finish(runID, "waiting_human")
			}
			return
		case "failed":
			log.Info().Str("run_id", runID).Str("node_id", node.ID).Str("err", outcome.err).Msg("node failed")
			e.saveState(c, node, outcome)
			// React clarify + sandbox infrastructure failure: node is failed but
			// the run stays running so the operator can retry from this node.
			if node.Type == "react" && outcome.sandboxSetup {
				if outcome.err != "" {
					c.setVar("last_error", outcome.err)
					e.persistVar(runID, "last_error", outcome.err)
				}
				e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "exit", Detail: "sandbox setup failed"})
				e.refreshProgress(c)
				return
			}
			// Expose the failure reason as `last_error` so a rollback edge that
			// carries it can inject the cause into the retried upstream node's
			// prompt ({{vars.last_error}}) — the agent learns what went wrong.
			if outcome.err != "" {
				c.setVar("last_error", outcome.err)
				e.persistVar(runID, "last_error", outcome.err)
			}
			next := e.routeFailure(c, node, outcome)
			if next == "" {
				// No workflow-defined failure/rollback edge. Before failing the
				// run, auto-retry this node from its failure position for a
				// bounded number of transient/contract-style faults.
				if e.tryAutoRetry(c, node, outcome) {
					cur = node.ID
					break // leave the switch; loop re-enters node.ID as a fresh visit
				}
				e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "exit", Detail: "no failure transition; run failed"})
				e.finish(runID, "failed")
				return
			}
			cur = next
		default: // completed
			e.saveState(c, node, outcome)
			c.nodeOutputs[node.ID] = outcome.outputs
			e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "exit"})
			next := e.routeSuccess(c, node, outcome)
			cur = next
		}
		e.refreshProgress(c)
	}
	// FSM drained to a terminal point. Normally status is still "running" and we
	// promote to completed. Under a resume/pause race (CI flake class), a late
	// finish("waiting_human") from the departing pause driver can clobber the
	// re-admitted "running" status after ReactReply/ResumeGate has already driven
	// this goroutine through remaining nodes — leaving status as waiting_human or
	// queued while every node has already executed. Coerce those to completed
	// too (cancelled/failed/completed are left alone).
	var run models.Run
	if err := e.db.Select("status").First(&run, "id = ?", runID).Error; err == nil {
		switch run.Status {
		case "running", "waiting_human", "queued":
			// waiting_human/queued: a late pause finish("waiting_human") can land
			// after ResumeGate already drained the graph (CI flake class). The
			// FSM has nowhere left to go — still mark completed.
			e.finish(runID, "completed")
			log.Info().Str("run_id", runID).Msg("run completed")
		}
	}
}

// routeSuccess selects the next state after a node succeeds.
func (e *Engine) routeSuccess(c *execCtx, node *models.Node, outcome nodeOutcome) string {
	if e.IsHalted() {
		e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "exit", Detail: "shutdown: scheduler halted"})
		e.finish(c.run.ID, "cancelled")
		return ""
	}
	// branch nodes route by their computed goto target.
	if node.Type == "branch" {
		if outcome.goto_ != "" {
			e.appendTrace(c, models.TraceEntry{NodeID: node.ID, Event: "transition", To: outcome.goto_, Kind: models.EdgeSuccess})
			return outcome.goto_
		}
		return ""
	}
	// human_gate / proposal_select may carry a direct action-goto target
	// (branch-style routing off the chosen action). When absent they fall
	// through to edge-guard routing (with `action` in Extra) below.
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
			firstSuccess = &edges[i]
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
	// Fallback: take the first success edge if guards were inconclusive.
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
		c.run.Attempt-- // not consumed
		return "", false
	}
	logDB(e.db.Model(&models.Run{}).Where("id = ?", c.run.ID).UpdateColumn("attempt", c.run.Attempt), c.run.ID, "rollback attempt")
	// Restore the checkpoint variable snapshot for the target, if recorded.
	if snap, ok := c.run.Checkpoints[ed.Target]; ok {
		for k, v := range snap {
			c.setVar(k, v)
			e.persistVar(c.run.ID, k, v)
		}
	}
	// Inject carried context (e.g. last_error) so the target prompt can use it.
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
		snap[k] = v
	}
	c.run.Checkpoints[nodeID] = snap
	// Only the checkpoints column — see appendTrace on why a full-row Save is
	// unsafe and why the struct + Select form is needed for the serializer.
	logDB(e.db.Model(&models.Run{}).Where("id = ?", c.run.ID).
		Select("Checkpoints").Updates(&models.Run{Checkpoints: c.run.Checkpoints}), c.run.ID, "snapshot checkpoint")
}

func (c *execCtx) setVar(name string, v any) { c.vars[name] = v }

func (e *Engine) persistVar(runID, name string, v any) {
	var rv models.RunVariable
	if err := e.db.Where("run_id = ? AND name = ?", runID, name).First(&rv).Error; err == nil {
		rv.Value = v
		logDB(e.db.Save(&rv), runID, "persist variable")
	} else {
		logDB(e.db.Create(&models.RunVariable{RunID: runID, Name: name, Type: inferType(v), Value: v}), runID, "create variable")
	}
}

func (e *Engine) appendTrace(c *execCtx, te models.TraceEntry) {
	te.At = time.Now().Format(time.RFC3339)
	c.run.Trace = append(c.run.Trace, te)
	// Persist only the trace column. A full-row Save(c.run) here would rewrite
	// every column from the in-memory snapshot, clobbering values written via
	// dedicated column updates elsewhere (progress via refreshProgress, status
	// via finish/resume, attempt via rollback) with a stale copy. The struct +
	// Select form is required so GORM applies the field's json serializer (a
	// bare Update("trace", slice) skips it and stores nothing).
	logDB(e.db.Model(&models.Run{}).Where("id = ?", c.run.ID).
		Select("Trace").Updates(&models.Run{Trace: c.run.Trace}), c.run.ID, "append trace")
	msg, _ := json.Marshal(map[string]any{"type": "trace", "runId": c.run.ID, "entry": te})
	e.broker.Publish(c.run.ID, msg)
}

func (e *Engine) refreshProgress(c *execCtx) {
	var total, done int64
	total = int64(len(c.graph.Nodes))
	// Count DISTINCT nodes that have completed at least once: a node may now
	// have several StateRun rows (one per execution / loop iteration), and
	// counting rows would overshoot total and push progress past 100%.
	e.db.Model(&models.StateRun{}).Where("run_id = ? AND status IN ?", c.run.ID, []string{"completed", "skipped"}).
		Distinct("node_id").Count(&done)
	if total > 0 {
		c.run.Progress = float64(done) / float64(total)
		logDB(e.db.Model(&models.Run{}).Where("id = ?", c.run.ID).Update("progress", c.run.Progress), c.run.ID, "refresh progress")
	}
}

// finalizeActiveStateRuns marks every still-active StateRun for a run with the
// same terminal status as the run itself. "Active" covers both running rows
// (panic/loadCtx/node-not-found paths that finish without updating the in-flight
// StateRun) and waiting_human rows (a node paused at a human gate / react turn):
// cancelling while paused must move that node off waiting_human, otherwise the
// paused visit lingers as "awaiting input" and the auto-resume picker can't tell
// it was terminated. Normal failure paths already call saveState first, so this
// is a no-op there.
func (e *Engine) finalizeActiveStateRuns(runID, status string) {
	if status != "failed" && status != "cancelled" {
		return
	}
	errMsg := "服务重启中断,执行协程丢失,节点未收尾"
	if status == "cancelled" {
		errMsg = "run 已取消,节点未收尾"
	}
	logDB(e.db.Model(&models.StateRun{}).
		Where("run_id = ? AND status IN ?", runID, []string{"running", "waiting_human"}).
		Updates(map[string]any{"status": status, "error": errMsg}), runID, "finalize active state_runs")
}

// supersedePendingGates resolves any still-open gate for a terminated run. When a
// run is cancelled (or fails) while paused at a human gate, the pending Gate row
// is otherwise left untouched; resuming from that point re-enters the gate at a
// new iteration and opens a *second* gate — leaving two approvals (and a phantom
// pending gate surfaced on the completed run). Marking the old gate resolved
// supersedes it so only the fresh (post-resume) gate remains actionable.
func (e *Engine) supersedePendingGates(runID, status string) {
	if status != "failed" && status != "cancelled" {
		return
	}
	logDB(e.db.Model(&models.Gate{}).
		Where("run_id = ? AND resolved = ?", runID, false).
		Update("resolved", true), runID, "supersede pending gates")
}

// runStatus returns the persisted run status, or "" when the row is missing.
func (e *Engine) runStatus(runID string) string {
	var run models.Run
	if err := e.db.Select("status").First(&run, "id = ?", runID).Error; err != nil {
		return ""
	}
	return run.Status
}

// pauseStillPending reports whether the node's current pause is still awaiting
// human input. ResumeGate / ReactReply mark the gate resolved or conversation
// done before scheduling the next node; if that landed during this driver's
// pause unwind, the waiting_human transition must not apply.
func (e *Engine) pauseStillPending(runID string, node *models.Node) bool {
	switch node.Type {
	case "human_gate", "app_preview", "proposal_select":
		var gate models.Gate
		if err := e.db.Where("run_id = ? AND node_id = ?", runID, node.ID).
			Order("iteration desc, id desc").First(&gate).Error; err != nil {
			return true
		}
		return !gate.Resolved
	case "react":
		var conv models.ReactConversation
		if err := e.db.Where("run_id = ? AND node_id = ?", runID, node.ID).
			Order("iteration desc, id desc").First(&conv).Error; err != nil {
			return true
		}
		return !conv.Done
	default:
		// A review-capable producer paused in its post-run ReAct review phase
		// is pending until its review conversation is done.
		if isReviewNode(node.Type) {
			var conv models.ReactConversation
			if err := e.db.Where("run_id = ? AND node_id = ?", runID, node.ID).
				Order("iteration desc, id desc").First(&conv).Error; err != nil {
				return true
			}
			return !conv.Done
		}
		return true
	}
}

func (e *Engine) finish(runID, status string) {
	updates := map[string]any{"status": status}
	if status == "completed" {
		updates["progress"] = 1.0
	}
	// Stamp the total elapsed time from the run's start so the run list / detail
	// show a real 耗时 instead of 00:00 (Run.DurationSec was never computed).
	var run models.Run
	if e.db.First(&run, "id = ?", runID).Error == nil && !run.StartedAt.IsZero() {
		if d := int(time.Since(run.StartedAt).Seconds()); d >= 0 {
			updates["duration_sec"] = d
		}
	}
	// Gate/react rows become externally actionable before this pause finish
	// runs (create gate → saveState → finish). A concurrent ResumeGate /
	// ReactReply can already have queued or re-admitted the run in that
	// window; an unconditional waiting_human write would clobber that and
	// strand the run with a resolved gate and no driver.
	q := e.db.Model(&models.Run{}).Where("id = ?", runID)
	if status == "waiting_human" {
		// If this run already had gate/react pause rows and every one is
		// resolved/done, a concurrent ResumeGate/ReactReply continued the
		// run — skip a late pause unwind so we do not clobber re-admitted
		// "running" (or queued) back to waiting_human.
		var totalGates, totalReact int64
		_ = e.db.Model(&models.Gate{}).Where("run_id = ?", runID).Count(&totalGates).Error
		_ = e.db.Model(&models.ReactConversation{}).Where("run_id = ?", runID).Count(&totalReact).Error
		if totalGates > 0 || totalReact > 0 {
			var pendingGates, pendingReact int64
			_ = e.db.Model(&models.Gate{}).
				Where("run_id = ? AND resolved = ?", runID, false).Count(&pendingGates).Error
			_ = e.db.Model(&models.ReactConversation{}).
				Where("run_id = ? AND done = ?", runID, false).Count(&pendingReact).Error
			if pendingGates == 0 && pendingReact == 0 {
				return
			}
		}
		q = q.Where("status = ?", "running")
	}
	res := q.Updates(updates)
	logDB(res, runID, "finish run")
	if status == "waiting_human" && (res.Error != nil || res.RowsAffected == 0) {
		return
	}
	if status == "failed" || status == "cancelled" {
		e.finalizeActiveStateRuns(runID, status)
		e.supersedePendingGates(runID, status)
	}
	if status == "completed" || status == "failed" || status == "cancelled" {
		e.host.UnregisterRun(runID)
		// Release any live react session (and its sandbox) still held for this
		// run. For completed runs the session is already closed, so this is a
		// no-op; for cancel/fail while paused at a react node it prevents the
		// sandbox from lingering forever as "busy".
		if ab, ok := e.provider.(runtime.RunAborter); ok {
			ab.AbortRun(runID)
		}
		e.mu.Lock()
		delete(e.tokens, runID)
		e.mu.Unlock()
	}
	msg, _ := json.Marshal(map[string]any{"type": "status", "runId": runID, "status": status})
	e.broker.Publish(runID, msg)
}

// logDB records a GORM write error on an engine best-effort persistence path.
// The FSM keeps driving the run from its in-memory context, but a silent DB
// write failure previously desynced the persisted run / UI from reality with no
// trace at all; this surfaces it in the logs so it can be diagnosed. Control
// flow is intentionally unchanged — these writes are recovery-friendly (a
// resume reloads from the DB) and aborting mid-transition would be worse.
func logDB(res *gorm.DB, runID, op string) {
	if res != nil && res.Error != nil {
		log.Error().Str("run_id", runID).Str("op", op).Err(res.Error).Msg("engine db write failed")
	}
}

func inferType(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case int, int64, float64:
		return "int"
	default:
		return "string"
	}
}
