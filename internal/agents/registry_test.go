package agents

import (
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	faqProvider := NewMockFAQProvider()
	faqAgent := newTestFAQAgent(faqProvider)

	registry.Register(faqAgent)

	if len(registry.GetAll()) != 1 {
		t.Errorf("Expected 1 agent, got %d", len(registry.GetAll()))
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()

	faqProvider := NewMockFAQProvider()
	faqAgent := newTestFAQAgent(faqProvider)
	registry.Register(faqAgent)

	agent := registry.Get("faq")
	if agent == nil {
		t.Fatal("Expected to get faq agent, got nil")
	}

	if agent.GetName() != "faq" {
		t.Errorf("Expected agent name 'faq', got '%s'", agent.GetName())
	}

	// Testar agente inexistente
	notFound := registry.Get("nonexistent")
	if notFound != nil {
		t.Error("Expected nil for nonexistent agent")
	}
}

func TestRegistry_GetEnabled(t *testing.T) {
	registry := NewRegistry()

	faqProvider := NewMockFAQProvider()
	faqAgent := newTestFAQAgent(faqProvider)
	registry.Register(faqAgent)

	memoryProvider := NewMockMemoryProvider()
	memoryAgent := newTestMemoryAgent(memoryProvider)
	registry.Register(memoryAgent)

	enabled := registry.GetEnabled()
	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled agents, got %d", len(enabled))
	}
}

func TestRegistry_GetDelegationTools(t *testing.T) {
	registry := NewRegistry()

	faqProvider := NewMockFAQProvider()
	faqAgent := newTestFAQAgent(faqProvider)
	registry.Register(faqAgent)

	memoryProvider := NewMockMemoryProvider()
	memoryAgent := newTestMemoryAgent(memoryProvider)
	registry.Register(memoryAgent)

	tools := registry.GetDelegationTools()
	// Cada agente gera 1 tool de delegação
	if len(tools) != 2 {
		t.Errorf("Expected 2 delegation tools, got %d", len(tools))
	}

	// Verifica os nomes das tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Function.Name] = true
	}

	if !toolNames["delegate_to_faq"] {
		t.Error("Expected delegate_to_faq tool")
	}
	if !toolNames["delegate_to_memory"] {
		t.Error("Expected delegate_to_memory tool")
	}
}

func TestRegistry_GetAllTools(t *testing.T) {
	registry := NewRegistry()

	faqProvider := NewMockFAQProvider()
	faqAgent := newTestFAQAgent(faqProvider)
	registry.Register(faqAgent)

	memoryProvider := NewMockMemoryProvider()
	memoryAgent := newTestMemoryAgent(memoryProvider)
	registry.Register(memoryAgent)

	tools := registry.GetAllTools()
	// FAQ tem 6 tools, Memory tem 4 tools
	if len(tools) != 10 {
		t.Errorf("Expected 10 tools (6 FAQ + 4 Memory), got %d", len(tools))
	}
}

func TestRegistry_FindAgentForTool(t *testing.T) {
	registry := NewRegistry()

	faqProvider := NewMockFAQProvider()
	faqAgent := newTestFAQAgent(faqProvider)
	registry.Register(faqAgent)

	memoryProvider := NewMockMemoryProvider()
	memoryAgent := newTestMemoryAgent(memoryProvider)
	registry.Register(memoryAgent)

	// Testar busca por tool de FAQ
	agent := registry.FindAgentForTool("faq_create")
	if agent == nil {
		t.Fatal("Expected to find agent for faq_create")
	}
	if agent.GetName() != "faq" {
		t.Errorf("Expected agent 'faq', got '%s'", agent.GetName())
	}

	// Testar busca por tool de Memory
	agent = registry.FindAgentForTool("memory_save")
	if agent == nil {
		t.Fatal("Expected to find agent for memory_save")
	}
	if agent.GetName() != "memory" {
		t.Errorf("Expected agent 'memory', got '%s'", agent.GetName())
	}

	// Testar tool inexistente
	agent = registry.FindAgentForTool("unknown_tool")
	if agent != nil {
		t.Error("Expected nil for unknown tool")
	}
}

func TestRegistry_ExecuteTool(t *testing.T) {
	registry := NewRegistry()

	faqProvider := NewMockFAQProvider()
	faqAgent := newTestFAQAgent(faqProvider)
	registry.Register(faqAgent)

	toolCall := ToolCall{
		ID:   "test-1",
		Type: "function",
		Function: FunctionCall{
			Name:      "faq_create",
			Arguments: `{"question": "Teste?", "answer": "Resposta"}`,
		},
	}

	result, err := registry.ExecuteTool(toolCall)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	if result == "" {
		t.Error("Expected non-empty result")
	}
}

func TestRegistry_ExecuteTool_UnknownTool(t *testing.T) {
	registry := NewRegistry()

	toolCall := ToolCall{
		ID:   "test-1",
		Type: "function",
		Function: FunctionCall{
			Name:      "unknown_tool",
			Arguments: `{}`,
		},
	}

	_, err := registry.ExecuteTool(toolCall)
	if err == nil {
		t.Error("Expected error for unknown tool")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry()

	faqProvider := NewMockFAQProvider()
	faqAgent := newTestFAQAgent(faqProvider)
	registry.Register(faqAgent)

	if len(registry.GetAll()) != 1 {
		t.Fatal("Expected 1 agent before unregister")
	}

	registry.Unregister("faq")

	if len(registry.GetAll()) != 0 {
		t.Errorf("Expected 0 agents after unregister, got %d", len(registry.GetAll()))
	}
}
