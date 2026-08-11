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
			name:     "completed kept when opted in",
			project:  models.ProjectNotifyPolicy{Enabled: boolPtr(true), DefaultEvents: []string{"waiting_human", "failed", "completed"}},
			workflow: models.WorkflowNotifyPolicy{Mode: "inherit"},
			want:     []string{"waiting_human", "failed", "completed"},
		},
		{
			name:     "custom completed only",
			project:  def,
			workflow: models.WorkflowNotifyPolicy{Mode: "custom", Events: []string{"completed"}},
			want:     []string{"completed"},
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

func TestNotifyPoliciesEqual_includesTemplates(t *testing.T) {
	base := models.ProjectNotifyPolicy{
		Enabled: boolPtr(true), DefaultEvents: []string{"waiting_human", "failed"},
	}
	if !notifyPoliciesEqual(base, base) {
		t.Fatal("identical should equal")
	}
	changed := base
	changed.WaitingHumanTemplate = "x"
	if notifyPoliciesEqual(base, changed) {
		t.Fatal("template-only change must not equal")
	}
	blank := base
	blank.WaitingHumanTemplate = "  \n"
	if !notifyPoliciesEqual(base, blank) {
		t.Fatal("whitespace-only template normalizes to empty")
	}
	completed := base
	completed.CompletedTemplate = "DONE {title}"
	if notifyPoliciesEqual(base, completed) {
		t.Fatal("completedTemplate-only change must not equal")
	}
}

func TestNormalizeProjectNotifyPolicy_trimsTemplates(t *testing.T) {
	got := NormalizeProjectNotifyPolicy(models.ProjectNotifyPolicy{
		WaitingHumanTemplate: "  hi  ",
		FailedTemplate:       "\n\t",
		CompletedTemplate:    " \n",
	})
	if got.WaitingHumanTemplate != "  hi  " {
		t.Fatalf("non-empty must keep surrounding spaces: %q", got.WaitingHumanTemplate)
	}
	if got.FailedTemplate != "" {
		t.Fatalf("failed=%q want empty", got.FailedTemplate)
	}
	if got.CompletedTemplate != "" {
		t.Fatalf("completed=%q want empty", got.CompletedTemplate)
	}
}
