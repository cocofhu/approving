package gateshare

import "testing"

func TestNonceStoreMultiTabAndConsume(t *testing.T) {
	s := NewNonceStore()
	hash := "abc" + string(make([]byte, 61))
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
