package credentials

import (
	"context"
	"encoding/json"
	"strings"

	"assistente/internal/database"
)

// ManagedCredentialSummary is an allowlisted UI projection, never a secret record.
// Listing does not resolve sources, refresh tokens or contact consumers.
type ManagedCredentialSummary struct {
	ID          string `json:"id"`
	Pattern     string `json:"pattern"`
	Source      string `json:"source"`
	Integration string `json:"integration"`
	ConsumerID  string `json:"consumerId"`
	State       string `json:"state"`
	Unreadable  bool   `json:"unreadable"`
}

func (m *Manager) ListManagedCredentials(ctx context.Context) ([]ManagedCredentialSummary, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ManagedCredentialSummary, 0)
	for _, dc := range m.credentials {
		if dc.UserID != userID {
			continue
		}
		oauth := strings.HasPrefix(dc.Pattern, "oauth:") && dc.Auth.Source == "oauth"
		connection := strings.HasPrefix(dc.Pattern, "connection:") && dc.Auth.Type == StaticConnectionType
		if !oauth && !connection {
			continue
		}
		row := ManagedCredentialSummary{ID: dc.ID, Pattern: dc.Pattern, Source: dc.Auth.Source, Unreadable: true}
		auth, decryptErr := m.decryptAuthRaw(dc.Auth)
		if decryptErr == nil && oauth && auth.OAuth != nil {
			r := auth.OAuth
			if r.Version == 1 && r.ID == dc.ID && r.UserID == userID && dc.Pattern == "oauth:"+r.ID {
				row.Integration, row.ConsumerID, row.State, row.Unreadable = r.Integration, r.ConsumerID, r.Summary().State, false
			}
		} else if decryptErr == nil && connection {
			var r staticConnectionRecord
			if json.Unmarshal([]byte(auth.Token), &r) == nil && r.Version == 1 && r.ID == dc.ID && r.UserID == userID && dc.Pattern == StaticConnectionPattern(r.ID) {
				row.Integration, row.ConsumerID, row.State, row.Unreadable = r.Integration, r.ConsumerID, "stored", false
			}
		}
		result = append(result, row)
	}
	return result, nil
}
