// Package blob stores attachment bytes outside the database and exposes them
// via opaque blob:{id} references.
package blob

import (
	"fmt"
	"strings"
)

const scheme = "blob:"

// Ref is an opaque attachment reference (blob:{id}).
type Ref string

// ParseRef validates and returns a Ref. Only the blob: scheme is accepted.
func ParseRef(s string) (Ref, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty blob ref")
	}
	if !strings.HasPrefix(s, scheme) {
		return "", fmt.Errorf("unsupported blob scheme: %q", s)
	}
	id := strings.TrimPrefix(s, scheme)
	if id == "" || strings.ContainsAny(id, "/\\..") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid blob id")
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return "", fmt.Errorf("invalid blob id character")
		}
	}
	return Ref(scheme + id), nil
}

// ID returns the id portion without the blob: prefix.
func (r Ref) ID() string {
	return strings.TrimPrefix(string(r), scheme)
}

// String returns the canonical blob:{id} form.
func (r Ref) String() string { return string(r) }

// MakeRef builds a Ref from a raw id (already validated by the store).
func MakeRef(id string) Ref { return Ref(scheme + id) }
