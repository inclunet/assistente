package usecases

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/chat"
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

// A expansão dinâmica do use case delega agora a chat.ToolSelectionPolicy
// (AEP-0077 F3, #119). Este teste fixa a regra de opt-in via a API pública que
// o pipeline de envio consome: perfil sem tools fixas (enabled nil) descarta
// tools opt-in; perfil com allowlist explícito pode selecioná-las.
func TestDynamicExpansionDropsOptInOnlyForDynamicProfile(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(testTool{name: "regular_tool"})
	registry.MustRegisterOptIn(testTool{name: "text_edit"})
	policy := chat.NewToolSelectionPolicy(registry)

	names := []string{"regular_tool", "text_edit"}

	dynamic := policy.ResolveExpandedToolDefs(nil, nil, names, chat.ProfileToolConfig{EnabledTools: nil})
	if len(dynamic) != 1 || dynamic[0].Function.Name != "regular_tool" {
		t.Fatalf("dynamic expansion should drop opt-in tools, got %#v", dynamic)
	}

	explicit := policy.ResolveExpandedToolDefs(nil, nil, names, chat.ProfileToolConfig{EnabledTools: []string{"text_edit"}})
	if len(explicit) != 1 || explicit[0].Function.Name != "text_edit" {
		t.Fatalf("explicit enabled_tools should allow opt-in tools, got %#v", explicit)
	}
}
