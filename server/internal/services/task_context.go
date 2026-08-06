package services

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TaskFocusTTL       = 30 * time.Minute
	TaskTerminalWindow = 30 * 24 * time.Hour
)

// ErrTaskIdentityScopeMismatch means the Run already belongs to a different
// project or user scope. Callers must treat the task as invisible, not re-home it.
var ErrTaskIdentityScopeMismatch = errors.New("task identity scope mismatch")

type TaskContextService struct {
	db           *gorm.DB
	now          func() time.Time
	autoBackfill bool
}

func NewTaskContextService(db *gorm.DB) *TaskContextService {
	return &TaskContextService{db: db, now: time.Now}
}

// EnableRunBackfill makes Search lazily materialize task identities from the
// project's real Runs, so IM callers never have to pre-register a task.
func (s *TaskContextService) EnableRunBackfill() { s.autoBackfill = true }

func (s *TaskContextService) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// DB exposes the underlying store for orchestration helpers that need to load
// a Run before EnsureIdentityForRun. Callers must not bypass service methods
// for task identity mutations.
func (s *TaskContextService) DB() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}

type EnsureTaskIdentityInput struct {
	RunID, ProjectID, UserID, ShortTitle, OriginalRequirement, Status string
	Aliases, Keywords                                                 []string
}

func (s *TaskContextService) EnsureIdentity(in EnsureTaskIdentityInput) (*models.TaskIdentity, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("task context database is unavailable")
	}
	in.RunID = strings.TrimSpace(in.RunID)
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.UserID = strings.TrimSpace(in.UserID)
	if in.RunID == "" || in.ProjectID == "" {
		return nil, errors.New("run_id and project_id are required")
	}
	now := s.now()
	var identity models.TaskIdentity
	err := s.db.Where("run_id = ? AND project_id = ?", in.RunID, in.ProjectID).First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		identity = models.TaskIdentity{
			ID: "task-" + uuid.NewString()[:12], RunID: in.RunID,
			ProjectID: in.ProjectID, UserID: in.UserID,
			ShortTitle:          strings.TrimSpace(in.ShortTitle),
			OriginalRequirement: strings.TrimSpace(in.OriginalRequirement),
			Aliases:             uniqueStrings(in.Aliases), Keywords: uniqueStrings(in.Keywords),
			Status: normalizeTaskStatus(in.Status), CreatedAt: now, UpdatedAt: now,
		}
		if isTerminalTaskStatus(identity.Status) {
			t := now
			identity.TerminalAt = &t
		}
		if err := s.db.Create(&identity).Error; err != nil {
			return nil, err
		}
		return &identity, nil
	}
	if err != nil {
		return nil, err
	}
	// Task metadata is project/user/Run scoped. A different user must never
	// update, discover, or re-home another user's task.
	if identity.ProjectID != in.ProjectID ||
		(identity.UserID != "" && in.UserID != "" && identity.UserID != in.UserID) {
		return nil, ErrTaskIdentityScopeMismatch
	}
	oldTitle := strings.TrimSpace(identity.ShortTitle)
	newTitle := strings.TrimSpace(in.ShortTitle)
	if newTitle != "" && oldTitle != "" && newTitle != oldTitle {
		identity.Aliases = uniqueStrings(append(identity.Aliases, oldTitle))
	}
	if newTitle != "" {
		identity.ShortTitle = newTitle
	}
	if req := strings.TrimSpace(in.OriginalRequirement); req != "" && identity.OriginalRequirement == "" {
		identity.OriginalRequirement = req
	}
	if identity.UserID == "" && in.UserID != "" {
		identity.UserID = in.UserID
	}
	identity.Aliases = uniqueStrings(append(identity.Aliases, in.Aliases...))
	identity.Keywords = uniqueStrings(append(identity.Keywords, in.Keywords...))
	if strings.TrimSpace(in.Status) != "" {
		wasTerminal := isTerminalTaskStatus(identity.Status)
		identity.Status = normalizeTaskStatus(in.Status)
		if isTerminalTaskStatus(identity.Status) && !wasTerminal {
			t := now
			identity.TerminalAt = &t
		} else if !isTerminalTaskStatus(identity.Status) {
			identity.TerminalAt = nil
		}
	}
	identity.UpdatedAt = now
	if err := s.db.Save(&identity).Error; err != nil {
		return nil, err
	}
	return &identity, nil
}

