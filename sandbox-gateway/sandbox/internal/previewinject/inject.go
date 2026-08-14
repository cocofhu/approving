// Package previewinject rewrites inbound HTML so Approving can load
// preview-pick.js on IP-direct preview without changing the app.
package previewinject

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

// ListenPort is the in-sandbox forwarder port. It sits below the preview
// pool (18080–18999) so KeepalivePort / set_preview never mistake this
// process for the app (the app still binds PREVIEW_PORT).
const ListenPort = 17980

var (
	reBodyClose  = regexp.MustCompile(`(?i)</body>`)
	reHTMLClose  = regexp.MustCompile(`(?i)</html>`)
	rePickScript = regexp.MustCompile(`(?i)preview-pick\.js`)
	reScriptSrc  = regexp.MustCompile(`(?i)^script-src\b`)
	reDefaultSrc = regexp.MustCompile(`(?i)^default-src\b`)
)

// AlreadyHasPickScript reports whether html already references preview-pick.js.
func AlreadyHasPickScript(html []byte) bool {
	return rePickScript.Match(html)
}

// ScriptOrigin returns scheme://host[:port] for scriptURL, or empty if unusable.
func ScriptOrigin(scriptURL string) string {
	u, err := url.Parse(strings.TrimSpace(scriptURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// InjectHTML inserts <script src="scriptURL"> before </body> (else </html>,
// else append). It is a no-op when the document already has preview-pick.js
// or scriptURL is empty. Paths and <base> are left untouched.
func InjectHTML(doc []byte, scriptURL string) []byte {
	scriptURL = strings.TrimSpace(scriptURL)
	if scriptURL == "" || AlreadyHasPickScript(doc) {
		return doc
	}
	tag := []byte(`<script src="` + html.EscapeString(scriptURL) + `"></script>`)
	if loc := reBodyClose.FindIndex(doc); loc != nil {
		return concat(doc[:loc[0]], tag, doc[loc[0]:])
	}
	if loc := reHTMLClose.FindIndex(doc); loc != nil {
		return concat(doc[:loc[0]], tag, doc[loc[0]:])
	}
	out := make([]byte, 0, len(doc)+len(tag))
	return append(append(out, doc...), tag...)
}

func concat(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// RelaxCSP adds scriptOrigin to Content-Security-Policy so the injected
// preview-pick.js is allowed. If script-src is absent but default-src is
// present, a script-src is added that copies default-src tokens (a bare
// script-src would replace default-src for scripts).
//
// nonce / strict-dynamic pages still block the tag; those stay on the
// Agent fallback. Empty policy or origin is returned unchanged.
func RelaxCSP(policy, scriptOrigin string) string {
	policy = strings.TrimSpace(policy)
	scriptOrigin = strings.TrimSpace(scriptOrigin)
	if policy == "" || scriptOrigin == "" {
		return policy
	}

	parts := strings.Split(policy, ";")
	out := make([]string, 0, len(parts)+1)
	hasScriptSrc := false
	defaultSrc := ""
	for _, raw := range parts {
		d := strings.TrimSpace(raw)
		if d == "" {
			continue
		}
		switch {
		case reScriptSrc.MatchString(d):
			hasScriptSrc = true
			if !directiveHasToken(d, scriptOrigin) {
				d = d + " " + scriptOrigin
			}
		case reDefaultSrc.MatchString(d):
			defaultSrc = d
		}
		out = append(out, d)
	}
	if !hasScriptSrc {
		if defaultSrc != "" {
			rest := strings.TrimSpace(reDefaultSrc.ReplaceAllString(defaultSrc, ""))
			if rest != "" {
				out = append(out, "script-src "+rest+" "+scriptOrigin)
			} else {
				out = append(out, "script-src "+scriptOrigin)
			}
		} else {
			out = append(out, "script-src "+scriptOrigin)
		}
	}
	return strings.Join(out, "; ")
}

func directiveHasToken(directive, token string) bool {
	for _, f := range strings.Fields(directive) {
		if strings.EqualFold(f, token) {
			return true
		}
	}
	return false
}
