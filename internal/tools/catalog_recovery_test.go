package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCatalogRecoveryOnlyPreparesAuthorizedLoad(t *testing.T) {
	for _, action := range []string{"search", "load", "denied", "failed"} {
		t.Run(action, func(t *testing.T) {
			entry := ToolCatalogEntry{Name: "remote_search", Origin: ToolOriginMCPBridge, AvailabilityStatus: ToolAvailabilityUnavailable}
			store := &fakeCatalogToolStore{entries: []ToolCatalogEntry{entry}}
			loads := NewLoadedToolStore()
			calls := 0
			runtime := ToolCatalogRuntime{Store: loads, ConversationID: "conv", ProfileSlug: "profile", VisibleNames: []string{entry.Name}, RecoverableNames: []string{entry.Name}, Prepare: func(context.Context, ToolCatalogEntry) error {
				calls++
				if action == "failed" {
					return errors.New("oauth denied")
				}
				return nil
			}}
			if action == "denied" {
				runtime.VisibleNames = []string{}
			}
			ctx := WithToolCatalogRuntime(WithToolCatalogVisibleNames(context.Background(), runtime.VisibleNames), runtime)
			req := catalogToolRequest{Action: "load", Tools: []string{entry.Name}}
			if action == "search" {
				req.Action = "search"
			}
			args, _ := json.Marshal(req)
			result, err := NewCatalogTool(store).Execute(ctx, args)
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := 0
			if action == "load" || action == "failed" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("prepare calls=%d want=%d", calls, wantCalls)
			}
			loaded := loads.Loaded("conv", "profile", nil)
			if (len(loaded) == 1) != (action == "load") {
				t.Fatalf("loaded=%v", loaded)
			}
			if action == "failed" && !result.IsError {
				t.Fatal("failed preparation reported success")
			}
		})
	}
}

func TestCatalogRecoverySearchBoundsDescriptions(t *testing.T) {
	entries := make([]ToolCatalogEntry, 50)
	for i := range entries {
		entries[i] = ToolCatalogEntry{Name: "tool", Description: strings.Repeat("Descrição útil. <>&\x00", 5000), AvailabilityStatus: ToolAvailabilityAvailable}
	}
	result, err := NewCatalogTool(&fakeCatalogToolStore{entries: entries}).Execute(context.Background(), json.RawMessage(`{"action":"search","limit":50}`))
	if err != nil || result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(result.Content) >= 102400 {
		t.Fatalf("search exceeded structured output limit: %d", len(result.Content))
	}
	var response catalogToolResponse
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		t.Fatal(err)
	}
	for _, entry := range response.Tools {
		if !entry.DescriptionTruncated || !utf8.ValidString(entry.Description) || len(entry.Description) > 1024 {
			t.Fatal("invalid abbreviated description")
		}
	}
}

func TestCatalogRecoverySearchBoundsWholePage(t *testing.T) {
	entries := make([]ToolCatalogEntry, 4)
	for i := range entries {
		entries[i] = ToolCatalogEntry{Name: fmt.Sprintf("tool%d_%s", i, strings.Repeat("n", 20000)), DisplayName: strings.Repeat("<", 3000), AvailabilityStatus: ToolAvailabilityAvailable}
	}
	tool := NewCatalogTool(&fakeCatalogToolStore{entries: entries})
	ctx := WithMaxResultSize(context.Background(), 80000)
	offset := 0
	for offset < len(entries) {
		args, _ := json.Marshal(catalogToolRequest{Action: "search", Query: "tool", Limit: 50, Offset: offset})
		result, err := tool.Execute(ctx, args)
		if err != nil || result.IsError || len(result.Content) > 80000 {
			t.Fatalf("invalid search response: error=%v isError=%v bytes=%d", err, result.IsError, len(result.Content))
		}
		var response catalogToolResponse
		if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
			t.Fatal(err)
		}
		if response.Count == 0 || response.Count != len(response.Tools) || response.Offset != offset {
			t.Fatal("invalid page count or offset")
		}
		for i, item := range response.Tools {
			if item.Name != entries[offset+i].Name {
				t.Fatal("search changed or skipped an identity")
			}
		}
		offset += response.Count
		if response.HasMore != (offset < len(entries)) || (response.HasMore && response.NextOffset != offset) {
			t.Fatal("invalid continuation")
		}
	}
	entries[0].Name = strings.Repeat("x", DefaultMaxResultSize)
	result, err := NewCatalogTool(&fakeCatalogToolStore{entries: entries[:1]}).Execute(context.Background(), json.RawMessage(`{"action":"search"}`))
	if err != nil || !result.IsError || len(result.Content) > DefaultMaxResultSize {
		t.Fatal("oversized identity must produce a bounded error")
	}
}
