package usecases

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
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

func TestExecuteReservaConversaAntesDoPipeline(t *testing.T) {
	streamMgr := chat.NewStreamingManager(nil)
	uc := NewSendMessageUseCase(SendMessageConfig{
		StreamMgr: streamMgr,
		Emitter:   events.NoopEmitter{},
	})
	releaseDeletion, err := streamMgr.PrepareConversationDeletion([]string{"conversation-1"})
	if err != nil {
		t.Fatalf("PrepareConversationDeletion: %v", err)
	}

	finished := make(chan error, 1)
	go func() {
		_, executeErr := uc.Execute(SendMessageRequest{
			Ctx:            database.WithUserID(context.Background(), "user-1"),
			ConversationID: "conversation-1",
			Params:         llm.ChatParams{AllowAssistantPrefill: true},
		})
		finished <- executeErr
	}()
	select {
	case <-finished:
		t.Fatal("pipeline atravessou gate de exclusão antes de reservar a conversa")
	case <-time.After(25 * time.Millisecond):
	}

	releaseDeletion()
	select {
	case executeErr := <-finished:
		if executeErr == nil {
			t.Fatal("esperava erro da validação de prefill após liberar o gate")
		}
	case <-time.After(time.Second):
		t.Fatal("pipeline não prosseguiu após release")
	}
}

// A expansão dinâmica do use case delega agora a chat.ToolSelectionPolicy
// (AEP-0077 F3, #119). Este teste fixa a regra de opt-in via a API pública que
// o pipeline de envio consome: perfil sem tools fixas (enabled nil) descarta
// tools opt-in; perfil com allowlist explícito pode selecioná-las.
func TestDynamicExpansionDropsOptInOnlyForDynamicProfile(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(testTool{name: tools.ToolCatalogName})
	registry.MustRegister(testTool{name: "regular_tool"})
	registry.MustRegisterOptIn(testTool{name: "text_edit"})
	policy := chat.NewToolSelectionPolicy(registry)

	names := []string{"regular_tool", "text_edit"}

	dynamic := policy.ResolveExpandedToolDefs(nil, nil, nil, names, chat.ProfileToolConfig{EnabledTools: nil})
	if len(dynamic) != 1 || dynamic[0].Function.Name != "regular_tool" {
		t.Fatalf("dynamic expansion should drop opt-in tools, got %#v", dynamic)
	}

	explicit := policy.ResolveExpandedToolDefs(nil, nil, nil, names, chat.ProfileToolConfig{EnabledTools: []string{"text_edit"}})
	if len(explicit) != 1 || explicit[0].Function.Name != "text_edit" {
		t.Fatalf("explicit enabled_tools should allow opt-in tools, got %#v", explicit)
	}
}
