package browser

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakePage struct {
	closed  bool
	onPick  func(Pick)
	mouse   []MouseEvent
	inspect bool
}

func (p *fakePage) StartScreencast(func(Frame)) error   { return nil }
func (p *fakePage) DispatchMouse(m MouseEvent) error    { p.mouse = append(p.mouse, m); return nil }
func (p *fakePage) DispatchKey(KeyEvent) error          { return nil }
func (p *fakePage) SetViewport(int, int, float64) error { return nil }
func (p *fakePage) SetInspect(on bool) error            { p.inspect = on; return nil }
func (p *fakePage) OnPick(cb func(Pick))                { p.onPick = cb }
func (p *fakePage) OnInspectCanceled(func())            {}
func (p *fakePage) Navigate(string) error               { return nil }
func (p *fakePage) Goto(string) error                   { return nil }
func (p *fakePage) Close() error                        { p.closed = true; return nil }

type fakeEngine struct {
	name     string
	failTab  bool
	failOnce bool
	closed   bool
	pages    []*fakePage
}

func (e *fakeEngine) NewTab(context.Context, string) (Page, error) {
	if e.failOnce {
		e.failOnce = false
		return nil, errors.New("tab failed")
	}
	if e.failTab {
		return nil, errors.New("tab failed")
	}
	p := &fakePage{}
	e.pages = append(e.pages, p)
	return p, nil
}
func (e *fakeEngine) Close() error { e.closed = true; return nil }

// fakeSandbox implements SandboxExecer: the only sandbox-manager capability the
// browser subsystem uses (start VNC on demand). It records exec invocations.
type fakeSandbox struct {
	mu      sync.Mutex
	execs   []string
	execErr error
}

func (d *fakeSandbox) Exec(_ context.Context, name string, _ time.Duration, _ ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.execs = append(d.execs, name)
	return "", d.execErr
}

func newFakeService(cfg Config) (*Service, *fakeSandbox, *clock) {
	d := &fakeSandbox{}
	s := New(d, cfg)
	c := &clock{t: time.Unix(1_700_000_000, 0)}
	s.reg.now = c.now
	s.readyProbe = func(context.Context, string) bool { return true }
	s.dial = func(_ context.Context, _ string) (Engine, error) {
		return &fakeEngine{name: "sandbox"}, nil
	}
	return s, d, c
}

func TestOpenInSandboxAttachesWithoutPool(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1})
	ctx := context.Background()
	sess, err := s.OpenInSandbox(ctx, "approving-sb-preview", "10.0.0.9", "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatalf("OpenInSandbox: %v", err)
	}
	defer sess.Close()
	if sess.container != "approving-sb-preview" {
		t.Fatalf("container = %q", sess.container)
	}
	vnc, err := sess.VNCWebSocketURL()
	if err != nil {
		t.Fatal(err)
	}
	if vnc != "ws://10.0.0.9:6080" {
		t.Fatalf("vnc url = %q", vnc)
	}
	// Stop only disconnects; the sandbox lifecycle is not our concern.
	s.Stop()
}

func TestOpenInSandboxSupersedesExistingSession(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1})
	ctx := context.Background()
	sandbox := "approving-sb-preview"
	s1, err := s.OpenInSandbox(ctx, sandbox, "10.0.0.9", "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatalf("first OpenInSandbox: %v", err)
	}
	s2, err := s.OpenInSandbox(ctx, sandbox, "10.0.0.9", "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatalf("second OpenInSandbox: %v", err)
	}
	select {
	case <-s1.Done():
	default:
		t.Fatal("s1 should have been superseded")
	}
	if s1.Reason() != "superseded" {
		t.Fatalf("s1 reason = %q, want superseded", s1.Reason())
	}
	s.mu.Lock()
	count := s.reg.containerCount(sandbox)
	s.mu.Unlock()
	if count != 1 {
		t.Fatalf("containerCount = %d, want 1", count)
	}
	if s2.Reason() != "" {
		t.Fatalf("s2 should still be active, reason = %q", s2.Reason())
	}
}

func TestOpenInSandboxRedialsEngineOnNewTabFailure(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1})
	dialCount := 0
	s.dial = func(_ context.Context, _ string) (Engine, error) {
		dialCount++
		return &fakeEngine{name: "sandbox", failOnce: dialCount == 1}, nil
	}
	ctx := context.Background()
	sess, err := s.OpenInSandbox(ctx, "approving-sb-preview", "10.0.0.9", "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatalf("OpenInSandbox: %v", err)
	}
	defer sess.Close()
	if dialCount != 2 {
		t.Fatalf("dial count = %d, want 2 (initial + redial)", dialCount)
	}
}

