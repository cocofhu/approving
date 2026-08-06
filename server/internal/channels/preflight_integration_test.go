package channels

import (
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func preflightFixture(t *testing.T) (*gorm.DB, *ChannelBridge, ResolvedChannel) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.WorkflowDef{}, &models.Run{}, &models.TaskIdentity{}, &models.MessageBinding{},
		&models.ConversationFocus{}, &models.RiskConfirmationTicket{},
	); err != nil {
		t.Fatal(err)
	}
	workflows := []models.WorkflowDef{
		{ID: "wf-p1", ProjectID: "p1", Name: "发布工作流"},
		{ID: "wf-p2", ProjectID: "p2", Name: "隔离工作流"},
	}
	for _, workflow := range workflows {
		if err := db.Create(&workflow).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	runs := []models.Run{
		{ID: "run-a", WorkflowID: "wf-p1", Title: "支付任务一", Status: "running", Inputs: map[string]any{"requirement": "支付服务"}, StartedAt: now},
		{ID: "run-b", WorkflowID: "wf-p1", Title: "支付任务二", Status: "queued", Inputs: map[string]any{"requirement": "支付服务"}, StartedAt: now},
		{ID: "run-risk", WorkflowID: "wf-p1", Title: "Alpha", Status: "running", Inputs: map[string]any{"prompt": "deploy alpha"}, StartedAt: now},
		{ID: "run-foreign", WorkflowID: "wf-p2", Title: "Foreign Secret", Status: "running", Inputs: map[string]any{"task": "foreign only"}, StartedAt: now},
	}
	for _, run := range runs {
		if err := db.Create(&run).Error; err != nil {
			t.Fatal(err)
		}
	}
	bridge := NewChannelBridge(nil, nil, nil, MCPTokenHooks{})
	bridge.SetTaskContext(services.NewTaskContextService(db))
	return db, bridge, ResolvedChannel{Type: "qq", ProjectID: "p1", ReplyMetadata: false}
}

func TestInboundRuntimeAmbiguitySelectionFocusExpiryAndIsolation(t *testing.T) {
	db, bridge, rc := preflightFixture(t)
	base := InboundMessage{Scene: SceneGroup, ConversationID: "group-1", UserID: "user-a"}

	ambiguous := base
	ambiguous.Text, ambiguous.MessageID = "支付服务状态", "msg-amb"
	reply, err := bridge.Handle(t.Context(), rc, ambiguous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Final == nil || !strings.Contains(reply.Final.Summary, "1.") || !strings.HasPrefix(reply.Final.Summary, "【") {
		t.Fatalf("QQ ambiguity fallback missing: %#v", reply)
	}

	selected := base
	selected.Text, selected.MessageID = "2", "msg-select"
	reply, err = bridge.Handle(t.Context(), rc, selected, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply.RunID == "" || reply.Final == nil || !strings.Contains(reply.Final.Summary, "已选择") {
		t.Fatalf("candidate selection failed: %#v", reply)
	}
	selectedRun := reply.RunID
	userScope := TaskUserScope("qq", base)
	focus, err := bridge.tasks.GetConversationFocus("p1", "qq", "group-1", userScope)
	if err != nil || focus.RunID != selectedRun {
		t.Fatalf("focus = %#v, %v", focus, err)
	}

	status := base
	status.Text, status.MessageID = "继续，状态怎么样", "msg-status"
	reply, err = bridge.Handle(t.Context(), rc, status, nil)
	if err != nil || reply.RunID != selectedRun || !strings.Contains(reply.Final.Summary, "当前状态") {
		t.Fatalf("focused status = %#v, %v", reply, err)
	}
	if err := db.Model(&models.ConversationFocus{}).
		Where("project_id = ? AND channel = ? AND conversation_id = ? AND user_id = ?", "p1", "qq", "group-1", userScope).
		Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	expired, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: status})
	if err != nil {
		t.Fatal(err)
	}
	if expired.Task != nil && expired.Task.RunID == selectedRun {
		t.Fatalf("expired focus leaked selected task: %#v", expired)
	}
	expiredAgain, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: status})
	if err != nil || (expiredAgain.Task != nil && expiredAgain.Task.RunID == selectedRun) {
		t.Fatalf("language tracking resurrected expired focus: %#v %v", expiredAgain, err)
	}

	otherUser := base
	otherUser.UserID, otherUser.Text, otherUser.MessageID = "user-b", "支付任务一 状态", "msg-other"
	other, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: otherUser})
	if err != nil || other.Task != nil || other.Disposition != PreflightRespond {
		t.Fatalf("other user saw claimed task: %#v %v", other, err)
	}
	if TaskUserScope("qq", otherUser) == userScope {
		t.Fatal("group user scopes collided")
	}
	foreign := base
	foreign.Text = "Foreign Secret status"
	foreignResult, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: foreign})
	if err != nil || foreignResult.Task != nil || foreignResult.Disposition != PreflightRespond {
		t.Fatalf("cross-project run exposed: %#v %v", foreignResult, err)
	}

	var identity models.TaskIdentity
	if err := db.Where("project_id = ? AND user_id = ? AND run_id = ?", "p1", userScope, "run-a").First(&identity).Error; err != nil {
		t.Fatal(err)
	}
	if identity.ShortTitle != "支付任务一" || identity.OriginalRequirement != "支付服务" || identity.Status != "running" || len(identity.Keywords) == 0 {
		t.Fatalf("auto-materialized identity = %#v", identity)
	}
}

