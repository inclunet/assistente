package mcp

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// OAuthDiscoveryResult contém os metadados OAuth descobertos de um servidor MCP.
type OAuthDiscoveryResult struct {
	Found           bool     `json:"found"`
	AuthType        AuthType `json:"authType"`
	AuthURL         string   `json:"authUrl"`
	TokenURL        string   `json:"tokenUrl"`
	Scopes          []string `json:"scopes"`
	ClientID        string   `json:"clientId,omitempty"`
	RegistrationURL string   `json:"registrationUrl,omitempty"`
	ResourceName    string   `json:"resourceName,omitempty"`
	SupportsPKCE    bool     `json:"supportsPkce"`
	Error           string   `json:"error,omitempty"`
}

var discoveryHTTPClient = &http.Client{Timeout: 5 * time.Second}

// DiscoverOAuth consulta os endpoints well-known de um servidor MCP para
// preencher automaticamente a configuração de autenticação OAuth.
//
// Segue a spec MCP Authorization (RFC 9470 + RFC 8414):
// 1. GET protected resource metadata (resource URL → origin fallback)
// 2. GET auth server metadata (issuer URL → RFC 8414 path → origin fallback)
func DiscoverOAuth(serverURL string) OAuthDiscoveryResult {
	origin, err := extractOrigin(serverURL)
	if err != nil {
		return OAuthDiscoveryResult{Error: err.Error()}
	}

	logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] Tentando discovery OAuth para %s (origin=%s)", serverURL, origin)

	authServerBase := origin
	var resourceName string
	var resourceScopes []string

	prm, err := fetchProtectedResourceMetadata(serverURL)
	if err == nil && prm != nil {
		if len(prm.AuthorizationServers) > 0 {
			authServerBase = prm.AuthorizationServers[0]
		}
		resourceName = prm.ResourceName
		resourceScopes = prm.ScopesSupported
		logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] Protected Resource Metadata encontrado: auth_server=%s, resource=%s",
			authServerBase, resourceName)
	} else {
		logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] Protected Resource Metadata não encontrado para %s, usando origin", serverURL)
	}

	asm, err := fetchAuthServerMetadata(authServerBase)
	if err != nil || asm == nil {
		logging.Errorf(context.Background(), "mcp.discovery", "[MCP:discovery] Auth Server Metadata não encontrado para %s", authServerBase)
		return OAuthDiscoveryResult{Found: false, Error: err.Error()}
	}

	logging.Infof(context.Background(), "mcp.discovery", "[MCP:discovery] Auth Server Metadata encontrado: auth_endpoint=%s, token_endpoint=%s",
		asm.AuthorizationEndpoint, asm.TokenEndpoint)

	scopes := resourceScopes
	if len(scopes) == 0 {
		scopes = asm.ScopesSupported
	}

	supportsPKCE := slices.Contains(asm.CodeChallengeMethodsSupported, "S256")

	authType := AuthOAuth2PKCE
	if !supportsPKCE && slices.Contains(asm.GrantTypesSupported, "client_credentials") {
		authType = AuthOAuth2ClientCredentials
	}

	return OAuthDiscoveryResult{
		Found:           true,
		AuthType:        authType,
		AuthURL:         asm.AuthorizationEndpoint,
		TokenURL:        asm.TokenEndpoint,
		Scopes:          scopes,
		RegistrationURL: asm.RegistrationEndpoint,
		ResourceName:    resourceName,
		SupportsPKCE:    supportsPKCE,
	}
}

// protectedResourceMetadata representa a resposta de
// GET /.well-known/oauth-protected-resource (RFC 9470 / MCP spec).
type protectedResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
	ResourceName         string   `json:"resource_name"`
	Resource             string   `json:"resource"`
}

// authServerMetadata representa a resposta de
// GET /.well-known/oauth-authorization-server (RFC 8414).
type authServerMetadata struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	RegistrationEndpoint          string   `json:"registration_endpoint"`
	DeviceAuthorizationEndpoint   string   `json:"device_authorization_endpoint"`
	ScopesSupported               []string `json:"scopes_supported"`
	GrantTypesSupported           []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
}

