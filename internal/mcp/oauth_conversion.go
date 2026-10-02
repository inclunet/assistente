package mcp

import (
	"context"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

// ConvertOAuthClientSnapshot is explicit and offline. Unknown legacy client
// authentication is supplied by the user, never guessed by migration.
func (m *Manager) ConvertOAuthClientSnapshot(ctx context.Context, snapshotID, authMethod string) error {
	if authMethod != "client_secret_basic" && authMethod != "client_secret_post" {
		return oauthflow.ErrClientConfiguration
	}
	dir, err := m.oauthSnapshotDirectory()
	if err != nil || m.credMgr == nil {
		return credentials.ErrSnapshot
	}
	var slug string
	err = m.credMgr.ConvertLegacyClientCredentials(ctx, dir, snapshotID, authMethod, func(row database.MCPServer, client *credentials.AuthConfig, id string) (oauthflow.Record, database.MCPServer, error) {
		cfg, err := serverModelToConfig(row)
		if err != nil {
			return oauthflow.Record{}, row, err
		}
		if cfg.AuthType != AuthOAuth2ClientCredentials || (cfg.Transport != TransportSSE && cfg.Transport != TransportStreamable) || cfg.OAuth2TokenURL == "" || (cfg.OAuth2ClientID != "" && client.ClientID != "" && cfg.OAuth2ClientID != client.ClientID) {
			return oauthflow.Record{}, row, oauthflow.ErrResource
		}
		if client.ClientID == "" {
			client.ClientID = cfg.OAuth2ClientID
		}
		if client.ClientID == "" {
			return oauthflow.Record{}, row, oauthflow.ErrClientConfiguration
		}
		record := oauthflow.Record{Version: 1, Revision: 1, ID: id, UserID: row.UserID, ConsumerID: row.ID, Integration: "mcp", GrantType: "client_credentials", State: "pending", Resource: cfg.URL, Audience: cfg.URL,
			RequestedScopes: append([]string(nil), cfg.OAuth2Scopes...), Endpoints: oauthflow.Endpoints{Token: cfg.OAuth2TokenURL}, Client: oauthflow.ClientRegistration{Method: "manual", ID: client.ClientID, Secret: client.ClientSecret, AuthMethod: authMethod}}
		if _, err := oauthflow.NewConfigured(record, nil); err != nil {
			return record, row, err
		}
		cfg.OAuthManaged, cfg.OAuthAuthorizationID = true, id
		clearOAuthConfiguration(&cfg)
		projected, err := serverConfigToModel(cfg)
		// Preserve metadata/flags not projected by the config mapper.
		projected.UUIDModel = row.UUIDModel
		projected.LastConnectedAt, projected.LastDiscoveredAt, projected.LastError = row.LastConnectedAt, row.LastDiscoveredAt, row.LastError
		return record, projected, err
	}, func(row database.MCPServer, _ bool) {
		cfg, err := serverModelToConfig(row)
		if err == nil {
			slug = cfg.Slug
			m.publishManagedOAuth(cfg)
		}
	})
	if err != nil {
		return err
	}
	if slug != "" {
		_ = m.disconnect(slug, true)
	}
	m.emit("mcp:config_changed", map[string]string{"slug": slug})
	return nil
}
