package gateshare

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

const nonceTTL = 15 * time.Minute

type nonceEntry struct {
	nonce   string
	expires time.Time
}

// NonceStore issues one-time preview nonces keyed by token hash (never plaintext).
type NonceStore struct {
	mu   sync.Mutex
	byHash map[string]nonceEntry
}

// NewNonceStore builds an in-process nonce map.
func NewNonceStore() *NonceStore {
	return &NonceStore{byHash: map[string]nonceEntry{}}
}

// Issue replaces any prior nonce for tokenHash.
func (s *NonceStore) Issue(tokenHash string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	n := hex.EncodeToString(buf)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	s.byHash[tokenHash] = nonceEntry{nonce: n, expires: time.Now().Add(nonceTTL)}
	return n, nil
}

// Consume validates and deletes the nonce. Constant-time compare.
func (s *NonceStore) Consume(tokenHash, nonce string) bool {
	nonce = hexNormalize(nonce)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byHash[tokenHash]
	if !ok || time.Now().After(e.expires) {
		delete(s.byHash, tokenHash)
		return false
	}
	got := []byte(e.nonce)
	want := []byte(nonce)
	if len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
		return false
	}
	delete(s.byHash, tokenHash)
	return true
}

func (s *NonceStore) gcLocked() {
	now := time.Now()
	for k, e := range s.byHash {
		if now.After(e.expires) {
			delete(s.byHash, k)
		}
	}
}

func hexNormalize(s string) string {
	return s
}
