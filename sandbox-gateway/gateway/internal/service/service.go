package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"sandbox-gateway/internal/config"
	"sandbox-gateway/internal/driver"
	"sandbox-gateway/internal/logging"
	"sandbox-gateway/internal/models"
	"sandbox-gateway/internal/store"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// destroyDriverTimeout bounds best-effort workload teardown on DELETE.
const destroyDriverTimeout = 2 * time.Minute

// ErrEndpointNotFound is returned by Host when the sandbox exists but the
// requested port is not exposed. Handlers map it to 404 (not 500).
var ErrEndpointNotFound = errors.New("no endpoint for port")

// ErrLogsUnsupported is returned when the configured driver cannot retrieve
// container logs. Handlers map it to 501. Docker and kubernetes implement Logs;
// the mapping remains for any driver that still returns driver.ErrLogsUnsupported.
var ErrLogsUnsupported = driver.ErrLogsUnsupported

// lbWaiter is optionally implemented by drivers that expose LoadBalancer-backed
// endpoints (kubernetes + MetalLB). The service waits for the IP before probing.
type lbWaiter interface {
	WaitLoadBalancerIP(ctx context.Context, id string, timeout time.Duration) (string, error)
}

// Config tunes the service orchestration.
type Config struct {
	Image string
	// ProviderImages maps an agent provider name to a specific image ref.
	ProviderImages map[string]string
	// ImageTemplate derives a per-provider image by convention ("{provider}").
	ImageTemplate string
	Ports         []int // ports to expose (image config)
	SessionPort   int   // port used for the readiness probe (default 8765)
	WorkspaceDir  string
	// FinalizeTimeout bounds async readiness (LB IP + session probe).
	FinalizeTimeout time.Duration
	// Resources is the default/max policy for per-sandbox CPU/memory/disk.
	Resources config.ResourceDefaults
	// OrphanGCInterval is how often SweepOrphans runs when RunOrphanGC is used.
	OrphanGCInterval time.Duration
	// OrphanGCMinAge skips live workloads younger than this during orphan sweeps.
	OrphanGCMinAge time.Duration
}

// SandboxService orchestrates the single configured driver and the store.
type SandboxService struct {
	drv   driver.Driver
	store *store.Store
	cfg   Config
}

// New builds a SandboxService.
func New(drv driver.Driver, st *store.Store, cfg Config) *SandboxService {
	if cfg.SessionPort == 0 {
		cfg.SessionPort = 8765
	}
	if cfg.FinalizeTimeout == 0 {
		cfg.FinalizeTimeout = 5 * time.Minute
	}
	return &SandboxService{drv: drv, store: st, cfg: cfg}
}

// CreateRequest is the caller-supplied sandbox creation input.
type CreateRequest struct {
	Env          map[string]string
	Labels       map[string]string
	WorkspaceDir string
	Ports        []int // extra ports to expose in addition to image defaults
	Mounts       []string
	Config       *driver.ConfigInject
	Image        string // override image (optional; wins over provider mapping)
	// Provider selects the agent CLI. When set (and Image is empty) the gateway
	// resolves it to a per-agent image, and injects AGENT_PROVIDER/ACP_BACKEND
	// into the sandbox env when the caller did not set them.
	Provider string
	// Resources are optional per-sandbox limits; zeros use gateway defaults.
	CPUCores float64
	MemoryMB int64
	DiskGi   int64
}

