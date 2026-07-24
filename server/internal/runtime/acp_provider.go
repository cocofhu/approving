package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
	"github.com/cocofhu/approving/internal/sandbox"
	"github.com/cocofhu/approving/internal/textutil"

	"github.com/rs/zerolog/log"
)

// sandboxManager is the sandbox-creation seam the provider depends on. The
// real backend is *sandbox.Manager (Docker); tests inject a fake that returns
// sandboxes wired to an in-process fake ACP bridge, so the provider's retry /
// idle / react-rehydrate / harvest logic is exercised without Docker.
type sandboxManager interface {
	Create(ctx context.Context, spec sandbox.Spec) (*sandbox.Sandbox, error)
}

// errSandboxSetup wraps a failure to acquire a working sandbox + ACP session
// (container create / ACP-ready wait / connect handshake). It is a retryable
// infrastructure fault: a fresh attempt in a new container often succeeds.
var errSandboxSetup = errors.New("sandbox setup failed")

// isRetryableSandboxErr reports whether err is a transient sandbox/ACP fault
// worth retrying in a fresh container: setup failures, a connection that
// dropped mid-turn, or an idle (stuck) turn. Agent-reported errors, the hard
// chat deadline (DeadlineExceeded), and contract misses are NOT retryable —
// re-running them would just waste a sandbox and likely fail identically.
func isRetryableSandboxErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errSandboxSetup) ||
		errors.Is(err, sandbox.ErrConnClosed) ||
		errors.Is(err, sandbox.ErrChatIdle)
}

