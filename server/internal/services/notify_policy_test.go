package services

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func boolPtr(v bool) *bool { return &v }

func TestResolveNotifyEvents(t *testing.T) {
	def := models.DefaultProjectNotifyPolicy()
	cases := []struct {
		name     string
		project  models.ProjectNotifyPolicy
		workflow models.WorkflowNotifyPolicy
		want     []string
	}{
		{
			name:     "hard close ignores custom",
			project:  models.ProjectNotifyPolicy{Enabled: boolPtr(false), DefaultEvents: []string{"waiting_human", "failed"}},
			workflow: models.WorkflowNotifyPolicy{Mode: "custom", Events: []string{"waiting_human", "failed"}},
			want:     nil,
		},
		{
			name:     "mode off",
			project:  def,
			workflow: models.WorkflowNotifyPolicy{Mode: "off", Events: []string{"waiting_human"}},
			want:     nil,
		},
		{
			name:     "inherit defaults",
			project:  def,
			workflow: models.WorkflowNotifyPolicy{Mode: "inherit"},
			want:     []string{"waiting_human", "failed"},
		},
		{
			name:     "empty mode equals inherit",
			project:  def,
			workflow: models.WorkflowNotifyPolicy{},
			want:     []string{"waiting_human", "failed"},
		},
		{
			name:     "custom subset failed only",
			project:  def,
			workflow: models.WorkflowNotifyPolicy{Mode: "custom", Events: []string{"failed"}},
			want:     []string{"failed"},
		},
		{
			name:     "custom empty set",
			project:  def,
			workflow: models.WorkflowNotifyPolicy{Mode: "custom", Events: []string{}},
			want:     nil,
		},
		{
			name:     "completed stripped from inherit",
			project:  models.ProjectNotifyPolicy{Enabled: boolPtr(true), DefaultEvents: []string{"waiting_human", "failed", "completed"}},
			workflow: models.WorkflowNotifyPolicy{Mode: "inherit"},
			want:     []string{"waiting_human", "failed"},
		},
		{
			name:     "nil project defaults enabled with waiting+failed",
			project:  models.ProjectNotifyPolicy{},
			workflow: models.WorkflowNotifyPolicy{Mode: "inherit"},
			want:     []string{"waiting_human", "failed"},
		},
		{
			name:     "explicit empty project defaults",
			project:  models.ProjectNotifyPolicy{Enabled: boolPtr(true), DefaultEvents: []string{}},
			workflow: models.WorkflowNotifyPolicy{Mode: "inherit"},
			want:     nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveNotifyEvents(tc.project, tc.workflow)
			if !stringSlicesEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestNotifyEventAllowed(t *testing.T) {
	ev := []string{"waiting_human", "failed"}
	if !NotifyEventAllowed(ev, "waiting_human") || !NotifyEventAllowed(ev, "failed") {
		t.Fatal("expected allowed")
	}
	if NotifyEventAllowed(ev, "completed") || NotifyEventAllowed(nil, "failed") {
		t.Fatal("expected denied")
	}
}
