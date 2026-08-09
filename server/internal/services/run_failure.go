package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cocofhu/approving/internal/models"
)

// RunErrorArtifactName is the unified failure product written when a run ends
// as failed. Infrastructure failures must not synthesize node_complete.json.
const RunErrorArtifactName = "run_error.json"

// Human-readable fallbacks for early-exit paths that never wrote StateRun.error.
const (
	DefaultRunFailureReason = "运行失败，未记录具体错误原因"
	NoSandboxLogMarker      = "无可用沙箱日志"
)

// Log summary caps for run_error.json (tail of archived sandbox logs).
const (
	maxLogSummaryLines = 40
	maxLogSummaryBytes = 8 * 1024
)

// RunFailureInfo is the single aggregated failure view distributed to Run detail
// DTO/API, run_error.json, and PM/MCP.
type RunFailureInfo struct {
	Reason          string `json:"reason"`
	FailedNode      string `json:"failedNode,omitempty"`
	LogSummaryOrRef string `json:"logSummaryOrRef,omitempty"`
	NoSandboxLog    bool   `json:"noSandboxLog,omitempty"`
}

// AggregateRunFailure builds RunFailureInfo from the latest non-empty failed
// StateRun.error, vars.last_error, and archived sandbox logs. Reason is never
// empty: early-exit / panic paths without StateRun.error fall back to a
// human-readable default. Call after finalizeActiveStateRuns when possible so
// in-flight nodes that were force-failed contribute their error text.
func (s *RunService) AggregateRunFailure(runID string) RunFailureInfo {
	info := RunFailureInfo{}
	if runID == "" || s == nil || s.db == nil {
		info.Reason = DefaultRunFailureReason
		info.NoSandboxLog = true
		return info
	}

	var sr models.StateRun
	if err := s.db.Where("run_id = ? AND status = ? AND error != ''", runID, "failed").
		Order("id desc").First(&sr).Error; err == nil {
		info.Reason = strings.TrimSpace(sr.Error)
		info.FailedNode = sr.NodeID
	} else if err := s.db.Where("run_id = ? AND status = ?", runID, "failed").
		Order("id desc").First(&sr).Error; err == nil {
		info.FailedNode = sr.NodeID
		info.Reason = strings.TrimSpace(sr.Error)
	}

	if info.Reason == "" {
		if le := s.runVarString(runID, "last_error"); le != "" {
			info.Reason = le
		}
	}
	if info.Reason == "" {
		info.Reason = DefaultRunFailureReason
	}

	logContent := s.sandboxLogContent(runID, info.FailedNode)
	if logContent != "" {
		info.LogSummaryOrRef = TruncateLogSummary(logContent)
		info.NoSandboxLog = false
	} else {
		info.NoSandboxLog = true
	}
	return info
}

// DisplayReason returns a one-line human-readable failure reason suitable for
// UI banners and MCP short fields (includes failed node and no-log marker).
func (info RunFailureInfo) DisplayReason() string {
	reason := strings.TrimSpace(info.Reason)
	if reason == "" {
		reason = DefaultRunFailureReason
	}
	if info.FailedNode != "" && !strings.Contains(reason, info.FailedNode) {
		reason = fmt.Sprintf("%s（节点 %s）", reason, info.FailedNode)
	}
	if info.NoSandboxLog && !strings.Contains(reason, NoSandboxLogMarker) {
		reason = reason + " · " + NoSandboxLogMarker
	}
	return reason
}

// ShortDisplayReason truncates DisplayReason for list rows.
func (info RunFailureInfo) ShortDisplayReason(max int) string {
	return truncateStr(info.DisplayReason(), max)
}

// MarshalRunErrorJSON serializes RunFailureInfo for ArtifactService.Save.
// The reason field uses DisplayReason so empty-product consumers see the same
// human text as the Web banner.
func MarshalRunErrorJSON(info RunFailureInfo) (string, error) {
	payload := map[string]any{
		"reason":       info.DisplayReason(),
		"noSandboxLog": info.NoSandboxLog,
	}
	if info.FailedNode != "" {
		payload["failedNode"] = info.FailedNode
	}
	if info.LogSummaryOrRef != "" {
		payload["logSummaryOrRef"] = info.LogSummaryOrRef
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// TruncateLogSummary keeps the last N lines within a byte budget.
func TruncateLogSummary(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > maxLogSummaryLines {
		lines = lines[len(lines)-maxLogSummaryLines:]
	}
	out := strings.Join(lines, "\n")
	if len(out) > maxLogSummaryBytes {
		out = out[len(out)-maxLogSummaryBytes:]
		if i := strings.IndexByte(out, '\n'); i >= 0 && i+1 < len(out) {
			out = out[i+1:]
		}
	}
	return out
}

func (s *RunService) runVarString(runID, name string) string {
	var rv models.RunVariable
	if err := s.db.Where("run_id = ? AND name = ?", runID, name).First(&rv).Error; err != nil {
		return ""
	}
	switch v := rv.Value.(type) {
	case string:
		return strings.TrimSpace(v)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func (s *RunService) sandboxLogContent(runID, nodeID string) string {
	var rec models.SandboxLog
	q := s.db.Where("run_id = ?", runID)
	if nodeID != "" {
		q = q.Where("node_id = ?", nodeID)
	}
	if err := q.Order("updated_at desc").First(&rec).Error; err == nil && strings.TrimSpace(rec.Content) != "" {
		return rec.Content
	}
	// Fall back to any archived log for the run (node id may be empty on early fail).
	if nodeID != "" {
		if err := s.db.Where("run_id = ?", runID).Order("updated_at desc").First(&rec).Error; err == nil {
			return rec.Content
		}
	}
	return ""
}
