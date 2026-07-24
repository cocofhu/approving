package handlers

import (
	"encoding/base64"
	"path"
	"strings"

	"github.com/cocofhu/approving/internal/mcp/structured"
	"github.com/cocofhu/approving/internal/models"
)

// decodeArtifactDownloadBody returns response bytes and Content-Type for an artifact
// download. Image artifacts (kind=image or common image suffixes) are base64-decoded
// from Artifact.Content; other kinds are returned as raw octet-stream.
func decodeArtifactDownloadBody(a models.Artifact) ([]byte, string) {
	content := a.Content
	mime := "application/octet-stream"
	if !isImageArtifact(a) {
		return []byte(content), mime
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(content))
	if err != nil {
		return []byte(content), mime
	}
	return decoded, structured.GuessImageMIME(a.Name)
}

func isImageArtifact(a models.Artifact) bool {
	if a.Kind == "image" {
		return true
	}
	switch strings.ToLower(path.Ext(a.Name)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}
