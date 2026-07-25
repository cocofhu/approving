package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

var (
	// ErrPmLeaderDisabled is returned when consult APIs are called while PM Leader is off.
	ErrPmLeaderDisabled = errors.New("PM Leader 未启用")
	// ErrPmLeaderNoAgent is returned when enabling without a bound agent, or the agent is missing.
	ErrPmLeaderNoAgent = errors.New("未绑定可用 Agent，请先到 Agent 配置中心准备后再绑定")
	// ErrPmLeaderAgentMissing is returned when the bound agent was deleted.
	ErrPmLeaderAgentMissing = errors.New("绑定的 Agent 已删除或不可用，请管理员重新绑定")
	// ErrPmLeaderProjectMismatch is returned when the agent's home project is not
	// this project. A PM Leader must have the consult project as its home.
	ErrPmLeaderProjectMismatch = errors.New("该 Agent 的主项目不是本项目，请先在 Agent Studio 将其主项目设为本项目")
	// ErrPmMemoryNotFound is returned when a memory item is missing.
	ErrPmMemoryNotFound = errors.New("记忆条目不存在")
	// ErrPmThreadNotFound is returned when a chat thread is missing or not owned.
	ErrPmThreadNotFound = errors.New("会话不存在")
	// ErrPmMessageNotFound is returned when a chat message is missing.
	ErrPmMessageNotFound = errors.New("消息不存在")
	// ErrPmMessageInvalidStatus is returned for unsupported status/failKind updates.
	ErrPmMessageInvalidStatus = errors.New("无效的消息失败状态")
	// ErrPmMessageInvalidRole is returned when failure metadata is applied to a non-user message.
	ErrPmMessageInvalidRole = errors.New("仅支持标记用户消息的失败状态")
	// ErrPmAdminRequired is returned for admin-only memory / admin paths (not PM Leader bind).
	ErrPmAdminRequired = errors.New("需要平台管理员权限")
	// ErrPmCronJobNotFound is returned when a cron job is missing for the project.
	ErrPmCronJobNotFound = errors.New("定时任务不存在")
	// ErrPmChannelReadOnly is returned when a Web client tries to write/delete a QQ channel thread.
	ErrPmChannelReadOnly = errors.New("渠道会话为只读，不可在 Web 发送或删除")
)

// IsQQChannelUserID reports whether userID is a QQ Channel synthetic identity (qq:…).
func IsQQChannelUserID(userID string) bool {
	return strings.HasPrefix(userID, "qq:")
}

// IsSyntheticThreadUserID reports cron/channel synthetic owners that must not trigger UI merge.
func IsSyntheticThreadUserID(userID string) bool {
	return IsQQChannelUserID(userID) || strings.HasPrefix(userID, "cron:")
}

// Valid PM chat fail kinds (product-level categories).
const (
	PmFailConnection = "connection"
	PmFailSandbox    = "sandbox"
	PmFailEmpty      = "empty"
	PmFailUnknown    = "unknown"
	PmFailStopped    = "stopped"
)

func validPmFailKind(kind string) bool {
	switch kind {
	case PmFailConnection, PmFailSandbox, PmFailEmpty, PmFailUnknown, PmFailStopped:
		return true
	default:
		return false
	}
}

// PmService manages project PM Leader binding, memory, and chat persistence.
type PmService struct {
	db     *gorm.DB
	skills *SkillService
}

// NewPmService builds the service.
func NewPmService(db *gorm.DB, skills *SkillService) *PmService {
	return &PmService{db: db, skills: skills}
}

// --- binding ----------------------------------------------------------------

// PmLeaderBinding is the API shape for enable/bind state.
type PmLeaderBinding struct {
	Enabled        bool   `json:"enabled"`
	AgentConfigRef string `json:"agentConfigRef"`
	AgentAvailable bool   `json:"agentAvailable"`
	AgentError     string `json:"agentError,omitempty"`
	// EnabledMcps lists PM-only MCP ids (pm-progress, pm-workflow-read,
	// pm-workflow-write). nil/omitted on disk means defaults; explicit empty means none.
	EnabledMcps []string `json:"enabledMcps"`
	// AclNote points users to Agent Studio for memory management.
	AclNote string `json:"aclNote"`
}

// DefaultPmEnabledMcps is the default PM-only MCP set.
var DefaultPmEnabledMcps = []string{"pm-progress", "pm-workflow-read", "pm-workflow-write"}

