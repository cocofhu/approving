package services

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/cocofhu/approving/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ConversationFocusTTL = 30 * time.Minute
	TaskTerminalLookback = 30 * 24 * time.Hour
	RiskTicketTTL        = 5 * time.Minute
	// RiskGrantTTL bounds how long a confirmed ticket stays spendable by the
	// destructive server method.
	RiskGrantTTL = 5 * time.Minute
)

var (
	ErrTaskAmbiguous       = errors.New("task reference is ambiguous")
	ErrTaskNotFound        = errors.New("task not found")
	ErrConfirmationStale   = errors.New("confirmation is stale or already consumed")
	ErrInvalidConfirmation = errors.New("reply is not confirm or cancel")
	ErrActionNotAuthorized = errors.New("high-risk action has no confirmed authorization")
)

type TaskContextService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewTaskContextService(db *gorm.DB) *TaskContextService {
	return &TaskContextService{db: db, now: time.Now}
}

type TaskIdentityInput struct {
	RunID, ProjectID, UserID, ShortTitle, OriginalRequirement, Status string
	Aliases, Keywords                                                 []string
}

// UpsertTaskIdentity creates or updates a Run identity. A changed title is
// appended to aliases before replacement, preserving old references.
func (s *TaskContextService) UpsertTaskIdentity(in TaskIdentityInput) (models.TaskIdentity, error) {
	if s == nil || s.db == nil {
		return models.TaskIdentity{}, fmt.Errorf("task context unavailable")
	}
	in.RunID, in.ProjectID, in.UserID = strings.TrimSpace(in.RunID), strings.TrimSpace(in.ProjectID), strings.TrimSpace(in.UserID)
	in.ShortTitle = strings.TrimSpace(in.ShortTitle)
	if in.RunID == "" || in.ProjectID == "" || in.UserID == "" || in.ShortTitle == "" {
		return models.TaskIdentity{}, fmt.Errorf("run, project, user and short title are required")
	}
	var out models.TaskIdentity
	err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("run_id = ? AND project_id = ? AND user_id = ?", in.RunID, in.ProjectID, in.UserID).First(&out).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			out = models.TaskIdentity{
				RunID: in.RunID, ProjectID: in.ProjectID, UserID: in.UserID,
				ShortTitle: in.ShortTitle, OriginalRequirement: strings.TrimSpace(in.OriginalRequirement),
				Aliases: uniqueStrings(in.Aliases), Keywords: uniqueStrings(in.Keywords), Status: strings.TrimSpace(in.Status),
			}
			return tx.Create(&out).Error
		}
		if err != nil {
			return err
		}
		aliases := append([]string(nil), out.Aliases...)
		if out.ShortTitle != "" && !strings.EqualFold(out.ShortTitle, in.ShortTitle) {
			aliases = append(aliases, out.ShortTitle)
		}
		aliases = append(aliases, in.Aliases...)
		out.ProjectID, out.UserID, out.ShortTitle = in.ProjectID, in.UserID, in.ShortTitle
		if strings.TrimSpace(in.OriginalRequirement) != "" {
			out.OriginalRequirement = strings.TrimSpace(in.OriginalRequirement)
		}
		out.Aliases = uniqueStrings(aliases)
		out.Keywords = uniqueStrings(in.Keywords)
		if strings.TrimSpace(in.Status) != "" {
			out.Status = strings.TrimSpace(in.Status)
		}
		return tx.Save(&out).Error
	})
	return out, err
}

func (s *TaskContextService) UpdateTaskTitle(projectID, userID, runID, title string) (models.TaskIdentity, error) {
	var cur models.TaskIdentity
	if err := s.db.Where("project_id = ? AND user_id = ? AND run_id = ?", projectID, userID, runID).First(&cur).Error; err != nil {
		return cur, err
	}
	return s.UpsertTaskIdentity(TaskIdentityInput{
		RunID: cur.RunID, ProjectID: cur.ProjectID, UserID: cur.UserID, ShortTitle: title,
		OriginalRequirement: cur.OriginalRequirement, Aliases: cur.Aliases, Keywords: cur.Keywords, Status: cur.Status,
	})
}

