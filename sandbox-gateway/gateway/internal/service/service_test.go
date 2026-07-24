package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"sandbox-gateway/internal/config"
	"sandbox-gateway/internal/database"
	"sandbox-gateway/internal/driver"
	"sandbox-gateway/internal/driver/fake"
	"sandbox-gateway/internal/store"
)

func TestConfigInjectEnv(t *testing.T) {
	if configInjectEnv(nil) != nil {
		t.Fatal("nil config must yield nil env")
	}
	if configInjectEnv(&driver.ConfigInject{ConfigRoot: "/root/.cursor"}) != nil {
		t.Fatal("no BundleURL must yield nil env")
	}

	got := configInjectEnv(&driver.ConfigInject{
		BundleURL:  "https://x/b.tgz",
		ConfigRoot: "/root/.cursor",
		Headers:    "Authorization: Bearer t",
	})
	if got["SANDBOX_INJECT"] != "https://x/b.tgz|/root/.cursor" {
		t.Fatalf("SANDBOX_INJECT=%q", got["SANDBOX_INJECT"])
	}
	if got["SANDBOX_INJECT_HEADERS"] != "Authorization: Bearer t" {
		t.Fatalf("headers=%q", got["SANDBOX_INJECT_HEADERS"])
	}

	noRoot := configInjectEnv(&driver.ConfigInject{BundleURL: "https://x/b.tgz"})
	if noRoot["SANDBOX_INJECT"] != "https://x/b.tgz" {
		t.Fatalf("no-root SANDBOX_INJECT=%q", noRoot["SANDBOX_INJECT"])
	}
}

func TestMergeEnv(t *testing.T) {
	base := map[string]string{"A": "1", "B": "2"}
	out := mergeEnv(base, map[string]string{"B": "override", "C": "3"})
	if out["A"] != "1" || out["B"] != "override" || out["C"] != "3" {
		t.Fatalf("merged wrong: %v", out)
	}
	if base["B"] != "2" {
		t.Fatal("base mutated")
	}
	if got := mergeEnv(nil, nil); len(got) != 0 {
		t.Fatalf("nil,nil should be empty, got %v", got)
	}
}

func testResources() config.ResourceDefaults {
	return config.ResourceDefaults{
		DefaultCPUCores: 2,
		DefaultMemoryMB: 4096,
		DefaultDiskGi:   25,
		MaxCPUCores:     8,
		MaxMemoryMB:     16384,
		MaxDiskGi:       500,
	}
}

func testDB(t *testing.T) *store.Store {
	t.Helper()
	// Unique in-memory DB per test (avoids slow filesystem SQLite in CI/sandbox).
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := database.Open(config.DBConfig{Driver: "sqlite", Path: dsn})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return store.New(db)
}

func testService(t *testing.T) (*SandboxService, *fake.Driver, *store.Store) {
	t.Helper()
	st := testDB(t)
	drv := fake.New()
	if err := drv.WithSessionListener(8765); err != nil {
		t.Fatalf("session listener: %v", err)
	}
	t.Cleanup(drv.Close)
	svc := New(drv, st, Config{
		Image:           "test-image:local",
		Ports:           []int{8765, 22},
		SessionPort:     8765,
		WorkspaceDir:    "/root/workspace",
		FinalizeTimeout: 2 * time.Second,
		Resources:       testResources(),
	})
	return svc, drv, st
}

// testServiceNoListener builds a service whose finalize waitTCP fails quickly
// (endpoints point at closed ports / missing session port).
func testServiceNoListener(t *testing.T, ports []int, sessionPort int) (*SandboxService, *fake.Driver, *store.Store) {
	t.Helper()
	st := testDB(t)
	drv := fake.New()
	svc := New(drv, st, Config{
		Image:           "test-image:local",
		Ports:           ports,
		SessionPort:     sessionPort,
		WorkspaceDir:    "/root/workspace",
		FinalizeTimeout: 500 * time.Millisecond,
		Resources:       testResources(),
	})
	return svc, drv, st
}

type lbFake struct {
	*fake.Driver
	waitErr error
	waitIP  string
}

func (l *lbFake) WaitLoadBalancerIP(ctx context.Context, id string, timeout time.Duration) (string, error) {
	if l.waitErr != nil {
		return "", l.waitErr
	}
	return l.waitIP, nil
}

type failCreateDriver struct {
	*fake.Driver
	createErr error
}