// isChatTimeoutErr reports whether err is the per-turn chat deadline (context
// deadline exceeded). Used to distinguish timeout truncation from contract misses
// in nudge re-prompt paths.
func isChatTimeoutErr(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// acpProvider is the ACP + Docker sandbox backend. Each agent/react node runs
// in a fresh container launched from the preset image
// (universal-sandbox-cursor:local). The in-container ACP agent is
// driven via the acp-bridge WebSocket bridge (ACP JSON-RPC). Declared produces
// are harvested from the container workspace and written through the run-scoped
// artifact-store MCP, satisfying the produces contract the engine enforces.
//
// Each node gets a per-node config-home mount (rules/skills + mcp.json) built
// from the Agent config. The run-scoped artifact-store MCP is not
// auto-injected: an Agent opts in by convention (an MCP named "artifact-store"),
// and the platform fills its run-scoped URL + token so artifacts stay isolated
// per run.
type acpProvider struct {
	host    *mcp.Host
	opts    Options
	mgr     sandboxManager
	backend AcpBackend

	// registry, when set, records each per-run node sandbox in the platform
	// sandbox store so it shows up in the sandbox UI while the node runs.
	registry SandboxRegistry

	// emit, when set by the engine, publishes a node's in-progress ACP events
	// (the same []models.AcpEvent shape that is persisted on completion) so the
	// run detail UI can show a live agent preview while the turn runs. busy is
	// the authoritative queue_state.busy flag (true while the agent is actively
	// processing the turn) used to drive the running/idle indicator.
	emit func(runID, nodeID string, events []models.AcpEvent, busy bool)

	mu       sync.Mutex
	sessions map[string]*reactSession    // runID|nodeID -> live react session
	live     map[string]*sandbox.Sandbox // runID|nodeID -> in-flight sandbox (for live event-log reads)
	// inflightACP tracks the ACP client for in-flight agent turns (not parked
	// in sessions). AbortRun closes these so Cancel-during-agent unblocks
	// streamChat instead of waiting out ChatTimeout.
	inflightACP map[string]*sandbox.ACPClient
}

// registerLive records a node's sandbox so its event log can be read straight
// from the container while the turn runs (survives a UI refresh). deregisterLive
// drops it when the sandbox is torn down. acp may be nil for callers that only
// need the event-log handle (react sessions already keep acp elsewhere).
func (c *acpProvider) registerLive(req NodeReq, sb *sandbox.Sandbox, acp *sandbox.ACPClient) {
	c.mu.Lock()
	key := reactKey(req)
	c.live[key] = sb
	if acp != nil {
		c.inflightACP[key] = acp
	}
	c.mu.Unlock()
}

func (c *acpProvider) deregisterLive(req NodeReq) {
	c.mu.Lock()
	key := reactKey(req)
	delete(c.live, key)
	delete(c.inflightACP, key)
	c.mu.Unlock()
}

// LiveNodeEvents reads the in-flight sandbox's full event log directly and
// returns it as the run timeline. ok=false, err=nil when the node is not
// currently running in a live sandbox (finished / never started here), so
// callers fall back to the persisted snapshot. When a live sandbox is
// registered but the bridge read fails, ok=false with a non-nil err so
// callers can surface a distinguishable failure (never pretend live+empty).
func (c *acpProvider) LiveNodeEvents(ctx context.Context, runID, nodeID string) ([]models.AcpEvent, bool, error) {
	c.mu.Lock()
	sb := c.live[runID+"|"+nodeID]
	c.mu.Unlock()
	if sb == nil {
		return nil, false, nil
	}
	res, _, err := sandbox.FetchEventLog(ctx, sb.Host, sb.Port)
	if err != nil {
		return nil, false, err
	}
	if res == nil {
		return nil, false, fmt.Errorf("event log unavailable")
	}
	return res.AcpEvents(), true, nil
}

// LiveNodeEventsPage reads a page of events from the in-flight sandbox.
// Fetch failures return ok=false with a non-nil err (aligned with
// LiveNodeEvents) — never live=true with an empty page that masks the error.
func (c *acpProvider) LiveNodeEventsPage(ctx context.Context, runID, nodeID, cursor string, limit int) ([]models.AcpEvent, string, bool, bool, error) {
	c.mu.Lock()
	sb := c.live[runID+"|"+nodeID]
	c.mu.Unlock()
	if sb == nil {
		return nil, "", false, false, nil
	}
	page, err := sandbox.FetchEventLogPage(ctx, sb.Host, sb.Port, cursor, limit)
	if err != nil {
		return nil, "", false, false, err
	}
	if page == nil {
		return nil, "", false, false, fmt.Errorf("event log page unavailable")
	}
	return sandbox.AggregateFrames(page.Events), page.NextCursor, page.HasMore, true, nil
}

// snapshotEvents captures the full agent event log straight from the sandbox so
// it can be persisted as the node's StateRun snapshot BEFORE the sandbox is
// destroyed — that saved snapshot is the only record once the container is
// gone. Best-effort: falls back to the streamed aggregation if the read fails.
func (c *acpProvider) snapshotEvents(ctx context.Context, sb *sandbox.Sandbox, fallback []models.AcpEvent) []models.AcpEvent {
	if sb == nil {
		return fallback
	}
	snap, _, err := sandbox.FetchEventLog(ctx, sb.Host, sb.Port)
	if err != nil || snap == nil {
		return fallback
	}
	if se := snap.AcpEvents(); len(se) > 0 {
		return se
	}
	return fallback
}

// SetEventSink wires a live-event publisher (the engine's broker). Optional;
// when nil the provider falls back to a single blocking turn.
func (c *acpProvider) SetEventSink(fn func(runID, nodeID string, events []models.AcpEvent, busy bool)) {
	c.emit = fn
}

// SetSandboxRegistry wires the platform sandbox store so per-run node
// sandboxes are recorded (and shown in the UI) for their lifetime.
func (c *acpProvider) SetSandboxRegistry(r SandboxRegistry) {
	c.registry = r
}

// beginRunSandbox records a "creating" placeholder row for a node sandbox before
// the (slow) gateway provisioning, so it shows up in the sandbox list / node
// live log as "starting" instead of a 404. No-op when the registry is absent or
// does not implement RunSandboxBeginner (e.g. test fakes → legacy behavior).
func (c *acpProvider) beginRunSandbox(req NodeReq, name, home string) {
	if c.registry == nil || name == "" {
		return
	}
	b, ok := c.registry.(RunSandboxBeginner)
	if !ok {
		return
	}
	b.BeginRunSandbox(RunSandboxInfo{
		Name:         name,
		Profile:      str2(req.Config["skill_profile"]),
		RunID:        req.RunID,
		WorkflowID:   req.WorkflowID,
		WorkflowName: req.WorkflowName,
		NodeID:       req.NodeID,
		RepoURL:      firstRepoURL(req),
		HomeDir:      home,
		Token:        req.Token,
	})
}

// registerRunSandbox records a freshly-created node sandbox in the platform
// store. No-op when no registry is wired.
func (c *acpProvider) registerRunSandbox(req NodeReq, sb *sandbox.Sandbox, home string) {
	if c.registry == nil || sb == nil {
		return
	}
	repo := firstRepoURL(req)
	c.registry.RegisterRunSandbox(RunSandboxInfo{
		Name:           sb.Name,
		Profile:        str2(req.Config["skill_profile"]),
		RunID:          req.RunID,
		WorkflowID:     req.WorkflowID,
		WorkflowName:   req.WorkflowName,
		NodeID:         req.NodeID,
		Host:           sb.Host,
		ACPPort:        sb.Port,
		CodeServerPort: sb.CodeServerPort,
		RepoURL:        repo,
		HomeDir:        home,
		Token:          req.Token,
	})
}

// deregisterRunSandbox clears a node sandbox record once its container is
// torn down. No-op when no registry is wired.
func (c *acpProvider) deregisterRunSandbox(name string) {
	if c.registry == nil || name == "" {
		return
	}
	c.registry.UnregisterRunSandbox(name)
}

// retireRunSandbox is the end-of-node teardown for a per-run sandbox. Rather
// than destroying the container immediately, it closes the driving ACP session
// but keeps the container (and its cursor-home mount) alive and hands it to the
// store's idle-TTL sweeper, so the sandbox can be inspected (terminal / IDE /
// ACP / container logs) for debugging. When the store can't retire (no registry
// or capability — e.g. tests), it falls back to an immediate destroy so nothing
// is leaked.
func (c *acpProvider) retireRunSandbox(sb *sandbox.Sandbox, acp *sandbox.ACPClient, home string) {
	if acp != nil {
		acp.Close()
	}
	if sb == nil {
		removeHome(home)
		return
	}
	if r, ok := c.registry.(RunSandboxRetirer); ok {
		// Store owns the container from here: it keeps it for the debug TTL and
		// removes the home dir when it finally destroys the container.
		r.RetireRunSandbox(sb.Name)
		return
	}
	// No retirer available: destroy immediately (legacy behavior).
	c.deregisterRunSandbox(sb.Name)
	sb.Destroy(context.Background())
	removeHome(home)
}

// streamChat runs one turn (prompt + optional image attachments), streaming
// incremental events to the sink when one is configured; otherwise it falls
// back to a single blocking aggregation.
func (c *acpProvider) streamChat(ctx context.Context, acp *sandbox.ACPClient, req NodeReq, prompt string, images []models.PromptImage) (*sandbox.ChatResult, error) {
	if c.emit == nil {
		return acp.ChatStructured(ctx, prompt, images)
	}
	return acp.ChatStreamResult(ctx, prompt, images, func(r *sandbox.ChatResult) {
		// Until a queue_state frame reports otherwise, an in-flight turn is
		// busy by definition (the prompt RPC is blocked in the bridge).
		busy := true
		if r.BusySet {
			busy = r.Busy
		}
		c.emit(req.RunID, req.NodeID, chatResultToEvents(r), busy)
	})
}

// absorbChat folds one turn's prompt_done.usage into the StateRun-scoped
// accumulator. Only per-turn ChatResult.Usage is used — never session
// CumulativeUsage — so cross-node session reuse cannot inflate a later node.
func absorbChat(usage **models.TokenUsage, events *[]models.AcpEvent, res *sandbox.ChatResult) {
	if res == nil {
		return
	}
	if events != nil {
		*events = append(*events, chatResultToEvents(res)...)
	}
	if usage != nil {
		*usage = models.AddTokenUsage(*usage, res.Usage)
	}
}

// reactSession keeps a sandbox + ACP connection alive across the human
// think-time of a multi-turn react dialogue (open → reply → … → done).
type reactSession struct {
	sb   *sandbox.Sandbox
	acp  *sandbox.ACPClient
	home string // temp /root/.cursor host dir to clean up
}

func newACPProvider(host *mcp.Host, opts Options) ExecProvider {
	return newBaseACPProvider(host, opts, BackendCursor)
}

func newBaseACPProvider(host *mcp.Host, opts Options, backend AcpBackend) ExecProvider {
	backend = NormalizeBackend(string(backend))
	image := resolveProviderImage(opts, backend)
	gw := sandbox.NewGatewayClient(opts.GatewayURL, opts.GatewayAPIKey)
	mgr := sandbox.NewManager(gw, sandbox.ManagerOptions{
		Image:           image,
		WorkspaceDir:    "/root/workspace",
		InstallHelpers:  true,
		InjectStore:     opts.InjectStore,
		InjectAdvertise: opts.MCPEndpoint,
		CreateTimeout:   opts.SandboxCreateTimeout,
	})
	log.Info().Str("image", mgr.Image).Str("gateway", opts.GatewayURL).Str("acpBackend", string(backend)).
		Str("bridge", AgentRuntimeLabel(backend)).Msg("sandbox exec provider ready")
	return &acpProvider{host: host, opts: opts, mgr: mgr, backend: backend,
		sessions: map[string]*reactSession{}, live: map[string]*sandbox.Sandbox{},
		inflightACP: map[string]*sandbox.ACPClient{}}
}

// resolveProviderImage picks the sandbox image for one acpBackend.
func resolveProviderImage(opts Options, backend AcpBackend) string {
	if img := strings.TrimSpace(opts.SandboxImage); img != "" {
		return img
	}
	b := string(NormalizeBackend(string(backend)))
	if opts.SandboxImages != nil {
		if img := strings.TrimSpace(opts.SandboxImages[b]); img != "" {
			return img
		}
	}
	return config.DefaultSandboxImage(b)
}

func (c *acpProvider) Name() string {
	if c.backend == "" {
		return string(BackendCursor)
	}
	return string(c.backend)
}

func (c *acpProvider) chatTimeout() time.Duration {
	if c.opts.ChatTimeout > 0 {
		return c.opts.ChatTimeout
	}
	return 10 * time.Minute
}

// nodeChatTimeout is the hard per-turn deadline for a node, honoring a per-node
// override before falling back to the global budget. Lets a heavy node (e.g.
// implement) get more headroom than a quick research. Two override keys are
// accepted: the editor card field `timeout` (minutes) and the legacy
// `chat_timeout` (seconds); `chat_timeout` wins when both are set.
func (c *acpProvider) nodeChatTimeout(req NodeReq) time.Duration {
	if v, ok := toInt(req.Config["chat_timeout"]); ok && v > 0 {
		return time.Duration(v) * time.Second
	}
	if v, ok := toInt(req.Config["timeout"]); ok && v > 0 {
		return time.Duration(v) * time.Minute
	}
	return c.chatTimeout()
}

// sandboxAttempts is the total number of node attempts (>=1) before a retryable
// sandbox fault gives up and the node fails for real.
func (c *acpProvider) sandboxAttempts() int {
	if c.opts.SandboxMaxAttempts > 1 {
		return c.opts.SandboxMaxAttempts
	}
	return 1
}

// backoff waits before the next sandbox retry (exponential from the configured
// base, capped at 30s), aborting early if ctx is cancelled. Returns false when
// ctx ended so the caller stops retrying.
func (c *acpProvider) backoff(ctx context.Context, attempt int) bool {
	base := c.opts.SandboxRetryBackoff
	if base <= 0 {
		base = 2 * time.Second
	}
	d := base << (attempt - 1)
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// discardSandbox tears down a broken sandbox between retries: close the ACP
// session, drop its registry/live bookkeeping, destroy the container, and
// remove its cursor-home. Unlike retireRunSandbox it does NOT keep the
// container for debugging — a faulted sandbox has no value and would leak.
// Best-effort snapshotEvents runs before destroy so ACP events are not lost
// when the container is recycled.
func (c *acpProvider) discardSandbox(ctx context.Context, req NodeReq, sb *sandbox.Sandbox, acp *sandbox.ACPClient, home string, fallback []models.AcpEvent) []models.AcpEvent {
	events := c.snapshotEvents(ctx, sb, fallback)
	c.deregisterLive(req)
	if acp != nil {
		acp.Close()
	}
	if sb == nil {
		removeHome(home)
		return events
	}
	c.deregisterRunSandbox(sb.Name)
	sb.Destroy(context.Background())
	removeHome(home)
	return events
}

// emitRetryNotice logs a retryable sandbox fault and, when a live event sink is
// wired, pushes a visible message into the node's event stream so the retry is
// observable in the execution log (without creating a separate execution row).
func (c *acpProvider) emitRetryNotice(req NodeReq, attempt, max int, err error) {
	log.Warn().Str("run", req.RunID).Str("node", req.NodeID).
		Int("attempt", attempt).Int("max", max).Err(err).
		Msg("retryable sandbox fault; retrying in a fresh sandbox")
	if c.emit != nil {
		c.emit(req.RunID, req.NodeID, []models.AcpEvent{{
			Kind:  "message",
			Title: "沙箱异常,自动重试",
			Text:  fmt.Sprintf("第 %d/%d 次尝试遇到沙箱/ACP 故障(%s),正在换新沙箱重试…", attempt, max, err.Error()),
		}}, true)
	}
}

// RunAgent executes an autonomous agent node end-to-end, transparently
// retrying in a fresh sandbox on a retryable sandbox/ACP fault (create/ready/
// connect failure, mid-turn connection drop, or idle stall). Non-retryable
// outcomes (agent errors, hard timeout, contract misses) return immediately so
// the engine's FSM handles them. Retries are logged/emitted but do not create a
// separate execution record.
func (c *acpProvider) RunAgent(ctx context.Context, req NodeReq) (NodeResult, error) {
	n := c.sandboxAttempts()
	for attempt := 1; ; attempt++ {
		res, err := c.runAgentOnce(ctx, req)
		if err == nil || !isRetryableSandboxErr(err) || attempt >= n || ctx.Err() != nil {
			return res, err
		}
		c.emitRetryNotice(req, attempt, n, err)
		if !c.backoff(ctx, attempt) {
			return res, err
		}
	}
}

// runAgentOnce is a single sandbox attempt of an agent node. It owns its
// sandbox lifecycle: on success or a non-retryable outcome it retires the
// sandbox (kept for the debug TTL); on a retryable fault it discards (destroys)
// the broken sandbox so the caller can retry cleanly.
func (c *acpProvider) runAgentOnce(ctx context.Context, req NodeReq) (res NodeResult, err error) {
	// Each sandbox attempt must earn its own node_complete. Drop any mark left
	// by a prior attempt that returned an error before TakeOutcome ran.
	c.host.ClearOutcome(req.RunID, req.NodeID)

	sb, acp, home, err := c.openSandbox(ctx, req)
	if err != nil {
		// openSandbox already destroyed the container/home on its own failure.
		return NodeResult{}, err
	}
	// Keep the sandbox alive after the node finishes (retire with a debug TTL)
	// instead of destroying it immediately, so it can be inspected for
	// troubleshooting; the store's sweeper reclaims it later. A retryable fault
	// flips this to a hard discard so a broken sandbox never leaks.
	keepForDebug := true
	// parked flips true when the node opts into KeepAliveForReview and the turn
	// succeeded: the live session is handed to c.sessions (owned by the review
	// phase / a downstream gate) instead of being retired or discarded here.
	parked := false
	var turnEvents []models.AcpEvent
	defer func() {
		if parked {
			return // session ownership transferred to c.sessions
		}
		if keepForDebug {
			c.retireRunSandbox(sb, acp, home)
		} else {
			snap := c.discardSandbox(context.Background(), req, sb, acp, home, turnEvents)
			if len(res.Events) == 0 && len(snap) > 0 {
				res.Events = snap
			}
		}
	}()
	c.registerLive(req, sb, acp)
	defer func() {
		if !parked {
			c.deregisterLive(req)
		}
	}()

	seeded := c.upstreamArtifacts(req)
	chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
	defer cancel()
	var chatRes *sandbox.ChatResult
	var usage *models.TokenUsage
	chatRes, err = c.streamChat(chatCtx, acp, req, c.buildAgentPrompt(req, seeded), req.PromptImages)
	if err != nil {
		if isRetryableSandboxErr(err) {
			keepForDebug = false
		}
		events := c.snapshotEvents(ctx, sb, turnEvents)
		return NodeResult{Events: events, Usage: usage}, fmt.Errorf("agent chat: %w", err)
	}
	absorbChat(&usage, &turnEvents, chatRes)
	out := map[string]any{"content": chatRes.Narration, "narration_summary": firstLine(chatRes.Narration)}

	// Implement nodes must drive the run plan to completion: if items remain,
	// re-prompt the agent (same session) to finish them, then fail the node if
	// it still can't. Folds any re-prompt turns into the event log.
	if req.NodeType == "implement" {
		if perr := c.ensurePlanComplete(ctx, req, acp, &turnEvents, &usage); perr != nil {
			events := c.snapshotEvents(ctx, sb, turnEvents)
			return NodeResult{OutputMd: chatRes.Narration, Outputs: out, Events: events, Usage: usage}, perr
		}
	}

	if req.NodeType == "app_preview" {
		if perr := c.ensurePreviewRegistered(ctx, req, acp, &turnEvents, &usage); perr != nil {
			events := c.snapshotEvents(ctx, sb, turnEvents)
			return NodeResult{OutputMd: chatRes.Narration, Outputs: out, Events: events, Usage: usage}, perr
		}
	}

	// Framework nodes whose deliverable is a reserved structured product
	// (implement result / research / test / review / proposals) get a
	// best-effort re-prompt to write it via the naming set_* tool; the engine
	// still fails the node if it is ultimately absent.
	if name, tool := structuredArtifactFor(req.NodeType); name != "" {
		if serr := c.ensureStructured(ctx, req, acp, name, tool, &turnEvents, &usage); serr != nil {
			events := c.snapshotEvents(ctx, sb, turnEvents)
			return NodeResult{OutputMd: chatRes.Narration, Outputs: out, Events: events, Usage: usage}, serr
		}
	}

	// Persist the full sandbox event log as the snapshot before teardown.
	events := c.snapshotEvents(ctx, sb, turnEvents)

	if produces := str2(req.Config["produces"]); produces != "" {
		// The agent may satisfy the contract either by calling write_artifact
		// (MCP → run store) or by writing the file into the workspace. Accept
		// the MCP path first and only harvest the workspace file as a fallback,
		// so a prompt that says "用 MCP 写产物,不要写进项目" still succeeds.
		if _, rerr := c.host.ReadArtifact(req.RunID, req.Token, produces); rerr != nil {
			if err := c.harvest(ctx, sb, req, produces, out, &events); err != nil {
				// Surface the partial result but report the contract miss so the
				// engine can route failure/rollback per the FSM definition.
				return NodeResult{OutputMd: chatRes.Narration, Outputs: out, Events: events, Usage: usage}, err
			}
		}
	}

	// Implement nodes must land on the remote: downstream nodes run in fresh
	// clones, so uncommitted/unpushed work is invisible to them. Commit any
	// leftover changes and push the working branch (best-effort) before we
	// snapshot the change report, so `pushed`/`branch` reflect reality.
	if req.NodeType == "implement" {
		c.ensurePushed(ctx, sb, req)
	}

	// Always report code changes via the sandbox protocol (VCS-neutral), so
	// every workflow Agent emits branch/commit/diff for downstream decisions —
	// independent of GitLab. This also fills res.Git.Branch (the engine writes
	// it to Run.branch) without needing detect_push.
	git := c.captureChanges(ctx, sb, req, out)

	// Optional GitLab reference extension: push detection + MR creation, layered
	// on top of the protocol change report. Not part of the core protocol.
	if b, _ := req.Config["detect_push"].(bool); b {
		if p := c.detectPush(ctx, sb, req); p != nil {
			if git == nil {
				git = &GitInfo{}
			}
			git.Pushed = p.Pushed
			git.PushedSHA = p.PushedSHA
			if p.Branch != "" {
				git.Branch = p.Branch
				out["branch"] = p.Branch
			}
			if p.MrURL != "" {
				git.MrURL = p.MrURL
				out["mr_url"] = p.MrURL
			}
			out["pushed_sha"] = p.PushedSHA
		}
	}

	// All agent nodes must call node_complete before the engine will accept
	// completion. Soft re-prompt here; the engine still fails if the mark is
	// ultimately absent. submit_mr no longer runs verifyMR (git/glab) — the
	// agent attests via node_complete outputs instead.
	if oerr := c.ensureOutcome(ctx, req, acp, &events, &usage); oerr != nil {
		return NodeResult{OutputMd: chatRes.Narration, Outputs: out, Events: events, Git: git, Usage: usage}, oerr
	}

	// Post-run ReAct review handoff: keep the live sandbox + ACP session alive
	// (parked in c.sessions) instead of retiring it, so the engine can drive an
	// interactive review — or a downstream approval gate's ReAct reject — in the
	// SAME sandbox. The session is retired later on review finish / gate approve
	// / run abort (closeSession / RetireSession / AbortRun).
	if req.KeepAliveForReview {
		key := reactKey(req)
		c.mu.Lock()
		c.sessions[key] = &reactSession{sb: sb, acp: acp, home: home}
		c.live[key] = sb
		delete(c.inflightACP, key)
		c.mu.Unlock()
		parked = true
		log.Info().Str("run", req.RunID).Str("node", req.NodeID).
			Msg("parked live session for post-run ReAct review")
	}

	return NodeResult{OutputMd: chatRes.Narration, Outputs: out, Events: events, Git: git, Usage: usage}, nil
}

// collectChanges builds the VCS-neutral change report over the SSH data plane,
// replacing the universal-sandbox's absent GET /api/changes. It runs git itself
// (via sandbox.GitChanges) rather than asking an in-container HTTP service:
//   - single-repo mode when the workspace root is itself a git repo (vcs:"git");
//   - otherwise multi-repo (flat) mode: one entry per configured repo under
//     /root/workspace/<name> that is a git repo (vcs:"multi").
//
// Returns {vcs:"none"} when nothing under the workspace is version-controlled.
func (c *acpProvider) collectChanges(ctx context.Context, sb *sandbox.Sandbox, req NodeReq) *sandbox.Changes {
	ws := sb.WorkspaceDir
	if ws == "" {
		ws = "/root/workspace"
	}
	if ch, ok := sb.GitChanges(ctx, ws); ok {
		return ch
	}
	var repos []sandbox.RepoChanges
	for _, r := range resolveRepos(req) {
		dir := repoWorkspacePath(r.Name)
		if rc, ok := sb.GitChanges(ctx, dir); ok {
			repos = append(repos, sandbox.RepoChanges{Name: r.Name, Path: dir, Changes: *rc})
		}
	}
	if len(repos) == 0 {
		return &sandbox.Changes{VCS: "none"}
	}
	return &sandbox.Changes{VCS: "multi", Repos: repos}
}

// captureChanges fetches the sandbox's VCS-neutral code-change report and maps
// it into the node outputs. The report is computed by approving over SSH (see
// collectChanges); the sandbox image exposes no change endpoint.
// Returns a *GitInfo carrying the branch so the engine can record Run.branch.
// Best-effort: any error degrades to no change report (returns nil).
// req is used in single-repo mode to emit vars.branches-compatible
// outputs["branches"] when the run has exactly one configured repo.
func (c *acpProvider) captureChanges(ctx context.Context, sb *sandbox.Sandbox, req NodeReq, out map[string]any) *GitInfo {
	ch := c.collectChanges(ctx, sb, req)
	if ch == nil || ch.VCS == "" || ch.VCS == "none" {
		return nil
	}
	// Multi-repo (flat) mode: no single top-level branch. Emit a per-repo change
	// list plus a name→branch map (`branches`) so downstream fresh clones can
	// check out each repo's working branch.
	if ch.VCS == "multi" {
		return captureMultiRepoChanges(ch, out)
	}
	out["branch"] = ch.Branch
	out["base_branch"] = ch.BaseBranch
	out["new_branch"] = ch.NewBranch
	out["commit_sha"] = ch.HeadSHA
	out["base_sha"] = ch.BaseSHA
	out["dirty"] = ch.Dirty
	out["commit_count"] = len(ch.Commits)
	out["changed_files_count"] = len(ch.ChangedFiles)
	out["diff_stat"] = ch.DiffStat
	// Push state — key downstream signal (CI/MR gating). Always emitted so a
	// `when` guard can branch on whether the work actually reached the remote.
	out["pushed"] = ch.Pushed
	out["unpushed_count"] = ch.Unpushed
	if ch.Pushed {
		out["pushed_sha"] = ch.HeadSHA
	}
	if len(ch.ChangedFiles) > 0 {
		out["changed_files"] = ch.ChangedFiles
	}
	// Emit name→branch so exportBranchVar / resolveRepos stay on vars.branches
	// even for a lone repo reported as vcs:"git" (workspace root is the repo).
	if ch.Branch != "" {
		if repos := resolveRepos(req); len(repos) == 1 {
			if b, err := json.Marshal(map[string]string{repos[0].Name: ch.Branch}); err == nil {
				out["branches"] = string(b)
			}
		}
	}
	if ch.Branch == "" {
		return nil
	}
	git := &GitInfo{Branch: ch.Branch, Pushed: ch.Pushed}
	if ch.Pushed {
		git.PushedSHA = ch.HeadSHA
	}
	return git
}

// captureMultiRepoChanges maps a multi-repo (flat) change report into node
// outputs: a per-repo change list (`repos_changes`), a name→branch map
// (`branches`) for downstream continuity, and an aggregate `pushed` flag (true
// only when every repo with changes has been pushed). Returns a *GitInfo whose
// Repos carries each repo's branch/pushed.
func captureMultiRepoChanges(ch *sandbox.Changes, out map[string]any) *GitInfo {
	if len(ch.Repos) == 0 {
		return nil
	}
	branches := map[string]string{}
	repoOut := make([]map[string]any, 0, len(ch.Repos))
	git := &GitInfo{}
	allPushed := true
	anyChange := false
	for _, r := range ch.Repos {
		if r.Branch != "" {
			branches[r.Name] = r.Branch
		}
		changed := len(r.ChangedFiles) > 0 || len(r.Commits) > 0 || r.Dirty
		if changed {
			anyChange = true
			if !r.Pushed {
				allPushed = false
			}
		}
		rg := RepoGit{Name: r.Name, Branch: r.Branch, Pushed: r.Pushed}
		if r.Pushed {
			rg.PushedSHA = r.HeadSHA
		}
		git.Repos = append(git.Repos, rg)
		repoOut = append(repoOut, map[string]any{
			"name":                r.Name,
			"path":                r.Path,
			"branch":              r.Branch,
			"base_branch":         r.BaseBranch,
			"new_branch":          r.NewBranch,
			"commit_sha":          r.HeadSHA,
			"base_sha":            r.BaseSHA,
			"dirty":               r.Dirty,
			"commit_count":        len(r.Commits),
			"changed_files_count": len(r.ChangedFiles),
			"diff_stat":           r.DiffStat,
			"pushed":              r.Pushed,
			"unpushed_count":      r.Unpushed,
			"changed_files":       r.ChangedFiles,
		})
	}
	out["repos_changes"] = repoOut
	if b, err := json.Marshal(branches); err == nil {
		out["branches"] = string(b)
	}
	git.Pushed = anyChange && allPushed
	out["pushed"] = git.Pushed
	if len(git.Repos) == 0 {
		return nil
	}
	return git
}

// ReactOpen launches a sandbox, opens the dialogue and keeps the session
// alive (stored in c.sessions) until the dialogue completes. The opening turn
// can itself finish the node (e.g. the agent asks nothing) — handled uniformly
// via finishReact.
func (c *acpProvider) ReactOpen(ctx context.Context, req NodeReq) ReactTurn {
	n := c.sandboxAttempts()
	seeded := c.upstreamArtifacts(req)
	for attempt := 1; ; attempt++ {
		// Fresh attempt: do not reuse a node_complete from a failed open/chat try.
		c.host.ClearOutcome(req.RunID, req.NodeID)
		sb, acp, home, err := c.openSandbox(ctx, req)
		if err == nil {
			chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
			var res *sandbox.ChatResult
			res, err = c.streamChat(chatCtx, acp, req, c.buildReactOpenPrompt(req, seeded), req.PromptImages)
			cancel()
			if err == nil {
				key := reactKey(req)
				sess := &reactSession{sb: sb, acp: acp, home: home}
				c.mu.Lock()
				c.sessions[key] = sess
				c.live[key] = sb
				c.mu.Unlock()
				qs := c.host.TakePendingQuestions(req.RunID, req.NodeID)
				var usage *models.TokenUsage
				var events []models.AcpEvent
				absorbChat(&usage, &events, res)
				// Opening turn with questions: pause for the human to choose.
				if len(qs) > 0 && !reactCapReached(req, nil) {
					return ReactTurn{Msg: res.Narration, Questions: qs, Events: events, Usage: usage}
				}
				// No questions on the opening turn ⇒ clarification is concluded.
				return c.finishReact(ctx, req, key, sess, res.Narration, nil, events, usage)
			}
			// Chat failed. Retryable → discard the broken sandbox and loop;
			// otherwise retain it (debug TTL) and surface the failure.
			if isRetryableSandboxErr(err) {
				c.discardSandbox(ctx, req, sb, acp, home, nil)
			} else {
				c.retireRunSandbox(sb, acp, home)
				log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).
					Msg("react open chat failed")
				return ReactTurn{
					Msg: "(澄清开场失败:" + err.Error() + ")",
					Events: []models.AcpEvent{{
						Kind: "message", Text: "react open chat failed: " + err.Error(),
					}},
				}
			}
		}
		// A retryable open/chat fault: back off and retry in a fresh sandbox.
		if isRetryableSandboxErr(err) && attempt < n && ctx.Err() == nil {
			c.emitRetryNotice(req, attempt, n, err)
			if c.backoff(ctx, attempt) {
				continue
			}
		}
		return ReactTurn{SetupErr: err, Msg: "(沙箱启动失败,无法开始澄清:" + err.Error() + ")",
			Events: []models.AcpEvent{{Kind: "message", Text: "react open failed: " + err.Error()}}}
	}
}

// rehydrateReact rebuilds a lost react session — server restart dropped the
// in-memory session, or its sandbox/ACP died during the human's think-time — in
// a fresh sandbox, re-priming the agent with the persisted Q&A transcript so it
// can continue coherently (its private reasoning is gone, but the visible
// dialogue is restored). Returns nil (sandbox cleaned up) when the rebuild
// itself fails, so the caller surfaces a retryable state rather than silently
// finishing the node with an empty deliverable.
func (c *acpProvider) rehydrateReact(ctx context.Context, req NodeReq, history []models.ReactMessage) *reactSession {
	n := c.sandboxAttempts()
	seeded := c.upstreamArtifacts(req)
	for attempt := 1; ; attempt++ {
		sb, acp, home, err := c.openSandbox(ctx, req)
		if err == nil {
			chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
			_, err = c.streamChat(chatCtx, acp, req, c.buildReactRehydratePrompt(req, seeded, history), req.PromptImages)
			cancel()
			if err == nil {
				// The priming turn is context-only; drop any questions it raised
				// so the pending set stays clean for the real reply that follows.
				c.host.TakePendingQuestions(req.RunID, req.NodeID)
				key := reactKey(req)
				sess := &reactSession{sb: sb, acp: acp, home: home}
				c.mu.Lock()
				c.sessions[key] = sess
				c.live[key] = sb
				c.mu.Unlock()
				log.Info().Str("run", req.RunID).Str("node", req.NodeID).
					Msg("react session rehydrated in a fresh sandbox")
				return sess
			}
			c.discardSandbox(ctx, req, sb, acp, home, nil)
			if !isRetryableSandboxErr(err) {
				log.Warn().Err(err).Str("node", req.NodeID).Msg("react rehydrate priming failed")
				return nil
			}
		}
		if isRetryableSandboxErr(err) && attempt < n && ctx.Err() == nil {
			c.emitRetryNotice(req, attempt, n, err)
			if c.backoff(ctx, attempt) {
				continue
			}
		}
		log.Warn().Err(err).Str("node", req.NodeID).Msg("react rehydrate failed")
		return nil
	}
}

// ReactReply advances the live dialogue. The clarification finishes when the
// agent raises no further questions, the round cap is hit, or the user forces
// an early finish; then the produces contract is ensured (with re-prompting)
// and the sandbox torn down.
func (c *acpProvider) ReactReply(ctx context.Context, req NodeReq, history []models.ReactMessage, human string, images []models.PromptImage, force bool) ReactTurn {
	key := reactKey(req)
	c.mu.Lock()
	sess := c.sessions[key]
	c.mu.Unlock()
	// Live session gone (server restart) or its sandbox/ACP died during the
	// human's think-time: rebuild from the persisted transcript in a fresh
	// sandbox instead of silently finishing the node.
	if sess == nil || sess.acp == nil || !sess.acp.IsConnected() {
		if sess != nil {
			// Drop the dead session + its broken sandbox before rebuilding.
			c.mu.Lock()
			delete(c.sessions, key)
			c.mu.Unlock()
			c.discardSandbox(ctx, req, sess.sb, sess.acp, sess.home, nil)
		}
		// The trailing entry is the reply we are about to send live; prime the
		// rebuilt session with the prior context only, to avoid double-sending.
		prior := history
		if len(prior) > 0 && prior[len(prior)-1].Role == "human" {
			prior = prior[:len(prior)-1]
		}
		sess = c.rehydrateReact(ctx, req, prior)
		if sess == nil {
			// Rebuild failed: keep the node awaiting a retry (not done) so a
			// later reply can try again — never a silent empty completion.
			return ReactTurn{Msg: "(澄清会话已失效,自动重建沙箱失败,请稍后重试)", Done: false}
		}
	}

	chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
	defer cancel()
	res, err := c.streamChat(chatCtx, sess.acp, req, human, images)
	if err != nil {
		log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).
			Msg("react reply chat failed")
		return ReactTurn{
			Msg: "(澄清回复失败:" + err.Error() + ")",
			Events: []models.AcpEvent{{
				Kind: "message", Text: "react reply chat failed: " + err.Error(),
			}},
		}
	}
	var usage *models.TokenUsage
	var events []models.AcpEvent
	absorbChat(&usage, &events, res)

	qs := c.host.TakePendingQuestions(req.RunID, req.NodeID)
	// Clarification is a gate: unless the user forced a finish or the round cap
	// is reached, we cannot complete while questions remain. That includes both
	// questions the agent raised this turn (ask_question) and unresolved
	// open_questions it left in the clarified requirement — the latter are
	// re-surfaced as ask_question so the user resolves every one.
	if !force && !reactCapReached(req, history) {
		if len(qs) > 0 {
			return ReactTurn{Msg: res.Narration, Questions: qs, Events: events, Usage: usage}
		}
		if gq, msg, ge, gu, ok := c.enforceOpenQuestionsGate(ctx, req, sess); ok {
			events = append(events, ge...)
			usage = models.AddTokenUsage(usage, gu)
			if strings.TrimSpace(msg) == "" {
				msg = res.Narration
			}
			return ReactTurn{Msg: msg, Questions: gq, Events: events, Usage: usage}
		} else if gu != nil {
			events = append(events, ge...)
			usage = models.AddTokenUsage(usage, gu)
		}
	}
	return c.finishReact(ctx, req, key, sess, res.Narration, history, events, usage)
}