func (s *TaskContextService) UpdateTaskStatus(projectID, userID, runID, status string) error {
	return s.db.Model(&models.TaskIdentity{}).
		Where("project_id = ? AND user_id = ? AND run_id = ?", projectID, userID, runID).
		Update("status", strings.TrimSpace(status)).Error
}

func (s *TaskContextService) GetTaskIdentity(projectID, userID, runID string) (models.TaskIdentity, error) {
	var out models.TaskIdentity
	err := s.db.Where("project_id = ? AND user_id = ? AND run_id = ?", projectID, userID, runID).First(&out).Error
	return out, err
}

func (s *TaskContextService) BindExternalMessage(binding models.MessageBinding) error {
	if s == nil || s.db == nil || strings.TrimSpace(binding.MessageID) == "" || strings.TrimSpace(binding.RunID) == "" {
		return fmt.Errorf("message binding requires message and run")
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "channel"}, {Name: "conversation_id"}, {Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "run_id"}),
	}).Create(&binding).Error
}

func (s *TaskContextService) GetMessageBinding(projectID, channel, conversationID, messageID string) (models.MessageBinding, error) {
	var out models.MessageBinding
	err := s.db.Where("project_id = ? AND channel = ? AND conversation_id = ? AND message_id = ?",
		projectID, channel, conversationID, messageID).First(&out).Error
	return out, err
}

func (s *TaskContextService) TouchConversationFocus(projectID, channel, conversationID, userID, runID string) error {
	now := s.now()
	row := models.ConversationFocus{
		ProjectID: projectID, Channel: channel, ConversationID: conversationID, UserID: userID,
		RunID: runID, PendingRunIDs: []string{}, ExpiresAt: now.Add(ConversationFocusTTL), UpdatedAt: now,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "channel"}, {Name: "conversation_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"run_id", "pending_run_ids", "expires_at", "updated_at"}),
	}).Create(&row).Error
}

func (s *TaskContextService) SetAmbiguityCandidates(projectID, channel, conversationID, userID string, runIDs []string) error {
	now := s.now()
	row := models.ConversationFocus{
		ProjectID: projectID, Channel: channel, ConversationID: conversationID, UserID: userID,
		PendingRunIDs: uniqueStrings(runIDs), ExpiresAt: now.Add(ConversationFocusTTL), UpdatedAt: now,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "channel"}, {Name: "conversation_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"run_id", "pending_run_ids", "expires_at", "updated_at"}),
	}).Create(&row).Error
}

func (s *TaskContextService) GetConversationFocus(projectID, channel, conversationID, userID string) (models.ConversationFocus, error) {
	var out models.ConversationFocus
	err := s.db.Where("project_id = ? AND channel = ? AND conversation_id = ? AND user_id = ? AND expires_at > ?",
		projectID, channel, conversationID, userID, s.now()).First(&out).Error
	return out, err
}

// RememberConversationLanguage records the IM copy language. It deliberately
// never touches ExpiresAt: only a successful bind/select/continue renews focus,
// so passive chatter cannot keep a stale task selected alive.
func (s *TaskContextService) RememberConversationLanguage(projectID, channel, conversationID, userID, language string) error {
	if language != "en" && language != "zh-CN" {
		return nil
	}
	now := s.now()
	_ = s.db.Model(&models.ConversationFocus{}).
		Where("project_id = ? AND channel = ? AND conversation_id = ? AND user_id = ? AND expires_at <= ?",
			projectID, channel, conversationID, userID, now).
		Updates(map[string]any{"run_id": "", "pending_run_ids": []string{}}).Error
	// A language-only row carries a zero ExpiresAt, so it is never an active
	// focus; TouchConversationFocus is the only writer that sets one.
	row := models.ConversationFocus{
		ProjectID: projectID, Channel: channel, ConversationID: conversationID, UserID: userID,
		Language: language, UpdatedAt: now,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "channel"}, {Name: "conversation_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"language", "updated_at"}),
	}).Create(&row).Error
}

