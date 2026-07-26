package kubernetes

import "testing"

func TestServicePortsSkipsOutOfRange(t *testing.T) {
	t.Parallel()
	got := servicePorts([]int{0, -1, 80, 65535, 65536, 70000})
	if len(got) != 2 {
		t.Fatalf("len=%d want 2 (80 and 65535), got %+v", len(got), got)
	}
	if got[0].Port != 80 || got[1].Port != 65535 {
		t.Fatalf("ports=%v", got)
	}
}
