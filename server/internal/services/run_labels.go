package services

import (
	"strings"

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

// RunOrigin says where a Run was dispatched from, for the runs that came in
// through a conversation rather than the web UI.
type RunOrigin struct {
	Channel        string `json:"channel,omitempty"`
	Scene          string `json:"scene,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	ExternalUserID string `json:"externalUserId,omitempty"`
	// Unbound means somebody detached this run on purpose, so it no longer
	// reports back there. The origin is still recorded — that is the point of
	// showing it: "came from here, does not talk to it any more" is a different
	// state from "came from nowhere".
	Unbound bool `json:"unbound,omitempty"`
}

// RunOriginFor resolves one run's dispatch origin. The second return says
// whether the run came from a conversation at all.
func (s *RunService) RunOriginFor(runID string) (RunOrigin, bool) {
	var row models.TaskIdentity
	if err := s.db.
		Where("run_id = ? AND origin_conversation_id <> ''", strings.TrimSpace(runID)).
		First(&row).Error; err != nil {
		return RunOrigin{}, false
	}
	return runOriginFromIdentity(row), true
}

func runOriginFromIdentity(row models.TaskIdentity) RunOrigin {
	return RunOrigin{
		Channel:        row.OriginChannel,
		Scene:          row.OriginScene,
		ConversationID: row.OriginConversationID,
		ExternalUserID: row.OriginExternalUserID,
		Unbound:        row.OriginUnboundAt != nil,
	}
}

// RunOrigins resolves dispatch origins for a page of runs in one query. Runs
// started from the web UI have no task identity and are simply absent from the
// result, which is what lets the caller render nothing for them.
func (s *RunService) RunOrigins(runs []models.Run) map[string]RunOrigin {
	if len(runs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(runs))
	for _, r := range runs {
		ids = append(ids, r.ID)
	}
	var rows []models.TaskIdentity
	if err := s.db.
		Select("run_id", "origin_channel", "origin_scene", "origin_conversation_id",
			"origin_external_user_id", "origin_unbound_at").
		Where("run_id IN ? AND origin_conversation_id <> ''", ids).
		Find(&rows).Error; err != nil {
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	out := make(map[string]RunOrigin, len(rows))
	for _, row := range rows {
		out[row.RunID] = runOriginFromIdentity(row)
	}
	return out
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
