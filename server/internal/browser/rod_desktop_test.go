package browser

import (
	"errors"
	"fmt"
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

func TestWindowBoundsReadyThreshold(t *testing.T) {
	fullW, fullH := ViewportWidth, ViewportHeight
	if !windowBoundsReady(&proto.BrowserBounds{Width: &fullW, Height: &fullH}) {
		t.Fatal("exact desktop should be ready")
	}
	okW := ViewportWidth - desktopBoundsSlackPx
	okH := ViewportHeight - desktopBoundsSlackPx
	if !windowBoundsReady(&proto.BrowserBounds{Width: &okW, Height: &okH}) {
		t.Fatal("slack boundary should be ready")
	}
	smallW := ViewportWidth - desktopBoundsSlackPx - 1
	smallH := ViewportHeight
	if windowBoundsReady(&proto.BrowserBounds{Width: &smallW, Height: &smallH}) {
		t.Fatal("undersized width must not be ready (g1.1/g1.2 gate)")
	}
	if windowBoundsReady(nil) || windowBoundsReady(&proto.BrowserBounds{}) {
		t.Fatal("nil/empty bounds must not be ready")
	}
}

func TestErrDesktopNotReadySentinel(t *testing.T) {
	err := fmt.Errorf("%w: bounds 800x600 want ~1920x1080", ErrDesktopNotReady)
	if !errors.Is(err, ErrDesktopNotReady) {
		t.Fatal("wrapped ErrDesktopNotReady must match errors.Is")
	}
}
