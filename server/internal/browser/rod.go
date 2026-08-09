package browser

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// Fixed remote CSS viewport. A stable 1920x1080 (16:9) desktop size; the UI
// scales/letterboxes it to whatever panel or fullscreen size the viewer uses, so
// the page never renders in an odd panel-shaped viewport. Pinned via
// Emulation.setDeviceMetricsOverride at DSF 1, so the captured frame's pixels equal
// CSS pixels and client click coordinates map 1:1 (no device-scale/retina games).
const (
	ViewportWidth  = 1920
	ViewportHeight = 1080
)

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
	// Pin the CSS viewport to 1920x1080 at DSF 1 so the page lays out at a stable
	// desktop size regardless of the image's default window, and the frame's pixels
	// equal CSS pixels (client clicks map 1:1). Best-effort — render still works.
	_ = rp.SetViewport(ViewportWidth, ViewportHeight, 1)
	rp.installPickListener()
	return rp, nil
}

func (e *rodEngine) Close() error { return e.browser.Close() }

type rodPage struct {
	engine *rodEngine
	page   *rod.Page
	ctxID  proto.BrowserBrowserContextID

	mu                sync.Mutex
	onPick            func(Pick)
	onInspectCanceled func()
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
		_ = proto.OverlayHideHighlight{}.Call(rp.page)
		err := proto.OverlaySetInspectMode{Mode: proto.OverlayInspectModeNone}.Call(rp.page)
		if err != nil {
			return err
		}
		_ = proto.OverlayDisable{}.Call(rp.page)
		return nil
	}
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
				return
			}
			if pick, err := rp.describeBackendNode(e.BackendNodeID); err == nil {
				cb(pick)
			}
			// One-shot: leave inspect mode after a pick.
			_ = rp.SetInspect(false)
		},
		func(_ *proto.OverlayInspectModeCanceled) {
			// User Esc (or equivalent) canceled SearchForNode — notify UI to clear sticky toggle.
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