func (s *TaskContextService) UpdateIdentity(in EnsureTaskIdentityInput) (*models.TaskIdentity, error) {
	return s.EnsureIdentity(in)
}

// EnsureIdentityForRun derives a task identity straight from a real Run, so
// callers never have to hand-assemble titles, requirements or keywords.
func (s *TaskContextService) EnsureIdentityForRun(run models.Run, projectID, userID string) (*models.TaskIdentity, error) {
	requirement := runRequirement(run)
	return s.EnsureIdentity(EnsureTaskIdentityInput{
		RunID:               run.ID,
		ProjectID:           projectID,
		UserID:              userID,
		ShortTitle:          runShortTitle(run),
		OriginalRequirement: requirement,
		Keywords:            runKeywords(run, requirement),
		Status:              run.Status,
	})
}

// BackfillProjectRuns lazily materializes identities for a project's Runs in
// the requesting user scope. Existing identities owned by another user remain
// invisible and are never re-homed.
func (s *TaskContextService) BackfillProjectRuns(projectID, userID string) error {
	if s == nil || s.db == nil {
		return errors.New("task context database is unavailable")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return errors.New("project_id is required")
	}
	var runs []models.Run
	err := s.db.
		Where("workflow_id IN (?)",
			s.db.Model(&models.WorkflowDef{}).Select("id").Where("project_id = ?", projectID)).
		Where("created_at >= ?", s.now().Add(-TaskTerminalWindow)).
		Find(&runs).Error
	if err != nil {
		return err
	}
	for _, run := range runs {
		if _, err := s.EnsureIdentityForRun(run, projectID, strings.TrimSpace(userID)); err != nil {
			if errors.Is(err, ErrTaskIdentityScopeMismatch) {
				continue // different project; stay invisible
			}
			return err
		}
	}
	return nil
}

const runShortTitleRunes = 24

func runShortTitle(run models.Run) string {
	for _, candidate := range []string{run.Title, run.WorkflowName} {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return truncateTaskRunes(firstLine(candidate), runShortTitleRunes)
		}
	}
	return truncateTaskRunes("Run "+run.ID, runShortTitleRunes)
}

// runRequirementKeys are the conventional Run input keys that hold the original
// human request, most specific first.
var runRequirementKeys = []string{"requirement", "request", "goal", "task", "prompt", "input", "description"}

func runRequirement(run models.Run) string {
	for _, key := range runRequirementKeys {
		if value, ok := run.Inputs[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return strings.TrimSpace(run.Title)
}

func runKeywords(run models.Run, requirement string) []string {
	keywords := append([]string(nil), run.Tags...)
	if name := strings.TrimSpace(run.WorkflowName); name != "" {
		keywords = append(keywords, name)
	}
	for _, field := range strings.Fields(normalizeSearchText(firstLine(requirement))) {
		if len([]rune(field)) >= 2 {
			keywords = append(keywords, field)
		}
	}
	if len(keywords) > 12 {
		keywords = keywords[:12]
	}
	return uniqueStrings(keywords)
}

func firstLine(value string) string {
	if idx := strings.IndexAny(value, "\r\n"); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return strings.TrimSpace(value)
}

func truncateTaskRunes(value string, limit int) string {
	r := []rune(strings.TrimSpace(value))
	if limit <= 0 || len(r) <= limit {
		return string(r)
	}
	return string(r[:limit])
}

type TaskScope struct {
	ProjectID, UserID, Channel, ConversationID string
}

func (s *TaskContextService) BindMessage(scope TaskScope, messageID string, identity *models.TaskIdentity) error {
	if identity == nil || strings.TrimSpace(messageID) == "" {
		return errors.New("message_id and identity are required")
	}
	if identity.ProjectID != scope.ProjectID || identity.UserID != scope.UserID {
		return ErrTaskIdentityScopeMismatch
	}
	b := models.MessageBinding{
		ProjectID: scope.ProjectID, UserID: scope.UserID, Channel: scope.Channel,
		ConversationID: scope.ConversationID, MessageID: strings.TrimSpace(messageID),
		TaskIdentityID: identity.ID, RunID: identity.RunID, CreatedAt: s.now(),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "user_id"}, {Name: "channel"}, {Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"conversation_id", "task_identity_id", "run_id"}),
	}).Create(&b).Error
}

