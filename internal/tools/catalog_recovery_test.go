package tools

import (
	"context"
	"encoding/json"
	"errors"
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
