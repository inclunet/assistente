package oauth

import (
	"time"
)

// Config representa a configuração de OAuth 2.0
type Config struct {
	// Tipo de grant
	GrantType string `json:"grant_type"` // client_credentials, authorization_code, refresh_token

	// Endpoints
	TokenURL     string `json:"token_url"`
	AuthorizeURL string `json:"authorize_url,omitempty"` // Para authorization_code

	// Credenciais
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`

	// Para authorization_code
	RedirectURI string `json:"redirect_uri,omitempty"`

	// Scopes
	Scopes []string `json:"scopes,omitempty"`

	// Opções extras
	SendCredentialsInBody bool   `json:"send_credentials_in_body"` // Alguns servidores requerem isso
	Audience              string `json:"audience,omitempty"`       // Para alguns provedores (Auth0)

	// Variáveis de ambiente (alternativa aos valores diretos)
	ClientIDEnv     string `json:"client_id_env,omitempty"`
	ClientSecretEnv string `json:"client_secret_env,omitempty"`
}

// Token representa um token OAuth 2.0
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"-"`
}

// IsExpired verifica se o token expirou
func (t *Token) IsExpired() bool {
	// Considera expirado 30 segundos antes para evitar race conditions
	return time.Now().Add(30 * time.Second).After(t.ExpiresAt)
}

// CallbackResult representa o resultado de um callback OAuth
type CallbackResult struct {
	Code  string
	State string
	Error string
}




