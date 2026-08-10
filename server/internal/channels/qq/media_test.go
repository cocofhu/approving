package qq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/channels"
)

func TestHostNeedsQQBotAuth(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"multimedia.nt.qq.com.cn", true},
		{"foo.multimedia.nt.qq.com.cn", true},
		{"multimedia.nt.qq.com", true},
		{"gchat.qpic.cn", false},
		{"grouptalk.c2c.qq.com", false},
		{"example.com", false},
		{"", false},
	}
	for _, c := range cases {
		if got := hostNeedsQQBotAuth(c.host); got != c.want {
			t.Errorf("hostNeedsQQBotAuth(%q) = %v want %v", c.host, got, c.want)
		}
	}
}

func TestNormalizeInboundImageMIME(t *testing.T) {
	webp := []byte("RIFF\x00\x00\x00\x00WEBP")
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	html := []byte("<!doctype html><html>err</html>")

	// Mislabelled WEBP claimed as jpeg → sniff wins (g1.3).
	if got := normalizeInboundImageMIME("image/jpeg; charset=utf-8", "image/jpeg", webp); got != "image/webp" {
		t.Fatalf("mislabelled webp = %q", got)
	}
	// Empty / file claim + PNG bytes.
	if got := normalizeInboundImageMIME("file", "", png); got != "image/png" {
		t.Fatalf("file+png = %q", got)
	}
	if got := normalizeInboundImageMIME("", "application/octet-stream", png); got != "image/png" {
		t.Fatalf("empty+png = %q", got)
	}
	// Claimed image/* but HTML error page → do not keep jpeg lie.
	if got := normalizeInboundImageMIME("image/jpeg", "image/jpeg", html); got == "image/jpeg" {
		t.Fatalf("html claimed jpeg should not stay image/jpeg, got %q", got)
	}
	// Non-image PDF claim is kept when bytes are not a supported image.
	if got := normalizeInboundImageMIME("application/pdf", "", []byte("%PDF-1.4")); got != "application/pdf" {
		t.Fatalf("pdf = %q", got)
	}
}

func TestFormatQQImageDownloadHint(t *testing.T) {
	cases := []struct {
		n, m int
		want string
	}{
		{0, 0, ""},
		{1, 0, ""},
		{1, 1, "收到 1 张图片，但下载失败，未能展示。"},
		{3, 3, "收到 3 张图片，但下载失败，未能展示。"},
		{3, 1, "收到 3 张图片，其中 1 张下载失败，未能展示。"},
		{5, 1, "收到 5 张图片，其中 1 张下载失败，未能展示。"},
	}
	for _, c := range cases {
		if got := formatQQImageDownloadHint(c.n, c.m); got != c.want {
			t.Errorf("formatQQImageDownloadHint(%d,%d) = %q want %q", c.n, c.m, got, c.want)
		}
	}
}

func TestLooksLikeInboundImage(t *testing.T) {
	cases := []struct {
		att  attachment
		want bool
	}{
		{attachment{ContentType: "image/png"}, true},
		{attachment{ContentType: "file", Filename: "70345B2BE"}, true},
		{attachment{ContentType: "", Filename: "70345B2BE"}, true},
		{attachment{ContentType: "application/pdf", Filename: "doc.pdf"}, false},
		{attachment{ContentType: "", Filename: "report.pdf"}, false},
		{attachment{ContentType: "application/zip"}, false},
	}
	for _, c := range cases {
		if got := looksLikeInboundImage(c.att); got != c.want {
			t.Errorf("looksLikeInboundImage(%+v) = %v want %v", c.att, got, c.want)
		}
	}
}

func TestAttachmentUnmarshalContentType(t *testing.T) {
	var snake attachment
	if err := json.Unmarshal([]byte(`{"content_type":"image/png","url":"https://x/a","filename":"a.png"}`), &snake); err != nil {
		t.Fatal(err)
	}
	if snake.ContentType != "image/png" || snake.Filename != "a.png" {
		t.Fatalf("snake = %+v", snake)
	}
	var camel attachment
	if err := json.Unmarshal([]byte(`{"contentType":"image/webp","url":"https://x/b","filename":"70345B2BE"}`), &camel); err != nil {
		t.Fatal(err)
	}
	if camel.ContentType != "image/webp" || camel.Filename != "70345B2BE" {
		t.Fatalf("camel = %+v", camel)
	}
	var both attachment
	if err := json.Unmarshal([]byte(`{"content_type":"image/jpeg","contentType":"image/png","url":"u"}`), &both); err != nil {
		t.Fatal(err)
	}
	if both.ContentType != "image/jpeg" {
		t.Fatalf("prefer content_type, got %q", both.ContentType)
	}
}

func TestCollectInboundAttachmentsCapCountsAsFailed(t *testing.T) {
	png := channels.Image{Data: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, MimeType: "image/png", Filename: "ok.png"}
	var calls int
	dl := func(ctx context.Context, att attachment, auth string) (channels.Image, error) {
		calls++
		return png, nil
	}
	atts := make([]attachment, 5)
	for i := range atts {
		atts[i] = attachment{ContentType: "image/png", Filename: fmt.Sprintf("p%d.png", i+1), URL: "https://x/" + fmt.Sprint(i)}
	}
	images, hint, oversized := collectInboundAttachments(context.Background(), atts, "QQBot tok", dl)
	if len(images) != maxInboundImages {
		t.Fatalf("images = %d want %d", len(images), maxInboundImages)
	}
	if calls != maxInboundImages {
		t.Fatalf("downloads = %d want %d (do not expand cap)", calls, maxInboundImages)
	}
	if len(oversized) != 0 {
		t.Fatalf("oversized = %v", oversized)
	}
	want := "收到 5 张图片，其中 1 张下载失败，未能展示。"
	if hint != want {
		t.Fatalf("hint = %q want %q", hint, want)
	}
}

