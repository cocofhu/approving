package correl

import "testing"

func TestID(t *testing.T) {
	a, b := ID(), ID()
	if len(a) != 8 || len(b) != 8 {
		t.Fatalf("len a=%s b=%s", a, b)
	}
	if a == b {
		// extremely unlikely; retry once
		c := ID()
		if a == c {
			t.Fatal("IDs not random enough")
		}
	}
}