// Create persists a "creating" record and returns immediately (HTTP 202).
// Driver provisioning (Namespace/Secret/PVC/Deployment/LB or docker run) and
// readiness finalize run in the background under FinalizeTimeout — same contract
// as Reinstall. This avoids hanging clients when the apiserver/CSI/storage is slow.
func (s *SandboxService) Create(_ context.Context, req CreateRequest) (*models.Sandbox, error) {
	cpu, mem, disk, err := s.cfg.Resources.Resolve(req.CPUCores, req.MemoryMB, req.DiskGi)
	if err != nil {
		return nil, err
	}

	id := uuid.NewString()[:12]
	// Resolve the image: explicit override > per-provider mapping > template >
	// default. See config.ImageConfig.Resolve for the shared ordering.
	image := config.ImageConfig{
		Ref:        s.cfg.Image,
		ByProvider: s.cfg.ProviderImages,
		Template:   s.cfg.ImageTemplate,
	}.Resolve(req.Image, req.Provider)
	workspace := req.WorkspaceDir
	if workspace == "" {
		workspace = s.cfg.WorkspaceDir
	}
	ports := mergePorts(s.cfg.Ports, req.Ports)

	sb := &models.Sandbox{
		ID:       id,
		Name:     id,
		Status:   models.StatusCreating,
		Image:    image,
		CPUCores: cpu,
		MemoryMB: mem,
		DiskGi:   disk,
	}
	// Persist config injection as env (SANDBOX_INJECT) on the record so it is
	// durable: Reinstall/reconcile rebuild the workload from stored env, and a
	// pod recreate must re-seed the same config. Without this, injection would
	// only survive the very first pod (Config lives in-memory on the request).
	env := mergeEnv(req.Env, configInjectEnv(req.Config))
	// When a provider is requested, make the sandbox select it at runtime too:
	// set AGENT_PROVIDER (and the legacy ACP_BACKEND alias) unless the caller
	// already provided them explicitly via env.
	if req.Provider != "" {
		if _, ok := env["AGENT_PROVIDER"]; !ok {
			env["AGENT_PROVIDER"] = req.Provider
		}
		if _, ok := env["ACP_BACKEND"]; !ok {
			env["ACP_BACKEND"] = req.Provider
		}
	}
	sb.SetEnv(env)
	sb.SetLabels(req.Labels)
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer persistCancel()
	log.Info().Str("sandbox_id", id).Msg("persist creating record")
	if err := s.store.Create(persistCtx, sb); err != nil {
		log.Error().Err(err).Str("sandbox_id", id).Msg("persist creating record failed")
		return nil, fmt.Errorf("persist creating record: %w", err)
	}
	log.Info().Str("sandbox_id", id).Msg("persist creating record ok")

	spec := driver.Spec{
		ID:           id,
		Image:        image,
		Env:          env,
		Ports:        ports,
		WorkspaceDir: workspace,
		Mounts:       req.Mounts,
		Config:       req.Config,
		Labels:       req.Labels,
		Resources: driver.Resources{
			CPUCores: cpu,
			MemoryMB: mem,
			DiskGi:   disk,
		},
	}

	// Detach from the request context so ingress/client cancel cannot leave a
	// half-applied Deployment/PVC without a matching store error flip.
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), s.cfg.FinalizeTimeout)
		defer cancel()
		log.Info().
			Str("sandbox_id", id).
			Str("image", image).
			Float64("cpu_cores", cpu).
			Int64("memory_mb", mem).
			Int64("disk_gi", disk).
			Msg("sandbox provision starting")
		h, err := s.drv.Create(bg, spec)
		if err != nil {
			s.fail(id, fmt.Sprintf("create: %v", err))
			return
		}
		cur, getErr := s.store.Get(context.Background(), id)
		if getErr != nil {
			log.Error().Err(getErr).Str("sandbox_id", id).Msg("create: reload after provision failed")
			return
		}
		cur.Name = h.Name
		cur.Namespace = h.Namespace
		if len(h.Endpoints) > 0 {
			cur.SetEndpoints(h.Endpoints)
		}
		s.persist(cur, "persist create provisional status")
		s.finalize(id)
	}()

	return sb, nil
}

// finalize waits for the sandbox to become reachable and backfills endpoints.
func (s *SandboxService) finalize(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.FinalizeTimeout)
	defer cancel()

	// For LB-backed drivers, wait for the external IP first.
	if w, ok := s.drv.(lbWaiter); ok {
		if _, err := w.WaitLoadBalancerIP(ctx, id, s.cfg.FinalizeTimeout); err != nil {
			s.fail(id, fmt.Sprintf("wait loadbalancer ip: %v", err))
			return
		}
	}

	eps, err := s.drv.Endpoints(ctx, id)
	if err != nil {
		s.fail(id, fmt.Sprintf("resolve endpoints: %v", err))
		return
	}
	addr := eps[s.cfg.SessionPort]
	if addr == "" {
		s.fail(id, fmt.Sprintf("no endpoint for session port %d", s.cfg.SessionPort))
		return
	}
	if err := waitTCP(ctx, addr); err != nil {
		s.fail(id, fmt.Sprintf("session port not ready: %v", err))
		return
	}

	sb, err := s.store.Get(context.Background(), id)
	if err != nil {
		log.Error().Err(err).Str("sandbox_id", id).Msg("finalize: load sandbox failed")
		return
	}
	sb.SetEndpoints(eps)
	sb.Status = models.StatusRunning
	sb.Error = ""
	s.persist(sb, "persist running status")
	log.Info().
		Str("sandbox_id", id).
		Interface("endpoints", eps).
		Msg("sandbox ready")
}

