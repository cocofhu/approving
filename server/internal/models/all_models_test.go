package models

import "testing"

func TestAllModelsAndComposite(t *testing.T) {
	ms := AllModels()
	if len(ms) < 10 {
		t.Fatalf("AllModels=%d", len(ms))
	}
	foundShare := false
	foundNonce := false
	foundTicket := false
	for _, m := range ms {
		if _, ok := m.(*GateShareLink); ok {
			foundShare = true
		}
		if _, ok := m.(*GateShareNonce); ok {
			foundNonce = true
		}
		if _, ok := m.(*GateSharePreviewTicket); ok {
			foundTicket = true
		}
	}
	if !foundShare {
		t.Fatal("AllModels missing GateShareLink")
	}
	if !foundNonce {
		t.Fatal("AllModels missing GateShareNonce")
	}
	if !foundTicket {
		t.Fatal("AllModels missing GateSharePreviewTicket")
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
