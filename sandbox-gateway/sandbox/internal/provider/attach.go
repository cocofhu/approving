package provider

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaterializeAttachments writes base64 image/file attachments under a unique
// /tmp directory and returns absolute paths the agent can open with ordinary
// Read tools. Caller must os.RemoveAll(dir) when the turn finishes.
func MaterializeAttachments(files []PromptImage) (dir string, paths []string, err error) {
	dir, err = os.MkdirTemp("", "sbx-attach-*")
	if err != nil {
		return "", nil, fmt.Errorf("attach: mkdir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	paths = make([]string, 0, len(files))
	for i, f := range files {
		raw, derr := DecodeAttachmentData(f.Data)
		if derr != nil {
			cleanup()
			return "", nil, fmt.Errorf("attach: decode #%d: %w", i+1, derr)
		}
		name := AttachmentFileName(i, f)
		p := filepath.Join(dir, name)
		if werr := os.WriteFile(p, raw, 0o600); werr != nil {
			cleanup()
			return "", nil, fmt.Errorf("attach: write %s: %w", name, werr)
		}
		paths = append(paths, p)
	}
	return dir, paths, nil
}

// DecodeAttachmentData decodes standard or raw base64 attachment payloads.
func DecodeAttachmentData(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty data")
	}
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// AttachmentFileName picks a safe basename for an attachment.
func AttachmentFileName(i int, f PromptImage) string {
	if n := filepath.Base(strings.TrimSpace(f.Name)); n != "" && n != "." && n != string(filepath.Separator) {
		return n
	}
	return fmt.Sprintf("attachment-%d%s", i+1, ExtForMIME(f.MimeType))
}

// ExtForMIME maps a MIME type to a file extension (default .bin).
func ExtForMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "text/markdown":
		return ".md"
	case "application/json", "text/json":
		return ".json"
	case "text/csv":
		return ".csv"
	case "text/xml", "application/xml":
		return ".xml"
	case "text/yaml", "application/yaml", "application/x-yaml":
		return ".yaml"
	default:
		return ".bin"
	}
}

// AppendAttachmentRefs appends absolute paths for the agent to read.
func AppendAttachmentRefs(text string, paths []string) string {
	if len(paths) == 0 {
		return text
	}
	var b strings.Builder
	if text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	b.WriteString("用户上传了以下附件，请直接读取这些本地文件路径：\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	return b.String()
}
