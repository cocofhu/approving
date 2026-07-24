package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveResourcesDefaultsAndMax(t *testing.T) {
	d := ResourceDefaults{
		DefaultCPUCores: 2,
		DefaultMemoryMB: 4096,
		DefaultDiskGi:   25,
		MaxCPUCores:     8,
		MaxMemoryMB:     16384,
		MaxDiskGi:       500,
	}
	cpu, mem, disk, err := d.Resolve(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cpu != 2 || mem != 4096 || disk != 25 {
		t.Fatalf("defaults: got %v %v %v", cpu, mem, disk)
	}
	if _, _, _, err := d.Resolve(16, 0, 0); err == nil {
		t.Fatal("expected cpu max error")
	}
	if _, _, _, err := d.Resolve(2, 0, 600); err == nil {
		t.Fatal("expected disk max error")
	}
	if _, _, _, err := d.Resolve(2, 999999, 1); err == nil {
		t.Fatal("expected memory max error")
	}
	cpu, mem, disk, err = d.Resolve(4, 8192, 160)
	if err != nil {
		t.Fatal(err)
	}
	if cpu != 4 || mem != 8192 || disk != 160 {
		t.Fatalf("custom: got %v %v %v", cpu, mem, disk)
	}
}

func TestResolveRejectsNegativesAndEmptyDefaults(t *testing.T) {
	d := ResourceDefaults{} // zeros: after fill, values stay <= 0
	if _, _, _, err := d.Resolve(-1, 0, 0); err == nil {
		t.Fatal("negative cpu")
	}
	if _, _, _, err := d.Resolve(0, -1, 0); err == nil {
		t.Fatal("negative memory")
	}
	if _, _, _, err := d.Resolve(0, 0, -1); err == nil {
		t.Fatal("negative disk")
	}
	if _, _, _, err := d.Resolve(0, 0, 0); err == nil {
		t.Fatal("empty defaults must reject zero effective cpu")
	}
	d.DefaultCPUCores = 1
	if _, _, _, err := d.Resolve(0, 0, 0); err == nil {
		t.Fatal("empty memory/disk defaults")
	}
	d.DefaultMemoryMB = 512
	if _, _, _, err := d.Resolve(0, 0, 0); err == nil {
		t.Fatal("empty disk default")
	}
}

func TestSandboxCPURequestFixed(t *testing.T) {
	cfg := &Config{K8s: K8sConfig{CPURequestCores: 0.25}}
	tests := []struct {
		limit, request float64
	}{
		{2, 0.25},
		{4, 0.25},
		{0.2, 0.2},
	}
	for _, tc := range tests {
		if got := cfg.SandboxCPURequest(tc.limit); got != tc.request {
			t.Fatalf("SandboxCPURequest(%v) = %v, want %v", tc.limit, got, tc.request)
		}
	}
}

func TestSandboxCPURequestRatio(t *testing.T) {
	cfg := &Config{K8s: K8sConfig{CPURequestCores: 0, CPURequestRatio: 0.25}}
	tests := []struct {
		limit, request float64
	}{
		{2, 0.5},
		{4, 1},
		{0.2, 0.05}, // 0.2×0.25 = 0.05 (min floor)
	}
	for _, tc := range tests {
		if got := cfg.SandboxCPURequest(tc.limit); got != tc.request {
			t.Fatalf("SandboxCPURequest(%v) = %v, want %v", tc.limit, got, tc.request)
		}
	}
}

func TestSandboxCPURequestEdgeBranches(t *testing.T) {
	if got := (&Config{}).SandboxCPURequest(0); got != 0 {
		t.Fatalf("non-positive limit: %v", got)
	}
	if got := (&Config{}).SandboxCPURequest(-1); got != -1 {
		t.Fatalf("negative limit: %v", got)
	}
	// fixed below min → floor
	cfg := &Config{K8s: K8sConfig{CPURequestCores: 0.01}}
	if got := cfg.SandboxCPURequest(2); got != 0.05 {
		t.Fatalf("min floor: %v", got)
	}
	// ratio defaults when unset; clamp >1; request > limit
	cfg2 := &Config{K8s: K8sConfig{CPURequestCores: 0, CPURequestRatio: 0}}
	if got := cfg2.SandboxCPURequest(2); got != 0.5 {
		t.Fatalf("default ratio: %v", got)
	}
	cfg3 := &Config{K8s: K8sConfig{CPURequestCores: 0, CPURequestRatio: 2}}
	if got := cfg3.SandboxCPURequest(2); got != 2 {
		t.Fatalf("ratio clamp: %v", got)
	}
	// tiny limit × ratio floors to minRequest (0.05); may exceed limit
	cfg4 := &Config{K8s: K8sConfig{CPURequestCores: 0, CPURequestRatio: 1}}
	if got := cfg4.SandboxCPURequest(0.01); got != 0.05 {
		t.Fatalf("min floor on ratio path: %v", got)
	}
}

func TestSandboxMemoryRequestFixed(t *testing.T) {
	cfg := &Config{K8s: K8sConfig{MemoryRequestMB: 512}}
	tests := []struct {
		limit, request int64
	}{
		{4096, 512},
		{2048, 512},
		{256, 256},
	}
	for _, tc := range tests {
		if got := cfg.SandboxMemoryRequest(tc.limit); got != tc.request {
			t.Fatalf("SandboxMemoryRequest(%v) = %v, want %v", tc.limit, got, tc.request)
		}
	}
}

func TestSandboxMemoryRequestRatio(t *testing.T) {
	cfg := &Config{K8s: K8sConfig{MemoryRequestMB: 0, MemoryRequestRatio: 0.25}}
	tests := []struct {
		limit, request int64
	}{
		{4096, 1024},
		{512, 128},
		{64, 128},
	}
	for _, tc := range tests {
		if got := cfg.SandboxMemoryRequest(tc.limit); got != tc.request {
			t.Fatalf("SandboxMemoryRequest(%v) = %v, want %v", tc.limit, got, tc.request)
		}
	}
}

func TestSandboxMemoryRequestEdgeBranches(t *testing.T) {
	if got := (&Config{}).SandboxMemoryRequest(0); got != 0 {
		t.Fatalf("non-positive: %v", got)
	}
	cfg := &Config{K8s: K8sConfig{MemoryRequestMB: 10}}
	if got := cfg.SandboxMemoryRequest(4096); got != 128 {
		t.Fatalf("min floor: %v", got)
	}
	cfg2 := &Config{K8s: K8sConfig{MemoryRequestMB: 0, MemoryRequestRatio: 0}}
	if got := cfg2.SandboxMemoryRequest(4096); got != 1024 {
		t.Fatalf("default ratio: %v", got)
	}
	cfg3 := &Config{K8s: K8sConfig{MemoryRequestMB: 0, MemoryRequestRatio: 2}}
	if got := cfg3.SandboxMemoryRequest(4096); got != 4096 {
		t.Fatalf("ratio clamp: %v", got)
	}
	// tiny limit × ratio floors to minRequest (128)
	cfg4 := &Config{K8s: K8sConfig{MemoryRequestMB: 0, MemoryRequestRatio: 1}}
	if got := cfg4.SandboxMemoryRequest(64); got != 128 {
		t.Fatalf("min floor on ratio path: %v", got)
	}
}

func TestNormalizeResourcePolicyEmptyFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	yaml := `
driver: docker
image:
  ref: x
database:
  driver: "  "
kubernetes:
  storageClass: ""
  maxCPUCores: 0
  maxMemoryMB: 0
  maxDataDiskGi: 0
  cpuRequestRatio: 0
  memoryRequestRatio: 0
  cpuRequestCores: 0
  memoryRequestMB: 0
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("db driver=%q", cfg.Database.Driver)
	}
	if cfg.K8s.StorageClass != "ugreen-iscsi" {
		t.Fatalf("storageClass=%q", cfg.K8s.StorageClass)
	}
	if cfg.K8s.MaxCPUCores != 8 || cfg.K8s.MaxMemoryMB != 16384 || cfg.K8s.MaxDataDiskGi != 500 {
		t.Fatalf("max defaults: %+v", cfg.K8s)
	}
	if cfg.K8s.CPURequestRatio != 0.25 || cfg.K8s.MemoryRequestRatio != 0.25 {
		t.Fatalf("ratios: %+v", cfg.K8s)
	}
	if cfg.K8s.CPURequestCores != 0.25 || cfg.K8s.MemoryRequestMB != 512 {
		t.Fatalf("fixed requests: %+v", cfg.K8s)
	}
}
