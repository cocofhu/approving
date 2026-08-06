package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/pmmcp"
	"github.com/cocofhu/approving/internal/sendable"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// notifyFakeAdapter is a transport stub that always accepts and reports no
// channel message id.
type notifyFakeAdapter struct{}

func (notifyFakeAdapter) Type() string { return "qq" }

func (notifyFakeAdapter) Start(ctx context.Context, onInbound channels.InboundHandler) error {
	return nil
}

func (notifyFakeAdapter) Send(ctx context.Context, out channels.OutboundMessage) (channels.SendResult, error) {
	return channels.SendResult{}, nil
}

func (notifyFakeAdapter) Stop() error { return nil }

func notifyTestManager(t *testing.T) *channels.Manager {
	t.Helper()
	db, err := gorm.Open(
		sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Discard},
	)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	m := channels.NewManager(nil, map[string]channels.AdapterFactory{
		"qq": func(cfg channels.AdapterConfig) (channels.Adapter, error) {
			return notifyFakeAdapter{}, nil
		},
	}, func(s string) (string, error) { return s, nil })
	m.SetSendablePolicy(sendable.NewPolicy(db, nil))
	m.SetRetryBackoff(func(int) time.Duration { return 0 })
	return m
}

// TestChannelNotifierReportsSuppressionWithoutAnError pins the production seam
// between the channel Manager and pm_notify_progress: policy suppression must
// arrive as a structured outcome, and only an unreachable target is an error.
func TestChannelNotifierReportsSuppressionWithoutAnError(t *testing.T) {
	m := notifyTestManager(t)
	m.Apply([]models.ChannelConfig{{
		ID: "c1", Type: "qq", ProjectID: "proj", AppID: "app", Enabled: true,
		CronDeliver: true, CronDeliverTarget: "c2c:user1",
	}})
	defer m.StopAll()

	n := channelIMNotifier{mgr: m}
	target := pmmcp.IMTarget{Scene: "c2c", ConversationID: "user1", UserID: "u1"}

	outcome, err := n.NotifyProgress("proj", "run-1", target, "progress", "已提交分支", "已提交分支", "", false, false)
	if err != nil || !outcome.Sent {
		t.Fatalf("first progress = %+v err=%v want sent", outcome, err)
	}

	outcome, err = n.NotifyProgress("proj", "run-1", target, "progress", "已提交分支", "已提交分支", "", false, false)
	if err != nil {
		t.Fatalf("policy suppression must not surface as an error: %v", err)
	}
	if outcome.Sent || outcome.Reason == "" {
		t.Fatalf("suppressed progress = %+v want sent=false with a reason", outcome)
	}
}

func TestChannelNotifierReportsMissingTargetAsError(t *testing.T) {
	m := notifyTestManager(t)
	defer m.StopAll()

	n := channelIMNotifier{mgr: m}
	outcome, err := n.NotifyProgress("proj", "run-1",
		pmmcp.IMTarget{Scene: "c2c", ConversationID: "user1", UserID: "u1"},
		"progress", "已提交分支", "已提交分支", "", false, false)
	if !errors.Is(err, channels.ErrNoSendableTarget) {
		t.Fatalf("missing egress target = %+v err=%v want ErrNoSendableTarget", outcome, err)
	}
	if outcome.Sent {
		t.Fatalf("missing target reported a send: %+v", outcome)
	}
}
