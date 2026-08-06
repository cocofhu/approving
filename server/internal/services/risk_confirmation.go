package services

import (
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const RiskConfirmationTTL = 5 * time.Minute

type RiskConfirmationService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewRiskConfirmationService(db *gorm.DB) *RiskConfirmationService {
	return &RiskConfirmationService{db: db, now: time.Now}
}

func (s *RiskConfirmationService) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

type RiskTicketInput struct {
	ProjectID, UserID, RunID, Action, Language string
	// ShortTitle overrides the task title snapshot; empty resolves it from the
	// Run's task identity so callers do not have to pass it.
	ShortTitle string
}

func (s *RiskConfirmationService) CreateTicket(in RiskTicketInput) (*models.RiskConfirmationTicket, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("risk confirmation database is unavailable")
	}
	if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.UserID) == "" ||
		strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.Action) == "" {
		return nil, errors.New("project_id, user_id, run_id and action are required")
	}
	now := s.now()
	ticket := models.RiskConfirmationTicket{
		ID: "risk-" + uuid.NewString()[:12], ProjectID: strings.TrimSpace(in.ProjectID),
		UserID: strings.TrimSpace(in.UserID), RunID: strings.TrimSpace(in.RunID),
		ShortTitle: s.resolveShortTitle(in),
		Action:     strings.TrimSpace(in.Action), Status: "pending",
		Language: DetectLanguage("", in.Language), ExpiresAt: now.Add(RiskConfirmationTTL),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.Create(&ticket).Error; err != nil {
		return nil, err
	}
	return &ticket, nil
}

// resolveShortTitle prefers the caller's snapshot, then the Run's task identity.
func (s *RiskConfirmationService) resolveShortTitle(in RiskTicketInput) string {
	if title := strings.TrimSpace(in.ShortTitle); title != "" {
		return title
	}
	var identity models.TaskIdentity
	if err := s.db.Where("run_id = ? AND project_id = ?",
		strings.TrimSpace(in.RunID), strings.TrimSpace(in.ProjectID)).First(&identity).Error; err == nil {
		return identity.ShortTitle
	}
	return ""
}

// ConfirmationPrompt is the user-facing ask, always echoing the short title.
func (s *RiskConfirmationService) ConfirmationPrompt(ticket models.RiskConfirmationTicket) string {
	kind := "高风险确认"
	if NormalizeLanguage(ticket.Language) == "en" {
		kind = "High-risk confirmation"
	}
	return FormatTaskType(ticket.ShortTitle, kind, ticket.Language) + " " +
		riskActionLine(ticket.Action, ticket.Language) + riskPrompt(ticket.Language)
}

type RiskResolution struct {
	Ticket  models.RiskConfirmationTicket
	Execute bool
	Message string
	// TaskStatus is the latest known task/Run status, so a repeated or expired
	// decision reports where the task actually stands, not just the ticket.
	TaskStatus string
}

// ResolveTicket atomically consumes a pending ticket. Repeated and expired
// decisions return the persisted latest status and never execute again.
func (s *RiskConfirmationService) ResolveTicket(in RiskTicketInput, response string) (RiskResolution, error) {
	var result RiskResolution
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var ticket models.RiskConfirmationTicket
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("project_id = ? AND user_id = ? AND run_id = ? AND action = ?",
				strings.TrimSpace(in.ProjectID), strings.TrimSpace(in.UserID),
				strings.TrimSpace(in.RunID), strings.TrimSpace(in.Action)).
			Order("created_at desc").First(&ticket).Error
		if err != nil {
			return err
		}
		now := s.now()
		if ticket.Status == "pending" && !ticket.ExpiresAt.After(now) {
			ticket.Status = "expired"
			ticket.ResolvedAt = &now
			ticket.UpdatedAt = now
			if err := tx.Save(&ticket).Error; err != nil {
				return err
			}
		}
		if ticket.Status != "pending" {
			result = s.resolutionFor(tx, ticket, false)
			return nil
		}
		decision := parseRiskDecision(response)
		if decision == "" {
			result = RiskResolution{
				Ticket: ticket, Message: s.ConfirmationPrompt(ticket),
				TaskStatus: s.latestTaskStatus(tx, ticket),
			}
			return nil
		}
		update := tx.Model(&models.RiskConfirmationTicket{}).
			Where("id = ? AND status = ? AND expires_at > ?", ticket.ID, "pending", now).
			Updates(map[string]any{"status": decision, "resolved_at": now, "updated_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			if err := tx.First(&ticket, "id = ?", ticket.ID).Error; err != nil {
				return err
			}
			result = s.resolutionFor(tx, ticket, false)
			return nil
		}
		if err := tx.First(&ticket, "id = ?", ticket.ID).Error; err != nil {
			return err
		}
		result = s.resolutionFor(tx, ticket, decision == "confirmed")
		return nil
	})
	return result, err
}

