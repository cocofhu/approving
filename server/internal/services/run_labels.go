package services

import (
	"github.com/cocofhu/approving/internal/models"
)

// CurrentNodeLabels returns current node display labels for active runs
// (running / waiting_human). Other statuses are omitted. Blank graph labels
// are omitted (no nodeID fallback).
func (s *RunService) CurrentNodeLabels(runs []models.Run) map[string]string {
	activeIDs := make([]string, 0)
	runByID := make(map[string]models.Run)
	for _, r := range runs {
		if r.Status == "running" || r.Status == "waiting_human" {
			activeIDs = append(activeIDs, r.ID)
			runByID[r.ID] = r
		}
	}
	if len(activeIDs) == 0 {
		return nil
	}

	var gates []models.Gate
	s.db.Where("run_id IN ? AND resolved = ?", activeIDs, false).Find(&gates)
	gateByRun := make(map[string]models.Gate, len(gates))
	for _, g := range gates {
		if existing, ok := gateByRun[g.RunID]; !ok || g.Iteration > existing.Iteration || (g.Iteration == existing.Iteration && g.ID > existing.ID) {
			gateByRun[g.RunID] = g
		}
	}

	var states []models.StateRun
	s.db.Where("run_id IN ? AND status IN ?", activeIDs, []string{"running", "waiting_human"}).
		Order("id desc").Find(&states)
	runningByRun := make(map[string]string)
	waitingHumanByRun := make(map[string]string)
	for _, sr := range states {
		if sr.Status == "running" {
			if _, ok := runningByRun[sr.RunID]; !ok {
				runningByRun[sr.RunID] = sr.NodeID
			}
		}
		if sr.Status == "waiting_human" {
			if _, ok := waitingHumanByRun[sr.RunID]; !ok {
				waitingHumanByRun[sr.RunID] = sr.NodeID
			}
		}
	}

	labels := make(map[string]string)
	for _, id := range activeIDs {
		r := runByID[id]
		var nodeID string
		switch r.Status {
		case "waiting_human":
			if g, ok := gateByRun[id]; ok {
				nodeID = g.NodeID
			} else if nid, ok := waitingHumanByRun[id]; ok {
				nodeID = nid
			}
		case "running":
			if nid, ok := runningByRun[id]; ok {
				nodeID = nid
			}
		}
		if nodeID == "" {
			continue
		}
		if label := GraphNodeLabel(r.Graph, nodeID); label != "" {
			labels[id] = label
		}
	}
	return labels
}

// CurrentNodeIDs returns current node ids for active runs (running /
// waiting_human). Other statuses are omitted.
func (s *RunService) CurrentNodeIDs(runs []models.Run) map[string]string {
	activeIDs := make([]string, 0)
	runByID := make(map[string]models.Run)
	for _, r := range runs {
		if r.Status == "running" || r.Status == "waiting_human" {
			activeIDs = append(activeIDs, r.ID)
			runByID[r.ID] = r
		}
	}
	if len(activeIDs) == 0 {
		return nil
	}

	var gates []models.Gate
	s.db.Where("run_id IN ? AND resolved = ?", activeIDs, false).Find(&gates)
	gateByRun := make(map[string]models.Gate, len(gates))
	for _, g := range gates {
		if existing, ok := gateByRun[g.RunID]; !ok || g.Iteration > existing.Iteration || (g.Iteration == existing.Iteration && g.ID > existing.ID) {
			gateByRun[g.RunID] = g
		}
	}

	var states []models.StateRun
	s.db.Where("run_id IN ? AND status IN ?", activeIDs, []string{"running", "waiting_human"}).
		Order("id desc").Find(&states)
	runningByRun := make(map[string]string)
	waitingHumanByRun := make(map[string]string)
	for _, sr := range states {
		if sr.Status == "running" {
			if _, ok := runningByRun[sr.RunID]; !ok {
				runningByRun[sr.RunID] = sr.NodeID
			}
		}
		if sr.Status == "waiting_human" {
			if _, ok := waitingHumanByRun[sr.RunID]; !ok {
				waitingHumanByRun[sr.RunID] = sr.NodeID
			}
		}
	}

	ids := make(map[string]string)
	for _, id := range activeIDs {
		r := runByID[id]
		var nodeID string
		switch r.Status {
		case "waiting_human":
			if g, ok := gateByRun[id]; ok {
				nodeID = g.NodeID
			} else if nid, ok := waitingHumanByRun[id]; ok {
				nodeID = nid
			}
		case "running":
			if nid, ok := runningByRun[id]; ok {
				nodeID = nid
			}
		}
		if nodeID != "" {
			ids[id] = nodeID
		}
	}
	return ids
}

// RunFailedError returns the latest error message from failed StateRuns for a
// failed run, or "" when none.
func (s *RunService) RunFailedError(runID string) string {
	var sr models.StateRun
	if err := s.db.Where("run_id = ? AND status = ? AND error != ''", runID, "failed").
		Order("id desc").First(&sr).Error; err != nil {
		return ""
	}
	return sr.Error
}
