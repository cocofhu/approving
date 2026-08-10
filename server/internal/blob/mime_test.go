package blob

import (
	"testing"
)

func TestStripContentTypeParams(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"image/jpeg", "image/jpeg"},
		{"image/jpeg; charset=utf-8", "image/jpeg"},
		{"image/webp; charset=binary", "image/webp"},
		{"  image/png ; charset=utf-8 ", "image/png"},
	}
	for _, c := range cases {
		if got := StripContentTypeParams(c.in); got != c.want {
			t.Errorf("StripContentTypeParams(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestSniffSupportedImageMIME(t *testing.T) {
	webp := []byte("RIFF\x00\x00\x00\x00WEBP")
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0, 0, 0, 0, 0, 0, 0}
	gif := []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00")
	html := []byte("<!doctype html><html></html>")

	if got := SniffSupportedImageMIME(webp); got != "image/webp" {
		t.Fatalf("webp12 = %q", got)
	}
	longWebp := make([]byte, 32)
	copy(longWebp, []byte("RIFF"))
	copy(longWebp[8:], []byte("WEBPVP8 "))
	if got := SniffSupportedImageMIME(longWebp); got != "image/webp" {
		t.Fatalf("webp32 = %q", got)
	}
	if got := SniffSupportedImageMIME(png); got != "image/png" {
		t.Fatalf("png = %q", got)
	}
	if got := SniffSupportedImageMIME(jpeg); got != "image/jpeg" {
		t.Fatalf("jpeg = %q", got)
	}
	if got := SniffSupportedImageMIME(gif); got != "image/gif" {
		t.Fatalf("gif = %q", got)
	}
	if got := SniffSupportedImageMIME(html); got != "" {
		t.Fatalf("html = %q want empty", got)
	}
	if got := SniffSupportedImageMIME(nil); got != "" {
		t.Fatalf("nil = %q", got)
	}
}
