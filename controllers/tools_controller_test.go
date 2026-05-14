package controllers

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/tools"
)

type testRegistryTool struct {
	name string
}

func (t testRegistryTool) Name() string { return t.name }

func (t testRegistryTool) Description() string { return t.name }

func (t testRegistryTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (t testRegistryTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

func TestGetAvailableToolsIncludesDiscoverableOptIn(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(testRegistryTool{name: "read_file"})
	registry.MustRegisterOptIn(testRegistryTool{name: "text_edit"})
	registry.MustRegisterDiscoverableOptIn(testRegistryTool{name: "job"})
	ctrl := NewToolsController(ToolsControllerConfig{ToolRegistry: registry})

	available := ctrl.GetAvailableTools()
	got := map[string]bool{}
	for _, tool := range available {
		got[tool.Name] = true
	}
	if !got["read_file"] || !got["job"] {
		t.Fatalf("available tools missing discoverable entries: %#v", available)
	}
	if got["text_edit"] {
		t.Fatalf("hidden opt-in tool should not be listed: %#v", available)
	}
}
