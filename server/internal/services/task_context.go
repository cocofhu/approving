package services

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TaskFocusTTL       = 30 * time.Minute
	TaskTerminalWindow = 30 * 24 * time.Hour
	// StaleDispatchTTL closes ephemeral dispatch:* ledger rows that never became
	// a real Run and were never marked terminal. Without this, every short IM
	// lookup stays "running" forever and crowds the director's briefing with
	// work that finished (or was abandoned) long ago.
	StaleDispatchTTL = 30 * time.Minute
)

// ErrTaskIdentityScopeMismatch means the Run already belongs to a different
// project or user scope. Callers must treat the task as invisible, not re-home it.
var ErrTaskIdentityScopeMismatch = errors.New("task identity scope mismatch")

type TaskContextService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewTaskContextService(db *gorm.DB) *TaskContextService {
	return &TaskContextService{db: db, now: time.Now}
}

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

	// Origin conversation, recorded once when the task is created from a
	// channel turn. Empty values never clear an already-recorded origin.
	OriginChannel, OriginScene, OriginConversationID, OriginExternalUserID string
	Language, RecentContext                                                string
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
			ShortTitle:          SanitizeShortTitle(in.ShortTitle),
			OriginalRequirement: strings.TrimSpace(in.OriginalRequirement),
			Aliases:             uniqueStrings(in.Aliases), Keywords: uniqueStrings(in.Keywords),
			OriginChannel:        strings.TrimSpace(in.OriginChannel),
			OriginScene:          strings.TrimSpace(in.OriginScene),
			OriginConversationID: strings.TrimSpace(in.OriginConversationID),
			OriginExternalUserID: strings.TrimSpace(in.OriginExternalUserID),
			Language:             NormalizeLanguage(in.Language),
			RecentContext:        strings.TrimSpace(in.RecentContext),
			Status:               normalizeTaskStatus(in.Status), CreatedAt: now, UpdatedAt: now,
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
	newTitle := SanitizeShortTitle(in.ShortTitle)
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
	// Origin is write-once: whoever created the task owns where its results go.
	// A later update from another surface must not re-home an existing task.
	if identity.OriginConversationID == "" {
		if conv := strings.TrimSpace(in.OriginConversationID); conv != "" {
			identity.OriginChannel = strings.TrimSpace(in.OriginChannel)
			identity.OriginScene = strings.TrimSpace(in.OriginScene)
			identity.OriginConversationID = conv
			identity.OriginExternalUserID = strings.TrimSpace(in.OriginExternalUserID)
		}
	}
	// Callers decide whether a message really represents a language switch (see
	// TaskLanguageFor); this just records the decision.
	if lang := NormalizeLanguage(in.Language); lang != "" {
		identity.Language = lang
	}
	if recent := strings.TrimSpace(in.RecentContext); recent != "" {
		identity.RecentContext = recent
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

const runShortTitleRunes = 24

// internalIDPattern matches the internal Run / task identifier shapes in any
// casing. Short titles are shown to users, who have no idea what a Run id is;
// letting one through produced titles like 「修复 Run run-1ca1876f」.
var internalIDPattern = regexp.MustCompile(`(?i)\b(run|task)[-_ ]?[0-9a-f]{6,}\b`)

// genericTitleWords are placeholders that carry no meaning on their own.
var genericTitleWords = map[string]bool{
	"run": true, "task": true, "job": true, "workflow": true, "任务": true, "运行": true,
}

// SanitizeShortTitle strips internal identifiers from a user-facing task title
// and reports the empty string when nothing meaningful survives, so callers can
// fall back to real words instead of persisting a leaked id.
func SanitizeShortTitle(title string) string {
	cleaned := internalIDPattern.ReplaceAllString(strings.TrimSpace(title), " ")
	words := strings.Fields(cleaned)
	// Removing the identifier from 「修复 Run run-1ca1876f」 leaves a dangling
	// "Run" that means nothing on its own.
	for len(words) > 0 && genericTitleWords[strings.ToLower(words[len(words)-1])] {
		words = words[:len(words)-1]
	}
	for len(words) > 0 && genericTitleWords[strings.ToLower(words[0])] {
		words = words[1:]
	}
	cleaned = strings.Trim(strings.Join(words, " "), " -_/|:：、,，")
	if cleaned == "" || genericTitleWords[strings.ToLower(cleaned)] {
		return ""
	}
	return truncateTaskRunes(cleaned, runShortTitleRunes)
}

// runShortTitle derives a human-readable label for a Run. It prefers the Run's
// own title, then the workflow name, then the first sentence of the original
// request — and never falls back to the Run id.
func runShortTitle(run models.Run) string {
	candidates := []string{run.Title, run.WorkflowName, firstSentence(runRequirement(run))}
	for _, candidate := range candidates {
		if title := SanitizeShortTitle(firstLine(candidate)); title != "" {
			return title
		}
	}
	return "未命名任务"
}

// firstSentence returns the leading sentence of a free-form request, which is
// usually the request itself in one line.
func firstSentence(value string) string {
	value = firstLine(value)
	if idx := strings.IndexAny(value, "。！？!?;；\n"); idx > 0 {
		return strings.TrimSpace(value[:idx])
	}
	return strings.TrimSpace(value)
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

// IdentityForRun loads the task identity of a Run inside one project. A Run
// without an identity yet returns (nil, nil) so callers can skip instead of
// treating it as a failure.
func (s *TaskContextService) IdentityForRun(runID, projectID string) (*models.TaskIdentity, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("task context database is unavailable")
	}
	runID, projectID = strings.TrimSpace(runID), strings.TrimSpace(projectID)
	if runID == "" || projectID == "" {
		return nil, errors.New("run_id and project_id are required")
	}
	var identity models.TaskIdentity
	err := s.db.Where("run_id = ? AND project_id = ?", runID, projectID).First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

// CountActive counts the user's non-terminal tasks in one conversation. Reply
// formatting uses it to decide whether a task label is needed at all: with a
// single task in flight the context is already unambiguous.
func (s *TaskContextService) CountActive(scope TaskScope) (int, error) {
	tasks, err := s.ActiveTasksForConversation(scope, 100)
	if err != nil {
		return 0, err
	}
	return len(tasks), nil
}

// ActiveTasksForConversation lists this conversation's live tasks, most
// recently updated first, for disambiguation prompts.
//
// Scope is the conversation that asked, not the whole project: an earlier bug
// listed every non-terminal row for the user, so finished work from other
// chats (and forgotten dispatch:* stubs) kept showing up as "还在跑".
func (s *TaskContextService) ActiveTasksForConversation(scope TaskScope, limit int) ([]models.TaskIdentity, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("task context database is unavailable")
	}
	if strings.TrimSpace(scope.ProjectID) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	q := s.db.Where("project_id = ? AND terminal_at IS NULL", scope.ProjectID)
	if uid := strings.TrimSpace(scope.UserID); uid != "" {
		q = q.Where("user_id = ? OR user_id = ''", uid)
	}
	if conv := strings.TrimSpace(scope.ConversationID); conv != "" {
		q = q.Where("origin_conversation_id = ?", conv)
	}
	// Pull a wider window so reaping stale rows can still fill `limit`.
	var rows []models.TaskIdentity
	if err := q.Order("updated_at DESC").Limit(limit * 3).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.TaskIdentity, 0, limit)
	for i := range rows {
		if s.reapStaleActiveTask(&rows[i]) {
			continue
		}
		out = append(out, rows[i])
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// reapStaleActiveTask marks a ledger row terminal when the underlying work is
// already gone, and reports whether the caller should hide it. It is the
// self-heal for rows that missed their completion write.
func (s *TaskContextService) reapStaleActiveTask(task *models.TaskIdentity) bool {
	if task == nil || task.TerminalAt != nil {
		return true
	}
	runID := strings.TrimSpace(task.RunID)
	now := s.now()
	if runID != "" && !strings.HasPrefix(runID, "dispatch:") {
		var run models.Run
		if err := s.db.Select("id", "status").Where("id = ?", runID).First(&run).Error; err == nil {
			if IsTerminalTaskStatus(run.Status) {
				_, _ = s.UpdateIdentity(EnsureTaskIdentityInput{
					RunID: runID, ProjectID: task.ProjectID, UserID: task.UserID,
					Status: run.Status,
				})
				return true
			}
		}
	}
	if strings.HasPrefix(runID, "dispatch:") && now.Sub(task.UpdatedAt) > StaleDispatchTTL {
		_, _ = s.UpdateIdentity(EnsureTaskIdentityInput{
			RunID: runID, ProjectID: task.ProjectID, UserID: task.UserID,
			Status: "completed",
		})
		return true
	}
	return false
}

// IdentityByID loads one task row inside a project.
func (s *TaskContextService) IdentityByID(id, projectID string) (*models.TaskIdentity, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("task context database is unavailable")
	}
	id, projectID = strings.TrimSpace(id), strings.TrimSpace(projectID)
	if id == "" || projectID == "" {
		return nil, errors.New("id and project_id are required")
	}
	var identity models.TaskIdentity
	err := s.db.Where("id = ? AND project_id = ?", id, projectID).First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
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

// ExpireFocus drops a conversation's task pointer. It is called when the task
// it points at is no longer something the conversation is about — a cancelled
// task that stays in focus makes every later "那个" resolve to work nobody is
// doing.
func (s *TaskContextService) ExpireFocus(scope TaskScope) error {
	if s == nil || s.db == nil {
		return errors.New("task context database is unavailable")
	}
	return s.db.Where("project_id = ? AND user_id = ? AND channel = ? AND conversation_id = ?",
		scope.ProjectID, scope.UserID, scope.Channel, scope.ConversationID).
		Delete(&models.ConversationFocus{}).Error
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

// languageSwitchRunes is how much of a message must be in the other language
// before it counts as the user switching rather than quoting.
const languageSwitchRunes = 12

// TaskLanguageFor decides which language to answer a task in.
//
// Per-message detection makes a conversation flip languages whenever someone
// pastes an English identifier or a Chinese label, so an established task
// language wins unless the user clearly switched: a message long enough to be a
// real sentence, entirely in the other language.
func TaskLanguageFor(established, message string) string {
	established = NormalizeLanguage(established)
	if established == "" {
		return DetectLanguage(message, "")
	}
	message = strings.TrimSpace(message)
	if utf8.RuneCountInString(message) < languageSwitchRunes {
		return established
	}
	if detected := DetectLanguage(message, ""); detected != established {
		return detected
	}
	return established
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

// FormatTaskType names which task a message is about, in the way a person
// would. The old bracketed 【title｜kind】 header made every message read like a
// ticket update; a reference only earns its place when more than one task could
// be meant, and then it should sound like speech. The kind is deliberately not
// part of it — naming the task is what disambiguates, and classifying it out
// loud is the ticket voice this replaced.
func FormatTaskType(shortTitle, language string) string {
	shortTitle = SanitizeShortTitle(shortTitle)
	if NormalizeLanguage(language) == "en" {
		if shortTitle == "" {
			return ""
		}
		return fmt.Sprintf("On \"%s\" — ", shortTitle)
	}
	if shortTitle == "" {
		return ""
	}
	return fmt.Sprintf("%s那个：", shortTitle)
}

// TaskStatusSentence names a task and states what is true of it. statusLabel is
// the predicate ("is done." / "还在做。"), so the two read as one sentence.
func TaskStatusSentence(shortTitle, statusLabel, language string) string {
	shortTitle = SanitizeShortTitle(shortTitle)
	statusLabel = strings.TrimSpace(statusLabel)
	if shortTitle == "" {
		return statusLabel
	}
	if NormalizeLanguage(language) == "en" {
		return fmt.Sprintf("\"%s\" %s", shortTitle, statusLabel)
	}
	return fmt.Sprintf("「%s」%s", shortTitle, statusLabel)
}

func normalizeTaskStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return "active"
	}
	return status
}

// IsTerminalTaskStatus reports whether a task has reached a state it can no
// longer leave.
func IsTerminalTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled", "canceled", "done":
		return true
	default:
		return false
	}
}

func isTerminalTaskStatus(status string) bool { return IsTerminalTaskStatus(status) }

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
