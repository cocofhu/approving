package services

import (
	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
)

// DashboardService computes summary statistics for the dashboard.
type DashboardService struct{ db *gorm.DB }

// NewDashboardService builds the service.
func NewDashboardService(db *gorm.DB) *DashboardService { return &DashboardService{db: db} }

// Stats is the dashboard summary payload.
type Stats struct {
	Running      int64 `json:"running"`
	WaitingHuman int64 `json:"waitingHuman"`
	Failed       int64 `json:"failed"`
	Completed    int64 `json:"completed"`
	Workflows    int64 `json:"workflows"`
	Artifacts    int64 `json:"artifacts"`
}

// Compute returns the current stats.
func (s *DashboardService) Compute() Stats {
	var st Stats
	count := func(status string) int64 {
		var n int64
		s.db.Model(&models.Run{}).Where("status = ?", status).Count(&n)
		return n
	}
	st.Running = count("running")
	st.WaitingHuman = count("waiting_human")
	st.Failed = count("failed")
	st.Completed = count("completed")
	s.db.Model(&models.WorkflowDef{}).Count(&st.Workflows)
	s.db.Model(&models.Artifact{}).Count(&st.Artifacts)
	return st
}
