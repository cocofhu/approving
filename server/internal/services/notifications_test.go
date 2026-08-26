package services

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNotificationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "notif.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.NotificationRead{},
		&models.NotificationBaseline{},
		&models.Run{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedTerminalRun(t *testing.T, db *gorm.DB, id, status, title string, started time.Time, durSec int) {
	t.Helper()
	if err := db.Create(&models.Run{
		ID:           id,
		WorkflowID:   "wf",
		WorkflowName: "demo-wf",
		Title:        title,
		Status:       status,
		StartedAt:    started,
		DurationSec:  durSec,
		Progress:     100,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestNotificationListFirstAccessHistoryRead(t *testing.T) {
	db := setupNotificationDB(t)
	svc := NewNotificationService(db, nil)
	past := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	seedTerminalRun(t, db, "old-1", "completed", "历史完成", past, 60)
	seedTerminalRun(t, db, "old-2", "failed", "历史失败", past.Add(time.Hour), 30)

	items, err := svc.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%d", len(items))
	}
	for _, it := range items {
		if it.Unread {
			t.Fatalf("first GET must treat pool as history: %+v", it)
		}
		if !it.BeforeBaseline {
			t.Fatalf("expected beforeBaseline: %+v", it)
		}
	}

	again, err := svc.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	var b1, b2 models.NotificationBaseline
	if err := db.First(&b1, "username = ?", "alice").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&b2, "username = ?", "alice").Error; err != nil {
		t.Fatal(err)
	}
	if !b1.EnabledAt.Equal(b2.EnabledAt) {
		t.Fatalf("enabledAt mutated: %v vs %v", b1.EnabledAt, b2.EnabledAt)
	}
	if len(again) != 2 {
		t.Fatalf("again=%d", len(again))
	}
}

func TestNotificationMarkReadIdempotentAndIsolation(t *testing.T) {
	db := setupNotificationDB(t)
	svc := NewNotificationService(db, nil)
	start := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	seedTerminalRun(t, db, "a", "completed", "A", start, 1)
	seedTerminalRun(t, db, "b", "failed", "B", start.Add(time.Hour), 1)

	if err := db.Create(&models.NotificationBaseline{
		Username:  "alice",
		EnabledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}

	items, err := svc.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	unread := 0
	for _, it := range items {
		if it.Unread {
			unread++
		}
	}
	if unread != 2 {
		t.Fatalf("unread=%d items=%+v", unread, items)
	}

	if err := svc.MarkRead("alice", "a"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkRead("alice", "a"); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&models.NotificationRead{}).Where("username = ? AND run_id = ?", "alice", "a").Count(&n)
	if n != 1 {
		t.Fatalf("idempotent insert grew rows: %d", n)
	}

	alice, err := svc.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	var aUnread, bUnread bool
	for _, it := range alice {
		if it.RunID == "a" {
			aUnread = it.Unread
		}
		if it.RunID == "b" {
			bUnread = it.Unread
		}
	}
	if aUnread || !bUnread {
		t.Fatalf("alice after mark a: %+v", alice)
	}

	bob, err := svc.List("bob")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range bob {
		if it.Unread {
			t.Fatalf("bob first GET must be history-read, got %+v", it)
		}
	}
	var bobReads int64
	db.Model(&models.NotificationRead{}).Where("username = ?", "bob").Count(&bobReads)
	if bobReads != 0 {
		t.Fatalf("bob inherited alice reads: %d", bobReads)
	}
}

func TestNotificationMarkAllReadScansPool(t *testing.T) {
	db := setupNotificationDB(t)
	svc := NewNotificationService(db, nil)
	start := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	seedTerminalRun(t, db, "a", "completed", "A", start, 1)
	seedTerminalRun(t, db, "b", "failed", "B", start.Add(time.Minute), 1)
	if err := db.Create(&models.NotificationBaseline{
		Username:  "alice",
		EnabledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.MarkAllRead("alice"); err != nil {
		t.Fatal(err)
	}
	items, err := svc.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Unread {
			t.Fatalf("mark-all left unread: %+v", it)
		}
	}
	var n int64
	db.Model(&models.NotificationRead{}).Where("username = ?", "alice").Count(&n)
	if n != 2 {
		t.Fatalf("expected 2 read rows, got %d", n)
	}

	// Empty-pool mark-all must not delete existing rows.
	if err := db.Where("1 = 1").Delete(&models.Run{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkAllRead("alice"); err != nil {
		t.Fatal(err)
	}
	db.Model(&models.NotificationRead{}).Where("username = ?", "alice").Count(&n)
	if n != 2 {
		t.Fatalf("empty mark-all cleared rows: %d", n)
	}
}

func TestNotificationSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notif.db")
	db1, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db1.AutoMigrate(&models.NotificationRead{}, &models.NotificationBaseline{}, &models.Run{}); err != nil {
		t.Fatal(err)
	}
	svc1 := NewNotificationService(db1, nil)
	if err := svc1.MarkRead("alice", "persist-me"); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db1.DB()
	_ = sqlDB.Close()

	db2, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db2.AutoMigrate(&models.NotificationRead{}, &models.NotificationBaseline{}, &models.Run{}); err != nil {
		t.Fatal(err)
	}
	var n int64
	db2.Model(&models.NotificationRead{}).Where("username = ? AND run_id = ?", "alice", "persist-me").Count(&n)
	if n != 1 {
		t.Fatalf("after reopen rows=%d", n)
	}
}

func TestNotificationValidation(t *testing.T) {
	db := setupNotificationDB(t)
	svc := NewNotificationService(db, nil)
	if _, err := svc.List("  "); err == nil {
		t.Fatal("expected username required")
	}
	if err := svc.MarkRead("alice", "  "); err == nil {
		t.Fatal("expected runId required")
	}
}

func TestMapRunToNotification(t *testing.T) {
	start := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	ok, mapped := MapRunToNotification(models.Run{
		ID: "r1", Status: "completed", Title: "Run r1", WorkflowName: "demo-wf",
		StartedAt: start, DurationSec: 1,
	})
	if !mapped || ok.RunID != "r1" || ok.Status != "completed" {
		t.Fatalf("completed map=%+v mapped=%v", ok, mapped)
	}
	if _, mapped := MapRunToNotification(models.Run{ID: "r3", Status: "running"}); mapped {
		t.Fatal("running must not map")
	}
	blank, mapped := MapRunToNotification(models.Run{
		ID: "r1", Status: "completed", Title: "  ", WorkflowName: "demo-wf", StartedAt: start,
	})
	if !mapped || blank.Title != "demo-wf" {
		t.Fatalf("title fallback=%+v", blank)
	}
	idTitle, mapped := MapRunToNotification(models.Run{
		ID: "r2", Status: "completed", StartedAt: start,
	})
	if !mapped || idTitle.Title != "r2" {
		t.Fatalf("id fallback=%+v", idTitle)
	}

	if !IsNoisyNotificationTitle("运行中 3 / 暂停 1") {
		t.Fatal("expected noisy")
	}
	if IsNoisyNotificationTitle("产物这里根据Run 分页") {
		t.Fatal("did not expect noisy")
	}
	noisy, mapped := MapRunToNotification(models.Run{
		ID: "r1", Status: "completed", Title: "运行中 2 · 等待人工 1",
		WorkflowName: "自我迭代", StartedAt: start, DurationSec: 120,
	})
	if !mapped || !noisy.TitleNeutral || noisy.Title != "自我迭代 · completed" {
		t.Fatalf("neutral=%+v", noisy)
	}
	wantFinish := start.Add(120 * time.Second)
	gotFinish, err := time.Parse(time.RFC3339Nano, noisy.FinishedApprox)
	if err != nil || !gotFinish.Equal(wantFinish) {
		t.Fatalf("finishedApprox=%q want %v err=%v", noisy.FinishedApprox, wantFinish, err)
	}
}

func TestNotificationListPageBeyondFifty(t *testing.T) {
	db := setupNotificationDB(t)
	svc := NewNotificationService(db, nil)
	start := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 55; i++ {
		seedTerminalRun(t, db, fmt.Sprintf("run-%02d", i), "completed", fmt.Sprintf("N%d", i),
			start.Add(time.Duration(i)*time.Minute), 30)
	}
	if err := db.Create(&models.NotificationBaseline{
		Username:  "alice",
		EnabledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}

	res, err := svc.ListPage("alice", "all", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.AllCount != 55 || res.UnreadCount != 55 || res.ReadCount != 0 {
		t.Fatalf("counts=%d/%d/%d", res.AllCount, res.UnreadCount, res.ReadCount)
	}
	if res.Total != 55 || len(res.Items) != 20 {
		t.Fatalf("page1 total=%d items=%d", res.Total, len(res.Items))
	}

	page3, err := svc.ListPage("alice", "all", 3, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3.Items) != 15 {
		t.Fatalf("page3 items=%d", len(page3.Items))
	}
	// Oldest run should appear on the last page (finished desc).
	last := page3.Items[len(page3.Items)-1]
	if last.RunID != "run-00" {
		t.Fatalf("expected oldest run-00 on page3 tail, got %s", last.RunID)
	}
}

func TestNotificationListPageFilterAndCounts(t *testing.T) {
	db := setupNotificationDB(t)
	svc := NewNotificationService(db, nil)
	start := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	seedTerminalRun(t, db, "a", "completed", "A", start, 1)
	seedTerminalRun(t, db, "b", "failed", "B", start.Add(time.Minute), 1)
	seedTerminalRun(t, db, "c", "completed", "C", start.Add(2*time.Minute), 1)
	if err := db.Create(&models.NotificationBaseline{
		Username:  "alice",
		EnabledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkRead("alice", "a"); err != nil {
		t.Fatal(err)
	}

	unread, err := svc.ListPage("alice", "unread", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if unread.AllCount != 3 || unread.UnreadCount != 2 || unread.ReadCount != 1 {
		t.Fatalf("global counts=%d/%d/%d", unread.AllCount, unread.UnreadCount, unread.ReadCount)
	}
	if unread.Total != 2 || len(unread.Items) != 2 {
		t.Fatalf("unread filter total=%d items=%d", unread.Total, len(unread.Items))
	}
	for _, it := range unread.Items {
		if !it.Unread {
			t.Fatalf("unread filter leaked read item %+v", it)
		}
	}
}

func TestNotificationMarkAllReadBeyondPool(t *testing.T) {
	db := setupNotificationDB(t)
	svc := NewNotificationService(db, nil)
	start := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 60; i++ {
		seedTerminalRun(t, db, fmt.Sprintf("run-%02d", i), "completed", fmt.Sprintf("N%d", i),
			start.Add(time.Duration(i)*time.Minute), 30)
	}
	if err := db.Create(&models.NotificationBaseline{
		Username:  "alice",
		EnabledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.MarkAllRead("alice"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.ListPage("alice", "all", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.UnreadCount != 0 {
		t.Fatalf("unread after mark-all=%d", res.UnreadCount)
	}
	page3, err := svc.ListPage("alice", "all", 3, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range page3.Items {
		if it.Unread {
			t.Fatalf("page3 still unread %+v", it)
		}
	}
	var n int64
	db.Model(&models.NotificationRead{}).Where("username = ?", "alice").Count(&n)
	if n != 60 {
		t.Fatalf("read rows=%d want 60", n)
	}
}

func TestNotificationListSortsByFinishedDesc(t *testing.T) {
	db := setupNotificationDB(t)
	svc := NewNotificationService(db, nil)
	// a starts earlier but finishes later (1h); b finishes 1 min after its start.
	aStart := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	bStart := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)
	seedTerminalRun(t, db, "a", "completed", "A", aStart, 3600)
	seedTerminalRun(t, db, "b", "failed", "B", bStart, 60)
	if err := db.Create(&models.NotificationBaseline{
		Username:  "alice",
		EnabledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}
	items, err := svc.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].RunID != "b" || items[1].RunID != "a" {
		t.Fatalf("order=%+v", items)
	}
}
