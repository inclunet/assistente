package mcp

import (
	"assistente/internal/oauthflow"
	"context"
)

// OAuthDiscoveryResult contém os metadados OAuth descobertos de um servidor MCP.
type OAuthDiscoveryResult struct {
	Found                    bool                    `json:"found"`
	Status                   string                  `json:"status"`
	ProtectedResourceFound   bool                    `json:"protectedResourceFound"`
	AuthorizationServerFound bool                    `json:"authorizationServerFound"`
	MetadataType             string                  `json:"metadataType,omitempty"`
	ManualCompletionRequired bool                    `json:"manualCompletionRequired"`
	AuthType                 AuthType                `json:"authType"`
	AuthURL                  string                  `json:"authUrl"`
	TokenURL                 string                  `json:"tokenUrl"`
	Scopes                   []string                `json:"scopes"`
	ClientID                 string                  `json:"clientId,omitempty"`
	RegistrationURL          string                  `json:"registrationUrl,omitempty"`
	ResourceName             string                  `json:"resourceName,omitempty"`
	SupportsPKCE             bool                    `json:"supportsPkce"`
	ResponseHints            []DiscoveryResponseHint `json:"responseHints,omitempty"`
	Error                    string                  `json:"error,omitempty"`
}

// DiscoveryResponseHint contém apenas dados limitados e saneados de uma
// resposta de discovery que não pôde ser usada como metadata.
type DiscoveryResponseHint struct {
	StatusCode      int    `json:"statusCode"`
	Classification  string `json:"classification"`
	WWWAuthenticate string `json:"wwwAuthenticate,omitempty"`
	Location        string `json:"location,omitempty"`
	JSONError       string `json:"jsonError,omitempty"`
	BodyTruncated   bool   `json:"bodyTruncated,omitempty"`
}

// DiscoverOAuth retains the MCP UI contract while the shared OAuth layer owns discovery.
func DiscoverOAuth(resourceURL string) OAuthDiscoveryResult {
	return DiscoverOAuthContext(context.Background(), resourceURL)
}

func DiscoverOAuthContext(ctx context.Context, resourceURL string) OAuthDiscoveryResult {
	result := oauthflow.DiscoverOAuthContext(ctx, resourceURL)
	authType := AuthType("")
	switch result.GrantType {
	case "authorization_code":
		authType = AuthOAuth2PKCE
	case "client_credentials":
		authType = AuthOAuth2ClientCredentials
	}
	hints := make([]DiscoveryResponseHint, len(result.ResponseHints))
	for i, hint := range result.ResponseHints {
		hints[i] = DiscoveryResponseHint(hint)
	}
	return OAuthDiscoveryResult{
		Found: result.Found, Status: result.Status,
		ProtectedResourceFound:   result.ProtectedResourceFound,
		AuthorizationServerFound: result.AuthorizationServerFound,
		MetadataType:             result.MetadataType, ManualCompletionRequired: result.ManualCompletionRequired,
		AuthType: authType, AuthURL: result.AuthURL, TokenURL: result.TokenURL,
		Scopes: result.Scopes, ClientID: result.ClientID, RegistrationURL: result.RegistrationURL,
		ResourceName: result.ResourceName, SupportsPKCE: result.SupportsPKCE,
		ResponseHints: hints, Error: result.Error,
	}
}
