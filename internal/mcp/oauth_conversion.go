package mcp

import (
	"context"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

// ReconnectOAuthSnapshot is an explicit new authorization, not a format-only
// conversion. No legacy token is reinterpreted as a grant with invented metadata.
func (m *Manager) ReconnectOAuthSnapshot(ctx context.Context, snapshotID, authMethod string) error {
	if authMethod != "none" && authMethod != "client_secret_basic" && authMethod != "client_secret_post" {
		return oauthflow.ErrClientConfiguration
	}
	dir, err := m.oauthSnapshotDirectory()
	if err != nil || m.credMgr == nil {
		return credentials.ErrSnapshot
	}
	var slug string
	err = m.credMgr.ReconnectLegacyOAuth(ctx, dir, snapshotID, authMethod,
		func(row database.MCPServer, client *credentials.AuthConfig, id string) (oauthflow.Record, error) {
			cfg, err := serverModelToConfig(row)
			if err != nil {
				return oauthflow.Record{}, err
			}
			if cfg.AuthType != AuthOAuth2PKCE || (cfg.Transport != TransportSSE && cfg.Transport != TransportStreamable) || (client.ClientID != "" && cfg.OAuth2ClientID != "" && client.ClientID != cfg.OAuth2ClientID) {
				return oauthflow.Record{}, oauthflow.ErrResource
			}
			if client.ClientID == "" {
				client.ClientID = cfg.OAuth2ClientID
			}
			// Preserve the existing registration and callback policy. A new DCR
			// registration is needed only when no client ID was stored.
			method := "manual"
			if client.ClientGrantType != "" || client.ClientID == "" {
				method = "dcr"
			}
			if (authMethod == "none" && client.ClientSecret != "") || (client.ClientID != "" && authMethod != "none" && client.ClientSecret == "") {
				return oauthflow.Record{}, oauthflow.ErrClientConfiguration
			}
			if client.ClientID == "" && (client.ClientSecret != "" || authMethod != "none") {
				return oauthflow.Record{}, oauthflow.ErrClientConfiguration
			}
			if client.ClientGrantType != "" && client.ClientGrantType != "authorization_code" && client.ClientGrantType != "urn:ietf:params:oauth:grant-type:device_code" {
				return oauthflow.Record{}, oauthflow.ErrClientConfiguration
			}
			r := oauthflow.Record{Version: 1, Revision: 1, ID: id, UserID: row.UserID, ConsumerID: row.ID, Integration: "mcp", GrantType: "authorization_code", State: "pending", Resource: cfg.URL, Audience: cfg.URL,
				RequestedScopes: append([]string(nil), cfg.OAuth2Scopes...), Client: oauthflow.ClientRegistration{ID: client.ClientID, Secret: client.ClientSecret, AuthMethod: authMethod, Method: method, GrantType: client.ClientGrantType},
				Endpoints: oauthflow.Endpoints{Authorization: cfg.OAuth2AuthURL, Token: cfg.OAuth2TokenURL, Registration: cfg.OAuth2RegistrationURL, Device: cfg.OAuth2DeviceAuthURL},
				Callback:  oauthflow.CallbackConfig{Host: cfg.OAuth2CallbackHost, Port: cfg.OAuth2CallbackPort, Path: "/callback", PortPolicy: "ephemeral"}}
			if r.Callback.Port != 0 {
				r.Callback.PortPolicy = "fixed"
			}
			_, err = oauthflow.NewConfigured(r, m.authorizeOAuthNetwork)
			return r, err
		}, func(flowCtx context.Context, store oauthflow.Store, r oauthflow.Record, row database.MCPServer) error {
			cfg, err := serverModelToConfig(row)
			if err != nil {
				return err
			}
			cfg.OAuthManaged, cfg.OAuthAuthorizationID = true, r.ID
			flowCtx, done, err := m.beginManagedAttempt(flowCtx, cfg.Slug)
			if err != nil {
				return err
			}
			defer done()
			service, err := oauthflow.NewConfigured(r, m.authorizeOAuthNetwork)
			if err != nil {
				return err
			}
			return m.authorizeOAuthWithStore(flowCtx, cfg.Slug, cfg, store, service)
		}, func(row database.MCPServer, r oauthflow.Record) (database.MCPServer, error) {
			if r.State != "connected" || r.Tokens.Access == "" || !strings.EqualFold(r.Tokens.Type, "Bearer") {
				return row, oauthflow.ErrReauthorize
			}
			cfg, err := serverModelToConfig(row)
			if err != nil {
				return row, err
			}
			cfg.OAuthManaged, cfg.OAuthAuthorizationID = true, r.ID
			clearOAuthConfiguration(&cfg)
			projected, err := serverConfigToModel(cfg)
			projected.UUIDModel = row.UUIDModel
			projected.LastConnectedAt, projected.LastDiscoveredAt, projected.LastError = row.LastConnectedAt, row.LastDiscoveredAt, row.LastError
			return projected, err
		}, func(row database.MCPServer) {
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
