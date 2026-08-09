package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"
	"github.com/cocofhu/approving/internal/sandbox"
	"github.com/cocofhu/approving/internal/textutil"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func randID() string { return uuid.NewString()[:8] }

// SandboxService manages long-lived, interactive sandboxes used for Agent
// chat-testing. Each sandbox is a Docker container bound to one Agent profile
// (its rules/skills/mcp/env), kept alive across multiple chat turns and
// reclaimed by an idle TTL sweeper. State is persisted (models.Sandbox) so the
// pool survives restarts; the live ACP WebSocket connection is re-established
// lazily on the next chat after a restart.
//
// Per-run workflow node sandboxes are NOT managed here — they remain ephemeral
// inside the runtime provider.
type SandboxService struct {
	db     *gorm.DB
	mgr    *sandbox.Manager
	skills *SkillService
	host   *mcp.Host

	profilesRoot      string
	platformRulesRoot string
	mcpEndpoint       string
	env               map[string]string
	chatTimeout       time.Duration
	// ttl / runTTL / max are runtime-tunable via the settings page, so they are
	// held atomically (read from many sites, some already under s.mu) rather
	// than guarded by s.mu — avoiding any lock-reentrancy at the read points.
	ttl    atomic.Int64 // idle TTL for interactive test sandboxes (ns)
	runTTL atomic.Int64 // retention TTL for finished run node sandboxes (ns)
	max    atomic.Int64 // max concurrently live interactive test sandboxes

	mu   sync.Mutex
	live map[uint]*liveSandbox
	// runActive marks per-run node sandboxes (by container name) that are
	// currently executing. These are owned by the runtime provider, not by
	// this service's ACP pool; the flag protects them from being stopped or
	// swept while in use.
	runActive map[string]bool
	// agentOnDestroy revokes thread-bound agent MCP sessions when a
	// purpose=agent|pm sandbox is torn down (optional; SetAgentSandboxDestroyHook).
	agentOnDestroy AgentSandboxDestroyHook
	// testScheduler hooks Register/Restore and Unregister for purpose=test
	// read-only task-scheduler injection (optional; SetTestSchedulerHooks).
	testScheduler TestSchedulerHooks
}

// TestSchedulerHooks wires purpose=test scheduler session lifecycle.
type TestSchedulerHooks struct {
	Register   func(projectID, profile, runID, token string)
	Unregister func(token string)
}

// liveSandbox is the in-memory connection state for a running sandbox.
type liveSandbox struct {
	sb   *sandbox.Sandbox
	acp  *sandbox.ACPClient
	home string
	busy bool
}

// resolveSandboxImage picks the per-acpBackend image from live config (nil-safe).
func resolveSandboxImage(backend string) string {
	return config.GetConfig().ResolveSandboxImage(backend)
}

// SandboxOptions configures the service.
type SandboxOptions struct {
	ProfilesRoot      string
	PlatformRulesRoot string
	MCPEndpoint       string
	// Env is the vendor-neutral sandbox env (e.g. CURSOR_API_KEY for the
	// reference image), injected into interactive test sandboxes.
	Env         map[string]string
	ChatTimeout time.Duration
	TTL         time.Duration
	// RunTTL is how long a finished run's node sandbox is retained (kept alive)
	// for debugging before the idle sweeper reclaims it.
	RunTTL time.Duration
	Max    int
}

// SandboxView is the API shape: the persisted record plus live-derived flags.
type SandboxView struct {
	models.Sandbox
	ContainerStatus string `json:"containerStatus"`
	Busy            bool   `json:"busy"`
	Connected       bool   `json:"connected"`
	HasCodeServer   bool   `json:"hasCodeServer"`
	HasACP          bool   `json:"hasAcp"`
	// Password is the sandbox Token also injected as PASSWORD /
	// ROOT_PASSWORD / CURSOR_ACP_PASSWORD. Exposed so operators can log into
	// code-server / ACP when opening the published host:port directly
	// (remote-dev Environment.password parity). Empty when unset.
	Password string `json:"password,omitempty"`
	// Endpoints are user-visible host:port addresses. GetView only returns
	// session/ide/ssh; CDP/noVNC (9222/6080) are internal and never included.
	// List leaves this nil so JSON omits the field.
	Endpoints map[string]string `json:"endpoints,omitempty"`
}

// NewSandboxService builds the service.
func NewSandboxService(db *gorm.DB, mgr *sandbox.Manager, skills *SkillService, host *mcp.Host, opts SandboxOptions) *SandboxService {
	if opts.TTL <= 0 {
		opts.TTL = 30 * time.Minute
	}
	if opts.Max <= 0 {
		opts.Max = 5
	}
	if opts.ChatTimeout <= 0 {
		opts.ChatTimeout = 10 * time.Minute
	}
	if opts.RunTTL <= 0 {
		opts.RunTTL = opts.TTL
	}
	s := &SandboxService{
		db: db, mgr: mgr, skills: skills, host: host,
		profilesRoot:      opts.ProfilesRoot,
		platformRulesRoot: opts.PlatformRulesRoot,
		mcpEndpoint:       opts.MCPEndpoint,
		env:               opts.Env,
		chatTimeout:       opts.ChatTimeout,
		live:              map[uint]*liveSandbox{},
		runActive:         map[string]bool{},
	}
	s.ttl.Store(int64(opts.TTL))
	s.runTTL.Store(int64(opts.RunTTL))
	s.max.Store(int64(opts.Max))
	return s
}

// TTL / RunTTL / MaxTestSandboxes return the live tunable values.
func (s *SandboxService) TTL() time.Duration    { return time.Duration(s.ttl.Load()) }
func (s *SandboxService) RunTTL() time.Duration { return time.Duration(s.runTTL.Load()) }
func (s *SandboxService) MaxTestSandboxes() int { return int(s.max.Load()) }

// Manager exposes the underlying docker manager (for preview proxy / probes).
func (s *SandboxService) Manager() *sandbox.Manager { return s.mgr }

// SetTTLs updates the interactive and run-sandbox TTLs at runtime (settings
// page). Non-positive values are ignored so a partial update can't zero a TTL.
func (s *SandboxService) SetTTLs(runTTL, testTTL time.Duration) {
	if testTTL > 0 {
		s.ttl.Store(int64(testTTL))
	}
	if runTTL > 0 {
		s.runTTL.Store(int64(runTTL))
	}
}

// SetMaxTestSandboxes updates the interactive sandbox cap at runtime.
func (s *SandboxService) SetMaxTestSandboxes(n int) {
	if n > 0 {
		s.max.Store(int64(n))
	}
}

