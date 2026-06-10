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

func TestToolCapabilityKind(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		// Builtins filesystem (read e write) → capability filesystem.
		{"read_file", ToolCapabilityFilesystem},
		{"write_file", ToolCapabilityFilesystem},
		{"grep_search", ToolCapabilityFilesystem},
		// Builtins de rede: categorias web e http → capability network.
		{"web_search", ToolCapabilityNetwork},
		{"web_fetch", ToolCapabilityNetwork},
		{"http_request", ToolCapabilityNetwork},
		{"feed_read", ToolCapabilityNetwork},
		// tool_catalog é meta-tool: não concede capacidade própria.
		{ToolCatalogName, ""},
		// Builtins sem capacidade específica → genérica ("").
		{"run_command", ""},
		{"search_conversations", ""},
		{"task_list", ""},
		// Nome desconhecido (tool dinâmica de servidor MCP) → MCP.
		{"mcp_some_server_tool", ToolCapabilityMCP},
		{"totalmente-inexistente", ToolCapabilityMCP},
	}
	for _, c := range cases {
		if got := ToolCapabilityKind(c.name); got != c.want {
			t.Errorf("ToolCapabilityKind(%q) = %q, quer %q", c.name, got, c.want)
		}
	}
}
