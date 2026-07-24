package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/browser"
)

type vncRecPage struct {
	inspect *bool
	navs    []string
	gotos   []string
}

func (p *vncRecPage) StartScreencast(func(browser.Frame)) error { return nil }
func (p *vncRecPage) DispatchMouse(browser.MouseEvent) error    { return nil }
func (p *vncRecPage) DispatchKey(browser.KeyEvent) error        { return nil }
func (p *vncRecPage) SetViewport(int, int, float64) error       { return nil }
func (p *vncRecPage) SetInspect(on bool) error                  { p.inspect = &on; return nil }
func (p *vncRecPage) OnPick(func(browser.Pick))                 {}
func (p *vncRecPage) Navigate(a string) error                   { p.navs = append(p.navs, a); return nil }
func (p *vncRecPage) Goto(u string) error                       { p.gotos = append(p.gotos, u); return nil }
func (p *vncRecPage) Close() error                              { return nil }

func decodeVnc(t *testing.T, raw string) vncClientMsg {
	t.Helper()
	var m vncClientMsg
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal %q: %v", raw, err)
	}
	return m
}

func TestApplyVncMsgInspectNavigate(t *testing.T) {
	h := &Handlers{}
	p := &vncRecPage{}
	h.applyVncMsg(p, decodeVnc(t, `{"type":"inspect","on":true}`))
	if p.inspect == nil || !*p.inspect {
		t.Fatal("inspect should be enabled")
	}
	h.applyVncMsg(p, decodeVnc(t, `{"type":"navigate","action":"reload"}`))
	if len(p.navs) != 1 || p.navs[0] != "reload" {
		t.Fatalf("navigate wrong: %v", p.navs)
	}
}

func TestApplyVncMsgIgnoresUnknown(t *testing.T) {
	h := &Handlers{}
	p := &vncRecPage{}
	h.applyVncMsg(p, decodeVnc(t, `{"type":"bogus"}`))
	if p.inspect != nil || len(p.navs) != 0 {
		t.Fatalf("unknown type should be no-op: inspect=%v navs=%v", p.inspect, p.navs)
	}
}

func TestApplyVncMsgGoto(t *testing.T) {
	h := &Handlers{}
	p := &vncRecPage{}
	h.applyVncMsg(p, decodeVnc(t, `{"type":"navigate","action":"goto","url":"https://example.com/"}`))
	if len(p.gotos) != 1 || p.gotos[0] != "https://example.com/" {
		t.Fatalf("goto wrong: %v", p.gotos)
	}
	h.applyVncMsg(p, decodeVnc(t, `{"type":"navigate","action":"goto"}`))
	if len(p.gotos) != 2 || p.gotos[1] != "about:blank" {
		t.Fatalf("empty goto should open about:blank: %v", p.gotos)
	}
}

func TestPreviewVNCOpenCtxBindsRequestContext(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	openCtx, openCancel := context.WithTimeout(parent, 90*time.Second)
	defer openCancel()

	cancel()
	select {
	case <-openCtx.Done():
		if !errors.Is(openCtx.Err(), context.Canceled) {
			t.Fatalf("openCtx.Err() = %v, want context.Canceled", openCtx.Err())
		}
	default:
		t.Fatal("openCtx should be canceled when request context is canceled")
	}
}
