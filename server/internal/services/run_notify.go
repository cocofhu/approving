package services

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// RunNotifyDeliverer pushes a formatted Run lifecycle message to the project's
// bound QQ target without requiring CronDeliver. Implementations must treat
// missing targets as a soft no-op (return nil or a sentinel that callers log).
type RunNotifyDeliverer interface {
	DeliverRunNotify(projectID, text string) error
}

// RunNotifyTrackingDeliverer is the richer egress: the caller hands over a
// token identifying the receipt, and the deliverer reports whether the message
// actually went out. Without it a push parked behind a busy conversation is
// indistinguishable from one that reached the user.
type RunNotifyTrackingDeliverer interface {
	DeliverRunNotifyTracked(projectID, text, trackID string) error
}

// ErrRunNotifyNoTarget is returned (and swallowed) when no QQ target is bound.
var ErrRunNotifyNoTarget = errors.New("no run-notify delivery target")

// ErrRunNotifyDeferred means the message is queued behind an in-flight user
// turn. Like ErrRunNotifyNoTarget this is a state rather than a fault: retrying
// here would only stack duplicates behind the same queue, so the receipt is
// parked and the egress reports back when the queue drains.
var ErrRunNotifyDeferred = errors.New("run notify deferred behind a busy conversation")

// RunNotifyEvent is the Engine → service payload for a lifecycle side-effect.
type RunNotifyEvent struct {
	ProjectID    string
	RunID        string
	WorkflowID   string
	WorkflowName string
	ProjectName  string
	NodeID       string
	NodeLabel    string
	Iteration    int
	Kind         string // waiting_human | failed
	DeepLinkBase string // PublicAdvertise; empty → relative /runs/{id}
}

// RunNotifyService resolves policy, claims the receipt key, then delivers.
type RunNotifyService struct {
	db       *gorm.DB
	deliver  RunNotifyDeliverer
	deepLink string
	// retryDelays is overridable so tests do not have to wait out the real
	// backoff.
	retryDelays []time.Duration
}

// NewRunNotifyService builds the service. deliver may be nil (always no-op send).
func NewRunNotifyService(db *gorm.DB, deliver RunNotifyDeliverer, publicAdvertise string) *RunNotifyService {
	return &RunNotifyService{
		db:       db,
		deliver:  deliver,
		deepLink: strings.TrimRight(strings.TrimSpace(publicAdvertise), "/"),
	}
}

// SetRetryDelays overrides the delivery backoff schedule (tests).
func (s *RunNotifyService) SetRetryDelays(delays []time.Duration) {
	if s != nil {
		s.retryDelays = delays
	}
}

// SetDeliverer hot-swaps the QQ egress (used after channel manager boots).
func (s *RunNotifyService) SetDeliverer(d RunNotifyDeliverer) {
	if s == nil {
		return
	}
	s.deliver = d
}

// AttemptDeliver runs the Demo attemptDeliver sequence:
// policy miss → no claim, no send; claim conflict → skip; no channel / send
// fail → still considered processed (receipt kept); never panics the caller.
func (s *RunNotifyService) AttemptDeliver(ev RunNotifyEvent) {
	if s == nil || s.db == nil {
		return
	}
	kind := strings.TrimSpace(ev.Kind)
	if kind != models.NotifyKindWaitingHuman && kind != models.NotifyKindFailed {
		return
	}
	if strings.TrimSpace(ev.RunID) == "" || strings.TrimSpace(ev.NodeID) == "" || ev.Iteration < 1 {
		// failed without node context (and malformed waiting_human) → skip, no claim
		log.Debug().Str("run_id", ev.RunID).Str("kind", kind).
			Msg("run-notify: skip — missing node context")
		return
	}

	project, workflow, ok := s.loadPolicy(ev)
	if !ok {
		return
	}
	events := ResolveNotifyEvents(project.NotifyPolicy, workflow.NotifyPolicy)
	if !NotifyEventAllowed(events, kind) {
		// Info, not debug: "the user was never told" is the exact symptom
		// operators report, and at debug this path is invisible in production.
		log.Info().Str("run_id", ev.RunID).Str("kind", kind).
			Strs("resolved", events).Msg("run-notify: policy miss")
		return
	}

	claimed, err := s.claimReceipt(ev.RunID, ev.NodeID, ev.Iteration, kind)
	if err != nil {
		log.Warn().Err(err).Str("run_id", ev.RunID).Str("kind", kind).
			Msg("run-notify: claim failed")
		return
	}
	if !claimed {
		log.Debug().Str("run_id", ev.RunID).Str("node_id", ev.NodeID).
			Int("iteration", ev.Iteration).Str("kind", kind).
			Msg("run-notify: duplicate key — skip")
		return
	}

	if ev.ProjectName == "" {
		ev.ProjectName = project.Name
	}
	if ev.WorkflowName == "" {
		ev.WorkflowName = workflow.Name
	}
	base := s.deepLinkBase(ev)
	if strings.TrimSpace(base) == "" {
		// QQ clients cannot open relative /runs/{id}; ops must set PublicAdvertise.
		log.Warn().
			Str("run_id", ev.RunID).
			Str("project", project.ID).
			Msg("run-notify: PublicAdvertise empty — deep link will be relative /runs/{id}")
	}
	text := RenderRunNotifyMessage(ev, base, templateForKind(project.NotifyPolicy, kind))
	if s.deliver == nil {
		log.Info().Str("run_id", ev.RunID).Str("kind", kind).
			Msg("run-notify: no deliverer — no-op after claim")
		return
	}
	s.deliverWithRetry(ev, project.ID, text, kind)
}

