package mcp

import (
	"errors"

	"assistente/internal/oauthflow"
)

var errOAuthMigrationRequired = errors.New("oauth_migration_required")

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
		return errOAuthMigrationRequired
	}
	return nil
}
