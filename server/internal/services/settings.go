package services

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/crypto"
	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Setting keys (the fixed set of UI-editable platform knobs).
const (
	KeyMaxConcurrentRuns = "max_concurrent_runs"
	KeyRunSandboxTTLMin  = "run_sandbox_ttl_minutes"
	KeyTestSandboxTTLMin = "test_sandbox_ttl_minutes"
	KeyMaxTestSandboxes  = "max_test_sandboxes"
	KeyNodeAutoRetryMax  = "node_auto_retry_max"

	KeyLiveEnabled        = "live_enabled"
	KeyLiveBaseURL        = "live_base_url"
	KeyLiveModel          = "live_model"
	KeyLiveAPIKey         = "live_api_key"
	KeyLiveTimeoutSeconds = "live_timeout_seconds"
)

// Knob value kinds. Ints render as steppers, strings as text inputs, secrets as
// password inputs that read back masked and treat a blank/mask write as "keep".
const (
	KindInt    = "int"
	KindString = "string"
	KindSecret = "secret"
)

// ConcurrencyController is the slice of the engine the settings layer drives:
// changing the live max_concurrent_runs. Defined here (consumer side) so the
// engine need not import services.
type ConcurrencyController interface {
	SetMaxConcurrent(n int)
	MaxConcurrent() int
}

// AutoRetryController is the optional slice of the engine that accepts the live
// node auto-retry cap. The concrete engine implements it; the settings layer
// type-asserts its ConcurrencyController to this so no constructor signature
// changes and test fakes that don't implement it are simply skipped.
type AutoRetryController interface {
	SetAutoRetryMax(n int)
}

// SandboxTuner is the slice of the sandbox service the settings layer drives.
type SandboxTuner interface {
	SetTTLs(runTTL, testTTL time.Duration)
	SetMaxTestSandboxes(n int)
}

// LiveTuner receives the effective conversation-model endpoint. Implemented by
// the liveagent client so a settings-page edit takes effect without a restart.
// The plaintext key is passed through here and never leaves this path.
type LiveTuner interface {
	SetLiveEndpoint(baseURL, apiKey, model string, timeout time.Duration)
}

// SettingsService is the DB override layer for platform scheduling params. It
// resolves effective values (env > DB > config-file > default), persists UI
// edits, and applies them to the running engine / sandbox service.
type SettingsService struct {
	db   *gorm.DB
	conc ConcurrencyController
	sbx  SandboxTuner
	live LiveTuner
}

func NewSettingsService(db *gorm.DB, conc ConcurrencyController, sbx SandboxTuner) *SettingsService {
	return &SettingsService{db: db, conc: conc, sbx: sbx}
}

// SetLiveTuner wires the conversation-model client. Separate from the
// constructor so existing callers and test fakes need no change.
func (s *SettingsService) SetLiveTuner(t LiveTuner) { s.live = t }

// knob describes one tunable: its kind, label, floor, optional env lock, and
// how to read the config-file/default fallback. Exactly one of fromCfg (int)
// and fromCfgStr (string/secret) is set, matching kind.
type knob struct {
	key        string
	label      string
	unit       string
	kind       string
	min        int    // int only
	envVar     string // "" when there is no env override for this knob
	fromCfg    func(*config.Config) int
	fromCfgStr func(*config.Config) string
}

