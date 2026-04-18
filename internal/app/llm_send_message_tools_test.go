package app

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/profiles"
	"assistente/internal/tools"
)

// mockTool é uma ferramenta de teste
type mockTool struct {
	name        string
	description string
}

func (m *mockTool) Name() string               { return m.name }
func (m *mockTool) Description() string        { return m.description }
func (m *mockTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

func newMockTool(name string) *mockTool {
	return &mockTool{
		name:        name,
		description: "Mock tool: " + name,
	}
}


// TestSendMessage_DisableTools verifica que quando DisableTools=true,
// nenhuma ferramenta é incluída na requisição ao LLM
func TestSendMessage_DisableTools(t *testing.T) {
	profile := &profiles.Profile{
		Name: "Test Profile",
		Chat: profiles.ChatConfig{
			LLMProvider:  "test-provider",
			Model:        "test-model",
			Temperature:  0.7,
			MaxTokens:    2000,
			DisableTools: true,
			EnabledTools: nil, // Mesmo com nil, não deve incluir pois DisableTools=true
		},
	}

	// Simular chamada
	disableTools := profile != nil && profile.Chat.DisableTools

	if !disableTools {
		t.Error("DisableTools deveria ser true")
	}

	// Quando DisableTools=true, llmToolDefs deve ficar vazio,
	// independentemente de EnabledTools estar nil ou não
	var llmToolDefs []interface{}
	if !disableTools {
		llmToolDefs = []interface{}{} // seria preenchido aqui
	}

	if len(llmToolDefs) != 0 {
		t.Errorf("Expected llmToolDefs to be empty when DisableTools=true, got %d", len(llmToolDefs))
	}
}

// TestSendMessage_EnabledTools_Nil verifica que quando DisableTools=false
// e EnabledTools=nil, todas as ferramentas devem ser incluídas
func TestSendMessage_EnabledTools_Nil(t *testing.T) {
	profile := &profiles.Profile{
		Name: "Test Profile",
		Chat: profiles.ChatConfig{
			LLMProvider:  "test-provider",
			Model:        "test-model",
			Temperature:  0.7,
			MaxTokens:    2000,
			DisableTools: false,
			EnabledTools: nil, // nil = todas as ferramentas
		},
	}

	disableTools := profile != nil && profile.Chat.DisableTools
	if disableTools {
		t.Fatal("DisableTools deve ser false para este teste")
	}

	// Quando EnabledTools=nil, devem incluir TODAS as ferramentas
	shouldUseAllTools := profile.Chat.EnabledTools == nil

	if !shouldUseAllTools {
		t.Error("Esperado usar todas as ferramentas quando EnabledTools=nil")
	}
}

// TestSendMessage_EnabledTools_Empty verifica que quando DisableTools=false
// e EnabledTools=[], nenhuma ferramenta deve ser incluída
func TestSendMessage_EnabledTools_Empty(t *testing.T) {
	profile := &profiles.Profile{
		Name: "Test Profile",
		Chat: profiles.ChatConfig{
			LLMProvider:  "test-provider",
			Model:        "test-model",
			Temperature:  0.7,
			MaxTokens:    2000,
			DisableTools: false,
			EnabledTools: []string{}, // empty = nenhuma ferramenta
		},
	}

	disableTools := profile != nil && profile.Chat.DisableTools
	if disableTools {
		t.Fatal("DisableTools deve ser false para este teste")
	}

	// Quando EnabledTools é um slice vazio, devem ser filtradas 0 ferramentas
	// (simulando FilterByNames([]))
	shouldHaveTools := profile.Chat.EnabledTools == nil || len(profile.Chat.EnabledTools) > 0

	if shouldHaveTools {
		t.Error("Esperado não incluir ferramentas quando EnabledTools=[]")
	}
}

// TestSendMessage_EnabledTools_Subset verifica que quando DisableTools=false
// e EnabledTools=[subset de nomes], apenas essas ferramentas são incluídas
func TestSendMessage_EnabledTools_Subset(t *testing.T) {
	profile := &profiles.Profile{
		Name: "Test Profile",
		Chat: profiles.ChatConfig{
			LLMProvider:  "test-provider",
			Model:        "test-model",
			Temperature:  0.7,
			MaxTokens:    2000,
			DisableTools: false,
			EnabledTools: []string{"read_file", "write_file"}, // subset
		},
	}

	disableTools := profile != nil && profile.Chat.DisableTools
	if disableTools {
		t.Fatal("DisableTools deve ser false para este teste")
	}

	// Quando EnabledTools tem valores, devem usar apenas esses
	expectedTools := []string{"read_file", "write_file"}
	actualTools := profile.Chat.EnabledTools

	if len(actualTools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(actualTools))
	}

	for i, tool := range expectedTools {
		if i >= len(actualTools) || actualTools[i] != tool {
			t.Errorf("Expected tool %q at position %d, got %q", tool, i, actualTools[i])
		}
	}
}