// BeginRunSandbox records a per-run node sandbox as "creating" BEFORE the (slow)
// gateway provisioning completes, so it shows up in the sandbox list and the
// node's live log as "starting" instead of a 404 during the cold-start window.
// The row uses a placeholder name; RegisterRunSandbox later adopts the real
// gateway id and flips it to "running", or UnregisterRunSandbox removes it on
// failure. Implements runtime.RunSandboxBeginner. Keyed by run+node so a retry
// replaces the previous placeholder row.
func (s *SandboxService) BeginRunSandbox(info runtime.RunSandboxInfo) {
	if info.Name == "" {
		return
	}
	fields := map[string]any{
		"name":          info.Name,
		"profile":       info.Profile,
		"purpose":       "run",
		"status":        "creating",
		"repo_url":      info.RepoURL,
		"run_id":        info.RunID,
		"workflow_id":   info.WorkflowID,
		"workflow_name": info.WorkflowName,
		"node_id":       info.NodeID,
		"home_dir":      info.HomeDir,
		"error":         "",
		"destroy_at":    nil,
		"updated_at":    time.Now(),
	}
	if info.Token != "" {
		fields["token"] = info.Token
	}
	if err := s.db.Where(models.Sandbox{RunID: info.RunID, NodeID: info.NodeID, Purpose: "run"}).
		Assign(fields).FirstOrCreate(&models.Sandbox{}).Error; err != nil {
		log.Warn().Err(err).Str("name", info.Name).Msg("begin run sandbox failed")
	}
	s.mu.Lock()
	s.runActive[info.Name] = true
	s.mu.Unlock()
}

// RegisterRunSandbox records a per-run workflow node sandbox so it appears in
// the sandbox list while the node runs. Implements runtime.SandboxRegistry.
// The row carries no DestroyAt (its lifecycle is owned by the runtime
// provider, not the idle-TTL sweeper) and is marked active so it cannot be
// stopped/destroyed from the UI mid-run. When a "creating" placeholder row was
// pre-registered (BeginRunSandbox), it is adopted in place: the gateway-assigned
// id replaces the placeholder name and the status flips to "running".
func (s *SandboxService) RegisterRunSandbox(info runtime.RunSandboxInfo) {
	if info.Name == "" {
		return
	}
	fields := map[string]any{
		"name":             info.Name,
		"profile":          info.Profile,
		"purpose":          "run",
		"status":           "running",
		"host":             info.Host,
		"acp_port":         info.ACPPort,
		"code_server_port": info.CodeServerPort,
		"repo_url":         info.RepoURL,
		"run_id":           info.RunID,
		"workflow_id":      info.WorkflowID,
		"workflow_name":    info.WorkflowName,
		"node_id":          info.NodeID,
		"home_dir":         info.HomeDir,
		"error":            "",
		"destroy_at":       nil,
		"updated_at":       time.Now(),
	}
	if info.Token != "" {
		fields["token"] = info.Token
	}
	// Adopt the pre-registered "creating" placeholder row (by run+node) in place,
	// swapping its placeholder name for the gateway id. Falls back to an upsert
	// by name when no placeholder exists (legacy path / no BeginRunSandbox).
	var existing models.Sandbox
	err := s.db.Where("run_id = ? AND node_id = ? AND purpose = ? AND status = ?",
		info.RunID, info.NodeID, "run", "creating").
		Order("updated_at desc").First(&existing).Error
	if err == nil {
		oldName := existing.Name
		if uerr := s.db.Model(&models.Sandbox{}).Where("id = ?", existing.ID).
			Updates(fields).Error; uerr != nil {
			log.Warn().Err(uerr).Str("name", info.Name).Msg("adopt run sandbox failed")
		}
		s.mu.Lock()
		if oldName != "" && oldName != info.Name {
			delete(s.runActive, oldName)
		}
		s.runActive[info.Name] = true
		s.mu.Unlock()
		return
	}
	if err := s.db.Where(models.Sandbox{Name: info.Name}).
		Assign(fields).FirstOrCreate(&models.Sandbox{}).Error; err != nil {
		log.Warn().Err(err).Str("name", info.Name).Msg("register run sandbox failed")
	}
	s.mu.Lock()
	s.runActive[info.Name] = true
	s.mu.Unlock()
}

// RetireRunSandbox keeps a finished run's node sandbox alive for a debug TTL
// instead of destroying it immediately: it clears the busy/active flag and sets
// an idle deadline so the sweeper reclaims it later, while it remains browsable
// (terminal / IDE / ACP / container logs) in the sandbox UI. Implements
// runtime.RunSandboxRetirer.
func (s *SandboxService) RetireRunSandbox(name string) {
	if name == "" {
		return
	}
	s.mu.Lock()
	delete(s.runActive, name)
	s.mu.Unlock()
	at := time.Now().Add(s.RunTTL())
	if err := s.db.Model(&models.Sandbox{}).Where("name = ?", name).
		Updates(map[string]any{"status": "running", "destroy_at": &at, "updated_at": time.Now()}).Error; err != nil {
		log.Warn().Err(err).Str("name", name).Msg("retire run sandbox failed")
		return
	}
	log.Info().Str("name", name).Dur("ttl", s.RunTTL()).Msg("run sandbox retired for debugging (idle ttl)")
}

// UnregisterRunSandbox clears a per-run node sandbox record once the runtime
// provider tears down its container. Implements runtime.SandboxRegistry.
func (s *SandboxService) UnregisterRunSandbox(name string) {
	if name == "" {
		return
	}
	// Archive the raw container logs before the provider removes the container,
	// so the startup/exec output (e.g. a failed git clone) survives for
	// post-mortem troubleshooting even after the sandbox is gone.
	s.archiveLog(context.Background(), name)
	s.mu.Lock()
	delete(s.runActive, name)
	s.mu.Unlock()
	if err := s.db.Where("name = ?", name).Delete(&models.Sandbox{}).Error; err != nil {
		log.Warn().Err(err).Str("name", name).Msg("unregister run sandbox failed")
	}
}

// archiveLog captures a container's combined stdout/stderr and upserts it into
// the sandbox_logs table (keyed by container name), copying run/node/profile
// attribution from the live sandbox row when present. Best-effort: a container
// that is already gone (empty output) is skipped so we never clobber a prior
// good snapshot with an empty one. Read failures are logged and skipped — they
// must not block teardown.
func (s *SandboxService) archiveLog(ctx context.Context, name string) {
	if name == "" {
		return
	}
	out, err := s.mgr.Logs(ctx, name, 5000)
	if err != nil {
		log.Warn().Err(err).Str("name", name).Msg("archive sandbox log: read failed")
		return
	}
	if strings.TrimSpace(out) == "" {
		return
	}
	// Cap stored size to keep the DB lean while preserving the tail (where
	// failures usually surface).
	const maxBytes = 256 * 1024
	if len(out) > maxBytes {
		out = textutil.TruncateTailBytes(out, maxBytes, "…(truncated)…\n")
	}
	var row models.Sandbox
	rec := models.SandboxLog{Name: name, Content: out}
	if err := s.db.Where("name = ?", name).First(&row).Error; err == nil {
		rec.RunID = row.RunID
		rec.NodeID = row.NodeID
		rec.Profile = row.Profile
	}
	if err := s.db.Where(models.SandboxLog{Name: name}).
		Assign(map[string]any{
			"run_id":     rec.RunID,
			"node_id":    rec.NodeID,
			"profile":    rec.Profile,
			"content":    rec.Content,
			"updated_at": time.Now(),
		}).FirstOrCreate(&models.SandboxLog{}).Error; err != nil {
		log.Warn().Err(err).Str("name", name).Msg("archive sandbox log failed")
	}
}

