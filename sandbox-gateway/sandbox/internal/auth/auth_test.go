package auth

import (
	"testing"
	"time"
)

func TestPasswordMatch(t *testing.T) {
	hash, err := HashPassword("toor")
	if err != nil {
		t.Fatal(err)
	}
	if !PasswordMatch(hash, "toor") {
		t.Fatal("equal should match")
	}
	if PasswordMatch(hash, "root") {
		t.Fatal("different should not match")
	}
	if PasswordMatch(nil, "x") {
		t.Fatal("empty hash vs non-empty")
	}
	if PasswordMatch([]byte(""), "x") {
		t.Fatal("empty hash vs non-empty")
	}
}

func TestSessionStore(t *testing.T) {
	s := NewStore(200 * time.Millisecond)
	tok, err := s.Issue()
	if err != nil || tok == "" {
		t.Fatalf("Issue: %v %q", err, tok)
	}
	if !s.Valid(tok) {
		t.Fatal("fresh token should be valid")
	}
	if s.Valid("deadbeef") {
		t.Fatal("unknown token")
	}
	s.Revoke(tok)
	if s.Valid(tok) {
		t.Fatal("revoked token still valid")
	}

	tok2, err := s.Issue()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if s.Valid(tok2) {
		t.Fatal("expired token still valid")
	}
}
