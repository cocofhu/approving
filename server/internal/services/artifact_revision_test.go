package services

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestArtifactSaveBumpsRevision(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.Run{ID: "r1", WorkflowID: "wf", WorkflowName: "wf"})
	s := NewArtifactService(db)

	id1, err := s.Save("r1", "n1", "page.html", "html", "<p>1</p>")
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := s.GetRecord("r1", "page.html")
	if !ok || rec.Revision != 1 || rec.ID != id1 {
		t.Fatalf("first write revision=%d id=%s", rec.Revision, rec.ID)
	}

	id2, err := s.Save("r1", "n1", "page.html", "html", "<p>2</p>")
	if err != nil {
		t.Fatal(err)
	}
	rec, ok = s.GetRecord("r1", "page.html")
	if !ok || rec.Revision != 2 || rec.ID != id2 || rec.Content != "<p>2</p>" {
		t.Fatalf("second write revision=%d id=%s content=%q", rec.Revision, rec.ID, rec.Content)
	}
}
