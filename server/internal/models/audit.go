package models

import "time"

// ProjectAuditEvent is an append-only, project-scoped operation audit record.
// Managers must not Update/Delete via public API; only List/Export are exposed.
type ProjectAuditEvent struct {
	ID             string         `gorm:"primaryKey" json:"id"`
	ProjectID      string         `gorm:"index:idx_audit_proj_occurred,priority:1;index" json:"projectId"`
	OccurredAt     time.Time      `gorm:"index:idx_audit_proj_occurred,priority:2;index" json:"occurredAt"`
	Actor          string         `gorm:"index" json:"actor"`
	Unattributable bool           `json:"unattributable"`
	Action         string         `gorm:"index" json:"action"`
	ResourceType   string         `gorm:"index" json:"resourceType"`
	ResourceID     string         `gorm:"index" json:"resourceId"`
	Outcome        string         `json:"outcome"` // ok | fail
	Summary        string         `json:"summary"`
	Payload        map[string]any `gorm:"serializer:json" json:"payload"`
	CreatedAt      time.Time      `json:"createdAt"`
}

// Audit action namespaces (aligned with Demo filters).
const (
	AuditActionProjectConfig   = "project.config"
	AuditActionWorkflowCreate  = "workflow.create"
	AuditActionWorkflowUpdate  = "workflow.update"
	AuditActionWorkflowDelete  = "workflow.delete"
	AuditActionWorkflowPublish = "workflow.publish"
	AuditActionGateDecide      = "gate.decide"
	AuditActionRunStart        = "run.start"
	AuditActionRunCancel       = "run.cancel"
	AuditActionRunCompleted    = "run.completed"
	AuditActionRunFailed       = "run.failed"
	AuditActionRunCancelled    = "run.cancelled"
	AuditActionMCPCall         = "mcp.call"
	AuditActionAuditExport     = "audit.export"
	AuditActionChannel         = "channel.config"
	AuditActionCron            = "cron.config"
)

// Audit outcome values.
const (
	AuditOutcomeOK   = "ok"
	AuditOutcomeFail = "fail"
)
