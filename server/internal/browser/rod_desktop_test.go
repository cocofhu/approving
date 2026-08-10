package browser

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

func TestDesktopWindowBoundsFillsViewport(t *testing.T) {
	b := desktopWindowBounds()
	if b == nil || b.Left == nil || b.Top == nil || b.Width == nil || b.Height == nil {
		t.Fatal("bounds incomplete")
	}
	if *b.Left != 0 || *b.Top != 0 {
		t.Fatalf("origin=%d,%d", *b.Left, *b.Top)
	}
	if *b.Width != ViewportWidth || *b.Height != ViewportHeight {
		t.Fatalf("size=%dx%d want %dx%d", *b.Width, *b.Height, ViewportWidth, ViewportHeight)
	}
	if b.WindowState != proto.BrowserWindowStateNormal {
		t.Fatalf("state=%q", b.WindowState)
	}
}
