package profiles

import (
	"testing"
)

// TestProfileMCPMethods testa os métodos relacionados a MCP no Profile
func TestProfileMCPMethods(t *testing.T) {
	profile := &Profile{
		Chat: ChatConfig{
			Model:   "claude-3-7-sonnet",
			MCPMode: MCPModeAuto,
		},
	}

	// Testa estado inicial
	if profile.MCPNativeWasTested() {
		t.Error("MCPNativeWasTested deveria retornar false inicialmente")
	}

	// Testa SetMCPNativeSupport(true)
	profile.SetMCPNativeSupport(true)
	if !profile.MCPNativeWasTested() {
		t.Error("MCPNativeWasTested deveria retornar true após SetMCPNativeSupport")
	}
	if profile.Chat.MCPNativeTested == nil || !*profile.Chat.MCPNativeTested {
		t.Error("MCPNativeTested deveria ser true")
	}

	// Testa ShouldUseMCPNative com modo auto e suporte = true
	if !profile.ShouldUseMCPNative() {
		t.Error("ShouldUseMCPNative deveria retornar true com modo auto e suporte confirmado")
	}

	// Testa SetMCPNativeSupport(false)
	profile.SetMCPNativeSupport(false)
	if profile.Chat.MCPNativeTested == nil || *profile.Chat.MCPNativeTested {
		t.Error("MCPNativeTested deveria ser false")
	}
	if profile.ShouldUseMCPNative() {
		t.Error("ShouldUseMCPNative deveria retornar false quando não suporta")
	}

	// Testa ClearMCPTest
	profile.ClearMCPTest()
	if profile.MCPNativeWasTested() {
		t.Error("MCPNativeWasTested deveria retornar false após ClearMCPTest")
	}
	if profile.Chat.MCPNativeTested != nil {
		t.Error("MCPNativeTested deveria ser nil após ClearMCPTest")
	}
}

// TestGetMCPMode testa o método GetMCPMode
func TestGetMCPMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected string
	}{
		{"Modo adapter", MCPModeAdapter, MCPModeAdapter},
		{"Modo native", MCPModeNative, MCPModeNative},
		{"Modo auto", MCPModeAuto, MCPModeAuto},
		{"Modo vazio (default auto)", "", MCPModeAuto},
		{"Modo inválido (default auto)", "invalid", MCPModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &Profile{
				Chat: ChatConfig{
					MCPMode: tt.mode,
				},
			}
			result := profile.GetMCPMode()
			if result != tt.expected {
				t.Errorf("GetMCPMode() = %v, esperado %v", result, tt.expected)
			}
		})
	}
}

// TestShouldUseMCPNative testa lógica de decisão sobre usar MCP nativo
func TestShouldUseMCPNative(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		tested   *bool
		expected bool
	}{
		{"Modo adapter - sempre false", MCPModeAdapter, nil, false},
		{"Modo native - sempre true", MCPModeNative, nil, true},
		{"Modo auto - não testado - false", MCPModeAuto, nil, false},
		{"Modo auto - testado true", MCPModeAuto, boolPtr(true), true},
		{"Modo auto - testado false", MCPModeAuto, boolPtr(false), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &Profile{
				Chat: ChatConfig{
					MCPMode:         tt.mode,
					MCPNativeTested: tt.tested,
				},
			}
			result := profile.ShouldUseMCPNative()
			if result != tt.expected {
				t.Errorf("ShouldUseMCPNative() = %v, esperado %v", result, tt.expected)
			}
		})
	}
}

// TestModelSupportsNativeMCP testa a função deprecated de detecção por modelo
func TestModelSupportsNativeMCP(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"claude-3-7-sonnet-20250219", true},
		{"claude-3.7-sonnet", true},
		{"claude-4-opus", true},
		{"gpt-4o", false},
		{"gemini-pro", false},
		{"mistral-large", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := ModelSupportsNativeMCP(tt.model)
			if result != tt.expected {
				t.Errorf("ModelSupportsNativeMCP(%s) = %v, esperado %v", tt.model, result, tt.expected)
			}
		})
	}
}

// Helper para criar ponteiro bool
func boolPtr(b bool) *bool {
	return &b
}

