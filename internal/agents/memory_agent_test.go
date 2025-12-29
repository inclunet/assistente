package agents

import (
	"encoding/json"
	"testing"

	"assistente/internal/memory"
)

// MockMemoryProvider implementa memory.Provider para testes
type MockMemoryProvider struct {
	memories map[uint]*memory.Data
	nextID   uint
}

func NewMockMemoryProvider() *MockMemoryProvider {
	return &MockMemoryProvider{
		memories: make(map[uint]*memory.Data),
		nextID:   1,
	}
}

func (m *MockMemoryProvider) Create(title, content, category string) (*memory.Data, error) {
	mem := &memory.Data{
		ID:       m.nextID,
		Title:    title,
		Content:  content,
		Category: category,
	}
	m.memories[m.nextID] = mem
	m.nextID++
	return mem, nil
}

func (m *MockMemoryProvider) GetAll() ([]memory.Data, error) {
	result := make([]memory.Data, 0, len(m.memories))
	for _, mem := range m.memories {
		result = append(result, *mem)
	}
	return result, nil
}

func (m *MockMemoryProvider) Search(query string) ([]memory.Data, error) {
	result := make([]memory.Data, 0)
	for _, mem := range m.memories {
		if contains(mem.Title, query) || contains(mem.Content, query) {
			result = append(result, *mem)
		}
	}
	return result, nil
}

func (m *MockMemoryProvider) Delete(id uint) error {
	delete(m.memories, id)
	return nil
}

// Helper para criar MemoryAgent para testes (sem LLM)
func newTestMemoryAgent(provider *MockMemoryProvider) *MemoryAgent {
	return NewMemoryAgent(provider, nil, "test-model")
}

func TestMemoryAgent_GetName(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	if agent.GetName() != "memory" {
		t.Errorf("Expected name 'memory', got '%s'", agent.GetName())
	}
}

func TestMemoryAgent_GetType(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	if agent.GetType() != "internal" {
		t.Errorf("Expected type 'internal', got '%s'", agent.GetType())
	}
}

func TestMemoryAgent_CanHandle(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	tests := []struct {
		toolName string
		expected bool
	}{
		{"memory_save", true},
		{"memory_search", true},
		{"memory_list", true},
		{"memory_delete", true},
		{"faq_create", false},
		{"other_tool", false},
	}

	for _, test := range tests {
		result := agent.CanHandle(test.toolName)
		if result != test.expected {
			t.Errorf("CanHandle(%s) = %v, expected %v", test.toolName, result, test.expected)
		}
	}
}

func TestMemoryAgent_GetTools(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	tools := agent.GetTools()
	if len(tools) != 4 {
		t.Errorf("Expected 4 tools, got %d", len(tools))
	}

	expectedTools := []string{"memory_save", "memory_search", "memory_list", "memory_delete"}
	for i, expectedName := range expectedTools {
		if tools[i].Function.Name != expectedName {
			t.Errorf("Tool %d: expected name '%s', got '%s'", i, expectedName, tools[i].Function.Name)
		}
	}
}

func TestMemoryAgent_Save(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	toolCall := ToolCall{
		ID:   "test-1",
		Type: "function",
		Function: FunctionCall{
			Name:      "memory_save",
			Arguments: `{"title": "Nome do usuário", "content": "Leonardo", "category": "core"}`,
		},
	}

	result, err := agent.ExecuteTool(toolCall)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Errorf("Expected success=true, got %v", response["success"])
	}

	if response["id"].(float64) != 1 {
		t.Errorf("Expected id=1, got %v", response["id"])
	}

	if response["category"] != "core" {
		t.Errorf("Expected category='core', got %v", response["category"])
	}
}

func TestMemoryAgent_SaveWithDefaultCategory(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	toolCall := ToolCall{
		ID:   "test-2",
		Type: "function",
		Function: FunctionCall{
			Name:      "memory_save",
			Arguments: `{"title": "Nota", "content": "Algo importante"}`,
		},
	}

	result, err := agent.ExecuteTool(toolCall)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["category"] != "geral" {
		t.Errorf("Expected default category='geral', got %v", response["category"])
	}
}

func TestMemoryAgent_Search(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	// Criar algumas memórias primeiro
	provider.Create("Nome", "Leonardo", "core")
	provider.Create("Projeto", "Assistente acessível", "projeto")

	toolCall := ToolCall{
		ID:   "test-3",
		Type: "function",
		Function: FunctionCall{
			Name:      "memory_search",
			Arguments: `{"query": "Leonardo"}`,
		},
	}

	result, err := agent.ExecuteTool(toolCall)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	count := response["count"].(float64)
	if count < 1 {
		t.Errorf("Expected at least 1 result, got %v", count)
	}
}

func TestMemoryAgent_List(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	// Criar algumas memórias
	provider.Create("Memória 1", "Conteúdo 1", "geral")
	provider.Create("Memória 2", "Conteúdo 2", "geral")

	toolCall := ToolCall{
		ID:   "test-4",
		Type: "function",
		Function: FunctionCall{
			Name:      "memory_list",
			Arguments: `{}`,
		},
	}

	result, err := agent.ExecuteTool(toolCall)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	count := response["count"].(float64)
	if count != 2 {
		t.Errorf("Expected 2 memories, got %v", count)
	}
}

func TestMemoryAgent_Delete(t *testing.T) {
	provider := NewMockMemoryProvider()
	agent := newTestMemoryAgent(provider)

	// Criar memória primeiro
	provider.Create("Para deletar", "Conteúdo", "geral")

	toolCall := ToolCall{
		ID:   "test-5",
		Type: "function",
		Function: FunctionCall{
			Name:      "memory_delete",
			Arguments: `{"id": 1}`,
		},
	}

	result, err := agent.ExecuteTool(toolCall)
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Errorf("Expected success=true, got %v", response["success"])
	}

	// Verificar se foi deletada
	memories, _ := provider.GetAll()
	if len(memories) != 0 {
		t.Errorf("Expected 0 memories after delete, got %d", len(memories))
	}
}