// FilterPmEnabledMcps returns validated unique PM MCP ids (may be empty).
func FilterPmEnabledMcps(in []string) []string {
	allowed := map[string]bool{"pm-progress": true, "pm-workflow-read": true, "pm-workflow-write": true}
	var out []string
	seen := map[string]bool{}
	for _, id := range in {
		id = strings.TrimSpace(id)
		if !allowed[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if out == nil {
		return []string{}
	}
	return out
}

// EffectivePmEnabledMcps resolves stored values: nil → default both; non-nil → filtered (may be empty).
func EffectivePmEnabledMcps(stored []string) []string {
	if stored == nil {
		return append([]string{}, DefaultPmEnabledMcps...)
	}
	return FilterPmEnabledMcps(stored)
}

// NormalizePmEnabledMcps is kept for callers that want the effective list.
func NormalizePmEnabledMcps(in []string) []string {
	return EffectivePmEnabledMcps(in)
}

const pmLeaderAclNote = "记忆请在已绑定主项目的 Agent Studio「数据 → 记忆」中管理；任意已登录用户可编辑。"

// GetBinding returns the PM Leader binding for a project.
func (s *PmService) GetBinding(projectID string) (PmLeaderBinding, error) {
	p, ok := s.project(projectID)
	if !ok {
		return PmLeaderBinding{}, ErrProjectNotFound
	}
	b := PmLeaderBinding{
		Enabled:        p.PmLeaderEnabled,
		AgentConfigRef: p.PmLeaderAgent,
		EnabledMcps:    EffectivePmEnabledMcps(p.PmEnabledMcps),
		AclNote:        pmLeaderAclNote,
	}
	if p.PmLeaderAgent == "" {
		b.AgentAvailable = false
		if p.PmLeaderEnabled {
			b.AgentError = ErrPmLeaderNoAgent.Error()
		}
		return b, nil
	}
	if s.skills != nil {
		ag, ok := s.skills.Get(p.PmLeaderAgent)
		if !ok {
			b.AgentAvailable = false
			b.AgentError = ErrPmLeaderAgentMissing.Error()
		} else if !AgentProjectMatches(ag, projectID) {
			b.AgentAvailable = false
			b.AgentError = ErrPmLeaderProjectMismatch.Error()
		} else {
			b.AgentAvailable = true
		}
	} else {
		b.AgentAvailable = true
	}
	return b, nil
}

// UpdateBinding patches enable/agent/enabledMcps. Enabling requires a resolvable agent.
func (s *PmService) UpdateBinding(projectID string, enabled *bool, agent *string, enabledMcps []string) (PmLeaderBinding, error) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", projectID).Error; err != nil {
		return PmLeaderBinding{}, ErrProjectNotFound
	}
	if agent != nil {
		name := strings.TrimSpace(*agent)
		if name != "" {
			if s.skills != nil {
				ag, ok := s.skills.Get(name)
				if !ok {
					return PmLeaderBinding{}, ErrPmLeaderNoAgent
				}
				if strings.TrimSpace(ag.ProjectID) != projectID {
					return PmLeaderBinding{}, ErrPmLeaderProjectMismatch
				}
			}
		}
		p.PmLeaderAgent = name
	}
	if enabledMcps != nil {
		// Explicit empty list disables all PM-only MCPs (do not expand to defaults).
		p.PmEnabledMcps = FilterPmEnabledMcps(enabledMcps)
	}
	if enabled != nil {
		if *enabled {
			if strings.TrimSpace(p.PmLeaderAgent) == "" {
				return PmLeaderBinding{}, ErrPmLeaderNoAgent
			}
			if s.skills != nil {
				ag, ok := s.skills.Get(p.PmLeaderAgent)
				if !ok {
					return PmLeaderBinding{}, ErrPmLeaderNoAgent
				}
				if strings.TrimSpace(ag.ProjectID) != projectID {
					return PmLeaderBinding{}, ErrPmLeaderProjectMismatch
				}
			}
		}
		p.PmLeaderEnabled = *enabled
	}
	p.UpdatedAt = time.Now()
	if err := s.db.Save(&p).Error; err != nil {
		return PmLeaderBinding{}, err
	}
	// Backfill legacy memories when a PM agent is bound (write path, not GetBinding).
	if strings.TrimSpace(p.PmLeaderAgent) != "" {
		if err := s.BackfillLegacyMemoriesToPMAgent(projectID); err != nil {
			log.Warn().Err(err).Str("project", projectID).Msg("backfill legacy memories failed")
		}
	}
	return s.GetBinding(projectID)
}

// RequireEnabled checks the project has PM Leader on with a usable agent.
func (s *PmService) RequireEnabled(projectID string) (models.Project, error) {
	p, ok := s.project(projectID)
	if !ok {
		return models.Project{}, ErrProjectNotFound
	}
	if !p.PmLeaderEnabled {
		return models.Project{}, ErrPmLeaderDisabled
	}
	if strings.TrimSpace(p.PmLeaderAgent) == "" {
		return models.Project{}, ErrPmLeaderNoAgent
	}
	if s.skills != nil {
		ag, ok := s.skills.Get(p.PmLeaderAgent)
		if !ok {
			return models.Project{}, ErrPmLeaderAgentMissing
		}
		if strings.TrimSpace(ag.ProjectID) != projectID {
			return models.Project{}, ErrPmLeaderProjectMismatch
		}
	}
	return p, nil
}

func (s *PmService) project(id string) (models.Project, bool) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return models.Project{}, false
	}
	return p, true
}

// --- memory -----------------------------------------------------------------

