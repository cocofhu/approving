package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// DefaultInjectBundleTTL is how long a published ConfigHome .tgz stays downloadable.
const DefaultInjectBundleTTL = 30 * time.Minute

// BundleStore holds short-lived ConfigHome archives for gateway config.bundleUrl
// inject. Sandbox startup.sh fetches SANDBOX_INJECT before services start.
type BundleStore struct {
	mu    sync.Mutex
	items map[string]bundleItem
}

type bundleItem struct {
	token string
	data  []byte
	exp   time.Time
}

// NewBundleStore creates an empty inject bundle registry.
func NewBundleStore() *BundleStore {
	return &BundleStore{items: map[string]bundleItem{}}
}

// Put registers gzipped-tar bytes and returns an id + bearer token.
func (s *BundleStore) Put(data []byte, ttl time.Duration) (id, token string) {
	if s == nil {
		return "", ""
	}
	if ttl <= 0 {
		ttl = DefaultInjectBundleTTL
	}
	id = randomHex(16)
	token = randomHex(24)
	s.mu.Lock()
	s.sweepLocked(time.Now())
	s.items[id] = bundleItem{token: token, data: data, exp: time.Now().Add(ttl)}
	s.mu.Unlock()
	return id, token
}

// Get returns bundle bytes when id+token match and not expired.
func (s *BundleStore) Get(id, token string) ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	id = strings.TrimSuffix(strings.TrimSpace(id), ".tgz")
	id = strings.TrimSuffix(id, ".tar.gz")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(time.Now())
	it, ok := s.items[id]
	if !ok || it.token == "" || it.token != token {
		return nil, false
	}
	return it.data, true
}

// Delete removes a bundle (best-effort after sandbox is running).
func (s *BundleStore) Delete(id string) {
	if s == nil {
		return
	}
	id = strings.TrimSuffix(strings.TrimSpace(id), ".tgz")
	s.mu.Lock()
	delete(s.items, id)
	s.mu.Unlock()
}

func (s *BundleStore) sweepLocked(now time.Time) {
	for id, it := range s.items {
		if now.After(it.exp) {
			delete(s.items, id)
		}
	}
}

// ServeHTTP serves GET /sandbox-inject/:id(.tgz) with Authorization: Bearer.
func (s *BundleStore) ServeHTTP(w http.ResponseWriter, r *http.Request, idParam string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := bearerToken(r.Header.Get("Authorization"))
	data, ok := s.Get(idParam, token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="config-home.tgz"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(data)
}

func bearerToken(h string) string {
	h = strings.TrimSpace(h)
	if len(h) >= 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// fallback: time-based (tests / constrained entropy)
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// PackNamedFileTarGz builds a .tar.gz containing a single named file at the tar
// root (used to bootstrap artifact-upload into sandboxes before git clone).
func PackNamedFileTarGz(name string, content []byte) ([]byte, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("tar entry name required")
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name:    filepath.ToSlash(name),
		Mode:    0755,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return nil, err
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PackConfigHomeTarGz builds a .tar.gz of hostDir for SANDBOX_INJECT / bundleUrl.
func PackConfigHomeTarGz(hostDir string) ([]byte, error) {
	info, err := os.Stat(hostDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("config home %q is not a directory", hostDir)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	err = filepath.WalkDir(hostDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(hostDir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			if !strings.HasSuffix(hdr.Name, "/") {
				hdr.Name += "/"
			}
			return tw.WriteHeader(hdr)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		_ = f.Close()
		return copyErr
	})
	if err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return nil, err
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	log.Debug().Int("bytes", buf.Len()).Str("dir", hostDir).Msg("packed config-home inject bundle")
	return buf.Bytes(), nil
}
