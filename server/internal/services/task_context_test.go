package services

import (
	"errors"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func taskContextTestService(t *testing.T) *TaskContextService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.TaskIdentity{}, &models.MessageBinding{}, &models.ConversationFocus{}, &models.RiskConfirmationTicket{}); err != nil {
		t.Fatal(err)
	}
	return NewTaskContextService(db)
}

func TestTaskIdentityRenameResolveAndScope(t *testing.T) {
	s := taskContextTestService(t)
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	in := TaskIdentityInput{
		RunID: "r1", ProjectID: "p1", UserID: "u1", ShortTitle: "支付修复",
		OriginalRequirement: "修复支付回调重复扣款", Keywords: []string{"支付", "回调"}, Status: "running",
	}
	if _, err := s.UpsertTaskIdentity(in); err != nil {
		t.Fatal(err)
	}
	renamed, err := s.UpdateTaskTitle("p1", "u1", "r1", "支付幂等修复")
	if err != nil {
		t.Fatal(err)
	}
	if len(renamed.Aliases) != 1 || renamed.Aliases[0] != "支付修复" {
		t.Fatalf("aliases not preserved: %#v", renamed.Aliases)
	}
	res, err := s.ResolveTask(TaskResolveRequest{ProjectID: "p1", UserID: "u1", Query: "支付修复", Now: now})
	if err != nil || res.Task == nil || res.Task.RunID != "r1" {
		t.Fatalf("alias resolve: %#v, %v", res, err)
	}
	if _, err := s.ResolveTask(TaskResolveRequest{ProjectID: "other", UserID: "u1", Query: "支付修复", Now: now}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-project leaked: %v", err)
	}
}

func TestConversationFocusExpiresAndCandidatesSelect(t *testing.T) {
	s := taskContextTestService(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	for _, id := range []string{"r1", "r2"} {
		if _, err := s.UpsertTaskIdentity(TaskIdentityInput{
			RunID: id, ProjectID: "p", UserID: "u", ShortTitle: "发布" + id, OriginalRequirement: "发布服务", Status: "running",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.TouchConversationFocus("p", "qq", "c", "u", "r2"); err != nil {
		t.Fatal(err)
	}
	res, err := s.ResolveTask(TaskResolveRequest{
		ProjectID: "p", UserID: "u", Query: "发布服务", Channel: "qq", ConversationID: "c", Now: now,
	})
	if err != nil || res.Task == nil || res.Task.RunID != "r2" {
		t.Fatalf("focus did not disambiguate: %#v %v", res, err)
	}
	picked, err := SelectTaskCandidate([]TaskCandidate{{Task: models.TaskIdentity{RunID: "r1", ShortTitle: "一"}}, {Task: models.TaskIdentity{RunID: "r2", ShortTitle: "二"}}}, "2")
	if err != nil || picked.RunID != "r2" {
		t.Fatalf("sequence selection: %#v %v", picked, err)
	}
}

func TestRiskConfirmationOneTimeScopedAndExpires(t *testing.T) {
	s := taskContextTestService(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	if _, err := s.CreateRiskConfirmation("p", "u", "r", "deploy"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ConsumeRiskConfirmation("p", "u", "r", "deploy", "确认")
	if err != nil || !got.Confirm {
		t.Fatalf("confirm: %#v %v", got, err)
	}
	repeated, err := s.ConsumeRiskConfirmation("p", "u", "r", "deploy", "yes")
	if !errors.Is(err, ErrConfirmationStale) || !repeated.Stale || repeated.Latest == nil {
		t.Fatalf("repeat should return latest without reexecute: %#v %v", repeated, err)
	}
	if _, err := s.CreateRiskConfirmation("p", "u", "r", "delete"); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now.Add(6 * time.Minute) }
	expired, err := s.ConsumeRiskConfirmation("p", "u", "r", "delete", "confirm")
	if !errors.Is(err, ErrConfirmationStale) || expired.Latest == nil || expired.Latest.Status != "expired" {
		t.Fatalf("expired ticket: %#v %v", expired, err)
	}
}
