package gateshare

import (
	"io"
	"strings"
	"sync"
)

// PreviewSessionHub tracks active public preview sessions (VNC / API proxy)
// so share-link revocation can cut in-flight connections.
type PreviewSessionHub struct {
	mu   sync.Mutex
	byTH map[string]map[uint64]io.Closer
	next uint64
}

// NewPreviewSessionHub builds an empty session hub.
func NewPreviewSessionHub() *PreviewSessionHub {
	return &PreviewSessionHub{byTH: map[string]map[uint64]io.Closer{}}
}

// Register attaches a closer under tokenHash; the returned unregister must be
// called when the session ends.
func (h *PreviewSessionHub) Register(tokenHash string, c io.Closer) (unregister func()) {
	if h == nil || c == nil {
		return func() {}
	}
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return func() {}
	}
	h.mu.Lock()
	id := h.next + 1
	h.next = id
	if h.byTH[tokenHash] == nil {
		h.byTH[tokenHash] = map[uint64]io.Closer{}
	}
	h.byTH[tokenHash][id] = c
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if m := h.byTH[tokenHash]; m != nil {
			delete(m, id)
			if len(m) == 0 {
				delete(h.byTH, tokenHash)
			}
		}
	}
}

// KickByTokenHash closes all sessions for a share-link token hash.
func (h *PreviewSessionHub) KickByTokenHash(tokenHash string) {
	if h == nil {
		return
	}
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return
	}
	h.mu.Lock()
	m := h.byTH[tokenHash]
	delete(h.byTH, tokenHash)
	h.mu.Unlock()
	for _, c := range m {
		_ = c.Close()
	}
}

// KickMany closes sessions for each token hash.
func (h *PreviewSessionHub) KickMany(tokenHashes []string) {
	seen := map[string]struct{}{}
	for _, th := range tokenHashes {
		th = strings.TrimSpace(th)
		if th == "" {
			continue
		}
		if _, ok := seen[th]; ok {
			continue
		}
		seen[th] = struct{}{}
		h.KickByTokenHash(th)
	}
}