func (s *TaskContextService) SetFocus(scope TaskScope, identity *models.TaskIdentity, language string) (*models.ConversationFocus, error) {
	if identity == nil {
		return nil, errors.New("identity is required")
	}
	if identity.ProjectID != scope.ProjectID || identity.UserID != scope.UserID {
		return nil, ErrTaskIdentityScopeMismatch
	}
	now := s.now()
	focus := models.ConversationFocus{
		ProjectID: scope.ProjectID, UserID: scope.UserID, Channel: scope.Channel,
		ConversationID: scope.ConversationID, TaskIdentityID: identity.ID,
		RunID: identity.RunID, Language: NormalizeLanguage(language),
		ExpiresAt: now.Add(TaskFocusTTL), CreatedAt: now, UpdatedAt: now,
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "user_id"}, {Name: "channel"}, {Name: "conversation_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"task_identity_id", "run_id", "language", "expires_at", "updated_at"}),
	}).Create(&focus).Error
	if err != nil {
		return nil, err
	}
	return &focus, nil
}

func (s *TaskContextService) GetFocus(scope TaskScope, renew bool) (*models.ConversationFocus, error) {
	var focus models.ConversationFocus
	err := s.db.Where("project_id = ? AND user_id = ? AND channel = ? AND conversation_id = ?",
		scope.ProjectID, scope.UserID, scope.Channel, scope.ConversationID).First(&focus).Error
	if err != nil {
		return nil, err
	}
	now := s.now()
	if !focus.ExpiresAt.After(now) {
		return nil, gorm.ErrRecordNotFound
	}
	if renew {
		focus.ExpiresAt = now.Add(TaskFocusTTL)
		focus.UpdatedAt = now
		if err := s.db.Save(&focus).Error; err != nil {
			return nil, err
		}
	}
	return &focus, nil
}

type TaskCandidate struct {
	Identity models.TaskIdentity
	Score    int
	Reasons  []string
}

type TaskResolution struct {
	Identity   *models.TaskIdentity
	Candidates []TaskCandidate
	Ambiguous  bool
	Reason     string
}

type ResolveTaskInput struct {
	Scope          TaskScope
	Query          string
	ReplyMessageID string
	Ordinal        int // 1-based selection from Candidates
	Candidates     []TaskCandidate
}

func (s *TaskContextService) ResolveTask(in ResolveTaskInput) (TaskResolution, error) {
	// Explicit reply binding always wins and is still strictly scoped.
	if ref := strings.TrimSpace(in.ReplyMessageID); ref != "" {
		var b models.MessageBinding
		err := s.db.Where("project_id = ? AND user_id = ? AND channel = ? AND message_id = ?",
			in.Scope.ProjectID, in.Scope.UserID, in.Scope.Channel, ref).First(&b).Error
		if err == nil {
			var task models.TaskIdentity
			if err := s.db.Where("id = ? AND project_id = ? AND user_id = ?",
				b.TaskIdentityID, in.Scope.ProjectID, in.Scope.UserID).First(&task).Error; err != nil {
				return TaskResolution{}, err
			}
			return s.resolvedWithFocus(in.Scope, task, in.Query, nil, "reply_binding")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return TaskResolution{}, err
		}
	}
	if in.Ordinal > 0 && in.Ordinal <= len(in.Candidates) {
		task := in.Candidates[in.Ordinal-1].Identity
		return s.resolvedWithFocus(in.Scope, task, in.Query, in.Candidates, "ordinal")
	}
	query := strings.TrimSpace(in.Query)
	if query == "" {
		focus, err := s.GetFocus(in.Scope, true)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return TaskResolution{Reason: "focus_missing_or_expired"}, nil
			}
			return TaskResolution{}, err
		}
		var task models.TaskIdentity
		if err := s.db.Where("id = ? AND project_id = ? AND user_id = ?",
			focus.TaskIdentityID, in.Scope.ProjectID, in.Scope.UserID).First(&task).Error; err != nil {
			return TaskResolution{}, err
		}
		return TaskResolution{Identity: &task, Reason: "conversation_focus"}, nil
	}
	candidates, err := s.Search(in.Scope, query)
	if err != nil {
		return TaskResolution{}, err
	}
	if len(candidates) == 0 {
		return TaskResolution{Reason: "no_match"}, nil
	}
	if len(candidates) > 1 && candidates[0].Score-candidates[1].Score < 15 {
		return TaskResolution{Candidates: candidates, Ambiguous: true, Reason: "score_margin"}, nil
	}
	task := candidates[0].Identity
	return s.resolvedWithFocus(in.Scope, task, in.Query, candidates, "unique_score")
}

