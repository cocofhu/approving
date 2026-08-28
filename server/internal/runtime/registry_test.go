package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
)

// writeProfileInto materializes <root>/<profile>/agent.json so multiple
// profiles can share a single ProfilesRoot (writeAgent uses a fresh TempDir
// per call and can't).
func writeProfileInto(t *testing.T, root, profile, agentJSON string) {
	t.Helper()
	dir := filepath.Join(root, profile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), []byte(agentJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

func reqFor(profile string) NodeReq {
	return NodeReq{Config: map[string]any{"agent_profile": profile}}
}

// TestNewProviderRegistryBuildsAllBackends asserts the registry wires one
// provider per known backend, each stamped with its own backend identity.
func TestNewProviderRegistryBuildsAllBackends(t *testing.T) {
	host := mcp.NewHost(newMemStore())
	reg := NewProviderRegistry(host, Options{})
	if reg.Name() != "registry" {
		t.Fatalf("Name = %q, want registry", reg.Name())
	}
	for _, b := range []AcpBackend{BackendCursor, BackendClaudeCode, BackendCodeBuddy, BackendTrae} {
		p, ok := reg.providers[b]
		if !ok || p == nil {
			t.Fatalf("missing provider for backend %q", b)
		}
		cp, ok := p.(*acpProvider)
		if !ok {
			t.Fatalf("provider %q is %T, want *acpProvider", b, p)
		}
		if cp.backend != b {
			t.Fatalf("provider[%q].backend = %q", b, cp.backend)
		}
	}
}

// TestProviderRegistryRouting covers backendFor / providerFor resolution from a
// agent_profile's agent.json acpBackend field, plus every fallback-to-cursor path.
func TestProviderRegistryRouting(t *testing.T) {
	root := t.TempDir()
	writeProfileInto(t, root, "cur", `{"acpBackend":"cursor"}`)
	writeProfileInto(t, root, "cc", `{"acpBackend":"claude_code"}`)
	writeProfileInto(t, root, "cb", `{"acpBackend":"codebuddy"}`)
	writeProfileInto(t, root, "tr", `{"acpBackend":"trae"}`)
	writeProfileInto(t, root, "weird", `{"acpBackend":"nope"}`) // unknown → cursor
	writeProfileInto(t, root, "nobackend", `{"env":{"X":"y"}}`) // absent field → cursor
	writeProfileInto(t, root, "broken", `{not-json`)            // unparsable → cursor

	host := mcp.NewHost(newMemStore())
	reg := NewProviderRegistry(host, Options{ProfilesRoot: root})

	cases := []struct {
		profile string
		want    AcpBackend
	}{
		{"cur", BackendCursor},
		{"cc", BackendClaudeCode},
		{"cb", BackendCodeBuddy},
		{"tr", BackendTrae},
		{"weird", BackendCursor},
		{"nobackend", BackendCursor},
		{"broken", BackendCursor},
		{"does-not-exist", BackendCursor}, // missing dir → cursor
		{"", BackendCursor},               // empty profile → cursor
		{"sub/cc", BackendClaudeCode},     // filepath.Base strips traversal → cc
	}
	for _, tc := range cases {
		req := reqFor(tc.profile)
		if got := reg.backendFor(req); got != tc.want {
			t.Errorf("backendFor(%q) = %q, want %q", tc.profile, got, tc.want)
		}
		p, ok := reg.providerFor(req).(*acpProvider)
		if !ok {
			t.Fatalf("providerFor(%q) not *acpProvider", tc.profile)
		}
		if p.backend != tc.want {
			t.Errorf("providerFor(%q).backend = %q, want %q", tc.profile, p.backend, tc.want)
		}
	}
}

// TestProviderRegistryNoProfilesRoot: without a ProfilesRoot the registry can't
// read agent.json, so every request routes to the cursor default.
func TestProviderRegistryNoProfilesRoot(t *testing.T) {
	host := mcp.NewHost(newMemStore())
	reg := NewProviderRegistry(host, Options{}) // no ProfilesRoot
	if got := reg.backendFor(reqFor("anything")); got != BackendCursor {
		t.Fatalf("no ProfilesRoot must route to cursor, got %q", got)
	}
}

// TestProviderRegistryProviderForMissingBackend guards providerFor's nil-map
// fallback: an unregistered backend still yields the cursor provider.
func TestProviderRegistryProviderForMissingBackend(t *testing.T) {
	host := mcp.NewHost(newMemStore())
	reg := NewProviderRegistry(host, Options{})
	delete(reg.providers, BackendTrae) // simulate a backend with no provider wired
	root := t.TempDir()
	writeProfileInto(t, root, "tr", `{"acpBackend":"trae"}`)
	reg.profilesRoot = root
	p := reg.providerFor(reqFor("tr")).(*acpProvider)
	if p.backend != BackendCursor {
		t.Fatalf("missing-backend fallback = %q, want cursor", p.backend)
	}
}

// TestProviderRegistrySinkPropagation asserts SetEventSink / SetSandboxRegistry
// fan out to every wrapped provider.
func TestProviderRegistrySinkPropagation(t *testing.T) {
	host := mcp.NewHost(newMemStore())
	reg := NewProviderRegistry(host, Options{})
	reg.SetEventSink(func(string, string, []models.AcpEvent, bool) {})
	regStore := &countingRegistry{}
	reg.SetSandboxRegistry(regStore)
	for b, p := range reg.providers {
		cp := p.(*acpProvider)
		if cp.emit == nil {
			t.Errorf("provider %q missing event sink", b)
		}
		if cp.registry != regStore {
			t.Errorf("provider %q missing sandbox registry", b)
		}
	}
}

// TestProviderRegistryLiveEventsNoLive verifies the fan-out read helpers report
// ok=false when no wrapped provider has a matching live node.
func TestProviderRegistryLiveEventsNoLive(t *testing.T) {
	host := mcp.NewHost(newMemStore())
	reg := NewProviderRegistry(host, Options{})
	if _, ok, err := reg.LiveNodeEvents(context.Background(), "r", "n"); ok || err != nil {
		t.Errorf("LiveNodeEvents should be ok=false err=nil with no live nodes, got ok=%v err=%v", ok, err)
	}
	if _, _, _, ok, err := reg.LiveNodeEventsPage(context.Background(), "r", "n", "", 10); ok || err != nil {
		t.Errorf("LiveNodeEventsPage should be ok=false err=nil with no live nodes, got ok=%v err=%v", ok, err)
	}
}

// TestProviderRegistryReviewProviderForwarding: production Engine type-asserts
// the top-level provider to ReviewProvider; the registry must implement and
// fan out HasLiveSession/RetireSession, and route ReviseInPlace by profile.
func TestProviderRegistryReviewProviderForwarding(t *testing.T) {
	host := mcp.NewHost(newMemStore())
	reg := NewProviderRegistry(host, Options{})
	var _ ReviewProvider = reg

	if reg.HasLiveSession("r", "n") {
		t.Fatal("HasLiveSession should be false with no parked sessions")
	}
	// RetireSession is an idempotent no-op when nothing is parked.
	reg.RetireSession("r", "n")

	// Without a live session ReviseInPlace rehydrates (and fails without Docker);
	// we only assert the call is forwarded and returns a non-panic turn.
	turn := reg.ReviseInPlace(context.Background(), NodeReq{RunID: "r", NodeID: "n"}, nil, "edit", nil)
	if turn.Done {
		t.Fatal("ReviseInPlace must never mark Done")
	}
	if turn.Err == nil && turn.Msg == "" {
		t.Fatal("expected a revise turn message or error when no session exists")
	}

	wrap := reg.OfferCommitOnConfirm(context.Background(), NodeReq{RunID: "r", NodeID: "n"})
	if wrap.Done {
		t.Fatal("OfferCommitOnConfirm must not mark Done")
	}

	// Without a parked session the confirm-time reconcile is a best-effort
	// no-op: the human already confirmed, so it must not fail the transition.
	rec := reg.ReconcileOnConfirm(context.Background(), NodeReq{RunID: "r", NodeID: "n"})
	if rec.Done || rec.Msg != "" || rec.AgentSummary != "" || rec.Err != nil {
		t.Fatalf("ReconcileOnConfirm without a session must be an empty turn, got %+v", rec)
	}
}
