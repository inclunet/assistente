package questionnaire

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCollectResponsesDescriptionGuidesSelectionAndCost(t *testing.T) {
	description := NewCollectResponses(nil).Description()
	normalized := strings.ToLower(description)
	for _, want := range []string{
		"use when",
		"do not use",
		"pauses work for user input",
		"example:",
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("Description should contain %q, got %q", want, description)
		}
	}
}

func TestCollectResponsesParameterDescriptionsCoverQuestionContract(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
			Items       *struct {
				Properties map[string]struct {
					Description string `json:"description"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(NewCollectResponses(nil).Parameters(), &schema); err != nil {
		t.Fatalf("Parameters returned invalid JSON: %v", err)
	}

	for _, field := range []string{"title", "description", "allow_cancel", "submit_label", "cancel_label", "questions"} {
		if strings.TrimSpace(schema.Properties[field].Description) == "" {
			t.Errorf("top-level parameter %q should explain its use", field)
		}
	}
	for _, field := range []string{"title", "description", "submit_label", "cancel_label"} {
		if !strings.Contains(strings.ToLower(schema.Properties[field].Description), "user's language") {
			t.Errorf("user-facing parameter %q should require the user's language", field)
		}
	}
	questions := schema.Properties["questions"]
	if questions.Items == nil {
		t.Fatal("questions should retain an item schema")
	}
	for _, field := range []string{"id", "type", "prompt", "description", "content", "required", "options", "min", "max", "step", "placeholder", "default"} {
		if strings.TrimSpace(questions.Items.Properties[field].Description) == "" {
			t.Errorf("question parameter %q should explain its use", field)
		}
	}
	for _, field := range []string{"prompt", "description", "options", "placeholder", "default"} {
		if !strings.Contains(strings.ToLower(questions.Items.Properties[field].Description), "user's language") {
			t.Errorf("user-facing question parameter %q should require the user's language", field)
		}
	}
}
