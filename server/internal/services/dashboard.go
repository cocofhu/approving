package services

import (
	"sync"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
)

// DashboardService computes summary statistics for the dashboard
// and the lightweight platform-status payload for AppTopbar.
type DashboardService struct {
	db       *gorm.DB
	projects *ProjectService

	// Process-local cache for 5m token buckets (platform-status hot path).
	statusMu       sync.Mutex
	statusCache    map[string]platformStatusCacheEntry
	statusInflight map[string]*platformStatusCall
	// loadPlatformUsageHook is optional; tests inject to observe singleflight.
	loadPlatformUsageHook func()
}

// NewDashboardService builds the service. projects may be nil (Token fields stay null).
func NewDashboardService(db *gorm.DB, projects *ProjectService) *DashboardService {
	return &DashboardService{db: db, projects: projects}
}

// Stats is the dashboard summary payload.
// Token fields use pointers so JSON null means "never reported"; 0 is a real zero.
type Stats struct {
	Running        int64  `json:"running"`
	WaitingHuman   int64  `json:"waitingHuman"`
	Failed         int64  `json:"failed"`
	Completed      int64  `json:"completed"`
	Workflows      int64  `json:"workflows"`
	Artifacts      int64  `json:"artifacts"`
	TotalTokens    *int64 `json:"totalTokens"`
	WorkflowTokens *int64 `json:"workflowTokens"`
	PMTokens       *int64 `json:"pmTokens"`
}

// Compute returns the current stats (Run status counts + platform Token totals).
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

	if s.projects != nil {
		bd := s.projects.PlatformTokenBreakdown()
		st.TotalTokens = bd.Total
		st.WorkflowTokens = bd.Workflow
		st.PMTokens = bd.PM
	}
	return st
}
