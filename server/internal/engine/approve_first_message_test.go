package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
)

func waitFirstHumanTurn(t *testing.T, db *gorm.DB, runID, nodeID string) models.ReactMessage {
	t.Helper()
	deadline := time.Now().Add(waitPollTimeout)
	for time.Now().Before(deadline) {
		var conv models.ReactConversation
		if err := db.Where("run_id = ? AND node_id = ?", runID, nodeID).First(&conv).Error; err == nil {
			for _, m := range conv.Messages {
				if m.Role == "human" {
					return m
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("first message was never delivered into the approve session")
	return models.ReactMessage{}
}

// The launcher's opening message is delivered by the engine once the approve
// node parks — no client-side park polling / follow-up reply.
func TestApproveFirstMessageDeliveredOnPark(t *testing.T) {
	eng, db, _ := setupEngineGraphP(t, approveOnlyGraph())
	msg := &models.CompositeText{
		Text: "把登录做清楚",
		Images: []models.PromptImage{
			{Data: "QUJD", MimeType: "image/png", Name: "shot.png"},
		},
	}
	run, err := eng.StartRunWithFirstMessage("wf", nil, "test", "", nil, nil, "", msg)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Inline image bytes are externalized before the run row is written.
	var stored models.Run
	if err := db.First(&stored, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if stored.FirstMessage == nil || stored.FirstMessage.Text != "把登录做清楚" {
		t.Fatalf("first message not persisted: %+v", stored.FirstMessage)
	}
	if len(stored.FirstMessage.Images) != 1 {
		t.Fatalf("first message images = %d, want 1", len(stored.FirstMessage.Images))
	}
	if img := stored.FirstMessage.Images[0]; img.Data != "" || !strings.HasPrefix(img.Ref, "blob:") {
		t.Fatalf("image not externalized: %+v", img)
	}

	turn := waitFirstHumanTurn(t, db, run.ID, "predev")
	if turn.Text != "把登录做清楚" {
		t.Fatalf("delivered text = %q", turn.Text)
	}
	if len(turn.Images) != 1 {
		t.Fatalf("delivered images = %d, want 1", len(turn.Images))
	}
	waitRunStatus(t, db, run.ID, "waiting_human")

	if err := db.First(&stored, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if stored.FirstMessageDeliveredAt == nil {
		t.Fatal("delivery latch must be set after delivery")
	}

	// Re-firing must not append a second human turn (the latch is held).
	c, err := eng.loadCtx(run.ID)
	if err != nil {
		t.Fatalf("loadCtx: %v", err)
	}
	eng.fireApproveFirstMessage(c, c.graph.FindNode("predev"))
	time.Sleep(80 * time.Millisecond)
	var conv models.ReactConversation
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "predev").First(&conv).Error; err != nil {
		t.Fatalf("conv: %v", err)
	}
	humans := 0
	for _, m := range conv.Messages {
		if m.Role == "human" {
			humans++
		}
	}
	if humans != 1 {
		t.Fatalf("human turns = %d, want exactly 1", humans)
	}
}

// Without a first message the approve node parks with an empty transcript.
func TestApproveWithoutFirstMessageParksEmpty(t *testing.T) {
	eng, db, _ := setupEngineGraphP(t, approveOnlyGraph())
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "predev")
	waitRunStatus(t, db, run.ID, "waiting_human")
	var conv models.ReactConversation
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "predev").First(&conv).Error; err != nil {
		t.Fatalf("conv: %v", err)
	}
	if len(conv.Messages) != 0 {
		t.Fatalf("expected empty transcript, got %+v", conv.Messages)
	}
	var stored models.Run
	db.First(&stored, "id = ?", run.ID)
	if stored.FirstMessageDeliveredAt != nil {
		t.Fatal("latch must stay unset when there is nothing to deliver")
	}
}
