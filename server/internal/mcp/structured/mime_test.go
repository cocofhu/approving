package structured

import "testing"

func TestGuessImageMIME(t *testing.T) {
	cases := map[string]string{
		"a.PNG":  "image/png",
		"b.jpg":  "image/jpeg",
		"c.jpeg": "image/jpeg",
		"d.webp": "image/webp",
		"e.gif":  "image/gif",
		"f.bin":  "image/png",
		"noext":  "image/png",
	}
	for in, want := range cases {
		if got := GuessImageMIME(in); got != want {
			t.Fatalf("%s: %q want %q", in, got, want)
		}
	}
}

func TestTestCountsMore(t *testing.T) {
	// Prefer recounting cases when failed counter is zero.
	raw := `{"summary":"s","failed":0,"cases":[{"name":"a","status":"failed"},{"name":"b","status":"skipped"},{"name":"c","status":"skipped"}]}`
	if TestFailedCount(raw) != 1 {
		t.Fatalf("failed=%d", TestFailedCount(raw))
	}
	if TestSkippedCount(raw) != 2 {
		t.Fatalf("skipped=%d", TestSkippedCount(raw))
	}
}
