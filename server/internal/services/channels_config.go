package services

import (
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/crypto"
	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Channel config errors.
var (
	ErrChannelNotFound           = errors.New("渠道配置不存在")
	ErrChannelAppIDExists        = errors.New("该 AppID 已被其它渠道占用")
	ErrChannelProjectRequired    = errors.New("必须绑定项目")
	ErrChannelAppIDRequired      = errors.New("必须填写 AppID")
	ErrChannelSecretRequired     = errors.New("必须填写 AppSecret")
	ErrChannelSecretKeyMissing   = errors.New("未配置加密主密钥，无法加密保存渠道凭据（config: security.secrets_key 或 APPROVING_SECRETS_KEY）")
	ErrChannelSecretKeyInvalid   = errors.New("加密主密钥无效，无法加密保存渠道凭据（需 base64 编码的 32 字节；config: security.secrets_key 或 APPROVING_SECRETS_KEY）")
	ErrChannelTypeUnsupported    = errors.New("不支持的渠道类型")
	ErrChannelCronTargetRequired = errors.New("开启定时投递时必须填写投递目标（c2c:/group:/guild:）")
	ErrChannelCronTargetInvalid  = errors.New("投递目标格式无效，应为 c2c:<id> / group:<id> / guild:<id>")
)

// supportedChannelTypes gates the Type field. Extend as adapters are added.
var supportedChannelTypes = map[string]bool{
	models.ChannelTypeQQ: true,
}

// ChannelConfigInput is the create/update payload (plaintext AppSecret).
// For updates, an empty or masked AppSecret keeps the stored value.
type ChannelConfigInput struct {
	Type               string
	Name               string
	Enabled            bool
	ProjectID          string
	AppID              string
	AppSecret          string
	TurnTimeoutSeconds int
	CronDeliver        bool
	CronDeliverTarget  string
	Config             map[string]any
}

// ChannelConfigDTO is the client-facing shape (secret masked to a boolean).
type ChannelConfigDTO struct {
	ID                 string         `json:"id"`
	Type               string         `json:"type"`
	Name               string         `json:"name"`
	Enabled            bool           `json:"enabled"`
	ProjectID          string         `json:"projectId"`
	AppID              string         `json:"appId"`
	AppSecretSet       bool           `json:"appSecretSet"`
	TurnTimeoutSeconds int            `json:"turnTimeoutSeconds"`
	CronDeliver        bool           `json:"cronDeliver"`
	CronDeliverTarget  string         `json:"cronDeliverTarget,omitempty"`
	Config             map[string]any `json:"config,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
}

// ChannelConfigService manages persisted external channel configs. Writes
// trigger onChange so the channels Manager hot-reloads adapters.
type ChannelConfigService struct {
	db       *gorm.DB
	onChange func()
}

// NewChannelConfigService builds the service.
func NewChannelConfigService(db *gorm.DB) *ChannelConfigService {
	return &ChannelConfigService{db: db}
}

// SetOnChange registers a callback fired after each successful write.
func (s *ChannelConfigService) SetOnChange(fn func()) { s.onChange = fn }

func (s *ChannelConfigService) notify() {
	if s.onChange != nil {
		go s.onChange()
	}
}

// ListRaw returns all stored configs (for the Manager loader).
func (s *ChannelConfigService) ListRaw() ([]models.ChannelConfig, error) {
	var rows []models.ChannelConfig
	if err := s.db.Order("created_at asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Create inserts a new config.
func (s *ChannelConfigService) Create(in ChannelConfigInput) (ChannelConfigDTO, error) {
	if err := s.validate(in, ""); err != nil {
		return ChannelConfigDTO{}, err
	}
	secret := strings.TrimSpace(in.AppSecret)
	if secret == "" || secret == SecretMask {
		return ChannelConfigDTO{}, ErrChannelSecretRequired
	}
	enc, err := crypto.Encrypt(secret)
	if err != nil {
		return ChannelConfigDTO{}, mapEncryptErr(err)
	}
	now := time.Now()
	row := models.ChannelConfig{
		ID: "chn-" + uuid.NewString()[:12], Type: in.Type, Name: strings.TrimSpace(in.Name),
		Enabled: in.Enabled, ProjectID: strings.TrimSpace(in.ProjectID), AppID: strings.TrimSpace(in.AppID),
		AppSecretEnc: enc, TurnTimeoutSeconds: in.TurnTimeoutSeconds,
		CronDeliver: in.CronDeliver, CronDeliverTarget: strings.TrimSpace(in.CronDeliverTarget),
		Config: in.Config, CreatedAt: now, UpdatedAt: now,
	}
	s.warnIfNoPmLeader(row.ProjectID)
	if err := s.db.Create(&row).Error; err != nil {
		return ChannelConfigDTO{}, err
	}
	s.notify()
	return toChannelDTO(row), nil
}

// Update patches an existing config. Empty/masked AppSecret keeps the stored one.
func (s *ChannelConfigService) Update(id string, in ChannelConfigInput) (ChannelConfigDTO, error) {
	var row models.ChannelConfig
	if err := s.db.First(&row, "id = ?", id).Error; err != nil {
		return ChannelConfigDTO{}, ErrChannelNotFound
	}
	if err := s.validate(in, id); err != nil {
		return ChannelConfigDTO{}, err
	}
	row.Type = in.Type
	row.Name = strings.TrimSpace(in.Name)
	row.Enabled = in.Enabled
	row.ProjectID = strings.TrimSpace(in.ProjectID)
	row.AppID = strings.TrimSpace(in.AppID)
	row.TurnTimeoutSeconds = in.TurnTimeoutSeconds
	row.CronDeliver = in.CronDeliver
	row.CronDeliverTarget = strings.TrimSpace(in.CronDeliverTarget)
	row.Config = in.Config
	if secret := strings.TrimSpace(in.AppSecret); secret != "" && secret != SecretMask {
		enc, err := crypto.Encrypt(secret)
		if err != nil {
			return ChannelConfigDTO{}, mapEncryptErr(err)
		}
		row.AppSecretEnc = enc
	}
	row.UpdatedAt = time.Now()
	s.warnIfNoPmLeader(row.ProjectID)
	if err := s.db.Save(&row).Error; err != nil {
		return ChannelConfigDTO{}, err
	}
	s.notify()
	return toChannelDTO(row), nil
}

// GetByProject returns the single channel bound to a project, or nil when none
// exists. Enforces the "one channel per project" model used by the per-project
// PM settings UI.
func (s *ChannelConfigService) GetByProject(projectID string) (*ChannelConfigDTO, error) {
	var r models.ChannelConfig
	err := s.db.Order("created_at asc").First(&r, "project_id = ?", strings.TrimSpace(projectID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dto := toChannelDTO(r)
	return &dto, nil
}

// UpsertForProject creates or updates the single channel bound to a project.
// ProjectID is forced from the path argument. When a channel already exists for
// the project it is patched in place (blank/masked AppSecret preserved),
// otherwise a new one is created.
func (s *ChannelConfigService) UpsertForProject(projectID string, in ChannelConfigInput) (ChannelConfigDTO, error) {
	in.ProjectID = strings.TrimSpace(projectID)
	var existing models.ChannelConfig
	err := s.db.Order("created_at asc").First(&existing, "project_id = ?", in.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.Create(in)
	}
	if err != nil {
		return ChannelConfigDTO{}, err
	}
	return s.Update(existing.ID, in)
}

// DeleteByProject removes the channel bound to a project. It is idempotent: a
// project with no channel is a successful no-op (DELETE stays safe to retry).
func (s *ChannelConfigService) DeleteByProject(projectID string) error {
	res := s.db.Where("project_id = ?", strings.TrimSpace(projectID)).Delete(&models.ChannelConfig{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		s.notify()
	}
	return nil
}

func (s *ChannelConfigService) validate(in ChannelConfigInput, selfID string) error {
	if !supportedChannelTypes[in.Type] {
		return ErrChannelTypeUnsupported
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return ErrChannelProjectRequired
	}
	var p models.Project
	if err := s.db.Select("id").First(&p, "id = ?", strings.TrimSpace(in.ProjectID)).Error; err != nil {
		return ErrProjectNotFound
	}
	appID := strings.TrimSpace(in.AppID)
	if appID == "" {
		return ErrChannelAppIDRequired
	}
	q := s.db.Model(&models.ChannelConfig{}).Where("app_id = ?", appID)
	if selfID != "" {
		q = q.Where("id <> ?", selfID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrChannelAppIDExists
	}
	if in.CronDeliver {
		target := strings.TrimSpace(in.CronDeliverTarget)
		if target == "" {
			return ErrChannelCronTargetRequired
		}
		if !validCronDeliverTarget(target) {
			return ErrChannelCronTargetInvalid
		}
	}
	return nil
}

// validCronDeliverTarget mirrors channels.parseTarget: scene:conversationId
// where scene is c2c|group|guild and conversationId is non-empty.
func validCronDeliverTarget(target string) bool {
	parts := strings.SplitN(strings.TrimSpace(target), ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return false
	}
	switch parts[0] {
	case "c2c", "group", "guild":
		return true
	default:
		return false
	}
}

// mapEncryptErr translates crypto key-source failures into channel-facing
// errors. Other Encrypt failures (e.g. RNG) are returned unchanged so the
// handler can surface them as 500 instead of a misleading "key missing".
func mapEncryptErr(err error) error {
	switch {
	case errors.Is(err, crypto.ErrNoSecretsKey):
		return ErrChannelSecretKeyMissing
	case errors.Is(err, crypto.ErrInvalidSecretsKey):
		return ErrChannelSecretKeyInvalid
	default:
		return err
	}
}

// warnIfNoPmLeader logs (non-fatal) when the bound project lacks a usable PM Leader.
func (s *ChannelConfigService) warnIfNoPmLeader(projectID string) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", projectID).Error; err != nil {
		return
	}
	if !p.PmLeaderEnabled || strings.TrimSpace(p.PmLeaderAgent) == "" {
		log.Warn().Str("project", projectID).Msg("channel bound to a project without an enabled PM Leader; turns will fail until enabled")
	}
}

func toChannelDTO(r models.ChannelConfig) ChannelConfigDTO {
	return ChannelConfigDTO{
		ID: r.ID, Type: r.Type, Name: r.Name, Enabled: r.Enabled, ProjectID: r.ProjectID,
		AppID: r.AppID, AppSecretSet: strings.TrimSpace(r.AppSecretEnc) != "",
		TurnTimeoutSeconds: r.TurnTimeoutSeconds, CronDeliver: r.CronDeliver,
		CronDeliverTarget: r.CronDeliverTarget, Config: r.Config,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
