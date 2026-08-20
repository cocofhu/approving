package gateshare

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

// A booting approve node has no session to share, so no chip state is attached.
func TestAttachInboxStatusSkipsStarting(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "share.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	now := time.Now()
	db.Create(&models.Run{ID: "run-1", Status: "running", StartedAt: now})
	s := NewService(db, nil)

	items := []any{
		services.ClarifyInboxItem{
			Type: "clarify", Kind: "clarify", State: "starting",
			RunID: "run-1", NodeID: "ap", Iteration: 1,
		},
		services.ClarifyInboxItem{
			Type: "clarify", Kind: "clarify",
			RunID: "run-1", NodeID: "ap2", Iteration: 1,
		},
	}
	s.AttachInboxStatus(items)

	if got := items[0].(services.ClarifyInboxItem); got.ShareLink != nil {
		t.Fatalf("starting item must not carry share state: %+v", got.ShareLink)
	}
	if got := items[1].(services.ClarifyInboxItem); got.ShareLink == nil {
		t.Fatal("parked clarify item should carry share state")
	}
}