func TestOpenInSandboxRespectsCanceledContext(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 1})
	s.readyProbe = func(ctx context.Context, _ string) bool {
		<-ctx.Done()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.OpenInSandbox(ctx, "approving-sb-preview", "10.0.0.9", "http://127.0.0.1:3000/")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// open is a helper that opens a tab in a distinct sandbox (each sandbox is one
// external CDP attachment).
func open(t *testing.T, s *Service, sandbox string) *Session {
	t.Helper()
	sess, err := s.OpenInSandbox(context.Background(), sandbox, "10.0.0.9", "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatalf("OpenInSandbox(%s): %v", sandbox, err)
	}
	return sess
}

func TestEvictsLRUWhenFull(t *testing.T) {
	s, _, c := newFakeService(Config{MaxTabs: 2, MaxTabsPerContainer: 1})
	s1 := open(t, s, "sb-a")
	c.add(time.Second)
	s2 := open(t, s, "sb-b")
	c.add(time.Second)
	// Full (2/2): opening a third sandbox evicts the LRU (s1).
	s3 := open(t, s, "sb-c")
	select {
	case <-s1.Done():
	default:
		t.Fatal("s1 should have been evicted")
	}
	if s1.Reason() != "evicted" {
		t.Fatalf("s1 reason = %q, want evicted", s1.Reason())
	}
	if !s1.page.(*fakePage).closed {
		t.Fatal("evicted page should be closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[s2.ID]; !ok {
		t.Fatal("s2 should still be present")
	}
	if _, ok := s.sessions[s3.ID]; !ok {
		t.Fatal("s3 should be present")
	}
	if s.reg.count() != 2 {
		t.Fatalf("tabs = %d, want 2", s.reg.count())
	}
}

func TestCapacityWhenMaxZero(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 0, MaxTabsPerContainer: 5})
	if _, err := s.OpenInSandbox(context.Background(), "sb-a", "10.0.0.9", "http://app/"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("err = %v, want ErrCapacity", err)
	}
}

func TestCloseSession(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 10, MaxTabsPerContainer: 5})
	sess := open(t, s, "sb-a")
	page := sess.page.(*fakePage)
	sess.Close()
	if !page.closed {
		t.Fatal("page not closed")
	}
	select {
	case <-sess.Done():
	default:
		t.Fatal("Done not closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reg.count() != 0 {
		t.Fatalf("tabs = %d, want 0", s.reg.count())
	}
}

func TestSweepFreesIdleAndDropsEngine(t *testing.T) {
	s, _, c := newFakeService(Config{MaxTabs: 10, MaxTabsPerContainer: 5, TabIdleTTL: time.Minute, ContainerIdleTTL: 2 * time.Minute})
	sess := open(t, s, "sb-a")
	// Advance past the tab idle TTL → sweep frees the tab.
	c.add(90 * time.Second)
	s.sweep()
	if s.reg.count() != 0 {
		t.Fatalf("tabs after idle sweep = %d, want 0", s.reg.count())
	}
	if sess.Reason() != "idle" {
		t.Fatalf("reason = %q, want idle", sess.Reason())
	}
	// Advance past the container idle TTL → sweep drops the cached CDP engine
	// (but never destroys the sandbox).
	c.add(3 * time.Minute)
	s.sweep()
	s.mu.Lock()
	nContainers := len(s.containers)
	s.mu.Unlock()
	if nContainers != 0 {
		t.Fatalf("containers after reap = %d, want 0", nContainers)
	}
}

func TestSetMaxTabsRejectsBelowOne(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 4, MaxTabsPerContainer: 5})
	if err := s.SetMaxTabs(0); !errors.Is(err, ErrInvalidMaxTabs) {
		t.Fatalf("SetMaxTabs(0) = %v, want ErrInvalidMaxTabs", err)
	}
	st := s.Stats()
	if st.MaxTabs != 4 {
		t.Fatalf("MaxTabs = %d, want 4 (unchanged)", st.MaxTabs)
	}
}

func TestSetMaxTabsPassiveShrink(t *testing.T) {
	s, _, c := newFakeService(Config{MaxTabs: 2, MaxTabsPerContainer: 5})
	s1 := open(t, s, "sb-a")
	c.add(time.Second)
	s2 := open(t, s, "sb-b")
	if err := s.SetMaxTabs(1); err != nil {
		t.Fatal(err)
	}
	for _, sess := range []*Session{s1, s2} {
		select {
		case <-sess.Done():
			t.Fatalf("session %s closed during passive shrink", sess.ID)
		default:
		}
	}
	st := s.Stats()
	if st.TabCount != 2 || st.MaxTabs != 1 {
		t.Fatalf("after shrink: %+v, want tabCount=2 maxTabs=1", st)
	}
}

func TestStatsSnapshot(t *testing.T) {
	s, _, _ := newFakeService(Config{MaxTabs: 8, MaxTabsPerContainer: 5})
	open(t, s, "sb-a")
	st := s.Stats()
	if st.TabCount != 1 || st.MaxTabs != 8 || st.ContainerCount != 1 {
		t.Fatalf("Stats = %+v", st)
	}
}