// runNotifyRetryDelays bound how long a failed delivery is retried. The receipt
// is claimed before the send, so a transport error used to consume the claim and
// lose the notification permanently. Retrying inside the claim closes that hole
// without weakening the once-only guarantee; when the attempts are spent the
// outcome is recorded on the receipt instead of vanishing into a log line.
var runNotifyRetryDelays = []time.Duration{time.Second, 3 * time.Second, 8 * time.Second}

func (s *RunNotifyService) deliverWithRetry(ev RunNotifyEvent, projectID, text, kind string) {
	delays := s.retryDelays
	if delays == nil {
		delays = runNotifyRetryDelays
	}
	track := receiptTrackID(ev.RunID, ev.NodeID, ev.Iteration, kind)
	var lastErr error
	for attempt := 0; ; attempt++ {
		err := s.deliverOnce(projectID, text, track)
		if err == nil {
			s.markReceipt(ev, kind, "delivered", "")
			return
		}
		// A project with no bound channel is a configuration state, not a
		// failure to retry against.
		if errors.Is(err, ErrRunNotifyNoTarget) {
			log.Info().Str("run_id", ev.RunID).Str("project", projectID).
				Msg("run-notify: no channel target — no-op after claim")
			s.markReceipt(ev, kind, "no_target", "")
			return
		}
		// The message is queued, not lost. Recording it as deferred keeps the
		// claim intact and lets the push sweeper settle it once the
		// conversation frees up.
		if errors.Is(err, ErrRunNotifyDeferred) {
			log.Info().Str("run_id", ev.RunID).Str("project", projectID).Str("kind", kind).
				Msg("run-notify: conversation busy — queued, awaiting flush")
			s.markReceipt(ev, kind, "deferred", "")
			return
		}
		lastErr = err
		if attempt >= len(delays) {
			break
		}
		log.Warn().Err(err).Str("run_id", ev.RunID).Str("project", projectID).
			Int("attempt", attempt+1).Msg("run-notify: send failed, retrying")
		time.Sleep(delays[attempt])
	}
	log.Error().Err(lastErr).Str("run_id", ev.RunID).Str("project", projectID).
		Msg("run-notify: send failed after retries")
	s.markReceipt(ev, kind, "failed", lastErr.Error())
}

func (s *RunNotifyService) deliverOnce(projectID, text, trackID string) error {
	if tracking, ok := s.deliver.(RunNotifyTrackingDeliverer); ok {
		return tracking.DeliverRunNotifyTracked(projectID, text, trackID)
	}
	return s.deliver.DeliverRunNotify(projectID, text)
}

// receiptTrackID packs the receipt key into the token handed to the egress, so
// a late delivery can be matched back to its row without keeping in-memory
// state that a restart would lose.
func receiptTrackID(runID, nodeID string, iteration int, kind string) string {
	return strings.Join([]string{runID, nodeID, strconv.Itoa(iteration), kind}, "|")
}