func (s *TaskContextService) resolvedWithFocus(scope TaskScope, task models.TaskIdentity, current string, candidates []TaskCandidate, reason string) (TaskResolution, error) {
	recent := ""
	if focus, err := s.GetFocus(scope, false); err == nil {
		recent = focus.Language
	}
	if _, err := s.SetFocus(scope, &task, DetectLanguage(current, recent)); err != nil {
		return TaskResolution{}, err
	}
	return TaskResolution{Identity: &task, Candidates: candidates, Reason: reason}, nil
}

func (s *TaskContextService) Search(scope TaskScope, query string) ([]TaskCandidate, error) {
	if s.autoBackfill {
		// Best-effort: a backfill failure must not make addressing unavailable.
		if err := s.BackfillProjectRuns(scope.ProjectID, scope.UserID); err != nil {
			log.Warn().Err(err).Str("project", scope.ProjectID).
				Msg("task identity backfill failed; searching existing identities only")
		}
	}
	now := s.now()
	var tasks []models.TaskIdentity
	if err := s.db.Where("project_id = ? AND user_id = ? AND (terminal_at IS NULL OR terminal_at >= ?)",
		scope.ProjectID, scope.UserID, now.Add(-TaskTerminalWindow)).Find(&tasks).Error; err != nil {
		return nil, err
	}
	query = normalizeSearchText(query)
	var out []TaskCandidate
	for _, task := range tasks {
		score, reasons := scoreTask(task, query)
		if score > 0 {
			out = append(out, TaskCandidate{Identity: task, Score: score, Reasons: reasons})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Identity.UpdatedAt.After(out[j].Identity.UpdatedAt)
	})
	return out, nil
}

func scoreTask(task models.TaskIdentity, query string) (int, []string) {
	if query == "" {
		return 0, nil
	}
	title := normalizeSearchText(task.ShortTitle)
	if query == title {
		return 100, []string{"exact_short_title"}
	}
	best := 0
	var reasons []string
	for _, alias := range task.Aliases {
		a := normalizeSearchText(alias)
		if query == a {
			return 90, []string{"exact_alias"}
		}
		if a != "" && (strings.Contains(a, query) || strings.Contains(query, a)) && best < 55 {
			best, reasons = 55, []string{"alias_contains"}
		}
	}
	if title != "" && (strings.Contains(title, query) || strings.Contains(query, title)) && best < 60 {
		best, reasons = 60, []string{"short_title_contains"}
	}
	for _, keyword := range task.Keywords {
		k := normalizeSearchText(keyword)
		if k != "" && strings.Contains(query, k) {
			best += 15
			reasons = append(reasons, "keyword:"+keyword)
		}
	}
	if req := normalizeSearchText(task.OriginalRequirement); strings.Contains(req, query) && best < 35 {
		best, reasons = 35, []string{"requirement_contains"}
	}
	return best, reasons
}

// ParseOrdinal recognizes a plain 1-based numeric selection.
func ParseOrdinal(text string) int {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 1 {
		return 0
	}
	return n
}

func SyntheticQQUserID(userID string) string {
	return "qq:" + strings.TrimSpace(userID)
}

func DetectLanguage(current, recent string) string {
	current = strings.TrimSpace(current)
	if current != "" {
		for _, r := range current {
			if unicode.Is(unicode.Han, r) {
				return "zh-CN"
			}
		}
		for _, r := range current {
			if unicode.IsLetter(r) {
				return "en"
			}
		}
	}
	if normalized := NormalizeLanguage(recent); normalized != "" {
		return normalized
	}
	return "zh-CN"
}

func NormalizeLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "en", "en-us", "en-gb":
		return "en"
	case "zh", "zh-cn", "zh-hans":
		return "zh-CN"
	default:
		return ""
	}
}

func FormatTaskType(shortTitle, kind, language string) string {
	shortTitle = strings.TrimSpace(shortTitle)
	kind = strings.TrimSpace(kind)
	if shortTitle == "" {
		if NormalizeLanguage(language) == "en" {
			shortTitle = "Task"
		} else {
			shortTitle = "任务"
		}
	}
	if kind == "" {
		if NormalizeLanguage(language) == "en" {
			kind = "Update"
		} else {
			kind = "更新"
		}
	}
	return fmt.Sprintf("【%s｜%s】", shortTitle, kind)
}

func normalizeTaskStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return "active"
	}
	return status
}

func isTerminalTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled", "canceled", "done":
		return true
	default:
		return false
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeSearchText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}
