package services

import (
	"errors"
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

func TestListAndCloseProjectTasksForManualCleanup(t *testing.T) {
	db := taskContextDB(t)
	svc := NewTaskContextService(db)
	live, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "run-open", ProjectID: "p1", UserID: SyntheticQQUserID("u1"),
		ShortTitle: "查主干", Status: "running", OriginConversationID: "c1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "run-done", ProjectID: "p1", UserID: SyntheticQQUserID("u1"),
		ShortTitle: "已结束", Status: "completed", OriginConversationID: "c1",
	}); err != nil {
		t.Fatal(err)
	}
	active, err := svc.ListProjectTasks("p1", ProjectTaskQuery{ActiveOnly: true, Limit: 50})
	if err != nil || len(active) != 1 || active[0].ID != live.ID {
		t.Fatalf("active = %+v err=%v", active, err)
	}
	closed, err := svc.CloseProjectTask("p1", live.ID, "cancelled")
	if err != nil || closed == nil || closed.TerminalAt == nil {
		t.Fatalf("close = %+v err=%v", closed, err)
	}
	active, err = svc.ListProjectTasks("p1", ProjectTaskQuery{ActiveOnly: true, Limit: 50})
	if err != nil || len(active) != 0 {
		t.Fatalf("after close active = %+v err=%v", active, err)
	}
}

