package mcp

import (
	"context"
	"fmt"

	"assistente/internal/credentials"
)

func (m *Manager) SaveServerAuth(slug, authType, token, username, password, clientSecret string) error {
	if m.credMgr == nil {
		return fmt.Errorf("credential manager nao inicializado")
	}

	cfg, err := m.GetConfig(slug)
	if err != nil {
		return err
	}

	hostname := hostnameFromURL(cfg.URL)
	if hostname == "" {
		return fmt.Errorf("servidor MCP '%s' nao tem URL valida para autenticacao", slug)
	}

	auth := &credentials.AuthConfig{}

	switch authType {
	case "bearer":
		auth.Type = "bearer"
		auth.Token = token
	case "basic":
		auth.Type = "basic"
		auth.Username = username
		auth.Password = password
	case "oauth2_client_credentials", "oauth2_pkce":
		auth.Type = "oauth2"
		auth.ClientSecret = clientSecret
		auth.ClientID = cfg.OAuth2ClientID
	default:
		return fmt.Errorf("tipo de autenticacao invalido: %s", authType)
	}

	return m.credMgr.RegisterPatternWithContext(context.Background(), hostname, auth)
}

func (m *Manager) DeleteServerAuth(slug string) error {
	if m.credMgr == nil {
		return fmt.Errorf("credential manager nao inicializado")
	}

	cfg, err := m.GetConfig(slug)
	if err != nil {
		return err
	}

	hostname := hostnameFromURL(cfg.URL)
	if hostname == "" {
		return fmt.Errorf("servidor MCP '%s' nao tem URL valida para autenticacao", slug)
	}

	return m.credMgr.DeletePattern(context.Background(), hostname)
}

func (m *Manager) GetServerAuthInfo(slug string) (string, bool, error) {
	if m.credMgr == nil {
		return "", false, fmt.Errorf("credential manager nao inicializado")
	}

	cfg, err := m.GetConfig(slug)
	if err != nil {
		return "", false, err
	}

	hostname := hostnameFromURL(cfg.URL)
	if hostname == "" {
		return "", false, nil
	}

	auth, err := m.credMgr.GetByPattern(hostname)
	if err != nil {
		return "", false, err
	}
	if auth == nil {
		return "", false, nil
	}

	resolvedType := ""
	if cfg.AuthType != "" && cfg.AuthType != AuthNone {
		resolvedType = string(cfg.AuthType)
	} else {
		switch auth.Type {
		case "bearer":
			resolvedType = "bearer"
		case "basic":
			resolvedType = "basic"
		case "oauth2":
			resolvedType = string(AuthOAuth2PKCE)
		default:
			resolvedType = auth.Type
		}
	}

	return resolvedType, true, nil
}
