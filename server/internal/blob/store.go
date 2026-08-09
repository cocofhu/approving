package blob

import (
	"context"
	"io"
)

// Meta describes a stored blob's presentation metadata.
type Meta struct {
	MimeType string `json:"mimeType"`
	Name     string `json:"name,omitempty"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256,omitempty"`
}

// Store persists attachment bytes addressed by Ref.
type Store interface {
	Put(ctx context.Context, r io.Reader, meta Meta) (Ref, error)
	Open(ctx context.Context, ref Ref) (io.ReadCloser, Meta, error)
	Delete(ctx context.Context, ref Ref) error
}
