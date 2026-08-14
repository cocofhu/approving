package previewinject

import (
	"strings"
	"testing"
)

const scriptURL = "http://app.example/preview-pick.js"

func TestCSPToken(t *testing.T) {
	t.Parallel()
	if got := CSPToken(ScriptPath); got != "'self'" {
		t.Fatalf("same-origin token=%q", got)
	}
	if got := CSPToken("http://app.example/preview-pick.js"); got != "http://app.example" {
		t.Fatalf("absolute token=%q", got)
	}
	if got := CSPToken(""); got != "" {
		t.Fatalf("empty token=%q", got)
	}
}

func TestScriptOrigin(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"http://app.example/preview-pick.js", "http://app.example"},
		{"https://app.example:8443/preview-pick.js", "https://app.example:8443"},
		{"  http://h/x.js  ", "http://h"},
		{"", ""},
		{"not-a-url", ""},
		{"/preview-pick.js", ""},
	}
	for _, tc := range cases {
		if got := ScriptOrigin(tc.in); got != tc.want {
			t.Errorf("ScriptOrigin(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestInjectHTML_BeforeBody(t *testing.T) {
	t.Parallel()
	in := []byte("<html><body><h1>hi</h1></body></html>")
	got := string(InjectHTML(in, scriptURL))
	if !strings.Contains(got, `<script src="`+scriptURL+`"></script></body>`) {
		t.Fatalf("inject before </body>: %s", got)
	}
	if strings.Contains(got, "<base") {
		t.Fatalf("must not inject <base>: %s", got)
	}
}

func TestInjectHTML_BeforeHTML_NoBody(t *testing.T) {
	t.Parallel()
	in := []byte("<html><p>x</p></html>")
	got := string(InjectHTML(in, scriptURL))
	if !strings.Contains(got, `<script src="`+scriptURL+`"></script></html>`) {
		t.Fatalf("inject before </html>: %s", got)
	}
}

func TestInjectHTML_AppendFragment(t *testing.T) {
	t.Parallel()
	in := []byte("<p>only</p>")
	got := string(InjectHTML(in, scriptURL))
	if !strings.HasSuffix(got, `<script src="`+scriptURL+`"></script>`) {
		t.Fatalf("append: %s", got)
	}
}

func TestInjectHTML_IdempotentSameOrigin(t *testing.T) {
	t.Parallel()
	in := []byte(`<html><body><script src="` + ScriptPath + `"></script></body></html>`)
	got := InjectHTML(in, ScriptPath)
	if string(got) != string(in) {
		t.Fatalf("same-origin already present must be unchanged:\n%s", got)
	}
	if n := strings.Count(string(InjectHTML(InjectHTML([]byte("<body></body>"), ScriptPath), ScriptPath)), ScriptPath); n != 1 {
		t.Fatalf("double inject count=%d", n)
	}
}

func TestInjectHTML_StillInjectsWhenLocalhostTagExists(t *testing.T) {
	t.Parallel()
	in := []byte(`<html><body><script src="http://localhost:8080/preview-pick.js"></script></body></html>`)
	got := string(InjectHTML(in, ""))
	if !strings.Contains(got, `src="`+ScriptPath+`"`) {
		t.Fatalf("must add same-origin script beside unreachable localhost tag: %s", got)
	}
}

func TestInjectHTML_EmptyURLUsesSameOrigin(t *testing.T) {
	t.Parallel()
	got := string(InjectHTML([]byte("<body></body>"), "  "))
	if !strings.Contains(got, `src="`+ScriptPath+`"`) {
		t.Fatalf("empty url should default to ScriptPath: %s", got)
	}
}

func TestInjectHTML_EscapesURL(t *testing.T) {
	t.Parallel()
	got := string(InjectHTML([]byte("<body></body>"), `http://x/"onclick="alert(1)`))
	if strings.Contains(got, `onclick="alert`) {
		t.Fatalf("unescaped: %s", got)
	}
	if !strings.Contains(got, "&#34;") && !strings.Contains(got, "&quot;") {
		t.Fatalf("expected escaped quote: %s", got)
	}
}

func TestRelaxCSP_ScriptSrc(t *testing.T) {
	t.Parallel()
	got := RelaxCSP("default-src 'self'; script-src 'self'", "http://app.example")
	if !strings.Contains(got, "script-src 'self' http://app.example") {
		t.Fatalf("script-src: %s", got)
	}
	if strings.Count(got, "http://app.example") != 1 {
		t.Fatalf("origin once: %s", got)
	}
	again := RelaxCSP(got, "http://app.example")
	if strings.Count(again, "http://app.example") != 1 {
		t.Fatalf("idempotent csp: %s", again)
	}
}

func TestRelaxCSP_DefaultSrcOnly(t *testing.T) {
	t.Parallel()
	got := RelaxCSP("default-src 'self'", "http://app.example")
	if !strings.Contains(got, "script-src 'self' http://app.example") {
		t.Fatalf("must copy default-src into script-src: %s", got)
	}
}

func TestRelaxCSP_NoDirectives(t *testing.T) {
	t.Parallel()
	got := RelaxCSP("frame-ancestors 'none'", "http://app.example")
	if !strings.Contains(got, "script-src http://app.example") {
		t.Fatalf("add script-src: %s", got)
	}
}

func TestRelaxCSP_Empty(t *testing.T) {
	t.Parallel()
	if RelaxCSP("", "http://x") != "" {
		t.Fatal("empty policy")
	}
	if RelaxCSP("script-src 'self'", "") != "script-src 'self'" {
		t.Fatal("empty origin")
	}
}