func (f *failCreateDriver) Create(ctx context.Context, spec driver.Spec) (*driver.Handle, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.Driver.Create(ctx, spec)
}

type failOpsDriver struct {
	*fake.Driver
	startErr   error
	stopErr    error
	destroyErr error
	reinstall  error
	endpoints  error
}

func (f *failOpsDriver) Start(ctx context.Context, id string) error {
	if f.startErr != nil {
		return f.startErr
	}
	return f.Driver.Start(ctx, id)
}
func (f *failOpsDriver) Stop(ctx context.Context, id string) error {
	if f.stopErr != nil {
		return f.stopErr
	}
	return f.Driver.Stop(ctx, id)
}
func (f *failOpsDriver) Destroy(ctx context.Context, id string) error {
	if f.destroyErr != nil {
		return f.destroyErr
	}
	return f.Driver.Destroy(ctx, id)
}
func (f *failOpsDriver) Reinstall(ctx context.Context, spec driver.Spec, preserveData bool) error {
	if f.reinstall != nil {
		return f.reinstall
	}
	return f.Driver.Reinstall(ctx, spec, preserveData)
}
func (f *failOpsDriver) Endpoints(ctx context.Context, id string) (map[int]string, error) {
	if f.endpoints != nil {
		return nil, f.endpoints
	}
	return f.Driver.Endpoints(ctx, id)
}

