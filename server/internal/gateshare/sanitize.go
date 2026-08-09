package gateshare

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxVisualHTMLBytes = 512 * 1024
	maxDescriptionRunes = 8000
	MaxCommentRunes     = 4000
	MaxExternalNameRunes = 80
)

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