// SandboxViewForRunNode returns the sandbox view for a workflow run's node,
// using the same DB lookup as PreviewService.SandboxForRunNode (latest by
// updated_at). Returns an error when no record exists or the container is not
// running.
func (s *SandboxService) SandboxViewForRunNode(ctx context.Context, runID, nodeID string) (*SandboxView, error) {
	var row models.Sandbox
	if err := s.db.Where("run_id = ? AND node_id = ? AND purpose = ?", runID, nodeID, "run").
		Order("updated_at desc").First(&row).Error; err != nil {
		return nil, fmt.Errorf("not found")
	}
	// A sandbox still provisioning (no live container yet) is surfaced as-is so
	// the UI can render a "starting" state during the cold-start window instead
	// of a 404. Running rows require a live container.
	if row.Status != "creating" && s.mgr.Status(ctx, row.Name) != "running" {
		return nil, fmt.Errorf("not found")
	}
	v := s.view(ctx, &row)
	return &v, nil
}

// NodeSandboxLog returns the container logs for a workflow run's node sandbox.
// If the sandbox is still live, it fetches fresh logs from the container
// (including a successful empty read → live=true); otherwise it falls back to
// the archived snapshot captured at teardown. Live read failures are returned
// to the caller and must not be disguised as "no log source".
func (s *SandboxService) NodeSandboxLog(ctx context.Context, runID, nodeID string) (content string, live bool, err error) {
	var row models.Sandbox
	q := s.db.Where("run_id = ? AND purpose = ?", runID, "run")
	if nodeID != "" {
		q = q.Where("node_id = ?", nodeID)
	}
	if e := q.Order("updated_at desc").First(&row).Error; e == nil {
		if s.mgr.Status(ctx, row.Name) == "running" {
			out, lerr := s.mgr.Logs(ctx, row.Name, 5000)
			if lerr != nil {
				return "", false, lerr
			}
			return out, true, nil
		}
	}
	var rec models.SandboxLog
	rq := s.db.Where("run_id = ?", runID)
	if nodeID != "" {
		rq = rq.Where("node_id = ?", nodeID)
	}
	if e := rq.Order("updated_at desc").First(&rec).Error; e != nil {
		return "", false, gorm.ErrRecordNotFound
	}
	return rec.Content, false, nil
}

// RunSandboxLogs returns all archived sandbox log snapshots for a run, ordered
// by node then capture time. Used by the run log export to bundle the raw
// container stdout/stderr (docker logs) alongside the agent event logs.
func (s *SandboxService) RunSandboxLogs(runID string) []models.SandboxLog {
	var rows []models.SandboxLog
	if err := s.db.Where("run_id = ?", runID).
		Order("node_id asc, updated_at asc").Find(&rows).Error; err != nil {
		return nil
	}
	return rows
}

// SandboxLogByID returns the container logs for a sandbox by its record id,
// preferring live logs (including successful empty reads) and falling back to
// the archived snapshot. Live read failures are propagated.
func (s *SandboxService) SandboxLogByID(ctx context.Context, id uint) (content string, live bool, err error) {
	row, err := s.Get(id)
	if err != nil {
		return "", false, err
	}
	if s.mgr.Status(ctx, row.Name) == "running" {
		out, lerr := s.mgr.Logs(ctx, row.Name, 5000)
		if lerr != nil {
			return "", false, lerr
		}
		return out, true, nil
	}
	var rec models.SandboxLog
	if e := s.db.Where("name = ?", row.Name).First(&rec).Error; e != nil {
		return "", false, gorm.ErrRecordNotFound
	}
	return rec.Content, false, nil
}

// findReusable returns an existing interactive test sandbox for the same agent
// profile and project with an empty-workspace (no repo) that can be reused, preferring a
// live/running container. A row still "creating" is returned as-is (the UI will
// poll it to running). "running" rows are only reused when the container is
// actually alive; stale rows are skipped so the caller creates a fresh one.
// Returns nil when nothing suitable exists. Callers only invoke this for the
// no-repo case (repo-backed sandboxes always get a fresh workspace).
func (s *SandboxService) findReusable(ctx context.Context, profile, projectID string) *models.Sandbox {
	var rows []models.Sandbox
	if err := s.db.
		Where("purpose = ? AND profile = ? AND repo_url = ? AND project_id = ? AND status IN ?",
			"test", profile, "", projectID, []string{"running", "creating"}).
		Order("created_at desc").Find(&rows).Error; err != nil {
		return nil
	}
	// Prefer a confirmed-running container; fall back to one still launching.
	for i := range rows {
		if rows[i].Status == "running" && s.mgr.Status(ctx, rows[i].Name) == "running" {
			return &rows[i]
		}
	}
	for i := range rows {
		if rows[i].Status == "creating" {
			return &rows[i]
		}
	}
	return nil
}

