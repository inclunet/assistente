package mcp

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"assistente/internal/credentials"
)

type inlineAuthConfig struct {
	Headers     map[string]string `json:"headers"`
	RequestInit struct {
		Headers map[string]string `json:"headers"`
	} `json:"requestInit"`
}

func (m *Manager) applyInlineAuthFromConfig(slug string, cfg *ServerConfig, data []byte) {
	token := extractBearerTokenFromConfig(data)
	if token == "" {
		return
	}

	cfg.AuthType = AuthBearer
	m.importBearerCredential(slug, cfg.URL, token)
}

func (m *Manager) importBearerCredential(slug, rawURL, token string) {
	if m.credMgr == nil {
		return
	}
	hostname := hostnameFromURL(rawURL)
	if hostname == "" {
		log.Printf("[MCP:%s] Authorization Bearer encontrado, mas URL inválida para credencial", slug)
		return
	}
	if err := m.credMgr.RegisterPatternWithContext(context.Background(), hostname, &credentials.AuthConfig{
		Type:  "bearer",
		Token: token,
	}); err != nil {
		log.Printf("[MCP:%s] Erro ao importar Authorization Bearer para credential manager: %v", slug, err)
		return
	}
	log.Printf("[MCP:%s] Authorization Bearer importado para credential manager (%s)", slug, hostname)
}

func extractBearerTokenFromConfig(data []byte) string {
	var raw inlineAuthConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}

	if token := extractBearerTokenFromHeaders(raw.RequestInit.Headers); token != "" {
		return token
	}
	return extractBearerTokenFromHeaders(raw.Headers)
}

func extractBearerTokenFromHeaders(headers map[string]string) string {
	for name, value := range headers {
		if !strings.EqualFold(name, "Authorization") {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(strings.ToLower(value), "bearer ") {
			return strings.TrimSpace(value[len("bearer "):])
		}
	}
	return ""
}

func bearerAuthorizationHeader(token string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return token
	}
	return "Bearer " + token
}