// resolutionFor renders the reply for a settled ticket, echoing the short title
// and the latest task status.
func (s *RiskConfirmationService) resolutionFor(tx *gorm.DB, ticket models.RiskConfirmationTicket, execute bool) RiskResolution {
	taskStatus := s.latestTaskStatus(tx, ticket)
	return RiskResolution{
		Ticket: ticket, Execute: execute, TaskStatus: taskStatus,
		Message: riskStatusMessage(ticket, taskStatus),
	}
}

// latestTaskStatus prefers the live Run status and falls back to the task
// identity snapshot.
func (s *RiskConfirmationService) latestTaskStatus(tx *gorm.DB, ticket models.RiskConfirmationTicket) string {
	var run models.Run
	if err := tx.Select("status").First(&run, "id = ?", ticket.RunID).Error; err == nil {
		if status := strings.TrimSpace(run.Status); status != "" {
			return status
		}
	}
	var identity models.TaskIdentity
	if err := tx.Where("run_id = ? AND project_id = ?",
		ticket.RunID, ticket.ProjectID).First(&identity).Error; err == nil {
		return identity.Status
	}
	return ""
}

func parseRiskDecision(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "确认", "确认执行", "同意", "yes", "y", "confirm", "confirmed", "proceed":
		return "confirmed"
	case "取消", "不同意", "否", "no", "n", "cancel", "cancelled", "canceled":
		return "cancelled"
	default:
		return ""
	}
}

// ParseRiskDecisionPublic exposes confirmation keyword parsing for IM orchestration.
func ParseRiskDecisionPublic(value string) string { return parseRiskDecision(value) }

func (s *RiskConfirmationService) LatestPending(userID, projectID string) (*models.RiskConfirmationTicket, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("risk confirmation database is unavailable")
	}
	var ticket models.RiskConfirmationTicket
	err := s.db.Where("project_id = ? AND user_id = ? AND status = ?",
		strings.TrimSpace(projectID), strings.TrimSpace(userID), "pending").
		Order("created_at desc").First(&ticket).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (s *RiskConfirmationService) LatestAny(userID, projectID string) (*models.RiskConfirmationTicket, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("risk confirmation database is unavailable")
	}
	var ticket models.RiskConfirmationTicket
	err := s.db.Where("project_id = ? AND user_id = ?",
		strings.TrimSpace(projectID), strings.TrimSpace(userID)).
		Order("created_at desc").First(&ticket).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// LatestForAction returns the newest ticket for one exact user/project/run/action
// tuple. MCP retries use it to observe an IM-confirmed action without creating a
// second ticket or executing the mutation again.
func (s *RiskConfirmationService) LatestForAction(userID, projectID, runID, action string) (*models.RiskConfirmationTicket, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("risk confirmation database is unavailable")
	}
	var ticket models.RiskConfirmationTicket
	err := s.db.Where("project_id = ? AND user_id = ? AND run_id = ? AND action = ?",
		strings.TrimSpace(projectID), strings.TrimSpace(userID),
		strings.TrimSpace(runID), strings.TrimSpace(action)).
		Order("created_at desc").First(&ticket).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

func riskPrompt(language string) string {
	if NormalizeLanguage(language) == "en" {
		return "Reply “confirm” to proceed or “cancel” to stop."
	}
	return "请回复“确认”继续，或回复“取消”停止。"
}

func riskActionLine(action, language string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return ""
	}
	if NormalizeLanguage(language) == "en" {
		return "Action: " + action + ". "
	}
	return "操作：" + action + "。"
}

// riskStatusMessage renders the settled reply: task title, ticket outcome, and
// the latest task status.
func riskStatusMessage(ticket models.RiskConfirmationTicket, taskStatus string) string {
	en := NormalizeLanguage(ticket.Language) == "en"
	var outcome, kind string
	switch ticket.Status {
	case "confirmed":
		kind, outcome = "已确认", "已确认，本次授权已使用。"
		if en {
			kind, outcome = "Confirmed", "Confirmed. This authorization has already been consumed."
		}
	case "cancelled":
		kind, outcome = "已取消", "已取消，操作未执行。"
		if en {
			kind, outcome = "Cancelled", "Cancelled. The action was not executed."
		}
	case "expired":
		kind, outcome = "已过期", "确认已过期，操作未执行。"
		if en {
			kind, outcome = "Expired", "This confirmation has expired. The action was not executed."
		}
	default:
		kind, outcome = "待确认", riskPrompt(ticket.Language)
		if en {
			kind = "Pending"
		}
	}
	message := FormatTaskType(ticket.ShortTitle, kind, ticket.Language) + " " + outcome
	if status := strings.TrimSpace(taskStatus); status != "" {
		if en {
			message += " Latest task status: " + status + "."
		} else {
			message += "当前任务状态：" + status + "。"
		}
	}
	return message
}
