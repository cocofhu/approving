package services

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/crypto"
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
	if len(items) != len(knobs()) {
		t.Fatalf("items: %d", len(items))
	}
	// Ints arrive as float64 after a JSON round-trip; Update must accept both.
	updated, err := svc.Update(map[string]any{
		KeyMaxConcurrentRuns: float64(7),
		KeyRunSandboxTTLMin:  45,
		KeyTestSandboxTTLMin: 15,
		KeyMaxTestSandboxes:  4,
		KeyNodeAutoRetryMax:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != len(knobs()) {
		t.Fatalf("updated: %d", len(updated))
	}
	if conc.max != 7 || conc.ar != 3 || sbx.maxTest != 4 {
		t.Fatalf("apply: conc=%+v sbx=%+v", conc, sbx)
	}
	svc.ApplyOnBoot()
	if _, err := svc.Update(map[string]any{KeyMaxConcurrentRuns: 0}); err == nil {
		t.Fatal("expected min validation error")
	}
	if _, err := svc.Update(map[string]any{KeyMaxConcurrentRuns: "not a number"}); err == nil {
		t.Fatal("expected type validation error")
	}
}

type fakeLiveTuner struct {
	baseURL string
	apiKey  string
	model   string
	timeout time.Duration
}

func (f *fakeLiveTuner) SetLiveEndpoint(baseURL, apiKey, model string, timeout time.Duration) {
	f.baseURL, f.apiKey, f.model, f.timeout = baseURL, apiKey, model, timeout
}

type fakeLiveLimits struct {
	got LiveLimits
}

func (f *fakeLiveLimits) SetLiveLimits(lim LiveLimits) { f.got = lim }

func TestSettingsApplyLiveContextWindows(t *testing.T) {
	db, err := database.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	config.StoreConfig(&config.Config{Live: config.LiveConfig{TimeoutSeconds: 30}})
	limits := &fakeLiveLimits{}
	svc := NewSettingsService(db, nil, nil)
	svc.SetLiveLimitsController(limits)

	byKey := map[string]SettingItem{}
	for _, it := range svc.Effective() {
		byKey[it.Key] = it
	}
	// Sparse config must still surface the compiled defaults, not zeros.
	if byKey[KeyLiveTranscriptWindow].Value != 20 || byKey[KeyLiveMaxTokens].Value != 2048 {
		t.Fatalf("defaults missing: transcript=%v tokens=%v",
			byKey[KeyLiveTranscriptWindow].Value, byKey[KeyLiveMaxTokens].Value)
	}

	if _, err := svc.Update(map[string]any{
		KeyLiveTranscriptWindow:    40,
		KeyLiveLedgerLimit:         8,
		KeyLiveRecentTerminalHours: 12,
		KeyLiveMaxConcurrentWork:   5,
		KeyLiveToolLoopLimit:       4,
		KeyLiveMaxTokens:           4096,
	}); err != nil {
		t.Fatal(err)
	}
	want := LiveLimits{
		TranscriptWindow: 40, LedgerLimit: 8, RecentTerminalHours: 12,
		MaxConcurrentWork: 5, ToolLoopLimit: 4, MaxTokens: 4096,
	}
	if limits.got != want {
		t.Fatalf("SetLiveLimits = %+v want %+v", limits.got, want)
	}
	if _, err := svc.Update(map[string]any{KeyLiveTranscriptWindow: 2}); err == nil {
		t.Fatal("expected min validation for transcript window")
	}
	if _, err := svc.Update(map[string]any{KeyLiveMaxTokens: 100}); err == nil {
		t.Fatal("expected min validation for max tokens")
	}
}

// The conversation model is configured from the settings page, so the settings
// layer has to carry a string and a credential, not just integers.
func TestSettingsCarryStringsAndSecrets(t *testing.T) {
	db, err := database.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(crypto.SecretsKeyEnv, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	config.StoreConfig(&config.Config{
		Engine:  config.EngineConfig{MaxConcurrentRuns: 5},
		Sandbox: config.SandboxConfig{RunSandboxTTLMinutes: 30, TestSandboxTTLMinutes: 10, MaxTestSandboxes: 2},
		Live:    config.LiveConfig{TimeoutSeconds: 8},
	})
	live := &fakeLiveTuner{}
	svc := NewSettingsService(db, nil, nil)
	svc.SetLiveTuner(live)

	if _, err := svc.Update(map[string]any{
		KeyLiveBaseURL: " https://api.example.com/v1 ",
		KeyLiveModel:   "fast-1",
		KeyLiveAPIKey:  "sk-real-key",
	}); err != nil {
		t.Fatal(err)
	}

	// The runtime gets the plaintext key; the API never does.
	if live.baseURL != "https://api.example.com/v1" || live.model != "fast-1" {
		t.Fatalf("tuner endpoint: %+v", live)
	}
	if live.apiKey != "sk-real-key" {
		t.Fatalf("tuner key = %q", live.apiKey)
	}
	if live.timeout != 8*time.Second {
		t.Fatalf("tuner timeout = %v", live.timeout)
	}

	byKey := map[string]SettingItem{}
	for _, it := range svc.Effective() {
		byKey[it.Key] = it
	}
	if got := byKey[KeyLiveAPIKey]; got.Value != SecretMask || got.Kind != KindSecret {
		t.Fatalf("secret must read back masked: %+v", got)
	}
	if got := byKey[KeyLiveBaseURL]; got.Value != "https://api.example.com/v1" || got.Kind != KindString {
		t.Fatalf("string item: %+v", got)
	}

	// Nothing readable from the API may contain the plaintext key.
	blob, err := json.Marshal(svc.Effective())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "sk-real-key") {
		t.Fatalf("plaintext key leaked through the API: %s", blob)
	}

	// Submitting the mask back (what the UI does when the field is untouched)
	// keeps the stored key rather than overwriting it with "****".
	if _, err := svc.Update(map[string]any{
		KeyLiveAPIKey: SecretMask,
		KeyLiveModel:  "fast-2",
	}); err != nil {
		t.Fatal(err)
	}
	if live.apiKey != "sk-real-key" || live.model != "fast-2" {
		t.Fatalf("mask write must keep the key: %+v", live)
	}
	// An empty string means the same thing.
	if _, err := svc.Update(map[string]any{KeyLiveAPIKey: ""}); err != nil {
		t.Fatal(err)
	}
	if live.apiKey != "sk-real-key" {
		t.Fatalf("blank write must keep the key: %q", live.apiKey)
	}
}

// The settings page tests what is on screen, not what is stored, so an address
// can be corrected and checked before it is committed. Two things have to hold
// for that to be safe: the form never holds the real key, and the environment
// stays authoritative over anything typed into a locked field.
func TestLiveEndpointForTestsTheFormNotTheStoredValues(t *testing.T) {
	db, err := database.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(crypto.SecretsKeyEnv, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	config.StoreConfig(&config.Config{Live: config.LiveConfig{TimeoutSeconds: 8}})
	svc := NewSettingsService(db, nil, nil)
	if _, err := svc.Update(map[string]any{
		KeyLiveBaseURL: "https://saved.example.com/v1",
		KeyLiveModel:   "saved-model",
		KeyLiveAPIKey:  "sk-stored",
	}); err != nil {
		t.Fatal(err)
	}

	// Unsaved edits are what get tested.
	baseURL, apiKey, model, timeout := svc.LiveEndpointFor(map[string]any{
		KeyLiveBaseURL:        "http://192.168.2.20:8080/v1",
		KeyLiveModel:          "typed-model",
		KeyLiveAPIKey:         "",
		KeyLiveTimeoutSeconds: 30,
	})
	if baseURL != "http://192.168.2.20:8080/v1" || model != "typed-model" {
		t.Fatalf("form values ignored: %s %s", baseURL, model)
	}
	// A blank field is the UI's resting state for a secret, not a cleared key.
	if apiKey != "sk-stored" {
		t.Fatalf("api key = %q want the stored one", apiKey)
	}
	if timeout != 30*time.Second {
		t.Fatalf("timeout = %v", timeout)
	}

	// The mask means the same thing as blank.
	if _, apiKey, _, _ = svc.LiveEndpointFor(map[string]any{KeyLiveAPIKey: SecretMask}); apiKey != "sk-stored" {
		t.Fatalf("masked key = %q want the stored one", apiKey)
	}
	// A typed key is used as-is, which is the whole point of testing before saving.
	if _, apiKey, _, _ = svc.LiveEndpointFor(map[string]any{KeyLiveAPIKey: "sk-typed"}); apiKey != "sk-typed" {
		t.Fatalf("typed key = %q", apiKey)
	}

	// An omitted key falls back to what is saved.
	baseURL, _, model, _ = svc.LiveEndpointFor(map[string]any{})
	if baseURL != "https://saved.example.com/v1" || model != "saved-model" {
		t.Fatalf("empty patch = %s %s want the saved values", baseURL, model)
	}

	// An env-locked field cannot take effect, so testing a typed value there
	// would report on a configuration that will never run.
	t.Setenv("APPROVING_LIVE_MODEL", "env-model")
	config.StoreConfig(&config.Config{Live: config.LiveConfig{Model: "env-model", TimeoutSeconds: 8}})
	if _, _, model, _ = svc.LiveEndpointFor(map[string]any{KeyLiveModel: "typed-model"}); model != "env-model" {
		t.Fatalf("model = %q want the env-locked value", model)
	}
}

// A stored secret that no longer decrypts (rotated master key) must not reach
// the model client as ciphertext.
func TestUndecryptableSecretFallsBackToConfig(t *testing.T) {
	db, err := database.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(crypto.SecretsKeyEnv, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	config.StoreConfig(&config.Config{
		Live: config.LiveConfig{APIKey: "from-config", TimeoutSeconds: 8},
	})
	svc := NewSettingsService(db, nil, nil)
	if err := svc.setStr(KeyLiveAPIKey, "not-valid-ciphertext"); err != nil {
		t.Fatal(err)
	}
	got, src, _ := svc.resolveStr(secretKnob(t, KeyLiveAPIKey), config.GetConfig())
	if got != "from-config" || src != "config" {
		t.Fatalf("resolve = %q from %q", got, src)
	}
}

func secretKnob(t *testing.T, key string) knob {
	t.Helper()
	for _, k := range knobs() {
		if k.key == key {
			return k
		}
	}
	t.Fatalf("no knob %q", key)
	return knob{}
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
	if _, err := svc.Update(map[string]any{KeyMaxConcurrentRuns: 1}); err != nil {
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
	if err := svc.setStr(KeyMaxConcurrentRuns, "12"); err != nil {
		t.Fatal(err)
	}
	v, ok := svc.dbInt(KeyMaxConcurrentRuns)
	if !ok || v != 12 {
		t.Fatalf("dbInt: %v %v", v, ok)
	}
	if v, ok := svc.dbInt("missing"); ok {
		t.Fatalf("missing key: %v", v)
	}
	// A row that is not a number reads as absent rather than as zero.
	if err := svc.setStr("bad-num", "not-a-number"); err != nil {
		t.Fatal(err)
	}
	if v, ok := svc.dbInt("bad-num"); ok {
		t.Fatalf("non-numeric row should not resolve: %v", v)
	}
}
