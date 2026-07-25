package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SecretMask is the placeholder returned for secret values on read and accepted
// on write to mean "keep the previously stored plaintext".
const SecretMask = "****"

var (
	// ErrEmptyProjectName is returned when a project name is blank.
	ErrEmptyProjectName = errors.New("项目名称不能为空")
	// ErrProjectNameExists is returned when another project already uses the name.
	ErrProjectNameExists = errors.New("项目名称已存在")
	// ErrProjectNotFound is returned when the requested project does not exist.
	ErrProjectNotFound = errors.New("project not found")
	// ErrProjectHasWorkflows is returned when deleting a project that still owns workflows.
	ErrProjectHasWorkflows = errors.New("项目下仍有流水线，请先删除全部流水线")
	// ErrSecretPlaceholderOnNewKey is returned when a new/renamed key is saved with only the mask.
	ErrSecretPlaceholderOnNewKey = errors.New("新密钥或重命名的键不能使用打码占位值，请重新填写明文")
)

// ProjectService manages project CRUD and secret-aware config updates.
type ProjectService struct{ db *gorm.DB }

// NewProjectService builds the service.
func NewProjectService(db *gorm.DB) *ProjectService { return &ProjectService{db: db} }

// List returns all projects, newest-updated first.
func (s *ProjectService) List() []models.Project {
	var ps []models.Project
	s.db.Order("updated_at desc").Find(&ps)
	return ps
}

// Get returns one project by id.
func (s *ProjectService) Get(id string) (models.Project, bool) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return models.Project{}, false
	}
	return p, true
}

// NameExists reports whether any project (excluding excludeID) uses name.
func (s *ProjectService) NameExists(name, excludeID string) bool {
	var count int64
	q := s.db.Model(&models.Project{}).Where("name = ?", name)
	if excludeID != "" {
		q = q.Where("id != ?", excludeID)
	}
	q.Count(&count)
	return count > 0
}

// WorkflowCount returns how many workflows belong to the project.
func (s *ProjectService) WorkflowCount(projectID string) int64 {
	var n int64
	s.db.Model(&models.WorkflowDef{}).Where("project_id = ?", projectID).Count(&n)
	return n
}

// tokenAggChunk caps IN (?) size for SQLite variable limits while keeping
// TotalTokensByProjectIDs a batched (non N+1) read-path aggregation.
const tokenAggChunk = 400

// TotalTokens returns the summed StateRun.Usage.Total for one project, or nil
// when no Usage has been reported (UI "—"). A non-nil 0 means usage was
// reported and totals to zero.
func (s *ProjectService) TotalTokens(projectID string) *int64 {
	return s.TotalTokensByProjectIDs([]string{projectID})[projectID]
}

// TotalTokensByProjectIDs batch-aggregates Project→WorkflowDef→Run→StateRun
// Usage.Total for the given project IDs. Only rows with non-nil Usage contribute;
// projects with no such rows are omitted (caller treats as null / "—").
// PM Leader dialogue usage is outside this chain and is not counted.
func (s *ProjectService) TotalTokensByProjectIDs(projectIDs []string) map[string]*int64 {
	out := make(map[string]*int64, len(projectIDs))
	if len(projectIDs) == 0 {
		return out
	}

	type wfRow struct {
		ID        string
		ProjectID string
	}
	var wfs []wfRow
	if err := s.db.Model(&models.WorkflowDef{}).
		Select("id", "project_id").
		Where("project_id IN ?", projectIDs).
		Find(&wfs).Error; err != nil || len(wfs) == 0 {
		return out
	}

	wfToProject := make(map[string]string, len(wfs))
	wfIDs := make([]string, 0, len(wfs))
	for _, w := range wfs {
		wfToProject[w.ID] = w.ProjectID
		wfIDs = append(wfIDs, w.ID)
	}

	type runRow struct {
		ID         string
		WorkflowID string
	}
	var runs []runRow
	for i := 0; i < len(wfIDs); i += tokenAggChunk {
		end := i + tokenAggChunk
		if end > len(wfIDs) {
			end = len(wfIDs)
		}
		var chunk []runRow
		if err := s.db.Model(&models.Run{}).
			Select("id", "workflow_id").
			Where("workflow_id IN ?", wfIDs[i:end]).
			Find(&chunk).Error; err != nil {
			return out
		}
		runs = append(runs, chunk...)
	}
	if len(runs) == 0 {
		return out
	}

	runToProject := make(map[string]string, len(runs))
	runIDs := make([]string, 0, len(runs))
	for _, r := range runs {
		if pid := wfToProject[r.WorkflowID]; pid != "" {
			runToProject[r.ID] = pid
			runIDs = append(runIDs, r.ID)
		}
	}
	if len(runIDs) == 0 {
		return out
	}

	sums := make(map[string]int64)
	has := make(map[string]struct{})
	for i := 0; i < len(runIDs); i += tokenAggChunk {
		end := i + tokenAggChunk
		if end > len(runIDs) {
			end = len(runIDs)
		}
		var srs []models.StateRun
		if err := s.db.Model(&models.StateRun{}).
			Select("run_id", "usage").
			Where("run_id IN ? AND usage IS NOT NULL", runIDs[i:end]).
			Find(&srs).Error; err != nil {
			return out
		}
		for _, sr := range srs {
			if sr.Usage == nil {
				continue
			}
			pid := runToProject[sr.RunID]
			if pid == "" {
				continue
			}
			has[pid] = struct{}{}
			sums[pid] += sr.Usage.Total()
		}
	}

	for pid := range has {
		v := sums[pid]
		out[pid] = &v
	}
	return out
}

