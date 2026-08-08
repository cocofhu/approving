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

	// Conversation-layer context windows. These used to be compiled constants;
	// exposing them lets an operator trade prompt size for recall without a
	// redeploy. Defaults match the previous hard-coded values.
	KeyLiveTranscriptWindow    = "live_transcript_window"
	KeyLiveLedgerLimit         = "live_ledger_limit"
	KeyLiveRecentTerminalHours = "live_recent_terminal_hours"
	KeyLiveMaxConcurrentWork   = "live_max_concurrent_work"
	KeyLiveToolLoopLimit       = "live_tool_loop_limit"
	KeyLiveMaxTokens           = "live_max_tokens"
	// KeyRunHeartbeatMinutes is how long a task may run without the platform
	// volunteering an update. 0 switches those updates off entirely.
	KeyRunHeartbeatMinutes = "run_heartbeat_minutes"

	// The editable half of the conversation model's instructions. These hold
	// the prompt *body* only: the persona that says who the model is and that
	// its output goes out verbatim is re-attached by the runtime and cannot be
	// edited away. Blank means the built-in body.
	KeyLiveSystemPromptBody    = "live_system_prompt_body"
	KeyLiveSynthesisPromptBody = "live_synthesis_prompt_body"
	// KeyLiveTemperature is blank when the endpoint's own default should
	// stand. It is stored as text rather than a number for exactly that
	// reason: 0 is a real setting, so "unset" needs to look different.
	KeyLiveTemperature = "live_temperature"
)

// Knob value kinds. Ints render as steppers, strings as text inputs, secrets as
// password inputs that read back masked and treat a blank/mask write as "keep",
// text as a multi-line box, floats as a decimal input that may be left blank.
const (
	KindInt    = "int"
	KindString = "string"
	KindSecret = "secret"
	KindText   = "text"
	KindFloat  = "float"
)

// maxLiveTemperature is the ceiling every OpenAI-compatible endpoint accepts.
// Above it the request is rejected at the endpoint, which surfaces as "the
// assistant went quiet" rather than as a settings error.
const maxLiveTemperature = 2.0

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

// LiveLimits is one snapshot of the conversation layer's tunables.
type LiveLimits struct {
	TranscriptWindow    int
	LedgerLimit         int
	RecentTerminalHours int
	MaxConcurrentWork   int
	ToolLoopLimit       int
	MaxTokens           int
	// RunHeartbeat is the minimum gap between volunteered updates about one
	// long-running task. Zero switches them off.
	RunHeartbeat time.Duration
}

// LiveLimitsController receives the effective conversation-layer windows.
// Implemented by channels.Manager so a settings edit takes effect on the next
// turn without a restart.
type LiveLimitsController interface {
	SetLiveLimits(LiveLimits)
}

// LivePrompts is the editable half of what the conversation model is told.
//
// The bodies are bodies, not whole prompts. The persona in front of them is
// assembled at call time and is not represented here, so there is no shape an
// operator can put in this struct that drops it.
type LivePrompts struct {
	// SystemBody replaces the routing prompt's body. Blank keeps the built-in.
	SystemBody string
	// SynthesisBody replaces the outcome-reporting body. Blank keeps the
	// built-in.
	SynthesisBody string
	// Temperature is nil when the endpoint's own default should stand.
	Temperature *float64
}

// LivePromptController receives the effective prompt bodies and temperature.
// Implemented at the composition root, which fans them out to the routing
// layer, the reporting layer and the model client.
type LivePromptController interface {
	SetLivePrompts(LivePrompts)
}

// LivePromptDefaults describes the built-in prompts to the settings UI, so a
// blank field can show what it is falling back to and what the immutable lead
// looks like. Injected rather than imported: the prompts live in the channels
// package, which depends on this one.
type LivePromptDefaults struct {
	// Prefix is the persona the runtime always puts in front of a body.
	Prefix string
	// SystemBody / SynthesisBody are the built-in bodies.
	SystemBody    string
	SynthesisBody string
}

// SettingsService is the DB override layer for platform scheduling params. It
// resolves effective values (env > DB > config-file > default), persists UI
// edits, and applies them to the running engine / sandbox service.
type SettingsService struct {
	db          *gorm.DB
	conc        ConcurrencyController
	sbx         SandboxTuner
	live        LiveTuner
	liveLimits  LiveLimitsController
	livePrompts LivePromptController
	promptHints LivePromptDefaults
}

func NewSettingsService(db *gorm.DB, conc ConcurrencyController, sbx SandboxTuner) *SettingsService {
	return &SettingsService{db: db, conc: conc, sbx: sbx}
}