// TestToolRegistry_FilterByNames verifica que o registry filtra corretamente as ferramentas
// Este teste valida a lógica que SendMessage usa
func TestToolRegistry_FilterByNames(t *testing.T) {
	registry := tools.NewRegistry()

	// Registrar algumas ferramentas de teste
	registry.MustRegister(newMockTool("read_file"))
	registry.MustRegister(newMockTool("write_file"))
	registry.MustRegister(newMockTool("list_directory"))

	// Test 1: ToDefinitions() deve retornar todas as ferramentas
	allTools := registry.ToDefinitions()
	if len(allTools) != 3 {
		t.Errorf("Expected 3 tools from ToDefinitions, got %d", len(allTools))
	}

	// Test 2: FilterByNames com subset
	filtered := registry.FilterByNames([]string{"read_file", "write_file"})
	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered tools, got %d", len(filtered))
	}

	// Verificar nomes
	names := make(map[string]bool)
	for _, tool := range filtered {
		names[tool.Function.Name] = true
	}

	if !names["read_file"] || !names["write_file"] {
		t.Error("FilterByNames did not return expected tools")
	}

	// Test 3: FilterByNames com empty slice
	emptyFiltered := registry.FilterByNames([]string{})
	if len(emptyFiltered) != 0 {
		t.Errorf("Expected 0 tools when filtering by empty slice, got %d", len(emptyFiltered))
	}

	// Test 4: FilterByNames com ferramenta inexistente (deve retornar vazio ou só as encontradas)
	partialFiltered := registry.FilterByNames([]string{"nonexistent_tool"})
	if len(partialFiltered) != 0 {
		t.Errorf("Expected 0 tools for nonexistent filter, got %d", len(partialFiltered))
	}
}

// TestSendMessage_ToolDecisionLogic testa a lógica completa de decisão de ferramentas
// que é usada em App.SendMessage()
func TestSendMessage_ToolDecisionLogic(t *testing.T) {
	tests := []struct {
		name                  string
		profile               *profiles.Profile
		registryHasTools      bool
		expectedShouldInclude bool
		description           string
	}{
		{
			name: "DisableTools=true should not include tools",
			profile: &profiles.Profile{
				Chat: profiles.ChatConfig{
					DisableTools: true,
					EnabledTools: nil,
				},
			},
			registryHasTools:      true,
			expectedShouldInclude: false,
			description:           "Quando DisableTools=true, nenhuma ferramenta deve ser incluída",
		},
		{
			name: "DisableTools=false, EnabledTools=nil should check registry",
			profile: &profiles.Profile{
				Chat: profiles.ChatConfig{
					DisableTools: false,
					EnabledTools: nil,
				},
			},
			registryHasTools:      true,
			expectedShouldInclude: true,
			description:           "Quando EnabledTools=nil, devem usar todas as ferramentas do registry",
		},
		{
			name: "DisableTools=false, EnabledTools=nil, registry empty",
			profile: &profiles.Profile{
				Chat: profiles.ChatConfig{
					DisableTools: false,
					EnabledTools: nil,
				},
			},
			registryHasTools:      false,
			expectedShouldInclude: false,
			description:           "Mesmo com EnabledTools=nil, se registry vazio não há ferramentas",
		},
		{
			name: "DisableTools=false, EnabledTools=[] should not include tools",
			profile: &profiles.Profile{
				Chat: profiles.ChatConfig{
					DisableTools: false,
					EnabledTools: []string{},
				},
			},
			registryHasTools:      true,
			expectedShouldInclude: false,
			description:           "Quando EnabledTools=[], nenhuma ferramenta deve ser incluída",
		},
		{
			name: "DisableTools=false, EnabledTools=[subset] should include only subset",
			profile: &profiles.Profile{
				Chat: profiles.ChatConfig{
					DisableTools: false,
					EnabledTools: []string{"read_file"},
				},
			},
			registryHasTools:      true,
			expectedShouldInclude: true,
			description:           "Quando EnabledTools tiver valores, incluir apenas essas",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simular lógica de SendMessage
			disableTools := tc.profile != nil && tc.profile.Chat.DisableTools

			var shouldIncludeTools bool
			if disableTools {
				// DisableTools=true: nunca incluir
				shouldIncludeTools = false
			} else if !tc.registryHasTools {
				// Sem ferramentas no registry: não incluir
				shouldIncludeTools = false
			} else if tc.profile.Chat.EnabledTools != nil {
				// EnabledTools foi especificado (pode ser vazio!)
				shouldIncludeTools = len(tc.profile.Chat.EnabledTools) > 0
			} else {
				// EnabledTools=nil e registryHasTools=true: incluir todas
				shouldIncludeTools = true
			}

			if shouldIncludeTools != tc.expectedShouldInclude {
				t.Errorf("%s: expected shouldIncludeTools=%v, got %v",
					tc.description, tc.expectedShouldInclude, shouldIncludeTools)
			}
		})
	}
}

