package oauth

import (
	"os"
)

// Provider representa um provedor OAuth pré-configurado
type Provider struct {
	ID            string   `json:"id"`             // google, microsoft, github, etc
	Name          string   `json:"name"`           // Nome para exibição
	Icon          string   `json:"icon"`           // Emoji ou ícone
	AuthorizeURL  string   `json:"authorize_url"`  // URL de autorização
	TokenURL      string   `json:"token_url"`      // URL para obter token
	RevokeURL     string   `json:"revoke_url"`     // URL para revogar token
	UserInfoURL   string   `json:"userinfo_url"`   // URL para obter info do usuário
	DefaultScopes []string `json:"default_scopes"` // Scopes padrão

	// Credenciais default (compiladas na aplicação)
	// Podem ser sobrescritas por variáveis de ambiente
	DefaultClientID     string `json:"-"`
	DefaultClientSecret string `json:"-"`

	// Nomes das variáveis de ambiente para override
	ClientIDEnv     string `json:"client_id_env"`
	ClientSecretEnv string `json:"client_secret_env"`
}

// GetClientID retorna o Client ID (env var tem prioridade sobre default)
func (p *Provider) GetClientID() string {
	if envVar := os.Getenv(p.ClientIDEnv); envVar != "" {
		return envVar
	}
	return p.DefaultClientID
}

// GetClientSecret retorna o Client Secret (env var tem prioridade sobre default)
func (p *Provider) GetClientSecret() string {
	if envVar := os.Getenv(p.ClientSecretEnv); envVar != "" {
		return envVar
	}
	return p.DefaultClientSecret
}

// IsConfigured verifica se o provider tem credenciais configuradas
func (p *Provider) IsConfigured() bool {
	return p.GetClientID() != "" && p.GetClientSecret() != ""
}

