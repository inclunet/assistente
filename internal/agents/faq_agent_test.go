package agents

import (
	"encoding/json"
	"testing"

	"assistente/internal/faq"
)

// MockFAQProvider implementa faq.Provider para testes
type MockFAQProvider struct {
	faqs   map[uint]*faq.Data
	nextID uint
}

func NewMockFAQProvider() *MockFAQProvider {
	return &MockFAQProvider{
		faqs:   make(map[uint]*faq.Data),
		nextID: 1,
	}
}

func (m *MockFAQProvider) Create(question, answer, tags string) (*faq.Data, error) {
	f := &faq.Data{
		ID:       m.nextID,
		Question: question,
		Answer:   answer,
		Tags:     tags,
	}
	m.faqs[m.nextID] = f
	m.nextID++
	return f, nil
}

func (m *MockFAQProvider) Get(id uint) (*faq.Data, error) {
	if f, ok := m.faqs[id]; ok {
		return f, nil
	}
	return nil, nil
}

func (m *MockFAQProvider) GetAll() ([]faq.Data, error) {
	result := make([]faq.Data, 0, len(m.faqs))
	for _, f := range m.faqs {
		result = append(result, *f)
	}
	return result, nil
}

func (m *MockFAQProvider) Update(id uint, question, answer, tags string) (*faq.Data, error) {
	if f, ok := m.faqs[id]; ok {
		f.Question = question
		f.Answer = answer
		f.Tags = tags
		return f, nil
	}
	return nil, nil
}

func (m *MockFAQProvider) Delete(id uint) error {
	delete(m.faqs, id)
	return nil
}

func (m *MockFAQProvider) Search(query string) ([]faq.Data, error) {
	result := make([]faq.Data, 0)
	for _, f := range m.faqs {
		if contains(f.Question, query) || contains(f.Answer, query) {
			result = append(result, *f)
		}
	}
	return result, nil
}

func contains(s, substr string) bool {
	if len(s) == 0 || len(substr) == 0 {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper para criar FAQAgent para testes (sem LLM)
func newTestFAQAgent(provider *MockFAQProvider) *FAQAgent {
	return NewFAQAgent(provider, nil, "test-model")
}

func TestFAQAgent_GetName(t *testing.T) {
	provider := NewMockFAQProvider()
	agent := newTestFAQAgent(provider)

	if agent.GetName() != "faq" {
		t.Errorf("Expected name 'faq', got '%s'", agent.GetName())
	}
}

func TestFAQAgent_GetType(t *testing.T) {
	provider := NewMockFAQProvider()
	agent := newTestFAQAgent(provider)

	if agent.GetType() != "internal" {
		t.Errorf("Expected type 'internal', got '%s'", agent.GetType())
	}
}

func TestFAQAgent_CanHandle(t *testing.T) {
	provider := NewMockFAQProvider()
	agent := newTestFAQAgent(provider)

	tests := []struct {
		toolName string
		expected bool
	}{
		{"faq_create", true},
		{"faq_search", true},
		{"faq_list", true},
		{"faq_get", true},
		{"faq_update", true},
		{"faq_delete", true},
		{"memory_save", false},
		{"other_tool", false},
	}

	for _, test := range tests {
		result := agent.CanHandle(test.toolName)
		if result != test.expected {
			t.Errorf("CanHandle(%s) = %v, expected %v", test.toolName, result, test.expected)
		}
	}
}

func TestFAQAgent_GetTools(t *testing.T) {
	provider := NewMockFAQProvider()
	agent := newTestFAQAgent(provider)

	tools := agent.GetTools()
	if len(tools) != 6 {
		t.Errorf("Expected 6 tools, got %d", len(tools))
	}

	expectedTools := []string{"faq_create", "faq_search", "faq_list", "faq_get", "faq_update", "faq_delete"}
	for i, expectedName := range expectedTools {
		if tools[i].Function.Name != expectedName {
			t.Errorf("Tool %d: expected name '%s', got '%s'", i, expectedName, tools[i].Function.Name)
		}
	}
}

func TestFAQAgent_Create(t *testing.T) {
	provider := NewMockFAQProvider()
	agent := newTestFAQAgent(provider)

	toolCall := ToolCall{
		ID:   "test-1",
		Type: "function",
		Function: FunctionCall{
			Name:      "faq_create",
			Arguments: `{"question": "O que é Go?", "answer": "Uma linguagem de programação", "tags": "go,programação"}`,
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
}

func TestFAQAgent_Search(t *testing.T) {
	provider := NewMockFAQProvider()
	agent := newTestFAQAgent(provider)

	// Criar algumas FAQs primeiro
	provider.Create("O que é Go?", "Uma linguagem de programação", "go")
	provider.Create("O que é Python?", "Outra linguagem", "python")

	toolCall := ToolCall{
		ID:   "test-2",
		Type: "function",
		Function: FunctionCall{
			Name:      "faq_search",
			Arguments: `{"query": "Go"}`,
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

func TestFAQAgent_List(t *testing.T) {
	provider := NewMockFAQProvider()
	agent := newTestFAQAgent(provider)

	// Criar algumas FAQs
	provider.Create("FAQ 1", "Resposta 1", "")
	provider.Create("FAQ 2", "Resposta 2", "")

	toolCall := ToolCall{
		ID:   "test-3",
		Type: "function",
		Function: FunctionCall{
			Name:      "faq_list",
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
		t.Errorf("Expected 2 FAQs, got %v", count)
	}
}

func TestFAQAgent_Delete(t *testing.T) {
	provider := NewMockFAQProvider()
	agent := newTestFAQAgent(provider)

	// Criar FAQ primeiro
	provider.Create("FAQ para deletar", "Resposta", "")

	toolCall := ToolCall{
		ID:   "test-4",
		Type: "function",
		Function: FunctionCall{
			Name:      "faq_delete",
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
	faqs, _ := provider.GetAll()
	if len(faqs) != 0 {
		t.Errorf("Expected 0 FAQs after delete, got %d", len(faqs))
	}
}
