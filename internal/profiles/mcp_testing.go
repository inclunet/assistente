package profiles

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// TestMCPNativeSupport testa se um modelo suporta MCP nativo.
// Faz chamada real à API para verificar suporte.
// Retorna (suporta, mensagemErro, erro)
func TestMCPNativeSupport(ctx context.Context, apiKey, baseURL, modelID string) (bool, string, error) {
	log.Printf("[MCP Test] Testando suporte MCP nativo para modelo: %s", modelID)

	// Passo 1: Tentar obter capabilities via API (se disponível)
	if supports, err := queryModelCapabilities(ctx, apiKey, baseURL, modelID); err == nil {
		log.Printf("[MCP Test] Capabilities API retornou: %v", supports)
		return supports, "", nil
	}

	// Passo 2: Fazer teste real com chamada MCP
	// Criamos uma requisição mínima com MCP server fake
	testResult, errMsg, err := testMCPCall(ctx, apiKey, baseURL, modelID)
	
	if err != nil {
		log.Printf("[MCP Test] Erro no teste: %v", err)
		return false, errMsg, err
	}

	log.Printf("[MCP Test] Resultado do teste: %v", testResult)
	return testResult, errMsg, nil
}

// queryModelCapabilities tenta obter capabilities do modelo via API.
// Alguns provedores expõem endpoint /v1/models/{id} com capabilities.
func queryModelCapabilities(ctx context.Context, apiKey, baseURL, modelID string) (bool, error) {
	// Normalizar baseURL
	baseURL = strings.TrimSuffix(baseURL, "/")
	
	// Tentar endpoint comum: GET /v1/models/{model_id}
	url := fmt.Sprintf("%s/v1/models/%s", baseURL, modelID)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("status %d", resp.StatusCode)
	}
	
	// Tentar parsear resposta
	var modelInfo struct {
		Capabilities struct {
			MCP bool `json:"mcp"`
		} `json:"capabilities"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&modelInfo); err != nil {
		return false, err
	}
	
	return modelInfo.Capabilities.MCP, nil
}

// testMCPCall faz uma chamada de teste real com MCP server.
// Retorna (suporta, mensagemErro, erro)
func testMCPCall(ctx context.Context, apiKey, baseURL, modelID string) (bool, string, error) {
	// Normalizar baseURL
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/v1/chat/completions", baseURL)
	
	// Payload mínimo com MCP server fake
	// Se o modelo suportar MCP, aceita este campo
	// Se não suportar, retorna erro específico
	payload := map[string]any{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": "test"},
		},
		"max_tokens": 5, // Mínimo para economizar tokens
		"mcp_servers": []map[string]any{
			{
				"name":       "test-server",
				"endpoint":   "http://test.local",
				"capabilities": []string{"tools"},
			},
		},
	}
	
	body, err := json.Marshal(payload)
	if err != nil {
		return false, "", err
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return false, "", err
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("Erro de rede: %v", err), err
	}
	defer resp.Body.Close()
	
	respBody, _ := io.ReadAll(resp.Body)
	
	// Analisar resposta
	switch resp.StatusCode {
	case 200:
		// Sucesso! Modelo aceitou MCP servers
		log.Printf("[MCP Test] ✓ Modelo suporta MCP nativo (status 200)")
		return true, "", nil
		
	case 400:
		// Bad request - analisar mensagem de erro
		errMsg := string(respBody)
		
		// Padrões de erro que indicam MCP não suportado
		unsupportedPatterns := []string{
			"mcp_servers",
			"unknown field",
			"not supported",
			"invalid parameter",
			"unrecognized field",
		}
		
		errLower := strings.ToLower(errMsg)
		for _, pattern := range unsupportedPatterns {
			if strings.Contains(errLower, pattern) {
				log.Printf("[MCP Test] ✗ Modelo NÃO suporta MCP (erro: %s)", pattern)
				return false, fmt.Sprintf("MCP não suportado: %s", pattern), nil
			}
		}
		
		// Erro 400 mas não relacionado a MCP - pode ser outro problema
		log.Printf("[MCP Test] ? Erro 400 ambíguo: %s", errMsg)
		return false, fmt.Sprintf("Erro ambíguo: %s", errMsg), fmt.Errorf("teste inconclusivo")
		
	case 401:
		return false, "API key inválida", fmt.Errorf("unauthorized")
		
	case 404:
		return false, "Modelo não encontrado", fmt.Errorf("model not found")
		
	default:
		log.Printf("[MCP Test] ? Status inesperado %d: %s", resp.StatusCode, string(respBody))
		return false, fmt.Sprintf("Status %d", resp.StatusCode), fmt.Errorf("status inesperado: %d", resp.StatusCode)
	}
}

// TestMCPNativeSupportForProfile testa MCP nativo e salva resultado no perfil.
// Esta é a função principal a ser chamada pela UI ao configurar perfil.
func TestMCPNativeSupportForProfile(ctx context.Context, profile *Profile, apiKey, baseURL string) error {
	if profile.Chat.Model == "" {
		return fmt.Errorf("modelo não configurado no perfil")
	}
	
	// Se modo não for auto ou native, não precisa testar
	mode := profile.GetMCPMode()
	if mode == MCPModeAdapter {
		log.Printf("[MCP Test] Perfil usa modo adapter, teste não necessário")
		return nil
	}
	
	// Se já foi testado, não testa novamente (a menos que force)
	if profile.MCPNativeWasTested() {
		log.Printf("[MCP Test] Perfil já foi testado anteriormente")
		return nil
	}
	
	// Fazer teste
	supported, errMsg, err := TestMCPNativeSupport(ctx, apiKey, baseURL, profile.Chat.Model)
	
	if err != nil {
		// Se erro for de rede/auth, não marca como testado
		// Usuário pode querer tentar novamente
		return fmt.Errorf("erro ao testar MCP: %v - %s", err, errMsg)
	}
	
	// Salvar resultado no perfil
	profile.SetMCPNativeSupport(supported)
	
	if supported {
		log.Printf("[MCP Test] ✓ Modelo '%s' suporta MCP nativo!", profile.Chat.Model)
	} else {
		log.Printf("[MCP Test] ✗ Modelo '%s' NÃO suporta MCP nativo", profile.Chat.Model)
	}
	
	return nil
}

// ClearMCPTest limpa resultado do teste para forçar re-teste.
func (p *Profile) ClearMCPTest() {
	p.Chat.MCPNativeTested = nil
}