// ListMemories returns memory items for a project (newest first).
// When agentName is non-empty, results are scoped to that Agent.
// Does not claim legacy (empty agent_name) rows — use BackfillLegacyMemoriesToPMAgent.
func (s *PmService) ListMemories(projectID, agentName string) ([]models.ProjectMemoryItem, error) {
	if _, ok := s.project(projectID); !ok {
		return nil, ErrProjectNotFound
	}
	q := s.db.Where("project_id = ?", projectID)
	if agentName != "" {
		q = q.Where("agent_name = ?", agentName)
	}
	var items []models.ProjectMemoryItem
	if err := q.Order("updated_at desc").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// BackfillLegacyMemoriesToPMAgent assigns empty-agent_name memories to the
// project's bound PM Leader agent only. Safe to call repeatedly; no-op when
// no PM agent is bound. Never claim legacy rows for an arbitrary Agent.
func (s *PmService) BackfillLegacyMemoriesToPMAgent(projectID string) error {
	p, ok := s.project(projectID)
	if !ok {
		return ErrProjectNotFound
	}
	agent := strings.TrimSpace(p.PmLeaderAgent)
	if agent == "" {
		return nil
	}
	return s.db.Model(&models.ProjectMemoryItem{}).
		Where("project_id = ? AND (agent_name = '' OR agent_name IS NULL)", projectID).
		Update("agent_name", agent).Error
}

// UpsertMemory creates or updates a memory item by title within a project+agent.
func (s *PmService) UpsertMemory(projectID, agentName, title, content, source, updatedBy string) (models.ProjectMemoryItem, error) {
	if _, ok := s.project(projectID); !ok {
		return models.ProjectMemoryItem{}, ErrProjectNotFound
	}
	title = strings.TrimSpace(title)
	agentName = strings.TrimSpace(agentName)
	if title == "" {
		return models.ProjectMemoryItem{}, fmt.Errorf("记忆标题不能为空")
	}
	if source == "" {
		source = "user"
	}
	now := time.Now()
	var existing models.ProjectMemoryItem
	err := s.db.Where("project_id = ? AND agent_name = ? AND title = ?", projectID, agentName, title).First(&existing).Error
	if err == nil {
		existing.Content = content
		existing.Source = source
		existing.UpdatedBy = updatedBy
		existing.UpdatedAt = now
		if err := s.db.Save(&existing).Error; err != nil {
			return models.ProjectMemoryItem{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ProjectMemoryItem{}, err
	}
	item := models.ProjectMemoryItem{
		ID: "mem-" + uuid.NewString()[:12], ProjectID: projectID, AgentName: agentName,
		Title: title, Content: content, Source: source, UpdatedBy: updatedBy,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.Create(&item).Error; err != nil {
		return models.ProjectMemoryItem{}, err
	}
	return item, nil
}

// UpdateMemoryByID patches content/title of an existing item (project-wide).
func (s *PmService) UpdateMemoryByID(projectID, id, title, content, updatedBy string) (models.ProjectMemoryItem, error) {
	return s.patchMemory(projectID, "", id, title, content, updatedBy)
}

// UpdateMemoryForAgent patches a memory only when it belongs to project+agent.
func (s *PmService) UpdateMemoryForAgent(projectID, agentName, id, title, content, updatedBy string) (models.ProjectMemoryItem, error) {
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return models.ProjectMemoryItem{}, ErrPmMemoryNotFound
	}
	return s.patchMemory(projectID, agentName, id, title, content, updatedBy)
}

func (s *PmService) patchMemory(projectID, agentName, id, title, content, updatedBy string) (models.ProjectMemoryItem, error) {
	var item models.ProjectMemoryItem
	q := s.db.Where("id = ? AND project_id = ?", id, projectID)
	if agentName != "" {
		q = q.Where("agent_name = ?", agentName)
	}
	if err := q.First(&item).Error; err != nil {
		return models.ProjectMemoryItem{}, ErrPmMemoryNotFound
	}
	if t := strings.TrimSpace(title); t != "" {
		item.Title = t
	}
	item.Content = content
	item.Source = "user"
	item.UpdatedBy = updatedBy
	item.UpdatedAt = time.Now()
	if err := s.db.Save(&item).Error; err != nil {
		return models.ProjectMemoryItem{}, err
	}
	return item, nil
}

// DeleteMemory removes one memory item by project+id (admin / project-wide).
func (s *PmService) DeleteMemory(projectID, id string) error {
	res := s.db.Where("id = ? AND project_id = ?", id, projectID).Delete(&models.ProjectMemoryItem{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPmMemoryNotFound
	}
	return nil
}

// DeleteMemoryForAgent removes one memory only when it belongs to project+agent.
func (s *PmService) DeleteMemoryForAgent(projectID, agentName, id string) error {
	agentName = strings.TrimSpace(agentName)
	id = strings.TrimSpace(id)
	if agentName == "" || id == "" {
		return ErrPmMemoryNotFound
	}
	res := s.db.Where("id = ? AND project_id = ? AND agent_name = ?", id, projectID, agentName).
		Delete(&models.ProjectMemoryItem{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPmMemoryNotFound
	}
	return nil
}

// ClearMemories deletes all memory items for a project (admin).
func (s *PmService) ClearMemories(projectID string) (int64, error) {
	if _, ok := s.project(projectID); !ok {
		return 0, ErrProjectNotFound
	}
	res := s.db.Where("project_id = ?", projectID).Delete(&models.ProjectMemoryItem{})
	return res.RowsAffected, res.Error
}

// ClearMemoriesForAgent deletes memories for one agent in a project.
func (s *PmService) ClearMemoriesForAgent(projectID, agentName string) (int64, error) {
	if _, ok := s.project(projectID); !ok {
		return 0, ErrProjectNotFound
	}
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return 0, fmt.Errorf("agentName required")
	}
	res := s.db.Where("project_id = ? AND agent_name = ?", projectID, agentName).
		Delete(&models.ProjectMemoryItem{})
	return res.RowsAffected, res.Error
}

// PurgeAgentProjectData removes every piece of an Agent's data scoped to one
// project: memories, chat threads (+ messages + drafts), and cron jobs (+ runs).
// It also disables the project's PM Leader when it points at this Agent.
// Idempotent; missing project is tolerated (rows matched by ids only).
func (s *PmService) PurgeAgentProjectData(projectID, agentName string) error {
	projectID = strings.TrimSpace(projectID)
	agentName = strings.TrimSpace(agentName)
	if projectID == "" || agentName == "" {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ? AND agent_name = ?", projectID, agentName).
			Delete(&models.ProjectMemoryItem{}).Error; err != nil {
			return err
		}
		var threadIDs []string
		if err := tx.Model(&models.ChatThread{}).
			Where("project_id = ? AND agent_name = ?", projectID, agentName).
			Pluck("id", &threadIDs).Error; err != nil {
			return err
		}
		if len(threadIDs) > 0 {
			if err := tx.Where("thread_id IN ?", threadIDs).Delete(&models.ChatTurnDraft{}).Error; err != nil {
				return err
			}
			if err := tx.Where("thread_id IN ?", threadIDs).Delete(&models.ChatMessage{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", threadIDs).Delete(&models.ChatThread{}).Error; err != nil {
				return err
			}
		}
		var jobIDs []string
		if err := tx.Model(&models.AgentCronJob{}).
			Where("project_id = ? AND agent_name = ?", projectID, agentName).
			Pluck("id", &jobIDs).Error; err != nil {
			return err
		}
		if len(jobIDs) > 0 {
			if err := tx.Where("job_id IN ?", jobIDs).Delete(&models.AgentCronRun{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", jobIDs).Delete(&models.AgentCronJob{}).Error; err != nil {
				return err
			}
		}
		var p models.Project
		if err := tx.First(&p, "id = ?", projectID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if strings.TrimSpace(p.PmLeaderAgent) == agentName {
			if err := tx.Model(&p).Select("PmLeaderAgent", "PmLeaderEnabled", "UpdatedAt").
				Updates(map[string]any{
					"pm_leader_agent":   "",
					"pm_leader_enabled": false,
					"updated_at":        time.Now(),
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// PurgeAgentEverywhere removes all agent-scoped rows across every project and
// clears any PM Leader binding that points at the agent. Used on Agent delete.
func (s *PmService) PurgeAgentEverywhere(agentName string) error {
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_name = ?", agentName).Delete(&models.ProjectMemoryItem{}).Error; err != nil {
			return err
		}
		var threadIDs []string
		if err := tx.Model(&models.ChatThread{}).Where("agent_name = ?", agentName).
			Pluck("id", &threadIDs).Error; err != nil {
			return err
		}
		if len(threadIDs) > 0 {
			if err := tx.Where("thread_id IN ?", threadIDs).Delete(&models.ChatTurnDraft{}).Error; err != nil {
				return err
			}
			if err := tx.Where("thread_id IN ?", threadIDs).Delete(&models.ChatMessage{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", threadIDs).Delete(&models.ChatThread{}).Error; err != nil {
				return err
			}
		}
		var jobIDs []string
		if err := tx.Model(&models.AgentCronJob{}).Where("agent_name = ?", agentName).
			Pluck("id", &jobIDs).Error; err != nil {
			return err
		}
		if len(jobIDs) > 0 {
			if err := tx.Where("job_id IN ?", jobIDs).Delete(&models.AgentCronRun{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", jobIDs).Delete(&models.AgentCronJob{}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.Project{}).
			Where("pm_leader_agent = ?", agentName).
			Updates(map[string]any{
				"pm_leader_agent":   "",
				"pm_leader_enabled": false,
				"updated_at":        time.Now(),
			}).Error
	})
}

// RenameAgentScopedData rewrites agent_name on memories/threads/cron and PM
// Leader refs when an Agent is renamed. Scoped to all projects (agent names are global).
func (s *PmService) RenameAgentScopedData(oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return nil
	}
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.ProjectMemoryItem{}).Where("agent_name = ?", oldName).
			Update("agent_name", newName).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.ChatThread{}).Where("agent_name = ?", oldName).
			Updates(map[string]any{"agent_name": newName, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.AgentCronJob{}).Where("agent_name = ?", oldName).
			Updates(map[string]any{"agent_name": newName, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&models.Project{}).Where("pm_leader_agent = ?", oldName).
			Updates(map[string]any{"pm_leader_agent": newName, "updated_at": now}).Error
	})
}

// --- chat threads / messages ------------------------------------------------

// ListThreads returns threads for the given user in the project.
// For normal Web users this is their own threads ∪ project QQ channel threads
// (user_id prefix qq:), excluding cron: threads, ordered by updated_at desc.
// For synthetic identities (qq:/cron:) it stays owner-only so ChannelBridge
// ensureThread continues to resolve a single conversation thread.
func (s *PmService) ListThreads(projectID, userID string) ([]models.ChatThread, error) {
	if _, ok := s.project(projectID); !ok {
		return nil, ErrProjectNotFound
	}
	var threads []models.ChatThread
	q := s.db.Where("project_id = ?", projectID)
	if IsSyntheticThreadUserID(userID) {
		q = q.Where("user_id = ?", userID)
	} else {
		// Own Web sessions ∪ qq: channel sessions; cron: never matches either arm.
		q = q.Where("user_id = ? OR user_id LIKE ?", userID, "qq:%")
	}
	if err := q.Order("updated_at desc").Find(&threads).Error; err != nil {
		return nil, err
	}
	return threads, nil
}

// CreateThread inserts a new private thread for the user.
// agentName scopes the thread for context-store isolation; kind defaults to user.
func (s *PmService) CreateThread(projectID, userID, title, agentName, kind string) (models.ChatThread, error) {
	if _, ok := s.project(projectID); !ok {
		return models.ChatThread{}, ErrProjectNotFound
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新会话"
	}
	if kind == "" {
		kind = models.ChatThreadKindUser
	}
	if agentName == "" {
		if p, ok := s.project(projectID); ok {
			agentName = p.PmLeaderAgent
		}
	}
	now := time.Now()
	t := models.ChatThread{
		ID: "thr-" + uuid.NewString()[:12], ProjectID: projectID, UserID: userID,
		AgentName: agentName, Kind: kind, Title: title, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.Create(&t).Error; err != nil {
		return models.ChatThread{}, err
	}
	return t, nil
}

// CreateCronThread creates an exclusive kind=cron thread for a scheduled job.
func (s *PmService) CreateCronThread(projectID, agentName, title string) (models.ChatThread, error) {
	return s.CreateThread(projectID, "cron:"+agentName, title, agentName, models.ChatThreadKindCron)
}

// GetThread returns a thread the user may read: owned by userID, or a QQ
// channel thread in the same project (readable by any project member).
func (s *PmService) GetThread(projectID, threadID, userID string) (models.ChatThread, error) {
	var t models.ChatThread
	if err := s.db.Where("id = ? AND project_id = ?", threadID, projectID).First(&t).Error; err != nil {
		return models.ChatThread{}, ErrPmThreadNotFound
	}
	if t.UserID == userID || IsQQChannelUserID(t.UserID) {
		return t, nil
	}
	return models.ChatThread{}, ErrPmThreadNotFound
}

// RequireWritableThread loads a readable thread and rejects QQ channel threads
// for Web write/delete/turn paths.
func (s *PmService) RequireWritableThread(projectID, threadID, userID string) (models.ChatThread, error) {
	t, err := s.GetThread(projectID, threadID, userID)
	if err != nil {
		return models.ChatThread{}, err
	}
	if IsQQChannelUserID(t.UserID) {
		return models.ChatThread{}, ErrPmChannelReadOnly
	}
	return t, nil
}

// GetThreadByID loads a thread without user check (PM MCP internal).
func (s *PmService) GetThreadByID(threadID string) (models.ChatThread, error) {
	var t models.ChatThread
	if err := s.db.Where("id = ?", threadID).First(&t).Error; err != nil {
		return models.ChatThread{}, ErrPmThreadNotFound
	}
	return t, nil
}

// SetThreadAgentName backfills agent_name on a legacy thread.
func (s *PmService) SetThreadAgentName(threadID, agentName string) error {
	return s.db.Model(&models.ChatThread{}).Where("id = ?", threadID).
		Updates(map[string]any{"agent_name": agentName, "updated_at": time.Now()}).Error
}

// BindSandbox stores the sandbox id on the thread.
func (s *PmService) BindSandbox(threadID string, sandboxID uint) error {
	ref := fmt.Sprintf("%d", sandboxID)
	return s.db.Model(&models.ChatThread{}).Where("id = ?", threadID).
		Updates(map[string]any{"sandbox_ref": ref, "updated_at": time.Now()}).Error
}

// ClearSandboxRef clears a dead sandbox binding.
func (s *PmService) ClearSandboxRef(threadID string) error {
	return s.db.Model(&models.ChatThread{}).Where("id = ?", threadID).
		Updates(map[string]any{"sandbox_ref": "", "updated_at": time.Now()}).Error
}

// ListMessages returns messages for a thread (oldest first).
func (s *PmService) ListMessages(threadID string) ([]models.ChatMessage, error) {
	var msgs []models.ChatMessage
	if err := s.db.Where("thread_id = ?", threadID).Order("created_at asc").Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

// ListMessagesWindow returns a newest-tail or before-cursor page of messages
// (oldest→newest). Without beforeID it returns the most recent `limit` rows;
// with beforeID it returns up to `limit` rows strictly older than that message.
// hasMore is true when older messages remain beyond the returned window.
// limit is clamped to [1, 100] (default 20). Unknown beforeID yields ErrPmMessageNotFound.
func (s *PmService) ListMessagesWindow(threadID string, limit int, beforeID string) ([]models.ChatMessage, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	q := s.db.Where("thread_id = ?", threadID)
	if beforeID != "" {
		anchor, err := s.GetMessage(threadID, beforeID)
		if err != nil {
			return nil, false, err
		}
		// Strictly older than the anchor (created_at, then id for same-timestamp ties).
		q = q.Where(
			"(created_at < ?) OR (created_at = ? AND id < ?)",
			anchor.CreatedAt, anchor.CreatedAt, anchor.ID,
		)
	}
	var newestFirst []models.ChatMessage
	if err := q.Order("created_at desc, id desc").Limit(limit + 1).Find(&newestFirst).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(newestFirst) > limit
	if hasMore {
		newestFirst = newestFirst[:limit]
	}
	// Reverse to oldest→newest for chat UI consumption.
	for i, j := 0, len(newestFirst)-1; i < j; i, j = i+1, j-1 {
		newestFirst[i], newestFirst[j] = newestFirst[j], newestFirst[i]
	}
	return newestFirst, hasMore, nil
}

// AppendMessage persists one chat message and bumps thread updated_at.
// Optional source tags the turn origin (user | cron); empty keeps legacy rows.
func (s *PmService) AppendMessage(threadID, role, content string, citations []models.ProgressCitation, attached *models.AttachedContext, images []models.PromptImage) (models.ChatMessage, error) {
	return s.AppendMessageSource(threadID, role, content, "", citations, attached, images)
}

// AppendMessageSource is AppendMessage with an explicit source tag.
func (s *PmService) AppendMessageSource(threadID, role, content, source string, citations []models.ProgressCitation, attached *models.AttachedContext, images []models.PromptImage) (models.ChatMessage, error) {
	if role == "" {
		return models.ChatMessage{}, fmt.Errorf("role required")
	}
	msg := models.ChatMessage{
		ID: "msg-" + uuid.NewString()[:12], ThreadID: threadID, Role: role, Content: content,
		Status: "ok", Source: source, Images: images, Citations: citations, AttachedContext: attached, CreatedAt: time.Now(),
	}
	if err := s.db.Create(&msg).Error; err != nil {
		return models.ChatMessage{}, err
	}
	if err := s.db.Model(&models.ChatThread{}).Where("id = ?", threadID).
		Updates(map[string]any{"updated_at": time.Now()}).Error; err != nil {
		log.Warn().Err(err).Str("thread", threadID).Msg("bump thread updated_at failed")
	}
	// Auto-title from first user message.
	if role == "user" {
		var t models.ChatThread
		if err := s.db.First(&t, "id = ?", threadID).Error; err == nil && (t.Title == "" || t.Title == "新会话") {
			title := strings.TrimSpace(content)
			if title == "" && len(images) > 0 {
				title = "图片消息"
			}
			if len([]rune(title)) > 40 {
				title = string([]rune(title)[:40]) + "…"
			}
			if title != "" {
				if err := s.db.Model(&t).Update("title", title).Error; err != nil {
					log.Warn().Err(err).Str("thread", threadID).Msg("auto-title thread failed")
				}
			}
		}
	}
	return msg, nil
}

// GetMessage loads one message by id within a thread.
func (s *PmService) GetMessage(threadID, messageID string) (models.ChatMessage, error) {
	var m models.ChatMessage
	if err := s.db.Where("id = ? AND thread_id = ?", messageID, threadID).First(&m).Error; err != nil {
		return models.ChatMessage{}, ErrPmMessageNotFound
	}
	return m, nil
}

// UpdateMessageFailure marks or clears failure metadata on a user message.
// status "failed" requires a valid failKind; status "ok" clears failKind.
// Only role=user messages may be updated (assistant/system are rejected).
func (s *PmService) UpdateMessageFailure(threadID, messageID, status, failKind string) (models.ChatMessage, error) {
	msg, err := s.GetMessage(threadID, messageID)
	if err != nil {
		return models.ChatMessage{}, err
	}
	if msg.Role != "user" {
		return models.ChatMessage{}, ErrPmMessageInvalidRole
	}
	switch status {
	case "ok", "":
		status = "ok"
		failKind = ""
	case "failed":
		if !validPmFailKind(failKind) {
			return models.ChatMessage{}, ErrPmMessageInvalidStatus
		}
	default:
		return models.ChatMessage{}, ErrPmMessageInvalidStatus
	}
	if err := s.db.Model(&models.ChatMessage{}).Where("id = ? AND thread_id = ?", messageID, threadID).
		Updates(map[string]any{"status": status, "fail_kind": failKind}).Error; err != nil {
		return models.ChatMessage{}, err
	}
	msg.Status = status
	msg.FailKind = failKind
	if err := s.db.Model(&models.ChatThread{}).Where("id = ?", threadID).
		Updates(map[string]any{"updated_at": time.Now()}).Error; err != nil {
		log.Warn().Err(err).Str("thread", threadID).Msg("bump thread updated_at failed")
	}
	return msg, nil
}

// RecentMessages returns the last n non-failed messages for context injection.
// Failed turns (and any failure placeholders) are skipped so retries do not pollute the agent preamble.
func (s *PmService) RecentMessages(threadID string, n int) ([]models.ChatMessage, error) {
	if n <= 0 {
		n = 20
	}
	var msgs []models.ChatMessage
	// Over-fetch then filter so we still return up to n usable turns after skipping failures.
	if err := s.db.Where("thread_id = ?", threadID).Order("created_at desc").Limit(n * 3).Find(&msgs).Error; err != nil {
		return nil, err
	}
	filtered := make([]models.ChatMessage, 0, n)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Status == "failed" {
			continue
		}
		filtered = append(filtered, m)
	}
	if len(filtered) > n {
		filtered = filtered[len(filtered)-n:]
	}
	return filtered, nil
}

// ListConversationsForAgent returns threads visible to a context-store session.
// Interactive users see their own user threads plus the agent's cron threads.
// Cron sessions (userID == "cron") only see cron threads for the agent.
func (s *PmService) ListConversationsForAgent(projectID, agentName, userID string) ([]models.ChatThread, error) {
	if _, ok := s.project(projectID); !ok {
		return nil, ErrProjectNotFound
	}
	q := s.db.Where("project_id = ? AND agent_name = ?", projectID, agentName)
	switch {
	case userID == "cron":
		q = q.Where("kind = ?", models.ChatThreadKindCron)
	case userID != "":
		q = q.Where(
			"(kind = ? OR ((kind = ? OR kind = '' OR kind IS NULL) AND user_id = ?))",
			models.ChatThreadKindCron, models.ChatThreadKindUser, userID,
		)
	}
	var threads []models.ChatThread
	if err := q.Order("updated_at desc").Find(&threads).Error; err != nil {
		return nil, err
	}
	return threads, nil
}

// CountMessagesByThreads returns message counts keyed by thread id.
func (s *PmService) CountMessagesByThreads(threadIDs []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(threadIDs) == 0 {
		return out, nil
	}
	type row struct {
		ThreadID string
		N        int64
	}
	var rows []row
	if err := s.db.Model(&models.ChatMessage{}).
		Select("thread_id as thread_id, count(*) as n").
		Where("thread_id IN ?", threadIDs).
		Group("thread_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ThreadID] = r.N
	}
	return out, nil
}

// GetMessagesPage returns messages for a thread with limit/offset (oldest-first page).
func (s *PmService) GetMessagesPage(threadID string, limit, offset int) ([]models.ChatMessage, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := s.db.Model(&models.ChatMessage{}).Where("thread_id = ?", threadID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var msgs []models.ChatMessage
	if err := s.db.Where("thread_id = ?", threadID).Order("created_at asc").
		Offset(offset).Limit(limit).Find(&msgs).Error; err != nil {
		return nil, 0, err
	}
	return msgs, total, nil
}

// SearchMessages finds messages visible to a context-store session by keyword.
func (s *PmService) SearchMessages(projectID, agentName, userID, q string, limit int) ([]map[string]any, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("query required")
	}
	if limit <= 0 {
		limit = 20
	}
	threads, err := s.ListConversationsForAgent(projectID, agentName, userID)
	if err != nil {
		return nil, err
	}
	if len(threads) == 0 {
		return []map[string]any{}, nil
	}
	ids := make([]string, len(threads))
	titleBy := map[string]string{}
	for i, t := range threads {
		ids[i] = t.ID
		titleBy[t.ID] = t.Title
	}
	var msgs []models.ChatMessage
	like := "%" + EscapeLike(q) + "%"
	if err := s.db.Where("thread_id IN ? AND content LIKE ? ESCAPE ?", ids, like, `\`).
		Order("created_at desc").Limit(limit).Find(&msgs).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		snippet := m.Content
		if len([]rune(snippet)) > 160 {
			snippet = string([]rune(snippet)[:160]) + "…"
		}
		out = append(out, map[string]any{
			"messageId": m.ID, "conversationId": m.ThreadID,
			"title": titleBy[m.ThreadID], "role": m.Role, "snippet": snippet,
			"createdAt": m.CreatedAt,
		})
	}
	return out, nil
}

// GetMemory returns one memory by id for project+agent.
func (s *PmService) GetMemory(projectID, agentName, id string) (models.ProjectMemoryItem, error) {
	var item models.ProjectMemoryItem
	if err := s.db.Where("id = ? AND project_id = ? AND agent_name = ?", id, projectID, agentName).
		First(&item).Error; err != nil {
		return models.ProjectMemoryItem{}, ErrPmMemoryNotFound
	}
	return item, nil
}

// SearchMemories finds memories by title/content keyword for an agent.
func (s *PmService) SearchMemories(projectID, agentName, q string, limit int) ([]map[string]any, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("query required")
	}
	if limit <= 0 {
		limit = 20
	}
	like := "%" + EscapeLike(q) + "%"
	var items []models.ProjectMemoryItem
	if err := s.db.Where("project_id = ? AND agent_name = ? AND (title LIKE ? ESCAPE ? OR content LIKE ? ESCAPE ?)",
		projectID, agentName, like, `\`, like, `\`).Order("updated_at desc").Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		summary := it.Content
		if len([]rune(summary)) > 120 {
			summary = string([]rune(summary)[:120]) + "…"
		}
		out = append(out, map[string]any{
			"id": it.ID, "title": it.Title, "summary": summary, "updatedAt": it.UpdatedAt,
		})
	}
	return out, nil
}

// DeleteThread removes a thread and its messages (owner only; channel threads rejected).
func (s *PmService) DeleteThread(projectID, threadID, userID string) error {
	if _, err := s.RequireWritableThread(projectID, threadID, userID); err != nil {
		return err
	}
	return s.DeleteThreadByID(threadID)
}

// DeleteThreadByID removes a thread and its messages without an ownership check
// (used for cron job cleanup / failed job create rollback).
func (s *PmService) DeleteThreadByID(threadID string) error {
	if strings.TrimSpace(threadID) == "" {
		return nil
	}
	_ = s.db.Where("thread_id = ?", threadID).Delete(&models.ChatTurnDraft{}).Error
	if err := s.db.Where("thread_id = ?", threadID).Delete(&models.ChatMessage{}).Error; err != nil {
		return err
	}
	return s.db.Where("id = ?", threadID).Delete(&models.ChatThread{}).Error
}

// ListCronJobs returns all AgentCronJob rows for a project (any agent).
func (s *PmService) ListCronJobs(projectID string) ([]models.AgentCronJob, error) {
	if _, ok := s.project(projectID); !ok {
		return nil, ErrProjectNotFound
	}
	var jobs []models.AgentCronJob
	if err := s.db.Where("project_id = ?", projectID).Order("updated_at desc").Find(&jobs).Error; err != nil {
		return nil, err
	}
	if jobs == nil {
		jobs = []models.AgentCronJob{}
	}
	return jobs, nil
}

// PatchCronJobDeliver updates deliverToChannel for a job scoped to projectID.
func (s *PmService) PatchCronJobDeliver(projectID, jobID string, deliver bool) (models.AgentCronJob, error) {
	if _, ok := s.project(projectID); !ok {
		return models.AgentCronJob{}, ErrProjectNotFound
	}
	var job models.AgentCronJob
	if err := s.db.Where("id = ? AND project_id = ?", jobID, projectID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.AgentCronJob{}, ErrPmCronJobNotFound
		}
		return models.AgentCronJob{}, err
	}
	job.DeliverToChannel = deliver
	job.UpdatedAt = time.Now().UTC()
	if err := s.db.Model(&job).Select("DeliverToChannel", "UpdatedAt").Updates(job).Error; err != nil {
		return models.AgentCronJob{}, err
	}
	return job, nil
}

// GetThreadForAgent returns one thread when it belongs to project+agent.
func (s *PmService) GetThreadForAgent(projectID, agentName, threadID string) (models.ChatThread, error) {
	agentName = strings.TrimSpace(agentName)
	var t models.ChatThread
	if err := s.db.Where("id = ? AND project_id = ? AND agent_name = ?", threadID, projectID, agentName).
		First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.ChatThread{}, ErrPmThreadNotFound
		}
		return models.ChatThread{}, err
	}
	return t, nil
}

// ListThreadsForAgent returns every thread (any owner/kind) for a project+agent.
// Used by Agent Studio context management (not the per-user consult sidebar).
func (s *PmService) ListThreadsForAgent(projectID, agentName string) ([]models.ChatThread, error) {
	if _, ok := s.project(projectID); !ok {
		return nil, ErrProjectNotFound
	}
	agentName = strings.TrimSpace(agentName)
	var threads []models.ChatThread
	if err := s.db.Where("project_id = ? AND agent_name = ?", projectID, agentName).
		Order("updated_at desc").Find(&threads).Error; err != nil {
		return nil, err
	}
	if threads == nil {
		threads = []models.ChatThread{}
	}
	return threads, nil
}

// DeleteThreadForAgent deletes a thread only when it belongs to project+agent.
func (s *PmService) DeleteThreadForAgent(projectID, agentName, threadID string) error {
	agentName = strings.TrimSpace(agentName)
	threadID = strings.TrimSpace(threadID)
	var t models.ChatThread
	if err := s.db.Where("id = ? AND project_id = ? AND agent_name = ?", threadID, projectID, agentName).
		First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPmThreadNotFound
		}
		return err
	}
	return s.DeleteThreadByID(threadID)
}

// ListCronJobsForAgent returns jobs scoped to one project+agent.
func (s *PmService) ListCronJobsForAgent(projectID, agentName string) ([]models.AgentCronJob, error) {
	if _, ok := s.project(projectID); !ok {
		return nil, ErrProjectNotFound
	}
	agentName = strings.TrimSpace(agentName)
	var jobs []models.AgentCronJob
	if err := s.db.Where("project_id = ? AND agent_name = ?", projectID, agentName).
		Order("updated_at desc").Find(&jobs).Error; err != nil {
		return nil, err
	}
	if jobs == nil {
		jobs = []models.AgentCronJob{}
	}
	return jobs, nil
}

// PatchCronJobForAgent toggles enabled and/or deliverToChannel for a job.
func (s *PmService) PatchCronJobForAgent(projectID, agentName, jobID string, enabled, deliver *bool) (models.AgentCronJob, error) {
	agentName = strings.TrimSpace(agentName)
	var job models.AgentCronJob
	if err := s.db.Where("id = ? AND project_id = ? AND agent_name = ?", jobID, projectID, agentName).
		First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.AgentCronJob{}, ErrPmCronJobNotFound
		}
		return models.AgentCronJob{}, err
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if enabled != nil {
		updates["enabled"] = *enabled
		job.Enabled = *enabled
	}
	if deliver != nil {
		updates["deliver_to_channel"] = *deliver
		job.DeliverToChannel = *deliver
	}
	if err := s.db.Model(&job).Updates(updates).Error; err != nil {
		return models.AgentCronJob{}, err
	}
	return job, nil
}

// DeleteCronJobForAgent removes a job, its runs, and exclusive cron thread.
func (s *PmService) DeleteCronJobForAgent(projectID, agentName, jobID string) error {
	agentName = strings.TrimSpace(agentName)
	var job models.AgentCronJob
	if err := s.db.Where("id = ? AND project_id = ? AND agent_name = ?", jobID, projectID, agentName).
		First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPmCronJobNotFound
		}
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ?", job.ID).Delete(&models.AgentCronRun{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", job.ID).Delete(&models.AgentCronJob{}).Error; err != nil {
			return err
		}
		if strings.TrimSpace(job.ThreadID) == "" {
			return nil
		}
		if err := tx.Where("thread_id = ?", job.ThreadID).Delete(&models.ChatTurnDraft{}).Error; err != nil {
			return err
		}
		if err := tx.Where("thread_id = ?", job.ThreadID).Delete(&models.ChatMessage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", job.ThreadID).Delete(&models.ChatThread{}).Error
	})
}

// Draft status constants.
const (
	PmDraftStreaming = "streaming"
	PmDraftDone      = "done"
	PmDraftFailed    = "failed"
)

// GetDraft returns the active draft for a thread, or nil when absent.
func (s *PmService) GetDraft(threadID string) (*models.ChatTurnDraft, error) {
	var d models.ChatTurnDraft
	err := s.db.Where("thread_id = ?", threadID).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UpsertDraft creates or updates the per-thread streaming checkpoint.
func (s *PmService) UpsertDraft(threadID, userMsgID, partialText, status string, chunkIndex, eventSeq int, sandboxID uint) (models.ChatTurnDraft, error) {
	if status == "" {
		status = PmDraftStreaming
	}
	now := time.Now()
	existing, err := s.GetDraft(threadID)
	if err != nil {
		return models.ChatTurnDraft{}, err
	}
	if existing == nil {
		d := models.ChatTurnDraft{
			ID:          "draft-" + uuid.NewString()[:12],
			ThreadID:    threadID,
			UserMsgID:   userMsgID,
			PartialText: partialText,
			ChunkIndex:  chunkIndex,
			EventSeq:    eventSeq,
			Status:      status,
			SandboxID:   sandboxID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.db.Create(&d).Error; err != nil {
			return models.ChatTurnDraft{}, err
		}
		return d, nil
	}
	updates := map[string]any{
		"user_msg_id":  userMsgID,
		"partial_text": partialText,
		"chunk_index":  chunkIndex,
		"event_seq":    eventSeq,
		"status":       status,
		"sandbox_id":   sandboxID,
		"updated_at":   now,
		"fail_kind":    "",
	}
	if err := s.db.Model(&models.ChatTurnDraft{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return models.ChatTurnDraft{}, err
	}
	existing.UserMsgID = userMsgID
	existing.PartialText = partialText
	existing.ChunkIndex = chunkIndex
	existing.EventSeq = eventSeq
	existing.Status = status
	existing.SandboxID = sandboxID
	existing.FailKind = ""
	existing.UpdatedAt = now
	return *existing, nil
}

// PatchDraftPartial updates only the streaming text progress (hot path).
func (s *PmService) PatchDraftPartial(threadID, partialText string, chunkIndex, eventSeq int) error {
	return s.db.Model(&models.ChatTurnDraft{}).Where("thread_id = ? AND status = ?", threadID, PmDraftStreaming).
		Updates(map[string]any{
			"partial_text": partialText,
			"chunk_index":  chunkIndex,
			"event_seq":    eventSeq,
			"updated_at":   time.Now(),
		}).Error
}

// FailDraft marks the draft failed (keeps partial for hydrate diagnostics).
func (s *PmService) FailDraft(threadID, failKind string) error {
	if failKind == "" {
		failKind = PmFailUnknown
	}
	return s.db.Model(&models.ChatTurnDraft{}).Where("thread_id = ?", threadID).
		Updates(map[string]any{
			"status":     PmDraftFailed,
			"fail_kind":  failKind,
			"updated_at": time.Now(),
		}).Error
}

// ClearDraft removes the thread draft after finalize or discard.
func (s *PmService) ClearDraft(threadID string) error {
	return s.db.Where("thread_id = ?", threadID).Delete(&models.ChatTurnDraft{}).Error
}

// HasAssistantAfter reports whether an assistant message exists after the given user message.
func (s *PmService) HasAssistantAfter(threadID, userMsgID string) (bool, error) {
	var user models.ChatMessage
	if err := s.db.Where("id = ? AND thread_id = ?", userMsgID, threadID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	var count int64
	err := s.db.Model(&models.ChatMessage{}).
		Where("thread_id = ? AND role = ? AND created_at >= ?", threadID, "assistant", user.CreatedAt).
		Count(&count).Error
	return count > 0, err
}
