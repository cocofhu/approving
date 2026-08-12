package engine

import (
	"strings"

	"github.com/cocofhu/approving/internal/models"
)

// hasRemainingHumanGate reports whether a node with Type=="human_gate" is
// reachable forward from fromNodeID (inclusive) on the given graph snapshot.
//
// Reachability neighbors are OutEdges targets plus config goto targets
// (branch.cases[].goto, human_gate.actions[].goto, structured exits.*.goto).
// Only human_gate counts — proposal_select, ReAct review waits, and platform
// auto gates do not. Missing/empty graph, missing from node, or unresolvable
// structure returns false (conservative: avoid falsely prioritizing).
// Cycles terminate via a visited set.
func hasRemainingHumanGate(graph *models.Graph, fromNodeID string) bool {
	if graph == nil || fromNodeID == "" {
		return false
	}
	start := graph.FindNode(fromNodeID)
	if start == nil {
		return false
	}
	if start.Type == "human_gate" {
		return true
	}
	visited := map[string]bool{fromNodeID: true}
	queue := []string{fromNodeID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		node := graph.FindNode(cur)
		if node == nil {
			continue
		}
		for _, next := range remainingPathNeighbors(graph, node) {
			if visited[next] {
				continue
			}
			visited[next] = true
			gn := graph.FindNode(next)
			if gn != nil && gn.Type == "human_gate" {
				return true
			}
			queue = append(queue, next)
		}
	}
	return false
}

// remainingPathNeighbors returns forward destinations from node: OutEdges
// targets union config goto targets, de-duplicated while preserving discovery
// order (edges first, then config gotos).
func remainingPathNeighbors(graph *models.Graph, node *models.Node) []string {
	if graph == nil || node == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, ed := range graph.OutEdges(node.ID) {
		add(ed.Target)
	}
	for _, g := range configGotoTargets(node) {
		add(g)
	}
	return out
}

// configGotoTargets extracts static goto destinations from node config that
// runtime routing may take without a corresponding OutEdge.
func configGotoTargets(node *models.Node) []string {
	if node == nil || node.Config == nil {
		return nil
	}
	var out []string
	appendGoto := func(v any) {
		if g := strings.TrimSpace(str(v)); g != "" {
			out = append(out, g)
		}
	}
	if cases, ok := node.Config["cases"].([]any); ok {
		for _, ci := range cases {
			m, ok := ci.(map[string]any)
			if !ok {
				continue
			}
			appendGoto(m["goto"])
		}
	}
	if actions, ok := node.Config["actions"].([]any); ok {
		for _, ai := range actions {
			m, ok := ai.(map[string]any)
			if !ok {
				continue
			}
			appendGoto(m["goto"])
		}
	}
	if exits, ok := node.Config["exits"].(map[string]any); ok {
		for _, side := range exits {
			m, ok := side.(map[string]any)
			if !ok {
				continue
			}
			appendGoto(m["goto"])
		}
	}
	return out
}

// continueFromNodeID resolves the admission continue point for a queued run:
// Checkpoints[__admission_from__].node when present, else Graph.StartNode().
// Returns ("", false) when neither is available.
func continueFromNodeID(run models.Run) (string, bool) {
	if run.Checkpoints != nil {
		if cp, ok := run.Checkpoints[admissionFromCheckpoint]; ok {
			if node, _ := cp["node"].(string); strings.TrimSpace(node) != "" {
				return node, true
			}
		}
	}
	if start := run.Graph.StartNode(); start != nil {
		return start.ID, true
	}
	return "", false
}