func knobs() []knob {
	return []knob{
		{key: KeyMaxConcurrentRuns, label: "最大并发运行数", kind: KindInt, min: 1,
			envVar:  "APPROVING_MAX_RUNS",
			fromCfg: func(c *config.Config) int { return c.Engine.MaxConcurrentRuns }},
		{key: KeyRunSandboxTTLMin, label: "运行沙箱保留时长", unit: "分钟", kind: KindInt, min: 1,
			fromCfg: func(c *config.Config) int { return c.Sandbox.RunSandboxTTLMinutes }},
		{key: KeyTestSandboxTTLMin, label: "测试沙箱空闲 TTL", unit: "分钟", kind: KindInt, min: 1,
			fromCfg: func(c *config.Config) int { return c.Sandbox.TestSandboxTTLMinutes }},
		{key: KeyMaxTestSandboxes, label: "最大测试沙箱数", kind: KindInt, min: 1,
			fromCfg: func(c *config.Config) int { return c.Sandbox.MaxTestSandboxes }},
		{key: KeyNodeAutoRetryMax, label: "节点自动重试次数", unit: "次", kind: KindInt, min: 0,
			envVar:  "APPROVING_NODE_AUTO_RETRY",
			fromCfg: func(c *config.Config) int { return c.Engine.NodeAutoRetryMax }},

		{key: KeyLiveBaseURL, label: "对话模型接口地址", kind: KindString,
			envVar:     "APPROVING_LIVE_BASE_URL",
			fromCfgStr: func(c *config.Config) string { return c.Live.BaseURL }},
		{key: KeyLiveModel, label: "对话模型名称", kind: KindString,
			envVar:     "APPROVING_LIVE_MODEL",
			fromCfgStr: func(c *config.Config) string { return c.Live.Model }},
		{key: KeyLiveAPIKey, label: "对话模型密钥", kind: KindSecret,
			envVar:     "APPROVING_LIVE_API_KEY",
			fromCfgStr: func(c *config.Config) string { return c.Live.APIKey }},
		{key: KeyLiveTimeoutSeconds, label: "对话模型超时", unit: "秒", kind: KindInt, min: 1,
			envVar:  "APPROVING_LIVE_TIMEOUT_SEC",
			fromCfg: func(c *config.Config) int { return c.Live.TimeoutSeconds }},
	}
}

// SettingItem is the API shape for one effective knob. Value is an int for
// KindInt and a string otherwise; secrets read back as SecretMask, never as
// plaintext.
type SettingItem struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Unit   string `json:"unit,omitempty"`
	Kind   string `json:"kind"`
	Value  any    `json:"value"`
	Min    int    `json:"min"`
	Source string `json:"source"` // env | db | config
	Locked bool   `json:"locked"` // true when pinned by an env var (UI read-only)
}

// Effective returns every knob's current effective value with provenance.
func (s *SettingsService) Effective() []SettingItem {
	cfg := config.GetConfig()
	all := knobs()
	out := make([]SettingItem, 0, len(all))
	for _, k := range all {
		item := SettingItem{
			Key: k.key, Label: k.label, Unit: k.unit, Kind: k.kind, Min: k.min,
		}
		if k.kind == KindInt {
			item.Value, item.Source, item.Locked = s.resolveInt(k, cfg)
		} else {
			v, src, locked := s.resolveStr(k, cfg)
			if k.kind == KindSecret && v != "" {
				v = SecretMask
			}
			item.Value, item.Source, item.Locked = v, src, locked
		}
		out = append(out, item)
	}
	return out
}

// envLocked reports whether the environment pins this knob, making it
// authoritative and the UI read-only.
func (k knob) envLocked() bool {
	return k.envVar != "" && strings.TrimSpace(os.Getenv(k.envVar)) != ""
}

// resolveInt applies env > DB > config(file/default). The config snapshot
// already folds env over file over default, so when a knob is env-locked its
// config value is exactly the env value.
func (s *SettingsService) resolveInt(k knob, cfg *config.Config) (value int, source string, locked bool) {
	cfgVal := k.fromCfg(cfg)
	if k.envLocked() {
		return cfgVal, "env", true
	}
	if v, ok := s.dbInt(k.key); ok {
		return v, "db", false
	}
	return cfgVal, "config", false
}

// resolveStr is resolveInt for string and secret knobs. Secrets come back
// decrypted; masking is the caller's job.
func (s *SettingsService) resolveStr(k knob, cfg *config.Config) (value string, source string, locked bool) {
	cfgVal := strings.TrimSpace(k.fromCfgStr(cfg))
	if k.envLocked() {
		return cfgVal, "env", true
	}
	if v, ok := s.dbStr(k.key, k.kind == KindSecret); ok {
		return v, "db", false
	}
	return cfgVal, "config", false
}

