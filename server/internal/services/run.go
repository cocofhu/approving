package services

import (
	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
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

// List returns runs, newest first. Queued runs have zero StartedAt (waiting
// time is excluded from duration), so they sort by created_at; all other
// statuses sort by started_at. Optional statuses (OR) and wf/projectId filter
// with AND semantics (wf wins over projectId when both set).
func (s *RunService) List(statuses []string, wf, projectID string) []models.Run {
	var runs []models.Run
	s.listQuery(statuses, wf, projectID).
		Order("CASE WHEN status = 'queued' THEN created_at ELSE started_at END DESC, id DESC").
		Find(&runs)
	return runs
}

// ListPage returns a page of runs plus the total matching count.
func (s *RunService) ListPage(statuses []string, wf, projectID string, page, pageSize int) ([]models.Run, int64) {
	q := s.listQuery(statuses, wf, projectID)
	var total int64
	q.Count(&total)
	var runs []models.Run
	offset := (page - 1) * pageSize
	q.Order("CASE WHEN status = 'queued' THEN created_at ELSE started_at END DESC, id DESC").
		Limit(pageSize).Offset(offset).Find(&runs)
	return runs, total
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
