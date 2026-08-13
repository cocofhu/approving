package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

// Fixed remote CSS viewport. A stable 1920x1080 (16:9) desktop size; the UI
// scales/letterboxes it to whatever panel or fullscreen size the viewer uses, so
// the page never renders in an odd panel-shaped viewport. Pinned via
// Emulation.setDeviceMetricsOverride at DSF 1, so the captured frame's pixels equal
// CSS pixels and client click coordinates map 1:1 (no device-scale/retina games).
const (
	ViewportWidth  = 1920
	ViewportHeight = 1080

	// desktopBoundsSlackPx: getWindowForTarget may report slightly under the
	// requested outer size (chrome frame / rounding). Below this, Overlay
	// hit-testing on Xvfb is unreliable → refuse inspect.
	desktopBoundsSlackPx = 80
	desktopBoundsRetries = 3
	desktopBoundsRetry   = 80 * time.Millisecond
)

// ErrDesktopNotReady means the headed Chromium window is not ≈ the Xvfb desktop
// after SetWindow; Overlay inspect would mis-hit (click-through). Callers must
// refuse entering inspect and surface a not-ready control message.
var ErrDesktopNotReady = errors.New("desktop window not ready for inspect")

// dialRod connects to a Chromium container's CDP endpoint (http://ip:9222) and
// returns an Engine. The browser is NOT bound to the caller's request context so
// it outlives individual requests; its lifetime is the container's.
func dialRod(_ context.Context, httpBase string) (Engine, error) {
	ws, err := launcher.ResolveURL(httpBase)
	if err != nil {
		return nil, fmt.Errorf("resolve cdp url: %w", err)
	}
	b := rod.New().ControlURL(ws)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &rodEngine{browser: b}, nil
}

type rodEngine struct{ browser *rod.Browser }

func (e *rodEngine) NewTab(_ context.Context, url string) (Page, error) {
	// Each tab gets its own browser context (isolated cookies/storage), disposed
	// on close, so multiple viewers of the same app don't share login state.
	ctxRes, err := proto.TargetCreateBrowserContext{DisposeOnDetach: false}.Call(e.browser)
	if err != nil {
		return nil, fmt.Errorf("create browser context: %w", err)
	}
	page, err := e.browser.Page(proto.TargetCreateTarget{URL: url, BrowserContextID: ctxRes.BrowserContextID})
	if err != nil {
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: ctxRes.BrowserContextID}.Call(e.browser)
		return nil, fmt.Errorf("create page: %w", err)
	}
	rp := &rodPage{engine: e, page: page, ctxID: ctxRes.BrowserContextID}
	// Headed Chromium on Xvfb (no window manager) opens NewTab as a second,
	// often undersized window — leaving a black desktop strip in noVNC and
	// breaking Overlay inspect hit-testing. Fill + focus before viewport pin.
	// Tab open stays best-effort; SetInspect(on) hard-gates on readiness.
	if err := rp.presentDesktop(); err != nil {
		log.Debug().Err(err).Msg("preview NewTab presentDesktop not ready yet")
	}
	// Pin the CSS viewport to 1920x1080 at DSF 1 so layout matches the outer
	// window we just sized (VNC mouse ↔ Overlay coordinates stay aligned).
	_ = rp.SetViewport(ViewportWidth, ViewportHeight, 1)
	rp.installPickListener()
	return rp, nil
}

// desktopWindowBounds is the outer Chromium window on the Xvfb 1920x1080 display.
func desktopWindowBounds() *proto.BrowserBounds {
	left, top := 0, 0
	w, h := ViewportWidth, ViewportHeight
	return &proto.BrowserBounds{
		Left: &left, Top: &top, Width: &w, Height: &h,
		WindowState: proto.BrowserWindowStateNormal,
	}
}

