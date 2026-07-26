package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// MaxAuditPayloadBytes caps serialized payload size while keeping structure.
const MaxAuditPayloadBytes = 16 * 1024

// ProjectAuditService writes and queries append-only project audit events.
// Writes are fail-open: persistence errors are logged and do not fail callers.
type ProjectAuditService struct{ db *gorm.DB }

// NewProjectAuditService builds the service.
func NewProjectAuditService(db *gorm.DB) *ProjectAuditService {
	return &ProjectAuditService{db: db}
}

// AuditActor is the attributed operator for an event.
type AuditActor struct {
	Username       string
	Unattributable bool
}

// ActorFromUsername returns a real user when username is non-empty; otherwise
// system + unattributable. Never fabricates a person name.
func ActorFromUsername(username string) AuditActor {
	u := strings.TrimSpace(username)
	if u == "" {
		return AuditActor{Username: "system", Unattributable: true}
	}
	return AuditActor{Username: u, Unattributable: false}
}

// SystemActor is the canonical unattributable system operator.
func SystemActor() AuditActor {
	return AuditActor{Username: "system", Unattributable: true}
}

// AuditRecord is the input for a single append-only event.
type AuditRecord struct {
	ProjectID      string
	Actor          AuditActor
	Action         string
	ResourceType   string
	ResourceID     string
	Outcome        string
	Summary        string
	Payload        map[string]any
	OccurredAt     time.Time // zero → now
}

// Record appends a masked audit event. Fail-open on DB errors.
func (s *ProjectAuditService) Record(rec AuditRecord) {
	if s == nil || s.db == nil || strings.TrimSpace(rec.ProjectID) == "" {
		return
	}
	actor := rec.Actor
	if strings.TrimSpace(actor.Username) == "" {
		actor = SystemActor()
	}
	outcome := rec.Outcome
	if outcome != models.AuditOutcomeFail {
		outcome = models.AuditOutcomeOK
	}
	at := rec.OccurredAt
	if at.IsZero() {
		at = time.Now()
	}
	payload := MaskAuditPayload(rec.Payload)
	ev := models.ProjectAuditEvent{
		ID:             "aud-" + uuid.NewString()[:12],
		ProjectID:      rec.ProjectID,
		OccurredAt:     at,
		Actor:          actor.Username,
		Unattributable: actor.Unattributable,
		Action:         rec.Action,
		ResourceType:   rec.ResourceType,
		ResourceID:     rec.ResourceID,
		Outcome:        outcome,
		Summary:        rec.Summary,
		Payload:        payload,
		CreatedAt:      time.Now(),
	}
	if err := s.db.Create(&ev).Error; err != nil {
		log.Warn().Err(err).
			Str("project_id", rec.ProjectID).
			Str("action", rec.Action).
			Msg("project audit write failed (fail-open)")
	}
}

// AuditListFilter holds query filters for project audit listing/export.
type AuditListFilter struct {
	ProjectID  string
	From       *time.Time
	To         *time.Time
	Actor      string
	Action     string // exact or prefix (e.g. "workflow" matches workflow.*)
	Resource   string // substring match on resource_type, resource_id, or summary
	Page       int
	PageSize   int
}

// ListPage returns a page of audit events (newest first) plus total count.
func (s *ProjectAuditService) ListPage(f AuditListFilter) ([]models.ProjectAuditEvent, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, fmt.Errorf("audit unavailable")
	}
	q := s.applyFilter(s.db.Model(&models.ProjectAuditEvent{}), f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	size := f.PageSize
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	var items []models.ProjectAuditEvent
	err := s.applyFilter(s.db.Model(&models.ProjectAuditEvent{}), f).
		Order("occurred_at desc, id desc").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	if items == nil {
		items = []models.ProjectAuditEvent{}
	}
	return items, total, nil
}

// ListAllMatching returns all events matching the filter (capped) for export.
func (s *ProjectAuditService) ListAllMatching(f AuditListFilter, limit int) ([]models.ProjectAuditEvent, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("audit unavailable")
	}
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	var items []models.ProjectAuditEvent
	err := s.applyFilter(s.db.Model(&models.ProjectAuditEvent{}), f).
		Order("occurred_at desc, id desc").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []models.ProjectAuditEvent{}
	}
	return items, nil
}

