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
	calls   int
}

func (s *fakeCatalogToolStore) ListTools(_ context.Context, filter ToolCatalogFilter) ([]ToolCatalogEntry, error) {
	s.calls++
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
		Tools      []catalogToolItem `json:"tools"`
		Count      int               `json:"count"`
		Limit      int               `json:"limit"`
		Offset     int               `json:"offset"`
		HasMore    bool              `json:"has_more"`
		NextOffset int               `json:"next_offset"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 1 || len(payload.Tools) != 1 || payload.Tools[0].Name != "read_file" {
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
		Tools      []catalogToolItem `json:"tools"`
		Count      int               `json:"count"`
		Limit      int               `json:"limit"`
		Offset     int               `json:"offset"`
		HasMore    bool              `json:"has_more"`
		NextOffset int               `json:"next_offset"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 50 || len(payload.Tools) != 50 {
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
		Tools      []catalogToolItem `json:"tools"`
		Count      int               `json:"count"`
		Limit      int               `json:"limit"`
		Offset     int               `json:"offset"`
		HasMore    bool              `json:"has_more"`
		NextOffset int               `json:"next_offset"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 50 || len(payload.Tools) != 50 {
		t.Fatalf("unexpected page size: %#v", payload)
	}
	if payload.Offset != 50 || payload.Limit != 50 || !payload.HasMore || payload.NextOffset != 100 {
		t.Fatalf("unexpected pagination metadata: %#v", payload)
	}
}

func TestCatalogToolFiltersByProfileVisibleNames(t *testing.T) {
	store := &fakeCatalogToolStore{
		entries: []ToolCatalogEntry{
			{Name: "read_file", DisplayName: "read_file", Origin: ToolOriginBuiltin, AvailabilityStatus: ToolAvailabilityAvailable},
			{Name: "write_file", DisplayName: "write_file", Origin: ToolOriginBuiltin, AvailabilityStatus: ToolAvailabilityAvailable},
		},
	}
	ctx := WithToolCatalogVisibleNames(context.Background(), []string{"read_file"})

	result, err := NewCatalogTool(store).Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if len(store.filter.NameIn) != 1 || store.filter.NameIn[0] != "read_file" {
		t.Fatalf("expected NameIn filter from profile visibility, got %#v", store.filter.NameIn)
	}

	var payload struct {
		Tools []catalogToolItem `json:"tools"`
		Count int               `json:"count"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 1 || len(payload.Tools) != 1 || payload.Tools[0].Name != "read_file" {
		t.Fatalf("unexpected filtered response: %#v", payload)
	}
}

func TestCatalogToolEmptyVisibleNamesReturnsNoTools(t *testing.T) {
	store := &fakeCatalogToolStore{
		entries: []ToolCatalogEntry{
			{Name: "read_file", DisplayName: "read_file", Origin: ToolOriginBuiltin, AvailabilityStatus: ToolAvailabilityAvailable},
		},
	}
	ctx := WithToolCatalogVisibleNames(context.Background(), []string{})

	result, err := NewCatalogTool(store).Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if store.calls != 0 {
		t.Fatalf("store não deveria ser consultado sem tools visíveis, calls=%d", store.calls)
	}

	var payload struct {
		SelectedTools []string `json:"selected_tools"`
		Count         int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 0 || len(payload.SelectedTools) != 0 {
		t.Fatalf("expected no visible tools, got %#v", payload)
	}
}

func TestCatalogToolLoadPersistsRuntimeTool(t *testing.T) {
	store := &fakeCatalogToolStore{
		entries: []ToolCatalogEntry{
			{Name: "read_file", DisplayName: "read_file", Origin: ToolOriginBuiltin, AvailabilityStatus: ToolAvailabilityAvailable},
		},
	}
	loadedStore := NewLoadedToolStore()
	ctx := WithToolCatalogRuntime(context.Background(), ToolCatalogRuntime{
		Store:          loadedStore,
		ConversationID: "conv-1",
		ProfileSlug:    "padrao",
		VisibleNames:   []string{"read_file", ToolCatalogName},
		PreloadedNames: []string{ToolCatalogName},
		ControlPlane:   []string{ToolCatalogName},
	})

	result, err := NewCatalogTool(store).Execute(ctx, json.RawMessage(`{"action":"load","tools":["read_file"]}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	var payload struct {
		LoadedTools   []string `json:"loaded_tools"`
		SelectedTools []string `json:"selected_tools"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.LoadedTools) != 1 || payload.LoadedTools[0] != "read_file" {
		t.Fatalf("unexpected loaded tools: %#v", payload)
	}
	if got := loadedStore.Loaded("conv-1", "padrao", []string{"read_file"}); len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("runtime store did not persist loaded tool: %#v", got)
	}
}

func TestCatalogToolLoadRejectsDisabledByProfile(t *testing.T) {
	store := &fakeCatalogToolStore{
		entries: []ToolCatalogEntry{
			{Name: "write_file", DisplayName: "write_file", Origin: ToolOriginBuiltin, AvailabilityStatus: ToolAvailabilityAvailable},
		},
	}
	ctx := WithToolCatalogRuntime(context.Background(), ToolCatalogRuntime{
		Store:          NewLoadedToolStore(),
		ConversationID: "conv-1",
		ProfileSlug:    "padrao",
		VisibleNames:   []string{"read_file"},
	})

	result, err := NewCatalogTool(store).Execute(ctx, json.RawMessage(`{"action":"load","tools":["write_file"]}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	var payload struct {
		LoadedTools   []string `json:"loaded_tools"`
		RejectedTools []struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		} `json:"rejected_tools"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.LoadedTools) != 0 || len(payload.RejectedTools) != 1 || payload.RejectedTools[0].Reason != LoadedToolRejectDisabled {
		t.Fatalf("unexpected load response: %#v", payload)
	}
}

func TestCatalogToolListAndUnloadLoadedTools(t *testing.T) {
	loadedStore := NewLoadedToolStore()
	loadedStore.Load("conv-1", "padrao", []string{"read_file"}, []string{"read_file"}, []string{ToolCatalogName}, []string{ToolCatalogName})
	ctx := WithToolCatalogRuntime(context.Background(), ToolCatalogRuntime{
		Store:          loadedStore,
		ConversationID: "conv-1",
		ProfileSlug:    "padrao",
		VisibleNames:   []string{"read_file", ToolCatalogName},
		PreloadedNames: []string{ToolCatalogName},
		ControlPlane:   []string{ToolCatalogName},
	})

	result, err := NewCatalogTool(&fakeCatalogToolStore{}).Execute(ctx, json.RawMessage(`{"action":"list_loaded"}`))
	if err != nil {
		t.Fatalf("Execute list_loaded: %v", err)
	}
	var listPayload struct {
		Loaded []LoadedToolRecord `json:"loaded"`
	}
	if err := json.Unmarshal([]byte(result.Content), &listPayload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listPayload.Loaded) != 2 || listPayload.Loaded[0].State != LoadedToolStateControlPlane || listPayload.Loaded[1].State != LoadedToolStateOnDemand {
		t.Fatalf("unexpected loaded list: %#v", listPayload.Loaded)
	}

	result, err = NewCatalogTool(&fakeCatalogToolStore{}).Execute(ctx, json.RawMessage(`{"action":"unload","tools":["read_file","tool_catalog"]}`))
	if err != nil {
		t.Fatalf("Execute unload: %v", err)
	}
	var unloadPayload struct {
		UnloadedTools []string `json:"unloaded_tools"`
		RejectedTools []struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		} `json:"rejected_tools"`
	}
	if err := json.Unmarshal([]byte(result.Content), &unloadPayload); err != nil {
		t.Fatalf("decode unload response: %v", err)
	}
	if len(unloadPayload.UnloadedTools) != 1 || unloadPayload.UnloadedTools[0] != "read_file" {
		t.Fatalf("unexpected unloaded tools: %#v", unloadPayload)
	}
	if len(unloadPayload.RejectedTools) != 1 || unloadPayload.RejectedTools[0].Reason != LoadedToolRejectControlPlane {
		t.Fatalf("expected control-plane rejection, got %#v", unloadPayload.RejectedTools)
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