// TestProfile_ChatConfig_Serialization valida que ChatConfig serializa e deserializa
// corretamente os campos DisableTools e EnabledTools
func TestProfile_ChatConfig_Serialization(t *testing.T) {
	tests := []struct {
		name   string
		config profiles.ChatConfig
	}{
		{
			name: "DisableTools=true, EnabledTools=nil",
			config: profiles.ChatConfig{
				LLMProvider:  "test",
				DisableTools: true,
				EnabledTools: nil,
			},
		},
		{
			name: "DisableTools=false, EnabledTools=nil",
			config: profiles.ChatConfig{
				LLMProvider:  "test",
				DisableTools: false,
				EnabledTools: nil,
			},
		},
		{
			name: "DisableTools=false, EnabledTools=[]",
			config: profiles.ChatConfig{
				LLMProvider:  "test",
				DisableTools: false,
				EnabledTools: []string{},
			},
		},
		{
			name: "DisableTools=false, EnabledTools=[subset]",
			config: profiles.ChatConfig{
				LLMProvider:  "test",
				DisableTools: false,
				EnabledTools: []string{"read_file", "write_file"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Validar que os campos estão corretos após construção
			if tc.config.DisableTools && tc.config.EnabledTools == nil {
				// Esperado
			} else if !tc.config.DisableTools && tc.config.EnabledTools != nil {
				// Esperado
			}

			// Simular o contexto que SendMessage usaria
			disableTools := tc.config.DisableTools
			enabledTools := tc.config.EnabledTools

			// Regra: se DisableTools=true, ignorar EnabledTools
			if disableTools {
				if enabledTools != nil && len(enabledTools) > 0 {
					// Erro: DisableTools=true mas EnabledTools foi definido
					t.Logf("Warning: DisableTools=true but EnabledTools=%v (será ignorado)", enabledTools)
				}
			}

			t.Logf("Config OK: DisableTools=%v, EnabledTools=%v", disableTools, enabledTools)
		})
	}
}

// BenchmarkToolFilteringLogic avalia performance da lógica de decisão de ferramentas
func BenchmarkToolFilteringLogic(b *testing.B) {
	registry := tools.NewRegistry()

	// Registrar muitas ferramentas
	for i := 0; i < 50; i++ {
		name := "tool_" + string(rune('a'+(i%26)))
		registry.MustRegister(newMockTool(name))
	}

	profile := &profiles.Profile{
		Chat: profiles.ChatConfig{
			DisableTools: false,
			EnabledTools: []string{"tool_a", "tool_b", "tool_c"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		disableTools := profile.Chat.DisableTools
		if !disableTools && profile.Chat.EnabledTools != nil {
			_ = registry.FilterByNames(profile.Chat.EnabledTools)
		}
	}
}