// Update validates and persists the given patch (only keys present are
// changed), skipping env-locked knobs, then applies the new effective values
// to the running components.
//
// Values arrive from JSON, so ints may be float64. A secret whose value is
// blank or SecretMask means "keep what is stored" — that is how the UI can
// submit the whole form without having to re-enter the key.
func (s *SettingsService) Update(patch map[string]any) ([]SettingItem, error) {
	for _, k := range knobs() {
		raw, ok := patch[k.key]
		if !ok {
			continue
		}
		// An env-locked knob is authoritative from the environment; ignore edits.
		if k.envLocked() {
			continue
		}
		switch k.kind {
		case KindInt:
			v, err := coerceInt(raw)
			if err != nil {
				return nil, fmt.Errorf("%s 必须是整数", k.label)
			}
			if v < k.min {
				return nil, fmt.Errorf("%s 不能小于 %d", k.label, k.min)
			}
			if err := s.setStr(k.key, strconv.Itoa(v)); err != nil {
				return nil, err
			}
		case KindString:
			v, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%s 必须是字符串", k.label)
			}
			if err := s.setStr(k.key, strings.TrimSpace(v)); err != nil {
				return nil, err
			}
		case KindSecret:
			v, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%s 必须是字符串", k.label)
			}
			v = strings.TrimSpace(v)
			if isSecretPlaceholder(v) {
				continue // keep the stored key
			}
			enc, err := crypto.Encrypt(v)
			if err != nil {
				return nil, mapEncryptErr(err)
			}
			if err := s.setStr(k.key, enc); err != nil {
				return nil, err
			}
		}
	}
	s.apply()
	return s.Effective(), nil
}

// coerceInt accepts the shapes an int can take after a JSON round-trip.
func coerceInt(raw any) (int, error) {
	switch v := raw.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		return strconv.Atoi(strings.TrimSpace(v))
	default:
		return 0, fmt.Errorf("not an int")
	}
}

// ApplyOnBoot pushes the persisted (DB) overrides onto the runtime components
// at startup, so UI-configured values survive a restart.
func (s *SettingsService) ApplyOnBoot() { s.apply() }

// apply computes the effective values and drives the runtime components.
func (s *SettingsService) apply() {
	cfg := config.GetConfig()
	m := map[string]int{}
	str := map[string]string{}
	for _, k := range knobs() {
		if k.kind == KindInt {
			m[k.key], _, _ = s.resolveInt(k, cfg)
			continue
		}
		// Secrets stay plaintext here: this map never leaves the process.
		str[k.key], _, _ = s.resolveStr(k, cfg)
	}
	if s.live != nil {
		s.live.SetLiveEndpoint(
			str[KeyLiveBaseURL], str[KeyLiveAPIKey], str[KeyLiveModel],
			time.Duration(m[KeyLiveTimeoutSeconds])*time.Second,
		)
	}
	if s.conc != nil {
		s.conc.SetMaxConcurrent(m[KeyMaxConcurrentRuns])
		// The concrete engine also accepts the live node auto-retry cap.
		if ar, ok := s.conc.(AutoRetryController); ok {
			ar.SetAutoRetryMax(m[KeyNodeAutoRetryMax])
		}
	}
	if s.sbx != nil {
		s.sbx.SetTTLs(
			time.Duration(m[KeyRunSandboxTTLMin])*time.Minute,
			time.Duration(m[KeyTestSandboxTTLMin])*time.Minute,
		)
		s.sbx.SetMaxTestSandboxes(m[KeyMaxTestSandboxes])
	}
}

func (s *SettingsService) dbInt(key string) (int, bool) {
	raw, ok := s.dbRaw(key)
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}

// dbStr reads a string knob, decrypting when the knob is a secret. A stored
// secret that will not decrypt (rotated master key, tampered row) is reported
// as absent so resolution falls back to config rather than handing ciphertext
// to the model client.
func (s *SettingsService) dbStr(key string, secret bool) (string, bool) {
	raw, ok := s.dbRaw(key)
	if !ok || raw == "" {
		return "", false
	}
	if !secret {
		return raw, true
	}
	plain, err := crypto.Decrypt(raw)
	if err != nil {
		log.Warn().Str("key", key).Msg("stored secret could not be decrypted; falling back to config")
		return "", false
	}
	return plain, true
}

func (s *SettingsService) dbRaw(key string) (string, bool) {
	var row models.Setting
	if err := s.db.First(&row, "key = ?", key).Error; err != nil {
		return "", false
	}
	return strings.TrimSpace(row.Value), true
}

func (s *SettingsService) setStr(key, v string) error {
	// Key is the primary key, so Save upserts (update when present, else insert).
	return s.db.Save(&models.Setting{Key: key, Value: v, UpdatedAt: time.Now()}).Error
}