// fetchProtectedResourceMetadata tenta descobrir metadata do recurso protegido (RFC 9470).
// Candidatos tentados em ordem:
//  1. {resourceURL}/.well-known/oauth-protected-resource (relativo ao recurso)
//  2. {origin}/.well-known/oauth-protected-resource (fallback no origin)
func fetchProtectedResourceMetadata(mcpURL string) (*protectedResourceMetadata, error) {
	candidates := buildPRMCandidates(mcpURL)
	for _, candidateURL := range candidates {
		logging.Errorf(context.Background(), "mcp.discovery", "[MCP:discovery] PRM: tentando %s", candidateURL)
		var result protectedResourceMetadata
		if err := fetchJSON(candidateURL, &result); err == nil {
			logging.Errorf(context.Background(), "mcp.discovery", "[MCP:discovery] PRM: encontrado em %s", candidateURL)
			return &result, nil
		}
	}
	return nil, fmt.Errorf("protected resource metadata not found (tentou %d URLs)", len(candidates))
}

func buildPRMCandidates(mcpURL string) []string {
	u, err := url.Parse(mcpURL)
	if err != nil {
		return nil
	}
	origin := u.Scheme + "://" + u.Host
	resourceBase := strings.TrimRight(mcpURL, "/")

	seen := make(map[string]bool)
	var candidates []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			candidates = append(candidates, s)
		}
	}

	if resourceBase != origin {
		add(resourceBase + "/.well-known/oauth-protected-resource")
	}
	add(origin + "/.well-known/oauth-protected-resource")
	return candidates
}

// fetchAuthServerMetadata tenta descobrir metadata do authorization server (RFC 8414).
// Candidatos tentados em ordem:
//  1. {base}/.well-known/oauth-authorization-server (implementação comum)
//  2. {base}/.well-known/openid-configuration
//  3. {origin}/.well-known/oauth-authorization-server{path} (RFC 8414 §3 para issuer com path)
//  4. {origin}/.well-known/openid-configuration{path}
//  5. {origin}/.well-known/oauth-authorization-server (origin root fallback)
//  6. {origin}/.well-known/openid-configuration
func fetchAuthServerMetadata(authServerBase string) (*authServerMetadata, error) {
	candidates := buildASMCandidates(authServerBase)
	for _, candidateURL := range candidates {
		logging.Errorf(context.Background(), "mcp.discovery", "[MCP:discovery] ASM: tentando %s", candidateURL)
		var result authServerMetadata
		if err := fetchJSON(candidateURL, &result); err == nil && result.TokenEndpoint != "" {
			logging.Errorf(context.Background(), "mcp.discovery", "[MCP:discovery] ASM: encontrado em %s", candidateURL)
			return &result, nil
		}
	}
	return nil, fmt.Errorf("auth server metadata not found (tentou %d URLs)", len(candidates))
}

func buildASMCandidates(authServerBase string) []string {
	base := strings.TrimRight(authServerBase, "/")
	u, err := url.Parse(base)
	if err != nil {
		return []string{base + "/.well-known/oauth-authorization-server"}
	}

	origin := u.Scheme + "://" + u.Host
	path := strings.TrimRight(u.Path, "/")

	seen := make(map[string]bool)
	var candidates []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			candidates = append(candidates, s)
		}
	}

	add(base + "/.well-known/oauth-authorization-server")
	add(base + "/.well-known/openid-configuration")

	if path != "" {
		// RFC 8414 §3: .well-known inserido no início do path component do issuer
		add(origin + "/.well-known/oauth-authorization-server" + path)
		add(origin + "/.well-known/openid-configuration" + path)
		// Origin root fallback
		add(origin + "/.well-known/oauth-authorization-server")
		add(origin + "/.well-known/openid-configuration")
	}

	return candidates
}

func fetchJSON(url string, target any) error {
	resp, err := discoveryHTTPClient.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return err
	}

	return json.Unmarshal(body, target)
}

func extractOrigin(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("URL inválida: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("URL precisa de scheme e host: %s", rawURL)
	}
	return u.Scheme + "://" + u.Host, nil
}
