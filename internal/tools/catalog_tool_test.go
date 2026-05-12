package tools

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeCatalogToolStore struct {
	entries []ToolCatalogEntry
	filter  ToolCatalogFilter
}

func (s *fakeCatalogToolStore) ListTools(_ context.Context, filter ToolCatalogFilter) ([]ToolCatalogEntry, error) {
	s.filter = filter
	return s.entries, nil
}

func TestCatalogToolReturnsSelectedTools(t *testing.T) {
	store := &fakeCatalogToolStore{
		entries: []ToolCatalogEntry{
			{Name: "read_file", DisplayName: "read_file", Description: "Read files", Origin: ToolOriginBuiltin, Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read", AvailabilityStatus: ToolAvailabilityAvailable},
			{Name: "grep_search", DisplayName: "grep_search", Description: "Search files", Origin: ToolOriginBuiltin, Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read", AvailabilityStatus: ToolAvailabilityAvailable},
		},
	}
	tool := NewCatalogTool(store)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"package":"coding_readonly","limit":1}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if store.filter.Package != "coding_readonly" {
		t.Fatalf("filter package = %q", store.filter.Package)
	}

	var payload struct {
		SelectedTools []string `json:"selected_tools"`
		Count         int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 1 || len(payload.SelectedTools) != 1 || payload.SelectedTools[0] != "read_file" {
		t.Fatalf("unexpected response: %#v", payload)
	}
}