// ReviseInPlace sends one review turn to the parked session and keeps it alive.
// Unlike ReactReply it never finishes/closes the node: it streams the human's
// annotated instruction, then re-prompts the agent to persist its structured
// product (best-effort, same as the finish path) so the store reflects the
// edit, and returns a non-Done turn. Used by both the node-inline review "send
// (keep editing)" and the approval-gate ReAct reject (against the upstream
// producer's parked session). A dead/lost session is rebuilt from the
// transcript, mirroring ReactReply.
func (c *acpProvider) ReviseInPlace(ctx context.Context, req NodeReq, history []models.ReactMessage, human string, images []models.PromptImage) ReactTurn {
	key := reactKey(req)
	c.mu.Lock()
	sess := c.sessions[key]
	c.mu.Unlock()
	if sess == nil || sess.acp == nil || !sess.acp.IsConnected() {
		if sess != nil {
			c.mu.Lock()
			delete(c.sessions, key)
			c.mu.Unlock()
			c.discardSandbox(ctx, req, sess.sb, sess.acp, sess.home, nil)
		}
		prior := history
		if len(prior) > 0 && prior[len(prior)-1].Role == "human" {
			prior = prior[:len(prior)-1]
		}
		sess = c.rehydrateReact(ctx, req, prior)
		if sess == nil {
			err := errors.New("复审会话已失效,自动重建沙箱失败,请稍后重试")
			log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).Msg("review revise rehydrate failed")
			return ReactTurn{Msg: "(" + err.Error() + ")", Done: false, Err: err}
		}
	}
	chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
	res, err := c.streamChat(chatCtx, sess.acp, req, human, images)
	cancel()
	if err != nil {
		log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).Msg("review revise chat failed")
		return ReactTurn{Msg: "(复审修改失败:" + err.Error() + ")", Err: err,
			Events: []models.AcpEvent{{Kind: "message", Text: "review revise chat failed: " + err.Error()}}}
	}
	var usage *models.TokenUsage
	var events []models.AcpEvent
	absorbChat(&usage, &events, res)
	// Drop any questions the agent raised — a review edit is not a clarify gate.
	c.host.TakePendingQuestions(req.RunID, req.NodeID)
	// Re-persist the structured product so the store reflects the edit (the
	// engine re-derives outputs from it). No-op for nodes without a set_* tool.
	if name, tool := structuredArtifactFor(req.NodeType); name != "" {
		if serr := c.ensureStructured(ctx, req, sess.acp, name, tool, &events, &usage); serr != nil {
			log.Warn().Err(serr).Str("node", req.NodeID).Msg("review revise ensure product failed")
		}
	}
	events = c.snapshotEvents(ctx, sess.sb, events)
	return ReactTurn{Msg: res.Narration, Done: false, Events: events, Usage: usage}
}

