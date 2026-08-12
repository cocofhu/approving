package handlers

import "testing"

func TestStripCSPDirective(t *testing.T) {
	got := stripCSPDirective("default-src 'self'; frame-ancestors 'none'; img-src data:", "frame-ancestors")
	if got != "default-src 'self'; img-src data:" {
		t.Fatalf("got %q", got)
	}
	got = stripCSPDirective("frame-ancestors 'none'", "frame-ancestors")
	if got != "" {
		t.Fatalf("want empty, got %q", got)
	}
	got = stripCSPDirective("default-src 'self'", "frame-ancestors")
	if got != "default-src 'self'" {
		t.Fatalf("got %q", got)
	}
}