// windowBoundsReady reports whether CDP window bounds are ≈ the Xvfb desktop.
func windowBoundsReady(b *proto.BrowserBounds) bool {
	if b == nil || b.Width == nil || b.Height == nil {
		return false
	}
	minW := ViewportWidth - desktopBoundsSlackPx
	minH := ViewportHeight - desktopBoundsSlackPx
	return *b.Width >= minW && *b.Height >= minH
}

// readWindowBounds uses Browser.getWindowForTarget (includes Bounds).
func (rp *rodPage) readWindowBounds() (*proto.BrowserBounds, error) {
	res, err := proto.BrowserGetWindowForTarget{TargetID: rp.page.TargetID}.Call(rp.page)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("getWindowForTarget: empty result")
	}
	return res.Bounds, nil
}

// presentDesktop focuses the tab, forces the headed window to cover Xvfb, then
// reads back bounds. Returns ErrDesktopNotReady when geometry is still wrong
// after short retries (inspect must not enter Overlay searchForNode).
func (rp *rodPage) presentDesktop() error {
	if rp == nil || rp.page == nil {
		return fmt.Errorf("%w: nil page", ErrDesktopNotReady)
	}
	_ = proto.PageBringToFront{}.Call(rp.page)
	if _, err := rp.page.Activate(); err != nil {
		log.Debug().Err(err).Msg("preview presentDesktop Activate")
	}

	var lastErr error
	for attempt := 0; attempt < desktopBoundsRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(desktopBoundsRetry)
		}
		if err := rp.page.SetWindow(desktopWindowBounds()); err != nil {
			lastErr = err
			log.Debug().Err(err).Int("attempt", attempt).Msg("preview presentDesktop SetWindow")
			continue
		}
		bounds, err := rp.readWindowBounds()
		if err != nil {
			lastErr = err
			log.Debug().Err(err).Int("attempt", attempt).Msg("preview presentDesktop getWindowForTarget")
			continue
		}
		if windowBoundsReady(bounds) {
			return nil
		}
		w, h := 0, 0
		if bounds.Width != nil {
			w = *bounds.Width
		}
		if bounds.Height != nil {
			h = *bounds.Height
		}
		lastErr = fmt.Errorf("bounds %dx%d want ~%dx%d", w, h, ViewportWidth, ViewportHeight)
		log.Debug().Int("w", w).Int("h", h).Int("attempt", attempt).Msg("preview presentDesktop bounds not ready")
	}
	if lastErr == nil {
		lastErr = errors.New("unknown")
	}
	return fmt.Errorf("%w: %v", ErrDesktopNotReady, lastErr)
}

func (e *rodEngine) Close() error { return e.browser.Close() }

type rodPage struct {
	engine *rodEngine
	page   *rod.Page
	ctxID  proto.BrowserBrowserContextID

	mu                sync.Mutex
	onPick            func(Pick)
	onInspectCanceled func()
	onDescribeFailed  func()
	inspectCancel     inspectCancelFilter
}

func (rp *rodPage) OnPick(cb func(Pick)) {
	rp.mu.Lock()
	rp.onPick = cb
	rp.mu.Unlock()
}

func (rp *rodPage) getPick() func(Pick) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.onPick
}

func (rp *rodPage) OnInspectCanceled(cb func()) {
	rp.mu.Lock()
	rp.onInspectCanceled = cb
	rp.mu.Unlock()
}

func (rp *rodPage) getInspectCanceled() func() {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.onInspectCanceled
}

func (rp *rodPage) OnDescribeFailed(cb func()) {
	rp.mu.Lock()
	rp.onDescribeFailed = cb
	rp.mu.Unlock()
}

func (rp *rodPage) getDescribeFailed() func() {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.onDescribeFailed
}