// HasLiveSession reports whether a parked review session is held for the node.
func (c *acpProvider) HasLiveSession(runID, nodeID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	sess := c.sessions[runID+"|"+nodeID]
	return sess != nil && sess.acp != nil && sess.acp.IsConnected()
}

// RetireSession closes and retires the parked session for the node, if any.
func (c *acpProvider) RetireSession(runID, nodeID string) {
	c.closeSession(runID + "|" + nodeID)
}

// enforceOpenQuestionsGate implements the clarification gate: when the agent
// tries to finish (raised no ask_question this turn) but its clarified
// requirement still lists unresolved open_questions, re-prompt it (same session)
// to surface those as ask_question so the user resolves every one. Returns the
// freshly raised questions, the wrap-up narration, the turn's events, and ok=true
// when the gate held (i.e. new questions were raised and the node must keep
// clarifying). ok=false lets the caller finish normally: no artifact yet, no
// open questions, or the agent declined to ask again (avoids an infinite loop).
func (c *acpProvider) enforceOpenQuestionsGate(ctx context.Context, req NodeReq, sess *reactSession) ([]models.ReactQuestion, string, []models.AcpEvent, *models.TokenUsage, bool) {
	content, err := c.host.ReadArtifact(req.RunID, req.Token, mcp.ClarifiedRequirementArtifactName)
	if err != nil {
		return nil, "", nil, nil, false
	}
	if !json.Valid([]byte(content)) {
		log.Warn().Str("run", req.RunID).Str("node", req.NodeID).
			Msg("react open-questions gate: clarified requirement unparseable; skipping")
		return nil, "", nil, nil, false
	}
	open := mcp.ClarifiedOpenQuestions(content)
	if len(open) == 0 {
		return nil, "", nil, nil, false
	}
	prompt := c.agentPrompts(str2(req.Config["skill_profile"])).ClarifiedOpenQuestionsRetryFor(open)
	chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
	res, err := c.streamChat(chatCtx, sess.acp, req, prompt, nil)
	cancel()
	if err != nil {
		log.Warn().Err(err).Str("node", req.NodeID).Msg("react open-questions gate re-prompt failed")
		return nil, "", nil, nil, false
	}
	var usage *models.TokenUsage
	var events []models.AcpEvent
	absorbChat(&usage, &events, res)
	qs := c.host.TakePendingQuestions(req.RunID, req.NodeID)
	if len(qs) == 0 {
		// The agent didn't raise the questions despite the nudge. Let the node
		// finish rather than loop forever; the requirement keeps its notes.
		log.Warn().Str("run", req.RunID).Str("node", req.NodeID).
			Int("open_questions", len(open)).
			Msg("react open-questions gate: agent declined to ask; finishing with unresolved notes")
		return nil, "", events, usage, false
	}
	return qs, res.Narration, events, usage, true
}

// finishReact runs the shared completion path for a react node: ensure the
// declared produces artifact exists (re-prompting the agent to write it),
// snapshot the event log, capture any code changes, tear the session down and
// return a Done ReactTurn.
func (c *acpProvider) finishReact(ctx context.Context, req NodeReq, key string, sess *reactSession, narration string, history []models.ReactMessage, events []models.AcpEvent, usage *models.TokenUsage) ReactTurn {
	// Ensure the node's reserved structured product exists (re-prompting the
	// agent to write it via its set_* tool). react → clarified requirement;
	// plan/proposal/research/review/implement → their own product. Visual and
	// generic agent nodes have no set_* tool (name==""), so this is skipped —
	// their product (page.html / produces) is enforced by the engine's
	// finalizeProduct on the finish path.
	if name, tool := structuredArtifactFor(req.NodeType); name != "" {
		if err := c.ensureStructured(ctx, req, sess.acp, name, tool, &events, &usage); err != nil {
			events = c.snapshotEvents(ctx, sess.sb, events)
			c.closeSession(key)
			return ReactTurn{Done: true, Err: err, Msg: err.Error(), Events: events, Usage: usage,
				Result: NodeResult{Events: events, Usage: usage}}
		}
	}
	if err := c.ensureOutcome(ctx, req, sess.acp, &events, &usage); err != nil {
		events = c.snapshotEvents(ctx, sess.sb, events)
		c.closeSession(key)
		return ReactTurn{Done: true, Err: err, Msg: err.Error(), Events: events, Usage: usage,
			Result: NodeResult{Events: events, Usage: usage}}
	}
	events = c.snapshotEvents(ctx, sess.sb, events)
	out := map[string]any{"clarified_requirement": narration, "content": narration, "transcript": renderTranscript(history)}
	git := c.captureChanges(ctx, sess.sb, req, out)
	c.closeSession(key)
	return ReactTurn{Msg: narration, Done: true, Events: events, Usage: usage,
		Result: NodeResult{OutputMd: narration, Outputs: out, Events: events, Git: git, Usage: usage}}
}

// ensureOutcome re-prompts the agent to call node_complete when the mark is
// still missing (best-effort; engine fails closed if ultimately absent).
func (c *acpProvider) ensureOutcome(ctx context.Context, req NodeReq, acp *sandbox.ACPClient, events *[]models.AcpEvent, usage **models.TokenUsage) error {
	if c.host.HasOutcome(req.RunID, req.NodeID) {
		return nil
	}
	for i := 0; i <= producesRetry; i++ {
		if c.host.HasOutcome(req.RunID, req.NodeID) {
			return nil
		}
		if i == producesRetry {
			log.Warn().Str("run", req.RunID).Str("node", req.NodeID).
				Int("retries", producesRetry).
				Msg("node_complete still missing after re-prompt; engine will fail closed")
			return nil
		}
		prompt := c.agentPrompts(str2(req.Config["skill_profile"])).OutcomeRetryText()
		chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
		res, err := c.streamChat(chatCtx, acp, req, prompt, nil)
		cancel()
		if err != nil {
			if isChatTimeoutErr(err) {
				return fmt.Errorf("agent chat: %w", err)
			}
			log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).
				Msg("node_complete re-prompt failed")
			return nil
		}
		absorbChat(usage, events, res)
		c.host.TakePendingQuestions(req.RunID, req.NodeID)
	}
	return nil
}

