package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"sandbox-gateway/internal/driver"
)

// mockRunner records docker CLI invocations and returns canned outputs by
// matching the first argument (subcommand) or a custom matcher.
type mockRunner struct {
	mu       sync.Mutex
	calls    [][]string
	byCmd    map[string]mockResp
	byPrefix map[string]mockResp // first-arg+second-arg key e.g. "inspect|--format"
	fallback mockResp
}

type mockResp struct {
	out string
	err error
}

func newMock() *mockRunner {
	return &mockRunner{byCmd: map[string]mockResp{}, byPrefix: map[string]mockResp{}}
}

func (m *mockRunner) on(cmd string, out string, err error) {
	m.byCmd[cmd] = mockResp{out: out, err: err}
}

func (m *mockRunner) run(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	_ = ctx
	_ = timeout
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]string(nil), args...)
	m.calls = append(m.calls, cp)
	if len(args) == 0 {
		return m.fallback.out, m.fallback.err
	}
	if r, ok := m.byCmd[args[0]]; ok {
		return r.out, r.err
	}
	return m.fallback.out, m.fallback.err
}

func (m *mockRunner) lastCall() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	return m.calls[len(m.calls)-1]
}

func (m *mockRunner) callsWith(first string) [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out [][]string
	for _, c := range m.calls {
		if len(c) > 0 && c[0] == first {
			out = append(out, c)
		}
	}
	return out
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func TestNewDefaults(t *testing.T) {
	d := New(Options{})
	if d.bindIP != "127.0.0.1" {
		t.Fatalf("bindIP=%q", d.bindIP)
	}
	if d.namePrefix != "sbx-" {
		t.Fatalf("namePrefix=%q", d.namePrefix)
	}
	if d.shmSize != "1g" {
		t.Fatalf("shmSize=%q", d.shmSize)
	}
	if d.run == nil {
		t.Fatal("run should be set")
	}
	if d.Name() != "docker" {
		t.Fatalf("Name=%q", d.Name())
	}
}

func TestNewCustomOptions(t *testing.T) {
	d := New(Options{BindIP: "10.0.0.1", Network: "net1", NamePrefix: "x-", ShmSize: "2g"})
	if d.bindIP != "10.0.0.1" || d.network != "net1" || d.namePrefix != "x-" || d.shmSize != "2g" {
		t.Fatalf("opts not applied: %+v", d)
	}
}

func TestCreateArgsHostPathBundleEnvMountsResourcesNetwork(t *testing.T) {
	m := newMock()
	// hostPort for each published port
	m.on("run", "cid", nil)
	m.on("inspect", "34567", nil) // hostPort responses for Create endpoints
	d := New(Options{BindIP: "192.168.1.1", Network: "sbx-net", NamePrefix: "sbx-", ShmSize: "512m"})
	d.run = m.run

	spec := driver.Spec{
		ID:           "abc123",
		Image:        "img:tag",
		Ports:        []int{8765, 22},
		WorkspaceDir: "/ws",
		Env:          map[string]string{"FOO": "bar"},
		Mounts:       []string{"/host/data:/data:ro"},
		Resources:    driver.Resources{CPUCores: 1.5, MemoryMB: 2048},
		Config: &driver.ConfigInject{
			HostPath:   "/host/cursor",
			ConfigRoot: "/root/.cursor",
		},
	}
	h, err := d.Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if h.ID != "abc123" || h.Name != "sbx-abc123" || h.Status != driver.StatusRunning {
		t.Fatalf("handle: %+v", h)
	}
	if h.Endpoints[8765] != "192.168.1.1:34567" {
		t.Fatalf("endpoints: %v", h.Endpoints)
	}

	runs := m.callsWith("run")
	if len(runs) != 1 {
		t.Fatalf("want 1 run call, got %d", len(runs))
	}
	args := runs[0]
	if !containsArg(args, "--privileged") {
		t.Fatalf("missing --privileged (required for DinD): %v", args)
	}
	if !containsArg(args, "--cpus") || !containsArg(args, "1.50") {
		t.Fatalf("missing cpus: %v", args)
	}
	if !containsPair(args, "--memory", "2048m") {
		t.Fatalf("missing memory: %v", args)
	}
	if !containsPair(args, "-e", "WORKSPACE_DIR=/ws") {
		t.Fatalf("missing WORKSPACE_DIR: %v", args)
	}
	if !containsPair(args, "-p", "192.168.1.1::8765") || !containsPair(args, "-p", "192.168.1.1::22") {
		t.Fatalf("missing publish: %v", args)
	}
	if !containsPair(args, "--network", "sbx-net") {
		t.Fatalf("missing network: %v", args)
	}
	if !containsPair(args, "-v", "/host/cursor:/root/.cursor:rw") {
		t.Fatalf("missing hostPath mount: %v", args)
	}
	if !containsPair(args, "-e", "FOO=bar") {
		t.Fatalf("missing env: %v", args)
	}
	if !containsPair(args, "-v", "/host/data:/data:ro") {
		t.Fatalf("missing mounts: %v", args)
	}
	if args[len(args)-1] != "img:tag" {
		t.Fatalf("image last: %v", args)
	}
	if !containsArg(args, "--shm-size=512m") {
		t.Fatalf("shm: %v", args)
	}
}

func TestCreateBundleURLWithHeadersAndDefaultConfigRoot(t *testing.T) {
	m := newMock()
	m.on("run", "cid", nil)
	m.on("inspect", "40000", nil)
	d := New(Options{})
	d.run = m.run

	_, err := d.Create(context.Background(), driver.Spec{
		ID:    "b1",
		Image: "img",
		Ports: []int{80},
		Config: &driver.ConfigInject{
			BundleURL: "https://ex/b.tgz",
			Headers:   "Authorization: Bearer t",
			// empty ConfigRoot → no pipe in SANDBOX_INJECT
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	args := m.callsWith("run")[0]
	if !containsPair(args, "-e", "SANDBOX_INJECT=https://ex/b.tgz") {
		t.Fatalf("inject: %v", args)
	}
	if !containsPair(args, "-e", "SANDBOX_INJECT_HEADERS=Authorization: Bearer t") {
		t.Fatalf("headers: %v", args)
	}

	// HostPath with empty ConfigRoot uses /root/.cursor
	m2 := newMock()
	m2.on("run", "cid", nil)
	m2.on("inspect", "40001", nil)
	d2 := New(Options{})
	d2.run = m2.run
	_, err = d2.Create(context.Background(), driver.Spec{
		ID: "b2", Image: "img", Ports: []int{80},
		Config: &driver.ConfigInject{HostPath: "/hp"},
	})
	if err != nil {
		t.Fatalf("Create hostpath: %v", err)
	}
	args2 := m2.callsWith("run")[0]
	if !containsPair(args2, "-v", "/hp:/root/.cursor:rw") {
		t.Fatalf("default root mount: %v", args2)
	}

	// BundleURL with ConfigRoot
	m3 := newMock()
	m3.on("run", "cid", nil)
	m3.on("inspect", "40002", nil)
	d3 := New(Options{})
	d3.run = m3.run
	_, err = d3.Create(context.Background(), driver.Spec{
		ID: "b3", Image: "img", Ports: []int{80},
		Config: &driver.ConfigInject{BundleURL: "https://ex/b.tgz", ConfigRoot: "/root/.cursor"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	args3 := m3.callsWith("run")[0]
	if !containsPair(args3, "-e", "SANDBOX_INJECT=https://ex/b.tgz|/root/.cursor") {
		t.Fatalf("inject with root: %v", args3)
	}
}

func TestCreateRunError(t *testing.T) {
	m := newMock()
	m.on("run", "", errors.New("boom"))
	d := New(Options{})
	d.run = m.run
	_, err := d.Create(context.Background(), driver.Spec{ID: "x", Image: "i", Ports: []int{1}})
	if err == nil || !strings.Contains(err.Error(), "docker run") {
		t.Fatalf("want docker run error, got %v", err)
	}
}

func TestCreateEndpointFailureDestroys(t *testing.T) {
	m := newMock()
	m.on("run", "cid", nil)
	m.on("inspect", "", errors.New("no port"))
	m.on("rm", "", nil)
	d := New(Options{})
	d.run = m.run
	_, err := d.Create(context.Background(), driver.Spec{ID: "x", Image: "i", Ports: []int{8765}})
	if err == nil {
		t.Fatal("expected endpoint error")
	}
	rms := m.callsWith("rm")
	if len(rms) == 0 {
		t.Fatal("expected destroy after endpoint failure")
	}
}

func TestStartStopDestroyReinstall(t *testing.T) {
	m := newMock()
	m.on("start", "", nil)
	m.on("stop", "", nil)
	m.on("rm", "", nil)
	m.on("run", "cid", nil)
	m.on("inspect", "12345", nil)
	d := New(Options{NamePrefix: "sbx-"})
	d.run = m.run

	if err := d.Start(context.Background(), "id1"); err != nil {
		t.Fatal(err)
	}
	if got := m.lastCall(); got[0] != "start" || got[1] != "sbx-id1" {
		t.Fatalf("start args: %v", got)
	}
	if err := d.Stop(context.Background(), "id1"); err != nil {
		t.Fatal(err)
	}
	if got := m.lastCall(); got[0] != "stop" || got[1] != "sbx-id1" {
		t.Fatalf("stop args: %v", got)
	}
	if err := d.Destroy(context.Background(), "id1"); err != nil {
		t.Fatal(err)
	}
	rms := m.callsWith("rm")
	found := false
	for _, c := range rms {
		if containsArg(c, "-v") && containsArg(c, "sbx-id1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("destroy should rm -f -v: %v", rms)
	}

	if err := d.Reinstall(context.Background(), driver.Spec{
		ID: "id1", Image: "img", Ports: []int{80},
	}, true); err != nil {
		t.Fatalf("Reinstall: %v", err)
	}
	// preserveData=true → destroy without -v
	var preserveRM []string
	for _, c := range m.callsWith("rm") {
		if containsArg(c, "sbx-id1") && !containsArg(c, "-v") {
			preserveRM = c
		}
	}
	if preserveRM == nil {
		// last rm before recreate should not include -v when preserveData
		// destroyByName(ctx, name, !preserveData) => removeVolumes=false
		all := m.callsWith("rm")
		lastBeforeCreate := all[len(all)-1]
		if containsArg(lastBeforeCreate, "-v") {
			t.Fatalf("preserveData reinstall should not pass -v: %v", all)
		}
	}
}

func TestGetRunningAndStopped(t *testing.T) {
	m := newMock()
	statusOut := "running"
	d := New(Options{})
	d.run = func(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
		m.mu.Lock()
		m.calls = append(m.calls, append([]string(nil), args...))
		m.mu.Unlock()
		if len(args) > 0 && args[0] == "inspect" {
			format := ""
			for i := 0; i+1 < len(args); i++ {
				if args[i] == "--format" {
					format = args[i+1]
				}
			}
			if strings.Contains(format, "State.Status") {
				return statusOut, nil
			}
			return "8765/tcp=30100\n22/tcp=30101\n", nil
		}
		return "", nil
	}

	h, err := d.Get(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != driver.StatusRunning {
		t.Fatalf("status=%s", h.Status)
	}
	if h.Endpoints[8765] != "127.0.0.1:30100" || h.Endpoints[22] != "127.0.0.1:30101" {
		t.Fatalf("eps: %v", h.Endpoints)
	}

	statusOut = "exited"
	h2, err := d.Get(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if h2.Status != driver.StatusStopped {
		t.Fatalf("stopped status=%s", h2.Status)
	}
	if len(h2.Endpoints) != 0 {
		t.Fatalf("stopped should not discover endpoints: %v", h2.Endpoints)
	}
}

func TestListParsing(t *testing.T) {
	m := newMock()
	m.on("ps", "sbx-aaa\trunning\t2024-01-02 03:04:05 +0000 UTC\nsbx-bbb\texited\t\n  \nbadline\nsbx-ccc\tcreated\t2024-01-02 03:04:05 +0000 UTC\n", nil)
	d := New(Options{NamePrefix: "sbx-"})
	d.run = m.run
	list, err := d.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 handles, got %d: %+v", len(list), list)
	}
	byID := map[string]driver.Status{}
	for _, h := range list {
		byID[h.ID] = h.Status
	}
	if byID["aaa"] != driver.StatusRunning || byID["bbb"] != driver.StatusStopped || byID["ccc"] != driver.StatusPending {
		t.Fatalf("states: %v", byID)
	}
	for _, h := range list {
		if h.ID == "aaa" && h.CreatedAt.IsZero() {
			t.Fatal("expected CreatedAt for aaa")
		}
	}
	ps := m.callsWith("ps")[0]
	if !containsPair(ps, "--filter", "name=sbx-") {
		t.Fatalf("filter: %v", ps)
	}
}

func TestListError(t *testing.T) {
	m := newMock()
	m.on("ps", "", errors.New("fail"))
	d := New(Options{})
	d.run = m.run
	_, err := d.List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStatusNotFound(t *testing.T) {
	m := newMock()
	m.on("inspect", "", errors.New("No such object"))
	d := New(Options{})
	d.run = m.run
	st, err := d.Status(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if st != driver.StatusNotFound {
		t.Fatalf("got %s", st)
	}
}

func TestMapState(t *testing.T) {
	cases := map[string]driver.Status{
		"running":    driver.StatusRunning,
		"created":    driver.StatusPending,
		"restarting": driver.StatusPending,
		"paused":     driver.StatusPending,
		"exited":     driver.StatusStopped,
		"dead":       driver.StatusStopped,
		"stopped":    driver.StatusStopped,
		"":           driver.StatusNotFound,
		"not_found":  driver.StatusNotFound,
		"weird":      driver.StatusStopped,
	}
	for in, want := range cases {
		if got := mapState(in); got != want {
			t.Fatalf("mapState(%q)=%s want %s", in, got, want)
		}
	}
}

func TestEndpointsDiscoverMode(t *testing.T) {
	m := newMock()
	m.on("inspect", "8765/tcp=4000\nbad\n22/tcp=abc\n80/tcp=0\n443/tcp=8443\n", nil)
	d := New(Options{BindIP: "10.1.2.3"})
	d.run = m.run
	eps, err := d.Endpoints(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if eps[8765] != "10.1.2.3:4000" || eps[443] != "10.1.2.3:8443" {
		t.Fatalf("eps: %v", eps)
	}
	if _, ok := eps[22]; ok {
		t.Fatal("non-numeric host port should be skipped")
	}
	if _, ok := eps[80]; ok {
		t.Fatal("zero host port should be skipped")
	}
}

func TestEndpointsDiscoverError(t *testing.T) {
	m := newMock()
	m.on("inspect", "", errors.New("gone"))
	d := New(Options{})
	d.run = m.run
	_, err := d.Endpoints(context.Background(), "id")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHostPortParseError(t *testing.T) {
	m := newMock()
	m.on("inspect", "not-a-port", nil)
	d := New(Options{})
	d.run = m.run
	_, err := d.hostPort(context.Background(), "sbx-x", 8765)
	if err == nil || !strings.Contains(err.Error(), "parse host port") {
		t.Fatalf("want parse error, got %v", err)
	}
}

func TestHostPortInspectError(t *testing.T) {
	m := newMock()
	m.on("inspect", "", fmt.Errorf("inspect fail"))
	d := New(Options{})
	d.run = m.run
	_, err := d.hostPort(context.Background(), "sbx-x", 22)
	if err == nil || !strings.Contains(err.Error(), "docker inspect port") {
		t.Fatalf("want inspect error, got %v", err)
	}
}

func TestDestroyWithoutVolumes(t *testing.T) {
	m := newMock()
	m.on("rm", "", nil)
	d := New(Options{})
	d.run = m.run
	if err := d.destroyByName(context.Background(), "sbx-z", false); err != nil {
		t.Fatal(err)
	}
	args := m.lastCall()
	if containsArg(args, "-v") {
		t.Fatalf("should not include -v: %v", args)
	}
}

func TestDestroyNoSuchContainer(t *testing.T) {
	m := newMock()
	m.on("rm", "", fmt.Errorf("Error: No such container: sbx-gone"))
	d := New(Options{})
	d.run = m.run
	if err := d.Destroy(context.Background(), "gone"); err != nil {
		t.Fatalf("missing container should be ok: %v", err)
	}
}

// TestPackageLevelRun exercises the real docker CLI helper. Docker may or may
// not be available; both success and error paths cover run().
func TestPackageLevelRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := run(ctx, 1500*time.Millisecond, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		// Common when docker is missing or the daemon is unreachable.
		if out != "" && !strings.Contains(err.Error(), out) {
			t.Logf("run error with stdout %q: %v", out, err)
		}
		return
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected non-empty docker version output")
	}
}

func TestPackageLevelRunError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := run(ctx, 200*time.Millisecond, "this-subcommand-does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error from unknown docker subcommand")
	}
}
