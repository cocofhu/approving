package channels

import (
	"encoding/base64"
	"testing"
)

func TestToPromptImagesPreservesFilename(t *testing.T) {
	in := []Image{
		{Data: []byte("pdf"), MimeType: "application/pdf", Filename: "需求.pdf"},
		{Data: []byte("png"), MimeType: "image/png"}, // no filename → fallback
	}
	out := toPromptImages(in)
	if len(out) != 2 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].Name != "需求.pdf" {
		t.Fatalf("name[0] = %q", out[0].Name)
	}
	if out[0].MimeType != "application/pdf" {
		t.Fatalf("mime[0] = %q", out[0].MimeType)
	}
	if got, _ := base64.StdEncoding.DecodeString(out[0].Data); string(got) != "pdf" {
		t.Fatalf("data[0] = %q", got)
	}
	if out[1].Name != "attachment-2.png" {
		t.Fatalf("fallback name = %q want attachment-2.png", out[1].Name)
	}
}
