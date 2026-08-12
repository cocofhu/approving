package feishu

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// buildPostContent turns assistant markdown into a Feishu post JSON body
// (msg_type=post). Title is the first heading when present; body uses md
// plus code_block tags. Never uses interactive cards.
func buildPostContent(text string) string {
	title, body := splitTitle(text)
	lines := strings.Split(body, "\n")
	var content [][]map[string]string
	var para []string
	flushPara := func() {
		if len(para) == 0 {
			return
		}
		joined := strings.TrimSpace(strings.Join(para, "\n"))
		if joined != "" {
			content = append(content, []map[string]string{{"tag": "md", "text": joined}})
		}
		para = nil
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if lang, ok := codeFenceOpen(line); ok {
			flushPara()
			var code []string
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			cell := map[string]string{"tag": "code_block", "text": strings.Join(code, "\n")}
			if lang != "" {
				cell["language"] = lang
			}
			content = append(content, []map[string]string{cell})
			continue
		}
		para = append(para, line)
	}
	flushPara()
	if len(content) == 0 {
		content = append(content, []map[string]string{{"tag": "md", "text": strings.TrimSpace(text)}})
	}
	payload := map[string]any{
		"zh_cn": map[string]any{
			"title":   title,
			"content": content,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return `{"zh_cn":{"title":"","content":[[{"tag":"md","text":""}]]}}`
	}
	if len(b) > 30*1024 {
		// Keep the post under Feishu's 30KB limit.
		trimmed := truncateRunes(text, 4000)
		payload = map[string]any{
			"zh_cn": map[string]any{
				"title":   title,
				"content": [][]map[string]string{{{"tag": "md", "text": trimmed}}},
			},
		}
		b, _ = json.Marshal(payload)
	}
	return string(b)
}

func splitTitle(text string) (title, body string) {
	s := strings.TrimSpace(text)
	if s == "" {
		return "", ""
	}
	first, rest, _ := strings.Cut(s, "\n")
	trim := strings.TrimSpace(first)
	if strings.HasPrefix(trim, "# ") {
		return strings.TrimSpace(strings.TrimPrefix(trim, "# ")), strings.TrimSpace(rest)
	}
	if strings.HasPrefix(trim, "## ") {
		return strings.TrimSpace(strings.TrimPrefix(trim, "## ")), strings.TrimSpace(rest)
	}
	return "", s
}

func codeFenceOpen(line string) (lang string, ok bool) {
	trim := strings.TrimSpace(line)
	if !strings.HasPrefix(trim, "```") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(trim, "```")), true
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func textMsgContent(s string) string {
	b, _ := json.Marshal(map[string]string{"text": s})
	return string(b)
}

func imageMsgContent(imageKey string) string {
	b, _ := json.Marshal(map[string]string{"image_key": imageKey})
	return string(b)
}