// ConversationLanguage reads remembered copy language independently of focus
// expiry.
func (s *TaskContextService) ConversationLanguage(projectID, channel, conversationID, userID string) string {
	var row models.ConversationFocus
	err := s.db.Select("language").
		Where("project_id = ? AND channel = ? AND conversation_id = ? AND user_id = ?",
			projectID, channel, conversationID, userID).First(&row).Error
	if err != nil {
		return ""
	}
	return row.Language
}

type TaskResolveRequest struct {
	ProjectID, UserID, Query, Channel, ConversationID string
	Now                                               time.Time
}

type TaskCandidate struct {
	Task    models.TaskIdentity `json:"task"`
	Score   int                 `json:"score"`
	Reasons []string            `json:"reasons"`
}

type TaskResolution struct {
	Task       *models.TaskIdentity `json:"task,omitempty"`
	Candidates []TaskCandidate      `json:"candidates,omitempty"`
	Ambiguous  bool                 `json:"ambiguous"`
}

// ResolveTask searches only the requesting project+user. Active tasks and
// terminal tasks updated in the last 30 days are eligible.
func (s *TaskContextService) ResolveTask(req TaskResolveRequest) (TaskResolution, error) {
	now := req.Now
	if now.IsZero() {
		now = s.now()
	}
	if err := s.materializeTaskIdentities(req.ProjectID, req.UserID, req.Query); err != nil {
		return TaskResolution{}, err
	}
	var rows []models.TaskIdentity
	if err := s.db.Where("project_id = ? AND user_id = ?", req.ProjectID, req.UserID).
		Where("status NOT IN ? OR updated_at >= ?", terminalTaskStatuses(), now.Add(-TaskTerminalLookback)).
		Find(&rows).Error; err != nil {
		return TaskResolution{}, err
	}
	focusRun := ""
	if req.Channel != "" && req.ConversationID != "" {
		var focus models.ConversationFocus
		if err := s.db.Where("project_id = ? AND channel = ? AND conversation_id = ? AND user_id = ? AND expires_at > ?",
			req.ProjectID, req.Channel, req.ConversationID, req.UserID, now).First(&focus).Error; err == nil {
			focusRun = focus.RunID
		}
	}
	q := normalizeSearch(req.Query)
	candidates := make([]TaskCandidate, 0, len(rows))
	for _, row := range rows {
		score, reasons := scoreTask(row, q, focusRun, now)
		if score > 0 {
			candidates = append(candidates, TaskCandidate{Task: row, Score: score, Reasons: reasons})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Task.UpdatedAt.After(candidates[j].Task.UpdatedAt)
	})
	if len(candidates) == 0 {
		return TaskResolution{}, ErrTaskNotFound
	}
	// Exact title is always explainable and unique within the candidate set.
	if candidates[0].Score >= 100 && (len(candidates) == 1 || candidates[0].Score > candidates[1].Score) {
		task := candidates[0].Task
		return TaskResolution{Task: &task, Candidates: candidates[:1]}, nil
	}
	// Otherwise require a high score and a meaningful margin.
	if candidates[0].Score >= 70 && (len(candidates) == 1 || candidates[0].Score-candidates[1].Score >= 20) {
		task := candidates[0].Task
		return TaskResolution{Task: &task, Candidates: candidates[:1]}, nil
	}
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	return TaskResolution{Candidates: candidates, Ambiguous: true}, ErrTaskAmbiguous
}

// MaterializeTaskIdentities projects canonical Runs into the current external
// user scope. It never reads Runs from another project and never copies an
// identity between users.
func (s *TaskContextService) MaterializeTaskIdentities(projectID, userID string) error {
	return s.materializeTaskIdentities(projectID, userID, "")
}