// ensureStructured makes a framework node's reserved structured product exist
// before the node completes: it checks the run store and, while absent,
// re-prompts the agent (same session) to call the naming set_* tool, looping up
// to producesRetry times. Intermediate turns are folded into events. Unlike the
// old produces path there is no workspace harvest — structured products are
// written only through MCP.
func (c *acpProvider) ensureStructured(ctx context.Context, req NodeReq, acp *sandbox.ACPClient, name, tool string, events *[]models.AcpEvent, usage **models.TokenUsage) error {
	satisfied := func() bool {
		_, err := c.host.ReadArtifact(req.RunID, req.Token, name)
		return err == nil
	}
	for i := 0; i <= producesRetry; i++ {
		if satisfied() {
			return nil
		}
		if i == producesRetry {
			log.Warn().Str("run", req.RunID).Str("node", req.NodeID).
				Str("artifact", name).Str("tool", tool).
				Int("retries", producesRetry).
				Msg("structured product still missing after re-prompt; engine will fail closed")
			return nil
		}
		prompt := c.agentPrompts(str2(req.Config["skill_profile"])).StructuredRetryFor(name, tool)
		chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
		res, err := c.streamChat(chatCtx, acp, req, prompt, nil)
		cancel()
		if err != nil {
			if isChatTimeoutErr(err) {
				return fmt.Errorf("agent chat: %w", err)
			}
			log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).
				Str("artifact", name).Msg("structured product re-prompt failed")
			return nil
		}
		absorbChat(usage, events, res)
		// Drop any questions raised during the wrap-up turn — the node is
		// finishing, not reopening a clarification.
		c.host.TakePendingQuestions(req.RunID, req.NodeID)
	}
	return nil
}

// structuredArtifactFor maps a framework node type to its reserved product
// name and the set_* tool that writes it (see nodereg).
func structuredArtifactFor(nodeType string) (name, tool string) {
	return nodereg.StructuredProduct(nodeType)
}

// ensurePlanComplete drives an implement node's run plan to completion. It
// reads the plan's outstanding items (host.PlanIncomplete); while any remain it
// re-prompts the agent (same session) to finish them, up to max_rounds times.
// A missing/unparseable plan is treated as "nothing to enforce" (nil). If items
// still remain after the loop it returns an error so the engine fails the node.
func (c *acpProvider) ensurePlanComplete(ctx context.Context, req NodeReq, acp *sandbox.ACPClient, events *[]models.AcpEvent, usage **models.TokenUsage) error {
	maxRounds := 3
	if mr, ok := toInt(req.Config["max_rounds"]); ok && mr > 0 {
		maxRounds = mr
	}
	for i := 0; i < maxRounds; i++ {
		inc, err := c.host.PlanIncomplete(req.RunID, req.Token)
		if err != nil {
			// Missing plan is the common fail-open; log only unexpected read/parse faults.
			if err.Error() != "mcp: no plan" {
				log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).
					Msg("plan incomplete check failed; skipping enforce")
			}
			return nil
		}
		if len(inc) == 0 {
			return nil
		}
		prompt := c.agentPrompts(str2(req.Config["skill_profile"])).PlanIncompleteRetryFor(inc)
		chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
		res, err := c.streamChat(chatCtx, acp, req, prompt, nil)
		cancel()
		if err != nil {
			if isChatTimeoutErr(err) {
				return fmt.Errorf("agent chat: %w", err)
			}
			log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).
				Msg("implement plan-complete re-prompt failed")
			break
		}
		absorbChat(usage, events, res)
	}
	inc, err := c.host.PlanIncomplete(req.RunID, req.Token)
	if err != nil {
		if err.Error() != "mcp: no plan" {
			log.Warn().Err(err).Str("run", req.RunID).Str("node", req.NodeID).
				Msg("plan incomplete final check failed; skipping enforce")
		}
		return nil
	}
	if len(inc) > 0 {
		return fmt.Errorf("计划未全部完成,仍有 %d 项未完成: %s", len(inc), strings.Join(inc, "; "))
	}
	return nil
}

// ensurePreviewRegistered drives an app_preview node to register at least one
// preview port via set_preview, re-prompting up to max_rounds when absent.
func (c *acpProvider) ensurePreviewRegistered(ctx context.Context, req NodeReq, acp *sandbox.ACPClient, events *[]models.AcpEvent, usage **models.TokenUsage) error {
	maxRounds := 3
	if mr, ok := toInt(req.Config["max_rounds"]); ok && mr > 0 {
		maxRounds = mr
	}
	for i := 0; i < maxRounds; i++ {
		if c.host.HasPreviewPorts(req.RunID, req.NodeID) {
			return nil
		}
		prompt := c.agentPrompts(str2(req.Config["skill_profile"])).PreviewRetryText()
		chatCtx, cancel := context.WithTimeout(ctx, c.nodeChatTimeout(req))
		res, err := c.streamChat(chatCtx, acp, req, prompt, nil)
		cancel()
		if err != nil {
			if isChatTimeoutErr(err) {
				return fmt.Errorf("agent chat: %w", err)
			}
			log.Warn().Err(err).Str("node", req.NodeID).Msg("app_preview set_preview re-prompt failed")
			break
		}
		absorbChat(usage, events, res)
		c.host.TakePendingQuestions(req.RunID, req.NodeID)
	}
	if !c.host.HasPreviewPorts(req.RunID, req.NodeID) {
		return fmt.Errorf("预览契约未满足:未调用 set_preview")
	}
	return nil
}

// --- internals ------------------------------------------------------------

func (c *acpProvider) openSandbox(ctx context.Context, req NodeReq) (*sandbox.Sandbox, *sandbox.ACPClient, string, error) {
	spec, err := c.spec(req)
	if err != nil {
		return nil, nil, "", fmt.Errorf("%w: %v", errSandboxSetup, err)
	}
	home := spec.ConfigHome
	// Pre-register a "creating" row (placeholder id) so the sandbox is visible in
	// the list and the node live log ("正在启动沙箱…") during the slow gateway
	// provisioning, instead of a 404. It is adopted (real id + "running") on
	// success or removed on any failure below.
	placeholder := sandbox.NewContainerName()
	spec.Name = placeholder
	c.beginRunSandbox(req, placeholder, home)
	sb, err := c.mgr.Create(ctx, spec)
	if err != nil {
		c.deregisterRunSandbox(placeholder)
		removeHome(home)
		return nil, nil, "", fmt.Errorf("%w: create sandbox: %v", errSandboxSetup, err)
	}
	if err := sandbox.WaitForACPReady(ctx, sb.Host, sb.Port, sb.Password, 120*time.Second); err != nil {
		c.deregisterRunSandbox(placeholder)
		sb.Destroy(context.Background())
		removeHome(home)
		return nil, nil, "", fmt.Errorf("%w: acp not ready: %v", errSandboxSetup, err)
	}
	acp := sb.ACP().WithSession(sb.WorkspaceDir, c.mcpServers(req)).WithIdleTimeout(c.opts.ChatIdleTimeout)
	if err := acp.Connect(ctx); err != nil {
		c.deregisterRunSandbox(placeholder)
		acp.Close()
		sb.Destroy(context.Background())
		removeHome(home)
		return nil, nil, "", fmt.Errorf("%w: acp connect: %v", errSandboxSetup, err)
	}
	c.registerRunSandbox(req, sb, home)
	return sb, acp, home, nil
}

func removeHome(home string) {
	if home != "" {
		_ = os.RemoveAll(home)
	}
}

func (c *acpProvider) spec(req NodeReq) (sandbox.Spec, error) {
	// Flat repo model: every repository in vars.repos (even a single one) is
	// cloned to <workspace>/<name>/ and the workspace root is never a git repo.
	// Empty repos == a pure artifact flow (empty workspace).
	repos := resolveRepos(req)
	env := map[string]string{}
	// Non-auth platform sandbox env (e.g. optional overrides) — API keys belong
	// in Agent env only (see mergeAuthEnv).
	for k, v := range c.opts.Env {
		if IsPlatformAuthEnvKey(k) {
			continue
		}
		env[k] = v
	}
	profile := str2(req.Config["skill_profile"])
	agentCfg := c.agentConfig(profile)
	vars := c.mcpVars(req)
	// Project sandbox env: platform < project < Agent < runtime. Skip ACP auth
	// keys; substitute ${vars.*}/${APPROVING_*} after run vars are available.
	if c.opts.ProjectEnvForWorkflow != nil && req.WorkflowID != "" {
		for k, v := range c.opts.ProjectEnvForWorkflow(req.WorkflowID) {
			if IsPlatformAuthEnvKey(k) {
				continue
			}
			env[k] = substVars(v, vars)
		}
	}
	for k, v := range agentCfg.Env {
		env[k] = substVars(v, vars)
	}
	merged, err := mergeAuthEnv(c.backend, env)
	if err != nil {
		return sandbox.Spec{}, err
	}
	env = merged
	env["ACP_BACKEND"] = string(c.backend)
	if req.NodeType == "app_preview" {
		// Start in-sandbox Xvfb/Chromium/x11vnc/websockify for noVNC isolation.
		// Image contract: VNC_PREVIEW (ENABLE_VNC_PREVIEW alias). Keep the old
		// APPROVING_* name briefly for any leftover docs/scripts.
		env["VNC_PREVIEW"] = "1"
		env["APPROVING_VNC_PREVIEW"] = "1"
	}
	// Export the run-scoped artifact-store coordinates as process env so
	// in-sandbox tools (e.g. the artifact-upload CLI) can reach the store
	// without the Agent inlining base64. These mirror the mcp.json template
	// vars; the token is already handed to the Agent via mcp.json, so this
	// exposes no new secret.
	for k, v := range vars {
		if strings.HasPrefix(k, "vars.") || v == "" {
			continue
		}
		env[k] = v
	}
	// The clone list is injected only via the Agent meta env reference
	// ("GIT_REPOS": "${vars.repos}", expanded by mcpVars) — the platform does
	// not inject GIT_REPOS itself. Derive GITLAB_URL so the in-sandbox git
	// credential host matches those repos: from the first repo's URL + a
	// GITLAB_TOKEN set in the Agent meta. An explicit GITLAB_URL still wins.
	if len(repos) > 0 && env["GITLAB_URL"] == "" {
		if base := gitBaseURL(repos[0].URL); base != "" {
			env["GITLAB_URL"] = base
		}
	}
	layout := c.agentLayout(profile, agentCfg)
	env["CONFIG_ROOT"] = layout.ConfigRoot
	// remote-dev parity: PASSWORD / ROOT_PASSWORD / CURSOR_ACP_PASSWORD.
	// Persisted as sandbox Token for SandboxProxy / SandboxACPProxy auto-login
	// and for direct host:port access (shown in SandboxView.password).
	sandbox.ApplyPasswords(env, req.Token)
	spec := sandbox.Spec{
		Image:        resolveProviderImage(c.opts, c.backend),
		Env:          env,
		ConfigHome:   c.buildConfigHome(req, env),
		ConfigRoot:   layout.ConfigRoot,
		WorkspaceDir: layout.WorkspaceDir,
	}
	if req.NodeType == "app_preview" {
		// DinD + Chromium need more headroom than the gateway's 4Gi default.
		spec.Resources = &sandbox.GWResources{CPUCores: 2, MemoryMB: 8192, DiskGi: 40}
	}
	return spec, nil
}

func (c *acpProvider) agentLayout(profile string, cfg agentFile) agentLayout {
	backend := c.backend
	if b := NormalizeBackend(cfg.AcpBackend); cfg.AcpBackend != "" {
		backend = b
	}
	root := cfg.Layout.ConfigRoot
	ws := cfg.Layout.WorkspaceDir
	if strings.TrimSpace(root) == "" {
		root = ResolveConfigRoot(backend, "")
	}
	if strings.TrimSpace(ws) == "" {
		ws = "/root/workspace"
	}
	return agentLayout{ConfigRoot: root, WorkspaceDir: ws}
}

