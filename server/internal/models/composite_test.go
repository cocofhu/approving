package models

import (
	"strings"
	"testing"
)

func TestVarDisplayText(t *testing.T) {
	if got := VarDisplayText("hello"); got != "hello" {
		t.Errorf("string = %q", got)
	}
	comp := map[string]any{
		"text":   "feature text",
		"images": []any{map[string]any{"data": "abc", "mimeType": "image/png"}},
	}
	if got := VarDisplayText(comp); got != "feature text" {
		t.Errorf("composite text = %q", got)
	}
	if got := VarDisplayText(comp); strings.Contains(got, "map[") {
		t.Errorf("should not fmt.Sprint map: %q", got)
	}
}

func TestIsBlankVar(t *testing.T) {
	if !IsBlankVar(nil) || !IsBlankVar("  ") {
		t.Error("nil/blank string should be blank")
	}
	if IsBlankVar("x") || IsBlankVar(3) || IsBlankVar(true) {
		t.Error("non-blank scalar")
	}
	onlyImages := map[string]any{
		"text":   "",
		"images": []any{map[string]any{"data": "abc", "mimeType": "image/png"}},
	}
	if IsBlankVar(onlyImages) {
		t.Error("images-only composite should not be blank")
	}
	emptyComposite := map[string]any{"text": "", "images": []any{}}
	if !IsBlankVar(emptyComposite) {
		t.Error("empty composite should be blank")
	}
}

func TestExtractImages(t *testing.T) {
	comp := map[string]any{
		"text": "t",
		"images": []any{
			map[string]any{"data": "a", "mimeType": "image/png"},
			map[string]any{"data": "b", "mimeType": "image/jpeg"},
		},
	}
	imgs := ExtractImages(comp)
	if len(imgs) != 2 || imgs[0].Data != "a" || imgs[1].MimeType != "image/jpeg" {
		t.Fatalf("ExtractImages = %+v", imgs)
	}
	if ExtractImages("plain") != nil {
		t.Error("string should yield nil images")
	}
}