func TestCreatePersistsSANDBOX_INJECT(t *testing.T) {
	svc, drv, st := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{
		Env: map[string]string{"FOO": "bar"},
		Config: &driver.ConfigInject{
			BundleURL:  "https://example.com/b.tgz",
			ConfigRoot: "/root/.cursor",
			Headers:    "Authorization: Bearer t",
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := st.Get(context.Background(), sb.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	env := got.Env()
	if env["SANDBOX_INJECT"] != "https://example.com/b.tgz|/root/.cursor" {
		t.Fatalf("stored SANDBOX_INJECT=%q", env["SANDBOX_INJECT"])
	}
	if env["SANDBOX_INJECT_HEADERS"] != "Authorization: Bearer t" {
		t.Fatalf("stored headers=%q", env["SANDBOX_INJECT_HEADERS"])
	}
	if env["FOO"] != "bar" {
		t.Fatalf("user env lost: %v", env)
	}
	deadline := time.Now().Add(2 * time.Second)
	for drv.LastCreate == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if drv.LastCreate == nil || drv.LastCreate.Env["SANDBOX_INJECT"] == "" {
		t.Fatalf("driver Create never saw SANDBOX_INJECT: %+v", drv.LastCreate)
	}
}

func TestHostEndpointNotFound(t *testing.T) {
	svc, _, _ := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Wait briefly for finalize to populate session endpoint.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := svc.Get(sb.ID)
		if cur != nil && cur.Status == "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_, err = svc.Host(context.Background(), sb.ID, 8765)
	if err != nil {
		t.Fatalf("exposed port: %v", err)
	}
	_, err = svc.Host(context.Background(), sb.ID, 12345)
	if !errors.Is(err, ErrEndpointNotFound) {
		t.Fatalf("want ErrEndpointNotFound, got %v", err)
	}
}

func TestReinstallPassesPersistedInjectEnv(t *testing.T) {
	svc, drv, _ := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{
		Config: &driver.ConfigInject{
			BundleURL:  "http://bundle/seed.tgz",
			ConfigRoot: "/root/.cursor",
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := svc.Get(sb.ID)
		if cur != nil && cur.Status == "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := svc.Reinstall(context.Background(), sb.ID, true); err != nil {
		t.Fatalf("Reinstall: %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if drv.LastReinstall != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if drv.LastReinstall == nil {
		t.Fatal("driver Reinstall never called")
	}
	if got := drv.LastReinstall.Env["SANDBOX_INJECT"]; got != "http://bundle/seed.tgz|/root/.cursor" {
		t.Fatalf("reinstall Spec.Env SANDBOX_INJECT=%q", got)
	}
	if drv.ReinstallPreserve == nil || !*drv.ReinstallPreserve {
		t.Fatalf("preserveData not forwarded: %v", drv.ReinstallPreserve)
	}
}

func TestCreateResourceOverLimit(t *testing.T) {
	svc, _, _ := testService(t)
	_, err := svc.Create(context.Background(), CreateRequest{CPUCores: 99})
	if err == nil {
		t.Fatal("expected resource error")
	}
}

func waitSvcRunning(t *testing.T, svc *SandboxService, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := svc.Get(id)
		if cur != nil && cur.Status == "running" {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("%s not running", id)
}

func TestStopDestroyList(t *testing.T) {
	svc, drv, st := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{Labels: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)

	list, err := svc.List(ListFilter{})
	if err != nil || len(list) < 1 {
		t.Fatalf("List: %v len=%d", err, len(list))
	}
	filtered, err := svc.List(ListFilter{Labels: map[string]string{"k": "v"}})
	if err != nil || len(filtered) != 1 || filtered[0].ID != sb.ID {
		t.Fatalf("List by label: %v %+v", err, filtered)
	}
	miss, err := svc.List(ListFilter{Labels: map[string]string{"k": "other"}})
	if err != nil || len(miss) != 0 {
		t.Fatalf("List miss: %v len=%d", err, len(miss))
	}

	if err := svc.Stop(context.Background(), sb.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Get(context.Background(), sb.ID)
	if got.Status != "stopped" {
		t.Fatalf("status=%s", got.Status)
	}
	stLive, err := drv.Status(context.Background(), sb.ID)
	if err != nil || stLive != driver.StatusStopped {
		t.Fatalf("driver status=%s err=%v", stLive, err)
	}

	if err := svc.Destroy(context.Background(), sb.ID); err != nil {
		t.Fatal(err)
	}
	_, err = st.Get(context.Background(), sb.ID)
	if err == nil {
		t.Fatal("record should be gone")
	}
	if err := svc.Destroy(context.Background(), "missing"); err == nil {
		t.Fatal("destroy missing should error")
	}
}

func TestDestroyByName(t *testing.T) {
	svc, _, st := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)
	got, err := st.Get(context.Background(), sb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name == "" || got.Name == sb.ID {
		// Name is updated to driver-native resource name after provision.
		t.Fatalf("expected driver name, got %q", got.Name)
	}
	if err := svc.Destroy(context.Background(), got.Name); err != nil {
		t.Fatalf("destroy by name: %v", err)
	}
	if _, err := st.Get(context.Background(), sb.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("record should be gone, got %v", err)
	}
}

func TestSweepOrphans(t *testing.T) {
	svc, drv, _ := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)

	// Orphan: present in driver, absent from store. Old enough to pass min-age.
	drv.SeedOrphan("orphan-old", time.Now().Add(-time.Hour))
	// Young orphan should be skipped when min-age is set.
	svc.cfg.OrphanGCMinAge = 10 * time.Minute
	drv.SeedOrphan("orphan-young", time.Now().Add(-time.Minute))

	n, err := svc.SweepOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("removed=%d want 1", n)
	}
	live, err := drv.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, h := range live {
		ids[h.ID] = true
	}
	if !ids[sb.ID] {
		t.Fatal("tracked sandbox must remain")
	}
	if ids["orphan-old"] {
		t.Fatal("old orphan should be destroyed")
	}
	if !ids["orphan-young"] {
		t.Fatal("young orphan should be kept")
	}

	svc.cfg.OrphanGCMinAge = 0
	n, err = svc.SweepOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("second sweep removed=%d want 1", n)
	}
}

func TestStartNotFound(t *testing.T) {
	svc, _, _ := testService(t)
	if err := svc.Start(context.Background(), "nope"); err == nil {
		t.Fatal("expected error")
	}
	if err := svc.Stop(context.Background(), "nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestStartResume(t *testing.T) {
	svc, _, st := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)
	if err := svc.Stop(context.Background(), sb.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), sb.ID); err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)
	got, _ := st.Get(context.Background(), sb.ID)
	if got.Status != "running" {
		t.Fatalf("%s", got.Status)
	}
}

func TestReconcileOnStartup(t *testing.T) {
	svc, drv, st := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)

	// Mark as stopped in driver; reconcile should sync.
	if err := drv.Stop(context.Background(), sb.ID); err != nil {
		t.Fatal(err)
	}
	svc.ReconcileOnStartup(context.Background())
	got, err := st.Get(context.Background(), sb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" {
		t.Fatalf("reconcile stopped: %s", got.Status)
	}

	// Restart in driver then reconcile → running with endpoints
	if err := drv.Start(context.Background(), sb.ID); err != nil {
		t.Fatal(err)
	}
	svc.ReconcileOnStartup(context.Background())
	got, _ = st.Get(context.Background(), sb.ID)
	if got.Status != "running" {
		t.Fatalf("reconcile running: %s", got.Status)
	}
	if len(got.Endpoints()) == 0 {
		t.Fatal("endpoints not backfilled")
	}

	// Destroy driver resource → not found → error status
	if err := drv.Destroy(context.Background(), sb.ID); err != nil {
		t.Fatal(err)
	}
	svc.ReconcileOnStartup(context.Background())
	got, _ = st.Get(context.Background(), sb.ID)
	if got.Status != "error" {
		t.Fatalf("reconcile missing: %s err=%s", got.Status, got.Error)
	}

	// Already error + not found stays error
	svc.ReconcileOnStartup(context.Background())
	got2, _ := st.Get(context.Background(), sb.ID)
	if got2.Status != "error" {
		t.Fatalf("%s", got2.Status)
	}
}

func TestStatusPassthrough(t *testing.T) {
	svc, _, _ := testService(t)
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)
	live, err := svc.Status(context.Background(), sb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if live != driver.StatusRunning {
		t.Fatalf("%s", live)
	}
	_, err = svc.Status(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewDefaultSessionPort(t *testing.T) {
	svc, drv, st := testService(t)
	_ = svc
	s2 := New(drv, st, Config{Image: "i", Ports: []int{1}})
	if s2.cfg.SessionPort != 8765 {
		t.Fatalf("default session port=%d", s2.cfg.SessionPort)
	}
	if s2.cfg.FinalizeTimeout != 5*time.Minute {
		t.Fatalf("default finalize=%v", s2.cfg.FinalizeTimeout)
	}
}

func waitSvcError(t *testing.T, svc *SandboxService, id string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := svc.Get(id)
		if cur != nil && cur.Status == "error" {
			return cur.Error
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("%s not error", id)
	return ""
}

func TestFinalizeSessionPortNotReady(t *testing.T) {
	// No listener: endpoints land on closed ports → waitTCP fails under short timeout.
	svc, _, _ := testServiceNoListener(t, []int{8765, 22}, 8765)
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	msg := waitSvcError(t, svc, sb.ID)
	if msg == "" || !strings.Contains(msg, "session port not ready") {
		t.Fatalf("error=%q", msg)
	}
}

func TestFinalizeNoSessionEndpoint(t *testing.T) {
	// Session port not in published ports and fake has no sessionPort override.
	svc, _, _ := testServiceNoListener(t, []int{22}, 8765)
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	msg := waitSvcError(t, svc, sb.ID)
	if !strings.Contains(msg, "no endpoint for session port") {
		t.Fatalf("error=%q", msg)
	}
}

func TestFinalizeLBWaitFailure(t *testing.T) {
	st := testDB(t)
	base := fake.New()
	drv := &lbFake{Driver: base, waitErr: errors.New("lb pending")}
	svc := New(drv, st, Config{
		Image: "i", Ports: []int{8765}, SessionPort: 8765,
		FinalizeTimeout: 500 * time.Millisecond, Resources: testResources(),
	})
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	msg := waitSvcError(t, svc, sb.ID)
	if !strings.Contains(msg, "wait loadbalancer ip") {
		t.Fatalf("error=%q", msg)
	}
}

func TestFinalizeEndpointsError(t *testing.T) {
	st := testDB(t)
	base := fake.New()
	drv := &failOpsDriver{Driver: base, endpoints: errors.New("eps down")}
	svc := New(drv, st, Config{
		Image: "i", Ports: []int{8765}, SessionPort: 8765,
		FinalizeTimeout: 500 * time.Millisecond, Resources: testResources(),
	})
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	msg := waitSvcError(t, svc, sb.ID)
	if !strings.Contains(msg, "resolve endpoints") {
		t.Fatalf("error=%q", msg)
	}
}

func TestCreateDriverErrorMarksRecord(t *testing.T) {
	st := testDB(t)
	base := fake.New()
	drv := &failCreateDriver{Driver: base, createErr: errors.New("provision failed")}
	svc := New(drv, st, Config{
		Image: "i", Ports: []int{8765}, SessionPort: 8765,
		FinalizeTimeout: 500 * time.Millisecond, Resources: testResources(),
	})
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatalf("Create should accept immediately: %v", err)
	}
	if sb.Status != "creating" {
		t.Fatalf("status=%q", sb.Status)
	}
	msg := waitSvcError(t, svc, sb.ID)
	if !strings.Contains(msg, "provision failed") {
		t.Fatalf("error=%q", msg)
	}
}

func TestFailMissingSandbox(t *testing.T) {
	svc, _, _ := testService(t)
	svc.fail("does-not-exist", "ignored")
}

func TestDriverOpErrors(t *testing.T) {
	st := testDB(t)
	base := fake.New()
	if err := base.WithSessionListener(8765); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Close)
	drv := &failOpsDriver{Driver: base}
	svc := New(drv, st, Config{
		Image: "i", Ports: []int{8765}, SessionPort: 8765,
		FinalizeTimeout: 2 * time.Second, Resources: testResources(),
	})
	sb, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)

	drv.startErr = errors.New("start boom")
	if err := svc.Start(context.Background(), sb.ID); err == nil {
		t.Fatal("start")
	}
	drv.startErr = nil
	drv.stopErr = errors.New("stop boom")
	if err := svc.Stop(context.Background(), sb.ID); err == nil {
		t.Fatal("stop")
	}
	drv.stopErr = nil
	drv.destroyErr = errors.New("destroy boom")
	// Driver failure must not block deleting the control-plane record.
	if err := svc.Destroy(context.Background(), sb.ID); err != nil {
		t.Fatalf("destroy with driver error: %v", err)
	}
	if _, err := st.Get(context.Background(), sb.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("record should be gone after destroy driver failure, got %v", err)
	}
	drv.destroyErr = nil

	sb2, err := svc.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb2.ID)

	drv.reinstall = errors.New("reinstall boom")
	if err := svc.Reinstall(context.Background(), sb2.ID, false); err != nil {
		t.Fatalf("Reinstall returns sync nil, got %v", err)
	}
	msg := waitSvcError(t, svc, sb2.ID)
	if !strings.Contains(msg, "reinstall") {
		t.Fatalf("error=%q", msg)
	}
}