// buildConfigHome materializes the per-node config home (rules/skills + mcp.json).
func (c *acpProvider) buildConfigHome(req NodeReq, env map[string]string) string {
	profile := str2(req.Config["skill_profile"])
	specs := c.resolvedMCPSpecs(req)
	home, err := sandbox.BuildConfigHome(sandbox.ConfigHomeSpec{
		WorkDirSrc:           c.workDir(profile),
		EmbeddedRules:        nodereg.EmbeddedRuleFiles(req.NodeType),
		IncludeArtifactStore: hasArtifactStore(specs),
		MCP:                  specs,
		Settings:             CodeBuddySettingsForEnv(c.backend, env),
		AgentName:            profile,
		ProfilesRoot:         c.opts.ProfilesRoot,
		GlobalRulesDir:       c.opts.PlatformRulesRoot,
	})
	if err != nil {
		log.Warn().Err(err).Str("node", req.NodeID).Msg("build cursor home failed; running without /root/.cursor mount")
		return ""
	}
	return home
}

const (
	agentWorkDirName       = "workspace"
	legacyAgentWorkDirName = "cursor" // compatibility window; remove in 0.2.0
)

// workDir returns the agent's on-disk working directory (workspace/ or legacy
// cursor/) if it exists, for verbatim copy into the sandbox config root.
func (c *acpProvider) workDir(profile string) string {
	if profile == "" || c.opts.ProfilesRoot == "" {
		return ""
	}
	base := filepath.Join(c.opts.ProfilesRoot, filepath.Base(profile))
	for _, sub := range []string{agentWorkDirName, legacyAgentWorkDirName} {
		d := filepath.Join(base, sub)
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	return ""
}

// agentMCP mirrors one MCP server entry of agent.json.
type agentMCP struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// agentLayout mirrors the sandbox-injection layout in agent.json (config root +
// workspace dir). Empty fields fall back to protocol defaults at use site.
type agentLayout struct {
	ConfigRoot   string `json:"configRoot"`
	WorkspaceDir string `json:"workspaceDir"`
}

// agentFile mirrors <ProfilesRoot>/<profile>/agent.json (mcp + env + layout +
// per-Agent prompt overrides).
type agentFile struct {
	AcpBackend string               `json:"acpBackend"`
	MCP        []agentMCP           `json:"mcp"`
	Env        map[string]string    `json:"env"`
	Layout     agentLayout          `json:"layout"`
	Prompts    *models.AgentPrompts `json:"prompts"`
}

// agentConfig reads the Agent's agent.json (best effort; empty on miss).
func (c *acpProvider) agentConfig(profile string) agentFile {
	var f agentFile
	if profile == "" || c.opts.ProfilesRoot == "" {
		return f
	}
	b, err := os.ReadFile(filepath.Join(c.opts.ProfilesRoot, filepath.Base(profile), "agent.json"))
	if err != nil {
		return f
	}
	_ = json.Unmarshal(b, &f)
	return f
}

// reservedArtifactStore is the conventional name for the platform's run-scoped
// artifact-store MCP. It is NOT special-cased in mcp.json: the whole MCP config
// is user-authored. The run-scoped endpoint + token are exposed only as
// template vars (see mcpVars) that the user references inside their config,
// e.g. url "${APPROVING_ARTIFACT_URL}" + header "Bearer ${APPROVING_ARTIFACT_TOKEN}".
// The name is used only to gate the convention doc rule and as the UI default.
const reservedArtifactStore = "artifact-store"

// mcpVars are the run-scoped template variables substituted into the Agent's
// MCP config and env values at runtime. They are the only way the dynamic
// artifact-store URL/token reach the user-authored mcp.json — so the token is
// never persisted in config and stays bound to this run (per-run isolation).
// gitToken resolves GITLAB_TOKEN from the platform env and the Agent-meta env
// (with ${...} substitution), mirroring how spec() builds the sandbox env. It
// gates optional MR creation; empty means "no credentials, skip MR".
func (c *acpProvider) gitToken(req NodeReq) string {
	vars := c.mcpVars(req)
	if v := substVars(c.agentConfig(str2(req.Config["skill_profile"])).Env["GITLAB_TOKEN"], vars); v != "" {
		return v
	}
	return c.opts.Env["GITLAB_TOKEN"]
}

// gitLabURL resolves GITLAB_URL for GitLab detection and MR gating. Explicit
// agent GITLAB_URL wins; otherwise derive from the node's repo URL only when
// GITLAB_TOKEN is configured and the repo is not GitHub (avoids a misconfigured
// token on GitHub).
func (c *acpProvider) gitLabURL(req NodeReq) string {
	vars := c.mcpVars(req)
	if v := substVars(c.agentConfig(str2(req.Config["skill_profile"])).Env["GITLAB_URL"], vars); v != "" {
		return v
	}
	repo := c.nodeRepoURL(req)
	host := gitRepoHost(repo)
	if host == "github.com" {
		return ""
	}
	if c.gitToken(req) != "" {
		return gitBaseURL(repo)
	}
	return ""
}

func (c *acpProvider) mcpVars(req NodeReq) map[string]string {
	m := map[string]string{
		"APPROVING_ARTIFACT_URL":   c.mcpURL(req),
		"APPROVING_ARTIFACT_TOKEN": req.Token,
		"APPROVING_RUN_ID":         req.RunID,
		"APPROVING_NODE_ID":        req.NodeID,
	}
	// Expose workflow global variables so Agent meta (env / MCP url·headers·args)
	// can reference any of them, e.g. "SOME_ENV": "${vars.feature}".
	for k, v := range req.Vars {
		if k == "" {
			continue
		}
		m["vars."+k] = str2(v)
	}
	// The reserved `repos` variable is exposed in the GIT_REPOS wire format
	// (name|url|branch, comma-separated) rather than raw JSON, so the Agent meta
	// env can wire the clone list the same reference way as credentials:
	//   "GIT_REPOS": "${vars.repos}".
	// Set unconditionally (empty when no repos) so the reference always resolves.
	m["vars.repos"] = sandbox.EncodeRepos(resolveRepos(req))
	return m
}

func substVars(s string, vars map[string]string) string {
	if s == "" {
		return s
	}
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

func substMap(m, vars map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = substVars(v, vars)
	}
	return out
}

func substSlice(s []string, vars map[string]string) []string {
	if len(s) == 0 {
		return nil
	}
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = substVars(v, vars)
	}
	return out
}

// resolvedMCPSpecs maps the Agent's user-authored MCP servers to sandbox specs,
// substituting the run-scoped template vars into every string field. A server
// whose URL/command resolves to empty (e.g. artifact-store while the MCP
// endpoint is disabled) is dropped.
//
// Workflow ACP never auto-injects live memory/context/scheduler endpoints;
// declared entries for those names (or URL paths) are dropped here.
func (c *acpProvider) resolvedMCPSpecs(req NodeReq) []sandbox.MCPServerSpec {
	mcp := c.agentConfig(str2(req.Config["skill_profile"])).MCP
	if len(mcp) == 0 {
		return nil
	}
	vars := c.mcpVars(req)
	out := make([]sandbox.MCPServerSpec, 0, len(mcp))
	for _, m := range mcp {
		if m.Name == "" {
			continue
		}
		// Keep in sync with services.IsProjectPlatformMCP (runtime cannot import
		// services — that package already depends on runtime).
		if isWorkflowDroppedProjectPlatformMCP(m.Name, m.URL) {
			continue
		}
		url := config.RewriteMisconfiguredMCPAdvertise(substVars(m.URL, vars))
		cmd := substVars(m.Command, vars)
		if url == "" && cmd == "" {
			continue
		}
		out = append(out, sandbox.MCPServerSpec{
			Name:    m.Name,
			URL:     url,
			Headers: substMap(m.Headers, vars),
			Command: cmd,
			Args:    substSlice(m.Args, vars),
			Env:     substMap(m.Env, vars),
		})
	}
	return out
}

func isWorkflowDroppedProjectPlatformMCP(name, url string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "memory-store", "context-store", "task-scheduler":
		return true
	}
	return strings.Contains(url, "/mcp/memory-store/") ||
		strings.Contains(url, "/mcp/context-store/") ||
		strings.Contains(url, "/mcp/task-scheduler/")
}

// hasArtifactStore reports whether a server named artifact-store survived
// resolution (used only to gate its convention doc rule).
func hasArtifactStore(specs []sandbox.MCPServerSpec) bool {
	for _, s := range specs {
		if s.Name == reservedArtifactStore {
			return true
		}
	}
	return false
}

// mcpURL is the run-scoped artifact-store MCP endpoint reachable from inside
// the sandbox (matches router path /mcp/runs/:runId). Empty when unconfigured.
// Prefers live config (hot-reload) over the boot-time Options snapshot.
func (c *acpProvider) mcpURL(req NodeReq) string {
	base := config.ResolveMCPAdvertise(c.opts.MCPEndpoint)
	if base == "" {
		return ""
	}
	return base + "/mcp/runs/" + req.RunID
}

// mcpServers builds the ACP mcpServers array injected at session/new from the
// Agent's declared MCP servers (with artifact-store resolved to its run-scoped
// URL+token). Returns nil when the Agent declares none.
func (c *acpProvider) mcpServers(req NodeReq) json.RawMessage {
	specs := c.resolvedMCPSpecs(req)
	if len(specs) == 0 {
		return nil
	}
	servers := make([]map[string]any, 0, len(specs))
	for _, m := range specs {
		switch {
		case m.URL != "":
			entry := map[string]any{"name": m.Name, "url": m.URL}
			if len(m.Headers) > 0 {
				hs := make([]map[string]string, 0, len(m.Headers))
				for k, v := range m.Headers {
					hs = append(hs, map[string]string{"name": k, "value": v})
				}
				entry["headers"] = hs
			}
			servers = append(servers, entry)
		case m.Command != "":
			entry := map[string]any{"name": m.Name, "command": m.Command}
			if len(m.Args) > 0 {
				entry["args"] = m.Args
			}
			if len(m.Env) > 0 {
				es := make([]map[string]string, 0, len(m.Env))
				for k, v := range m.Env {
					es = append(es, map[string]string{"name": k, "value": v})
				}
				entry["env"] = es
			}
			servers = append(servers, entry)
		}
	}
	if len(servers) == 0 {
		return nil
	}
	b, _ := json.Marshal(servers)
	return b
}

// harvest copies a declared produces file out of the container and writes it
// through the run-scoped MCP host into the platform artifact store.
func (c *acpProvider) harvest(ctx context.Context, sb *sandbox.Sandbox, req NodeReq, produces string, out map[string]any, events *[]models.AcpEvent) error {
	data, err := sb.ReadFile(ctx, produces)
	if err != nil {
		return fmt.Errorf("produces %q not found in sandbox: %w", produces, err)
	}
	kind := artifactKind(produces)
	id, err := c.host.WriteArtifact(req.RunID, req.Token, req.NodeID, produces, string(data), kind)
	if err != nil {
		return fmt.Errorf("write_artifact %s: %w", produces, err)
	}
	out["artifact_id"] = id
	*events = append(*events, models.AcpEvent{
		Kind: "tool_call", Title: "write_artifact(" + produces + ")", Status: "completed",
		Artifact: &models.ArtifactMeta{Name: produces, Kind: kind},
	})
	return nil
}

// ensurePushed guarantees the implement node's working branch reaches origin so
// downstream sandboxes (fresh clones) can check it out. It commits any leftover
// changes the agent didn't commit, then pushes the current branch. Fully
// best-effort: no repo / no remote / no credentials → silent no-op (never fails
// the node).
func (c *acpProvider) ensurePushed(ctx context.Context, sb *sandbox.Sandbox, req NodeReq) {
	repos := resolveRepos(req)
	if len(repos) == 0 {
		return
	}
	for _, r := range repos {
		script := `cd ` + shellArg(repoWorkspacePath(r.Name)) + ` || exit 0
git rev-parse --git-dir >/dev/null 2>&1 || exit 0
git remote get-url origin >/dev/null 2>&1 || exit 0
branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
if [ -z "$branch" ] || [ "$branch" = "HEAD" ]; then exit 0; fi
# Never auto-push protected branches: if the agent forgot to create a feature
# branch and worked on main/master, don't pollute the trunk on their behalf.
case "$branch" in
  main|master|develop|release-*) echo "on protected branch $branch; skip auto push"; exit 0;;
esac
git add -A 2>/dev/null || true
git diff --cached --quiet 2>/dev/null || git commit -m "chore(approving): implement 收尾自动提交" >/dev/null 2>&1 || true
git push -u origin "$branch" 2>&1 || true`
		if out, err := sb.Exec(ctx, 90*time.Second, "bash", "-lc", script); err != nil {
			log.Debug().Err(err).Str("repo", r.Name).Str("out", strings.TrimSpace(out)).Msg("implement ensurePushed (best-effort)")
		}
	}
}