// Open binds an interactive sandbox to an Agent profile. It persists a
// "creating" record and returns immediately; the (potentially slow) container
// launch + ACP handshake runs in the background and flips the record to
// "running" on success or "error" on failure. Callers (the UI) poll Get /
// List to observe the transition. repos empty/nil = empty workspace.
// Existing live sandboxes for the same agent are reused when no repos are
// configured (see findReusable); any configured clone list forces a fresh sandbox.
func (s *SandboxService) Open(ctx context.Context, profile string, repos []sandbox.RepoSpec, projectID string) (*models.Sandbox, error) {
	agent, ok := s.skills.Get(profile)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", profile)
	}
	// Data ownership follows the Agent's home project, not the UI-selected one.
	_ = projectID
	projectID = strings.TrimSpace(agent.ProjectID)
	// Reuse an existing test sandbox bound to the same agent when one is still
	// alive, instead of spinning up a fresh container every time. The chat path
	// (ensureConnected) transparently re-attaches if the live ACP connection was
	// dropped, so a reused "running" row is immediately usable.
	//
	// Skip reuse when any repos are configured: the caller wants those clones
	// in a fresh workspace, so always create a new sandbox for them.
	if len(repos) == 0 {
		if row := s.findReusable(ctx, profile, projectID); row != nil {
			at := time.Now().Add(s.TTL())
			s.db.Model(&models.Sandbox{}).Where("id = ?", row.ID).Updates(map[string]any{"destroy_at": &at, "updated_at": time.Now()})
			row.DestroyAt = &at
			log.Info().Str("name", row.Name).Str("profile", profile).Str("project", projectID).Uint("id", row.ID).Msg("reusing existing test sandbox")
			return row, nil
		}
	}
	if maxN := s.MaxTestSandboxes(); s.activeCount() >= maxN {
		return nil, fmt.Errorf("已达到测试沙箱上限(%d),请先清理空闲沙箱", maxN)
	}

	runID := "test-" + strings.ReplaceAll(filepath.Base(profile), "/", "") + "-" + randID()
	token := s.host.RegisterRun(runID)
	// Pre-allocate the container name so the "creating" record references the
	// same container the background launch will create (keeps startup reconcile
	// consistent even if the process restarts mid-launch).
	name := sandbox.NewContainerName()

	row := &models.Sandbox{
		Name: name, Profile: profile, Purpose: "test", Status: "creating",
		RepoURL: firstTestRepoURL(repos), RunID: runID, Token: token, ProjectID: projectID,
	}
	if err := s.db.Create(row).Error; err != nil {
		s.host.UnregisterRun(runID)
		return nil, err
	}
	log.Info().Str("name", name).Str("profile", profile).Str("project", projectID).Uint("id", row.ID).Msg("test sandbox creating")
	go s.startContainer(row.ID, name, profile, projectID, runID, token, repos, agent)
	return row, nil
}

func firstTestRepoURL(repos []sandbox.RepoSpec) string {
	if len(repos) == 0 {
		return ""
	}
	return repos[0].URL
}

// startContainer performs the slow part of Open in the background: build the
// cursor home, launch the container, wait for ACP, connect, then flip the DB
// record to running. Any failure flips it to "error" (with a short TTL so the
// idle sweeper reclaims the dead row). Runs on a detached context since the
// originating HTTP request has already returned.
func (s *SandboxService) startContainer(id uint, name, profile, projectID, runID, token string, repos []sandbox.RepoSpec, agent Agent) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fail := func(err error) {
		log.Warn().Err(err).Str("name", name).Uint("id", id).Msg("test sandbox start failed")
		s.unregisterTestScheduler(projectID, token)
		s.host.UnregisterRun(runID)
		at := time.Now().Add(2 * time.Minute)
		s.db.Model(&models.Sandbox{}).Where("id = ?", id).Updates(map[string]any{
			"status": "error", "error": truncErr(err), "destroy_at": &at, "updated_at": time.Now(),
		})
	}
	// This runs detached from the originating request; a panic here would leave
	// the row stuck in "creating" forever. Recover and route through fail() so
	// the idle sweeper can reclaim it.
	defer func() {
		if r := recover(); r != nil {
			log.Error().Str("name", name).Uint("id", id).Interface("panic", r).
				Bytes("stack", debug.Stack()).Msg("test sandbox start panicked")
			fail(fmt.Errorf("panic during sandbox start: %v", r))
		}
	}()

	vars := s.testMcpVars(runID, token, projectID, profile)
	specs := s.buildTestSandboxSpecs(projectID, profile, runID, token, agent, vars)

	env := map[string]string{}
	for k, v := range s.env {
		if runtime.IsPlatformAuthEnvKey(k) {
			continue
		}
		env[k] = v
	}
	for k, v := range agent.Env {
		env[k] = substTemplate(v, vars)
	}
	backend := runtime.NormalizeBackend(agent.AcpBackend)
	merged, err := runtime.MergeAuthEnv(backend, env)
	if err != nil {
		fail(err)
		return
	}
	env = merged

	home, err := sandbox.BuildConfigHome(sandbox.ConfigHomeSpec{
		WorkDirSrc:           s.skills.WorkDir(profile),
		IncludeArtifactStore: hasArtifactStoreSpec(specs),
		MCP:                  specs,
		Settings:             runtime.CodeBuddySettingsForEnv(backend, env),
		AgentName:            profile,
		ProfilesRoot:         s.profilesRoot,
		GlobalRulesDir:       s.platformRulesRoot,
	})
	if err != nil {
		fail(fmt.Errorf("build cursor home: %w", err))
		return
	}

	env["ACP_BACKEND"] = string(backend)
	env["CONFIG_ROOT"] = agent.Layout.ConfigRoot
	// Expose the run-scoped artifact-store coordinates as process env so the
	// in-sandbox artifact-upload CLI can reach the store (parity with the
	// cursor runtime); the token is already provided via mcp.json.
	for k, v := range vars {
		if v == "" {
			continue
		}
		env[k] = v
	}
	// remote-dev parity: PASSWORD / ROOT_PASSWORD / CURSOR_ACP_PASSWORD so
	// code-server (8744) and ACP bridge (8765) accept the same secret for
	// proxied auto-login and direct host:port access.
	sandbox.ApplyPasswords(env, token)
	// This interactive path has no workflow `repos` variable to reference, so
	// wire GIT_REPOS explicitly here. Overrides any stray "${vars.repos}" the
	// Agent env may carry (unresolved in this path).
	env["GIT_REPOS"] = sandbox.EncodeRepos(repos)

	sb, err := s.mgr.Create(ctx, sandbox.Spec{
		Name:         name,
		Image:        resolveSandboxImage(string(backend)),
		Env:          env,
		ConfigHome:   home,
		ConfigRoot:   agent.Layout.ConfigRoot,
		WorkspaceDir: agent.Layout.WorkspaceDir,
	})
	if err != nil {
		_ = os.RemoveAll(home)
		fail(fmt.Errorf("create sandbox: %w", err))
		return
	}

	if err := sandbox.WaitForACPReady(ctx, sb.Host, sb.Port, sb.Password, 120*time.Second); err != nil {
		sb.Destroy(context.Background())
		_ = os.RemoveAll(home)
		fail(fmt.Errorf("acp not ready: %w", err))
		return
	}
	acp := sb.ACP().
		WithSession(sb.WorkspaceDir, mcpServersJSON(specs)).
		WithBridgeModel(env["ACP_BRIDGE_MODEL"])
	if err := acp.Connect(ctx); err != nil {
		acp.Close()
		sb.Destroy(context.Background())
		_ = os.RemoveAll(home)
		fail(fmt.Errorf("acp connect: %w", err))
		return
	}

	destroyAt := time.Now().Add(s.TTL())
	// The gateway assigns the authoritative sandbox id; adopt it as the row's
	// Name (the pre-allocated placeholder was only a correlation label) so all
	// later control-plane calls (Status/Attach/Destroy) address the real id.
	res := s.db.Model(&models.Sandbox{}).Where("id = ?", id).Updates(map[string]any{
		"status": "running", "name": sb.Name, "host": sb.Host, "acp_port": sb.Port,
		"code_server_port": sb.CodeServerPort, "error": "",
		"destroy_at": &destroyAt, "updated_at": time.Now(),
	})
	// Row gone (user destroyed it mid-launch) or update failed: tear everything
	// down instead of leaking an orphan container.
	if res.Error != nil || res.RowsAffected == 0 {
		acp.Close()
		sb.Destroy(context.Background())
		_ = os.RemoveAll(home)
		s.unregisterTestScheduler(projectID, token)
		s.host.UnregisterRun(runID)
		return
	}
	s.mu.Lock()
	s.live[id] = &liveSandbox{sb: sb, acp: acp, home: home}
	s.mu.Unlock()
	log.Info().Str("name", sb.Name).Str("profile", profile).Uint("id", id).Msg("test sandbox opened")
}

