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

// ConfirmationPrompt asks the user to authorize a destructive action. It always
// names the task, because "confirm?" with no subject is how the wrong thing gets
// cancelled — but it asks the way a person would.
func (s *RiskConfirmationService) ConfirmationPrompt(ticket models.RiskConfirmationTicket) string {
	title := SanitizeShortTitle(ticket.ShortTitle)
	action := riskActionPhrase(ticket.Action, ticket.Language)
	if NormalizeLanguage(ticket.Language) == "en" {
		subject := "that task"
		if title != "" {
			subject = "\"" + title + "\""
		}
		return "Just to be sure — you want me to " + action + " " + subject + "? " + riskPrompt(ticket.Language)
	}
	subject := "这个任务"
	if title != "" {
		subject = "「" + title + "」"
	}
	return "确认一下，你是要" + action + subject + "吗？" + riskPrompt(ticket.Language)
}

// riskActionPhrase names a destructive action as a verb rather than an action
// code. Callers may also pass a plain-language description, which is used as-is;
// only the internal codes need translating.
func riskActionPhrase(action, language string) string {
	en := NormalizeLanguage(language) == "en"
	action = strings.TrimSpace(action)
	base, _, _ := strings.Cut(action, ":")
	switch base {
	case "cancel_run":
		if en {
			return "cancel"
		}
		return "取消"
	case "delete_run":
		if en {
			return "delete"
		}
		return "删除"
	case "approve_gate", "resume_gate":
		if en {
			return "approve"
		}
		return "批准"
	case "reject_gate":
		if en {
			return "reject"
		}
		return "驳回"
	default:
		// Not a known code. Anything with code punctuation is internal and must
		// not be read out; anything else is already a human description.
		if action != "" && !strings.ContainsAny(action, "_:") {
			return action
		}
		if en {
			return "run that action on"
		}
		return "操作"
	}
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

// MarkPrompted records that a ticket's question reached the user. Until this is
// set the ticket is invisible to LatestAnswerable, which is what keeps an
// undelivered question from swallowing the answer to a different one.
func (s *RiskConfirmationService) MarkPrompted(ticketID string) error {
	if s == nil || s.db == nil {
		return errors.New("risk confirmation database is unavailable")
	}
	now := s.now()
	return s.db.Model(&models.RiskConfirmationTicket{}).
		Where("id = ? AND prompted_at IS NULL", strings.TrimSpace(ticketID)).
		Updates(map[string]any{"prompted_at": now, "updated_at": now}).Error
}

// LatestAnswerable returns the pending ticket a bare confirmation belongs to:
// the most recent one whose question the user actually received.
//
// The plain newest-pending ticket is the wrong answer and was a real incident.
// An agent asked to cancel task A, the user hesitated, the agent then asked to
// cancel task B and that prompt was suppressed on the way out; the user's
// 「确认」 — meant for the only question they had seen — settled B and cancelled
// the wrong task. Confirming one thing must never destroy another, so a ticket
// the user never saw is not a candidate.
func (s *RiskConfirmationService) LatestAnswerable(userID, projectID string) (*models.RiskConfirmationTicket, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("risk confirmation database is unavailable")
	}
	var ticket models.RiskConfirmationTicket
	err := s.db.Where("project_id = ? AND user_id = ? AND status = ? AND prompted_at IS NOT NULL",
		strings.TrimSpace(projectID), strings.TrimSpace(userID), "pending").
		Order("prompted_at desc").First(&ticket).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// StatusMessageFor renders the settled-ticket reply against the task's status
// right now. It exists because the reply has to be built after the action runs:
// rendering it beforehand produced 「已经取消了。现在是 running」, which told the
// user two contradictory things about the same task in one breath.
func (s *RiskConfirmationService) StatusMessageFor(ticket models.RiskConfirmationTicket) string {
	if s == nil || s.db == nil {
		return riskStatusMessage(ticket, "")
	}
	return riskStatusMessage(ticket, s.latestTaskStatus(s.db, ticket))
}

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
		return "Say “confirm” and I'll do it, or “cancel” to leave it alone."
	}
	return "回「确认」我就做，回「取消」就算了。"
}

// riskStatusMessage reports how a confirmation settled. It names the task only
// when the outcome would otherwise be ambiguous, and states what happened rather
// than the ticket's internal state.
func riskStatusMessage(ticket models.RiskConfirmationTicket, taskStatus string) string {
	en := NormalizeLanguage(ticket.Language) == "en"
	title := SanitizeShortTitle(ticket.ShortTitle)
	action := riskActionPhrase(ticket.Action, ticket.Language)

	var subject string
	if en {
		subject = "that one"
		if title != "" {
			subject = "\"" + title + "\""
		}
	} else {
		subject = "那个"
		if title != "" {
			subject = "「" + title + "」"
		}
	}

	var message string
	switch ticket.Status {
	case "confirmed":
		if en {
			message = "Already done — " + subject + " was " + pastTense(action) + "."
		} else {
			message = subject + "已经" + action + "了。"
		}
	case "cancelled":
		if en {
			message = "OK, I left " + subject + " alone."
		} else {
			message = "好，" + subject + "我没动。"
		}
	case "expired":
		if en {
			message = "That confirmation timed out, so I didn't touch " + subject + ". Tell me again if you still want it."
		} else {
			message = "刚才那个确认过期了，" + subject + "我没动。还要的话再说一次。"
		}
	default:
		return riskPrompt(ticket.Language)
	}
	if status := strings.TrimSpace(taskStatus); status != "" {
		if en {
			message += " It's now " + status + "."
		} else {
			message += "现在是" + status + "。"
		}
	}
	return message
}

// pastTense renders an action verb for an outcome sentence.
func pastTense(verb string) string {
	switch verb {
	case "cancel":
		return "cancelled"
	case "delete":
		return "deleted"
	case "approve":
		return "approved"
	case "reject":
		return "rejected"
	default:
		return "handled"
	}
}