// detectPush inspects the container repo for branch/HEAD and whether the
// branch exists on the remote (i.e. the agent pushed). MR creation via glab
// is gated: GitLab repo + GITLAB_TOKEN + create_mr config must all be set.
func (c *acpProvider) detectPush(ctx context.Context, sb *sandbox.Sandbox, req NodeReq) *GitInfo {
	dir, repo := c.nodeRepo(req)
	cd := "cd " + shellArg(dir) + " && "
	branch, _ := sb.Exec(ctx, 10*time.Second, "bash", "-lc", cd+"git rev-parse --abbrev-ref HEAD 2>/dev/null || true")
	sha, _ := sb.Exec(ctx, 10*time.Second, "bash", "-lc", cd+"git rev-parse HEAD 2>/dev/null || true")
	branch, sha = strings.TrimSpace(branch), strings.TrimSpace(sha)
	if sha == "" {
		return nil
	}
	info := &GitInfo{Branch: branch, PushedSHA: sha}
	// Branch present on remote => the agent pushed it.
	remote, _ := sb.Exec(ctx, 15*time.Second, "bash", "-lc",
		cd+"git ls-remote --heads origin "+shellArg(branch)+" 2>/dev/null || true")
	info.Pushed = strings.TrimSpace(remote) != ""

	createMR, _ := req.Config["create_mr"].(bool)
	if info.Pushed && createMR && c.gitToken(req) != "" && isGitLabRepo(repo, c.gitLabURL(req)) {
		if url := c.findOrCreateMR(ctx, sb, dir, branch); url != "" {
			info.MrURL = url
		}
	}
	return info
}

func (c *acpProvider) findOrCreateMR(ctx context.Context, sb *sandbox.Sandbox, dir, branch string) string {
	cd := "cd " + shellArg(dir) + " && "
	view, err := sb.Exec(ctx, 25*time.Second, "bash", "-lc",
		cd+"glab mr list --source-branch "+shellArg(branch)+" -F json 2>/dev/null || true")
	if err == nil {
		if url := firstMRURL(view); url != "" {
			return url
		}
	}
	create, _ := sb.Exec(ctx, 40*time.Second, "bash", "-lc",
		cd+"glab mr create --fill --yes --source-branch "+shellArg(branch)+" 2>&1 || true")
	for _, line := range strings.Split(create, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			return line
		}
	}
	return ""
}

func firstMRURL(jsonOut string) string {
	url, _ := firstMR(jsonOut)
	return url
}

// firstMR parses `glab mr list -F json` output and returns the first MR's web
// URL and iid (0 when absent/unparseable).
func firstMR(jsonOut string) (url string, iid int) {
	var arr []map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &arr); err != nil || len(arr) == 0 {
		return "", 0
	}
	if u, ok := arr[0]["web_url"].(string); ok {
		url = u
	}
	if v, ok := arr[0]["iid"].(float64); ok {
		iid = int(v)
	}
	return url, iid
}

func shellArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// mrBranches resolves a submit_mr node's source/target branches. Source falls
// back to vars.branches[repo] so the MR node picks up the implement node's
// working branch by default; target may be empty (== the repository's default
// branch, resolved by glab).
func mrBranches(req NodeReq) (source, target string) {
	source = strings.TrimSpace(str2(req.Config["source_branch"]))
	// Derive the source branch from the selected repo's entry in the run's
	// branches map (vars.branches[repo]). The target repo is config["repo"], or
	// the lone configured repo when the node didn't pin one.
	if source == "" {
		repo := strings.TrimSpace(str2(req.Config["repo"]))
		if repo == "" {
			if repos := resolveRepos(req); len(repos) == 1 {
				repo = repos[0].Name
			}
		}
		if repo != "" {
			if bm := parseBranchesVar(req.Vars["branches"]); bm != nil {
				source = strings.TrimSpace(bm[repo])
			}
		}
	}
	target = strings.TrimSpace(str2(req.Config["target_branch"]))
	return source, target
}

// mrTargetDisplay renders the target branch for prompts; an empty target means
// the repository default branch.
func mrTargetDisplay(target string) string {
	if target == "" {
		return "仓库默认分支"
	}
	return target
}

// AbortRun tears down live react sessions and in-flight agent ACP connections
// for a run. Called on cancel/fail so Cancel-during-agent unblocks RunAgent
// (and react sandboxes are not left busy forever).
func (c *acpProvider) AbortRun(runID string) {
	prefix := runID + "|"
	c.mu.Lock()
	var sessionKeys []string
	for k := range c.sessions {
		if strings.HasPrefix(k, prefix) {
			sessionKeys = append(sessionKeys, k)
		}
	}
	var agentKeys []string
	var agentACPs []*sandbox.ACPClient
	var agentSBs []*sandbox.Sandbox
	for k, acp := range c.inflightACP {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		// React sessions own their ACP via closeSession; skip duplicates.
		if _, parked := c.sessions[k]; parked {
			continue
		}
		agentKeys = append(agentKeys, k)
		agentACPs = append(agentACPs, acp)
		agentSBs = append(agentSBs, c.live[k])
	}
	for _, k := range agentKeys {
		delete(c.inflightACP, k)
		delete(c.live, k)
	}
	c.mu.Unlock()
	for _, k := range sessionKeys {
		c.closeSession(k)
	}
	for i, acp := range agentACPs {
		if acp != nil {
			acp.Close()
		}
		if sb := agentSBs[i]; sb != nil {
			// Hard-destroy so a stuck bridge cannot hold the agent goroutine.
			c.deregisterRunSandbox(sb.Name)
			sb.Destroy(context.Background())
		}
	}
}

func (c *acpProvider) closeSession(key string) {
	c.mu.Lock()
	sess := c.sessions[key]
	delete(c.sessions, key)
	delete(c.live, key)
	c.mu.Unlock()
	if sess != nil {
		// Retain the react sandbox after the dialogue ends (or the run is
		// aborted) for a debug TTL instead of destroying it immediately.
		c.retireRunSandbox(sess.sb, sess.acp, sess.home)
	}
}

func (c *acpProvider) buildAgentPrompt(req NodeReq, seeded []string) string {
	var b strings.Builder
	profile := str2(req.Config["skill_profile"])
	if sys := str2(req.Config["system"]); sys != "" {
		b.WriteString(sys + "\n\n")
	}
	b.WriteString(str2(req.Config["prompt"]))
	prompts := c.agentPrompts(profile)
	if len(seeded) > 0 {
		b.WriteString(prompts.UpstreamHeader())
		for _, n := range seeded {
			fmt.Fprintf(&b, "- `%s`\n", n)
		}
	}
	// Per-type contract clause. Generic agent nodes use the produces contract;
	// framework nodes each get the clause naming their structured deliverable.
	source, target := mrBranches(req)
	if clause := nodereg.PromptContractText(prompts, req.NodeType, source, mrTargetDisplay(target)); clause != "" {
		b.WriteString(clause)
	} else if produces := str2(req.Config["produces"]); produces != "" {
		b.WriteString(prompts.ProducesContractFor(produces))
	}
	// Cross-cutting completion mark: every agent-class node must call node_complete.
	if nodeNeedsOutcome(req.NodeType) {
		b.WriteString(prompts.OutcomeContractText())
	}
	// Conditional prompt injection: append the configured text only when the
	// referenced global variable exists and is non-empty. Lets a downstream
	// ReAct interaction steer the agent without editing the base prompt.
	if inject := conditionalInjection(req); inject != "" {
		b.WriteString("\n\n" + inject)
	}
	if extra := testNodePromptExtras(req); extra != "" {
		b.WriteString(extra)
	}
	// Multi-repo (flat) layout context for workspace-touching nodes, so the
	// agent operates in the right per-repo directory. No-op in single-repo mode.
	if nodeTouchesRepos(req.NodeType) {
		if layout := multiRepoLayoutText(req); layout != "" {
			b.WriteString(layout)
		}
	}
	if note := submitMRRepoNote(req); note != "" {
		b.WriteString(note)
	}
	return strings.TrimSpace(b.String())
}

// nodeTouchesRepos reports whether a node type operates on the cloned repos
// (and thus benefits from the flat multi-repo layout description).
func nodeTouchesRepos(nodeType string) bool {
	switch nodeType {
	case "agent", "implement", "review", "test", "submit_mr", "research", "app_preview":
		return true
	default:
		return false
	}
}

// conditionalInjection returns the node's conditional_prompt text when its
// when_var global variable is present and non-empty; otherwise "". The text is
// already interpolated by the engine (nodeReq) before reaching here.
func conditionalInjection(req NodeReq) string {
	cp, ok := req.Config["conditional_prompt"].(map[string]any)
	if !ok {
		return ""
	}
	whenVar := strings.TrimSpace(str2(cp["when_var"]))
	text := strings.TrimSpace(str2(cp["text"]))
	if whenVar == "" || text == "" {
		return ""
	}
	if v, ok := req.Vars[whenVar]; ok && !models.IsBlankVar(v) && models.VarDisplayText(v) != "false" {
		return text
	}
	return ""
}

func (c *acpProvider) buildReactOpenPrompt(req NodeReq, seeded []string) string {
	p := c.buildAgentPrompt(req, seeded)
	return p + c.agentPrompts(str2(req.Config["skill_profile"])).ReactOpenSuffixText()
}

// buildReactRehydratePrompt re-opens a dialogue after a crash/restart: the base
// open prompt plus the persisted Q&A transcript as recovery context, instructing
// the agent to resume without re-asking and to await the next human reply.
func (c *acpProvider) buildReactRehydratePrompt(req NodeReq, seeded []string, history []models.ReactMessage) string {
	var b strings.Builder
	b.WriteString(c.buildReactOpenPrompt(req, seeded))
	b.WriteString("\n\n## 会话恢复(重要)\n之前的澄清对话因故中断,现在在新会话中恢复。以下是此前的完整问答记录,请据此恢复上下文并继续:不要重复已经问过的问题,先不要提出新问题,等待我接下来的回复。\n\n")
	b.WriteString(renderTranscript(history))
	return b.String()
}

// agentPrompts returns the Agent's per-profile prompt overrides (from its
// agent.json), or nil when unset — the *models.AgentPrompts helpers are all
// nil-safe and fall back to the built-in defaults.
func (c *acpProvider) agentPrompts(profile string) *models.AgentPrompts {
	return c.agentConfig(profile).Prompts
}

// upstreamArtifacts lists this run's existing artifact names so the agent can
// pull them on demand through the read_artifact MCP tool. It deliberately does
// NOT write anything into the workspace: seeding files under .approving/artifacts/
// polluted the node's code-change report (they showed up as untracked changes)
// and is unnecessary, since the artifact-store MCP is always mounted in-sandbox.
func (c *acpProvider) upstreamArtifacts(req NodeReq) []string {
	infos, err := c.host.ListArtifacts(req.RunID, req.Token)
	if err != nil || len(infos) == 0 {
		return nil
	}
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name)
	}
	return names
}

// --- helpers --------------------------------------------------------------

func reactKey(req NodeReq) string { return req.RunID + "|" + req.NodeID }

// nodeNeedsOutcome reports whether this node type must call node_complete.
func nodeNeedsOutcome(nodeType string) bool {
	switch nodeType {
	case "agent", "plan", "implement", "react", "research", "proposal",
		"test", "review", "submit_mr", "visual", "app_preview":
		return true
	default:
		return false
	}
}

