package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
// Segue a spec MCP Authorization:
// 1. GET {origin}/.well-known/oauth-protected-resource
// 2. GET {auth_server}/.well-known/oauth-authorization-server
func DiscoverOAuth(serverURL string) OAuthDiscoveryResult {
	origin, err := extractOrigin(serverURL)
	if err != nil {
		return OAuthDiscoveryResult{Error: err.Error()}
	}

	log.Printf("[MCP:discovery] Tentando discovery OAuth para %s", origin)

	authServerBase := origin
	var resourceName string
	var resourceScopes []string

	prm, err := fetchProtectedResourceMetadata(origin)
	if err == nil && prm != nil {
		if len(prm.AuthorizationServers) > 0 {
			authServerBase = prm.AuthorizationServers[0]
		}
		resourceName = prm.ResourceName
		resourceScopes = prm.ScopesSupported
		log.Printf("[MCP:discovery] Protected Resource Metadata encontrado: auth_server=%s, resource=%s",
			authServerBase, resourceName)
	} else {
		log.Printf("[MCP:discovery] Protected Resource Metadata não encontrado para %s, usando origin", origin)
	}

	asm, err := fetchAuthServerMetadata(authServerBase)
	if err != nil || asm == nil {
		log.Printf("[MCP:discovery] Auth Server Metadata não encontrado para %s", authServerBase)
		return OAuthDiscoveryResult{Found: false}
	}

	log.Printf("[MCP:discovery] Auth Server Metadata encontrado: auth_endpoint=%s, token_endpoint=%s",
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
	Issuer                         string   `json:"issuer"`
	AuthorizationEndpoint          string   `json:"authorization_endpoint"`
	TokenEndpoint                  string   `json:"token_endpoint"`
	RegistrationEndpoint           string   `json:"registration_endpoint"`
	DeviceAuthorizationEndpoint    string   `json:"device_authorization_endpoint"`
	ScopesSupported                []string `json:"scopes_supported"`
	GrantTypesSupported            []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported  []string `json:"code_challenge_methods_supported"`
	ResponseTypesSupported         []string `json:"response_types_supported"`
}

func fetchProtectedResourceMetadata(origin string) (*protectedResourceMetadata, error) {
	wellKnownURL := strings.TrimRight(origin, "/") + "/.well-known/oauth-protected-resource"
	var result protectedResourceMetadata
	if err := fetchJSON(wellKnownURL, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func fetchAuthServerMetadata(authServerBase string) (*authServerMetadata, error) {
	base := strings.TrimRight(authServerBase, "/")

	urls := []string{
		base + "/.well-known/oauth-authorization-server",
		base + "/.well-known/openid-configuration",
	}

	for _, u := range urls {
		var result authServerMetadata
		if err := fetchJSON(u, &result); err == nil {
			if result.TokenEndpoint != "" {
				return &result, nil
			}
		}
	}

	return nil, fmt.Errorf("auth server metadata not found")
}

func fetchJSON(url string, target any) error {
	resp, err := discoveryHTTPClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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