// TestMCPProfileWorkflow testa o workflow completo de configuração MCP
func TestMCPProfileWorkflow(t *testing.T) {
	// Simula um perfil novo
	profile := &Profile{
		Chat: ChatConfig{
			Model:   "claude-3-7-sonnet",
			MCPMode: MCPModeAuto,
		},
	}

	// 1. Verifica estado inicial
	if profile.MCPNativeWasTested() {
		t.Fatal("Perfil novo não deveria ter sido testado")
	}
	if profile.ShouldUseMCPNative() {
		t.Fatal("Sem teste, não deveria usar MCP nativo")
	}

	// 2. Simula teste bem-sucedido
	profile.SetMCPNativeSupport(true)
	if !profile.MCPNativeWasTested() {
		t.Fatal("Após teste, deveria estar marcado como testado")
	}
	if !profile.ShouldUseMCPNative() {
		t.Fatal("Com suporte confirmado, deveria usar MCP nativo")
	}

	// 3. Testa modo forçado adapter
	profile.Chat.MCPMode = MCPModeAdapter
	if profile.ShouldUseMCPNative() {
		t.Fatal("Modo adapter não deveria usar MCP nativo, mesmo com suporte")
	}

	// 4. Testa modo forçado native
	profile.Chat.MCPMode = MCPModeNative
	if !profile.ShouldUseMCPNative() {
		t.Fatal("Modo native deveria usar MCP nativo sempre")
	}

	// 5. Testa clear e re-teste
	profile.Chat.MCPMode = MCPModeAuto
	profile.ClearMCPTest()
	if profile.MCPNativeWasTested() {
		t.Fatal("Após clear, não deveria estar testado")
	}

	// 6. Re-testa com resultado negativo
	profile.SetMCPNativeSupport(false)
	if profile.ShouldUseMCPNative() {
		t.Fatal("Sem suporte, não deveria usar MCP nativo")
	}
}

// TestMCPModeDefaultBehavior testa comportamento padrão sem configuração
func TestMCPModeDefaultBehavior(t *testing.T) {
	profile := &Profile{
		Chat: ChatConfig{
			Model: "gpt-4o",
			// MCPMode não especificado
		},
	}

	// Default deveria ser auto
	if profile.GetMCPMode() != MCPModeAuto {
		t.Errorf("Modo default deveria ser 'auto', got '%s'", profile.GetMCPMode())
	}

	// Sem teste, não deveria usar nativo
	if profile.ShouldUseMCPNative() {
		t.Error("Sem teste MCP, não deveria usar modo nativo")
	}
}

// TestMCPEdgeCases testa casos extremos e edge cases
func TestMCPEdgeCases(t *testing.T) {
	t.Run("Múltiplas chamadas SetMCPNativeSupport", func(t *testing.T) {
		profile := &Profile{Chat: ChatConfig{MCPMode: MCPModeAuto}}
		
		// Alterna várias vezes
		profile.SetMCPNativeSupport(true)
		if !*profile.Chat.MCPNativeTested {
			t.Error("Deveria ser true")
		}
		
		profile.SetMCPNativeSupport(false)
		if *profile.Chat.MCPNativeTested {
			t.Error("Deveria ser false")
		}
		
		profile.SetMCPNativeSupport(true)
		if !*profile.Chat.MCPNativeTested {
			t.Error("Deveria voltar a true")
		}
	})

	t.Run("ClearMCPTest múltiplas vezes", func(t *testing.T) {
		profile := &Profile{Chat: ChatConfig{MCPMode: MCPModeAuto}}
		
		profile.SetMCPNativeSupport(true)
		profile.ClearMCPTest()
		profile.ClearMCPTest() // Segunda vez não deveria causar erro
		
		if profile.MCPNativeWasTested() {
			t.Error("Deveria estar limpo")
		}
	})

	t.Run("Modo inválido com teste positivo", func(t *testing.T) {
		profile := &Profile{
			Chat: ChatConfig{
				MCPMode: "modo-invalido-xyz",
			},
		}
		
		// Modo inválido normaliza para auto
		if profile.GetMCPMode() != MCPModeAuto {
			t.Error("Modo inválido deveria normalizar para auto")
		}
		
		// Com auto e sem teste, não usa nativo
		if profile.ShouldUseMCPNative() {
			t.Error("Sem teste não deveria usar nativo")
		}
		
		// Adiciona teste positivo
		profile.SetMCPNativeSupport(true)
		if !profile.ShouldUseMCPNative() {
			t.Error("Com teste positivo deveria usar nativo")
		}
	})

	t.Run("Modo native ignora resultado do teste", func(t *testing.T) {
		profile := &Profile{
			Chat: ChatConfig{
				MCPMode: MCPModeNative,
			},
		}
		
		// Mesmo com teste negativo, deveria usar nativo (forçado)
		profile.SetMCPNativeSupport(false)
		if !profile.ShouldUseMCPNative() {
			t.Error("Modo native deveria ignorar teste e usar nativo sempre")
		}
	})

	t.Run("Modo adapter ignora resultado do teste", func(t *testing.T) {
		profile := &Profile{
			Chat: ChatConfig{
				MCPMode: MCPModeAdapter,
			},
		}
		
		// Mesmo com teste positivo, não deveria usar nativo (forçado adapter)
		profile.SetMCPNativeSupport(true)
		if profile.ShouldUseMCPNative() {
			t.Error("Modo adapter deveria ignorar teste e nunca usar nativo")
		}
	})
}

// TestMCPConstants valida que as constantes estão corretas
func TestMCPConstants(t *testing.T) {
	if MCPModeAdapter != "adapter" {
		t.Errorf("MCPModeAdapter = '%s', esperado 'adapter'", MCPModeAdapter)
	}
	if MCPModeNative != "native" {
		t.Errorf("MCPModeNative = '%s', esperado 'native'", MCPModeNative)
	}
	if MCPModeAuto != "auto" {
		t.Errorf("MCPModeAuto = '%s', esperado 'auto'", MCPModeAuto)
	}
}