// Chat runs one streaming chat turn (text + optional image attachments)
// against a sandbox, forwarding each raw ACP event frame to onEvent. While a
// turn runs the sandbox is marked busy and its idle TTL is suspended. The busy
// guard stays as a safety net; serial ordering of multiple messages is handled
// by the caller's single-worker queue (see handlers.SandboxChat).
func (s *SandboxService) Chat(ctx context.Context, id uint, text string, images []models.PromptImage, onEvent func(json.RawMessage)) error {
	_, _, err := s.ChatWithTimeout(ctx, id, text, images, 0, onEvent)
	return err
}

// ChatWithTimeout is Chat with a per-call turn timeout override. timeout<=0
// uses the service default (s.chatTimeout). Used by channel/cron turns that may
// legitimately run longer than an interactive UI turn.
//
// Returns the turn's TokenUsage (+ optional by-model breakdown) from
// prompt_done (nil = not reported). Callers that account usage (PmTurnRunner)
// must persist only after a successful turn.
func (s *SandboxService) ChatWithTimeout(ctx context.Context, id uint, text string, images []models.PromptImage, timeout time.Duration, onEvent func(json.RawMessage)) (*models.TokenUsage, models.TokenUsageByModel, error) {
	ls, row, err := s.ensureConnected(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	if ls.busy {
		s.mu.Unlock()
		return nil, nil, fmt.Errorf("sandbox busy")
	}
	ls.busy = true
	s.mu.Unlock()
	// Suspend TTL while the turn runs.
	s.db.Model(&models.Sandbox{}).Where("id = ?", id).Update("destroy_at", nil)
	defer func() {
		s.mu.Lock()
		ls.busy = false
		s.mu.Unlock()
		at := time.Now().Add(s.TTL())
		s.db.Model(&models.Sandbox{}).Where("id = ?", id).Updates(map[string]any{"destroy_at": &at, "updated_at": time.Now()})
	}()

	turnTimeout := s.chatTimeout
	if timeout > 0 {
		turnTimeout = timeout
	}
	chatCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	defer cancel()
	result, err := ls.acp.ChatStream(chatCtx, text, images, onEvent)
	_ = row
	if result == nil {
		return nil, nil, err
	}
	return models.CloneTokenUsage(result.Usage), models.CloneTokenUsageByModel(result.UsageByModel), err
}

// acpHostPort resolves the live ACP host/port for a sandbox, preferring the
// in-memory connection and falling back to attaching to the running container.
func (s *SandboxService) acpHostPort(ctx context.Context, id uint) (string, int, error) {
	row, err := s.Get(id)
	if err != nil {
		return "", 0, err
	}
	s.mu.Lock()
	ls := s.live[id]
	s.mu.Unlock()
	host, port := row.Host, row.ACPPort
	if ls != nil && ls.sb != nil {
		host, port = ls.sb.Host, ls.sb.Port
	}
	if host == "" || port == 0 {
		if s.mgr.Status(ctx, row.Name) != "running" {
			return "", 0, fmt.Errorf("sandbox container 不在运行")
		}
		sb, aerr := s.mgr.Attach(ctx, row.Name)
		if aerr != nil {
			return "", 0, fmt.Errorf("attach: %w", aerr)
		}
		host, port = sb.Host, sb.Port
	}
	return host, port, nil
}

// ACPUpstream returns the reachable "host:port" for the sandbox ACP bridge
// (session endpoint). Aligns iframe reverse-proxy dialing with chat/events.
func (s *SandboxService) ACPUpstream(ctx context.Context, id uint) (string, error) {
	// Prefer the gateway-published session endpoint when available so K8s
	// ClusterIP/node hosts win over a stale DB host.
	if row, err := s.Get(id); err == nil && s.mgr != nil && row.Name != "" {
		if addr, eerr := s.mgr.EndpointAddr(ctx, row.Name, "session"); eerr == nil && strings.TrimSpace(addr) != "" {
			return addr, nil
		}
	}
	host, port, err := s.acpHostPort(ctx, id)
	if err != nil {
		return "", err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, port), nil
}

// IDEUpstream returns the reachable "host:port" for the sandbox code-server
// (ide endpoint). Never hard-codes 127.0.0.1; Docker gateways that publish
// loopback endpoints still resolve to 127.0.0.1:<port> naturally.
func (s *SandboxService) IDEUpstream(ctx context.Context, id uint) (string, error) {
	row, err := s.Get(id)
	if err != nil {
		return "", err
	}
	if row.CodeServerPort == 0 {
		return "", fmt.Errorf("no code-server for this sandbox")
	}
	if s.mgr != nil && row.Name != "" {
		if addr, eerr := s.mgr.EndpointAddr(ctx, row.Name, "ide"); eerr == nil && strings.TrimSpace(addr) != "" {
			return addr, nil
		}
		if addr, herr := s.mgr.HostForPort(ctx, row.Name, row.CodeServerPort); herr == nil && strings.TrimSpace(addr) != "" {
			return addr, nil
		}
		if s.mgr.Status(ctx, row.Name) == "running" {
			if sb, aerr := s.mgr.Attach(ctx, row.Name); aerr == nil && sb != nil && sb.CodeServerPort > 0 {
				host := sb.Host
				if host == "" {
					host = sb.SSHHost
				}
				if host == "" {
					host = "127.0.0.1"
				}
				return fmt.Sprintf("%s:%d", host, sb.CodeServerPort), nil
			}
		}
	}
	host := row.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, row.CodeServerPort), nil
}

// Events reads the sandbox's full agent event log directly from the container
// (the cursor-acp bridge is the single source of truth) and returns it as the
// AcpEvent timeline. Works the same way for every sandbox — interactive test
// sandboxes here and per-run node sandboxes in the engine.
func (s *SandboxService) Events(ctx context.Context, id uint) ([]models.AcpEvent, error) {
	host, port, err := s.acpHostPort(ctx, id)
	if err != nil {
		return nil, err
	}
	res, _, ferr := sandbox.FetchEventLog(ctx, host, port)
	if ferr != nil {
		return nil, ferr
	}
	return res.AcpEvents(), nil
}

