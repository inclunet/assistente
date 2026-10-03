package mcp

import (
	"errors"

	"assistente/internal/oauthflow"
)

var errOAuthMigrationRequired = errors.New("oauth_migration_required")
var errOAuthAuthenticationSelection = errors.New("oauth_authentication_selection_required")

// Historical configuration remains readable for snapshots and explicit
// migration. It is no longer an operational OAuth authorization.
func oauthRuntimeError(cfg ServerConfig) error {
	if cfg.Transport != TransportSSE && cfg.Transport != TransportStreamable {
		return nil
	}
	if cfg.OAuthAuthorizationID != "" {
		return nil // The shared resolver validates the reference and its owner.
	}
	if cfg.OAuthManaged {
		return oauthflow.ErrResource
	}
	if cfg.AuthType == AuthOAuth2PKCE || cfg.AuthType == AuthOAuth2ClientCredentials {
		// URL-only historical configurations inferred PKCE even for public
		// services. Discovery-only OAuth has the same shape: never guess that
		// an anonymous request proves authentication is unnecessary.
		if cfg.AuthType == AuthOAuth2PKCE && cfg.OAuth2ClientID == "" && cfg.OAuth2AuthURL == "" && cfg.OAuth2TokenURL == "" && cfg.OAuth2RegistrationURL == "" && cfg.OAuth2DeviceAuthURL == "" && len(cfg.OAuth2Scopes) == 0 && cfg.OAuth2CallbackPort == 0 && cfg.OAuth2CallbackHost == "" && cfg.OAuth2ClientMethod == "" && cfg.OAuth2TokenAuthMethod == "" {
			return errOAuthAuthenticationSelection
		}
		return errOAuthMigrationRequired
	}
	return nil
}