func TestReinstallNotFoundAndWorkspaceFromEnv(t *testing.T) {
	svc, drv, st := testService(t)
	if err := svc.Reinstall(context.Background(), "missing", true); err == nil {
		t.Fatal("expected not found")
	}
	sb, err := svc.Create(context.Background(), CreateRequest{
		Env: map[string]string{"WORKSPACE_DIR": "/from/env"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc, sb.ID)
	if err := svc.Reinstall(context.Background(), sb.ID, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if drv.LastReinstall != nil && drv.LastReinstall.WorkspaceDir == "/from/env" {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	if drv.LastReinstall == nil {
		t.Fatal("reinstall not called")
	}
	if drv.LastReinstall.WorkspaceDir != "/from/env" {
		t.Fatalf("workspace=%q", drv.LastReinstall.WorkspaceDir)
	}
	_ = st
}

func TestHostMissingSandboxAndDriverEndpointsError(t *testing.T) {
	svc, _, _ := testService(t)
	if _, err := svc.Host(context.Background(), "missing", 80); err == nil {
		t.Fatal("expected error")
	}

	st := testDB(t)
	base := fake.New()
	if err := base.WithSessionListener(8765); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Close)
	drv := &failOpsDriver{Driver: base}
	svc2 := New(drv, st, Config{
		Image: "i", Ports: []int{8765}, SessionPort: 8765,
		FinalizeTimeout: 2 * time.Second, Resources: testResources(),
	})
	sb, err := svc2.Create(context.Background(), CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSvcRunning(t, svc2, sb.ID)
	// Clear stored endpoints so Host must ask the driver.
	got, _ := st.Get(context.Background(), sb.ID)
	got.SetEndpoints(nil)
	_ = st.Save(context.Background(), got)
	drv.endpoints = errors.New("eps fail")
	if _, err := svc2.Host(context.Background(), sb.ID, 8765); err == nil {
		t.Fatal("expected endpoints error")
	}
}

func TestMergePortsSkipsNonPositive(t *testing.T) {
	got := mergePorts([]int{80, 0, -1, 80}, []int{443, 0})
	if len(got) != 2 || got[0] != 80 || got[1] != 443 {
		t.Fatalf("%v", got)
	}
}
