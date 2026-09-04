package agent

import (
	"context"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/tools"
)

func TestSuccessfulAgenticToolUseUpdatesConversationRecency(t *testing.T) {
	store := tools.NewLoadedToolStore()
	ctx := tools.WithToolCatalogRuntime(context.Background(), tools.ToolCatalogRuntime{
		Store:          store,
		ConversationID: "conv-1",
		ProfileSlug:    "padrao",
	})
	recordRecentToolUsage(ctx, []tools.ToolExecutionResult{
		{ToolName: "read_file", Result: tools.ToolResult{Content: "ok"}},
		{ToolName: "write_file", Result: tools.ToolResult{IsError: true}},
		{ToolName: tools.ToolCatalogName, Result: tools.ToolResult{Content: "{}"}},
	})
	got := store.RecentNames("conv-1", "padrao")
	if len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("recência agêntica = %#v", got)
	}
}

func TestSelectedToolsFromCatalog(t *testing.T) {
	selected := selectedToolsFromCatalog([]tools.ToolExecutionResult{
		{
			ToolName: tools.ToolCatalogName,
			Result: tools.ToolResult{
				Content: `{"loaded_tools":["read_file","grep_search","read_file","tool_catalog"]}`,
			},
		},
	})

	if len(selected) != 2 || selected[0] != "read_file" || selected[1] != "grep_search" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestSelectedToolsFromCatalogIgnoresSearchResults(t *testing.T) {
	selected := selectedToolsFromCatalog([]tools.ToolExecutionResult{
		{
			ToolName: tools.ToolCatalogName,
			Result: tools.ToolResult{
				Content: `{"selected_tools":["read_file","grep_search"]}`,
			},
		},
	})

	if len(selected) != 0 {
		t.Fatalf("search results must not load tools, selected = %#v", selected)
	}
}

func TestUnloadedToolsFromCatalog(t *testing.T) {
	unloaded := unloadedToolsFromCatalog([]tools.ToolExecutionResult{
		{
			ToolName: tools.ToolCatalogName,
			Result: tools.ToolResult{
				Content: `{"unloaded_tools":["read_file","grep_search","read_file","tool_catalog"]}`,
			},
		},
	})

	if len(unloaded) != 2 || unloaded[0] != "read_file" || unloaded[1] != "grep_search" {
		t.Fatalf("unloaded = %#v", unloaded)
	}
}

func TestAppendUniqueToolDefs(t *testing.T) {
	existing := []llm.ToolDefinition{{Function: llm.FunctionDefinition{Name: tools.ToolCatalogName}}}
	result := appendUniqueToolDefs(existing,
		llm.ToolDefinition{Function: llm.FunctionDefinition{Name: "read_file"}},
		llm.ToolDefinition{Function: llm.FunctionDefinition{Name: tools.ToolCatalogName}},
	)

	if len(result) != 2 || result[1].Function.Name != "read_file" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExpandToolDefsFromCatalogResults(t *testing.T) {
	existing := []llm.ToolDefinition{{Function: llm.FunctionDefinition{Name: tools.ToolCatalogName}}}
	results := []tools.ToolExecutionResult{
		{
			ToolName: tools.ToolCatalogName,
			Result: tools.ToolResult{
				Content: `{"loaded_tools":["read_file","grep_search"]}`,
			},
		},
	}
	expanded := expandToolDefsFromCatalogResults(existing, results, func(active []llm.ToolDefinition, names []string) []llm.ToolDefinition {
		if len(names) != 2 || names[0] != "read_file" || names[1] != "grep_search" {
			t.Fatalf("resolver received names = %#v", names)
		}
		// O resolver agora devolve o conjunto ACUMULADO (ativas + novas).
		out := append([]llm.ToolDefinition{}, active...)
		return append(out,
			llm.ToolDefinition{Function: llm.FunctionDefinition{Name: "read_file"}},
			llm.ToolDefinition{Function: llm.FunctionDefinition{Name: "grep_search"}},
		)
	})
	if len(expanded) != 3 {
		t.Fatalf("expanded len = %d, want 3: %#v", len(expanded), expanded)
	}
	if expanded[0].Function.Name != tools.ToolCatalogName || expanded[1].Function.Name != "read_file" || expanded[2].Function.Name != "grep_search" {
		t.Fatalf("unexpected expanded defs: %#v", expanded)
	}
}

func TestExpandToolDefsFromCatalogResultsRemovesUnloaded(t *testing.T) {
	existing := []llm.ToolDefinition{
		{Function: llm.FunctionDefinition{Name: tools.ToolCatalogName}},
		{Function: llm.FunctionDefinition{Name: "read_file"}},
		{Function: llm.FunctionDefinition{Name: "grep_search"}},
	}
	results := []tools.ToolExecutionResult{
		{
			ToolName: tools.ToolCatalogName,
			Result: tools.ToolResult{
				Content: `{"unloaded_tools":["read_file"]}`,
			},
		},
	}
	expanded := expandToolDefsFromCatalogResults(existing, results, func(active []llm.ToolDefinition, names []string) []llm.ToolDefinition {
		if len(names) != 0 {
			t.Fatalf("resolver received names = %#v", names)
		}
		return active
	})
	if len(expanded) != 2 {
		t.Fatalf("expanded len = %d, want 2: %#v", len(expanded), expanded)
	}
	if expanded[0].Function.Name != tools.ToolCatalogName || expanded[1].Function.Name != "grep_search" {
		t.Fatalf("unexpected expanded defs: %#v", expanded)
	}
}
