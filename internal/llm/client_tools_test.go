package llm

import (
	"encoding/json"
	"testing"
)

// TestChatRequest_NoToolsWhenEmpty verifica que quando tools está vazio,
// o request JSON não inclui campos desnecessários
func TestChatRequest_NoToolsWhenEmpty(t *testing.T) {
	req := ChatRequest{
		Model:       "gpt-4",
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		Temperature: 0.7,
		MaxTokens:   2000,
		Tools:       nil, // nil para não aparecer no JSON
		ToolChoice:  nil, // nil para não aparecer no JSON
	}

	// Serializar para JSON
	jsonBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ChatRequest: %v", err)
	}

	// Deserializar para verificar
	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verificar que "tools" NÃO aparece quando é nil
	if tools, exists := decoded["tools"]; exists {
		t.Errorf("Expected 'tools' to be omitted when nil, but got: %v", tools)
	}

	// Verificar que "tool_choice" NÃO aparece quando é nil
	if toolChoice, exists := decoded["tool_choice"]; exists {
		t.Errorf("Expected 'tool_choice' to be omitted when nil, but got: %v", toolChoice)
	}

	t.Logf("JSON (no tools): %s", string(jsonBytes))
}

// TestChatRequest_WithTools verifica que quando tools é preenchida,
// o request JSON inclui "tools" e "tool_choice"
func TestChatRequest_WithTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
	}

	req := ChatRequest{
		Model:       "gpt-4",
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		Temperature: 0.7,
		MaxTokens:   2000,
		Tools:       tools,
		ToolChoice:  "auto",
	}

	// Serializar para JSON
	jsonBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ChatRequest: %v", err)
	}

	// Deserializar para verificar
	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verificar que "tools" aparece
	toolsField, exists := decoded["tools"]
	if !exists {
		t.Error("Expected 'tools' field in JSON, but it's missing")
	} else if toolsSlice, ok := toolsField.([]interface{}); ok {
		if len(toolsSlice) != 1 {
			t.Errorf("Expected 1 tool, got %d", len(toolsSlice))
		}
	} else {
		t.Errorf("Expected 'tools' to be an array, got: %T", toolsField)
	}

	// Verificar que "tool_choice" aparece
	toolChoice, exists := decoded["tool_choice"]
	if !exists || toolChoice != "auto" {
		t.Errorf("Expected 'tool_choice'='auto' in JSON, got: %v", toolChoice)
	}
}

// TestChatRequest_DisabledToolsPattern simula o padrão real de SendMessage
// onde DisableTools=true resulta em llmToolDefs vazio (nil)
func TestChatRequest_DisabledToolsPattern(t *testing.T) {
	// Cenário: DisableTools=true resultou em llmToolDefs nil (não slice vazio)
	var llmToolDefs []ToolDefinition // nil, não []

	req := ChatRequest{
		Model:       "gpt-4",
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		Temperature: 0.7,
		MaxTokens:   2000,
		Tools:       llmToolDefs, // nil
		ToolChoice:  nil,         // nil
	}

	// Serializar
	jsonBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Validar: não deve ter "tools" ou deve ser vazio, não deve ter "tool_choice"
	if _, toolsExists := decoded["tools"]; toolsExists {
		t.Error("'tools' field should be omitted when nil")
	}

	if _, toolChoiceExists := decoded["tool_choice"]; toolChoiceExists {
		t.Error("'tool_choice' field should be omitted when nil")
	}

	t.Logf("JSON with DisableTools=true (correct - no tools field):\n%s", string(jsonBytes))
}

// TestChatRequest_StructTags verifica que os struct tags estão corretos
// para garantir que campos vazios sejam omitidos no JSON
func TestChatRequest_StructTags(t *testing.T) {
	// Verificar manualmente que a struct usa "omitempty"
	type testStruct struct {
		Name       string `json:"name"`
		EmptyField string `json:"empty_field,omitempty"`
		Count      int    `json:"count,omitempty"`
	}

	ts := testStruct{
		Name:       "test",
		EmptyField: "",
		Count:      0,
	}

	jsonBytes, _ := json.Marshal(ts)
	var decoded map[string]interface{}
	_ = json.Unmarshal(jsonBytes, &decoded)

	// empty_field e count não devem aparecer
	if _, exists := decoded["empty_field"]; exists {
		t.Error("empty_field deveria ser omitido")
	}
	if _, exists := decoded["count"]; exists {
		t.Error("count deveria ser omitido")
	}

	// Se ChatRequest usa omitempty, campos vazios serão omitidos automaticamente
	// Isso é importante para que "Tools: []" não apareça como "tools": []
	t.Log("Reminder: ChatRequest struct must use '...omitempty' for optional fields")
}

// TestStreamChatRequest_NoToolsWhenDisabled simula o fluxo real de StreamChat()
// quando DisableTools=true
func TestStreamChatRequest_NoToolsWhenDisabled(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are a helper"},
		{Role: "user", Content: "Help me"},
	}

	// Simular DisableTools=true: llmToolDefs é nil (não slice vazio)
	var tools []ToolDefinition // nil
	var toolChoice interface{} // nil

	// Construir request como StreamChat faria
	body := ChatRequest{
		Model:       "gemini-2.0-flash",
		Messages:    messages,
		Temperature: 0.75,
		MaxTokens:   4096,
		Stream:      true,
		Tools:       tools,
		ToolChoice:  toolChoice,
	}

	// Validar
	if body.Tools != nil {
		t.Errorf("Expected Tools to be nil when DisableTools=true, got %v", body.Tools)
	}
	if body.ToolChoice != nil {
		t.Errorf("Expected ToolChoice to be nil when DisableTools=true, got %v", body.ToolChoice)
	}

	// Serializar e verificar JSON final
	jsonBytes, _ := json.Marshal(body)
	jsonStr := string(jsonBytes)

	var decoded map[string]interface{}
	_ = json.Unmarshal(jsonBytes, &decoded)

	// Verificações: "tools" e "tool_choice" não devem aparecer
	if _, exists := decoded["tools"]; exists {
		t.Error("'tools' should not appear in JSON when DisableTools=true")
	}
	if _, exists := decoded["tool_choice"]; exists {
		t.Error("'tool_choice' should not appear in JSON when DisableTools=true")
	}

	t.Logf("Generated JSON (DisableTools=true - correct):\n%s", jsonStr)
}

// BenchmarkChatRequestMarshal avalia performance de serialização
func BenchmarkChatRequestMarshal(b *testing.B) {
	req := ChatRequest{
		Model:       "gpt-4",
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		Temperature: 0.7,
		MaxTokens:   2000,
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "read_file",
					Description: "Read a file",
					Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
				},
			},
		},
		ToolChoice: "auto",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(req)
	}
}
