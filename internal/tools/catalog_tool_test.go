package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if store.filter.Limit != 2 {
		t.Fatalf("filter limit = %d, want 2", store.filter.Limit)
	}

	var payload struct {
		SelectedTools []string `json:"selected_tools"`
		Count         int      `json:"count"`
		Limit         int      `json:"limit"`
		Offset        int      `json:"offset"`
		HasMore       bool     `json:"has_more"`
		NextOffset    int      `json:"next_offset"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 1 || len(payload.SelectedTools) != 1 || payload.SelectedTools[0] != "read_file" {
		t.Fatalf("unexpected response: %#v", payload)
	}
	if payload.Limit != 1 || payload.Offset != 0 || !payload.HasMore || payload.NextOffset != 1 {
		t.Fatalf("unexpected pagination metadata: %#v", payload)
	}
}

func TestCatalogToolClampsReturnedPageToMaximum(t *testing.T) {
	entries := make([]ToolCatalogEntry, 0, 51)
	for i := 0; i < 51; i++ {
		name := fmt.Sprintf("tool_%03d", i)
		entries = append(entries, ToolCatalogEntry{
			Name:               name,
			DisplayName:        name,
			Origin:             ToolOriginBuiltin,
			AvailabilityStatus: ToolAvailabilityAvailable,
		})
	}
	store := &fakeCatalogToolStore{
		entries: entries,
	}
	tool := NewCatalogTool(store)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"limit":51}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if store.filter.Limit != 51 {
		t.Fatalf("filter limit = %d, want 51", store.filter.Limit)
	}

	var payload struct {
		SelectedTools []string `json:"selected_tools"`
		Count         int      `json:"count"`
		Limit         int      `json:"limit"`
		Offset        int      `json:"offset"`
		HasMore       bool     `json:"has_more"`
		NextOffset    int      `json:"next_offset"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 50 || len(payload.SelectedTools) != 50 {
		t.Fatalf("unexpected clamped page size: %#v", payload)
	}
	if payload.Limit != 50 || payload.Offset != 0 || !payload.HasMore || payload.NextOffset != 50 {
		t.Fatalf("unexpected pagination metadata: %#v", payload)
	}
}

func TestCatalogToolPaginatesAfterFirstPage(t *testing.T) {
	entries := make([]ToolCatalogEntry, 0, 51)
	for i := 50; i < 101; i++ {
		name := fmt.Sprintf("mcp_big__tool_%03d", i)
		entries = append(entries, ToolCatalogEntry{
			Name:               name,
			DisplayName:        name,
			Origin:             ToolOriginMCPBridge,
			Category:           "mcp:big",
			Class:              "mcp_tool",
			Package:            "mcp:big",
			Risk:               "network",
			AvailabilityStatus: ToolAvailabilityAvailable,
		})
	}
	store := &fakeCatalogToolStore{entries: entries}

	result, err := NewCatalogTool(store).Execute(context.Background(), json.RawMessage(`{"package":"mcp:big","limit":50,"offset":50}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if store.filter.Package != "mcp:big" || store.filter.Offset != 50 || store.filter.Limit != 51 {
		t.Fatalf("unexpected filter: %#v", store.filter)
	}

	var payload struct {
		SelectedTools []string `json:"selected_tools"`
		Count         int      `json:"count"`
		Limit         int      `json:"limit"`
		Offset        int      `json:"offset"`
		HasMore       bool     `json:"has_more"`
		NextOffset    int      `json:"next_offset"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 50 || len(payload.SelectedTools) != 50 {
		t.Fatalf("unexpected page size: %#v", payload)
	}
	if payload.Offset != 50 || payload.Limit != 50 || !payload.HasMore || payload.NextOffset != 100 {
		t.Fatalf("unexpected pagination metadata: %#v", payload)
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