func TestActiveTasksAreScopedToTheConversationAndReapGhosts(t *testing.T) {
	db := taskContextDB(t)
	if err := db.AutoMigrate(&models.Run{}); err != nil {
		t.Fatal(err)
	}
	svc := NewTaskContextService(db)
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })

	scope := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("u1"), Channel: "qq", ConversationID: "c1"}
	live, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "run-live", ProjectID: scope.ProjectID, UserID: scope.UserID,
		ShortTitle: "查主干", Status: "running",
		OriginConversationID: "c1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "run-other-chat", ProjectID: scope.ProjectID, UserID: scope.UserID,
		ShortTitle: "别的会话的活", Status: "running",
		OriginConversationID: "c2",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "dispatch:old", ProjectID: scope.ProjectID, UserID: scope.UserID,
		ShortTitle: "早答完的查询", Status: "running",
		OriginConversationID: "c1",
	}); err != nil {
		t.Fatal(err)
	}
	// Backdate the ephemeral stub past the reap TTL.
	if err := db.Model(&models.TaskIdentity{}).Where("run_id = ?", "dispatch:old").
		Update("updated_at", now.Add(-StaleDispatchTTL-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Run{ID: "run-done", Status: "completed"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "run-done", ProjectID: scope.ProjectID, UserID: scope.UserID,
		ShortTitle: "Run 已结束但账本忘了", Status: "running",
		OriginConversationID: "c1",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ActiveTasksForConversation(scope, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != live.ID {
		t.Fatalf("active = %+v want only the live task in this conversation", got)
	}
	var stub models.TaskIdentity
	if err := db.Where("run_id = ?", "dispatch:old").First(&stub).Error; err != nil {
		t.Fatal(err)
	}
	if stub.Status != "cancelled" || stub.TerminalAt == nil {
		t.Fatalf("stale dispatch should be cancelled, got status=%q terminal=%v", stub.Status, stub.TerminalAt)
	}
}

func TestRecentTerminalTasksSurfaceFailuresForStatus(t *testing.T) {
	db := taskContextDB(t)
	svc := NewTaskContextService(db)
	now := time.Date(2026, 8, 8, 3, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	scope := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("u1"), Channel: "qq", ConversationID: "c1"}

	failed, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "run-fail", ProjectID: scope.ProjectID, UserID: scope.UserID,
		ShortTitle: "统一错误码", Status: "failed", OriginConversationID: "c1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "run-ok", ProjectID: scope.ProjectID, UserID: scope.UserID,
		ShortTitle: "别的会话成功", Status: "completed", OriginConversationID: "c2",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.RecentTerminalTasksForConversation(scope, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != failed.ID || got[0].Status != "failed" {
		t.Fatalf("recent = %+v want the failed task in this conversation", got)
	}
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
	// Another QQ identity in the same project must remain outside this user's
	// candidate set.
	ensureTask(t, svc, "r4", scope.ProjectID, SyntheticQQUserID("other"), "登录页性能优化")

	res, err := svc.ResolveTask(ResolveTaskInput{Scope: scope, Query: "登录页"})
	if err != nil || !res.Ambiguous || len(res.Candidates) < 2 {
		t.Fatalf("ambiguous resolution=%+v err=%v", res, err)
	}
	for _, c := range res.Candidates {
		if c.Identity.ProjectID != scope.ProjectID {
			t.Fatalf("cross-project candidate leaked: %+v", c.Identity)
		}
		if c.Identity.UserID != scope.UserID {
			t.Fatalf("cross-user candidate leaked: %+v", c.Identity)
		}
	}
	otherScope := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("other"), Channel: "qq", ConversationID: "c-other"}
	if _, err := svc.SetFocus(otherScope, &res.Candidates[0].Identity, "zh-CN"); !errors.Is(err, ErrTaskIdentityScopeMismatch) {
		t.Fatalf("cross-user focus error = %v", err)
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

func TestClearConversationLedgerCancelsActiveAndDropsFocus(t *testing.T) {
	db := taskContextDB(t)
	svc := NewTaskContextService(db)
	scope := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("u1"), Channel: "qq", ConversationID: "c-clear"}
	task, err := svc.EnsureIdentity(EnsureTaskIdentityInput{
		RunID: "run-clear", ProjectID: scope.ProjectID, UserID: scope.UserID,
		ShortTitle: "清上下文", Status: "active",
		OriginChannel: scope.Channel, OriginConversationID: scope.ConversationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetFocus(scope, task, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if err := svc.BindMessage(scope, "m-1", task); err != nil {
		t.Fatal(err)
	}
	n, err := svc.ClearConversationLedger(scope.ProjectID, scope.Channel, scope.ConversationID)
	if err != nil || n != 1 {
		t.Fatalf("cancelled=%d err=%v", n, err)
	}
	var focusCount int64
	if err := db.Model(&models.ConversationFocus{}).Count(&focusCount).Error; err != nil || focusCount != 0 {
		t.Fatalf("focus left=%d err=%v", focusCount, err)
	}
	var bindCount int64
	if err := db.Model(&models.MessageBinding{}).Count(&bindCount).Error; err != nil || bindCount != 0 {
		t.Fatalf("bindings left=%d err=%v", bindCount, err)
	}
	stored, err := svc.IdentityByID(task.ID, scope.ProjectID)
	if err != nil || stored == nil || !IsTerminalTaskStatus(stored.Status) {
		t.Fatalf("task should be terminal: %+v err=%v", stored, err)
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
	// A task reference reads like speech, not like a ticket header, and is
	// omitted entirely when there is no task to name.
	if got := FormatTaskType("Login", "en"); !strings.Contains(got, "Login") ||
		strings.Contains(got, "【") || strings.Contains(got, "｜") {
		t.Fatalf("format=%q", got)
	}
	if got := FormatTaskType("登录页", "zh-CN"); got != "登录页那个：" {
		t.Fatalf("zh format=%q", got)
	}
	if got := FormatTaskType("", "zh-CN"); got != "" {
		t.Fatalf("a missing title must not become a placeholder prefix: %q", got)
	}
}

func TestSanitizeShortTitleStripsRunIdentifiers(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"修复 Run run-1ca1876f", "修复"},
		{"run-1ca1876f", ""},
		{"Run", ""},
		{"task-9f8e7d6c5b4a", ""},
		{"登录页性能优化", "登录页性能优化"},
	} {
		if got := SanitizeShortTitle(tc.in); got != tc.want {
			t.Fatalf("SanitizeShortTitle(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeShortTitleDoesNotCutMidToken(t *testing.T) {
	// 24-rune hard cut used to yield 「调研 Approving 最近关于快模型和 wo」。
	in := "调研 Approving 最近关于快模型和 worker 的架构精简与重构可行性分析"
	got := SanitizeShortTitle(in)
	if got == "" {
		t.Fatal("empty title")
	}
	trimmed := strings.TrimSuffix(got, "…")
	if strings.HasSuffix(trimmed, " wo") || strings.HasSuffix(trimmed, "wo") && !strings.HasSuffix(trimmed, "worker") {
		t.Fatalf("mid-token cut still present: %q", got)
	}
	if len([]rune(in)) > runShortTitleRunes && !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated title should end with ellipsis: %q", got)
	}
	if n := len([]rune(strings.TrimSuffix(got, "…"))); n > runShortTitleRunes {
		t.Fatalf("title length %d > %d: %q", n, runShortTitleRunes, got)
	}
}

func TestRunShortTitleNeverExposesRunID(t *testing.T) {
	got := runShortTitle(models.Run{
		ID:     "run-1ca1876f",
		Inputs: map[string]any{"requirement": "把登录页首屏时间降下来。顺便看看埋点。"},
	})
	if got != "把登录页首屏时间降下来" {
		t.Fatalf("short title = %q", got)
	}
	if strings.Contains(runShortTitle(models.Run{ID: "run-1ca1876f"}), "run-") {
		t.Fatal("a run with no usable text must not fall back to its id")
	}
}

func TestTaskLanguageFollowsTaskNotMessage(t *testing.T) {
	// A short foreign-language fragment inside an established task does not
	// flip the language; a full sentence does.
	if got := TaskLanguageFor("zh-CN", "PR #12"); got != "zh-CN" {
		t.Fatalf("short fragment switched language: %q", got)
	}
	if got := TaskLanguageFor("zh-CN", "Could you also check the login page performance?"); got != "en" {
		t.Fatalf("a clear switch was ignored: %q", got)
	}
	if got := TaskLanguageFor("", "怎么样了"); got != "zh-CN" {
		t.Fatalf("no established language should fall back to detection: %q", got)
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
	// The prompt names the task and the action so the wrong thing cannot be
	// confirmed by accident, but it asks the way a person would.
	prompt := risk.ConfirmationPrompt(*ticket)
	if !strings.Contains(prompt, "结算页") || !strings.Contains(prompt, "删除生产分支") {
		t.Fatalf("prompt = %q", prompt)
	}
	if strings.Contains(prompt, "【") || strings.Contains(prompt, "｜") {
		t.Fatalf("confirmation prompt still uses the ticket header form: %q", prompt)
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
	// An expired confirmation still names the task and says the action did not
	// happen, so the user is never left guessing what state things are in.
	if !strings.Contains(expired.Message, "结算页") || expired.TaskStatus != "completed" {
		t.Fatalf("expired message = %q status=%q", expired.Message, expired.TaskStatus)
	}
	if !strings.Contains(expired.Message, "过期") || strings.Contains(expired.Message, "【") {
		t.Fatalf("expired message should explain the timeout in plain language: %q", expired.Message)
	}
}

func TestEnsureIdentityForRunDerivesFieldsAndBackfillsScoped(t *testing.T) {
	db := taskContextDB(t)
	if err := db.AutoMigrate(&models.Run{}, &models.WorkflowDef{}); err != nil {
		t.Fatal(err)
	}
	svc := NewTaskContextService(db)

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
	// Search must not claim project Runs on behalf of the first caller.
	candidates, err := svc.Search(scope, "登录页")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("unclaimed run became visible: %+v", candidates)
	}
	var unclaimedCount int64
	if err := db.Model(&models.TaskIdentity{}).Where("run_id = ?", "r1").Count(&unclaimedCount).Error; err != nil {
		t.Fatal(err)
	}
	if unclaimedCount != 0 {
		t.Fatalf("search claimed an unowned run: count=%d", unclaimedCount)
	}

	var run models.Run
	if err := db.First(&run, "id = ?", "r1").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureIdentityForRun(run, "p1", scope.UserID); err != nil {
		t.Fatal(err)
	}
	candidates, err = svc.Search(scope, "登录页")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Identity.RunID != "r1" {
		t.Fatalf("explicitly owned candidates = %+v", candidates)
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
	// A different QQ identity in the same project must not discover a task
	// owned by the first identity.
	intruder := TaskScope{ProjectID: "p1", UserID: SyntheticQQUserID("u2"), Channel: "qq", ConversationID: "c9"}
	res, err := svc.ResolveTask(ResolveTaskInput{Scope: intruder, Query: "修复登录页跳转"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Identity != nil || len(res.Candidates) != 0 || res.Reason != "no_match" {
		t.Fatalf("cross-user task leaked: %+v", res)
	}
	if _, err := svc.EnsureIdentityForRun(models.Run{
		ID: "r1", Status: "running", Title: "修复登录页跳转",
	}, "p1", intruder.UserID); !errors.Is(err, ErrTaskIdentityScopeMismatch) {
		t.Fatalf("cross-user identity update error = %v", err)
	}
	var count int64
	if err := db.Model(&models.TaskIdentity{}).Where("run_id = ?", "r1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("run r1 has %d identities; a Run has one project-scoped identity", count)
	}
}

func TestIdentityForRunIsProjectScopedAndAbsenceIsNotAnError(t *testing.T) {
	db := taskContextDB(t)
	svc := NewTaskContextService(db)
	ensureTask(t, svc, "r1", "p1", SyntheticQQUserID("u1"), "登录页")

	got, err := svc.IdentityForRun("r1", "p1")
	if err != nil || got == nil || got.UserID != SyntheticQQUserID("u1") {
		t.Fatalf("identity for run = %+v err=%v", got, err)
	}
	// Another project must never see this Run's identity, and a miss is a
	// nil identity rather than an error so callers can skip binding.
	for _, miss := range []struct{ run, project string }{
		{"r1", "p2"},
		{"missing", "p1"},
	} {
		got, err := svc.IdentityForRun(miss.run, miss.project)
		if err != nil || got != nil {
			t.Fatalf("IdentityForRun(%q,%q) = %+v err=%v", miss.run, miss.project, got, err)
		}
	}
	for _, bad := range []struct{ run, project string }{{"", "p1"}, {"r1", ""}} {
		if _, err := svc.IdentityForRun(bad.run, bad.project); err == nil {
			t.Fatalf("IdentityForRun(%q,%q) must reject empty scope", bad.run, bad.project)
		}
	}
}