func (rp *rodPage) StartScreencast(onFrame func(Frame)) error {
	go rp.page.EachEvent(func(e *proto.PageScreencastFrame) {
		w, h := 0, 0
		if e.Metadata != nil {
			w, h = int(e.Metadata.DeviceWidth), int(e.Metadata.DeviceHeight)
		}
		onFrame(Frame{Data: e.Data, DeviceWidth: w, DeviceHeight: h})
		_ = proto.PageScreencastFrameAck{SessionID: e.SessionID}.Call(rp.page)
	})()
	// EveryNthFrame caps encoding at ~30fps (of a ~60fps compositor): a static page
	// emits no frames anyway, so this only bounds bursts and spares server JPEG-encode
	// CPU / bandwidth. Client + server both keep only the latest frame on top of this.
	q, nth := 85, 2
	return proto.PageStartScreencast{
		Format: proto.PageStartScreencastFormatJpeg, Quality: &q, EveryNthFrame: &nth,
	}.Call(rp.page)
}

func (rp *rodPage) DispatchMouse(m MouseEvent) error {
	ev := proto.InputDispatchMouseEvent{X: m.X, Y: m.Y, ClickCount: m.ClickCount, Button: mapButton(m.Button)}
	buttons := m.Buttons
	ev.Buttons = &buttons
	switch m.Type {
	case "move":
		ev.Type = proto.InputDispatchMouseEventTypeMouseMoved
	case "down":
		ev.Type = proto.InputDispatchMouseEventTypeMousePressed
	case "up":
		ev.Type = proto.InputDispatchMouseEventTypeMouseReleased
	case "wheel":
		ev.Type = proto.InputDispatchMouseEventTypeMouseWheel
		ev.DeltaX = m.DeltaX
		ev.DeltaY = m.DeltaY
	default:
		return fmt.Errorf("unknown mouse event %q", m.Type)
	}
	return ev.Call(rp.page)
}

func mapButton(b string) proto.InputMouseButton {
	switch b {
	case "left":
		return proto.InputMouseButtonLeft
	case "middle":
		return proto.InputMouseButtonMiddle
	case "right":
		return proto.InputMouseButtonRight
	default:
		return proto.InputMouseButtonNone
	}
}

func (rp *rodPage) DispatchKey(k KeyEvent) error {
	ev := proto.InputDispatchKeyEvent{Key: k.Key, Code: k.Code, Text: k.Text, WindowsVirtualKeyCode: k.KeyCode}
	switch k.Type {
	case "down":
		ev.Type = proto.InputDispatchKeyEventTypeKeyDown
	case "up":
		ev.Type = proto.InputDispatchKeyEventTypeKeyUp
	case "char":
		ev.Type = proto.InputDispatchKeyEventTypeChar
	default:
		return fmt.Errorf("unknown key event %q", k.Type)
	}
	return ev.Call(rp.page)
}

func (rp *rodPage) SetViewport(width, height int, dpr float64) error {
	if dpr <= 0 {
		dpr = 1
	}
	return proto.EmulationSetDeviceMetricsOverride{
		Width: width, Height: height, DeviceScaleFactor: dpr, Mobile: false,
	}.Call(rp.page)
}

func (rp *rodPage) SetInspect(on bool) error {
	if !on {
		rp.inspectCancel.set(false)
		_ = proto.OverlayHideHighlight{}.Call(rp.page)
		err := proto.OverlaySetInspectMode{Mode: proto.OverlayInspectModeNone}.Call(rp.page)
		if err != nil {
			return err
		}
		_ = proto.OverlayDisable{}.Call(rp.page)
		return nil
	}
	// Hard gate: refuse Overlay searchForNode when geometry is still wrong —
	// otherwise VNC clicks hit the page (focus/input) instead of inspect.
	if err := rp.presentDesktop(); err != nil {
		return err
	}
	rp.inspectCancel.set(true)
	_ = proto.DOMEnable{}.Call(rp.page)
	_ = proto.OverlayEnable{}.Call(rp.page)
	content, border := 0.4, 1.0
	cfg := &proto.OverlayHighlightConfig{
		ContentColor: &proto.DOMRGBA{R: 111, G: 168, B: 220, A: &content},
		BorderColor:  &proto.DOMRGBA{R: 79, G: 133, B: 214, A: &border},
	}
	return proto.OverlaySetInspectMode{Mode: proto.OverlayInspectModeSearchForNode, HighlightConfig: cfg}.Call(rp.page)
}

