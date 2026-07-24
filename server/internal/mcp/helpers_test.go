package mcp

import "testing"

func TestAsIntBoolHelpers(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{float64(3), 3},
		{int(4), 4},
		{"6", 6},
		{"x", 0},
		{true, 0},
		{nil, 0},
		{int64(5), 0}, // not handled → 0
	}
	for _, tc := range cases {
		if got := asInt(tc.in); got != tc.want {
			t.Fatalf("asInt(%v)=%d want %d", tc.in, got, tc.want)
		}
	}
	if !asBool(true) || !asBool("true") || !asBool("1") {
		t.Fatal("asBool true cases")
	}
	if asBool(false) || asBool("no") || asBool(0) || asBool(nil) || asBool(float64(1)) {
		t.Fatal("asBool false cases")
	}
	if firstNonEmpty("", "b") != "b" || firstNonEmpty("a", "b") != "a" {
		t.Fatal("firstNonEmpty")
	}
}