func (s *SandboxService) fail(id, msg string) {
	sb, err := s.store.Get(context.Background(), id)
	if err != nil {
		log.Error().Err(err).Str("sandbox_id", id).Str("fail_reason", msg).Msg("fail: load sandbox failed")
		return
	}
	sb.Status = models.StatusError
	sb.Error = msg
	s.persist(sb, "persist error status")
	log.Error().Str("sandbox_id", id).Str("err", msg).Msg("sandbox failed")
}

// persist writes sb and records any store error (logging-spec: never silence failures).
func (s *SandboxService) persist(sb *models.Sandbox, msg string) {
	logging.SaveErr(s.store.Save(context.Background(), sb), msg, map[string]any{"sandbox_id": sb.ID, "status": sb.Status})
}

// Get returns a sandbox record, refreshing endpoints if it is running.
func (s *SandboxService) Get(id string) (*models.Sandbox, error) {
	return s.store.Get(context.Background(), id)
}

// ListFilter is the store filter exposed at the service boundary.
type ListFilter = store.ListFilter

// List returns sandbox records matching filter.
func (s *SandboxService) List(filter ListFilter) ([]models.Sandbox, error) {
	return s.store.List(context.Background(), filter)
}

// Start resumes a stopped sandbox.
func (s *SandboxService) Start(ctx context.Context, id string) error {
	sb, err := s.store.Get(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.drv.Start(ctx, id); err != nil {
		return err
	}
	sb.Status = models.StatusCreating
	s.persist(sb, "persist start status")
	go s.finalize(id)
	return nil
}

// Stop stops a sandbox but retains it.
func (s *SandboxService) Stop(ctx context.Context, id string) error {
	sb, err := s.store.Get(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.drv.Stop(ctx, id); err != nil {
		return err
	}
	sb.Status = models.StatusStopped
	s.persist(sb, "persist stop status")
	return nil
}

// Destroy removes a sandbox and its record.
//
// Driver teardown is best-effort: a missing/failing workload must not block
// deleting the control-plane row (otherwise DELETE leaves stuck DB records when
// docker rm fails, the request context is canceled, or k8s cleanup errors).
// idOrName may be the sandbox id or the driver-native name (e.g. sbx-<id>).
func (s *SandboxService) Destroy(_ context.Context, idOrName string) error {
	sb, err := s.lookup(context.Background(), idOrName)
	if err != nil {
		return err
	}
	bg, cancel := context.WithTimeout(context.Background(), destroyDriverTimeout)
	defer cancel()
	if err := s.drv.Destroy(bg, sb.ID); err != nil {
		log.Warn().Err(err).Str("sandbox_id", sb.ID).Msg("destroy: driver cleanup failed; removing record anyway")
	}
	return s.store.Delete(context.Background(), sb.ID)
}

// lookup resolves a sandbox by id, then by driver-native name.
func (s *SandboxService) lookup(ctx context.Context, idOrName string) (*models.Sandbox, error) {
	sb, err := s.store.Get(ctx, idOrName)
	if err == nil {
		return sb, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return s.store.GetByName(ctx, idOrName)
}

// Reinstall rebuilds the sandbox workload. preserveData keeps the data PVC
// (kubernetes) / anonymous volumes (docker); workspace wipe deletes them.
// Host-mounted shared config (ConfigInject.HostPath / .cursor) is never removed.
func (s *SandboxService) Reinstall(ctx context.Context, id string, preserveData bool) error {
	sb, err := s.store.Get(context.Background(), id)
	if err != nil {
		return err
	}

	cpu, mem, disk, err := s.cfg.Resources.Resolve(sb.CPUCores, sb.MemoryMB, sb.DiskGi)
	if err != nil {
		return err
	}

	ports := s.cfg.Ports
	for p := range sb.Endpoints() {
		ports = mergePorts(ports, []int{p})
	}

	env := sb.Env()
	workspace := s.cfg.WorkspaceDir
	if v := env["WORKSPACE_DIR"]; v != "" {
		workspace = v
	}

	spec := driver.Spec{
		ID:           id,
		Image:        sb.Image,
		Env:          env,
		Ports:        ports,
		WorkspaceDir: workspace,
		Labels:       sb.Labels(),
		Resources: driver.Resources{
			CPUCores: cpu,
			MemoryMB: mem,
			DiskGi:   disk,
		},
	}

	sb.Status = models.StatusCreating
	sb.Error = ""
	s.persist(sb, "persist reinstall status")

	// Reinstall deletes then recreates the workload, which can take far longer
	// than an HTTP request/ingress timeout. Running it under the request context
	// would cancel a half-done delete (leaving the sandbox stuck in error with no
	// deployment), so we detach to a background context and probe readiness
	// asynchronously — same contract as Create (returns 202, poll GET /:id).
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), s.cfg.FinalizeTimeout)
		defer cancel()
		if err := s.drv.Reinstall(bg, spec, preserveData); err != nil {
			s.fail(id, fmt.Sprintf("reinstall: %v", err))
			return
		}
		s.finalize(id)
	}()
	return nil
}