func TestCollectInboundAttachmentsPartialFail(t *testing.T) {
	png := channels.Image{Data: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, MimeType: "image/png", Filename: "ok.png"}
	dl := func(ctx context.Context, att attachment, auth string) (channels.Image, error) {
		if strings.Contains(att.Filename, "bad") {
			return channels.Image{}, fmt.Errorf("download image http 403")
		}
		return png, nil
	}
	atts := []attachment{
		{ContentType: "image/jpeg", Filename: "ok1.jpg"},
		{ContentType: "file", Filename: "bad-md5"},
		{ContentType: "image/png", Filename: "ok2.png"},
	}
	images, hint, _ := collectInboundAttachments(context.Background(), atts, "", dl)
	if len(images) != 2 {
		t.Fatalf("images = %d want 2", len(images))
	}
	want := "收到 3 张图片，其中 1 张下载失败，未能展示。"
	if hint != want {
		t.Fatalf("hint = %q want %q", hint, want)
	}
}

func TestCollectInboundAttachmentsAllFail(t *testing.T) {
	dl := func(ctx context.Context, att attachment, auth string) (channels.Image, error) {
		return channels.Image{}, fmt.Errorf("download image http 403")
	}
	atts := []attachment{{ContentType: "file", Filename: "70345B2BE"}}
	images, hint, _ := collectInboundAttachments(context.Background(), atts, "QQBot tok", dl)
	if len(images) != 0 {
		t.Fatalf("images = %d", len(images))
	}
	if hint != "收到 1 张图片，但下载失败，未能展示。" {
		t.Fatalf("hint = %q", hint)
	}
}

func TestCollectInboundAttachmentsOversizedNotInHint(t *testing.T) {
	dl := func(ctx context.Context, att attachment, auth string) (channels.Image, error) {
		return channels.Image{}, errInboundTooLarge
	}
	atts := []attachment{{ContentType: "image/png", Filename: "huge.png"}}
	images, hint, oversized := collectInboundAttachments(context.Background(), atts, "", dl)
	if len(images) != 0 || hint != "" {
		t.Fatalf("images=%d hint=%q", len(images), hint)
	}
	if len(oversized) != 1 || oversized[0] != "huge.png" {
		t.Fatalf("oversized = %v", oversized)
	}
}

func TestCollectInboundAttachmentsPassesAuthHeader(t *testing.T) {
	png := channels.Image{Data: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, MimeType: "image/png"}
	var gotAuth string
	dl := func(ctx context.Context, att attachment, auth string) (channels.Image, error) {
		gotAuth = auth
		return png, nil
	}
	_, _, _ = collectInboundAttachments(context.Background(), []attachment{{ContentType: "image/png", Filename: "a.png"}}, "QQBot tok", dl)
	if gotAuth != "QQBot tok" {
		t.Fatalf("auth = %q want QQBot tok (g1.1)", gotAuth)
	}
}

func TestDownloadImageSniffsMIMEWithoutAuthOnPublicIP(t *testing.T) {
	webp := []byte("RIFF\x00\x00\x00\x00WEBP")
	var gotAuth string
	orig := inboundHTTP
	t.Cleanup(func() { inboundHTTP = orig })
	inboundHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"image/jpeg; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader(string(webp))),
			Request:    r,
		}, nil
	})}

	img, err := downloadImage(context.Background(), attachment{
		URL:         "https://8.8.8.8/a.webp",
		ContentType: "image/jpeg",
		Filename:    "70345B2BE",
	}, "QQBot tok")
	if err != nil {
		t.Fatal(err)
	}
	if img.MimeType != "image/webp" {
		t.Fatalf("mime = %q want image/webp (g1.3/g1.5)", img.MimeType)
	}
	if img.Filename != "70345B2BE" {
		t.Fatalf("filename = %q", img.Filename)
	}
	if gotAuth != "" {
		t.Fatalf("non-auth CDN host must not send QQBot token, got %q", gotAuth)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, c := range cases {
		got := isBlockedIP(net.ParseIP(c.ip))
		if got != c.want {
			t.Errorf("isBlockedIP(%s) = %v want %v", c.ip, got, c.want)
		}
	}
}

func TestValidatePublicHTTPURL(t *testing.T) {
	if err := validatePublicHTTPURL("ftp://example.com/a.png"); err == nil {
		t.Fatal("expected scheme reject")
	}
	if err := validatePublicHTTPURL("https://127.0.0.1/a.png"); err == nil {
		t.Fatal("expected loopback reject")
	}
	if err := validatePublicHTTPURL("https://10.1.2.3/a.png"); err == nil {
		t.Fatal("expected private reject")
	}
	if err := validatePublicHTTPURL("https://8.8.8.8/a.png"); err != nil {
		t.Fatalf("public IP should pass: %v", err)
	}
}
