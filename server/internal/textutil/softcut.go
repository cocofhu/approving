package textutil

import "strings"

// Shortening text is where user-facing copy goes wrong most often. Three
// separate bugs shipped from three separate hand-rolled cuts: a byte slice that
// split a Chinese character into mojibake, a rune slice that ended a title at
// 「…快模型和 wo」, and a model reply cut off at 「CI 还没全跑完：两个」. The
// helpers here are the one implementation every user-visible path is expected
// to use, so a fix lands once instead of per call site.

// SoftTruncateRunes shortens value to at most limit runes and marks the cut
// with an ellipsis. When the budget lands in the middle of a Latin word the cut
// walks back to the nearest boundary, however far back that is: giving up a few
// more characters costs the reader nothing, while 「快模型和 wo」 costs them the
// ability to tell which task is being talked about. Only a run of text with no
// boundary at all — one long unbroken token — is cut where the budget falls.
func SoftTruncateRunes(value string, limit int) string {
	r := []rune(strings.TrimSpace(value))
	if limit <= 0 || len(r) <= limit {
		return string(r)
	}
	// Chinese does not space its words, so a cut between two Han characters is
	// already at a boundary and only a Latin token needs walking back.
	if !splitsLatinToken(r, limit) {
		return trimCutEdge(string(r[:limit])) + "…"
	}
	for i := limit - 1; i >= 0; i-- {
		if isSeparator(r[i]) {
			if kept := trimCutEdge(string(r[:i])); kept != "" {
				return kept + "…"
			}
			break
		}
		if isHan(r[i]) {
			return trimCutEdge(string(r[:i+1])) + "…"
		}
	}
	return trimCutEdge(string(r[:limit])) + "…"
}

// splitsLatinToken reports whether cutting at index at would land inside a
// Latin word or number rather than between two tokens.
func splitsLatinToken(r []rune, at int) bool {
	if at <= 0 || at >= len(r) {
		return false
	}
	return isTokenChar(r[at-1]) && isTokenChar(r[at])
}

func isTokenChar(r rune) bool {
	return isASCIIAlpha(r) || (r >= '0' && r <= '9')
}

func isSeparator(r rune) bool {
	switch r {
	case ' ', '　', '\n', '\t', '/', '-', '_', '|', '·',
		'。', '，', ',', '.', ';', '；', '、', ':', '：':
		return true
	}
	return false
}

func isHan(r rune) bool {
	return r >= 0x4e00 && r <= 0x9fff
}

// trimCutEdge drops the whitespace and dangling punctuation a cut leaves at the
// end, so the ellipsis attaches to a word instead of to 「，」 or a stray slash.
func trimCutEdge(s string) string {
	out := strings.TrimRight(strings.TrimSpace(s), " 　\t\n/-_|·，,、；;：:")
	if out == "" {
		return strings.TrimSpace(s)
	}
	return out
}

// HealBrokenTail repairs text that already ends in the wreckage of a hard cut:
// a one-to-three letter Latin stub trailing a Chinese phrase, as in
// 「快模型和 wo」. It exists for data that was persisted before the cut sites
// were fixed — new text should never need it.
//
// A longer stub is left alone. There is no way to tell a cut 「findi」 from a
// written 「fix」 beyond a length that short, and inventing an ellipsis inside
// someone's real sentence is the worse failure.
func HealBrokenTail(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return text
	}
	base := strings.TrimRight(trimmed, "…")
	base = strings.TrimSuffix(base, "...")
	if !HasBrokenLatinStub(base) {
		return text
	}
	r := []rune(base)
	i := len(r) - 1
	for i >= 0 && isASCIIAlpha(r[i]) {
		i--
	}
	for i >= 0 && (r[i] == ' ' || r[i] == '　') {
		i--
	}
	if i < 0 {
		return text
	}
	return string(r[:i+1]) + "…"
}

// HasBrokenLatinStub reports whether text ends in a Latin fragment that reads
// as the debris of a truncation rather than a word someone meant to write.
func HasBrokenLatinStub(text string) bool {
	r := []rune(strings.TrimSpace(text))
	if len(r) == 0 {
		return false
	}
	i := len(r) - 1
	for i >= 0 && isASCIIAlpha(r[i]) {
		i--
	}
	stub := string(r[i+1:])
	if len(stub) < 1 || len(stub) > 3 {
		return false
	}
	// A unit or a suffix is attached to what precedes it: 「降到 1.1s」 and
	// 「耗时 30m」 are measurements, not the debris of a cut.
	if i >= 0 && !isStubBoundary(r[i]) {
		return false
	}
	// Acronyms are written in caps and are the normal way to end a Chinese
	// sentence about 「合并 PR」 or 「跑 CI」.
	if stub == strings.ToUpper(stub) {
		return false
	}
	if knownShortTokens[strings.ToLower(stub)] {
		return false
	}
	// Only suspicious inside Chinese text. In English prose a short word at the
	// end is just a word.
	for _, c := range r[:i+1] {
		if c >= 0x4e00 && c <= 0x9fff {
			return true
		}
	}
	return false
}

// knownShortTokens are short Latin tokens that legitimately end a sentence, so
// they are never mistaken for truncation debris.
var knownShortTokens = map[string]bool{
	"a": true, "an": true, "i": true, "is": true, "it": true, "in": true, "on": true,
	"at": true, "to": true, "of": true, "or": true, "by": true, "be": true, "do": true,
	"go": true, "if": true, "me": true, "my": true, "no": true, "ok": true, "so": true,
	"up": true, "us": true, "we": true, "you": true, "the": true, "and": true, "for": true,
	"not": true, "all": true, "new": true, "old": true, "one": true, "two": true, "yes": true,
	"add": true, "fix": true, "run": true, "job": true, "log": true, "dev": true, "ops": true,
	"app": true, "web": true, "key": true, "tag": true, "bug": true, "out": true, "err": true,
	"api": true, "cli": true, "sdk": true, "git": true, "npm": true, "ssh": true, "sql": true,
	"url": true, "uri": true, "cpu": true, "gpu": true, "ram": true, "dns": true, "css": true,
	"dom": true, "env": true, "pkg": true, "lib": true, "src": true, "bin": true, "tmp": true,
	"req": true, "res": true, "rpc": true, "orm": true, "ide": true, "cmd": true, "llm": true,
	"mcp": true, "jwt": true, "cdn": true, "k8s": true, "pod": true, "ttl": true, "eof": true,
	"id": true, "db": true, "js": true, "ts": true, "py": true, "os": true, "io": true,
	"ui": true, "ux": true, "ai": true, "ml": true, "ip": true, "vm": true, "qa": true,
	"ci": true, "cd": true, "pr": true, "mr": true, "qq": true,
}

// isStubBoundary reports whether a trailing Latin fragment stands on its own
// rather than continuing the token before it.
func isStubBoundary(r rune) bool {
	return r == ' ' || r == '　' || isHan(r)
}

func isASCIIAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
