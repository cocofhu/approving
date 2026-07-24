package services

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

type fakeDeliverer struct {
	calls   int
	project string
	text    string
}

func (f *fakeDeliverer) Deliver(projectID, text string) error {
	f.calls++
	f.project = projectID
	f.text = text
	return nil
}

func setupCronDeliver(t *testing.T) (*CronScheduler, *PmService, models.ChatThread, models.ChatMessage, string) {
	t.Helper()
	db := newTestDB(t)
	pm := NewPmService(db, nil)
	p, err := NewProjectService(db).Create("CronProj", "", nil, nil)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	thread, err := pm.CreateThread(p.ID, "cron-user", "T", "agent", models.ChatThreadKindUser)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	userMsg, err := pm.AppendMessage(thread.ID, "user", "run please", nil, nil, nil)
	if err != nil {
		t.Fatalf("append user: %v", err)
	}
	if _, err := pm.AppendMessage(thread.ID, "assistant", "done: result text", nil, nil, nil); err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	sched := NewCronScheduler(db, pm, nil, nil, CronTokenHooks{})
	return sched, pm, thread, userMsg, p.ID
}

func TestMaybeDeliverPushesWhenEnabled(t *testing.T) {
	sched, _, thread, userMsg, pid := setupCronDeliver(t)
	fd := &fakeDeliverer{}
	sched.SetChannelDeliverer(fd)

	job := &models.AgentCronJob{ID: "j1", ProjectID: pid, ThreadID: thread.ID, DeliverToChannel: true}
	sched.maybeDeliver(job, userMsg)

	if fd.calls != 1 {
		t.Fatalf("expected 1 delivery, got %d", fd.calls)
	}
	if fd.project != pid {
		t.Errorf("delivered to project %q want %q", fd.project, pid)
	}
	if fd.text != "done: result text" {
		t.Errorf("delivered text = %q", fd.text)
	}
}

func TestMaybeDeliverSkipsWhenDisabled(t *testing.T) {
	sched, _, thread, userMsg, pid := setupCronDeliver(t)
	fd := &fakeDeliverer{}
	sched.SetChannelDeliverer(fd)

	job := &models.AgentCronJob{ID: "j1", ProjectID: pid, ThreadID: thread.ID, DeliverToChannel: false}
	sched.maybeDeliver(job, userMsg)

	if fd.calls != 0 {
		t.Fatalf("expected no delivery when DeliverToChannel=false, got %d", fd.calls)
	}
}

func TestMaybeDeliverNoDelivererIsNoop(t *testing.T) {
	sched, _, thread, userMsg, pid := setupCronDeliver(t)
	job := &models.AgentCronJob{ID: "j1", ProjectID: pid, ThreadID: thread.ID, DeliverToChannel: true}
	// No deliverer set — must not panic.
	sched.maybeDeliver(job, userMsg)
}
