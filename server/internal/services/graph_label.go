package services

import (
	"strings"

	"github.com/cocofhu/approving/internal/models"
)

// GraphNodeLabel returns a node's display label from a pinned graph snapshot.
// Empty when the node is missing or its label is blank (no nodeID fallback).
func GraphNodeLabel(g models.Graph, nodeID string) string {
	if n := g.FindNode(nodeID); n != nil {
		return strings.TrimSpace(n.Label)
	}
	return ""
}
