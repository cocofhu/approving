package driver

import "testing"

func TestPreviewDirectEnabled(t *testing.T) {
	if PreviewDirectEnabled(nil) || PreviewDirectEnabled(map[string]string{}) {
		t.Fatal("empty env must be off")
	}
	if !PreviewDirectEnabled(map[string]string{EnvPreviewDirect: "1"}) {
		t.Fatal("PREVIEW_DIRECT=1 must be on")
	}
}

func TestAppendPort(t *testing.T) {
	got := AppendPort([]int{80}, 18080)
	if len(got) != 2 || got[1] != 18080 {
		t.Fatalf("append: %v", got)
	}
	if got2 := AppendPort(got, 18080); len(got2) != 2 {
		t.Fatalf("dedupe: %v", got2)
	}
	if got3 := AppendPort(got, 0); len(got3) != 2 {
		t.Fatalf("skip 0: %v", got3)
	}
}

func TestApplyK8sPreviewDirect(t *testing.T) {
	spec := Spec{Ports: []int{8765, 22}, Env: map[string]string{EnvPreviewDirect: "1"}}
	ApplyK8sPreviewDirect(&spec)
	if spec.Env[EnvPreviewPort] != "18080" {
		t.Fatalf("PREVIEW_PORT=%q", spec.Env[EnvPreviewPort])
	}
	found := false
	for _, p := range spec.Ports {
		if p == 18080 {
			found = true
		}
	}
	if !found {
		t.Fatalf("ports missing 18080: %v", spec.Ports)
	}
	ApplyK8sPreviewDirect(&spec)
	n := 0
	for _, p := range spec.Ports {
		if p == 18080 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("18080 duplicated: %v", spec.Ports)
	}

	off := Spec{Env: map[string]string{}}
	ApplyK8sPreviewDirect(&off)
	if off.Env[EnvPreviewPort] != "" {
		t.Fatal("off must not set PREVIEW_PORT")
	}

	ApplyK8sPreviewDirect(nil)

	custom := Spec{Ports: []int{80}, Env: map[string]string{EnvPreviewDirect: "1", EnvPreviewPort: "19000"}}
	ApplyK8sPreviewDirect(&custom)
	if custom.Env[EnvPreviewPort] != "19000" {
		t.Fatalf("custom PREVIEW_PORT=%q", custom.Env[EnvPreviewPort])
	}
	bad := Spec{Env: map[string]string{EnvPreviewDirect: "1", EnvPreviewPort: "nope"}}
	ApplyK8sPreviewDirect(&bad)
	if bad.Env[EnvPreviewPort] != "18080" {
		t.Fatalf("invalid PREVIEW_PORT should fall back, got %q", bad.Env[EnvPreviewPort])
	}
	if AppendPort(nil, 70000) != nil {
		t.Fatal("port > 65535 must be skipped")
	}
}
