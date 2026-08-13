package mcp

import (
	"context"
	"testing"
	"time"
)

type memPreviewStore struct {
	ports []PreviewPort
	err   error
}

func (m *memPreviewStore) UpsertPreviewPort(rec PreviewPort) error {
	if m.err != nil {
		return m.err
	}
	for i, p := range m.ports {
		if p.RunID == rec.RunID && p.NodeID == rec.NodeID && p.Port == rec.Port {
			m.ports[i] = rec
			return nil
		}
	}
	m.ports = append(m.ports, rec)
	return nil
}
func (m *memPreviewStore) ListPreviewPorts(runID, nodeID string) ([]PreviewPort, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []PreviewPort
	for _, p := range m.ports {
		if p.RunID == runID && p.NodeID == nodeID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (m *memPreviewStore) GetPreviewPort(runID, nodeID string, port int) (*PreviewPort, bool) {
	for i, p := range m.ports {
		if p.RunID == runID && p.NodeID == nodeID && p.Port == port {
			return &m.ports[i], true
		}
	}
	return nil, false
}
func (m *memPreviewStore) UpdatePreviewHealth(string, string, int, bool) error { return nil }

type fakePreviewOps struct {
	name    string
	ok      bool
	healthy bool
	up      string
	warmed  []string
	direct  bool
}

func (f *fakePreviewOps) SandboxForRunNode(string, string) (string, bool) { return f.name, f.ok }
func (f *fakePreviewOps) ProbeHTTPPort(context.Context, string, int) bool { return f.healthy }
func (f *fakePreviewOps) KeepalivePort(context.Context, string, int) (int, error) {
	return 4242, nil
}
func (f *fakePreviewOps) PreviewUpstream(context.Context, string, int) (string, bool) {
	if f.up == "" {
		return "", false
	}
	return f.up, true
}
func (f *fakePreviewOps) WarmPreviewVNC(sandboxName string) {
	f.warmed = append(f.warmed, sandboxName)
}
func (f *fakePreviewOps) DirectPreview(string, string) bool { return f.direct }

func TestParsePreviewPortTypes(t *testing.T) {
	cases := []struct {
		in   any
		want int
		ok   bool
	}{
		{float64(3000), 3000, true},
		{int(80), 80, true},
		{int64(443), 443, true},
		{"8080", 8080, true},
		{" 9 ", 9, true},
		{"bad", 0, false},
		{true, 0, false},
	}
	for _, tc := range cases {
		got, err := parsePreviewPort(tc.in)
		if tc.ok {
			if err != nil || got != tc.want {
				t.Fatalf("in=%v got=%d err=%v", tc.in, got, err)
			}
		} else if err == nil {
			t.Fatalf("in=%v want error", tc.in)
		}
	}
}

func TestListPreviewPortsMergeStore(t *testing.T) {
	h := NewHost(&memStore{})
	store := &memPreviewStore{ports: []PreviewPort{{
		RunID: "r", NodeID: "n", Port: 1, Label: "db", ProxyURL: "/preview/r/n/1/",
	}}}
	h.SetPreviewStore(store)
	h.SetPreviewSandboxOps(&fakePreviewOps{name: "sb", ok: true, healthy: true, up: "http://10.0.0.1:1"})

	// memory + db merge
	h.mu.Lock()
	h.previewMem = map[string][]PreviewPort{"r|n": {{
		RunID: "r", NodeID: "n", Port: 2, Label: "mem", ProxyURL: "/preview/r/n/2/",
	}}}
	h.mu.Unlock()
	ports := h.ListPreviewPorts("r", "n")
	if len(ports) != 2 {
		t.Fatalf("merged=%d %+v", len(ports), ports)
	}

	url, err := h.setPreviewPort("r", "n", 3000, "web")
	if err != nil || url == "" {
		t.Fatalf("setPreviewPort: %v %q", err, url)
	}
	ops := h.previewOps.(*fakePreviewOps)
	if len(ops.warmed) != 1 || ops.warmed[0] != "sb" {
		t.Fatalf("warmed=%v", ops.warmed)
	}
	if _, err := h.setPreviewPort("r", "n", 0, ""); err == nil {
		t.Fatal("port 0")
	}
	if _, err := h.setPreviewPort("r", "n", 70000, ""); err == nil {
		t.Fatal("port high")
	}
	_ = time.Second
}

func TestPreviewReadySignalAndKeepalivePID(t *testing.T) {
	h := NewHost(&memStore{})
	h.SetPreviewSandboxOps(&fakePreviewOps{name: "sb", ok: true, healthy: true, up: "http://10.0.0.1:3000"})
	ch := h.PreviewReadyChan("r", "n")
	select {
	case <-ch:
		t.Fatal("should not be ready yet")
	default:
	}
	if _, err := h.setPreviewPort("r", "n", 3000, "web"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	default:
		t.Fatal("expected PreviewReady after healthy set_preview")
	}
	pids := h.ListPreviewKeepalivePIDs("r", "n")
	if len(pids) != 1 || pids[0] != 4242 {
		t.Fatalf("keepalive pids=%v", pids)
	}
	if !h.IsPreviewKeepalivePID("r", 4242) {
		t.Fatal("expected whitelist hit")
	}
	if h.IsPreviewKeepalivePID("r", 1) {
		t.Fatal("unexpected whitelist hit")
	}
}

func TestSetPreviewDirectSkipsVNCWarmAndSetsDirectURL(t *testing.T) {
	h := NewHost(&memStore{})
	h.SetPreviewSandboxOps(&fakePreviewOps{name: "sb", ok: true, healthy: true, up: "http://10.0.0.8:18081", direct: true})
	if _, err := h.setPreviewPort("r", "n", 18081, "web"); err != nil {
		t.Fatal(err)
	}
	ops := h.previewOps.(*fakePreviewOps)
	if len(ops.warmed) != 0 {
		t.Fatalf("direct mode must not warm VNC: %v", ops.warmed)
	}
	ports := h.ListPreviewPorts("r", "n")
	if len(ports) != 1 || ports[0].Mode != "direct" || ports[0].DirectURL != "http://10.0.0.8:18081/" {
		t.Fatalf("ports=%+v", ports)
	}
}
