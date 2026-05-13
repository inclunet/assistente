package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCatalogEntryFromToolUsesDeterministicBuiltinMetadata(t *testing.T) {
	entry := CatalogEntryFromTool(&catalogMetadataTestTool{name: "search_conversations"})
	if entry.Category != "history" || entry.Package != "history" || entry.Class != "read_context" || entry.Risk != "read" {
		t.Fatalf("unexpected metadata for search_conversations: %#v", entry)
	}

	entry = CatalogEntryFromTool(&catalogMetadataTestTool{name: "grep_search"})
	if entry.Category != "filesystem" || entry.Package != "coding_readonly" {
		t.Fatalf("unexpected metadata for grep_search: %#v", entry)
	}
}

type catalogMetadataTestTool struct {
	name string
}

func (t *catalogMetadataTestTool) Name() string        { return t.name }
func (t *catalogMetadataTestTool) Description() string { return "test" }
func (t *catalogMetadataTestTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *catalogMetadataTestTool) Execute(context.Context, json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}