// Providers contém todos os provedores suportados
var Providers = map[string]*Provider{
	"google": {
		ID:           "google",
		Name:         "Google",
		Icon:         "🔵",
		AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		RevokeURL:    "https://oauth2.googleapis.com/revoke",
		UserInfoURL:  "https://www.googleapis.com/oauth2/v2/userinfo",
		DefaultScopes: []string{
			"openid",
			"email",
			"profile",
		},
		ClientIDEnv:     "GOOGLE_CLIENT_ID",
		ClientSecretEnv: "GOOGLE_CLIENT_SECRET",
	},

	"microsoft": {
		ID:           "microsoft",
		Name:         "Microsoft",
		Icon:         "🟦",
		AuthorizeURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		RevokeURL:    "", // Microsoft não tem endpoint de revoke padrão
		UserInfoURL:  "https://graph.microsoft.com/v1.0/me",
		DefaultScopes: []string{
			"openid",
			"email",
			"profile",
			"User.Read",
		},
		ClientIDEnv:     "MICROSOFT_CLIENT_ID",
		ClientSecretEnv: "MICROSOFT_CLIENT_SECRET",
	},

	"github": {
		ID:           "github",
		Name:         "GitHub",
		Icon:         "🐙",
		AuthorizeURL: "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		RevokeURL:    "", // GitHub usa revoke via API
		UserInfoURL:  "https://api.github.com/user",
		DefaultScopes: []string{
			"read:user",
			"user:email",
		},
		ClientIDEnv:     "GITHUB_CLIENT_ID",
		ClientSecretEnv: "GITHUB_CLIENT_SECRET",
	},

	"facebook": {
		ID:           "facebook",
		Name:         "Facebook / Meta",
		Icon:         "📘",
		AuthorizeURL: "https://www.facebook.com/v18.0/dialog/oauth",
		TokenURL:     "https://graph.facebook.com/v18.0/oauth/access_token",
		RevokeURL:    "", // Facebook usa revoke via API
		UserInfoURL:  "https://graph.facebook.com/me?fields=id,name,email",
		DefaultScopes: []string{
			"email",
			"public_profile",
		},
		ClientIDEnv:     "FACEBOOK_CLIENT_ID",
		ClientSecretEnv: "FACEBOOK_CLIENT_SECRET",
	},

	"linkedin": {
		ID:           "linkedin",
		Name:         "LinkedIn",
		Icon:         "💼",
		AuthorizeURL: "https://www.linkedin.com/oauth/v2/authorization",
		TokenURL:     "https://www.linkedin.com/oauth/v2/accessToken",
		RevokeURL:    "",
		UserInfoURL:  "https://api.linkedin.com/v2/userinfo",
		DefaultScopes: []string{
			"openid",
			"profile",
			"email",
		},
		ClientIDEnv:     "LINKEDIN_CLIENT_ID",
		ClientSecretEnv: "LINKEDIN_CLIENT_SECRET",
	},

	"slack": {
		ID:           "slack",
		Name:         "Slack",
		Icon:         "💬",
		AuthorizeURL: "https://slack.com/oauth/v2/authorize",
		TokenURL:     "https://slack.com/api/oauth.v2.access",
		RevokeURL:    "https://slack.com/api/auth.revoke",
		UserInfoURL:  "https://slack.com/api/users.identity",
		DefaultScopes: []string{
			"users:read",
			"users:read.email",
		},
		ClientIDEnv:     "SLACK_CLIENT_ID",
		ClientSecretEnv: "SLACK_CLIENT_SECRET",
	},

	"discord": {
		ID:           "discord",
		Name:         "Discord",
		Icon:         "🎮",
		AuthorizeURL: "https://discord.com/api/oauth2/authorize",
		TokenURL:     "https://discord.com/api/oauth2/token",
		RevokeURL:    "https://discord.com/api/oauth2/token/revoke",
		UserInfoURL:  "https://discord.com/api/users/@me",
		DefaultScopes: []string{
			"identify",
			"email",
		},
		ClientIDEnv:     "DISCORD_CLIENT_ID",
		ClientSecretEnv: "DISCORD_CLIENT_SECRET",
	},

	"twitter": {
		ID:           "twitter",
		Name:         "Twitter / X",
		Icon:         "🐦",
		AuthorizeURL: "https://twitter.com/i/oauth2/authorize",
		TokenURL:     "https://api.twitter.com/2/oauth2/token",
		RevokeURL:    "https://api.twitter.com/2/oauth2/revoke",
		UserInfoURL:  "https://api.twitter.com/2/users/me",
		DefaultScopes: []string{
			"tweet.read",
			"users.read",
		},
		ClientIDEnv:     "TWITTER_CLIENT_ID",
		ClientSecretEnv: "TWITTER_CLIENT_SECRET",
	},

	"notion": {
		ID:              "notion",
		Name:            "Notion",
		Icon:            "📝",
		AuthorizeURL:    "https://api.notion.com/v1/oauth/authorize",
		TokenURL:        "https://api.notion.com/v1/oauth/token",
		RevokeURL:       "",
		UserInfoURL:     "https://api.notion.com/v1/users/me",
		DefaultScopes:   []string{}, // Notion não usa scopes tradicionais
		ClientIDEnv:     "NOTION_CLIENT_ID",
		ClientSecretEnv: "NOTION_CLIENT_SECRET",
	},

	"spotify": {
		ID:           "spotify",
		Name:         "Spotify",
		Icon:         "🎵",
		AuthorizeURL: "https://accounts.spotify.com/authorize",
		TokenURL:     "https://accounts.spotify.com/api/token",
		RevokeURL:    "",
		UserInfoURL:  "https://api.spotify.com/v1/me",
		DefaultScopes: []string{
			"user-read-email",
			"user-read-private",
		},
		ClientIDEnv:     "SPOTIFY_CLIENT_ID",
		ClientSecretEnv: "SPOTIFY_CLIENT_SECRET",
	},

	"dropbox": {
		ID:              "dropbox",
		Name:            "Dropbox",
		Icon:            "📦",
		AuthorizeURL:    "https://www.dropbox.com/oauth2/authorize",
		TokenURL:        "https://api.dropboxapi.com/oauth2/token",
		RevokeURL:       "https://api.dropboxapi.com/2/auth/token/revoke",
		UserInfoURL:     "https://api.dropboxapi.com/2/users/get_current_account",
		DefaultScopes:   []string{}, // Dropbox usa scopes diferente
		ClientIDEnv:     "DROPBOX_CLIENT_ID",
		ClientSecretEnv: "DROPBOX_CLIENT_SECRET",
	},

	"atlassian": {
		ID:           "atlassian",
		Name:         "Atlassian (Jira/Confluence)",
		Icon:         "🔷",
		AuthorizeURL: "https://auth.atlassian.com/authorize",
		TokenURL:     "https://auth.atlassian.com/oauth/token",
		RevokeURL:    "",
		UserInfoURL:  "https://api.atlassian.com/me",
		DefaultScopes: []string{
			"read:me",
			"read:jira-work",
			"write:jira-work",
		},
		ClientIDEnv:     "ATLASSIAN_CLIENT_ID",
		ClientSecretEnv: "ATLASSIAN_CLIENT_SECRET",
	},

	"trello": {
		ID:           "trello",
		Name:         "Trello",
		Icon:         "📋",
		AuthorizeURL: "https://trello.com/1/authorize",
		TokenURL:     "https://trello.com/1/OAuthGetAccessToken", // Trello usa OAuth 1.0a
		RevokeURL:    "",
		UserInfoURL:  "https://api.trello.com/1/members/me",
		DefaultScopes: []string{
			"read",
			"write",
		},
		ClientIDEnv:     "TRELLO_API_KEY",
		ClientSecretEnv: "TRELLO_API_SECRET",
	},

	"asana": {
		ID:              "asana",
		Name:            "Asana",
		Icon:            "✅",
		AuthorizeURL:    "https://app.asana.com/-/oauth_authorize",
		TokenURL:        "https://app.asana.com/-/oauth_token",
		RevokeURL:       "",
		UserInfoURL:     "https://app.asana.com/api/1.0/users/me",
		DefaultScopes:   []string{},
		ClientIDEnv:     "ASANA_CLIENT_ID",
		ClientSecretEnv: "ASANA_CLIENT_SECRET",
	},

	"hubspot": {
		ID:           "hubspot",
		Name:         "HubSpot",
		Icon:         "🧡",
		AuthorizeURL: "https://app.hubspot.com/oauth/authorize",
		TokenURL:     "https://api.hubapi.com/oauth/v1/token",
		RevokeURL:    "",
		UserInfoURL:  "https://api.hubapi.com/oauth/v1/access-tokens",
		DefaultScopes: []string{
			"crm.objects.contacts.read",
		},
		ClientIDEnv:     "HUBSPOT_CLIENT_ID",
		ClientSecretEnv: "HUBSPOT_CLIENT_SECRET",
	},

	"salesforce": {
		ID:           "salesforce",
		Name:         "Salesforce",
		Icon:         "☁️",
		AuthorizeURL: "https://login.salesforce.com/services/oauth2/authorize",
		TokenURL:     "https://login.salesforce.com/services/oauth2/token",
		RevokeURL:    "https://login.salesforce.com/services/oauth2/revoke",
		UserInfoURL:  "https://login.salesforce.com/services/oauth2/userinfo",
		DefaultScopes: []string{
			"openid",
			"profile",
			"email",
			"api",
		},
		ClientIDEnv:     "SALESFORCE_CLIENT_ID",
		ClientSecretEnv: "SALESFORCE_CLIENT_SECRET",
	},

	"zendesk": {
		ID:           "zendesk",
		Name:         "Zendesk",
		Icon:         "🎫",
		AuthorizeURL: "https://{subdomain}.zendesk.com/oauth/authorizations/new",
		TokenURL:     "https://{subdomain}.zendesk.com/oauth/tokens",
		RevokeURL:    "",
		UserInfoURL:  "https://{subdomain}.zendesk.com/api/v2/users/me",
		DefaultScopes: []string{
			"read",
			"write",
		},
		ClientIDEnv:     "ZENDESK_CLIENT_ID",
		ClientSecretEnv: "ZENDESK_CLIENT_SECRET",
	},
}

