package engine

import (
	"math"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestMaxConcurrentAndSema(t *testing.T) {
	e, _ := setupEngine(t)
	if e.MaxConcurrent() <= 0 {
		t.Fatal("MaxConcurrent")
	}
	e.SetMaxConcurrent(3)
	if e.MaxConcurrent() != 3 {
		t.Fatalf("after set: %d", e.MaxConcurrent())
	}
	e.SetAutoRetryMax(-1)
	if e.AutoRetryMax() != 0 {
		t.Fatal("auto retry clamp")
	}
	e.SetAutoRetryMax(2)
	if e.AutoRetryMax() != 2 {
		t.Fatal("auto retry set")
	}

	s := newSema(1)
	if !s.TryAcquire() {
		t.Fatal("try")
	}
	if s.TryAcquire() {
		t.Fatal("full")
	}
	s.Release()
	s.Acquire()
	s.Release()
	if s.Limit() != 1 {
		t.Fatalf("limit=%d", s.Limit())
	}
	s.SetLimit(2)
	if s.Limit() != 2 {
		t.Fatal("set limit")
	}
}

// TestSetAutoRetryMaxInt64Bounds covers CodeQL #7: negative→0, MaxInt32 and
// MaxInt32+1 are stored without MaxInt32 product clamping (atomic.Int64).
func TestSetAutoRetryMaxInt64Bounds(t *testing.T) {
	e := &Engine{}
	e.SetAutoRetryMax(-5)
	if e.AutoRetryMax() != 0 {
		t.Fatalf("negative: got %d", e.AutoRetryMax())
	}
	e.SetAutoRetryMax(math.MaxInt32)
	if e.AutoRetryMax() != math.MaxInt32 {
		t.Fatalf("MaxInt32: got %d", e.AutoRetryMax())
	}
	e.SetAutoRetryMax(math.MaxInt32 + 1)
	if e.AutoRetryMax() != math.MaxInt32+1 {
		t.Fatalf("MaxInt32+1: got %d want %d", e.AutoRetryMax(), math.MaxInt32+1)
	}
	e.SetAutoRetryMax(math.MaxInt)
	if e.AutoRetryMax() != math.MaxInt {
		t.Fatalf("MaxInt: got %d", e.AutoRetryMax())
	}
}

func TestDefaultAppPreviewHelpers(t *testing.T) {
	acts := defaultAppPreviewActions(map[string]any{})
	if len(acts) != 2 {
		t.Fatalf("default actions: %+v", acts)
	}
	acts = defaultAppPreviewActions(map[string]any{
		"actions": []any{map[string]any{"id": "ok", "label": "OK"}},
	})
	if len(acts) != 1 || acts[0].ID != "ok" {
		t.Fatalf("custom actions: %+v", acts)
	}
	form := defaultAppPreviewForm(map[string]any{})
	if form == nil {
		form = []models.GateField{}
	}
	_ = form
	form = defaultAppPreviewForm(map[string]any{
		"form": []any{map[string]any{"id": "c", "label": "Comment", "type": "textarea"}},
	})
	if len(form) != 1 {
		t.Fatalf("form: %+v", form)
	}
}
