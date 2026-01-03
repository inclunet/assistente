package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"assistente/internal/llm"
)

// Client gerencia autenticação OAuth 2.0
type Client struct {
	config     Config
	envVars    map[string]string
	httpClient *http.Client

	// Cache de token
	mu          sync.RWMutex
	cachedToken *Token
}

// NewClient cria um novo cliente OAuth 2.0
func NewClient(config Config, envVars map[string]string) *Client {
	return &Client{
		config:     config,
		envVars:    envVars,
		httpClient: llm.NewHTTPClientWithTimeout(30 * time.Second), // Usa pool compartilhado
	}
}

// GetAccessToken obtém um access token (do cache ou renovando)
func (c *Client) GetAccessToken(ctx context.Context) (string, error) {
	// Verifica cache
	c.mu.RLock()
	if c.cachedToken != nil && !c.cachedToken.IsExpired() {
		token := c.cachedToken.AccessToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	// Precisa obter novo token
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check após obter lock
	if c.cachedToken != nil && !c.cachedToken.IsExpired() {
		return c.cachedToken.AccessToken, nil
	}

	// Tenta refresh se tiver refresh_token
	if c.cachedToken != nil && c.cachedToken.RefreshToken != "" {
		token, err := c.refreshToken(ctx, c.cachedToken.RefreshToken)
		if err == nil {
			c.cachedToken = token
			return token.AccessToken, nil
		}
		// Se falhar, tenta obter novo token
	}

	// Obtém novo token baseado no grant type
	var token *Token
	var err error

	switch c.config.GrantType {
	case "client_credentials":
		token, err = c.clientCredentialsGrant(ctx)
	case "authorization_code":
		// Não suportado diretamente (requer interação do usuário)
		return "", fmt.Errorf("grant_type 'authorization_code' requer autenticação interativa")
	default:
		return "", fmt.Errorf("grant_type não suportado: %s", c.config.GrantType)
	}

	if err != nil {
		return "", err
	}

	c.cachedToken = token
	return token.AccessToken, nil
}

// clientCredentialsGrant implementa o Client Credentials grant
func (c *Client) clientCredentialsGrant(ctx context.Context) (*Token, error) {
	clientID := c.resolveValue(c.config.ClientID, c.config.ClientIDEnv)
	clientSecret := c.resolveValue(c.config.ClientSecret, c.config.ClientSecretEnv)

	if clientID == "" {
		return nil, fmt.Errorf("client_id não configurado")
	}

	// Prepara o body
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	if len(c.config.Scopes) > 0 {
		data.Set("scope", strings.Join(c.config.Scopes, " "))
	}

	if c.config.Audience != "" {
		data.Set("audience", c.config.Audience)
	}

	// Alguns servidores requerem credenciais no body
	if c.config.SendCredentialsInBody {
		data.Set("client_id", clientID)
		data.Set("client_secret", clientSecret)
	}

	// Cria a requisição
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Adiciona Basic auth se não enviar no body
	if !c.config.SendCredentialsInBody {
		auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
		req.Header.Set("Authorization", "Basic "+auth)
	}

	return c.doTokenRequest(req)
}

// refreshToken renova um token usando refresh_token
func (c *Client) refreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	clientID := c.resolveValue(c.config.ClientID, c.config.ClientIDEnv)
	clientSecret := c.resolveValue(c.config.ClientSecret, c.config.ClientSecretEnv)

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	if c.config.SendCredentialsInBody {
		data.Set("client_id", clientID)
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if !c.config.SendCredentialsInBody && clientSecret != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
		req.Header.Set("Authorization", "Basic "+auth)
	}

	return c.doTokenRequest(req)
}

// ExchangeAuthorizationCode troca um código de autorização por tokens
func (c *Client) ExchangeAuthorizationCode(ctx context.Context, code, redirectURI string) (*Token, error) {
	clientID := c.resolveValue(c.config.ClientID, c.config.ClientIDEnv)
	clientSecret := c.resolveValue(c.config.ClientSecret, c.config.ClientSecretEnv)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	if c.config.SendCredentialsInBody {
		data.Set("client_id", clientID)
		if clientSecret != "" {
			data.Set("client_secret", clientSecret)
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if !c.config.SendCredentialsInBody && clientSecret != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
		req.Header.Set("Authorization", "Basic "+auth)
	}

	token, err := c.doTokenRequest(req)
	if err != nil {
		return nil, err
	}

	// Salva no cache
	c.mu.Lock()
	c.cachedToken = token
	c.mu.Unlock()

	return token, nil
}

// GetAuthorizationURL retorna a URL para iniciar o fluxo de autorização
func (c *Client) GetAuthorizationURL(redirectURI, state string) string {
	clientID := c.resolveValue(c.config.ClientID, c.config.ClientIDEnv)

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)

	if len(c.config.Scopes) > 0 {
		params.Set("scope", strings.Join(c.config.Scopes, " "))
	}

	if c.config.Audience != "" {
		params.Set("audience", c.config.Audience)
	}

	return c.config.AuthorizeURL + "?" + params.Encode()
}

// doTokenRequest executa uma requisição de token
func (c *Client) doTokenRequest(req *http.Request) (*Token, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Tenta extrair mensagem de erro
		var errorResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if json.Unmarshal(body, &errorResp) == nil && errorResp.Error != "" {
			return nil, fmt.Errorf("erro OAuth: %s - %s", errorResp.Error, errorResp.ErrorDescription)
		}
		return nil, fmt.Errorf("erro HTTP %d: %s", resp.StatusCode, string(body))
	}

	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("erro ao parsear token: %w", err)
	}

	// Calcula tempo de expiração
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	} else {
		// Default: 1 hora
		token.ExpiresAt = time.Now().Add(1 * time.Hour)
	}

	return &token, nil
}

// resolveValue resolve um valor direto ou via variável de ambiente
func (c *Client) resolveValue(direct, envKey string) string {
	if direct != "" {
		return direct
	}
	if envKey != "" {
		return c.envVars[envKey]
	}
	return ""
}

// ClearCache limpa o cache de tokens
func (c *Client) ClearCache() {
	c.mu.Lock()
	c.cachedToken = nil
	c.mu.Unlock()
}

// SetToken define um token manualmente (útil para tokens obtidos externamente)
func (c *Client) SetToken(token *Token) {
	c.mu.Lock()
	c.cachedToken = token
	c.mu.Unlock()
}






