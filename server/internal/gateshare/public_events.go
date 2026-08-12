package gateshare

import (
	"encoding/json"
	"strings"

	"github.com/cocofhu/approving/internal/models"
)

// PublicDialogueNodeID is the leak-free node id rewritten onto public WS frames
// so the unauthenticated workbench can consume review/ACP without host ids.
const PublicDialogueNodeID = "public-gate"

// SanitizeLiveEvents keeps only message/thought rails and redacts leaky URLs.
func SanitizeLiveEvents(events []models.AcpEvent) []PreviewLiveEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]PreviewLiveEvent, 0, 2)
	for _, ev := range events {
		kind := strings.ToLower(strings.TrimSpace(ev.Kind))
		if kind != "message" && kind != "thought" {
			continue
		}
		text := capTurnText(SanitizeDescription(ev.Text))
		if text == "" {
			continue
		}
		out = append(out, PreviewLiveEvent{Kind: kind, Text: text})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FilterPublicBrokerFrame rewrites a run-broker payload for the public
// workbench: only review/acp for producerID, strip runId, rewrite nodeId,
// drop tool_call/plan and images.
func FilterPublicBrokerFrame(raw []byte, producerID string) ([]byte, bool) {
	producerID = strings.TrimSpace(producerID)
	if producerID == "" || len(raw) == 0 {
		return nil, false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	typ, _ := m["type"].(string)
	nodeID, _ := m["nodeId"].(string)
	if strings.TrimSpace(nodeID) != producerID {
		return nil, false
	}
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "review":
		return marshalPublicReviewFrame(m)
	case "acp":
		return marshalPublicAcpFrame(m)
	default:
		return nil, false
	}
}

func marshalPublicReviewFrame(m map[string]any) ([]byte, bool) {
	event, _ := m["event"].(string)
	event = strings.TrimSpace(event)
	if event == "" {
		return nil, false
	}
	out := map[string]any{
		"type":   "review",
		"nodeId": PublicDialogueNodeID,
		"event":  event,
	}
	if waiting, ok := jsonNumber(m["waiting"]); ok {
		out["waiting"] = waiting
	}
	if busy, ok := m["busy"].(bool); ok {
		out["busy"] = busy
	}
	if interrupted, ok := m["interrupted"].(bool); ok {
		out["interrupted"] = interrupted
	}
	if msg, _ := m["message"].(string); strings.TrimSpace(msg) != "" {
		out["message"] = capTurnText(SanitizeDescription(msg))
	}
	if items := queueItemsFromAny(m["items"]); len(items) > 0 {
		out["items"] = items
	}
	if ai := activeItemFromAny(m["activeItem"]); ai != nil {
		out["activeItem"] = ai
	}
	if item := activeItemFromAny(m["item"]); item != nil {
		out["item"] = item
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

func marshalPublicAcpFrame(m map[string]any) ([]byte, bool) {
	events := acpEventsFromAny(m["events"])
	rails := SanitizeLiveEvents(events)
	if len(rails) == 0 {
		return nil, false
	}
	out := map[string]any{
		"type":   "acp",
		"nodeId": PublicDialogueNodeID,
		"events": rails,
	}
	if busy, ok := m["busy"].(bool); ok {
		out["busy"] = busy
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

func jsonNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func queueItemsFromAny(v any) []PreviewQueueItem {
	switch items := v.(type) {
	case []map[string]any:
		return SanitizeQueueItems(items)
	case []any:
		parsed := make([]map[string]any, 0, len(items))
		for _, raw := range items {
			am, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			parsed = append(parsed, am)
		}
		return SanitizeQueueItems(parsed)
	default:
		return nil
	}
}

func activeItemFromAny(v any) map[string]any {
	switch item := v.(type) {
	case map[string]any:
		ai := SanitizeActiveItem(item)
		return previewActiveItemMap(ai)
	default:
		return nil
	}
}

func previewActiveItemMap(ai *PreviewActiveItem) map[string]any {
	if ai == nil {
		return nil
	}
	m := map[string]any{}
	if ai.ID != "" {
		m["id"] = ai.ID
	}
	if ai.Text != "" {
		m["text"] = ai.Text
	}
	if len(ai.Annotations) > 0 {
		m["annotations"] = ai.Annotations
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func acpEventsFromAny(v any) []models.AcpEvent {
	switch events := v.(type) {
	case []models.AcpEvent:
		return events
	case []any:
		out := make([]models.AcpEvent, 0, len(events))
		for _, raw := range events {
			am, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := am["kind"].(string)
			text, _ := am["text"].(string)
			out = append(out, models.AcpEvent{Kind: kind, Text: text})
		}
		return out
	default:
		return nil
	}
}
