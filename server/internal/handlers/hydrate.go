package handlers

import (
	"fmt"

	"github.com/cocofhu/approving/internal/mcp/structured"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func (h *Handlers) readRunArtifactContent(runID, name string) (string, error) {
	content, ok := h.Arts.Get(runID, name)
	if !ok {
		return "", fmt.Errorf("artifact not found: %s", name)
	}
	return content, nil
}

// hydrateTestResultJSON injects inline screenshot data for API/Agent buffers
// (get_test_result, inbox/node outputs). ArtifactContent intentionally does
// NOT call this so the web UI can lazy-load by artifact name.
func (h *Handlers) hydrateTestResultJSON(raw string, runID string) string {
	if runID == "" || raw == "" {
		return raw
	}
	hydrated, err := structured.HydrateTestResultContent(raw, func(name string) (string, error) {
		return h.readRunArtifactContent(runID, name)
	})
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("test_result hydrate failed; returning raw content")
		return raw
	}
	return hydrated
}

func (h *Handlers) hydrateTestResultOutputs(outputs map[string]any, runID string) {
	if outputs == nil {
		return
	}
	raw, ok := outputs["test_result_json"].(string)
	if !ok {
		return
	}
	outputs["test_result_json"] = h.hydrateTestResultJSON(raw, runID)
}

func (h *Handlers) hydrateNodeExecutions(nodeExecutions map[string][]gin.H, runID string) {
	for _, execs := range nodeExecutions {
		for i := range execs {
			outs, ok := execs[i]["outputs"].(map[string]any)
			if !ok {
				continue
			}
			h.hydrateTestResultOutputs(outs, runID)
		}
	}
}
