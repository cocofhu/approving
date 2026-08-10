package gateshare

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/cocofhu/approving/internal/models"
)

const (
	maxVisualHTMLBytes   = 512 * 1024
	maxDescriptionRunes  = 8000
	MaxCommentRunes      = 4000
	MaxExternalNameRunes = 80
	maxTurns             = 80
	maxTurnRunes         = 4000
	maxAnnotationRunes   = 400
	maxUpstreamBytes     = 64 * 1024
)

// PreviewTurn is a leak-free conversation turn for the public workbench.
type PreviewTurn struct {
	Role         string              `json:"role"`
	Text         string              `json:"text,omitempty"`
	At           string              `json:"at,omitempty"`
	Interrupted  bool                `json:"interrupted,omitempty"`
	Annotations  []PreviewAnnotation `json:"annotations,omitempty"`
}

// PreviewAnnotation is a leak-free annotation chip (no blob URLs).
type PreviewAnnotation struct {
	Selector string `json:"selector,omitempty"`
	JSONPath string `json:"jsonPath,omitempty"`
	Label    string `json:"label,omitempty"`
	Note     string `json:"note,omitempty"`
	Quote    string `json:"quote,omitempty"`
}

// PreviewQueueItem is a leak-free pending-send row for polling resume.
type PreviewQueueItem struct {
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
}

// PreviewActiveItem is a leak-free in-flight turn hint (no images / blob URLs).
type PreviewActiveItem struct {
	ID          string              `json:"id,omitempty"`
	Text        string              `json:"text,omitempty"`
	Annotations []PreviewAnnotation `json:"annotations,omitempty"`
}

var (
	leakyURLRe = regexp.MustCompile(`(?i)(?:blob:[^\s"'<>]*|/api/[^\s"'<>]*|/preview/[^\s"'<>]*|/sandbox[^\s"'<>]*|/v1/[^\s"'<>]*|https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[0-1])\.\d{1,3}\.\d{1,3})(?::\d+)?[^\s"'<>]*)`)
	internalHostRe = regexp.MustCompile(`(?i)\b(?:localhost|127\.0\.0\.1|0\.0\.0\.0)\b`)
)

// SanitizeDescription redacts internal URLs / blob addresses from gate body text.
func SanitizeDescription(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = leakyURLRe.ReplaceAllString(s, "[redacted]")
	if utf8.RuneCountInString(s) > maxDescriptionRunes {
		r := []rune(s)
		s = string(r[:maxDescriptionRunes]) + "…"
	}
	return s
}

// SanitizeVisualHTML strips leaky addresses from visual page.html. Scripts stay
// (HtmlPreview sandbox); size is capped.
func SanitizeVisualHTML(html string) string {
	html = strings.TrimSpace(html)
	if html == "" {
		return ""
	}
	if len(html) > maxVisualHTMLBytes {
		html = html[:maxVisualHTMLBytes]
	}
	html = leakyURLRe.ReplaceAllString(html, "#")
	return html
}

// SanitizeStructured extracts a leak-free preview object from artifact JSON/text.
func SanitizeStructured(name, content string) map[string]any {
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	out := map[string]any{}
	if name != "" && !looksInternalName(name) {
		out["name"] = safeArtifactName(name)
	}
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		text := SanitizeDescription(content)
		if text != "" {
			out["text"] = text
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	walk := sanitizeJSONValue(parsed, 0)
	if m, ok := walk.(map[string]any); ok {
		if t := stringField(m, "title", "summary"); t != "" {
			out["title"] = t
		}
		if g, ok := m["goals"]; ok {
			if cleaned := sanitizeJSONValue(g, 0); cleaned != nil {
				out["goals"] = cleaned
			}
		}
		if d := stringField(m, "description", "detail", "body"); d != "" {
			out["description"] = d
		}
		if doc := capStructuredDoc(m); doc != nil {
			out["doc"] = doc
		}
	}
	if len(out) == 1 && out["name"] != nil {
		if s, ok := walk.(string); ok && strings.TrimSpace(s) != "" {
			out["text"] = SanitizeDescription(s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func looksInternalName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "env") || strings.Contains(n, "secret") || strings.Contains(n, "token")
}

func safeArtifactName(name string) string {
	base := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		base = name[i+1:]
	}
	return strings.TrimSpace(base)
}

func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				s = SanitizeDescription(s)
				if s != "" && !internalHostRe.MatchString(s) {
					return s
				}
			}
		}
	}
	return ""
}

