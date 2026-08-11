package models

import "time"

// RequirementDraftStatus values for project-scoped requirement drafts.
const (
	RequirementDraftStatusOpen = "open"
	RequirementDraftStatusDone = "done"
)

// RequirementDraft is a project-scoped requirement memo (「需求草稿」).
// Independent of Run / Memory / PreviewIssue / Board / plan todos.
// Status open|done; done means archived, not deleted.
type RequirementDraft struct {
	ID           string `gorm:"primaryKey" json:"id"`
	ProjectID    string `gorm:"index:idx_req_draft_proj_status_updated,priority:1;index" json:"projectId"`
	Title        string `json:"title"`
	BodyMarkdown string `gorm:"type:text" json:"bodyMarkdown"`
	// Status is open (未完成) or done (已完成/归档).
	Status    string    `gorm:"index:idx_req_draft_proj_status_updated,priority:2" json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `gorm:"index:idx_req_draft_proj_status_updated,priority:3" json:"updatedAt"`
}
