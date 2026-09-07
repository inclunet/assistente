package tasklist

import (
	"strings"
	"testing"
)

func TestTasklistToolDescriptionsExposeDecisionGuidance(t *testing.T) {
	tests := []struct {
		name        string
		description string
		concepts    []string
	}{
		{
			name:        "task",
			description: NewTask(nil).Description(),
			concepts:    []string{"individual task", "task_list", "task_note", "database", "destructive", "example"},
		},
		{
			name:        "task_list",
			description: NewTaskList(nil).Description(),
			concepts:    []string{"container", "workflow", "task_note", "summary_only", "database", "example"},
		},
		{
			name:        "task_note",
			description: NewTaskNote(nil).Description(),
			concepts:    []string{"note/comment", "task's core fields", "memory", "database", "idempotent", "example"},
		},
		{
			name:        "update_plan",
			description: NewUpdatePlan(nil).Description(),
			concepts:    []string{"current conversation", "full snapshot", "task_note", "persisted", "omitted", "example"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDescriptionGuidance(t, test.description, test.concepts)
		})
	}
}

func assertDescriptionGuidance(t *testing.T, description string, concepts []string) {
	t.Helper()
	normalized := strings.ToLower(description)
	for _, section := range []string{"use when:", "do not use:", "risk", "cost"} {
		if !strings.Contains(normalized, section) {
			t.Errorf("description missing guidance section %q", section)
		}
	}
	for _, concept := range concepts {
		if !strings.Contains(normalized, strings.ToLower(concept)) {
			t.Errorf("description missing decision concept %q", concept)
		}
	}
}