func (s *TaskContextService) materializeTaskIdentities(projectID, userID, query string) error {
	if s == nil || s.db == nil || strings.TrimSpace(projectID) == "" || strings.TrimSpace(userID) == "" {
		return fmt.Errorf("project and user are required")
	}
	if !s.db.Migrator().HasTable(&models.Run{}) || !s.db.Migrator().HasTable(&models.WorkflowDef{}) {
		return nil
	}
	type scopedRun struct {
		models.Run
		ProjectID       string
		WorkflowDisplay string
	}
	var runs []scopedRun
	err := s.db.Table("runs").
		Select("runs.*, workflow_defs.project_id AS project_id, workflow_defs.name AS workflow_display").
		Joins("JOIN workflow_defs ON workflow_defs.id = runs.workflow_id").
		Where("workflow_defs.project_id = ?", projectID).
		Find(&runs).Error
	if err != nil {
		return err
	}
	for _, scoped := range runs {
		original := originalRequirement(scoped.Run.Inputs)
		title := naturalTaskTitle(scoped.Run.Title, original, scoped.WorkflowDisplay, scoped.Run.ID)
		if strings.TrimSpace(query) != "" && !materializationMatches(query, title, original, scoped.WorkflowDisplay, scoped.Run.ID) {
			continue
		}
		var owner models.TaskIdentity
		ownerErr := s.db.Select("id, user_id").Where("run_id = ? AND project_id = ?", scoped.Run.ID, projectID).Limit(1).Find(&owner).Error
		if ownerErr != nil {
			return ownerErr
		}
		if owner.ID != 0 && owner.UserID != userID {
			continue
		}
		if owner.ID != 0 && strings.TrimSpace(owner.ShortTitle) != "" {
			title = owner.ShortTitle
		}
		row, err := s.UpsertTaskIdentity(TaskIdentityInput{
			RunID: scoped.Run.ID, ProjectID: projectID, UserID: userID,
			ShortTitle: title, OriginalRequirement: original,
			Keywords: taskKeywords(title, original, scoped.WorkflowDisplay), Status: scoped.Run.Status,
		})
		if err != nil {
			return err
		}
		activityAt := scoped.Run.StartedAt
		if activityAt.IsZero() {
			activityAt = scoped.Run.CreatedAt
		}
		if scoped.Run.DurationSec > 0 {
			activityAt = activityAt.Add(time.Duration(scoped.Run.DurationSec) * time.Second)
		}
		if !activityAt.IsZero() {
			_ = s.db.Model(&models.TaskIdentity{}).Where("id = ?", row.ID).UpdateColumn("updated_at", activityAt).Error
		}
	}
	return nil
}

func materializationMatches(query string, fields ...string) bool {
	q := normalizeSearch(query)
	for _, field := range fields {
		value := normalizeSearch(field)
		if value != "" && (strings.Contains(q, value) || strings.Contains(value, q)) {
			return true
		}
	}
	return false
}

func originalRequirement(inputs map[string]any) string {
	for _, key := range []string{"originalRequirement", "requirement", "task", "prompt", "query", "description"} {
		if value, ok := inputs[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
				return truncateTaskText(text, 500)
			}
		}
	}
	return ""
}

func naturalTaskTitle(runTitle, original, workflowName, runID string) string {
	for _, candidate := range []string{runTitle, original, workflowName, runID} {
		if text := strings.TrimSpace(candidate); text != "" {
			if line := strings.IndexAny(text, "\r\n"); line >= 0 {
				text = text[:line]
			}
			return truncateTaskText(text, 30)
		}
	}
	return "任务"
}

func truncateTaskText(text string, max int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}

func taskKeywords(parts ...string) []string {
	var out []string
	for _, part := range parts {
		for _, token := range strings.FieldsFunc(part, func(r rune) bool {
			return unicode.IsSpace(r) || strings.ContainsRune("，。！？、,.;:!?[]【】()（）/_-", r)
		}) {
			token = truncateTaskText(token, 32)
			if len([]rune(token)) >= 2 {
				out = append(out, token)
			}
		}
	}
	return uniqueStrings(out)
}

