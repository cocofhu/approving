package channels

import (
	"encoding/base64"
	"testing"
)

func TestToPromptImagesUsesCorrectedMimeType(t *testing.T) {
	webp := []byte("RIFF\x00\x00\x00\x00WEBP")
	in := []Image{
		{Data: webp, MimeType: "image/webp", Filename: "70345B2BE"},
	}
	out := toPromptImages(in)
	if len(out) != 1 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].MimeType != "image/webp" {
		t.Fatalf("mime = %q want corrected image/webp (g1.5)", out[0].MimeType)
	}
	if out[0].Name != "70345B2BE" {
		t.Fatalf("name = %q", out[0].Name)
	}
	if out[0].Data == "" {
		t.Fatal("expected inline data for ACP ResolveForWire")
	}
}

func TestToPromptImagesSkipsEmptyData(t *testing.T) {
	out := toPromptImages([]Image{
		{MimeType: "image/png", Filename: "missing.png"}, // download failed / not ingested
		{Data: []byte("png"), MimeType: "image/png", Filename: "ok.png"},
	})
	if len(out) != 1 || out[0].Name != "ok.png" {
		t.Fatalf("failed downloads must not reach Agent images: %+v", out)
	}
}

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
