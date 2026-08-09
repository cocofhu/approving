package gateshare

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

const (
	nonceTTL         = 15 * time.Minute
	maxNoncesPerLink = 4 // multi-tab preview: keep the last N unused nonces
)

type nonceItem struct {
	nonce   string
	expires time.Time
}

type nonceBucket struct {
	items []nonceItem
}

// NonceStore issues one-time preview nonces keyed by token hash (never plaintext).
// In-process only: multi-replica deployments must share storage (see SECURITY.md).
type NonceStore struct {
	mu     sync.Mutex
	byHash map[string]*nonceBucket
}

// NewNonceStore builds an in-process nonce map.
func NewNonceStore() *NonceStore {
	return &NonceStore{byHash: map[string]*nonceBucket{}}
}

// Issue adds a nonce for tokenHash without invalidating prior unexpired ones
// (up to maxNoncesPerLink) so multiple tabs can submit after each previewed.
func (s *NonceStore) Issue(tokenHash string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	n := hex.EncodeToString(buf)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	b := s.byHash[tokenHash]
	if b == nil {
		b = &nonceBucket{}
		s.byHash[tokenHash] = b
	}
	b.items = append(b.items, nonceItem{nonce: n, expires: now.Add(nonceTTL)})
	if len(b.items) > maxNoncesPerLink {
		b.items = b.items[len(b.items)-maxNoncesPerLink:]
	}
	return n, nil
}

// Consume validates and deletes a matching nonce. Constant-time compare per item.
func (s *NonceStore) Consume(tokenHash, nonce string) bool {
	nonce = hexNormalize(nonce)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.byHash[tokenHash]
	if !ok || len(b.items) == 0 {
		return false
	}
	want := []byte(nonce)
	idx := -1
	for i, e := range b.items {
		if now.After(e.expires) {
			continue
		}
		got := []byte(e.nonce)
		if len(got) == len(want) && subtle.ConstantTimeCompare(got, want) == 1 {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.gcBucketLocked(b, now)
		if len(b.items) == 0 {
			delete(s.byHash, tokenHash)
		}
		return false
	}
	b.items = append(b.items[:idx], b.items[idx+1:]...)
	if len(b.items) == 0 {
		delete(s.byHash, tokenHash)
	}
	return true
}

func (s *NonceStore) gcLocked(now time.Time) {
	for k, b := range s.byHash {
		s.gcBucketLocked(b, now)
		if len(b.items) == 0 {
			delete(s.byHash, k)
		}
	}
}

func (s *NonceStore) gcBucketLocked(b *nonceBucket, now time.Time) {
	if b == nil {
		return
	}
	alive := b.items[:0]
	for _, e := range b.items {
		if now.After(e.expires) {
			continue
		}
		alive = append(alive, e)
	}
	b.items = alive
}

func hexNormalize(s string) string {
	return s
}
