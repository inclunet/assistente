package mcp

import (
	"fmt"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

func (m *Manager) SaveServerAuth(slug, authType, token, username, password, clientSecret string) error {
	if m.credMgr == nil {
		return fmt.Errorf("credential manager nao inicializado")
	}

	cfg, err := m.GetConfig(slug)
	if err != nil {
		return err
	}

	ctx := m.credentialContext()
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}

	switch authType {
	case "bearer":
		hostname := hostnameFromURL(cfg.URL)
		if hostname == "" {
			return fmt.Errorf("servidor MCP '%s' nao tem URL valida", slug)
		}
		return m.credMgr.RegisterPatternWithContext(ctx, hostname, &credentials.AuthConfig{
			Type:  "bearer",
			Token: token,
		})

	case "basic":
		hostname := hostnameFromURL(cfg.URL)
		if hostname == "" {
			return fmt.Errorf("servidor MCP '%s' nao tem URL valida", slug)
		}
		return m.credMgr.RegisterPatternWithContext(ctx, hostname, &credentials.AuthConfig{
			Type:     "basic",
			Username: username,
			Password: password,
		})

	case "oauth2_client_credentials", "oauth2_pkce":
		return m.credMgr.RegisterPatternWithContext(ctx, clientCredPattern(slug), &credentials.AuthConfig{
			Type:         "oauth2",
			ClientID:     cfg.OAuth2ClientID,
			ClientSecret: clientSecret,
		})

	default:
		return fmt.Errorf("tipo de autenticacao invalido: %s", authType)
	}
}

func (m *Manager) DeleteServerAuth(slug string) error {
	if m.credMgr == nil {
		return fmt.Errorf("credential manager nao inicializado")
	}

	ctx := m.credentialContext()
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}

	// Limpar entradas OAuth (client + tokens)
	_ = m.credMgr.DeletePattern(ctx, clientCredPattern(slug))
	_ = m.credMgr.DeletePattern(ctx, userTokensPattern(slug))

	// Limpar entrada legacy por hostname (bearer/basic)
	cfg, err := m.GetConfig(slug)
	if err == nil {
		if hostname := hostnameFromURL(cfg.URL); hostname != "" {
			_ = m.credMgr.DeletePattern(ctx, hostname)
		}
	}

	return nil
}

func (m *Manager) GetServerAuthInfo(slug string) (string, bool, error) {
	if m.credMgr == nil {
		return "", false, fmt.Errorf("credential manager nao inicializado")
	}

	cfg, err := m.GetConfig(slug)
	if err != nil {
		return "", false, err
	}

	// Verifica entrada OAuth (mcp-client:{slug})
	ctx := m.credentialContext()
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", false, err
	}

	clientAuth, _ := m.credMgr.GetByPatternWithContext(ctx, clientCredPattern(slug))
	if clientAuth != nil {
		if cfg.AuthType != "" && cfg.AuthType != AuthNone {
			return string(cfg.AuthType), true, nil
		}
		return string(AuthOAuth2PKCE), true, nil
	}

	// Verifica entrada legacy por hostname (bearer/basic)
	hostname := hostnameFromURL(cfg.URL)
	if hostname == "" {
		return "", false, nil
	}

	auth, err := m.credMgr.GetByPatternWithContext(ctx, hostname)
	if err != nil || auth == nil {
		return "", false, err
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