// EventLog reads the sandbox's raw agent event frames (unaggregated), used to
// rebuild the interactive chat transcript (with the original user prompts) when
// reopening a reused sandbox. Returns the frames as raw JSON messages.
func (s *SandboxService) EventLog(ctx context.Context, id uint) ([]json.RawMessage, error) {
	host, port, err := s.acpHostPort(ctx, id)
	if err != nil {
		return nil, err
	}
	frames, _, ferr := sandbox.FetchEventLogRaw(ctx, host, port)
	if ferr != nil {
		return nil, ferr
	}
	return frames, nil
}

// EventLogPage returns one page of raw event frames with cursor metadata.
func (s *SandboxService) EventLogPage(ctx context.Context, id uint, cursor string, limit int) (*sandbox.EventLogPageResult, error) {
	host, port, err := s.acpHostPort(ctx, id)
	if err != nil {
		return nil, err
	}
	return sandbox.FetchEventLogPage(ctx, host, port, cursor, limit)
}

// OpenTerminal attaches an interactive PTY shell to a sandbox over SSH.
func (s *SandboxService) OpenTerminal(ctx context.Context, id uint) (*sandbox.SSHTerminal, error) {
	row, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if s.mgr.Status(ctx, row.Name) != "running" {
		return nil, fmt.Errorf("container 不在运行")
	}
	return s.mgr.ExecPTY(ctx, row.Name, nil)
}

// Cancel asks the agent to abort the current turn (best effort).
func (s *SandboxService) Cancel(id uint) {
	s.mu.Lock()
	ls := s.live[id]
	s.mu.Unlock()
	if ls != nil && ls.acp != nil {
		_ = ls.acp.Cancel()
	}
}

// ensureConnected returns the live connection, re-establishing it lazily if the
// process restarted while the container kept running.
func (s *SandboxService) ensureConnected(ctx context.Context, id uint) (*liveSandbox, *models.Sandbox, error) {
	var row models.Sandbox
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, nil, fmt.Errorf("sandbox not found")
	}
	s.mu.Lock()
	ls := s.live[id]
	s.mu.Unlock()
	if ls != nil && ls.acp != nil && ls.acp.IsConnected() {
		return ls, &row, nil
	}
	if s.mgr.Status(ctx, row.Name) != "running" {
		return nil, nil, fmt.Errorf("sandbox container 不在运行(状态:%s)", row.Status)
	}
	sb, err := s.mgr.Attach(ctx, row.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("attach: %w", err)
	}
	// Attach rebuilds the handle from the gateway record, which carries no env;
	// restore the acp-bridge secret so ACP() can log in before dialing /ws.
	sb.Password = row.Token
	agent, _ := s.skills.Get(row.Profile)
	// Attach can't recover the per-Agent workspace dir; restore it from the
	// Agent's layout so the reconnected session matches the original.
	if agent.Layout.WorkspaceDir != "" {
		sb.WorkspaceDir = agent.Layout.WorkspaceDir
	}
	if agent.Layout.ConfigRoot != "" {
		sb.ConfigRoot = agent.Layout.ConfigRoot
	}
	specs := s.buildTestSandboxSpecs(row.ProjectID, row.Profile, row.RunID, row.Token, agent,
		s.testMcpVars(row.RunID, row.Token, row.ProjectID, row.Profile))
	// Rebuild + SSH-sync ConfigHome so remote pods missing mcp.json/rules
	// (hostPath ignored by K8s gateway) heal on reconnect.
	if s.mgr != nil {
		if home, err := sandbox.BuildConfigHome(sandbox.ConfigHomeSpec{
			WorkDirSrc:           s.skills.WorkDir(row.Profile),
			IncludeArtifactStore: hasArtifactStoreSpec(specs),
			MCP:                  specs,
			AgentName:            row.Profile,
			ProfilesRoot:         s.profilesRoot,
			GlobalRulesDir:       s.platformRulesRoot,
		}); err != nil {
			log.Warn().Err(err).Uint("sandbox", id).Msg("reconnect: rebuild config home failed")
		} else {
			s.mgr.EnsureConfigHome(ctx, sb, home, sb.ConfigRoot)
			_ = os.RemoveAll(home)
		}
		// Re-seed helpers on reconnect so SPA mcp_advertise proxies /
		// artifact-upload land after control-plane upgrades. Idempotent.
		s.mgr.EnsureHelpers(ctx, sb)
	}
	acp := sb.ACP().
		WithSession(sb.WorkspaceDir, mcpServersJSON(specs)).
		WithBridgeModel(agent.Env["ACP_BRIDGE_MODEL"])
	if err := acp.Connect(ctx); err != nil {
		acp.Close()
		return nil, nil, fmt.Errorf("reconnect acp: %w", err)
	}
	ls = &liveSandbox{sb: sb, acp: acp}
	s.mu.Lock()
	s.live[id] = ls
	s.mu.Unlock()
	return ls, &row, nil
}

// List returns all interactive sandboxes with live-derived status.
// Container statuses are filled from a single batch docker ps (ListStatuses)
// instead of N serial inspects. If Docker is unavailable, every row gets
// ContainerStatus "unknown" and the list still succeeds (HTTP 200).
func (s *SandboxService) List(ctx context.Context) ([]SandboxView, error) {
	var rows []models.Sandbox
	if err := s.db.Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	statusMap, dockerErr := s.mgr.ListStatuses(ctx)
	if dockerErr != nil {
		log.Warn().Err(dockerErr).Msg("sandbox list: batch docker ps failed; container status unknown")
	}
	out := make([]SandboxView, 0, len(rows))
	for i := range rows {
		out = append(out, s.viewWithStatus(ctx, &rows[i], statusMap, dockerErr != nil))
	}
	return out, nil
}

// Get returns the persisted record for one sandbox.
func (s *SandboxService) Get(id uint) (*models.Sandbox, error) {
	var row models.Sandbox
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, fmt.Errorf("not found")
	}
	return &row, nil
}

// GetView returns one sandbox with live-derived status (used by the UI to poll
// a "creating" sandbox until it turns running/error). It also lazily attaches
// gateway Endpoints for the detail modal; gateway failures degrade to an empty
// map so the rest of the view still succeeds.
func (s *SandboxService) GetView(ctx context.Context, id uint) (*SandboxView, error) {
	row, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	v := s.view(ctx, row)
	v.Endpoints = map[string]string{}
	if gw := s.mgr.Gateway(); gw != nil {
		if sb, err := gw.Get(ctx, row.Name); err != nil {
			log.Debug().Err(err).Str("sandbox", row.Name).Msg("sandbox GetView: gateway Get failed; endpoints empty")
		} else if sb != nil && sb.Endpoints != nil {
			v.Endpoints = publicSandboxEndpoints(sb.Endpoints)
		}
	}
	return &v, nil
}