// Status returns the live driver status of a sandbox.
func (s *SandboxService) Status(ctx context.Context, id string) (driver.Status, error) {
	if _, err := s.store.Get(context.Background(), id); err != nil {
		return "", err
	}
	return s.drv.Status(ctx, id)
}

// Logs returns combined PID1 stdout/stderr for a sandbox (non-follow, tailed).
// The sandbox must exist in the control-plane store; drivers that do not
// support logs return ErrLogsUnsupported.
func (s *SandboxService) Logs(ctx context.Context, id string, tail int) (string, error) {
	sb, err := s.lookup(ctx, id)
	if err != nil {
		return "", err
	}
	if tail <= 0 {
		tail = 5000
	}
	out, err := s.drv.Logs(ctx, sb.ID, tail)
	if err != nil {
		if errors.Is(err, driver.ErrLogsUnsupported) {
			return "", ErrLogsUnsupported
		}
		return "", err
	}
	return out, nil
}

// Host returns the client-reachable address for a single port.
func (s *SandboxService) Host(ctx context.Context, id string, port int) (string, error) {
	sb, err := s.store.Get(context.Background(), id)
	if err != nil {
		return "", err
	}
	if addr := sb.Endpoints()[port]; addr != "" {
		return addr, nil
	}
	eps, err := s.drv.Endpoints(ctx, id)
	if err != nil {
		return "", err
	}
	addr := eps[port]
	if addr == "" {
		return "", fmt.Errorf("%w %d", ErrEndpointNotFound, port)
	}
	return addr, nil
}

// SweepOrphans destroys driver workloads that have no matching store record.
// Workloads younger than OrphanGCMinAge (when set and CreatedAt is known) are
// skipped to avoid racing create/read-replica lag.
func (s *SandboxService) SweepOrphans(ctx context.Context) (int, error) {
	live, err := s.drv.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list driver workloads: %w", err)
	}
	records, err := s.store.List(context.Background(), ListFilter{})
	if err != nil {
		return 0, fmt.Errorf("list store records: %w", err)
	}
	known := make(map[string]struct{}, len(records))
	for i := range records {
		known[records[i].ID] = struct{}{}
	}

	removed := 0
	for _, h := range live {
		if h == nil || h.ID == "" {
			continue
		}
		if _, ok := known[h.ID]; ok {
			continue
		}
		if s.cfg.OrphanGCMinAge > 0 && !h.CreatedAt.IsZero() && time.Since(h.CreatedAt) < s.cfg.OrphanGCMinAge {
			log.Debug().
				Str("sandbox_id", h.ID).
				Str("name", h.Name).
				Dur("age", time.Since(h.CreatedAt)).
				Msg("orphan gc: skip young workload")
			continue
		}
		log.Info().
			Str("sandbox_id", h.ID).
			Str("name", h.Name).
			Str("driver", s.drv.Name()).
			Msg("orphan gc: destroying workload with no store record")
		bg, cancel := context.WithTimeout(ctx, destroyDriverTimeout)
		err := s.drv.Destroy(bg, h.ID)
		cancel()
		if err != nil {
			log.Warn().Err(err).Str("sandbox_id", h.ID).Msg("orphan gc: destroy failed")
			continue
		}
		removed++
	}
	return removed, nil
}