// gitBaseURL returns the scheme://host origin of an http(s) repo URL, used to
// derive GITLAB_URL (the git credential host) from repo_url. Returns "" for
// non-http URLs (e.g. ssh git@host:…) or unparseable input.
func gitBaseURL(repo string) string {
	u, err := url.Parse(strings.TrimSpace(repo))
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// gitRepoScheme classifies a repo URL as https, ssh, or "".
func gitRepoScheme(repo string) string {
	repo = strings.TrimSpace(repo)
	switch {
	case strings.HasPrefix(repo, "http://"), strings.HasPrefix(repo, "https://"):
		return "https"
	case strings.HasPrefix(repo, "ssh://"):
		return "ssh"
	case strings.Contains(repo, "@") && strings.Contains(repo, ":"):
		return "ssh"
	default:
		return ""
	}
}

// gitRepoHost extracts the hostname from https, ssh://, or SCP-style repo URLs.
func gitRepoHost(repo string) string {
	repo = strings.TrimSpace(repo)
	if u, err := url.Parse(repo); err == nil && u.Host != "" {
		host := u.Host
		if i := strings.Index(host, ":"); i >= 0 {
			host = host[:i]
		}
		return host
	}
	if at := strings.Index(repo, "@"); at >= 0 {
		rest := repo[at+1:]
		if colon := strings.Index(rest, ":"); colon >= 0 {
			return rest[:colon]
		}
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return rest[:slash]
		}
	}
	return ""
}

// isGitLabRepo reports whether repo_url points at GitLab (gitlab.com or GITLAB_URL host).
func isGitLabRepo(repo, gitlabURL string) bool {
	host := gitRepoHost(repo)
	if host == "" {
		return false
	}
	if host == "gitlab.com" {
		return true
	}
	glHost := gitRepoHost(gitlabURL)
	if glHost == "" {
		glHost = gitRepoHost("https://" + strings.TrimPrefix(strings.TrimPrefix(gitlabURL, "https://"), "http://"))
	}
	return glHost != "" && host == glHost
}

// producesRetry caps how many times finishReact re-prompts the agent to write
// a missing declared produces artifact before falling back to the engine's
// contract-miss handling (which routes failure/rollback per the FSM).
const producesRetry = 3

// ReactCapReached exposes the same max_rounds safety cap the provider enforces
// so the engine's auto-clarify loop (auto_var) stops after the same number of
// rounds instead of replying forever.
func ReactCapReached(req NodeReq, history []models.ReactMessage) bool {
	return reactCapReached(req, history)
}

// reactCapReached reports whether the max_rounds safety cap is hit (counting
// the reply currently being processed). When true the dialogue must finish even
// if the agent still wants to ask more. Completion is otherwise agent-driven
// (no questions raised this turn); the old keyword heuristic is gone.
func reactCapReached(req NodeReq, history []models.ReactMessage) bool {
	humanTurns := 1 // the reply being processed
	for _, h := range history {
		if h.Role == "human" {
			humanTurns++
		}
	}
	maxRounds := 3
	if mr, ok := toInt(req.Config["max_rounds"]); ok && mr > 0 {
		maxRounds = mr
	}
	return humanTurns >= maxRounds
}

// chatResultToEvents flattens a ChatResult into ordered AcpEvents for the run
// timeline. Thin wrapper over the shared sandbox converter (single source of
// truth for the event shape).
func chatResultToEvents(res *sandbox.ChatResult) []models.AcpEvent {
	return res.AcpEvents()
}

type repoEntry struct {
	Name   string `json:"name"`
	URL    string `json:"url,omitempty"`
	Branch string `json:"branch,omitempty"`
}

func parseReposVar(v any) []repoEntry {
	if v == nil || models.IsBlankVar(v) {
		return nil
	}
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		var repos []repoEntry
		if json.Unmarshal([]byte(s), &repos) != nil {
			return nil
		}
		out := make([]repoEntry, 0, len(repos))
		for _, r := range repos {
			r.Name = strings.TrimSpace(r.Name)
			r.URL = strings.TrimSpace(r.URL)
			r.Branch = strings.TrimSpace(r.Branch)
			// A blank name defaults to the repo derived from its clone URL.
			if r.Name == "" {
				r.Name = repoNameFromURL(r.URL)
			}
			if !safeRepoName(r.Name) {
				continue
			}
			out = append(out, r)
		}
		return out
	case []any:
		out := make([]repoEntry, 0, len(t))
		for _, item := range t {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			url := strings.TrimSpace(str2(m["url"]))
			name := strings.TrimSpace(str2(m["name"]))
			// A blank name defaults to the repo derived from its clone URL.
			if name == "" {
				name = repoNameFromURL(url)
			}
			if !safeRepoName(name) {
				continue
			}
			out = append(out, repoEntry{
				Name:   name,
				URL:    url,
				Branch: strings.TrimSpace(str2(m["branch"])),
			})
		}
		return out
	default:
		return nil
	}
}

func repoWorkspacePath(name string) string {
	name = strings.TrimSpace(name)
	// Flat multi-repo layout: every named repo lives at <workspace>/<name>/.
	// Empty falls back to the workspace root (single-repo callers).
	if name == "" {
		return "/root/workspace"
	}
	return "/root/workspace/" + name
}

// safeRepoName reports whether name is safe to use as a flat workspace subdir
// (<workspace>/<name>/). It rejects empty names, "."/"..", and any name with a
// path separator so a malicious or mistyped `repos[].name` can never escape the
// workspace root when clones/paths are derived from it. Unsafe entries are
// dropped at parse time (parseReposVar) so they never reach GIT_REPOS, prompts,
// or git/glab commands; startup.sh applies the same guard defensively.
func safeRepoName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, "/\\")
}

// repoNameFromURL derives a workspace subdir name from a clone URL: the last
// path segment without a trailing ".git". Empty when it can't be derived.
func repoNameFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimSuffix(raw, "/")
	seg := raw
	if i := strings.LastIndexAny(seg, "/:"); i >= 0 {
		seg = seg[i+1:]
	}
	seg = strings.TrimSuffix(seg, ".git")
	return strings.TrimSpace(seg)
}

// parseBranchesVar parses the run-scoped `branches` variable (name→branch map)
// that an upstream implement node publishes so downstream fresh clones check
// out each repo's working branch. Accepts a JSON string or a decoded map.
func parseBranchesVar(v any) map[string]string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		m := map[string]string{}
		if json.Unmarshal([]byte(s), &m) != nil {
			return nil
		}
		return m
	case map[string]any:
		m := map[string]string{}
		for k, val := range t {
			if s := strings.TrimSpace(str2(val)); s != "" {
				m[k] = s
			}
		}
		return m
	case map[string]string:
		return t
	default:
		return nil
	}
}

// resolveRepos builds the (flat) clone list from vars.repos. Every repository —
// including a lone one — is cloned to <workspace>/<name>/; the workspace root is
// never a git repo. Returns nil when no usable repos are configured (a pure
// artifact flow / empty workspace). Each repo's checkout branch resolves from
// vars.branches[name] (downstream continuity) over the entry's own branch.
func resolveRepos(req NodeReq) []sandbox.RepoSpec {
	entries := parseReposVar(req.Vars["repos"])
	if len(entries) == 0 {
		return nil
	}
	branches := parseBranchesVar(req.Vars["branches"])
	seen := map[string]bool{}
	out := make([]sandbox.RepoSpec, 0, len(entries))
	for _, e := range entries {
		name := strings.TrimSpace(e.Name)
		repoURL := strings.TrimSpace(e.URL)
		if name == "" || repoURL == "" || seen[name] {
			continue
		}
		branch := e.Branch
		if b := strings.TrimSpace(branches[name]); b != "" {
			branch = b
		}
		seen[name] = true
		out = append(out, sandbox.RepoSpec{Name: name, URL: repoURL, Branch: branch})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// firstRepoURL returns the clone URL of the run's first configured repo (for
// GITLAB_URL host derivation / sandbox display). Empty when no repos.
func firstRepoURL(req NodeReq) string {
	repos := resolveRepos(req)
	if len(repos) == 0 {
		return ""
	}
	return repos[0].URL
}

// nodeRepo resolves the single repository a node's git/glab commands operate
// in: submit_mr pins config["repo"] (target repo name); otherwise a lone
// configured repo is used. Returns the in-sandbox working directory (the repo's
// flat subdir, or the workspace root when nothing resolves) and the repo's
// clone URL (empty when it can't be determined).
func (c *acpProvider) nodeRepo(req NodeReq) (dir, url string) {
	repos := resolveRepos(req)
	name := strings.TrimSpace(str2(req.Config["repo"]))
	if name == "" && len(repos) == 1 {
		name = repos[0].Name
	}
	for _, r := range repos {
		if r.Name == name {
			url = r.URL
			break
		}
	}
	return repoWorkspacePath(name), url
}

// nodeRepoURL is nodeRepo's URL, falling back to the first repo so GitLab host
// derivation still works when the node didn't pin a specific repo.
func (c *acpProvider) nodeRepoURL(req NodeReq) string {
	if _, u := c.nodeRepo(req); u != "" {
		return u
	}
	return firstRepoURL(req)
}

func configTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

// testNodePromptExtras injects block_on_skipped and, in multi-repo mode, the
// repoScope testing guidance. The flat multi-repo layout itself is injected
// separately (multiRepoLayoutText) for every workspace-touching node.
func testNodePromptExtras(req NodeReq) string {
	if req.NodeType != "test" {
		return ""
	}
	var b strings.Builder
	if configTruthy(req.Config["block_on_skipped"]) {
		b.WriteString("\n\n## 节点配置:block_on_skipped\n本节点已启用 **block_on_skipped=true**:任一 skipped 用例将阻塞测试门禁,请尽量避免无理由跳过,或在 detail 说明具体原因。\n")
	}
	// repoScope guidance keys off vars.repos names (works even for name-only
	// entries); the concrete clone layout is described by multiRepoLayoutText.
	repos := parseReposVar(req.Vars["repos"])
	if len(repos) == 0 {
		return b.String()
	}
	repoScope := strings.TrimSpace(str2(req.Config["repoScope"]))
	if repoScope == "" {
		repoScope = "all"
	}
	b.WriteString("\n\n## 多仓测试范围\n")
	if strings.EqualFold(repoScope, "all") {
		b.WriteString("- **repoScope=all**:须对全部相关仓分别执行测试并汇总至单一 `set_test_result`;cases 的 `name` 使用 `[仓名] 用例描述` 前缀。\n")
	} else {
		fmt.Fprintf(&b, "- **repoScope=%s**:仅在该仓目录 `%s/` 内执行测试、读写文件与运行命令;不要操作其它仓。\n", repoScope, repoWorkspacePath(repoScope))
	}
	return b.String()
}

// multiRepoLayoutText describes the flat workspace layout for the agent: the
// workspace root is never a git repo and every repository — even a lone one —
// lives at /root/workspace/<name>/. Returns "" when no repos are configured
// (a pure artifact flow / empty workspace).
func multiRepoLayoutText(req NodeReq) string {
	repos := resolveRepos(req)
	if len(repos) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## 工作区仓库布局(repos)\n")
	b.WriteString("- **平级布局**:工作区根 `/root/workspace` 本身不是 git 仓库;每个仓库(即使只有一个)位于 `/root/workspace/<name>/`,各自独立 git 根。\n")
	b.WriteString("- **仓库清单**:\n")
	for _, r := range repos {
		fmt.Fprintf(&b, "  - `%s` → `%s/`\n", r.Name, repoWorkspacePath(r.Name))
	}
	b.WriteString("- 操作某个仓前先 `cd` 进其目录;每个仓的 `git` 提交/推送、依赖安装、测试与建 MR 都在对应仓目录内进行。\n")
	return b.String()
}

// submitMRRepoNote tells a submit_mr node which repo directory to operate in
// when the run is multi-repo. Returns "" for single-repo or non-submit_mr nodes.
func submitMRRepoNote(req NodeReq) string {
	if req.NodeType != "submit_mr" {
		return ""
	}
	repos := resolveRepos(req)
	if len(repos) == 0 {
		return ""
	}
	repo := strings.TrimSpace(str2(req.Config["repo"]))
	if repo == "" {
		return "\n\n## 多仓 MR 目标仓\n本节点未配置 `repo`(目标仓名)。请对存在待合并工作分支的仓分别 `cd` 进其目录后完成 push 与按托管商建单（git + 对应 CLI glab/gh）。\n"
	}
	return fmt.Sprintf("\n\n## 多仓 MR 目标仓\n本节点针对仓 `%s`:所有 `git` 与对应 CLI（`glab`/`gh`）操作前先 `cd %s`,仅在该仓目录内 push 源分支并建合并请求。\n", repo, repoWorkspacePath(repo))
}

func str2(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if len(s) > 120 {
		return textutil.TruncateBytes(s, 120, "")
	}
	return s
}
