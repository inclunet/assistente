package credentials

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"assistente/internal/database"
)

// LegacyOAuthEntry is a diagnostic projection, never a credential resolver.
// ClientID stays backend-only so callers can detect conflicting registrations.
type LegacyOAuthEntry struct {
	Pattern         string
	Source          string
	AuthType        string
	Readable        bool
	HasAccess       bool
	HasRefresh      bool
	HasSecret       bool
	ClientID        string `json:"-"`
	ClientGrantType string
}

func (e LegacyOAuthEntry) MatchesHost(host string) bool {
	// Native MCP fallback also uses an exact lookup, preserving the case of
	// hostname patterns written by legacy imports.
	if e.Pattern == host {
		return true
	}
	matched, _ := regexp.MatchString(wildcardToRegex(e.Pattern), strings.ToLower(host))
	return matched
}

// InspectLegacyOAuth reads persisted user data without executing command/keyring
// sources, refreshing tokens, or accepting the legacy plaintext fallback. An
// unreadable field needs investigation; it is never evidence of a valid grant.
func (m *Manager) InspectLegacyOAuth(ctx context.Context, resourceHosts ...string) ([]LegacyOAuthEntry, error) {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	store, ok := m.store.(*DBStore)
	if !ok || !m.persist {
		return nil, errors.New("oauth_vault_persistence_required")
	}
	db, err := store.ensureDB()
	if err != nil {
		return nil, err
	}
	var rows []database.CredentialEntry
	if err := db.WithContext(ctx).Where("user_id = ?", user).
		Where("pattern LIKE ? OR pattern LIKE ? OR auth_type IN ?", "mcp-client:%", "mcp-tokens:%", []string{"oauth2", "bearer"}).
		Order("pattern").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]LegacyOAuthEntry, 0, len(rows))
	for _, row := range rows {
		if strings.HasPrefix(row.Pattern, "oauth:") {
			continue
		}
		entry := LegacyOAuthEntry{Pattern: row.Pattern, Source: row.Source, AuthType: row.AuthType, ClientGrantType: row.ClientGrantType}
		// Imported MCP tokens can be stored as hostname bearer credentials. Only
		// inspect these when a consumer host matches; unrelated API keys stay out.
		if row.AuthType == "bearer" && !strings.HasPrefix(row.Pattern, "mcp-client:") && !strings.HasPrefix(row.Pattern, "mcp-tokens:") {
			matches := false
			for _, host := range resourceHosts {
				if entry.MatchesHost(host) {
					matches = true
					break
				}
			}
			if !matches {
				continue
			}
		}
		// External sources are configuration, not stored OAuth grants.
		if row.Source != "" && row.Source != "static" {
			result = append(result, entry)
			continue
		}
		entry.Readable = true
		values := make([]string, 4)
		for i, encrypted := range []string{row.TokenEnc, row.RefreshTokenEnc, row.ClientIDEnc, row.ClientSecretEnc} {
			if encrypted == "" {
				continue
			}
			values[i], err = m.decrypt(encrypted)
			if err != nil {
				entry.Readable = false
				break
			}
		}
		if entry.Readable {
			entry.HasAccess, entry.HasRefresh = values[0] != "", values[1] != ""
			entry.ClientID, entry.HasSecret = values[2], values[3] != ""
		}
		result = append(result, entry)
	}
	return result, ctx.Err()
}
