package gateshare

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// TokenByteLen is 256-bit CSPRNG entropy.
	TokenByteLen = 32
	// TokenHexLen is hex(32 bytes).
	TokenHexLen = TokenByteLen * 2
	// HashHexLen is hex(SHA-256).
	HashHexLen = sha256.Size * 2
)

// GenerateToken returns a 256-bit hex token. Never persist the return value.
func GenerateToken() (string, error) {
	buf := make([]byte, TokenByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("csprng: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// HashToken returns SHA-256 hex of a plaintext token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

// EqualHash is a constant-time compare of two hex hashes.
func EqualHash(stored, computed string) bool {
	a := []byte(strings.TrimSpace(stored))
	b := []byte(strings.TrimSpace(computed))
	if len(a) != HashHexLen || len(b) != HashHexLen {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

// ValidTokenShape reports whether s looks like a 256-bit hex token.
func ValidTokenShape(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != TokenHexLen {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
