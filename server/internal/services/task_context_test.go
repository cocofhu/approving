package services

import (
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func taskContextDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.TaskIdentity{}, &models.MessageBinding{}, &models.ConversationFocus{},
		&models.RiskConfirmationTicket{}, &models.SendableDeliveryReceipt{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func ensureTask(t *testing.T, svc *TaskContextService, run, project, user, title string) *models.TaskIdentity {
	t.Helper()
	task, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: run, ProjectID: project, UserID: user, ShortTitle: title,
		OriginalRequirement: "实现" + title, Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestTaskContextMigrationIdentityResolutionAndIsolation(t *testing.T) {
	db := taskContextDB(t)
	svc := NewTaskContextService(db)
	scope := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("u1"), Channel: "qq", ConversationID: "c"}
	task := ensureTask(t, svc, "r1", scope.ProjectID, scope.UserID, "支付登录页")
	renamed, err := svc.UpdateIdentity(EnsureTaskIdentityInput{
		RunID: "r1", ProjectID: scope.ProjectID, UserID: scope.UserID, ShortTitle: "商户登录页",
		Status: "active",
	})
	if err != nil || len(renamed.Aliases) != 1 || renamed.Aliases[0] != "支付登录页" {
		t.Fatalf("rename=%+v err=%v", renamed, err)
	}
	if err := svc.BindMessage(scope, "m1", task); err != nil {
		t.Fatal(err)
	}
	ensureTask(t, svc, "r2", scope.ProjectID, scope.UserID, "用户登录页")
	ensureTask(t, svc, "r3", "other-project", scope.UserID, "登录页")
	// Another QQ identity may seed the same project; metadata stays project-scoped
	// and searchable, while focus/bindings remain per synthetic identity.
	ensureTask(t, svc, "r4", scope.ProjectID, SyntheticQQUserID("other"), "登录页性能优化")

	res, err := svc.ResolveTask(ResolveTaskInput{Scope: scope, Query: "登录页"})
	if err != nil || !res.Ambiguous || len(res.Candidates) < 2 {
		t.Fatalf("ambiguous resolution=%+v err=%v", res, err)
	}
	for _, c := range res.Candidates {
		if c.Identity.ProjectID != scope.ProjectID {
			t.Fatalf("cross-project candidate leaked: %+v", c.Identity)
		}
	}
	otherScope := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("other"), Channel: "qq", ConversationID: "c-other"}
	if _, err := svc.SetFocus(otherScope, &res.Candidates[0].Identity, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if focus, err := svc.GetFocus(scope, false); err == nil {
		t.Fatalf("u1 must not see other identity focus: %+v", focus)
	}

	ref, err := svc.ResolveTask(ResolveTaskInput{
		Scope: scope, Query: "用户登录页", ReplyMessageID: "m1",
	})
	if err != nil || ref.Identity == nil || ref.Identity.RunID != "r1" || ref.Reason != "reply_binding" {
		t.Fatalf("binding precedence=%+v err=%v", ref, err)
	}
	ordinal, err := svc.ResolveTask(ResolveTaskInput{
		Scope: scope, Ordinal: 2, Candidates: res.Candidates,
	})
	if err != nil || ordinal.Identity == nil {
		t.Fatalf("ordinal=%+v err=%v", ordinal, err)
	}
}

func TestConversationFocusRenewAndExpireWithoutGuess(t *testing.T) {
	db := taskContextDB(t)
	svc := NewTaskContextService(db)
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	scope := TaskScope{ProjectID: "p", UserID: "qq:u", Channel: "qq", ConversationID: "c"}
	task := ensureTask(t, svc, "r", "p", "qq:u", "任务")
	if _, err := svc.SetFocus(scope, task, "en"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Minute)
	res, err := svc.ResolveTask(ResolveTaskInput{Scope: scope})
	if err != nil || res.Identity == nil || res.Reason != "conversation_focus" {
		t.Fatalf("focused=%+v err=%v", res, err)
	}
	var focus models.ConversationFocus
	if err := db.First(&focus).Error; err != nil || !focus.ExpiresAt.Equal(now.Add(TaskFocusTTL)) {
		t.Fatalf("focus not renewed: %+v err=%v", focus, err)
	}
	now = now.Add(31 * time.Minute)
	res, err = svc.ResolveTask(ResolveTaskInput{Scope: scope})
	if err != nil || res.Identity != nil || res.Reason != "focus_missing_or_expired" {
		t.Fatalf("expired focus guessed: %+v err=%v", res, err)
	}
}

func TestRiskConfirmationOneShotDuplicateExpiredAndLanguage(t *testing.T) {
	db := taskContextDB(t)
	svc := NewRiskConfirmationService(db)
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	input := RiskTicketInput{ProjectID: "p", UserID: "qq:u", RunID: "r", Action: "delete", Language: "en"}
	if _, err := svc.CreateTicket(input); err != nil {
		t.Fatal(err)
	}
	first, err := svc.ResolveTicket(input, "confirm")
	if err != nil || !first.Execute || first.Ticket.Status != "confirmed" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	repeated, err := svc.ResolveTicket(input, "确认")
	if err != nil || repeated.Execute || repeated.Ticket.Status != "confirmed" {
		t.Fatalf("repeated=%+v err=%v", repeated, err)
	}
	expiring := RiskTicketInput{ProjectID: "p", UserID: "qq:u", RunID: "r2", Action: "deploy", Language: "zh-CN"}
	if _, err := svc.CreateTicket(expiring); err != nil {
		t.Fatal(err)
	}
	now = now.Add(RiskConfirmationTTL + time.Second)
	expired, err := svc.ResolveTicket(expiring, "确认")
	if err != nil || expired.Execute || expired.Ticket.Status != "expired" {
		t.Fatalf("expired=%+v err=%v", expired, err)
	}
	if DetectLanguage("please continue", "zh-CN") != "en" ||
		DetectLanguage("", "en") != "en" || DetectLanguage("", "") != "zh-CN" {
		t.Fatal("language fallback order is incorrect")
	}
	if got := FormatTaskType("Login", "Risk", "en"); got != "【Login｜Risk】" {
		t.Fatalf("format=%q", got)
	}
}

func TestRiskConfirmationEchoesShortTitleAndLatestTaskStatus(t *testing.T) {
	db := taskContextDB(t)
	if err := db.AutoMigrate(&models.Run{}, &models.WorkflowDef{}); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskContextService(db)
	risk := NewRiskConfirmationService(db)
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	tasks.SetClock(func() time.Time { return now })
	risk.SetClock(func() time.Time { return now })

	if err := db.Create(&models.Run{ID: "r1", Status: "running", Title: "结算页重构"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "r1", ProjectID: "p", UserID: "qq:u", ShortTitle: "结算页", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}

	// The short title is resolved from the task identity, not passed by hand.
	input := RiskTicketInput{ProjectID: "p", UserID: "qq:u", RunID: "r1", Action: "删除生产分支", Language: "zh-CN"}
	ticket, err := risk.CreateTicket(input)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.ShortTitle != "结算页" {
		t.Fatalf("ticket short title = %q", ticket.ShortTitle)
	}
	prompt := risk.ConfirmationPrompt(*ticket)
	if !strings.HasPrefix(prompt, "【结算页｜高风险确认】") || !strings.Contains(prompt, "删除生产分支") {
		t.Fatalf("prompt = %q", prompt)
	}

	confirmed, err := risk.ResolveTicket(input, "确认")
	if err != nil || !confirmed.Execute {
		t.Fatalf("confirm = %+v err=%v", confirmed, err)
	}

	// The Run moves on; a repeated decision reports the latest task status.
	if err := db.Model(&models.Run{}).Where("id = ?", "r1").
		Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	repeated, err := risk.ResolveTicket(input, "确认")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Execute {
		t.Fatal("repeated confirmation must not execute again")
	}
	if repeated.TaskStatus != "completed" {
		t.Fatalf("latest task status = %q want completed", repeated.TaskStatus)
	}
	if !strings.Contains(repeated.Message, "结算页") || !strings.Contains(repeated.Message, "completed") {
		t.Fatalf("repeat message = %q", repeated.Message)
	}

	// Expiry also reports the short title and the latest status.
	expiring := RiskTicketInput{ProjectID: "p", UserID: "qq:u", RunID: "r1", Action: "回滚发布", Language: "zh-CN"}
	if _, err := risk.CreateTicket(expiring); err != nil {
		t.Fatal(err)
	}
	now = now.Add(RiskConfirmationTTL + time.Second)
	expired, err := risk.ResolveTicket(expiring, "确认")
	if err != nil {
		t.Fatal(err)
	}
	if expired.Execute || expired.Ticket.Status != "expired" {
		t.Fatalf("expired = %+v", expired)
	}
	if !strings.Contains(expired.Message, "【结算页｜已过期】") || expired.TaskStatus != "completed" {
		t.Fatalf("expired message = %q status=%q", expired.Message, expired.TaskStatus)
	}
}

func TestEnsureIdentityForRunDerivesFieldsAndBackfillsScoped(t *testing.T) {
	db := taskContextDB(t)
	if err := db.AutoMigrate(&models.Run{}, &models.WorkflowDef{}); err != nil {
		t.Fatal(err)
	}
	svc := NewTaskContextService(db)
	svc.EnableRunBackfill()

	if err := db.Create(&models.WorkflowDef{ID: "wf1", ProjectID: "p1", Name: "登录流程"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.WorkflowDef{ID: "wf2", ProjectID: "p2", Name: "结算流程"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Run{
		ID: "r1", WorkflowID: "wf1", WorkflowName: "登录流程", Status: "running",
		Title: "修复登录页跳转", Tags: []string{"login"},
		Inputs:    map[string]any{"requirement": "修复登录页在移动端的跳转问题"},
		CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Run{
		ID: "r2", WorkflowID: "wf2", WorkflowName: "结算流程", Status: "running",
		Title: "结算页", CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	scope := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("u1"), Channel: "qq", ConversationID: "c1"}
	// The caller supplies nothing but the query; identities are derived lazily.
	candidates, err := svc.Search(scope, "登录页")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Identity.RunID != "r1" {
		t.Fatalf("backfilled candidates = %+v", candidates)
	}
	got := candidates[0].Identity
	if got.ShortTitle != "修复登录页跳转" {
		t.Fatalf("derived short title = %q", got.ShortTitle)
	}
	if got.OriginalRequirement != "修复登录页在移动端的跳转问题" {
		t.Fatalf("derived requirement = %q", got.OriginalRequirement)
	}
	if got.Status != "running" || len(got.Keywords) == 0 {
		t.Fatalf("derived identity = %+v", got)
	}

	// A different project's Run is never exposed to this identity.
	if others, err := svc.Search(scope, "结算页"); err != nil || len(others) != 0 {
		t.Fatalf("cross-project leak: %+v err=%v", others, err)
	}
	// A different QQ identity in the same project must still find project-scoped
	// task metadata; only focus/bindings stay isolated per synthetic identity.
	intruder := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("u2"), Channel: "qq", ConversationID: "c9"}
	res, err := svc.ResolveTask(ResolveTaskInput{Scope: intruder, Query: "修复登录页跳转"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Identity == nil || res.Identity.RunID != "r1" {
		t.Fatalf("same-project QQ identity should resolve project tasks: %+v", res)
	}
	focusA, err := svc.SetFocus(scope, res.Identity, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	focusB, err := svc.GetFocus(intruder, false)
	if err != nil {
		t.Fatal(err)
	}
	if focusA.UserID == focusB.UserID {
		t.Fatalf("focus must stay isolated per QQ identity: %+v %+v", focusA, focusB)
	}
	var count int64
	if err := db.Model(&models.TaskIdentity{}).Where("run_id = ?", "r1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("run r1 has %d identities; a Run has one project-scoped identity", count)
	}
}
