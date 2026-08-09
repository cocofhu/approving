package models

import "testing"

func TestAllModelsAndComposite(t *testing.T) {
	ms := AllModels()
	if len(ms) < 10 {
		t.Fatalf("AllModels=%d", len(ms))
	}
	foundShare := false
	for _, m := range ms {
		if _, ok := m.(*GateShareLink); ok {
			foundShare = true
			break
		}
	}
	if !foundShare {
		t.Fatal("AllModels missing GateShareLink")
	}
	if IsCompositeText("x") || !IsCompositeText(map[string]any{"text": "hi"}) {
		t.Fatal("IsCompositeText")
	}
	if !IsCompositeText(map[string]any{"images": []any{}}) {
		t.Fatal("images only")
	}
	if VarDisplayText(nil) != "" || VarDisplayText("a") != "a" {
		t.Fatal("VarDisplayText")
	}
	if VarDisplayText(map[string]any{"text": "t", "images": []any{}}) != "t" {
		t.Fatal("composite display")
	}
}
