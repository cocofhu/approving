package qq

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/channels"
)

func TestIsImageAttachment(t *testing.T) {
	// Helper still classifies mime/ext; inbound path no longer filters on it
	// (PDF/zip are accepted via downloadImage). Outbound image send still uses
	// filterSendableImages separately.
	cases := []struct {
		att  attachment
		want bool
	}{
		{attachment{ContentType: "image/png"}, true},
		{attachment{ContentType: "image/jpeg"}, true},
		{attachment{Filename: "photo.PNG"}, true},
		{attachment{URL: "https://x.com/a.webp"}, true},
		{attachment{ContentType: "application/pdf", Filename: "doc.pdf"}, false},
		{attachment{}, false},
	}
	for _, c := range cases {
		if got := isImageAttachment(c.att); got != c.want {
			t.Errorf("isImageAttachment(%+v) = %v want %v", c.att, got, c.want)
		}
	}
}

func TestFallbackAttachmentName(t *testing.T) {
	if got := fallbackAttachmentName(attachment{Filename: ""}, "application/pdf"); got != "attachment.pdf" {
		t.Fatalf("pdf fallback = %q", got)
	}
	if got := fallbackAttachmentName(attachment{URL: "https://x.com/files/report.zip?x=1"}, "application/zip"); got != "attachment.zip" {
		t.Fatalf("zip mime fallback = %q", got)
	}
}

func TestMaxInboundImageBytesIs20MiB(t *testing.T) {
	if maxInboundImageBytes != 20<<20 {
		t.Fatalf("maxInboundImageBytes = %d want 20MiB", maxInboundImageBytes)
	}
}

func TestFilterSendableImages(t *testing.T) {
	in := []string{
		"https://x.com/a.png",
		"https://x.com/b.JPG",
		"https://x.com/c.gif",  // gif not sendable
		"https://x.com/d.webp", // webp not sendable
		"https://x.com/e.jpeg?sig=abc",
	}
	got := filterSendableImages(in)
	want := []string{"https://x.com/a.png", "https://x.com/b.JPG", "https://x.com/e.jpeg?sig=abc"}
	if len(got) != len(want) {
		t.Fatalf("filterSendableImages = %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filterSendableImages[%d] = %q want %q", i, got[i], want[i])
		}
	}
}

func TestFilterSendableImagesCap(t *testing.T) {
	in := []string{"1.png", "2.png", "3.png", "4.png", "5.png"}
	if got := filterSendableImages(in); len(got) != maxOutboundImages {
		t.Fatalf("expected cap %d, got %d", maxOutboundImages, len(got))
	}
}

func TestCleanContent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<@!123456> hello", "hello"},
		{"<@987> hi there", "hi there"},
		{"  spaced  ", "spaced"},
		{"no mention", "no mention"},
	}
	for _, c := range cases {
		if got := cleanContent(c.in); got != c.want {
			t.Errorf("cleanContent(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestNextSeqIncrements(t *testing.T) {
	c := newClient("app", "secret", "", true)
	if got := c.nextSeq("m1"); got != 1 {
		t.Fatalf("first nextSeq = %d want 1", got)
	}
	if got := c.nextSeq("m1"); got != 2 {
		t.Fatalf("second nextSeq = %d want 2", got)
	}
	// Independent counters per msg_id.
	if got := c.nextSeq("m2"); got != 1 {
		t.Fatalf("nextSeq(m2) = %d want 1", got)
	}
}

func TestMarkdownContent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"line1\nline2", "line1\n\nline2"},
		{"a\n\n\nb", "a\n\nb"},
		{"crlf\r\nnext", "crlf\n\nnext"},
		{"- a\n- b", "- a\n\n- b"},
		{"single", "single"},
		// Fenced code blocks keep their single newlines (no double-spacing).
		{"text\n```\nl1\nl2\n```\nmore", "text\n\n```\nl1\nl2\n```\n\nmore"},
		{"```\na\n\nb\n```", "```\na\n\nb\n```"},
		{"```go\nfmt.Println()\n```", "```go\nfmt.Println()\n```"},
	}
	for _, c := range cases {
		if got := markdownContent(c.in); got != c.want {
			t.Errorf("markdownContent(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestMarkdownCapabilityCache(t *testing.T) {
	c := newClient("app", "secret", "", true)
	const key = "guild:chan1"
	if !c.markdownSupported(key) {
		t.Fatal("markdown should be supported before any failure")
	}
	c.markMarkdownUnsupported(key)
	if c.markdownSupported(key) {
		t.Fatal("markdown should be unsupported after marking")
	}
	// Expire the entry and confirm it recovers.
	c.mdCapMu.Lock()
	c.mdUnsupported[key] = time.Now().Add(-time.Minute)
	c.mdCapMu.Unlock()
	if !c.markdownSupported(key) {
		t.Fatal("expired negative-cache entry should be treated as supported")
	}

	// Disabled client never attempts markdown.
	disabled := newClient("app", "secret", "", false)
	if disabled.markdownSupported(key) {
		t.Fatal("disabled client should never report markdown supported")
	}
}

func TestMarkdownRejected(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"network error", errors.New("dial tcp: connection refused"), false},
		{"bad request", &apiError{StatusCode: 400}, true},
		{"forbidden", &apiError{StatusCode: 403}, true},
		{"rate limited", &apiError{StatusCode: 429}, false},
		{"server error", &apiError{StatusCode: 500}, false},
		{"bad gateway", &apiError{StatusCode: 502}, false},
		{"wrapped 400", fmt.Errorf("send failed: %w", &apiError{StatusCode: 400}), true},
	}
	for _, tc := range cases {
		if got := markdownRejected(tc.err); got != tc.want {
			t.Errorf("markdownRejected(%s) = %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestNewAdapterValidation(t *testing.T) {
	if _, err := New(channels.AdapterConfig{Type: "qq"}); err == nil {
		t.Fatal("New should reject missing appId/appSecret")
	}
	a, err := New(channels.AdapterConfig{Type: "qq", AppID: "app", AppSecret: "sec"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Type() != "qq" {
		t.Fatalf("Type() = %q", a.Type())
	}
}

func TestNewAdapterSandboxBase(t *testing.T) {
	a, err := New(channels.AdapterConfig{
		Type: "qq", AppID: "app", AppSecret: "sec",
		Config: map[string]any{"sandbox": true},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	qa := a.(*Adapter)
	if qa.client.apiBase != sandboxAPIBase {
		t.Fatalf("apiBase = %q want sandbox base", qa.client.apiBase)
	}
}

func TestIsDuplicate(t *testing.T) {
	a, _ := New(channels.AdapterConfig{Type: "qq", AppID: "app", AppSecret: "sec"})
	qa := a.(*Adapter)
	if qa.isDuplicate("msg1") {
		t.Fatal("first sighting should not be duplicate")
	}
	if !qa.isDuplicate("msg1") {
		t.Fatal("second sighting should be duplicate")
	}
	if qa.isDuplicate("msg2") {
		t.Fatal("different id should not be duplicate")
	}
}
