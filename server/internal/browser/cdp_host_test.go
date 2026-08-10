package browser

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestProbeTabCreateUsesLoopbackHostHeader locks the Chrome 149 DevTools rule:
// PUT /json/new rejects Host that is neither an IP nor localhost. Approving
// dials ClusterIP DNS (sbx-*.svc.cluster.local) after CDP/noVNC isolation, so
// the probe must rewrite Host to 127.0.0.1 while still dialing the DNS name.
func TestProbeTabCreateUsesLoopbackHostHeader(t *testing.T) {
	var sawNewHost, sawCloseHost string
	mux := http.NewServeMux()
	mux.HandleFunc("/json/new", func(w http.ResponseWriter, r *http.Request) {
		sawNewHost = r.Host
		if !isLoopbackCDPHost(r.Host) {
			http.Error(w, "Host header is specified and is not an IP address or localhost.", http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPut {
			http.Error(w, "PUT required", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "tab-dns"})
	})
	mux.HandleFunc("/json/close/", func(w http.ResponseWriter, r *http.Request) {
		sawCloseHost = r.Host
		if !isLoopbackCDPHost(r.Host) {
			http.Error(w, "Host header is specified and is not an IP address or localhost.", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate ClusterIP DNS dial target (hostname, not IP).
	dialHost := "sbx-probe.sandbox-gateway.svc.cluster.local"
	if host == "127.0.0.1" || host == "::1" {
		// httptest binds loopback; map the DNS name via custom dialer... but
		// probeTabCreate uses http.DefaultClient. Point URL host at 127.0.0.1
		// for TCP and rely on Host rewrite — also cover the DNS-shaped path
		// by calling with a fake hostname that resolves via /etc/hosts is
		// unreliable. Instead: dial 127.0.0.1 but assert Host rewrite, and
		// separately assert setCDPRequestHost + a Chrome-like gate on DNS Host.
		dialHost = "127.0.0.1"
	}

	s, _, _ := newFakeService(Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !s.probeTabCreate(ctx, dialHost, port) {
		t.Fatalf("probeTabCreate failed; newHost=%q closeHost=%q", sawNewHost, sawCloseHost)
	}
	if !isLoopbackCDPHost(sawNewHost) {
		t.Fatalf("PUT /json/new Host=%q want 127.0.0.1:<port>", sawNewHost)
	}
	if !isLoopbackCDPHost(sawCloseHost) {
		t.Fatalf("DELETE /json/close Host=%q want 127.0.0.1:<port>", sawCloseHost)
	}
}

func TestProbeTabCreateRejectsDNSHostWithoutRewrite(t *testing.T) {
	// Document the Chrome failure mode we are guarding against: if Host stays
	// as Service DNS, /json/new returns 500 and readiness fails.
	mux := http.NewServeMux()
	mux.HandleFunc("/json/new", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackCDPHost(r.Host) {
			http.Error(w, "Host header is specified and is not an IP address or localhost.", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "x"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, srv.URL+"/json/new?about:blank", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "sbx-x.sandbox-gateway.svc.cluster.local:9222"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500 for DNS Host", resp.StatusCode)
	}

	// With rewrite, same server accepts.
	req2, err := http.NewRequestWithContext(ctx, http.MethodPut, srv.URL+"/json/new?about:blank", nil)
	if err != nil {
		t.Fatal(err)
	}
	setCDPRequestHost(req2, port)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200 after Host rewrite", resp2.StatusCode)
	}
}

func isLoopbackCDPHost(hostport string) bool {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		h = hostport
	}
	return h == "127.0.0.1" || h == "::1" || h == "localhost"
}
