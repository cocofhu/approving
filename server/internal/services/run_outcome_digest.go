package services

import (
	"path"
	"strings"
	"unicode"
)

// preferredOutcomeArtifactNames are tried first when digesting a finished Run
// for IM reflow. Agents that leave a short summary under one of these names
// get that text into the conversation instead of a hollow "弄完了".
var preferredOutcomeArtifactNames = []string{
	"summary.md", "summary.txt", "conclusion.md", "conclusion.txt",
	"findings.md", "findings.txt", "report.md", "result.md", "final.md",
	"README.md",
}

// DigestedRunOutcome returns a short, user-facing fact digest from a run's
// artifacts. Empty when nothing readable exists — callers must still have a
// fallback that does not pretend details are waiting elsewhere.
func (s *ArtifactService) DigestedRunOutcome(runID string, maxRunes int) string {
	if s == nil || s.db == nil {
		return ""
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = 800
	}
	for _, name := range preferredOutcomeArtifactNames {
		if content, ok := s.Get(runID, name); ok {
			if dig := normalizeOutcomeDigest(content, maxRunes); dig != "" {
				return dig
			}
		}
	}
	// Case-insensitive match on preferred basenames (agents vary capitalization).
	meta := s.ByRun(runID)
	want := map[string]bool{}
	for _, n := range preferredOutcomeArtifactNames {
		want[strings.ToLower(n)] = true
	}
	for _, a := range meta {
		base := strings.ToLower(path.Base(a.Name))
		if !want[base] {
			continue
		}
		content, ok := s.Get(runID, a.Name)
		if !ok {
			continue
		}
		if dig := normalizeOutcomeDigest(content, maxRunes); dig != "" {
			return dig
		}
	}
	// Last resort: first textual artifact that is not orchestration scaffolding.
	for _, a := range meta {
		base := strings.ToLower(path.Base(a.Name))
		if base == "plan.json" || strings.HasSuffix(base, ".json") {
			continue
		}
		if !looksLikeTextArtifact(a.Kind, a.Name) {
			continue
		}
		content, ok := s.Get(runID, a.Name)
		if !ok {
			continue
		}
		if dig := normalizeOutcomeDigest(content, maxRunes); dig != "" {
			return dig
		}
	}
	return ""
}

// AppendRunDeliveryURL adds a run's delivery URL (e.g. submit_mr's mr_url) to
// the completion digest so later follow-ups can answer from stored facts.
func AppendRunDeliveryURL(digest, url string) string {
	digest = strings.TrimSpace(digest)
	url = strings.TrimSpace(url)
	if url == "" {
		return digest
	}
	if strings.Contains(digest, url) {
		return digest
	}
	line := "交付链接：" + url
	if digest == "" {
		return line
	}
	return digest + "\n" + line
}

func looksLikeTextArtifact(kind, name string) bool {
	k := strings.ToLower(strings.TrimSpace(kind))
	n := strings.ToLower(path.Base(name))
	switch {
	case strings.Contains(k, "markdown"), k == "md", k == "text", k == "txt", k == "doc":
		return true
	case strings.HasSuffix(n, ".md"), strings.HasSuffix(n, ".txt"), strings.HasSuffix(n, ".markdown"):
		return true
	}
	return false
}

func normalizeOutcomeDigest(content string, maxRunes int) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	// Drop fenced reasoning / huge dumps: keep the first coherent paragraphs.
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			continue
		}
		lower := strings.ToLower(trim)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(lower, "tool_call") {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(trim)
		if len([]rune(b.String())) >= maxRunes {
			break
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, out)
	runes := []rune(out)
	if len(runes) > maxRunes {
		out = string(runes[:maxRunes]) + "…"
	}
	return strings.TrimSpace(out)
}
