package services

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"
)

func TestAuditMaskAndRecord(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectAuditService(db)

	masked := MaskAuditPayload(map[string]any{
		"API_TOKEN": "super-secret",
		"region":    "cn-east",
		"nested":    map[string]any{"password": "p@ss", "ok": true},
	})
	if masked["API_TOKEN"] != SecretMask {
		t.Fatalf("API_TOKEN not masked: %#v", masked["API_TOKEN"])
	}
	if masked["region"] != "cn-east" {
		t.Fatalf("region should stay: %#v", masked["region"])
	}
	nested := masked["nested"].(map[string]any)
	if nested["password"] != SecretMask || nested["ok"] != true {
		t.Fatalf("nested mask failed: %#v", nested)
	}

	s.Record(AuditRecord{
		ProjectID:    "proj-a",
		Actor:        ActorFromUsername("alice"),
		Action:       models.AuditActionProjectConfig,
		ResourceType: "project",
		ResourceID:   "proj-a",
		Outcome:      models.AuditOutcomeOK,
		Summary:      "update vars",
		Payload: map[string]any{
			"token": "should-mask",
			"ok":    "visible",
		},
	})
	s.Record(AuditRecord{
		ProjectID:    "proj-a",
		Actor:        SystemActor(),
		Action:       models.AuditActionRunStart,
		ResourceType: "run",
		ResourceID:   "run-1",
		Outcome:      models.AuditOutcomeOK,
		Summary:      "cron start",
		Payload:      map[string]any{"trigger": "cron"},
	})
	s.Record(AuditRecord{
		ProjectID:    "proj-b",
		Actor:        ActorFromUsername("bob"),
		Action:       models.AuditActionProjectConfig,
		ResourceType: "project",
		ResourceID:   "proj-b",
		Outcome:      models.AuditOutcomeOK,
		Summary:      "other project",
	})

	items, total, err := s.ListPage(AuditListFilter{ProjectID: "proj-a", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("proj-a isolation: total=%d len=%d", total, len(items))
	}
	for _, ev := range items {
		if ev.ProjectID != "proj-a" {
			t.Fatalf("leaked project %s", ev.ProjectID)
		}
		raw, _ := json.Marshal(ev.Payload)
		if strings.Contains(string(raw), "should-mask") || strings.Contains(string(raw), "super-secret") {
			t.Fatalf("plaintext secret in payload: %s", raw)
		}
		if ev.Action == models.AuditActionRunStart && !ev.Unattributable {
			t.Fatalf("system actor should be unattributable: %#v", ev)
		}
		if ev.Action == models.AuditActionProjectConfig && (ev.Actor != "alice" || ev.Unattributable) {
			t.Fatalf("alice attribution: %#v", ev)
		}
	}

	from := time.Now().Add(-1 * time.Hour)
	_, total24, err := s.ListPage(AuditListFilter{ProjectID: "proj-a", From: &from, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total24 != 2 {
		t.Fatalf("window filter: total=%d", total24)
	}

	sys := ActorFromUsername("")
	if !sys.Unattributable || sys.Username != "system" {
		t.Fatalf("empty actor: %#v", sys)
	}
}

func TestFormatAuditText(t *testing.T) {
	txt := FormatAuditText([]models.ProjectAuditEvent{{
		Actor: "alice", Action: "project.config", ResourceType: "project", ResourceID: "p1",
		Outcome: "ok", Summary: "upd", OccurredAt: time.Now(),
		Payload: map[string]any{"token": SecretMask},
	}})
	if !strings.Contains(txt, "alice") || !strings.Contains(txt, "project.config") {
		t.Fatalf("text export missing fields: %s", txt)
	}
}
