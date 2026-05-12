package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeCatalogToolStore struct {
	entries []ToolCatalogEntry
	filter  ToolCatalogFilter
	err     error
}

func (s *fakeCatalogToolStore) ListTools(_ context.Context, filter ToolCatalogFilter) ([]ToolCatalogEntry, error) {
	s.filter = filter
	if s.err != nil {
		return nil, s.err
	}
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
	if store.filter.Limit != 1 {
		t.Fatalf("filter limit = %d, want 1", store.filter.Limit)
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

func TestCatalogToolClampsLimitToMaximum(t *testing.T) {
	store := &fakeCatalogToolStore{
		entries: []ToolCatalogEntry{
			{Name: "read_file", DisplayName: "read_file", Origin: ToolOriginBuiltin, AvailabilityStatus: ToolAvailabilityAvailable},
		},
	}
	tool := NewCatalogTool(store)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"limit":51}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if store.filter.Limit != 50 {
		t.Fatalf("filter limit = %d, want 50", store.filter.Limit)
	}
}

func TestCatalogToolErrorsArePortuguese(t *testing.T) {
	result, err := NewCatalogTool(nil).Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute nil store: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "catálogo de tools não configurado") {
		t.Fatalf("unexpected nil store result: %#v", result)
	}

	store := &fakeCatalogToolStore{err: errors.New("boom")}
	result, err = NewCatalogTool(store).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute store error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "erro ao consultar catálogo de tools") {
		t.Fatalf("unexpected store error result: %#v", result)
	}
}
