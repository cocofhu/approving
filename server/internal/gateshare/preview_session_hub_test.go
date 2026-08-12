package gateshare

import (
	"io"
	"sync/atomic"
	"testing"
)

type closeCounter struct {
	n atomic.Int32
}

func (c *closeCounter) Close() error {
	c.n.Add(1)
	return nil
}

func TestPreviewSessionHubRegisterKick(t *testing.T) {
	h := NewPreviewSessionHub()
	a := &closeCounter{}
	b := &closeCounter{}
	unregA := h.Register("th1", a)
	_ = h.Register("th1", b)
	_ = h.Register("th2", &closeCounter{})

	h.KickByTokenHash("th1")
	if a.n.Load() != 1 || b.n.Load() != 1 {
		t.Fatalf("kick th1 closes=%d/%d", a.n.Load(), b.n.Load())
	}
	// Unregister after kick is a no-op.
	unregA()

	c := &closeCounter{}
	unregC := h.Register("th3", c)
	unregC()
	h.KickByTokenHash("th3")
	if c.n.Load() != 0 {
		t.Fatalf("unregistered session should not be kicked, closes=%d", c.n.Load())
	}

	var nilCloser io.Closer
	if unreg := h.Register("", nilCloser); unreg == nil {
		t.Fatal("expected non-nil unregister noop")
	}
	h.KickMany([]string{"", "th2", "th2"})
}

func TestPreviewSessionHubNilSafe(t *testing.T) {
	var h *PreviewSessionHub
	unreg := h.Register("x", &closeCounter{})
	unreg()
	h.KickByTokenHash("x")
	h.KickMany([]string{"x"})
}
