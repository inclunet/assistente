package usecases

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/tools"
)

type testTool struct {
	name string
}

func (t testTool) Name() string { return t.name }

func (t testTool) Description() string { return t.name }

func (t testTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (t testTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

func TestFilterExpandedToolNamesDropsOptInOnlyForDynamicProfile(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(testTool{name: "regular_tool"})
	registry.MustRegisterOptIn(testTool{name: "text_edit"})

	dynamic := filterExpandedToolNames(registry, []string{"regular_tool", "text_edit"}, nil, false)
	if len(dynamic) != 1 || dynamic[0] != "regular_tool" {
		t.Fatalf("dynamic expansion should drop opt-in tools, got %#v", dynamic)
	}

	explicit := filterExpandedToolNames(registry, []string{"regular_tool", "text_edit"}, []string{"text_edit"}, false)
	if len(explicit) != 1 || explicit[0] != "text_edit" {
		t.Fatalf("explicit enabled_tools should allow opt-in tools, got %#v", explicit)
	}
}