// RunOrphanGC periodically calls SweepOrphans until ctx is cancelled.
func (s *SandboxService) RunOrphanGC(ctx context.Context) {
	interval := s.cfg.OrphanGCInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	log.Info().
		Dur("interval", interval).
		Dur("min_age", s.cfg.OrphanGCMinAge).
		Msg("orphan gc started")

	run := func() {
		c, cancel := context.WithTimeout(ctx, destroyDriverTimeout+30*time.Second)
		defer cancel()
		n, err := s.SweepOrphans(c)
		if err != nil {
			log.Error().Err(err).Msg("orphan gc sweep failed")
			return
		}
		log.Info().Int("removed", n).Msg("orphan gc sweep complete")
	}

	run()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("orphan gc stopped")
			return
		case <-t.C:
			run()
		}
	}
}

// ReconcileOnStartup reconciles persisted records against live driver state
// after a gateway restart: it refreshes statuses/endpoints and marks records
// whose resources vanished as error.
func (s *SandboxService) ReconcileOnStartup(ctx context.Context) {
	records, err := s.store.List(context.Background(), ListFilter{})
	if err != nil {
		log.Error().Err(err).Msg("reconcile: list records failed")
		return
	}
	for i := range records {
		sb := &records[i]
		st, err := s.drv.Status(ctx, sb.ID)
		if err != nil {
			log.Warn().Err(err).Str("sandbox_id", sb.ID).Msg("reconcile: driver status failed")
			continue
		}
		switch st {
		case driver.StatusNotFound:
			if sb.Status != models.StatusError {
				sb.Status = models.StatusError
				sb.Error = "resource not found on reconcile"
				s.persist(sb, "reconcile: persist not-found status")
			}
		case driver.StatusRunning:
			eps, epErr := s.drv.Endpoints(ctx, sb.ID)
			if epErr != nil {
				logging.WarnErr(epErr, "reconcile: endpoints failed", map[string]any{"sandbox_id": sb.ID})
			} else if len(eps) > 0 {
				sb.SetEndpoints(eps)
			}
			sb.Status = models.StatusRunning
			s.persist(sb, "reconcile: persist running status")
		case driver.StatusStopped:
			sb.Status = models.StatusStopped
			s.persist(sb, "reconcile: persist stopped status")
		}
	}
	log.Info().Int("count", len(records)).Msg("reconcile complete")
}

// configInjectEnv translates a URL config bundle into the SANDBOX_INJECT env
// contract understood by the image's startup.sh. HostPath (docker same-host
// bind-mount) cannot be expressed as env and is handled by the driver via
// Spec.Config at create time; only the URL path is persisted here so it survives
// reinstall/reconcile.
func configInjectEnv(cfg *driver.ConfigInject) map[string]string {
	if cfg == nil || cfg.BundleURL == "" {
		return nil
	}
	inject := cfg.BundleURL
	if cfg.ConfigRoot != "" {
		inject += "|" + cfg.ConfigRoot
	}
	out := map[string]string{"SANDBOX_INJECT": inject}
	if cfg.Headers != "" {
		out["SANDBOX_INJECT_HEADERS"] = cfg.Headers
	}
	return out
}

// mergeEnv overlays extra onto a copy of base (extra wins). Nil-safe.
func mergeEnv(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func mergePorts(base, extra []int) []int {
	seen := map[int]struct{}{}
	var out []int
	for _, p := range append(append([]int{}, base...), extra...) {
		if p <= 0 {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func waitTCP(ctx context.Context, addr string) error {
	dialer := net.Dialer{Timeout: 3 * time.Second}
	for {
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			logging.WarnErr(conn.Close(), "waitTCP: close probe conn", map[string]any{"addr": addr})
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