// SelectTaskCandidate accepts a one-based sequence or exact short title.
func SelectTaskCandidate(candidates []TaskCandidate, selection string) (models.TaskIdentity, error) {
	sel := strings.TrimSpace(selection)
	for _, c := range candidates {
		if strings.EqualFold(strings.TrimSpace(c.Task.ShortTitle), sel) {
			return c.Task, nil
		}
	}
	var n int
	if _, err := fmt.Sscanf(sel, "%d", &n); err == nil && n >= 1 && n <= len(candidates) {
		return candidates[n-1].Task, nil
	}
	return models.TaskIdentity{}, ErrTaskNotFound
}

func (s *TaskContextService) SelectPendingCandidate(projectID, channel, conversationID, userID, selection string) (models.TaskIdentity, error) {
	focus, err := s.GetConversationFocus(projectID, channel, conversationID, userID)
	if err != nil || len(focus.PendingRunIDs) == 0 {
		return models.TaskIdentity{}, ErrTaskNotFound
	}
	var rows []models.TaskIdentity
	if err := s.db.Where("project_id = ? AND user_id = ? AND run_id IN ?", projectID, userID, focus.PendingRunIDs).Find(&rows).Error; err != nil {
		return models.TaskIdentity{}, err
	}
	byID := make(map[string]models.TaskIdentity, len(rows))
	for _, row := range rows {
		byID[row.RunID] = row
	}
	candidates := make([]TaskCandidate, 0, len(focus.PendingRunIDs))
	for _, runID := range focus.PendingRunIDs {
		if row, ok := byID[runID]; ok {
			candidates = append(candidates, TaskCandidate{Task: row})
		}
	}
	picked, err := SelectTaskCandidate(candidates, selection)
	if err != nil {
		return models.TaskIdentity{}, err
	}
	if err := s.TouchConversationFocus(projectID, channel, conversationID, userID, picked.RunID); err != nil {
		return models.TaskIdentity{}, err
	}
	return picked, nil
}

func scoreTask(t models.TaskIdentity, q, focusRun string, now time.Time) (int, []string) {
	score := 0
	var why []string
	title := normalizeSearch(t.ShortTitle)
	if q != "" && title == q {
		score += 100
		why = append(why, "exact_title")
	} else if q != "" && strings.Contains(title, q) {
		score += 55
		why = append(why, "title")
	} else if q != "" && title != "" && strings.Contains(q, title) {
		score += 50
		why = append(why, "title_in_query")
	}
	original := normalizeSearch(t.OriginalRequirement)
	if q != "" && (strings.Contains(original, q) || (original != "" && strings.Contains(q, original))) {
		score += 35
		why = append(why, "original_requirement")
	}
	for _, alias := range t.Aliases {
		if q != "" && normalizeSearch(alias) == q {
			score += 70
			why = append(why, "exact_alias")
			break
		}
		if q != "" && strings.Contains(normalizeSearch(alias), q) {
			score += 30
			why = append(why, "alias")
			break
		}
		if q != "" && normalizeSearch(alias) != "" && strings.Contains(q, normalizeSearch(alias)) {
			score += 28
			why = append(why, "alias_in_query")
			break
		}
	}
	for _, kw := range t.Keywords {
		if q != "" && strings.Contains(normalizeSearch(kw), q) {
			score += 15
			why = append(why, "keyword")
			break
		}
	}
	if !isTerminalTaskStatus(t.Status) {
		score += 15
		why = append(why, "active")
	}
	if t.RunID == focusRun {
		score += 25
		why = append(why, "conversation_focus")
	}
	if now.Sub(t.UpdatedAt) <= 24*time.Hour {
		score += 5
		why = append(why, "recent")
	}
	return score, why
}

func normalizeSearch(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("，。！？、,.;:!?[]【】()（）-_", r) {
			return -1
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(s))
}

