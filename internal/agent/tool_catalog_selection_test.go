package agent

import (
	"testing"

	"assistente/internal/llm"
	"assistente/internal/tools"
)

func TestSelectedToolsFromCatalog(t *testing.T) {
	selected := selectedToolsFromCatalog([]tools.ToolExecutionResult{
		{
			ToolName: tools.ToolCatalogName,
			Result: tools.ToolResult{
				Content: `{"selected_tools":["read_file","grep_search","read_file","tool_catalog"]}`,
			},
		},
	})

	if len(selected) != 2 || selected[0] != "read_file" || selected[1] != "grep_search" {
		t.Fatalf("selected = %#v", selected)
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