func sanitizeJSONValue(v any, depth int) any {
	if depth > 6 {
		return nil
	}
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range t {
			lk := strings.ToLower(strings.ReplaceAll(k, "-", "_"))
			if strings.Contains(lk, "token") || strings.Contains(lk, "secret") ||
				strings.Contains(lk, "password") || strings.Contains(lk, "apikey") ||
				strings.Contains(lk, "env") || lk == "runid" || lk == "run_id" ||
				lk == "projectid" || lk == "project_id" || lk == "workflowid" ||
				lk == "workflow_id" || lk == "workflow_name" || lk == "workflowname" ||
				strings.Contains(lk, "member") || lk == "url" || lk == "href" ||
				strings.Contains(lk, "blob") {
				continue
			}
			if cleaned := sanitizeJSONValue(val, depth+1); cleaned != nil {
				out[k] = cleaned
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			if cleaned := sanitizeJSONValue(item, depth+1); cleaned != nil {
				out = append(out, cleaned)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		s := SanitizeDescription(t)
		if s == "" || leakyURLRe.MatchString(t) {
			return nil
		}
		return s
	case float64, bool, nil:
		return t
	default:
		return nil
	}
}

// SanitizeTurns redacts conversation history for the public ReAct sidebar.
// Images / blob URLs are dropped; text and annotation chips are size-capped.
func SanitizeTurns(msgs []models.ReactMessage) []PreviewTurn {
	if len(msgs) == 0 {
		return nil
	}
	if len(msgs) > maxTurns {
		msgs = msgs[len(msgs)-maxTurns:]
	}
	out := make([]PreviewTurn, 0, len(msgs))
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "agent" && role != "human" {
			continue
		}
		text := capTurnText(SanitizeDescription(m.Text))
		turn := PreviewTurn{Role: role, Text: text, At: strings.TrimSpace(m.At), Interrupted: m.Interrupted}
		if anns := sanitizeAnnotations(m.Annotations); len(anns) > 0 {
			turn.Annotations = anns
		}
		if turn.Text == "" && len(turn.Annotations) == 0 && !turn.Interrupted {
			continue
		}
		out = append(out, turn)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SanitizeQueueItems redacts pending FIFO rows for the public ReAct sidebar.
func SanitizeQueueItems(items []map[string]any) []PreviewQueueItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]PreviewQueueItem, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		id, _ := it["id"].(string)
		text, _ := it["text"].(string)
		text = capTurnText(SanitizeDescription(text))
		id = strings.TrimSpace(id)
		if id == "" && text == "" {
			continue
		}
		out = append(out, PreviewQueueItem{ID: id, Text: text})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SanitizeActiveItem redacts the in-flight turn for polling resume. Images are dropped.
func SanitizeActiveItem(m map[string]any) *PreviewActiveItem {
	if m == nil {
		return nil
	}
	id, _ := m["id"].(string)
	text, _ := m["text"].(string)
	item := &PreviewActiveItem{
		ID:   strings.TrimSpace(id),
		Text: capTurnText(SanitizeDescription(text)),
	}
	switch anns := m["annotations"].(type) {
	case []models.ReactAnnotation:
		item.Annotations = sanitizeAnnotations(anns)
	case []any:
		parsed := make([]models.ReactAnnotation, 0, len(anns))
		for _, raw := range anns {
			am, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			parsed = append(parsed, models.ReactAnnotation{
				Selector: stringMapField(am, "selector"),
				JSONPath: stringMapField(am, "jsonPath"),
				Label:    stringMapField(am, "label"),
				Note:     stringMapField(am, "note"),
				Quote:    stringMapField(am, "quote"),
			})
		}
		item.Annotations = sanitizeAnnotations(parsed)
	}
	if item.ID == "" && item.Text == "" && len(item.Annotations) == 0 {
		return nil
	}
	return item
}

func capTurnText(text string) string {
	if utf8.RuneCountInString(text) <= maxTurnRunes {
		return text
	}
	r := []rune(text)
	return string(r[:maxTurnRunes]) + "…"
}

func stringMapField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

func sanitizeAnnotations(anns []models.ReactAnnotation) []PreviewAnnotation {
	if len(anns) == 0 {
		return nil
	}
	out := make([]PreviewAnnotation, 0, len(anns))
	for _, a := range anns {
		pa := PreviewAnnotation{
			Selector: clampAnnotationField(a.Selector),
			JSONPath: clampAnnotationField(a.JSONPath),
			Label:    clampAnnotationField(a.Label),
			Note:     clampAnnotationField(a.Note),
			Quote:    clampAnnotationField(a.Quote),
		}
		if pa.Selector == "" && pa.JSONPath == "" && pa.Label == "" && pa.Note == "" && pa.Quote == "" {
			continue
		}
		out = append(out, pa)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clampAnnotationField(s string) string {
	s = SanitizeDescription(s)
	if utf8.RuneCountInString(s) > maxAnnotationRunes {
		r := []rune(s)
		s = string(r[:maxAnnotationRunes]) + "…"
	}
	return s
}

// SanitizeUpstream extracts a leak-free clarified_requirement (or similar) summary.
func SanitizeUpstream(name, content string) map[string]any {
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	out := map[string]any{}
	if name != "" && !looksInternalName(name) {
		out["name"] = safeArtifactName(name)
	} else {
		out["name"] = "clarified_requirement.json"
	}
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		text := SanitizeDescription(content)
		if text == "" {
			return nil
		}
		out["text"] = text
		return out
	}
	walk := sanitizeJSONValue(parsed, 0)
	m, ok := walk.(map[string]any)
	if !ok {
		if s, ok := walk.(string); ok && strings.TrimSpace(s) != "" {
			out["text"] = SanitizeDescription(s)
			return out
		}
		return nil
	}
	if t := stringField(m, "title", "summary"); t != "" {
		out["title"] = t
	}
	if s := stringField(m, "summary", "background"); s != "" {
		out["summary"] = s
	}
	if d := stringField(m, "description", "detail", "background"); d != "" {
		out["description"] = d
	}
	if doc := capStructuredDoc(m); doc != nil {
		out["doc"] = doc
	}
	if len(out) <= 1 {
		return nil
	}
	return out
}

func capStructuredDoc(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) == 0 {
		return nil
	}
	if len(b) > maxUpstreamBytes {
		// Keep title/summary/goals so the main thesis remains reviewable.
		slim := map[string]any{}
		for _, k := range []string{"title", "summary", "background", "goals", "description", "in_scope", "out_of_scope"} {
			if v, ok := m[k]; ok {
				slim[k] = v
			}
		}
		if len(slim) == 0 {
			return nil
		}
		return slim
	}
	return m
}

// ClampComment bounds external comment length.
func ClampComment(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxCommentRunes {
		return "", false
	}
	return s, true
}

// ClampExternalName bounds optional external display name.
func ClampExternalName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxExternalNameRunes {
		return "", false
	}
	return s, true
}