func (rp *rodPage) Navigate(action string) error {
	switch action {
	case "reload":
		return rp.page.Reload()
	case "back":
		return rp.page.NavigateBack()
	case "forward":
		return rp.page.NavigateForward()
	default:
		return fmt.Errorf("unknown navigate action %q", action)
	}
}

func (rp *rodPage) Goto(url string) error {
	if url == "" {
		url = "about:blank"
	}
	return rp.page.Navigate(url)
}

func (rp *rodPage) Close() error {
	rp.inspectCancel.stop()
	err := rp.page.Close()
	// Dispose the isolated browser context so it doesn't leak in a long-lived
	// container.
	_ = proto.TargetDisposeBrowserContext{BrowserContextID: rp.ctxID}.Call(rp.engine.browser)
	return err
}

// installPickListener wires Overlay pick + cancel events to page callbacks.
func (rp *rodPage) installPickListener() {
	go rp.page.EachEvent(
		func(e *proto.OverlayInspectNodeRequested) {
			cb := rp.getPick()
			if cb == nil {
				_ = rp.SetInspect(false)
				return
			}
			pick, err := rp.describeBackendNode(e.BackendNodeID)
			// One-shot: leave inspect mode after a pick attempt.
			_ = rp.SetInspect(false)
			if err != nil {
				// Distinct from Esc cancel so the UI can show describe-failed tip.
				log.Warn().Err(err).Msg("preview describeBackendNode failed")
				if fail := rp.getDescribeFailed(); fail != nil {
					fail()
				}
				return
			}
			cb(pick)
		},
		func(_ *proto.OverlayInspectModeCanceled) {
			// Enabling inspect often fires this for the previous mode; ignore that
			// or the UI toggle desyncs and cancel never sticks.
			if !rp.inspectCancel.onCanceled() {
				return
			}
			if cb := rp.getInspectCanceled(); cb != nil {
				cb()
			}
		},
	)()
}

// pickScript computes a stable-ish CSS selector, tag, outerHTML and box for the
// picked element. `this` is bound to the element by rod's Element.Eval.
const pickScript = `() => {
  const el = this;
  function seg(e){
    if (e.id) return '#' + CSS.escape(e.id);
    let s = e.tagName.toLowerCase();
    const p = e.parentElement;
    if (!p) return s;
    const same = Array.from(p.children).filter(c => c.tagName === e.tagName);
    if (same.length > 1) s += ':nth-of-type(' + (same.indexOf(e)+1) + ')';
    return s;
  }
  function path(e){
    const parts = [];
    while (e && e.nodeType === 1 && e.tagName.toLowerCase() !== 'html'){
      const s = seg(e);
      parts.unshift(s);
      if (s.charAt(0) === '#') break;
      e = e.parentElement;
    }
    return parts.join(' > ');
  }
  const r = el.getBoundingClientRect();
  return {
    selector: path(el),
    tagName: el.tagName.toLowerCase(),
    outerHTML: (el.outerHTML || '').slice(0, 4000),
    url: String(location.href || ''),
    x: r.x, y: r.y, width: r.width, height: r.height
  };
}`

func (rp *rodPage) describeBackendNode(id proto.DOMBackendNodeID) (Pick, error) {
	el, err := rp.page.ElementFromNode(&proto.DOMNode{BackendNodeID: id})
	if err != nil {
		return Pick{}, err
	}
	res, err := el.Eval(pickScript)
	if err != nil {
		return Pick{}, err
	}
	v := res.Value
	return Pick{
		Selector:  v.Get("selector").Str(),
		TagName:   v.Get("tagName").Str(),
		OuterHTML: v.Get("outerHTML").Str(),
		URL:       v.Get("url").Str(),
		Box:       [4]float64{v.Get("x").Num(), v.Get("y").Num(), v.Get("width").Num(), v.Get("height").Num()},
	}, nil
}