var userVisibleEndpointKeys = map[string]struct{}{
	"session": {},
	"ide":     {},
	"ssh":     {},
}

// publicSandboxEndpoints is the user-side GetView whitelist: only session/ide/ssh.
// Named cdp/novnc, numeric 9222/6080, and host:port values ending in those
// ports are dropped. VNC WS paths are not written here — the UI builds them.
func publicSandboxEndpoints(eps map[string]string) map[string]string {
	out := map[string]string{}
	if eps == nil {
		return out
	}
	for k, v := range eps {
		if _, ok := userVisibleEndpointKeys[k]; !ok {
			continue
		}
		if isSensitiveDirectAddr(v) {
			continue
		}
		out[k] = v
	}
	return out
}

func isSensitiveDirectAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	return strings.HasSuffix(addr, ":9222") || strings.HasSuffix(addr, ":6080")
}

func (s *SandboxService) view(ctx context.Context, row *models.Sandbox) SandboxView {
	return s.viewWithStatus(ctx, row, nil, false)
}

// viewWithStatus builds a SandboxView. When statusMap is non-nil (batch List path),
// ContainerStatus is looked up from the map (missing → "not_found"). When
// dockerFailed is true, every row gets "unknown". Otherwise (GetView path) a
// single docker inspect is used via mgr.Status.
func (s *SandboxService) viewWithStatus(ctx context.Context, row *models.Sandbox, statusMap map[string]string, dockerFailed bool) SandboxView {
	s.mu.Lock()
	ls := s.live[row.ID]
	busy := ls != nil && ls.busy
	if row.Purpose == "run" && s.runActive[row.Name] {
		busy = true
	}
	connected := ls != nil && ls.acp != nil && ls.acp.IsConnected()
	s.mu.Unlock()

	var containerStatus string
	switch {
	case dockerFailed:
		containerStatus = "unknown"
	case statusMap != nil:
		if st, ok := statusMap[row.Name]; ok {
			containerStatus = st
		} else {
			containerStatus = "not_found"
		}
	default:
		containerStatus = s.mgr.Status(ctx, row.Name)
	}

	return SandboxView{
		Sandbox:         *row,
		ContainerStatus: containerStatus,
		Busy:            busy,
		Connected:       connected,
		HasCodeServer:   row.CodeServerPort > 0,
		HasACP:          row.ACPPort > 0,
		Password:        row.Token,
	}
}

// Stop removes the container but keeps the DB record (status=stopped).
func (s *SandboxService) Stop(ctx context.Context, id uint) error {
	row, err := s.Get(id)
	if err != nil {
		return err
	}
	if s.isBusyRow(row) {
		return fmt.Errorf("sandbox 正在执行任务,无法停止")
	}
	s.teardownLive(id)
	s.archiveLog(ctx, row.Name)
	_ = s.mgr.DestroyByName(ctx, row.Name)
	return s.db.Model(&models.Sandbox{}).Where("id = ?", id).
		Updates(map[string]any{"status": "stopped", "destroy_at": nil, "updated_at": time.Now()}).Error
}

// Destroy removes the container and deletes the DB record.
func (s *SandboxService) Destroy(ctx context.Context, id uint) error {
	row, err := s.Get(id)
	if err != nil {
		return err
	}
	if s.isBusyRow(row) {
		return fmt.Errorf("sandbox 正在执行任务,无法销毁")
	}
	s.teardownLive(id)
	s.archiveLog(ctx, row.Name)
	_ = s.mgr.DestroyByName(ctx, row.Name)
	if row.HomeDir != "" {
		_ = os.RemoveAll(row.HomeDir)
	}
	if isAgentSandboxPurpose(row.Purpose) {
		if s.agentOnDestroy != nil {
			s.agentOnDestroy(row.ProjectID, row.ThreadID, row.Token)
		}
	} else if row.Purpose == "test" {
		s.unregisterTestScheduler(row.ProjectID, row.Token)
		if row.RunID != "" {
			s.host.UnregisterRun(row.RunID)
		}
	} else if row.RunID != "" {
		s.host.UnregisterRun(row.RunID)
	}
	return s.db.Delete(&models.Sandbox{}, id).Error
}

// CleanupIdle destroys every non-busy sandbox; returns (destroyed, skipped).
func (s *SandboxService) CleanupIdle(ctx context.Context) (destroyed, skipped int) {
	var rows []models.Sandbox
	if err := s.db.Find(&rows).Error; err != nil {
		return 0, 0
	}
	for i := range rows {
		if s.isBusyRow(&rows[i]) {
			skipped++
			continue
		}
		if err := s.Destroy(ctx, rows[i].ID); err == nil {
			destroyed++
		} else {
			skipped++
		}
	}
	return destroyed, skipped
}

// ShutdownAllTestSandboxes tears down every interactive test sandbox. When
// force is true, busy sandboxes are cancelled and destroyed immediately.
func (s *SandboxService) ShutdownAllTestSandboxes(ctx context.Context, force bool) int {
	var rows []models.Sandbox
	if err := s.db.Where("purpose = ?", "test").Find(&rows).Error; err != nil {
		log.Error().Err(err).Msg("shutdown test sandboxes: query failed")
		return 0
	}
	destroyed := 0
	for i := range rows {
		row := &rows[i]
		if !force && s.isBusyRow(row) {
			continue
		}
		if force {
			s.Cancel(row.ID)
		}
		s.teardownLive(row.ID)
		s.archiveLog(ctx, row.Name)
		_ = s.mgr.DestroyByName(ctx, row.Name)
		if row.HomeDir != "" {
			_ = os.RemoveAll(row.HomeDir)
		}
		s.unregisterTestScheduler(row.ProjectID, row.Token)
		if row.RunID != "" {
			s.host.UnregisterRun(row.RunID)
		}
		if err := s.db.Delete(&models.Sandbox{}, row.ID).Error; err == nil {
			destroyed++
		}
	}
	if destroyed > 0 {
		log.Info().Int("count", destroyed).Msg("interactive sandbox destroyed")
	}
	return destroyed
}

// RunSweeper periodically destroys idle sandboxes past their TTL deadline.
func (s *SandboxService) RunSweeper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepOnce(ctx)
		}
	}
}

func (s *SandboxService) sweepOnce(ctx context.Context) {
	var rows []models.Sandbox
	if err := s.db.Where("destroy_at IS NOT NULL AND destroy_at <= ?", time.Now()).Find(&rows).Error; err != nil {
		return
	}
	for i := range rows {
		if s.isBusyRow(&rows[i]) {
			continue
		}
		if err := s.Destroy(ctx, rows[i].ID); err == nil {
			log.Info().Str("name", rows[i].Name).Msg("test sandbox swept (idle ttl)")
		}
	}
}

