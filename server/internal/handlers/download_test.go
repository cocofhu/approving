package handlers

import (
	"encoding/base64"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestDecodeArtifactDownloadBody(t *testing.T) {
	pngMagic := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	b64 := base64.StdEncoding.EncodeToString(pngMagic)

	t.Run("image kind decodes base64 PNG", func(t *testing.T) {
		body, mime := decodeArtifactDownloadBody(models.Artifact{
			Name: "shot.png", Kind: "image", Content: b64,
		})
		if mime != "image/png" {
			t.Fatalf("mime = %q, want image/png", mime)
		}
		if len(body) < 8 || string(body[:8]) != string(pngMagic) {
			t.Fatalf("body = %v, want PNG magic", body)
		}
	})

	t.Run("png suffix decodes without image kind", func(t *testing.T) {
		body, mime := decodeArtifactDownloadBody(models.Artifact{
			Name: "screenshot.png", Kind: "json", Content: b64,
		})
		if mime != "image/png" {
			t.Fatalf("mime = %q", mime)
		}
		if len(body) != len(pngMagic) {
			t.Fatalf("len = %d", len(body))
		}
	})

	t.Run("non-image keeps octet-stream", func(t *testing.T) {
		body, mime := decodeArtifactDownloadBody(models.Artifact{
			Name: "test_result.json", Kind: "json", Content: `{"summary":"ok"}`,
		})
		if mime != "application/octet-stream" {
			t.Fatalf("mime = %q", mime)
		}
		if string(body) != `{"summary":"ok"}` {
			t.Fatalf("body = %q", body)
		}
	})

	t.Run("invalid base64 falls back to raw content", func(t *testing.T) {
		body, mime := decodeArtifactDownloadBody(models.Artifact{
			Name: "bad.png", Kind: "image", Content: "not-valid-base64!!!",
		})
		if mime != "application/octet-stream" {
			t.Fatalf("mime = %q", mime)
		}
		if string(body) != "not-valid-base64!!!" {
			t.Fatalf("body = %q", string(body))
		}
	})
}
