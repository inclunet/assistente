package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"assistente/internal/oauth"
)

// ==================== OAuth API ====================

// OAuthProviderInfo representa informações de um provider OAuth para a UI
type OAuthProviderInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Icon          string   `json:"icon"`
	IsConfigured  bool     `json:"is_configured"`
	DefaultScopes []string `json:"default_scopes"`
}

// OAuthConnectionInfo representa uma conexão OAuth para a UI
type OAuthConnectionInfo struct {
	ID           uint   `json:"id"`
	ProviderID   string `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	ProviderIcon string `json:"provider_icon"`
	UserEmail    string `json:"user_email"`
	UserName     string `json:"user_name"`
	Scopes       string `json:"scopes"`
	IsExpired    bool   `json:"is_expired"`
	ExpiresAt    string `json:"expires_at"`
	LastUsedAt   string `json:"last_used_at"`
	CreatedAt    string `json:"created_at"`
}

// Gerenciador de fluxo OAuth global
var oauthFlowManager *oauth.FlowManager

// GetOAuthProviders retorna todos os providers OAuth disponíveis
func (a *App) GetOAuthProviders() []OAuthProviderInfo {
	providers := oauth.GetAllProviders()
	result := make([]OAuthProviderInfo, 0, len(providers))

	for _, p := range providers {
		result = append(result, OAuthProviderInfo{
			ID:            p.ID,
			Name:          p.Name,
			Icon:          p.Icon,
			IsConfigured:  p.IsConfigured(),
			DefaultScopes: p.DefaultScopes,
		})
	}

	return result
}

// GetOAuthConnections retorna todas as conexões OAuth ativas
func (a *App) GetOAuthConnections() ([]OAuthConnectionInfo, error) {
	conns, err := a.GetAllOAuthConnections()
	if err != nil {
		return nil, err
	}

	result := make([]OAuthConnectionInfo, 0, len(conns))
	for _, conn := range conns {
		provider := oauth.GetProvider(conn.ProviderID)
		icon := "🔗"
		if provider != nil {
			icon = provider.Icon
		}

		result = append(result, OAuthConnectionInfo{
			ID:           conn.ID,
			ProviderID:   conn.ProviderID,
			ProviderName: conn.ProviderName,
			ProviderIcon: icon,
			UserEmail:    conn.UserEmail,
			UserName:     conn.UserName,
			Scopes:       conn.Scopes,
			IsExpired:    conn.IsExpired(),
			ExpiresAt:    conn.ExpiresAt.Format("02/01/2006 15:04"),
			LastUsedAt:   conn.LastUsedAt.Format("02/01/2006 15:04"),
			CreatedAt:    conn.CreatedAt.Format("02/01/2006 15:04"),
		})
	}

	return result, nil
}

// StartOAuthFlow inicia o fluxo de autorização OAuth
// Retorna a URL que deve ser aberta no navegador
func (a *App) StartOAuthFlow(providerID string, scopes []string) (string, error) {
	if oauthFlowManager == nil {
		oauthFlowManager = oauth.NewFlowManager()
	}

	authURL, err := oauthFlowManager.StartAuthorizationFlow(providerID, scopes)
	if err != nil {
		return "", err
	}

	fmt.Printf("🔐 [OAUTH] Iniciando fluxo para %s. URL: %s\n", providerID, authURL)
	return authURL, nil
}

// CompleteOAuthFlow aguarda o callback e finaliza a autorização
func (a *App) CompleteOAuthFlow(providerID string, timeoutSeconds int) (*OAuthConnectionInfo, error) {
	if oauthFlowManager == nil {
		return nil, fmt.Errorf("fluxo OAuth não iniciado")
	}

	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Minute // Default: 5 minutos
	}

	fmt.Printf("🔐 [OAUTH] Aguardando callback para %s (timeout: %v)...\n", providerID, timeout)

	token, err := oauthFlowManager.WaitForAuthorization(providerID, timeout)
	if err != nil {
		return nil, err
	}

	// Busca informações do usuário
	provider := oauth.GetProvider(providerID)
	userEmail := ""
	userName := ""
	userID := ""

	if provider != nil && provider.UserInfoURL != "" {
		userInfo, err := a.fetchUserInfo(provider.UserInfoURL, token.AccessToken)
		if err == nil {
			userEmail = userInfo["email"]
			userName = userInfo["name"]
			userID = userInfo["id"]
		}
	}

	// Salva a conexão
	conn, err := a.CreateOAuthConnection(
		providerID,
		provider.Name,
		userEmail,
		userName,
		userID,
		token.AccessToken,
		token.RefreshToken,
		token.TokenType,
		"", // TODO: salvar scopes
		token.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao salvar conexão: %w", err)
	}

	fmt.Printf("✅ [OAUTH] Conexão salva! Provider: %s, User: %s\n", providerID, userEmail)

	return &OAuthConnectionInfo{
		ID:           conn.ID,
		ProviderID:   conn.ProviderID,
		ProviderName: conn.ProviderName,
		ProviderIcon: provider.Icon,
		UserEmail:    conn.UserEmail,
		UserName:     conn.UserName,
		IsExpired:    false,
		ExpiresAt:    conn.ExpiresAt.Format("02/01/2006 15:04"),
		CreatedAt:    conn.CreatedAt.Format("02/01/2006 15:04"),
	}, nil
}

// fetchUserInfo busca informações do usuário
func (a *App) fetchUserInfo(url, accessToken string) (map[string]string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro HTTP %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	result := make(map[string]string)

	// Extrai campos comuns (diferentes providers usam nomes diferentes)
	if v, ok := data["email"].(string); ok {
		result["email"] = v
	}
	if v, ok := data["mail"].(string); ok { // Microsoft
		result["email"] = v
	}
	if v, ok := data["name"].(string); ok {
		result["name"] = v
	}
	if v, ok := data["displayName"].(string); ok { // Microsoft
		result["name"] = v
	}
	if v, ok := data["login"].(string); ok { // GitHub
		result["name"] = v
	}
	if v, ok := data["id"].(string); ok {
		result["id"] = v
	}
	if v, ok := data["sub"].(string); ok { // OpenID
		result["id"] = v
	}
	if v, ok := data["id"].(float64); ok {
		result["id"] = fmt.Sprintf("%.0f", v)
	}

	return result, nil
}

// RefreshOAuthConnection renova o token de uma conexão
func (a *App) RefreshOAuthConnection(connectionID uint) error {
	conn, err := a.GetOAuthConnection(connectionID)
	if err != nil {
		return err
	}

	if conn.RefreshToken == "" {
		return fmt.Errorf("conexão não possui refresh token")
	}

	if oauthFlowManager == nil {
		oauthFlowManager = oauth.NewFlowManager()
	}

	token, err := oauthFlowManager.RefreshToken(conn.ProviderID, conn.RefreshToken)
	if err != nil {
		return fmt.Errorf("erro ao renovar token: %w", err)
	}

	return a.UpdateOAuthTokens(connectionID, token.AccessToken, token.RefreshToken, token.ExpiresAt)
}

// DisconnectOAuth desconecta uma conta OAuth
func (a *App) DisconnectOAuth(connectionID uint) error {
	return a.DeleteOAuthConnection(connectionID)
}

// GetOAuthAccessToken retorna o access token de uma conexão (renovando se necessário)
func (a *App) GetOAuthAccessToken(connectionID uint) (string, error) {
	conn, err := a.GetOAuthConnection(connectionID)
	if err != nil {
		return "", err
	}

	// Renova se necessário
	if conn.NeedsRefresh() && conn.RefreshToken != "" {
		if err := a.RefreshOAuthConnection(connectionID); err != nil {
			// Se falhar ao renovar, tenta usar o token atual mesmo assim
			fmt.Printf("⚠️ [OAUTH] Erro ao renovar token: %v\n", err)
		} else {
			// Recarrega após renovação
			conn, err = a.GetOAuthConnection(connectionID)
			if err != nil {
				return "", err
			}
		}
	}

	// Atualiza último uso
	a.UpdateOAuthConnectionLastUsed(connectionID)

	return conn.AccessToken, nil
}

// GetOAuthAccessTokenForProvider retorna o access token da conexão mais recente de um provider
func (a *App) GetOAuthAccessTokenForProvider(providerID string) (string, error) {
	conn, err := a.GetActiveOAuthConnectionForProvider(providerID)
	if err != nil {
		return "", fmt.Errorf("nenhuma conexão ativa para %s", providerID)
	}

	return a.GetOAuthAccessToken(conn.ID)
}