func TestInboundRuntimeBindingAndRiskTicketOneTime(t *testing.T) {
	db, bridge, rc := preflightFixture(t)
	in := InboundMessage{
		Scene: SceneC2C, ConversationID: "openid-1", UserID: "openid-1",
		Text: "delete Alpha", MessageID: "msg-risk", NodeID: "node-1", GateID: "gate-1",
	}
	first, err := bridge.Handle(t.Context(), rc, in, nil)
	if err != nil || first.Final == nil || !strings.Contains(first.Final.Summary, "5 minutes") {
		t.Fatalf("risk interception: %#v %v", first, err)
	}
	scope := TaskUserScope("qq", in)
	var binding models.MessageBinding
	if err := db.First(&binding, "message_id = ?", "msg-risk").Error; err != nil {
		t.Fatal(err)
	}
	if binding.RunID != "run-risk" || binding.NodeID != "node-1" || binding.GateID != "gate-1" || binding.Action != "delete" {
		t.Fatalf("binding context = %#v", binding)
	}

	bound := in
	bound.Text, bound.MessageID, bound.ReplyToMessageID = "continue with details", "msg-bound", "msg-risk"
	boundResult, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: bound})
	if err != nil || boundResult.Task == nil || boundResult.Task.RunID != "run-risk" {
		t.Fatalf("reply binding did not resolve exactly: %#v %v", boundResult, err)
	}

	confirm := in
	confirm.Text, confirm.MessageID = "确认", "msg-confirm"
	authorized, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: confirm})
	if err != nil || authorized.Disposition != PreflightProceed || authorized.Task == nil ||
		authorized.Task.RunID != "run-risk" || authorized.AuthorizedAction != "delete Alpha" || authorized.TicketID == "" {
		t.Fatalf("confirmation authorization = %#v %v", authorized, err)
	}
	repeated, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: confirm})
	if err != nil || repeated.Disposition != PreflightRespond || repeated.Reply.Final == nil ||
		!strings.Contains(repeated.Reply.Final.Summary, "已处理") {
		t.Fatalf("repeated confirmation reauthorized: %#v %v", repeated, err)
	}

	cancelRisk := in
	cancelRisk.Text, cancelRisk.MessageID = "cancel Alpha", "msg-cancel-risk"
	if _, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: cancelRisk}); err != nil {
		t.Fatal(err)
	}
	cancel := in
	cancel.Text, cancel.MessageID = "cancel", "msg-cancel"
	cancelled, err := bridge.PreflightInbound(InboundPreflightRequest{Channel: rc, Message: cancel})
	if err != nil || cancelled.Disposition != PreflightRespond || !strings.Contains(cancelled.Reply.Final.Summary, "will not run") {
		t.Fatalf("cancel confirmation = %#v %v", cancelled, err)
	}

	var tickets int64
	if err := db.Model(&models.RiskConfirmationTicket{}).Where("project_id = ? AND user_id = ?", "p1", scope).Count(&tickets).Error; err != nil {
		t.Fatal(err)
	}
	if tickets != 2 {
		t.Fatalf("ticket count = %d", tickets)
	}
}
