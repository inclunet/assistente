package skillloader

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDescriptionExplainsSkillLoadingBoundariesAndCost(t *testing.T) {
	description := New(nil, nil).Description()
	normalized := strings.ToLower(description)
	for _, want := range []string{
		"use before acting",
		"do not use",
		"loading consumes context",
		"runtime executes it first",
		"example:",
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("Description should contain %q, got %q", want, description)
		}
	}
}

func TestParameterDescriptionsRequireCatalogIdentityAndSpecificReason(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(New(nil, nil).Parameters(), &schema); err != nil {
		t.Fatalf("Parameters returned invalid JSON: %v", err)
	}

	for _, want := range []struct {
		field string
		text  string
	}{
		{field: "skill", text: "shown in the current prompt catalog"},
		{field: "skill", text: "do not guess"},
		{field: "reason", text: "task-specific reason"},
		{field: "reason", text: "same batch"},
	} {
		if !strings.Contains(strings.ToLower(schema.Properties[want.field].Description), want.text) {
			t.Errorf("parameter %q should contain %q, got %q", want.field, want.text, schema.Properties[want.field].Description)
		}
	}
}