func terminalTaskStatuses() []string { return []string{"completed", "failed", "cancelled"} }
func isTerminalTaskStatus(s string) bool {
	for _, v := range terminalTaskStatuses() {
		if strings.EqualFold(strings.TrimSpace(s), v) {
			return true
		}
	}
	return false
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		k := strings.ToLower(v)
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}

type RiskTicketResult struct {
	Ticket  models.RiskConfirmationTicket
	Latest  *models.RiskConfirmationTicket
	Confirm bool
	Cancel  bool
	Stale   bool
}

func (s *TaskContextService) CreateRiskConfirmation(projectID, userID, runID, action string) (models.RiskConfirmationTicket, error) {
	return s.CreateRiskConfirmationWithKind(projectID, userID, runID, "", action)
}

func (s *TaskContextService) CreateRiskConfirmationWithKind(projectID, userID, runID, actionKind, action string) (models.RiskConfirmationTicket, error) {
	now := s.now()
	t := models.RiskConfirmationTicket{
		ID: "confirm-" + uuid.NewString()[:12], ProjectID: projectID, UserID: userID, RunID: runID,
		Action: action, ActionKind: actionKind, Status: models.RiskTicketPending,
		ExpiresAt: now.Add(RiskTicketTTL), CreatedAt: now, UpdatedAt: now,
	}
	return t, s.db.Create(&t).Error
}

// ConsumeRiskConfirmation atomically consumes one pending ticket. Chinese and
// English affirmative/cancel replies are accepted; stale/repeated replies
// return the latest ticket and cannot authorize execution again.
func (s *TaskContextService) ConsumeRiskConfirmation(projectID, userID, runID, action, reply string) (RiskTicketResult, error) {
	decision, ok := ParseConfirmationReply(reply)
	if !ok {
		return RiskTicketResult{}, ErrInvalidConfirmation
	}
	now := s.now()
	var ticket models.RiskConfirmationTicket
	err := s.db.Where("project_id = ? AND user_id = ? AND run_id = ? AND action = ?",
		projectID, userID, runID, action).Order("created_at desc").First(&ticket).Error
	if err != nil {
		return RiskTicketResult{}, err
	}
	status := "confirmed"
	if !decision {
		status = "cancelled"
	}
	updates := map[string]any{"status": status, "consumed_at": now, "updated_at": now}
	if decision {
		// Confirming turns the ticket into the persistent authorization grant.
		grantUntil := now.Add(RiskGrantTTL)
		updates["grant_expires_at"] = grantUntil
	}
	res := s.db.Model(&models.RiskConfirmationTicket{}).
		Where("id = ? AND status = ? AND expires_at > ?", ticket.ID, models.RiskTicketPending, now).
		Updates(updates)
	if res.Error != nil {
		return RiskTicketResult{}, res.Error
	}
	if res.RowsAffected == 1 {
		ticket.Status, ticket.ConsumedAt, ticket.UpdatedAt = status, &now, now
		return RiskTicketResult{Ticket: ticket, Confirm: decision, Cancel: !decision}, nil
	}
	latest := ticket
	_ = s.db.First(&latest, "id = ?", ticket.ID).Error
	if latest.Status == models.RiskTicketPending && !latest.ExpiresAt.After(now) {
		_ = s.db.Model(&latest).Where("status = ?", models.RiskTicketPending).
			Update("status", models.RiskTicketExpired).Error
		latest.Status = models.RiskTicketExpired
	}
	return RiskTicketResult{Ticket: ticket, Latest: &latest, Stale: true}, ErrConfirmationStale
}

// GuardChannelThread marks a PM thread as channel-originated. Guarded threads
// may only perform destructive PM MCP mutations against a confirmed ticket.
func (s *TaskContextService) GuardChannelThread(projectID, threadID, channel, userID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("task context unavailable")
	}
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(threadID) == "" {
		return fmt.Errorf("project and thread are required")
	}
	now := s.now()
	row := models.ChannelActionGuard{
		ProjectID: projectID, ThreadID: threadID, Channel: channel, UserID: userID,
		CreatedAt: now, UpdatedAt: now,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "thread_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"channel", "user_id", "updated_at"}),
	}).Create(&row).Error
}

