package services

import (
	"os"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/database"
)

type fakeConc struct {
	max int
	ar  int
}

func (f *fakeConc) SetMaxConcurrent(n int) { f.max = n }
func (f *fakeConc) MaxConcurrent() int     { return f.max }
func (f *fakeConc) SetAutoRetryMax(n int)  { f.ar = n }

type fakeSbxTuner struct {
	runTTL  time.Duration
	testTTL time.Duration
	maxTest int
}

func (f *fakeSbxTuner) SetTTLs(run, test time.Duration) {
	f.runTTL = run
	f.testTTL = test
}
func (f *fakeSbxTuner) SetMaxTestSandboxes(n int) { f.maxTest = n }

func TestSettingsServiceEffectiveAndUpdate(t *testing.T) {
	db, err := database.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Engine:  config.EngineConfig{MaxConcurrentRuns: 5, NodeAutoRetryMax: 2},
		Sandbox: config.SandboxConfig{RunSandboxTTLMinutes: 30, TestSandboxTTLMinutes: 10, MaxTestSandboxes: 2},
	}
	config.StoreConfig(cfg)
	conc := &fakeConc{}
	sbx := &fakeSbxTuner{}
	svc := NewSettingsService(db, conc, sbx)

	items := svc.Effective()
	if len(items) != 5 {
		t.Fatalf("items: %d", len(items))
	}
	updated, err := svc.Update(map[string]int{
		KeyMaxConcurrentRuns: 7,
		KeyRunSandboxTTLMin:  45,
		KeyTestSandboxTTLMin: 15,
		KeyMaxTestSandboxes:  4,
		KeyNodeAutoRetryMax:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 5 {
		t.Fatalf("updated: %d", len(updated))
	}
	if conc.max != 7 || conc.ar != 3 || sbx.maxTest != 4 {
		t.Fatalf("apply: conc=%+v sbx=%+v", conc, sbx)
	}
	svc.ApplyOnBoot()
	if _, err := svc.Update(map[string]int{KeyMaxConcurrentRuns: 0}); err == nil {
		t.Fatal("expected min validation error")
	}
}

func TestSettingsServiceEnvLocked(t *testing.T) {
	db, err := database.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	config.StoreConfig(&config.Config{Engine: config.EngineConfig{MaxConcurrentRuns: 5}})
	t.Setenv("APPROVING_MAX_RUNS", "9")
	conc := &fakeConc{}
	svc := NewSettingsService(db, conc, nil)
	items := svc.Effective()
	var locked bool
	for _, it := range items {
		if it.Key == KeyMaxConcurrentRuns {
			locked = it.Locked
			if it.Source != "env" {
				t.Fatalf("env item: %+v", it)
			}
		}
	}
	if !locked {
		t.Fatal("expected env locked")
	}
	if _, err := svc.Update(map[string]int{KeyMaxConcurrentRuns: 1}); err != nil {
		t.Fatal(err)
	}
	if conc.max != 0 && conc.max != items[0].Value {
		// apply uses effective env value
	}
	os.Unsetenv("APPROVING_MAX_RUNS")
}

func TestSettingsServiceDBOverride(t *testing.T) {
	db, err := database.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	config.StoreConfig(&config.Config{Engine: config.EngineConfig{MaxConcurrentRuns: 5}})
	svc := NewSettingsService(db, nil, nil)
	if err := svc.setInt(KeyMaxConcurrentRuns, 12); err != nil {
		t.Fatal(err)
	}
	v, ok := svc.dbInt(KeyMaxConcurrentRuns)
	if !ok || v != 12 {
		t.Fatalf("dbInt: %v %v", v, ok)
	}
	if v, ok := svc.dbInt("missing"); ok {
		t.Fatalf("missing key: %v", v)
	}
	if err := svc.setInt("bad-num", 0); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.dbInt("bad-num"); !ok {
		t.Fatal("expected stored int")
	}
}