func (s *ProjectAuditService) applyFilter(q *gorm.DB, f AuditListFilter) *gorm.DB {
	q = q.Where("project_id = ?", f.ProjectID)
	if f.From != nil {
		q = q.Where("occurred_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("occurred_at <= ?", *f.To)
	}
	if a := strings.TrimSpace(f.Actor); a != "" {
		q = q.Where("actor = ?", a)
	}
	if act := strings.TrimSpace(f.Action); act != "" {
		if strings.Contains(act, ".") {
			q = q.Where("action = ?", act)
		} else {
			q = q.Where("action = ? OR action LIKE ?", act, act+".%")
		}
	}
	if res := strings.TrimSpace(f.Resource); res != "" {
		like := "%" + res + "%"
		q = q.Where(
			"resource_type LIKE ? OR resource_id LIKE ? OR summary LIKE ?",
			like, like, like,
		)
	}
	return q
}

// FormatAuditText renders events as human-readable plain text.
func FormatAuditText(events []models.ProjectAuditEvent) string {
	var b strings.Builder
	b.WriteString("=== Approving Project Audit Export ===\n")
	b.WriteString(fmt.Sprintf("Exported: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Count: %d\n\n", len(events)))
	for i, ev := range events {
		actor := ev.Actor
		if ev.Unattributable {
			actor = ev.Actor + " (unattributable)"
		}
		b.WriteString(fmt.Sprintf("--- #%d ---\n", i+1))
		b.WriteString(fmt.Sprintf("Time: %s\n", ev.OccurredAt.Format(time.RFC3339)))
		b.WriteString(fmt.Sprintf("Actor: %s\n", actor))
		b.WriteString(fmt.Sprintf("Action: %s\n", ev.Action))
		b.WriteString(fmt.Sprintf("Resource: %s/%s\n", ev.ResourceType, ev.ResourceID))
		b.WriteString(fmt.Sprintf("Outcome: %s\n", ev.Outcome))
		if ev.Summary != "" {
			b.WriteString(fmt.Sprintf("Summary: %s\n", ev.Summary))
		}
		if len(ev.Payload) > 0 {
			raw, _ := json.MarshalIndent(ev.Payload, "", "  ")
			b.WriteString("Payload:\n")
			b.Write(raw)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// sensitiveKeyHints are case-insensitive substrings that mark secret-like keys.
var sensitiveKeyHints = []string{
	"password", "passwd", "secret", "token", "apikey", "api_key",
	"access_key", "private_key", "credential", "authorization", "auth",
	"sandboxenv", "app_secret", "appsecret",
}

// valueScanParentKeys trigger extra string-value heuristics under high-risk parents
// (gate form / MCP arguments/result / free-text fields).
var valueScanParentKeys = map[string]bool{
	"form": true, "arguments": true, "args": true, "result": true,
	"content": true, "prompt": true, "text": true, "body": true,
	"message": true, "messages": true, "input": true, "inputs": true,
}

// sensitiveValueREs redact common embedded secret shapes inside free-text values.
var sensitiveValueREs = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_\-]{16,})\b`),
	regexp.MustCompile(`(?i)\b(ghp_[A-Za-z0-9]{20,})\b`),
	regexp.MustCompile(`(?i)\b(github_pat_[A-Za-z0-9_]{20,})\b`),
	regexp.MustCompile(`(?i)\b(xox[baprs]-[A-Za-z0-9\-]{10,})\b`),
	regexp.MustCompile(`(?i)\b(Bearer\s+[A-Za-z0-9\-_\.=]{16,})\b`),
	regexp.MustCompile(`(?i)\b((?:api[_-]?key|password|passwd|secret|token)\s*[:=]\s*)([^\s"'\\]{6,})`),
}

// MaskAuditPayload deeply redacts sensitive fields and truncates oversized trees.
func MaskAuditPayload(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := maskValue(in, 0, false).(map[string]any)
	raw, err := json.Marshal(out)
	if err != nil {
		return map[string]any{"_error": "payload marshal failed"}
	}
	if len(raw) <= MaxAuditPayloadBytes {
		return out
	}
	return map[string]any{
		"_truncated": true,
		"_note":      "payload exceeded size cap; structure summarized",
		"keys":       mapKeys(out),
		"preview":    string(raw[:MaxAuditPayloadBytes/4]) + "…",
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func maskValue(v any, depth int, scanValues bool) any {
	if depth > 12 {
		return "[max-depth]"
	}
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if isSensitiveKey(k) {
				out[k] = SecretMask
				continue
			}
			childScan := scanValues || valueScanParentKeys[strings.ToLower(k)]
			out[k] = maskValue(val, depth+1, childScan)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = maskValue(val, depth+1, scanValues)
		}
		return out
	case string:
		s := t
		if scanValues {
			s = redactSensitiveString(s)
		}
		if len(s) > 4000 {
			return s[:4000] + "…"
		}
		return s
	default:
		return v
	}
}

func redactSensitiveString(s string) string {
	out := s
	for _, re := range sensitiveValueREs {
		if re.NumSubexp() >= 2 {
			out = re.ReplaceAllString(out, "${1}"+SecretMask)
		} else {
			out = re.ReplaceAllString(out, SecretMask)
		}
	}
	return out
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	if k == "value" {
		// Project variables often nest {name,value,secret}; handled by callers
		// when they pass already-masked structures. Still treat common aliases.
	}
	for _, h := range sensitiveKeyHints {
		if strings.Contains(k, h) {
			return true
		}
	}
	return false
}

// MaskProjectVarsForAudit returns variables with secret values replaced.
func MaskProjectVarsForAudit(vars []models.ProjectVariable) []map[string]any {
	out := make([]map[string]any, 0, len(vars))
	for _, v := range vars {
		val := v.Value
		if v.Secret {
			val = SecretMask
		}
		out = append(out, map[string]any{
			"name": v.Name, "type": v.Type, "value": val, "secret": v.Secret,
		})
	}
	return out
}

// MaskSandboxEnvForAudit returns env entries with secret values replaced.
func MaskSandboxEnvForAudit(env []models.EnvEntry) []map[string]any {
	out := make([]map[string]any, 0, len(env))
	for _, e := range env {
		val := e.Value
		if e.Secret {
			val = SecretMask
		}
		out = append(out, map[string]any{
			"key": e.Key, "value": val, "secret": e.Secret,
		})
	}
	return out
}

// ResolveProjectIDForWorkflow looks up project ownership for a workflow.
func ResolveProjectIDForWorkflow(db *gorm.DB, workflowID string) string {
	if db == nil || workflowID == "" {
		return ""
	}
	var wf models.WorkflowDef
	if err := db.Select("project_id").First(&wf, "id = ?", workflowID).Error; err != nil {
		return ""
	}
	return wf.ProjectID
}

// ResolveProjectIDForRun resolves project via run → workflow.
func ResolveProjectIDForRun(db *gorm.DB, runID string) string {
	if db == nil || runID == "" {
		return ""
	}
	var run models.Run
	if err := db.Select("workflow_id").First(&run, "id = ?", runID).Error; err != nil {
		return ""
	}
	return ResolveProjectIDForWorkflow(db, run.WorkflowID)
}