// SettlePushSent flips a deferred receipt to delivered once the egress reports
// the queued message finally went out. Rows in any other state are left alone:
// a receipt that already reads delivered, failed or no_target has a settled
// story that a late queue drain should not rewrite.
func (s *RunNotifyService) SettlePushSent(trackID string) {
	if s == nil || s.db == nil {
		return
	}
	parts := strings.Split(trackID, "|")
	if len(parts) != 4 {
		return
	}
	iteration, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	res := s.db.Model(&models.NotifyDeliveryReceipt{}).
		Where("run_id = ? AND node_id = ? AND iteration = ? AND kind = ? AND delivery_status = ?",
			parts[0], parts[1], iteration, parts[3], "deferred").
		Updates(map[string]any{"delivery_status": "delivered", "delivery_error": ""})
	if res.Error != nil {
		log.Warn().Err(res.Error).Str("run_id", parts[0]).
			Msg("run-notify: settling deferred receipt failed")
		return
	}
	if res.RowsAffected > 0 {
		log.Info().Str("run_id", parts[0]).Str("kind", parts[3]).
			Msg("run-notify: deferred notification delivered on flush")
	}
}

// markReceipt records the delivery outcome on the claimed receipt so a failed
// notification is visible in the data, not only in the logs.
func (s *RunNotifyService) markReceipt(ev RunNotifyEvent, kind, status, detail string) {
	if s.db == nil {
		return
	}
	if len(detail) > 500 {
		detail = detail[:500]
	}
	err := s.db.Model(&models.NotifyDeliveryReceipt{}).
		Where("run_id = ? AND node_id = ? AND iteration = ? AND kind = ?",
			ev.RunID, ev.NodeID, ev.Iteration, kind).
		Updates(map[string]any{"delivery_status": status, "delivery_error": detail}).Error
	if err != nil {
		log.Warn().Err(err).Str("run_id", ev.RunID).Msg("run-notify: recording delivery status failed")
	}
}

func (s *RunNotifyService) deepLinkBase(ev RunNotifyEvent) string {
	if b := strings.TrimRight(strings.TrimSpace(ev.DeepLinkBase), "/"); b != "" {
		return b
	}
	return s.deepLink
}

func (s *RunNotifyService) loadPolicy(ev RunNotifyEvent) (models.Project, models.WorkflowDef, bool) {
	var wf models.WorkflowDef
	wfID := strings.TrimSpace(ev.WorkflowID)
	if wfID == "" {
		var run models.Run
		if err := s.db.Select("workflow_id").First(&run, "id = ?", ev.RunID).Error; err != nil {
			log.Warn().Err(err).Str("run_id", ev.RunID).Msg("run-notify: load run failed")
			return models.Project{}, models.WorkflowDef{}, false
		}
		wfID = run.WorkflowID
	}
	if err := s.db.First(&wf, "id = ?", wfID).Error; err != nil {
		log.Warn().Err(err).Str("workflow", wfID).Msg("run-notify: load workflow failed")
		return models.Project{}, models.WorkflowDef{}, false
	}
	projectID := strings.TrimSpace(ev.ProjectID)
	if projectID == "" {
		projectID = wf.ProjectID
	}
	var project models.Project
	if err := s.db.First(&project, "id = ?", projectID).Error; err != nil {
		log.Warn().Err(err).Str("project", projectID).Msg("run-notify: load project failed")
		return models.Project{}, models.WorkflowDef{}, false
	}
	return project, wf, true
}

