package services

import (
	"context"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestACPAndIDEUpstream(t *testing.T) {
	db := newTestDB(t)
	ds := &dockerState{}
	s := newSandboxService(t, db, ds)
	ctx := context.Background()

	if _, err := s.ACPUpstream(ctx, 999); err == nil {
		t.Fatal("missing row ACP")
	}
	if _, err := s.IDEUpstream(ctx, 999); err == nil {
		t.Fatal("missing row IDE")
	}

	row := models.Sandbox{
		Name: "up-sb", Profile: "agentA", Purpose: "test", Status: "running",
		Host: "10.0.0.8", ACPPort: 34567, CodeServerPort: 8080,
		Token: "tok",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	ds.setStatus(row.Name, "running")
	ds.fg.SetEndpoints(row.Name, map[string]string{
		"session": "10.0.0.8:34567",
		"ide":     "10.0.0.8:8080",
		"ssh":     "10.0.0.8:2222",
	})

	addr, err := s.ACPUpstream(ctx, row.ID)
	if err != nil || addr == "" {
		t.Fatalf("ACPUpstream: %q %v", addr, err)
	}
	ide, err := s.IDEUpstream(ctx, row.ID)
	if err != nil || ide == "" {
		t.Fatalf("IDEUpstream: %q %v", ide, err)
	}

	// no code-server port
	row2 := models.Sandbox{Name: "up-sb2", Profile: "agentA", Purpose: "test", Status: "running", ACPPort: 1}
	db.Create(&row2)
	if _, err := s.IDEUpstream(ctx, row2.ID); err == nil {
		t.Fatal("expected no code-server error")
	}

	// Fallback to persisted Host when gateway has no ide endpoint
	row3 := models.Sandbox{
		Name: "up-sb3", Profile: "agentA", Purpose: "test", Status: "running",
		Host: "192.168.1.9", CodeServerPort: 9090,
	}
	db.Create(&row3)
	ds.setStatus(row3.Name, "running")
	ds.fg.SetEndpoints(row3.Name, map[string]string{
		"session": "192.168.1.9:1",
		"ssh":     "192.168.1.9:22",
	})
	ide2, err := s.IDEUpstream(ctx, row3.ID)
	if err != nil || ide2 == "" {
		t.Fatalf("IDE fallback: %q %v", ide2, err)
	}
}

func TestEventLogPageMissingSandbox(t *testing.T) {
	db := newTestDB(t)
	s := newSandboxService(t, db, &dockerState{})
	if _, err := s.EventLogPage(context.Background(), 1, "", 10); err == nil {
		t.Fatal("expected error")
	}
	if s.Manager() == nil {
		t.Fatal("Manager")
	}
	_ = time.Second
}
