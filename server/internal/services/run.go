package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
)

// defaultRunListOrder is the production default: hybrid time DESC + id DESC.
// Queued runs use created_at; all others use started_at.
const defaultRunListOrder = "CASE WHEN status = 'queued' THEN created_at ELSE started_at END DESC, id DESC"

// runListOrderBy maps whitelist sort/order to a fixed ORDER BY clause.
// sort and order must both be valid as a pair; otherwise the default order is used.
// Never concatenates raw user input into SQL identifiers.
func runListOrderBy(sort, order string) string {
	sort = strings.TrimSpace(sort)
	order = strings.ToLower(strings.TrimSpace(order))
	var dir string
	switch order {
	case "asc":
		dir = "ASC"
	case "desc":
		dir = "DESC"
	default:
		return defaultRunListOrder
	}
	switch sort {
	case "started_at":
		return "CASE WHEN status = 'queued' THEN created_at ELSE started_at END " + dir + ", id DESC"
	case "priority":
		return "priority " + dir + ", id DESC"
	default:
		return defaultRunListOrder
	}
}

// deletableRunStatuses is the allowlist for DeleteRun. Narrower than
// terminalRunStatuses on purpose: cancelled runs are terminal for inbox/artifact
// rules but must not be deletable here.
var deletableRunStatuses = []string{"completed", "failed"}

var (
	// ErrRunNotFound is returned when Delete targets a missing run id.
	ErrRunNotFound = errors.New("run not found")
	// ErrRunNotDeletable is returned when the run status is outside
	// deletableRunStatuses (queued/running/waiting_human/cancelled, etc.).
	ErrRunNotDeletable = errors.New("cannot delete run: only completed or failed runs can be deleted")
)

// RunService assembles run views from the underlying tables.
type RunService struct{ db *gorm.DB }

// NewRunService builds the service.
func NewRunService(db *gorm.DB) *RunService { return &RunService{db: db} }

func (s *RunService) listQuery(statuses []string, wf, projectID string) *gorm.DB {
	q := s.db.Model(&models.Run{})
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	if wf != "" {
		q = q.Where("workflow_id = ?", wf)
	} else if projectID != "" {
		q = q.Where("workflow_id IN (?)", s.db.Model(&models.WorkflowDef{}).Select("id").Where("project_id = ?", projectID))
	}
	return q
}

// List returns runs, newest first by default. Queued runs have zero StartedAt
// (waiting time is excluded from duration), so they sort by created_at; all
// other statuses sort by started_at. Optional statuses (OR) and wf/projectId
// filter with AND semantics (wf wins over projectId when both set).
//
// Optional trailing sortOrder is [sort, order]. Allowed sort: started_at|priority;
// allowed order: asc|desc. Both must be valid as a pair; otherwise the default
// hybrid-time DESC + id DESC is kept. Omitted args keep the default.
func (s *RunService) List(statuses []string, wf, projectID string, sortOrder ...string) []models.Run {
	sort, order := parseSortOrderArgs(sortOrder)
	var runs []models.Run
	s.listQuery(statuses, wf, projectID).
		Order(runListOrderBy(sort, order)).
		Find(&runs)
	return runs
}

// ListPage returns a page of runs plus the total matching count.
// Optional trailing sortOrder is [sort, order]; see List for whitelist rules.
func (s *RunService) ListPage(statuses []string, wf, projectID string, page, pageSize int, sortOrder ...string) ([]models.Run, int64) {
	sort, order := parseSortOrderArgs(sortOrder)
	q := s.listQuery(statuses, wf, projectID)
	var total int64
	q.Count(&total)
	var runs []models.Run
	offset := (page - 1) * pageSize
	q.Order(runListOrderBy(sort, order)).
		Limit(pageSize).Offset(offset).Find(&runs)
	return runs, total
}

func parseSortOrderArgs(sortOrder []string) (sort, order string) {
	if len(sortOrder) >= 1 {
		sort = sortOrder[0]
	}
	if len(sortOrder) >= 2 {
		order = sortOrder[1]
	}
	return sort, order
}

// Get returns a single run.
func (s *RunService) Get(id string) (models.Run, bool) {
	var run models.Run
	if err := s.db.First(&run, "id = ?", id).Error; err != nil {
		return models.Run{}, false
	}
	return run, true
}

// States returns the per-node StateRun records for a run.
func (s *RunService) States(runID string) []models.StateRun {
	var states []models.StateRun
	// Ordered oldest→newest per node so callers can treat the last row for a
	// node as its latest execution and build an in-order execution history.
	s.db.Where("run_id = ?", runID).Order("iteration asc, id asc").Find(&states)
	return states
}

