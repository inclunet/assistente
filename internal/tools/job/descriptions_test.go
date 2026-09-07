package job

import (
	"strings"
	"testing"
)

func TestJobDescriptionsDistinguishExecutionFromGrouping(t *testing.T) {
	tests := []struct {
		name        string
		description string
		concepts    []string
	}{
		{
			name:        "job",
			description: NewJob(nil).Description(),
			concepts:    []string{"one configured tool", "job_pipeline", "database", "dry_run", "side effects", "example"},
		},
		{
			name:        "job_pipeline",
			description: NewPipeline(nil).Description(),
			concepts:    []string{"groups", "do not execute tools", "step order", "use job", "database", "example"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized := strings.ToLower(test.description)
			for _, section := range []string{"use when:", "do not use:", "risk", "cost"} {
				if !strings.Contains(normalized, section) {
					t.Errorf("description missing guidance section %q", section)
				}
			}
			for _, concept := range test.concepts {
				if !strings.Contains(normalized, strings.ToLower(concept)) {
					t.Errorf("description missing decision concept %q", concept)
				}
			}
		})
	}
}