// Create inserts a new project.
func (s *ProjectService) Create(name, description string, env []models.EnvEntry, vars []models.ProjectVariable) (models.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.Project{}, ErrEmptyProjectName
	}
	if s.NameExists(name, "") {
		return models.Project{}, ErrProjectNameExists
	}
	if env == nil {
		env = []models.EnvEntry{}
	}
	if vars == nil {
		vars = []models.ProjectVariable{}
	}
	sanitizedEnv, err := sanitizeEnvEntries(env)
	if err != nil {
		return models.Project{}, err
	}
	sanitizedVars, err := sanitizeProjectVars(vars)
	if err != nil {
		return models.Project{}, err
	}
	now := time.Now()
	p := models.Project{
		ID:          "proj-" + uuid.NewString()[:8],
		Name:        name,
		Description: description,
		SandboxEnv:  sanitizedEnv,
		Variables:   sanitizedVars,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.db.Create(&p).Error; err != nil {
		return models.Project{}, err
	}
	return p, nil
}

// Update patches name/description/sandboxEnv/variables. Nil slices mean "leave
// unchanged"; non-nil slices replace the whole list with secret-preserving merge.
func (s *ProjectService) Update(id string, name *string, description *string, env *[]models.EnvEntry, vars *[]models.ProjectVariable) (models.Project, error) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return models.Project{}, ErrProjectNotFound
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return models.Project{}, ErrEmptyProjectName
		}
		if s.NameExists(n, id) {
			return models.Project{}, ErrProjectNameExists
		}
		p.Name = n
	}
	if description != nil {
		p.Description = *description
	}
	if env != nil {
		merged, err := mergeEnvEntries(p.SandboxEnv, *env)
		if err != nil {
			return models.Project{}, err
		}
		p.SandboxEnv = merged
	}
	if vars != nil {
		merged, err := mergeProjectVars(p.Variables, *vars)
		if err != nil {
			return models.Project{}, err
		}
		p.Variables = merged
	}
	p.UpdatedAt = time.Now()
	if err := s.db.Save(&p).Error; err != nil {
		return models.Project{}, err
	}
	return p, nil
}

// Delete removes a project when it has no workflows.
func (s *ProjectService) Delete(id string) error {
	if _, ok := s.Get(id); !ok {
		return ErrProjectNotFound
	}
	if s.WorkflowCount(id) > 0 {
		return ErrProjectHasWorkflows
	}
	return s.db.Delete(&models.Project{}, "id = ?", id).Error
}

// SandboxEnvForWorkflow returns the owning project's sandbox env (plaintext).
// Missing workflow/project yields nil (caller treats as empty).
func (s *ProjectService) SandboxEnvForWorkflow(workflowID string) []models.EnvEntry {
	var wf models.WorkflowDef
	if err := s.db.Select("project_id").First(&wf, "id = ?", workflowID).Error; err != nil || wf.ProjectID == "" {
		return nil
	}
	p, ok := s.Get(wf.ProjectID)
	if !ok {
		return nil
	}
	return p.SandboxEnv
}

// VariablesForWorkflow returns the owning project's workflow variables (plaintext).
func (s *ProjectService) VariablesForWorkflow(workflowID string) []models.ProjectVariable {
	var wf models.WorkflowDef
	if err := s.db.Select("project_id").First(&wf, "id = ?", workflowID).Error; err != nil || wf.ProjectID == "" {
		return nil
	}
	p, ok := s.Get(wf.ProjectID)
	if !ok {
		return nil
	}
	return p.Variables
}

// DefaultProjectID returns the id of「默认项目」when present, else the oldest project.
func (s *ProjectService) DefaultProjectID() string {
	var p models.Project
	if err := s.db.Where("id = ?", models.DefaultProjectID).First(&p).Error; err == nil {
		return p.ID
	}
	if err := s.db.Order("created_at asc").First(&p).Error; err == nil {
		return p.ID
	}
	return ""
}

