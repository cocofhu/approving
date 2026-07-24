package provider

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeAttachmentsImageAndFile(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47})
	txt := base64.StdEncoding.EncodeToString([]byte("hello"))
	dir, paths, err := MaterializeAttachments([]PromptImage{
		{Data: png, MimeType: "image/png", Name: "a.png"},
		{Data: txt, MimeType: "text/plain", Name: "b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if len(paths) != 2 {
		t.Fatalf("paths=%v", paths)
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Fatalf("want abs path, got %q", p)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	got := AppendAttachmentRefs("看一下", paths)
	if !strings.Contains(got, "看一下") || !strings.Contains(got, "a.png") || !strings.Contains(got, "b.txt") {
		t.Fatalf("refs=%q", got)
	}
}
