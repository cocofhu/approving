package previewinject

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var errUnsupportedEncoding = errors.New("unsupported content-encoding")

const flushInterval = 100 * time.Millisecond

// NewHandler reverse-proxies to upstream and injects preview-pick.js into
// text/html. Host is preserved (Vite allowedHosts / HMR). WebSocket 101 and
// non-HTML bodies are passed through. Location is not rewritten (direct
// preview has no /preview prefix).
//
// Accept-Encoding is forced to identity so HTML can be rewritten. If the
// upstream still sends gzip/deflate, the body is decoded. brotli/zstd are
// left untouched (no rewrite) rather than corrupted.
//
// When upstream is unreachable the connection is closed without an HTTP
// status. Approving ProbeHTTPPort treats any HTTP response as healthy; a
// 502 here would make set_preview succeed before the app is listening.
func NewHandler(upstream *url.URL, scriptURL string) http.Handler {
	if upstream == nil {
		panic("previewinject: nil upstream")
	}
	scriptURL = strings.TrimSpace(scriptURL)
	origin := ScriptOrigin(scriptURL)

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			inHost := r.In.Host
			r.SetURL(upstream)
			r.Out.Host = inHost
			r.Out.Header.Set("Accept-Encoding", "identity")
		},
		ModifyResponse: modifyResponse(scriptURL, origin),
		FlushInterval:  flushInterval,
		ErrorHandler:   closeOnUpstreamError,
	}
	return proxy
}

func closeOnUpstreamError(w http.ResponseWriter, _ *http.Request, _ error) {
	if hj, ok := w.(http.Hijacker); ok {
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close()
			return
		}
	}
	w.WriteHeader(http.StatusBadGateway)
}

func modifyResponse(scriptURL, scriptOrigin string) func(*http.Response) error {
	return func(resp *http.Response) error {
		if resp.StatusCode == http.StatusSwitchingProtocols {
			return nil
		}
		if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
			return nil
		}
		if scriptURL == "" {
			return nil
		}

		enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
		if unsupportedEncoding(enc) {
			return nil
		}

		body, err := readDecodableBody(resp)
		if err != nil {
			if errors.Is(err, errUnsupportedEncoding) {
				return nil
			}
			return err
		}

		rewritten := InjectHTML(body, scriptURL)
		if scriptOrigin != "" {
			for _, h := range []string{"Content-Security-Policy", "Content-Security-Policy-Report-Only"} {
				if csp := resp.Header.Get(h); csp != "" {
					resp.Header.Set(h, RelaxCSP(csp, scriptOrigin))
				}
			}
		}

		resp.Body = io.NopCloser(bytes.NewReader(rewritten))
		resp.ContentLength = int64(len(rewritten))
		resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
		resp.Header.Del("Content-Encoding")
		return nil
	}
}

func unsupportedEncoding(enc string) bool {
	switch enc {
	case "", "identity", "gzip", "x-gzip", "deflate":
		return false
	default:
		return true
	}
}

func readDecodableBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	var r io.Reader = resp.Body
	switch enc {
	case "", "identity":
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		r = gr
	case "deflate":
		fr := flate.NewReader(resp.Body)
		defer fr.Close()
		r = fr
	default:
		return nil, errUnsupportedEncoding
	}
	return io.ReadAll(r)
}