// SetLiveTuner wires the conversation-model client. Separate from the
// constructor so existing callers and test fakes need no change.
func (s *SettingsService) SetLiveTuner(t LiveTuner) { s.live = t }

// SetLiveLimitsController wires the conversation-layer window knobs. Called
// after the channel manager exists, because settings boot before channels.
func (s *SettingsService) SetLiveLimitsController(c LiveLimitsController) { s.liveLimits = c }

// SetLivePromptController wires the prompt/temperature fan-out, and tells the
// settings page what the built-in prompts are so a blank field can preview the
// text it falls back to.
func (s *SettingsService) SetLivePromptController(c LivePromptController, d LivePromptDefaults) {
	s.livePrompts, s.promptHints = c, d
}

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
		{key: KeyLiveTranscriptWindow, label: "对话历史条数", unit: "条", kind: KindInt, min: 4,
			envVar:  "APPROVING_LIVE_TRANSCRIPT_WINDOW",
			fromCfg: func(c *config.Config) int { return liveIntOr(c.Live.TranscriptWindow, 20) }},
		{key: KeyLiveLedgerLimit, label: "台账任务条数", unit: "条", kind: KindInt, min: 1,
			envVar:  "APPROVING_LIVE_LEDGER_LIMIT",
			fromCfg: func(c *config.Config) int { return liveIntOr(c.Live.LedgerLimit, 5) }},
		{key: KeyLiveRecentTerminalHours, label: "终态任务回看", unit: "小时", kind: KindInt, min: 1,
			envVar:  "APPROVING_LIVE_RECENT_TERMINAL_HOURS",
			fromCfg: func(c *config.Config) int { return liveIntOr(c.Live.RecentTerminalHours, 24) }},
		{key: KeyLiveMaxConcurrentWork, label: "会话并发任务", unit: "个", kind: KindInt, min: 1,
			envVar:  "APPROVING_LIVE_MAX_CONCURRENT_WORK",
			fromCfg: func(c *config.Config) int { return liveIntOr(c.Live.MaxConcurrentWork, 3) }},
		{key: KeyLiveToolLoopLimit, label: "单轮工具循环", unit: "次", kind: KindInt, min: 1,
			envVar:  "APPROVING_LIVE_TOOL_LOOP_LIMIT",
			fromCfg: func(c *config.Config) int { return liveIntOr(c.Live.ToolLoopLimit, 3) }},
		{key: KeyLiveMaxTokens, label: "单次回复上限", unit: "token", kind: KindInt, min: 256,
			envVar:  "APPROVING_LIVE_MAX_TOKENS",
			fromCfg: func(c *config.Config) int { return liveIntOr(c.Live.MaxTokens, 2048) }},
		// min 0 on purpose: 0 is the off switch. Everything else here has a
		// floor because a too-small value degrades quietly, but a project that
		// genuinely does not want unprompted updates needs a way to say so.
		{key: KeyRunHeartbeatMinutes, label: "长跑任务主动汇报间隔", unit: "分钟（0=不汇报）", kind: KindInt, min: 0,
			envVar: "APPROVING_RUN_HEARTBEAT_MINUTES",
			fromCfg: func(c *config.Config) int {
				if c.Live.RunHeartbeatMinutes != nil {
					return *c.Live.RunHeartbeatMinutes
				}
				return 30
			}},

		{key: KeyLiveSystemPromptBody, label: "对话模型指令正文", kind: KindText,
			envVar:     "APPROVING_LIVE_SYSTEM_PROMPT_BODY",
			fromCfgStr: func(c *config.Config) string { return c.Live.SystemPromptBody }},
		{key: KeyLiveSynthesisPromptBody, label: "汇报措辞指令正文", kind: KindText,
			envVar:     "APPROVING_LIVE_SYNTHESIS_PROMPT_BODY",
			fromCfgStr: func(c *config.Config) string { return c.Live.SynthesisPromptBody }},
		{key: KeyLiveTemperature, label: "对话模型 temperature", unit: "留空=按端点默认", kind: KindFloat,
			envVar:     "APPROVING_LIVE_TEMPERATURE",
			fromCfgStr: func(c *config.Config) string { return c.Live.Temperature }},
	}
}

// liveIntOr keeps a sparse LiveConfig from reading as zero in the settings UI.
func liveIntOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
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
	// Prefix is text the runtime always puts in front of this value and that
	// no edit can remove. Set only for prompt bodies; the UI shows it read-only
	// above the editor so nobody writes a persona line that gets duplicated.
	Prefix string `json:"prefix,omitempty"`
	// Preview is what a blank value falls back to. Without it "留空即用默认"
	// is a promise the user has no way to inspect.
	Preview string `json:"preview,omitempty"`
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
		s.describeDefaults(&item)
		out = append(out, item)
	}
	return out
}

