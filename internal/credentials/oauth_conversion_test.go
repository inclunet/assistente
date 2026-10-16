package credentials

import (
	"sync"
	"testing"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

func TestPublishedClientConversionPreservesHistoricalSecrets(t *testing.T) {
	for _, release := range []string{"0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(release, func(t *testing.T) {
			m, db, ctx := publishedOAuthFixture(t, release, "client_credentials")
			dir := t.TempDir()
			info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "fixture-server")
			if err != nil {
				t.Fatal(err)
			}
			prepare := func(row database.MCPServer, auth *AuthConfig, id string) (oauthflow.Record, database.MCPServer, error) {
				record := oauthflow.Record{Version: 1, Revision: 1, ID: id, UserID: row.UserID, ConsumerID: row.ID, Integration: "mcp", GrantType: "client_credentials", State: "pending", Resource: row.URL, Client: oauthflow.ClientRegistration{ID: auth.ClientID, Secret: auth.ClientSecret, Method: "manual", AuthMethod: "client_secret_post"}}
				row.OAuthManaged, row.OAuthAuthorizationID = true, id
				row.OAuth2ClientID, row.OAuth2TokenURL = "", ""
				return record, row, nil
			}
			// Two callers using the same snapshot converge to one authorization.
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for range 2 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs <- m.ConvertLegacyClientCredentials(ctx, dir, info.ID, "client_secret_post", prepare, nil)
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			var row database.MCPServer
			if err := db.First(&row, "id = ?", "fixture-server").Error; err != nil {
				t.Fatal(err)
			}
			store, err := m.OAuthStore(ctx)
			if err != nil {
				t.Fatal(err)
			}
			record, err := store.Load(ctx, row.OAuthAuthorizationID)
			if err != nil || record.Client.ID != "fixture-client" || record.Client.Secret != "fixture-secret" {
				t.Fatal("historical secret lost", err)
			}
			var count int64
			if err := db.Model(&database.CredentialEntry{}).Where("source = ?", "oauth").Count(&count).Error; err != nil || count != 1 {
				t.Fatal("not idempotent", err)
			}
		})
	}
}
