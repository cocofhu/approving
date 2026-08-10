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
	ErrChannelNotFound              = errors.New("渠道配置不存在")
	ErrChannelAppIDExists           = errors.New("该 AppID 已被其它渠道占用")
	ErrChannelProjectRequired       = errors.New("必须绑定项目")
	ErrChannelAppIDRequired         = errors.New("必须填写 AppID")
	ErrChannelSecretRequired        = errors.New("必须填写 AppSecret")
	ErrChannelSecretKeyMissing      = errors.New("未配置加密主密钥，无法加密保存渠道凭据（config: security.secrets_key 或 APPROVING_SECRETS_KEY）")
	ErrChannelSecretKeyInvalid      = errors.New("加密主密钥无效，无法加密保存渠道凭据（需 base64 编码的 32 字节；config: security.secrets_key 或 APPROVING_SECRETS_KEY）")
	ErrChannelTypeUnsupported       = errors.New("不支持的渠道类型")
	ErrChannelCronTargetRequired    = errors.New("开启定时投递时必须填写投递目标（c2c:/group:/guild:）")
	ErrChannelCronTargetInvalid     = errors.New("投递目标格式无效，应为 c2c:<id> / group:<id> / guild:<id>")
	ErrChannelAgentRequired         = errors.New("必须绑定 Agent")
	ErrChannelAgentTaken            = errors.New("该 Agent 已被其它渠道占用")
	ErrChannelAgentNotInProject     = errors.New("只能绑定主项目为本项目的 Agent")
	ErrChannelAgentUnavailable      = errors.New("无可用空闲 Agent，请先创建 Agent")
	ErrChannelDualPrimary           = errors.New("每个项目至多一个主 Channel")
	ErrChannelPromoteForbidden      = errors.New("不允许将副 Channel 提升为主；请通过删除主流指定新主")
	ErrChannelDeletePrimaryNeedsAck = errors.New("删除主 Channel 须指定新主或确认无主")
	ErrChannelNewPrimaryNotFound    = errors.New("指定的新主 Channel 不存在或不属于本项目")
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
	AgentName          string
	IsPrimary          bool
	EnabledMcps        []string
	AppID              string
	AppSecret          string
	TurnTimeoutSeconds int
	CronDeliver        bool
	CronDeliverTarget  string
	Config             map[string]any
	// SyncPmLeader, when true on a primary-channel agent rebind, updates
	// Project.PmLeaderAgent to the new AgentName. UI must confirm first.
	SyncPmLeader bool
}

// ChannelDeleteOpts controls primary-channel deletion.
type ChannelDeleteOpts struct {
	// NewPrimaryID promotes another channel of the same project to primary
	// before deleting the current primary. Mutually exclusive with ConfirmNoPrimary.
	NewPrimaryID string
	// ConfirmNoPrimary acknowledges leaving the project without a primary channel.
	ConfirmNoPrimary bool
	// SyncPmLeader updates Project.PmLeaderAgent to the new primary's Agent
	// when NewPrimaryID is set and the agent differs.
	SyncPmLeader bool
}

// ChannelConfigDTO is the client-facing shape (secret masked to a boolean).
type ChannelConfigDTO struct {
	ID                 string         `json:"id"`
	Type               string         `json:"type"`
	Name               string         `json:"name"`
	Enabled            bool           `json:"enabled"`
	ProjectID          string         `json:"projectId"`
	AgentName          string         `json:"agentName"`
	IsPrimary          bool           `json:"isPrimary"`
	EnabledMcps        []string       `json:"enabledMcps"`
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
	skills   *SkillService
	onChange func()
}

// NewChannelConfigService builds the service.
func NewChannelConfigService(db *gorm.DB) *ChannelConfigService {
	return &ChannelConfigService{db: db}
}

// SetSkillService wires Agent lookups for home-project / free-agent checks.
func (s *ChannelConfigService) SetSkillService(skills *SkillService) { s.skills = skills }

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

// ListByProject returns all channels for a project (primary first, then created_at).
func (s *ChannelConfigService) ListByProject(projectID string) ([]ChannelConfigDTO, error) {
	projectID = strings.TrimSpace(projectID)
	var rows []models.ChannelConfig
	if err := s.db.Where("project_id = ?", projectID).
		Order("is_primary desc, created_at asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ChannelConfigDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toChannelDTO(r))
	}
	return out, nil
}

// GetByID returns one channel DTO or ErrChannelNotFound.
func (s *ChannelConfigService) GetByID(id string) (ChannelConfigDTO, error) {
	var row models.ChannelConfig
	if err := s.db.First(&row, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChannelConfigDTO{}, ErrChannelNotFound
		}
		return ChannelConfigDTO{}, err
	}
	return toChannelDTO(row), nil
}