// StateRun returns one node's persisted record (used as the event-log fallback
// once the live sandbox is gone).
func (s *RunService) StateRun(runID, nodeID string) (models.StateRun, bool) {
	var sr models.StateRun
	// Latest execution of the node (a node may have several after loop-backs).
	if err := s.db.Where("run_id = ? AND node_id = ?", runID, nodeID).
		Order("iteration desc, id desc").First(&sr).Error; err != nil {
		return models.StateRun{}, false
	}
	return sr, true
}

// Variables returns the live global variable values for a run.
func (s *RunService) Variables(runID string) []models.RunVariable {
	var vars []models.RunVariable
	s.db.Where("run_id = ?", runID).Find(&vars)
	return vars
}

// PendingGate returns the unresolved gate for a run, if any. Terminal runs
// (completed / failed / cancelled) never surface a pending gate: once a run ends
// any dangling gate is not actionable, so a stale row must not appear as an open
// approval on the run detail (the engine also supersedes such gates on finish).
func (s *RunService) PendingGate(runID string) (models.Gate, bool) {
	var g models.Gate
	if err := s.db.Joins("JOIN runs ON runs.id = gates.run_id").
		Where("gates.run_id = ? AND gates.resolved = ? AND runs.status NOT IN ?",
			runID, false, []string{"completed", "failed", "cancelled"}).
		Order("gates.iteration desc, gates.id desc").First(&g).Error; err != nil {
		return models.Gate{}, false
	}
	return g, true
}

// AllPendingGates returns every unresolved gate whose run is still alive (gates
// inbox). Gates on terminal runs (completed / failed / cancelled) are excluded:
// once a run ends its dangling gate is no longer actionable and must not linger
// in the approvals inbox.
func (s *RunService) AllPendingGates() []models.Gate {
	var gates []models.Gate
	s.db.Joins("JOIN runs ON runs.id = gates.run_id").
		Where("gates.resolved = ? AND runs.status NOT IN ?", false, []string{"completed", "failed", "cancelled"}).
		Order("gates.requested_at desc").Find(&gates)
	return gates
}

// Conversation returns the react conversation the run is currently at: the
// active (not-done) dialogue if the run is paused awaiting human input,
// otherwise the most recent completed one. A run may accumulate several react
// conversations (one per react node); returning the oldest would strand the
// user on a finished dialogue while a later react node waits for a reply.
func (s *RunService) Conversation(runID string) (models.ReactConversation, bool) {
	var conv models.ReactConversation
	if err := s.db.Where("run_id = ? AND done = ?", runID, false).
		Order("id desc").First(&conv).Error; err == nil {
		return conv, true
	}
	if err := s.db.Where("run_id = ?", runID).Order("id desc").First(&conv).Error; err != nil {
		return models.ReactConversation{}, false
	}
	return conv, true
}

// Conversations returns every react conversation accumulated by a run (one per
// react node), so the UI can render each react node's own dialogue history
// rather than only the run's current one.
func (s *RunService) Conversations(runID string) []models.ReactConversation {
	var convs []models.ReactConversation
	s.db.Where("run_id = ?", runID).Order("id asc").Find(&convs)
	return convs
}

// DB exposes the underlying handle for handlers needing ad-hoc queries.
func (s *RunService) DB() *gorm.DB { return s.db }

// Delete permanently removes a completed/failed run and its associated rows so
// the run no longer appears in lists/details or related UI entry points.
// Active (queued/running/waiting_human) and cancelled runs are rejected with
// ErrRunNotDeletable. Missing runs return ErrRunNotFound. Does not touch
// WorkflowDef, WorkflowVersion, or WorkflowAPIKey.
func (s *RunService) Delete(id string) error {
	run, ok := s.Get(id)
	if !ok {
		return ErrRunNotFound
	}
	if err := rejectIfNotDeletable(run.Status); err != nil {
		return err
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Re-check inside the transaction so a status change between the
		// pre-check and delete is rejected atomically; a concurrent delete
		// surfaces as not-found.
		var current models.Run
		if err := tx.First(&current, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRunNotFound
			}
			return err
		}
		if err := rejectIfNotDeletable(current.Status); err != nil {
			return err
		}

		for _, m := range []any{
			&models.StateRun{},
			&models.RunVariable{},
			&models.Artifact{},
			&models.Gate{},
			&models.ReactConversation{},
			&models.PreviewIssue{},
			&models.RunPreviewPort{},
			&models.SandboxLog{},
			&models.Sandbox{},
		} {
			if err := tx.Where("run_id = ?", id).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&models.Run{}, "id = ?", id).Error
	})
}

func rejectIfNotDeletable(status string) error {
	if containsString(deletableRunStatuses, status) {
		return nil
	}
	switch status {
	case "queued", "running", "waiting_human":
		return fmt.Errorf("%w: cancel or wait until the run ends (status %q)", ErrRunNotDeletable, status)
	default:
		// cancelled and any unexpected non-deletable status
		return fmt.Errorf("%w (status %q)", ErrRunNotDeletable, status)
	}
}
