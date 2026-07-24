package models

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestParsePriorityLabel(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", PriorityNormal, false},
		{"normal", PriorityNormal, false},
		{"HIGH", PriorityHigh, false},
		{" low ", PriorityLow, false},
		{"urgent", 0, true},
		{"medium", 0, true},
	}
	for _, tc := range cases {
		got, err := ParsePriorityLabel(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParsePriorityLabel(%q): want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsePriorityLabel(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParsePriorityLabel(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestPriorityLabel(t *testing.T) {
	if PriorityLabel(PriorityHigh) != PriorityLabelHigh {
		t.Fatal("high")
	}
	if PriorityLabel(PriorityLow) != PriorityLabelLow {
		t.Fatal("low")
	}
	if PriorityLabel(PriorityNormal) != PriorityLabelNormal {
		t.Fatal("normal")
	}
	if PriorityLabel(0) != PriorityLabelNormal {
		t.Fatal("zero defaults to normal")
	}
	if PriorityLabel(99) != PriorityLabelNormal {
		t.Fatal("unknown defaults to normal")
	}
}

func TestValidPriorityInt(t *testing.T) {
	if !ValidPriorityInt(PriorityLow) || !ValidPriorityInt(PriorityNormal) || !ValidPriorityInt(PriorityHigh) {
		t.Fatal("defined levels should be valid")
	}
	if ValidPriorityInt(0) || ValidPriorityInt(4) || ValidPriorityInt(-1) {
		t.Fatal("out-of-range values should be invalid")
	}
}

func TestRunBeforeCreateDefaultPriority(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:priority_before_create?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := &Run{ID: "pri-zero", Status: "queued"}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var stored Run
	if err := db.First(&stored, "id = ?", "pri-zero").Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if stored.Priority != PriorityNormal {
		t.Fatalf("priority = %d, want %d", stored.Priority, PriorityNormal)
	}
	// Explicit high must be preserved.
	hi := &Run{ID: "pri-hi", Status: "queued", Priority: PriorityHigh}
	if err := db.Create(hi).Error; err != nil {
		t.Fatalf("create high: %v", err)
	}
	var storedHi Run
	if err := db.First(&storedHi, "id = ?", "pri-hi").Error; err != nil {
		t.Fatalf("load high: %v", err)
	}
	if storedHi.Priority != PriorityHigh {
		t.Fatalf("priority = %d, want high", storedHi.Priority)
	}
}