// GetPrimaryByProject returns the primary channel, or nil when the project has none.
func (s *ChannelConfigService) GetPrimaryByProject(projectID string) (*ChannelConfigDTO, error) {
	var r models.ChannelConfig
	err := s.db.Where("project_id = ? AND is_primary = ?", strings.TrimSpace(projectID), true).
		Order("created_at asc").First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dto := toChannelDTO(r)
	return &dto, nil
}

// GetByAgent returns the channel bound to agentName, or nil.
func (s *ChannelConfigService) GetByAgent(agentName string) (*ChannelConfigDTO, error) {
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return nil, nil
	}
	var r models.ChannelConfig
	err := s.db.Where("agent_name = ?", agentName).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dto := toChannelDTO(r)
	return &dto, nil
}

// Create inserts a new config. First channel (or first while no primary) becomes
// primary; when a primary already exists the new row is secondary.
func (s *ChannelConfigService) Create(in ChannelConfigInput) (ChannelConfigDTO, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.AgentName = strings.TrimSpace(in.AgentName)
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

	hasPrimary, err := s.projectHasPrimary(in.ProjectID)
	if err != nil {
		return ChannelConfigDTO{}, err
	}
	isPrimary := !hasPrimary // first / no-primary → primary; else secondary
	// Ignore client-supplied IsPrimary elevation of a secondary create.
	if in.IsPrimary && hasPrimary {
		return ChannelConfigDTO{}, ErrChannelDualPrimary
	}

	if in.AgentName == "" && isPrimary {
		if agent := s.defaultPrimaryAgent(in.ProjectID); agent != "" {
			if err := s.ensureAgentFree(agent, ""); err == nil && s.agentInProject(agent, in.ProjectID) {
				in.AgentName = agent
			}
		}
	}
	if in.AgentName == "" {
		return ChannelConfigDTO{}, ErrChannelAgentRequired
	}
	if err := s.ensureAgentFree(in.AgentName, ""); err != nil {
		return ChannelConfigDTO{}, err
	}
	if !s.agentInProject(in.AgentName, in.ProjectID) {
		return ChannelConfigDTO{}, ErrChannelAgentNotInProject
	}

	now := time.Now()
	var storedMcps []string
	if in.EnabledMcps != nil {
		storedMcps = FilterPmEnabledMcps(in.EnabledMcps)
	}
	row := models.ChannelConfig{
		ID: "chn-" + uuid.NewString()[:12], Type: in.Type, Name: strings.TrimSpace(in.Name),
		Enabled: in.Enabled, ProjectID: in.ProjectID, AgentName: in.AgentName, IsPrimary: isPrimary,
		EnabledMcps: storedMcps,
		AppID:       strings.TrimSpace(in.AppID), AppSecretEnc: enc,
		TurnTimeoutSeconds: in.TurnTimeoutSeconds,
		CronDeliver:        in.CronDeliver, CronDeliverTarget: strings.TrimSpace(in.CronDeliverTarget),
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
// Primary flag cannot be raised on a secondary via Update (delete-primary flow only).
func (s *ChannelConfigService) Update(id string, in ChannelConfigInput) (ChannelConfigDTO, error) {
	var row models.ChannelConfig
	if err := s.db.First(&row, "id = ?", id).Error; err != nil {
		return ChannelConfigDTO{}, ErrChannelNotFound
	}
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	if in.ProjectID == "" {
		in.ProjectID = row.ProjectID
	}
	in.AgentName = strings.TrimSpace(in.AgentName)
	if in.AgentName == "" {
		in.AgentName = row.AgentName
	}
	if err := s.validate(in, id); err != nil {
		return ChannelConfigDTO{}, err
	}
	if in.AgentName == "" {
		return ChannelConfigDTO{}, ErrChannelAgentRequired
	}
	if err := s.ensureAgentFree(in.AgentName, id); err != nil {
		return ChannelConfigDTO{}, err
	}
	if !s.agentInProject(in.AgentName, in.ProjectID) {
		return ChannelConfigDTO{}, ErrChannelAgentNotInProject
	}
	// Forbid promoting secondary → primary via update.
	if in.IsPrimary && !row.IsPrimary {
		return ChannelConfigDTO{}, ErrChannelPromoteForbidden
	}
	// Forbid demoting primary via update (use delete-primary flow).
	if row.IsPrimary && !in.IsPrimary {
		in.IsPrimary = true
	}

	oldAgent := row.AgentName
	row.Type = in.Type
	row.Name = strings.TrimSpace(in.Name)
	row.Enabled = in.Enabled
	row.ProjectID = strings.TrimSpace(in.ProjectID)
	row.AgentName = in.AgentName
	row.IsPrimary = row.IsPrimary // unchanged except forbid demote above
	if in.EnabledMcps == nil {
		// keep existing
	} else {
		row.EnabledMcps = FilterPmEnabledMcps(in.EnabledMcps)
	}
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

	// Primary rebind → optional PmLeader sync (UI confirmed via SyncPmLeader).
	if row.IsPrimary && in.SyncPmLeader && row.AgentName != "" && row.AgentName != oldAgent {
		_ = s.syncProjectPmLeader(row.ProjectID, row.AgentName)
	}

	s.notify()
	return toChannelDTO(row), nil
}

// GetByProject returns the primary channel bound to a project (legacy alias),
// or nil when none exists. Falls back to earliest channel if no primary marked.
func (s *ChannelConfigService) GetByProject(projectID string) (*ChannelConfigDTO, error) {
	dto, err := s.GetPrimaryByProject(projectID)
	if err != nil || dto != nil {
		return dto, err
	}
	var r models.ChannelConfig
	err = s.db.Order("created_at asc").First(&r, "project_id = ?", strings.TrimSpace(projectID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := toChannelDTO(r)
	return &out, nil
}

// UpsertForProject creates or updates the primary channel (legacy single-channel
// API alias). ProjectID is forced from the path argument.
func (s *ChannelConfigService) UpsertForProject(projectID string, in ChannelConfigInput) (ChannelConfigDTO, error) {
	in.ProjectID = strings.TrimSpace(projectID)
	primary, err := s.GetPrimaryByProject(in.ProjectID)
	if err != nil {
		return ChannelConfigDTO{}, err
	}
	if primary == nil {
		// Preserve legacy: if any channel exists without primary flag, patch earliest.
		var existing models.ChannelConfig
		err := s.db.Order("created_at asc").First(&existing, "project_id = ?", in.ProjectID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			in.IsPrimary = true
			if strings.TrimSpace(in.AgentName) == "" {
				in.AgentName = s.defaultPrimaryAgent(in.ProjectID)
			}
			return s.Create(in)
		}
		if err != nil {
			return ChannelConfigDTO{}, err
		}
		in.IsPrimary = true
		return s.Update(existing.ID, in)
	}
	in.IsPrimary = true
	if strings.TrimSpace(in.AgentName) == "" {
		in.AgentName = primary.AgentName
	}
	return s.Update(primary.ID, in)
}

// DeleteByProject removes the primary channel (legacy alias). Prefer Delete.
func (s *ChannelConfigService) DeleteByProject(projectID string) error {
	primary, err := s.GetPrimaryByProject(projectID)
	if err != nil {
		return err
	}
	if primary == nil {
		// Legacy: delete earliest / any single row.
		res := s.db.Where("project_id = ?", strings.TrimSpace(projectID)).Delete(&models.ChannelConfig{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			s.notify()
		}
		return nil
	}
	return s.Delete(primary.ID, ChannelDeleteOpts{ConfirmNoPrimary: true})
}

// Delete removes a channel by id. Deleting the primary requires NewPrimaryID or ConfirmNoPrimary.
func (s *ChannelConfigService) Delete(id string, opts ChannelDeleteOpts) error {
	var row models.ChannelConfig
	if err := s.db.First(&row, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChannelNotFound
		}
		return err
	}

	if row.IsPrimary {
		newPrimaryID := strings.TrimSpace(opts.NewPrimaryID)
		if newPrimaryID == "" && !opts.ConfirmNoPrimary {
			return ErrChannelDeletePrimaryNeedsAck
		}
		if newPrimaryID != "" {
			var next models.ChannelConfig
			if err := s.db.First(&next, "id = ? AND project_id = ?", newPrimaryID, row.ProjectID).Error; err != nil {
				return ErrChannelNewPrimaryNotFound
			}
			if next.ID == row.ID {
				return ErrChannelNewPrimaryNotFound
			}
			if err := s.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&models.ChannelConfig{}).Where("id = ?", row.ID).
					Update("is_primary", false).Error; err != nil {
					return err
				}
				if err := tx.Model(&models.ChannelConfig{}).Where("id = ?", next.ID).
					Update("is_primary", true).Error; err != nil {
					return err
				}
				if err := tx.Delete(&models.ChannelConfig{}, "id = ?", row.ID).Error; err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
			if opts.SyncPmLeader && strings.TrimSpace(next.AgentName) != "" {
				_ = s.syncProjectPmLeader(row.ProjectID, next.AgentName)
			}
			s.removeNotifyChannelID(row.ProjectID, row.ID)
			s.notify()
			return nil
		}
		// ConfirmNoPrimary: delete and leave project without primary.
	}

	if err := s.db.Delete(&models.ChannelConfig{}, "id = ?", row.ID).Error; err != nil {
		return err
	}
	s.removeNotifyChannelID(row.ProjectID, row.ID)
	s.notify()
	return nil
}

func (s *ChannelConfigService) removeNotifyChannelID(projectID, channelID string) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", projectID).Error; err != nil {
		return
	}
	pol := p.NotifyPolicy
	if len(pol.ChannelIDs) == 0 {
		return
	}
	next := make([]string, 0, len(pol.ChannelIDs))
	changed := false
	for _, id := range pol.ChannelIDs {
		if id == channelID {
			changed = true
			continue
		}
		next = append(next, id)
	}
	if !changed {
		return
	}
	pol.ChannelIDs = next
	_ = s.db.Model(&models.Project{}).Where("id = ?", projectID).Update("notify_policy", pol).Error
}

func (s *ChannelConfigService) syncProjectPmLeader(projectID, agentName string) error {
	return s.db.Model(&models.Project{}).Where("id = ?", projectID).
		Updates(map[string]any{
			"pm_leader_agent":  agentName,
			"pm_leader_enabled": true,
			"updated_at":       time.Now(),
		}).Error
}

func (s *ChannelConfigService) projectHasPrimary(projectID string) (bool, error) {
	var n int64
	if err := s.db.Model(&models.ChannelConfig{}).
		Where("project_id = ? AND is_primary = ?", projectID, true).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *ChannelConfigService) defaultPrimaryAgent(projectID string) string {
	var p models.Project
	if err := s.db.Select("pm_leader_agent").First(&p, "id = ?", projectID).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(p.PmLeaderAgent)
}

func (s *ChannelConfigService) ensureAgentFree(agentName, selfID string) error {
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return ErrChannelAgentRequired
	}
	q := s.db.Model(&models.ChannelConfig{}).Where("agent_name = ?", agentName)
	if selfID != "" {
		q = q.Where("id <> ?", selfID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrChannelAgentTaken
	}
	return nil
}

func (s *ChannelConfigService) agentInProject(agentName, projectID string) bool {
	agentName = strings.TrimSpace(agentName)
	projectID = strings.TrimSpace(projectID)
	if agentName == "" || projectID == "" {
		return false
	}
	if s.skills == nil {
		// Skills not wired (some unit tests): allow any non-empty agent.
		return true
	}
	ag, ok := s.skills.Get(agentName)
	if !ok {
		return false
	}
	return AgentProjectMatches(ag, projectID)
}

// ListFreeAgents returns agents whose home project is projectID and who are
// not bound to another channel (except excludeChannelID).
func (s *ChannelConfigService) ListFreeAgents(projectID, excludeChannelID string) []string {
	if s.skills == nil {
		return nil
	}
	projectID = strings.TrimSpace(projectID)
	var bound []models.ChannelConfig
	_ = s.db.Select("id", "agent_name").Where("project_id = ?", projectID).Find(&bound).Error
	taken := map[string]bool{}
	for _, b := range bound {
		if b.ID == excludeChannelID {
			continue
		}
		if a := strings.TrimSpace(b.AgentName); a != "" {
			taken[a] = true
		}
	}
	var free []string
	for _, ag := range s.skills.List() {
		if !AgentProjectMatches(ag, projectID) {
			continue
		}
		if taken[ag.Name] {
			continue
		}
		free = append(free, ag.Name)
	}
	return free
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
		log.Warn().Str("project", projectID).Msg("channel bound to a project without an enabled PM Leader; web/gate turns will fail until enabled")
	}
}

func toChannelDTO(r models.ChannelConfig) ChannelConfigDTO {
	mcps := r.EnabledMcps
	if mcps == nil {
		mcps = EffectivePmEnabledMcps(nil)
	} else {
		mcps = FilterPmEnabledMcps(mcps)
	}
	return ChannelConfigDTO{
		ID: r.ID, Type: r.Type, Name: r.Name, Enabled: r.Enabled, ProjectID: r.ProjectID,
		AgentName: r.AgentName, IsPrimary: r.IsPrimary, EnabledMcps: mcps,
		AppID: r.AppID, AppSecretSet: strings.TrimSpace(r.AppSecretEnc) != "",
		TurnTimeoutSeconds: r.TurnTimeoutSeconds, CronDeliver: r.CronDeliver,
		CronDeliverTarget: r.CronDeliverTarget, Config: r.Config,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
