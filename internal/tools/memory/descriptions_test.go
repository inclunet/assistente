package memory

import (
	"strings"
	"testing"
)

func TestMemoryDescriptionExplainsDurabilityAndContextCost(t *testing.T) {
	description := strings.ToLower(New(nil).Description())
	for _, section := range []string{"use when:", "do not use:", "risk", "cost"} {
		if !strings.Contains(description, section) {
			t.Errorf("description missing guidance section %q", section)
		}
	}
	for _, concept := range []string{
		"long-term memory",
		"search first",
		"task progress",
		"credentials",
		"persist",
		"prompt budget",
		"archive",
		"delete",
		"example",
	} {
		if !strings.Contains(description, concept) {
			t.Errorf("description missing decision concept %q", concept)
		}
	}
}