// GetProvider retorna um provider pelo ID
func GetProvider(id string) *Provider {
	return Providers[id]
}

// GetAllProviders retorna todos os providers
func GetAllProviders() []*Provider {
	providers := make([]*Provider, 0, len(Providers))
	for _, p := range Providers {
		providers = append(providers, p)
	}
	return providers
}

// GetConfiguredProviders retorna apenas providers com credenciais configuradas
func GetConfiguredProviders() []*Provider {
	providers := make([]*Provider, 0)
	for _, p := range Providers {
		if p.IsConfigured() {
			providers = append(providers, p)
		}
	}
	return providers
}

// ProviderScopes define scopes adicionais por serviço
var ProviderScopes = map[string]map[string][]string{
	"google": {
		"gmail":     {"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/gmail.send"},
		"calendar":  {"https://www.googleapis.com/auth/calendar", "https://www.googleapis.com/auth/calendar.events"},
		"drive":     {"https://www.googleapis.com/auth/drive", "https://www.googleapis.com/auth/drive.file"},
		"sheets":    {"https://www.googleapis.com/auth/spreadsheets"},
		"docs":      {"https://www.googleapis.com/auth/documents"},
		"contacts":  {"https://www.googleapis.com/auth/contacts.readonly"},
		"youtube":   {"https://www.googleapis.com/auth/youtube.readonly"},
		"analytics": {"https://www.googleapis.com/auth/analytics.readonly"},
	},
	"microsoft": {
		"outlook":    {"Mail.Read", "Mail.Send", "Calendars.ReadWrite"},
		"onedrive":   {"Files.ReadWrite.All"},
		"teams":      {"Chat.Read", "Chat.ReadWrite"},
		"sharepoint": {"Sites.Read.All", "Sites.ReadWrite.All"},
	},
	"github": {
		"repos":    {"repo", "public_repo"},
		"gists":    {"gist"},
		"actions":  {"workflow"},
		"packages": {"read:packages", "write:packages"},
		"admin":    {"admin:org", "admin:repo_hook"},
	},
	"slack": {
		"channels":  {"channels:read", "channels:write"},
		"messages":  {"chat:write", "chat:write.public"},
		"files":     {"files:read", "files:write"},
		"reactions": {"reactions:read", "reactions:write"},
	},
}

// GetScopesForService retorna scopes para um serviço específico de um provider
func GetScopesForService(providerID, service string) []string {
	if services, ok := ProviderScopes[providerID]; ok {
		if scopes, ok := services[service]; ok {
			return scopes
		}
	}
	return nil
}






