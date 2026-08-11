// Package handlers implements the REST + WS API binding services and the
// FSM engine to HTTP. Response shapes mirror the frontend types so the Vue
// app maps responses to its view models with minimal transformation.
package handlers

import (
	"sync"

	"github.com/cocofhu/approving/internal/auth"
	"github.com/cocofhu/approving/internal/blob"
	"github.com/cocofhu/approving/internal/browser"
	"github.com/cocofhu/approving/internal/contextmcp"
	"github.com/cocofhu/approving/internal/engine"
	"github.com/cocofhu/approving/internal/gateshare"
	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/memorymcp"
	"github.com/cocofhu/approving/internal/pmmcp"
	"github.com/cocofhu/approving/internal/sandbox"
	"github.com/cocofhu/approving/internal/schedulermcp"
	"github.com/cocofhu/approving/internal/services"
	"github.com/cocofhu/approving/internal/shutdown"
)

// Handlers bundles dependencies for route handlers.
type Handlers struct {
	WF               *services.WorkflowService
	Projects         *services.ProjectService
	Runs             *services.RunService
	Arts             *services.ArtifactService
	APIKeys          *services.APIKeyService
	Skill            *services.SkillService
	Org              *services.OrgService
	Dash             *services.DashboardService
	Sbx              *services.SandboxService
	Eng              *engine.Engine
	MCP              *mcp.Host
	Pm               *services.PmService
	PmProgress       *services.PmProgress
	PmTurns          *services.PmTurnRunner
	PMMCP            *pmmcp.Host
	MemoryMCP        *memorymcp.Host
	ContextMCP       *contextmcp.Host
	SchedulerMCP     *schedulermcp.Host
	Preview          *services.PreviewService
	Issues           *services.IssueService
	Settings         *services.SettingsService
	Shutdown         *shutdown.Coordinator
	Auth             *auth.Service
	PlatformRules    *services.PlatformRuleService
	Channels         *services.ChannelConfigService
	Browser          *browser.Service
	Audit            *services.ProjectAuditService
	Onboarding       *services.OnboardingService
	GateShare        *gateshare.Service
	GateShareNonces  *gateshare.NonceStore
	GateShareLimiter *gateshare.IPLimiter
	// PublicAdvertise is the browser-facing origin for share URLs.
	// Public CSRF compares Origin/Referer to this request's Host (never client
	// X-Forwarded-Host; advertise host is not used for CSRF).
	PublicAdvertise string
	Team            *services.TeamService
	// CanViewProjectAudit optionally overrides the default audit ACL
	// (is_admin OR authenticated user who can UpdateProject). Tests use this
	// to simulate a read-only member denial while production keeps the hook nil.
	CanViewProjectAudit func(username, projectID string) bool
	// InjectBundles serves ConfigHome .tgz for gateway SANDBOX_INJECT (no session auth).
	InjectBundles *sandbox.BundleStore
	// Blobs serves externalized attachment bytes (GET /api/blobs/:id).
	Blobs          blob.Store
	doctorMu       sync.Mutex
	doctorSessions map[string]doctorArtifactSession
}
