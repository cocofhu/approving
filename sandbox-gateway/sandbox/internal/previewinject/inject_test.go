package previewinject

import (
	"strings"
	"testing"
)

const scriptURL = "http://app.example/preview-pick.js"

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

func TestInjectHTML_Idempotent(t *testing.T) {
	t.Parallel()
	in := []byte(`<html><body><script src="http://other/preview-pick.js"></script></body></html>`)
	got := InjectHTML(in, scriptURL)
	if string(got) != string(in) {
		t.Fatalf("already present must be unchanged:\n%s", got)
	}
	if n := strings.Count(string(InjectHTML(InjectHTML([]byte("<body></body>"), scriptURL), scriptURL)), "preview-pick.js"); n != 1 {
		t.Fatalf("double inject count=%d", n)
	}
}

func TestInjectHTML_EmptyURL(t *testing.T) {
	t.Parallel()
	in := []byte("<body></body>")
	if string(InjectHTML(in, "  ")) != string(in) {
		t.Fatal("empty url must be no-op")
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
