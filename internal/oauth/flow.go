package oauth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"assistente/internal/llm"
)

// FlowManager gerencia fluxos OAuth completos
type FlowManager struct {
	callbackServer *CallbackServer
	httpClient     *http.Client
}

// NewFlowManager cria um novo gerenciador de fluxo OAuth
func NewFlowManager() *FlowManager {
	return &FlowManager{
		httpClient: llm.NewHTTPClientWithTimeout(30 * time.Second), // Usa pool compartilhado
	}
}

// StartAuthorizationFlow inicia o fluxo de autorização
func (m *FlowManager) StartAuthorizationFlow(providerID string, scopes []string) (string, error) {
	provider := GetProvider(providerID)
	if provider == nil {
		return "", fmt.Errorf("provider não encontrado: %s", providerID)
	}

	if !provider.IsConfigured() {
		return "", fmt.Errorf("provider %s não está configurado (faltam credenciais)", providerID)
	}

	// Inicia servidor de callback se não estiver rodando
	if m.callbackServer == nil {
		m.callbackServer = NewCallbackServer()
		if err := m.callbackServer.Start(); err != nil {
			return "", fmt.Errorf("erro ao iniciar servidor de callback: %w", err)
		}
	}

	// Gera state
	state := GenerateState()
	m.callbackServer.SetPendingAuth(providerID, state)

	// Combina scopes
	allScopes := append([]string{}, provider.DefaultScopes...)
	allScopes = append(allScopes, scopes...)

	// Monta URL de autorização
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", provider.GetClientID())
	params.Set("redirect_uri", m.callbackServer.GetRedirectURI())
	params.Set("state", state)
	if len(allScopes) > 0 {
		params.Set("scope", strings.Join(allScopes, " "))
	}

	// Parâmetros específicos por provider
	switch providerID {
	case "google":
		params.Set("access_type", "offline") // Para obter refresh_token
		params.Set("prompt", "consent")
	case "microsoft":
		params.Set("response_mode", "query")
	}

	authURL := provider.AuthorizeURL + "?" + params.Encode()
	return authURL, nil
}

// WaitForAuthorization aguarda o callback e troca o código por tokens
func (m *FlowManager) WaitForAuthorization(providerID string, timeout time.Duration) (*Token, error) {
	if m.callbackServer == nil {
		return nil, fmt.Errorf("fluxo de autorização não iniciado")
	}

	// Aguarda callback
	result, err := m.callbackServer.WaitForCallback(timeout)
	if err != nil {
		return nil, err
	}

	if result.Error != "" {
		return nil, fmt.Errorf("erro OAuth: %s", result.Error)
	}

	// Troca código por token
	return m.ExchangeCode(providerID, result.Code)
}

// ExchangeCode troca o código de autorização por tokens
func (m *FlowManager) ExchangeCode(providerID, code string) (*Token, error) {
	provider := GetProvider(providerID)
	if provider == nil {
		return nil, fmt.Errorf("provider não encontrado: %s", providerID)
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", m.callbackServer.GetRedirectURI())
	data.Set("client_id", provider.GetClientID())
	data.Set("client_secret", provider.GetClientSecret())

	req, err := http.NewRequest("POST", provider.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("erro OAuth: %s - %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf("erro HTTP %d: %s", resp.StatusCode, string(body))
	}

	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("erro ao parsear token: %w", err)
	}

	// Calcula expiração
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	} else {
		token.ExpiresAt = time.Now().Add(1 * time.Hour)
	}

	return &token, nil
}

// RefreshToken renova um token usando o refresh_token
func (m *FlowManager) RefreshToken(providerID, refreshToken string) (*Token, error) {
	provider := GetProvider(providerID)
	if provider == nil {
		return nil, fmt.Errorf("provider não encontrado: %s", providerID)
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", provider.GetClientID())
	data.Set("client_secret", provider.GetClientSecret())

	req, err := http.NewRequest("POST", provider.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro ao renovar token: %s", string(body))
	}

	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("erro ao parsear token: %w", err)
	}

	// Mantém o refresh_token se não veio um novo
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}

	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}

// Stop para o servidor de callback
func (m *FlowManager) Stop() {
	if m.callbackServer != nil {
		m.callbackServer.Stop()
		m.callbackServer = nil
	}
}






