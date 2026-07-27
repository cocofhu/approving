package models

import "testing"

func TestNormalizeRunTags(t *testing.T) {
	got, err := NormalizeRunTags([]string{" bugfix ", "spike", "bugfix", "中文-tag"})
	if err != nil {
		t.Fatalf("NormalizeRunTags: %v", err)
	}
	want := []string{"bugfix", "spike", "中文-tag"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d = %q want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizeRunTagsRejectsInvalid(t *testing.T) {
	cases := [][]string{
		{"bad tag"},
		{"bad@tag"},
		{"abcdefghijklmnopqrstuvwxyz1234567"},
		{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
	}
	for _, input := range cases {
		if _, err := NormalizeRunTags(input); err == nil {
			t.Fatalf("NormalizeRunTags(%v): want error", input)
		}
	}
}
