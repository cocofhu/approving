package gateshare

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openNonceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "nonce.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.GateShareNonce{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestNonceStoreMultiTabAndConsume(t *testing.T) {
	s := NewNonceStore(openNonceTestDB(t))
	hash := strings.Repeat("ab", 32)
	n1, err := s.Issue(hash)
	if err != nil || n1 == "" {
		t.Fatalf("issue1: %v %s", err, n1)
	}
	n2, err := s.Issue(hash)
	if err != nil || n2 == "" || n2 == n1 {
		t.Fatalf("issue2: %v %s", err, n2)
	}
	if !s.Consume(hash, n1) {
		t.Fatal("first tab nonce should still be valid")
	}
	if !s.Consume(hash, n2) {
		t.Fatal("second tab nonce should still be valid")
	}
	if s.Consume(hash, n1) {
		t.Fatal("consumed nonce must not replay")
	}
}

func TestNonceStoreSharedAcrossInstances(t *testing.T) {
	db := openNonceTestDB(t)
	a := NewNonceStore(db)
	b := NewNonceStore(db)
	hash := strings.Repeat("cd", 32)
	n, err := a.Issue(hash)
	if err != nil || n == "" {
		t.Fatalf("issue: %v %s", err, n)
	}
	if !b.Consume(hash, n) {
		t.Fatal("other instance must consume the same DB nonce")
	}
	if a.Consume(hash, n) {
		t.Fatal("consumed nonce must not replay on issuer instance")
	}
}

func TestNonceStoreCapacityAndTTL(t *testing.T) {
	db := openNonceTestDB(t)
	s := NewNonceStore(db)
	hash := strings.Repeat("ef", 32)
	issued := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		n, err := s.Issue(hash)
		if err != nil || n == "" {
			t.Fatalf("issue %d: %v %s", i, err, n)
		}
		issued = append(issued, n)
	}
	if s.Consume(hash, issued[0]) {
		t.Fatal("oldest beyond cap 4 should be gone")
	}
	for i := 1; i < 5; i++ {
		if !s.Consume(hash, issued[i]) {
			t.Fatalf("nonce %d should still be valid", i)
		}
	}

	n, err := s.Issue(hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.GateShareNonce{}).Where("nonce = ?", n).
		Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if s.Consume(hash, n) {
		t.Fatal("expired nonce must not consume")
	}
}

func TestParsePublicAdvertise(t *testing.T) {
	origin, host := ParsePublicAdvertise("https://app.example:8443/unused")
	if origin != "https://app.example:8443" || host != "app.example:8443" {
		t.Fatalf("got %q %q", origin, host)
	}
	if o, h := ParsePublicAdvertise(""); o != "" || h != "" {
		t.Fatalf("empty: %q %q", o, h)
	}
	if o, h := ParsePublicAdvertise("not a url"); o != "" || h != "" {
		t.Fatalf("invalid: %q %q", o, h)
	}
}