// IsGuardedThread reports whether destructive mutations on this thread need an
// explicit grant. Unknown threads (web/API consults) are not guarded.
func (s *TaskContextService) IsGuardedThread(projectID, threadID string) bool {
	if s == nil || s.db == nil {
		return false
	}
	var count int64
	if err := s.db.Model(&models.ChannelActionGuard{}).
		Where("project_id = ? AND thread_id = ?", projectID, threadID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// BindActionGrant attaches a confirmed ticket to the thread allowed to spend
// it. Only a confirmed ticket can be bound.
func (s *TaskContextService) BindActionGrant(ticketID, threadID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("task context unavailable")
	}
	res := s.db.Model(&models.RiskConfirmationTicket{}).
		Where("id = ? AND status = ?", ticketID, models.RiskTicketConfirmed).
		Updates(map[string]any{"thread_id": threadID, "updated_at": s.now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrActionNotAuthorized
	}
	return nil
}

// ReleaseActionGrant retires a grant that the authorizing turn did not spend,
// so it can never be replayed by a later turn on the same thread.
func (s *TaskContextService) ReleaseActionGrant(ticketID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Model(&models.RiskConfirmationTicket{}).
		Where("id = ? AND status = ?", ticketID, models.RiskTicketConfirmed).
		Updates(map[string]any{"status": models.RiskTicketExpired, "updated_at": s.now()}).Error
}

// ConsumeActionGrant is the server-side authorization check executed by the
// destructive method itself. It succeeds at most once per ticket, and only for
// the exact project, thread, target and action kind the user confirmed.
func (s *TaskContextService) ConsumeActionGrant(projectID, threadID, target, actionKind string) error {
	if s == nil || s.db == nil {
		return ErrActionNotAuthorized
	}
	target, actionKind = strings.TrimSpace(target), strings.TrimSpace(actionKind)
	if projectID == "" || threadID == "" || target == "" || actionKind == "" {
		return ErrActionNotAuthorized
	}
	now := s.now()
	var ticket models.RiskConfirmationTicket
	err := s.db.Where("project_id = ? AND thread_id = ? AND run_id = ? AND action_kind = ? AND status = ?",
		projectID, threadID, target, actionKind, models.RiskTicketConfirmed).
		Where("grant_expires_at IS NOT NULL AND grant_expires_at > ?", now).
		Order("updated_at desc").First(&ticket).Error
	if err != nil {
		return ErrActionNotAuthorized
	}
	res := s.db.Model(&models.RiskConfirmationTicket{}).
		Where("id = ? AND status = ?", ticket.ID, models.RiskTicketConfirmed).
		Updates(map[string]any{
			"status": models.RiskTicketExecuted, "executed_at": now, "updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrActionNotAuthorized
	}
	return nil
}

func (s *TaskContextService) ConsumeLatestRiskConfirmation(projectID, userID, reply string) (RiskTicketResult, error) {
	var ticket models.RiskConfirmationTicket
	err := s.db.Where("project_id = ? AND user_id = ?", projectID, userID).
		Order("created_at desc").First(&ticket).Error
	if err != nil {
		return RiskTicketResult{}, err
	}
	return s.ConsumeRiskConfirmation(projectID, userID, ticket.RunID, ticket.Action, reply)
}

func ParseConfirmationReply(reply string) (confirm bool, ok bool) {
	v := strings.ToLower(strings.TrimSpace(reply))
	switch v {
	case "确认", "确认执行", "同意", "是", "yes", "y", "confirm", "proceed":
		return true, true
	case "取消", "不同意", "否", "no", "n", "cancel", "stop":
		return false, true
	default:
		return false, false
	}
}