func sanitizeEnvEntries(in []models.EnvEntry) ([]models.EnvEntry, error) {
	out := make([]models.EnvEntry, 0, len(in))
	seen := map[string]struct{}{}
	for _, e := range in {
		k := strings.TrimSpace(e.Key)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		if e.Value == SecretMask {
			return nil, ErrSecretPlaceholderOnNewKey
		}
		// Official ACP auth keys may be stored as project baseline; always force Secret.
		secret := e.Secret || runtime.IsPlatformAuthEnvKey(k)
		out = append(out, models.EnvEntry{Key: k, Value: e.Value, Secret: secret})
	}
	return out, nil
}

func sanitizeProjectVars(in []models.ProjectVariable) ([]models.ProjectVariable, error) {
	out := make([]models.ProjectVariable, 0, len(in))
	seen := map[string]struct{}{}
	for _, v := range in {
		n := strings.TrimSpace(v.Name)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		v.Name = n
		if v.Type == "" {
			v.Type = "string"
		}
		if v.Value == SecretMask {
			return nil, ErrSecretPlaceholderOnNewKey
		}
		out = append(out, v)
	}
	return out, nil
}

// mergeEnvEntries applies an incoming full-list replacement while preserving
// plaintext when the client sends empty or the mask placeholder, regardless of
// the target secret flag (so un-secreting a key cannot persist ****).
func mergeEnvEntries(existing, incoming []models.EnvEntry) ([]models.EnvEntry, error) {
	byKey := make(map[string]models.EnvEntry, len(existing))
	for _, e := range existing {
		byKey[e.Key] = e
	}
	out := make([]models.EnvEntry, 0, len(incoming))
	seen := map[string]struct{}{}
	for _, e := range incoming {
		k := strings.TrimSpace(e.Key)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		if isSecretPlaceholder(e.Value) {
			if old, ok := byKey[k]; ok {
				e.Value = old.Value
			} else if e.Value == SecretMask {
				return nil, ErrSecretPlaceholderOnNewKey
			}
		}
		// Official ACP auth keys may be stored as project baseline; always force Secret.
		secret := e.Secret || runtime.IsPlatformAuthEnvKey(k)
		out = append(out, models.EnvEntry{Key: k, Value: e.Value, Secret: secret})
	}
	return out, nil
}

func mergeProjectVars(existing, incoming []models.ProjectVariable) ([]models.ProjectVariable, error) {
	byName := make(map[string]models.ProjectVariable, len(existing))
	for _, v := range existing {
		byName[v.Name] = v
	}
	out := make([]models.ProjectVariable, 0, len(incoming))
	seen := map[string]struct{}{}
	for _, v := range incoming {
		n := strings.TrimSpace(v.Name)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		v.Name = n
		if v.Type == "" {
			v.Type = "string"
		}
		if isSecretVarPlaceholder(v.Value) {
			if old, ok := byName[n]; ok {
				v.Value = old.Value
			} else if v.Value == SecretMask {
				return nil, ErrSecretPlaceholderOnNewKey
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func isSecretPlaceholder(v string) bool {
	return v == "" || v == SecretMask
}

func isSecretVarPlaceholder(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	if !ok {
		return false
	}
	return isSecretPlaceholder(s)
}

// MaskedSandboxEnv returns env entries with secret values replaced by SecretMask.
func MaskedSandboxEnv(env []models.EnvEntry) []models.EnvEntry {
	out := make([]models.EnvEntry, len(env))
	for i, e := range env {
		out[i] = e
		if e.Secret {
			out[i].Value = SecretMask
		}
	}
	return out
}

// MaskedProjectVars returns variables with secret values replaced by SecretMask.
func MaskedProjectVars(vars []models.ProjectVariable) []models.ProjectVariable {
	out := make([]models.ProjectVariable, len(vars))
	for i, v := range vars {
		out[i] = v
		if v.Secret {
			out[i].Value = SecretMask
		}
	}
	return out
}

// ProjectEnvMap converts sandbox env entries to a key→value map for Spec.Env merge.
func ProjectEnvMap(env []models.EnvEntry) map[string]string {
	out := make(map[string]string, len(env))
	for _, e := range env {
		if e.Key == "" {
			continue
		}
		out[e.Key] = e.Value
	}
	return out
}

// FormatProjectHasWorkflowsError returns a stable API error string.
func FormatProjectHasWorkflowsError(n int64) string {
	return fmt.Sprintf("%s（%d）", ErrProjectHasWorkflows.Error(), n)
}