// ReconcileOnStartup syncs DB rows with live Docker state after a restart and
// drops orphan containers not tracked in the DB.
func (s *SandboxService) ReconcileOnStartup(ctx context.Context) {
	var rows []models.Sandbox
	if err := s.db.Find(&rows).Error; err != nil {
		return
	}
	tracked := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		tracked[row.Name] = true
		// Per-run node sandboxes are owned by the runtime provider; after a
		// restart their driving goroutine is gone, so any surviving container
		// is an orphan. Destroy it and drop the record.
		if row.Purpose == "run" {
			s.archiveLog(ctx, row.Name)
			_ = s.mgr.DestroyByName(ctx, row.Name)
			if row.HomeDir != "" {
				_ = os.RemoveAll(row.HomeDir)
			}
			if row.RunID != "" {
				s.host.UnregisterRun(row.RunID)
			}
			s.db.Delete(&models.Sandbox{}, row.ID)
			continue
		}
		switch s.mgr.Status(ctx, row.Name) {
		case "running":
			if sb, err := s.mgr.Attach(ctx, row.Name); err == nil {
				upd := map[string]any{"status": "running", "host": sb.Host, "acp_port": sb.Port, "code_server_port": sb.CodeServerPort}
				if row.DestroyAt == nil {
					at := time.Now().Add(s.TTL())
					upd["destroy_at"] = &at
				}
				s.db.Model(&models.Sandbox{}).Where("id = ?", row.ID).Updates(upd)
			}
		default:
			// Container gone: drop the record and free its run token.
			if row.RunID != "" {
				s.host.UnregisterRun(row.RunID)
			}
			s.db.Delete(&models.Sandbox{}, row.ID)
		}
	}
	// Destroy orphan containers (present in docker, absent from DB).
	if names, err := s.mgr.List(ctx); err == nil {
		for _, n := range names {
			if !tracked[n] {
				_ = s.mgr.DestroyByName(ctx, n)
				log.Info().Str("name", n).Msg("destroyed orphan sandbox container")
			}
		}
	}
}

func (s *SandboxService) isBusyRow(row *models.Sandbox) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ls := s.live[row.ID]; ls != nil && ls.busy {
		return true
	}
	return row.Purpose == "run" && s.runActive[row.Name]
}

func (s *SandboxService) teardownLive(id uint) {
	s.mu.Lock()
	ls := s.live[id]
	delete(s.live, id)
	s.mu.Unlock()
	if ls != nil {
		if ls.acp != nil {
			ls.acp.Close()
		}
		if ls.home != "" {
			_ = os.RemoveAll(ls.home)
		}
	}
}

func (s *SandboxService) activeCount() int {
	var n int64
	s.db.Model(&models.Sandbox{}).
		Where("status IN ? AND purpose = ?", []string{"running", "creating"}, "test").
		Count(&n)
	return int(n)
}

// truncErr renders an error to a bounded string for persistence/display.
func truncErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

func (s *SandboxService) mcpVars(runID, token string) map[string]string {
	url := ""
	base := config.ResolveMCPAdvertise(s.mcpEndpoint)
	if base != "" {
		url = base + "/mcp/runs/" + runID
	}
	return map[string]string{
		"APPROVING_ARTIFACT_URL":   url,
		"APPROVING_ARTIFACT_TOKEN": token,
		"APPROVING_RUN_ID":         runID,
		"APPROVING_NODE_ID":        "test",
	}
}

func (s *SandboxService) testMcpVars(runID, token, projectID, profile string) map[string]string {
	vars := s.mcpVars(runID, token)
	projectID = strings.TrimSpace(projectID)
	profile = strings.TrimSpace(profile)
	if projectID == "" || profile == "" {
		return vars
	}
	base := strings.TrimRight(config.ResolveMCPAdvertise(s.mcpEndpoint), "/")
	if base == "" {
		return vars
	}
	vars["APPROVING_SCHEDULER_URL"] = config.RewriteMisconfiguredMCPAdvertise(
		base + "/mcp/task-scheduler/" + url.PathEscape(profile))
	vars["APPROVING_SCHEDULER_TOKEN"] = token
	return vars
}

func (s *SandboxService) buildTestSandboxSpecs(projectID, profile, runID, token string, agent Agent, vars map[string]string) []sandbox.MCPServerSpec {
	specs := filterAgentPlatformMCP(resolveAgentMCP(agent.MCP, vars))
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return specs
	}
	s.registerTestScheduler(projectID, profile, runID, token)
	if sched := BuildTestSchedulerMCPSpec(profile, token); sched.Name != "" {
		specs = dedupeMCPByName(append(specs, sched))
	}
	return specs
}

// --- helpers shared with the runtime's MCP wiring (kept local to avoid a
// cross-package dependency) -------------------------------------------------

func substTemplate(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

func substTemplateMap(m, vars map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = substTemplate(v, vars)
	}
	return out
}

func substTemplateSlice(in []string, vars map[string]string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = substTemplate(v, vars)
	}
	return out
}

// resolveAgentMCP maps an Agent's MCP entries to sandbox specs, substituting
// run-scoped template vars and dropping entries that resolve to empty.
func resolveAgentMCP(in []MCPServer, vars map[string]string) []sandbox.MCPServerSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]sandbox.MCPServerSpec, 0, len(in))
	for _, m := range in {
		if m.Name == "" {
			continue
		}
		url := config.RewriteMisconfiguredMCPAdvertise(substTemplate(m.URL, vars))
		cmd := substTemplate(m.Command, vars)
		if url == "" && cmd == "" {
			continue
		}
		out = append(out, sandbox.MCPServerSpec{
			Name:    m.Name,
			URL:     url,
			Headers: substTemplateMap(m.Headers, vars),
			Command: cmd,
			Args:    substTemplateSlice(m.Args, vars),
			Env:     substTemplateMap(m.Env, vars),
		})
	}
	return out
}

func hasArtifactStoreSpec(specs []sandbox.MCPServerSpec) bool {
	for _, sp := range specs {
		if sp.Name == ArtifactStoreMCP {
			return true
		}
	}
	return false
}

// mcpServersJSON builds the ACP session mcpServers array from resolved specs.
func mcpServersJSON(specs []sandbox.MCPServerSpec) json.RawMessage {
	if len(specs) == 0 {
		return nil
	}
	servers := make([]map[string]any, 0, len(specs))
	for _, m := range specs {
		switch {
		case m.URL != "":
			// type:http required by Claude Code / CodeBuddy; url-only is skipped.
			entry := map[string]any{"name": m.Name, "type": "http", "url": m.URL}
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
