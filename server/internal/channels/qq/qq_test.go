package qq

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/sendable"
)

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

// testAdapter builds an adapter talking to srvURL with a pre-primed token so no
// real QQ endpoint is contacted.
func testAdapter(t *testing.T, srvURL string) *Adapter {
	t.Helper()
	a, err := New(channels.AdapterConfig{
		Type: "qq", AppID: "app", AppSecret: "sec", ProjectID: "proj",
		Config: map[string]any{"apiBase": srvURL, "markdown": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	qa := a.(*Adapter)
	qa.client.token = "token"
	qa.client.tokenExp = time.Now().Add(time.Hour)
	return qa
}

func testEnvelope() sendable.DeliveryEnvelope {
	return sendable.AppendSendable(sendable.DeliveryEnvelope{
		Priority: sendable.PriorityHigh, RunID: "r1", ProjectID: "proj",
		ConversationID: "user1", UserID: "user1",
		Reason: "test", Kind: sendable.KindSafetyNotice,
	}, sendable.ChannelQQ)
}

func TestSendReturnsRealMessageID(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = w.Write([]byte(`{"file_info":"file-1"}`))
		default:
			_, _ = w.Write([]byte(`{"id":"MSG-REAL-1"}`))
		}
	}))
	defer srv.Close()
	a := testAdapter(t, srv.URL)

	res, err := a.Send(context.Background(), channels.OutboundMessage{
		Scene: channels.SceneC2C, ConversationID: "user1", Text: "hello",
		ImageURLs: []string{"https://x.com/a.png"}, Envelope: testEnvelope(),
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Text goes first, so the multi-part send is identified by the text message.
	if res.MessageID != "MSG-REAL-1" {
		t.Fatalf("MessageID = %q want the id from the QQ response", res.MessageID)
	}
	if len(paths) != 3 {
		t.Fatalf("request paths = %v want text + upload + media", paths)
	}
}

func TestSendWithoutResponseIDReportsNoMessageID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	a := testAdapter(t, srv.URL)

	res, err := a.Send(context.Background(), channels.OutboundMessage{
		Scene: channels.SceneGroup, ConversationID: "group1", Text: "hello",
		Envelope: testEnvelope(),
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	// No id in the response means no id at all: nothing local may stand in.
	if res.MessageID != "" {
		t.Fatalf("MessageID = %q want empty when QQ reported none", res.MessageID)
	}
}

func TestSendGuildReturnsRealMessageID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"GUILD-MSG-1"}`))
	}))
	defer srv.Close()
	a := testAdapter(t, srv.URL)

	res, err := a.Send(context.Background(), channels.OutboundMessage{
		Scene: channels.SceneGuild, ConversationID: "chan1", Text: "hello",
		Envelope: testEnvelope(),
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.MessageID != "GUILD-MSG-1" {
		t.Fatalf("MessageID = %q", res.MessageID)
	}
}

func TestSendFailClosedOnPolicyGate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("gate must close before any request (%s)", r.URL.Path)
	}))
	defer srv.Close()
	a := testAdapter(t, srv.URL)

	// Internal-only envelope: the adapter's second gate refuses it.
	if _, err := a.Send(context.Background(), channels.OutboundMessage{
		Scene: channels.SceneC2C, ConversationID: "user1", Text: "hello",
		Envelope: sendable.Internal(sendable.KindAgentRaw, "not_sendable"),
	}); err == nil {
		t.Fatal("Send must fail closed for a non-sendable envelope")
	}
}

func TestApplyOversizedNoticeRoutesThroughManager(t *testing.T) {
	base := channels.InboundMessage{
		Scene: channels.SceneC2C, ConversationID: "user1", UserID: "user1", MessageID: "m1",
	}

	// Nothing but rejected attachments: the tip becomes a notice for the
	// Manager's single egress, keyed for reconnect idempotency.
	only := applyOversizedNotice(base, []string{"big.zip"})
	if only.Safety == nil || !only.Safety.Only {
		t.Fatalf("pure oversize inbound = %+v want a notice-only safety notice", only)
	}
	if only.Safety.DedupeKey != "m1:oversize" || only.Safety.Reason != "oversized_attachment" {
		t.Fatalf("safety notice = %+v", only.Safety)
	}
	if !strings.Contains(only.Safety.Text, "big.zip") ||
		!strings.Contains(only.Safety.Text, fmt.Sprintf("%d MiB", qqAttachMaxMiB)) {
		t.Fatalf("safety notice text = %q", only.Safety.Text)
	}
	if only.Text != "" {
		t.Fatalf("pure oversize must not fabricate turn text: %q", only.Text)
	}

	// With user text the turn still runs and the tip rides along.
	mixed := base
	mixed.Text = "看下这个"
	mixed = applyOversizedNotice(mixed, []string{"big.zip"})
	if mixed.Safety != nil {
		t.Fatalf("mixed inbound must not short-circuit the turn: %+v", mixed.Safety)
	}
	if !strings.Contains(mixed.Text, "看下这个") || !strings.Contains(mixed.Text, "big.zip") {
		t.Fatalf("mixed text = %q", mixed.Text)
	}

	// No rejected attachment changes nothing.
	if got := applyOversizedNotice(base, nil); got.Safety != nil || got.Text != "" {
		t.Fatalf("unchanged inbound = %+v", got)
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
