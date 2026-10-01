package mcp

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

// OAuthInventoryItem contains only diagnostic codes and consumer labels. No
// credential, endpoint, client identifier or command configuration reaches UI.
type OAuthInventoryItem struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	Issues []string `json:"issues"`
}

// InspectOAuthInventory is a read-only prerequisite for AEP-0112 phase 3.
// Presence is not proof of token validity or permission to migrate/delete data.
func (m *Manager) InspectOAuthInventory(ctx context.Context) ([]OAuthInventoryItem, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	if m.credMgr == nil || m.repository() == nil {
		return nil, oauthflow.ErrResource
	}
	store, err := m.credMgr.OAuthStore(ctx)
	if err != nil {
		return nil, err
	}
	session, cancel := store.(interface {
		SessionContext(context.Context) (context.Context, context.CancelFunc)
	}).SessionContext(ctx)
	defer cancel()
	ctx = session
	configs, err := m.repository().ListServers(ctx)
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, cfg := range configs {
		if !isOAuthInventoryServer(cfg) {
			continue
		}
		parsed, parseErr := url.Parse(cfg.URL)
		if parseErr == nil && parsed.Hostname() != "" {
			hosts = append(hosts, parsed.Hostname())
		}
	}
	entries, err := m.credMgr.InspectLegacyOAuth(ctx, hosts...)
	if err != nil {
		return nil, err
	}
	result := classifyOAuthInventory(configs, entries)
	for i := range result {
		if result[i].Kind != "managed" {
			continue
		}
		for _, cfg := range configs {
			if cfg.ID != result[i].ID {
				continue
			}
			r, loadErr := store.Load(ctx, cfg.OAuthAuthorizationID)
			if loadErr != nil || validateMCPAuthorization(cfg, r) != nil {
				result[i].Issues = append(result[i].Issues, "invalid_reference")
			}
			break
		}
	}
	// Check the captured vault epoch synchronously: cancellation callbacks can
	// run after Reset has already changed the key/session.
	if err := store.(interface {
		WithSession(context.Context, func() error) error
	}).WithSession(ctx, func() error { return nil }); err != nil {
		return nil, err
	}
	return result, ctx.Err()
}

func classifyOAuthInventory(configs []ServerConfig, entries []credentials.LegacyOAuthEntry) []OAuthInventoryItem {
	byPattern := make(map[string]credentials.LegacyOAuthEntry, len(entries))
	used := make(map[string]bool)
	for _, entry := range entries {
		byPattern[entry.Pattern] = entry
	}
	result := make([]OAuthInventoryItem, 0)
	for _, cfg := range configs {
		if !isOAuthInventoryServer(cfg) {
			continue
		}
		item := OAuthInventoryItem{ID: cfg.ID, Name: cfg.Name, Kind: "legacy", Issues: []string{}}
		if item.Name == "" {
			item.Name = cfg.Slug
		}
		if cfg.AuthType == AuthOAuth2ClientCredentials {
			item.Kind = "client_credentials"
		}
		client, hasClient := byPattern[clientCredPattern(cfg.Slug)]
		tokens, hasTokens := byPattern[userTokensPattern(cfg.Slug)]
		used[client.Pattern], used[tokens.Pattern] = true, true
		for _, entry := range []credentials.LegacyOAuthEntry{client, tokens} {
			if entry.Pattern != "" {
				item.Issues = append(item.Issues, legacyOAuthEntryIssues(entry, true)...)
			}
		}
		if cfg.OAuthManaged || cfg.OAuthAuthorizationID != "" {
			item.Kind = "managed"
			if hasClient || hasTokens {
				item.Issues = append(item.Issues, "legacy_residue")
			}
		} else {
			if cfg.OAuth2ClientID == "" && client.ClientID == "" {
				item.Issues = append(item.Issues, "missing_client")
			}
			if cfg.OAuth2ClientID != "" && client.ClientID != "" && cfg.OAuth2ClientID != client.ClientID {
				item.Issues = append(item.Issues, "conflicting_client")
			}
			if cfg.OAuth2TokenURL == "" {
				item.Issues = append(item.Issues, "missing_endpoint")
			}
			if cfg.AuthType == AuthOAuth2PKCE {
				if !tokens.HasAccess && !tokens.HasRefresh {
					item.Issues = append(item.Issues, "missing_tokens")
				}
				if client.ClientGrantType == "" {
					item.Issues = append(item.Issues, "unknown_registration")
				}
			} else if !client.HasSecret {
				item.Issues = append(item.Issues, "missing_secret")
			}
		}
		parsed, _ := url.Parse(cfg.URL)
		if parsed != nil && parsed.Hostname() != "" {
			for _, entry := range entries {
				if strings.HasPrefix(entry.Pattern, "mcp-client:") || strings.HasPrefix(entry.Pattern, "mcp-tokens:") {
					continue
				}
				if entry.MatchesHost(parsed.Hostname()) {
					item.Issues = append(item.Issues, "hostname_credential")
				}
			}
		}
		sort.Strings(item.Issues)
		item.Issues = uniqueIssues(item.Issues)
		result = append(result, item)
	}
	for _, entry := range entries {
		if used[entry.Pattern] {
			continue
		}
		kind := "hostname"
		if strings.HasPrefix(entry.Pattern, "mcp-client:") || strings.HasPrefix(entry.Pattern, "mcp-tokens:") {
			kind = "unassociated"
		}
		item := OAuthInventoryItem{ID: "credential:" + entry.Pattern, Name: entry.Pattern, Kind: kind, Issues: []string{}}
		item.Issues = append(item.Issues, legacyOAuthEntryIssues(entry, kind == "unassociated")...)
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func legacyOAuthEntryIssues(entry credentials.LegacyOAuthEntry, requiresOAuthType bool) []string {
	issues := []string{}
	if entry.Source != "" && entry.Source != "static" {
		issues = append(issues, "external_source")
	} else if !entry.Readable {
		issues = append(issues, "unreadable")
	}
	if requiresOAuthType && entry.AuthType != "oauth2" {
		issues = append(issues, "unexpected_type")
	}
	return issues
}

func isOAuthInventoryServer(cfg ServerConfig) bool {
	return cfg.AuthType == AuthOAuth2PKCE || cfg.AuthType == AuthOAuth2ClientCredentials || cfg.OAuthAuthorizationID != "" || cfg.OAuthManaged
}

func uniqueIssues(items []string) []string {
	result := items[:0]
	for _, item := range items {
		if len(result) == 0 || result[len(result)-1] != item {
			result = append(result, item)
		}
	}
	return result
}