// describeDefaults attaches the immutable prefix and the built-in body a blank
// prompt field falls back to.
func (s *SettingsService) describeDefaults(item *SettingItem) {
	switch item.Key {
	case KeyLiveSystemPromptBody:
		item.Prefix, item.Preview = s.promptHints.Prefix, s.promptHints.SystemBody
	case KeyLiveSynthesisPromptBody:
		item.Prefix, item.Preview = s.promptHints.Prefix, s.promptHints.SynthesisBody
	}
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
		case KindString, KindText:
			v, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%s 必须是字符串", k.label)
			}
			if err := s.setStr(k.key, strings.TrimSpace(v)); err != nil {
				return nil, err
			}
		case KindFloat:
			v, err := coerceOptionalFloat(raw)
			if err != nil {
				return nil, fmt.Errorf("%s 必须是数字，留空表示按端点默认", k.label)
			}
			if v != "" {
				f, _ := strconv.ParseFloat(v, 64)
				if f < 0 || f > maxLiveTemperature {
					return nil, fmt.Errorf("%s 只能在 0 到 %g 之间", k.label, maxLiveTemperature)
				}
			}
			if err := s.setStr(k.key, v); err != nil {
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

// LiveEndpointFor resolves the conversation-model endpoint a patch describes,
// without saving anything.
//
// It exists so the settings page can test what is on screen rather than what is
// stored: a base URL you have not saved yet is exactly the one you want to
// check. Two rules make that safe. A blank or masked key falls back to the
// stored one, because the form never holds the real key to send back. An
// env-locked knob ignores the form entirely, because the environment is
// authoritative and testing a value that cannot take effect would be a lie.
//
// The key comes back in plaintext and must not be logged or returned to a
// client.
func (s *SettingsService) LiveEndpointFor(patch map[string]any) (baseURL, apiKey, model string, timeout time.Duration) {
	cfg := config.GetConfig()
	pick := func(k knob) string {
		stored, _, _ := s.resolveStr(k, cfg)
		raw, ok := patch[k.key]
		if !ok || k.envLocked() {
			return stored
		}
		v, isStr := raw.(string)
		if !isStr {
			return stored
		}
		v = strings.TrimSpace(v)
		if k.kind == KindSecret && isSecretPlaceholder(v) {
			return stored
		}
		return v
	}
	seconds := 0
	for _, k := range knobs() {
		switch k.key {
		case KeyLiveBaseURL:
			baseURL = pick(k)
		case KeyLiveModel:
			model = pick(k)
		case KeyLiveAPIKey:
			apiKey = pick(k)
		case KeyLiveTimeoutSeconds:
			seconds, _, _ = s.resolveInt(k, cfg)
			if raw, ok := patch[k.key]; ok && !k.envLocked() {
				if v, err := coerceInt(raw); err == nil && v >= k.min {
					seconds = v
				}
			}
		}
	}
	return baseURL, apiKey, model, time.Duration(seconds) * time.Second
}

// coerceOptionalFloat normalises a float knob to its stored text. Blank stays
// blank — that is the "leave it to the endpoint" value, not a missing one.
func coerceOptionalFloat(raw any) (string, error) {
	switch v := raw.(type) {
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return "", nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat(f, 'g', -1, 64), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("not a number")
	}
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
	if s.liveLimits != nil {
		s.liveLimits.SetLiveLimits(LiveLimits{
			TranscriptWindow:    m[KeyLiveTranscriptWindow],
			LedgerLimit:         m[KeyLiveLedgerLimit],
			RecentTerminalHours: m[KeyLiveRecentTerminalHours],
			MaxConcurrentWork:   m[KeyLiveMaxConcurrentWork],
			ToolLoopLimit:       m[KeyLiveToolLoopLimit],
			MaxTokens:           m[KeyLiveMaxTokens],
			RunHeartbeat:        time.Duration(m[KeyRunHeartbeatMinutes]) * time.Minute,
		})
	}
	if s.livePrompts != nil {
		s.livePrompts.SetLivePrompts(LivePrompts{
			SystemBody:    str[KeyLiveSystemPromptBody],
			SynthesisBody: str[KeyLiveSynthesisPromptBody],
			Temperature:   parseOptionalFloat(str[KeyLiveTemperature]),
		})
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

// parseOptionalFloat turns the stored text into a value or an explicit "the
// endpoint decides". Anything unparseable reads as unset rather than as zero:
// a corrupted row must not silently pin the model to deterministic output.
func parseOptionalFloat(raw string) *float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		log.Warn().Str("value", raw).Msg("stored temperature is not a number; leaving it to the endpoint")
		return nil
	}
	return &f
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
