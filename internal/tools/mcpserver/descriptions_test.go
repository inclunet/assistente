package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDescriptionDistinguishesManagementFromToolDiscovery(t *testing.T) {
	description := New(&fakeManager{}).Description()
	normalized := strings.ToLower(description)
	for _, want := range []string{
		"use list/get/logs",
		"do not use this to discover or invoke",
		"start local processes",
		"permanently delete",
		"examples:",
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("Description should contain %q, got %q", want, description)
		}
	}
}

func TestParameterDescriptionsDocumentEveryManagementField(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(New(&fakeManager{}).Parameters(), &schema); err != nil {
		t.Fatalf("Parameters returned invalid JSON: %v", err)
	}

	for name, property := range schema.Properties {
		if strings.TrimSpace(property.Description) == "" {
			t.Errorf("parameter %q should document its use and constraints", name)
		}
	}
	for _, want := range []struct {
		field string
		text  string
	}{
		{field: "action", text: "prefer explicit actions for mutations"},
		{field: "env", text: "values may be secrets"},
		{field: "oauth2_scopes", text: "least privilege"},
		{field: "prefer_bridge", text: "future tool calls are routed"},
	} {
		if !strings.Contains(strings.ToLower(schema.Properties[want.field].Description), want.text) {
			t.Errorf("parameter %q should contain %q, got %q", want.field, want.text, schema.Properties[want.field].Description)
		}
	}
}