// claimReceipt inserts the unique receipt. Returns claimed=false on duplicate.
func (s *RunNotifyService) claimReceipt(runID, nodeID string, iteration int, kind string) (bool, error) {
	row := models.NotifyDeliveryReceipt{
		RunID:     runID,
		NodeID:    nodeID,
		Iteration: iteration,
		Kind:      kind,
		CreatedAt: time.Now(),
	}
	err := s.db.Create(&row).Error
	if err == nil {
		return true, nil
	}
	if isUniqueViolation(err) {
		return false, nil
	}
	return false, err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

// Run notify template placeholders (locked; UI chips + frontend preview share this set).
const (
	NotifyPlaceholderProject  = "{project}"
	NotifyPlaceholderWorkflow = "{workflow}"
	NotifyPlaceholderRunID    = "{run_id}"
	NotifyPlaceholderNode     = "{node}"
	NotifyPlaceholderLink     = "{link}"
	NotifyPlaceholderTitle    = "{title}"
)

func templateForKind(p models.ProjectNotifyPolicy, kind string) string {
	switch kind {
	case models.NotifyKindWaitingHuman:
		return p.WaitingHumanTemplate
	case models.NotifyKindFailed:
		return p.FailedTemplate
	default:
		return ""
	}
}

// RunNotifyTitle returns the event title used by {title} and the default formatter.
func RunNotifyTitle(kind string) string {
	if kind == models.NotifyKindWaitingHuman {
		return "等待人工处理"
	}
	return "运行失败"
}

// RunNotifyNodeDisplay returns Label if set, otherwise NodeID (may be empty).
func RunNotifyNodeDisplay(ev RunNotifyEvent) string {
	if n := strings.TrimSpace(ev.NodeLabel); n != "" {
		return n
	}
	return strings.TrimSpace(ev.NodeID)
}

// ReplaceRunNotifyPlaceholders performs literal six-key replacement.
// Unknown placeholders are left as-is; never panics.
func ReplaceRunNotifyPlaceholders(tmpl string, project, workflow, runID, node, link, title string) string {
	replacer := strings.NewReplacer(
		NotifyPlaceholderProject, project,
		NotifyPlaceholderWorkflow, workflow,
		NotifyPlaceholderRunID, runID,
		NotifyPlaceholderNode, node,
		NotifyPlaceholderLink, link,
		NotifyPlaceholderTitle, title,
	)
	return replacer.Replace(tmpl)
}

// RenderRunNotifyMessage uses a custom template when trim-non-empty; otherwise
// falls back to FormatRunNotifyMessage (byte-compatible with legacy hardcode).
func RenderRunNotifyMessage(ev RunNotifyEvent, base, template string) string {
	if strings.TrimSpace(template) == "" {
		return FormatRunNotifyMessage(ev, base)
	}
	projectName := strings.TrimSpace(ev.ProjectName)
	if projectName == "" {
		projectName = "—"
	}
	wfName := strings.TrimSpace(ev.WorkflowName)
	if wfName == "" {
		wfName = "—"
	}
	node := RunNotifyNodeDisplay(ev) // custom path: empty stays empty (no line delete)
	link := runDeepLink(base, ev.RunID)
	title := RunNotifyTitle(ev.Kind)
	return ReplaceRunNotifyPlaceholders(template, projectName, wfName, ev.RunID, node, link, title)
}

// FormatRunNotifyMessage builds the QQ body (independent of FormatCronPush).
// Empty/missing project templates must call this path for zero regression
// (including omitting the whole「节点：」line when node display is empty).
func FormatRunNotifyMessage(ev RunNotifyEvent, base string) string {
	title := RunNotifyTitle(ev.Kind)
	projectName := strings.TrimSpace(ev.ProjectName)
	if projectName == "" {
		projectName = "—"
	}
	wfName := strings.TrimSpace(ev.WorkflowName)
	if wfName == "" {
		wfName = "—"
	}
	node := RunNotifyNodeDisplay(ev)
	link := runDeepLink(base, ev.RunID)
	var b strings.Builder
	b.WriteString("【Approving】")
	b.WriteString(title)
	b.WriteByte('\n')
	b.WriteString("项目：")
	b.WriteString(projectName)
	b.WriteByte('\n')
	b.WriteString("工作流：")
	b.WriteString(wfName)
	b.WriteByte('\n')
	b.WriteString("Run：")
	b.WriteString(ev.RunID)
	if node != "" {
		b.WriteByte('\n')
		b.WriteString("节点：")
		b.WriteString(node)
	}
	b.WriteByte('\n')
	b.WriteString("打开：")
	b.WriteString(link)
	return b.String()
}

func runDeepLink(base, runID string) string {
	path := "/runs/" + runID
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return path
	}
	return base + path
}

// FormatRunDeepLinkForTest exposes runDeepLink for unit tests.
func FormatRunDeepLinkForTest(base, runID string) string {
	return runDeepLink(base, runID)
}

// ClaimReceiptForTest exposes claimReceipt for unit tests.
func (s *RunNotifyService) ClaimReceiptForTest(runID, nodeID string, iteration int, kind string) (bool, error) {
	return s.claimReceipt(runID, nodeID, iteration, kind)
}

// HasClaimedForTest reports whether a receipt exists.
func (s *RunNotifyService) HasClaimedForTest(runID, nodeID string, iteration int, kind string) bool {
	var n int64
	s.db.Model(&models.NotifyDeliveryReceipt{}).
		Where("run_id = ? AND node_id = ? AND iteration = ? AND kind = ?", runID, nodeID, iteration, kind).
		Count(&n)
	return n > 0
}
