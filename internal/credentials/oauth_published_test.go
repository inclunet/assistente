package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const publishedOAuthOwner = "018f0000-0000-7000-8000-000000000001"

// Import frozen SQL before applying current models. The original ciphertexts
// must never be recreated through today's serializers, which would hide drift.
func publishedOAuthFixture(t *testing.T, release, variant string) (*Manager, *gorm.DB, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "published.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	for _, path := range []string{filepath.Join("..", "database", "testdata", "published", release+".sql"), "testdata/oauth-legacy.sql"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(raw)).Error; err != nil {
			t.Fatal(err)
		}
	}
	if db.Migrator().HasColumn("credential_entries", "source") || db.Migrator().HasColumn("mcp_servers", "o_auth_managed") {
		t.Fatal("fixture already contains the new OAuth format")
	}
	switch variant {
	case "client_credentials":
		if err := db.Exec("UPDATE mcp_servers SET auth_type = 'oauth2_client_credentials', o_auth2_client_id = '' WHERE id = 'fixture-server'").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("DELETE FROM credential_entries WHERE id = 'fixture-token-row'").Error; err != nil {
			t.Fatal(err)
		}
	case "tokens_only":
		if err := db.Exec("DELETE FROM credential_entries WHERE id = 'fixture-client-row'").Error; err != nil {
			t.Fatal(err)
		}
	case "unreadable":
		if err := db.Exec("UPDATE credential_entries SET client_secret_enc = 'invalid-ciphertext' WHERE id = 'fixture-client-row'").Error; err != nil {
			t.Fatal(err)
		}
	}
	// Only the two tables under recovery are upgraded here. Full application
	// migrations remain covered by database/published_upgrade_test.go.
	type baseline struct {
		columns []string
		rows    []map[string]interface{}
	}
	baselines := map[string]baseline{}
	for _, table := range []string{"credential_entries", "mcp_servers"} {
		columns, err := db.Migrator().ColumnTypes(table)
		if err != nil {
			t.Fatal(err)
		}
		var before baseline
		for _, column := range columns {
			before.columns = append(before.columns, column.Name())
		}
		if err := db.Table(table).Select(before.columns).Order("id").Find(&before.rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(before.rows) == 0 {
			t.Fatal("empty historical baseline", table)
		}
		baselines[table] = before
	}
	for range 2 {
		if err := db.AutoMigrate(&database.CredentialEntry{}, &database.MCPServer{}); err != nil {
			t.Fatal(err)
		}
		for table, before := range baselines {
			var after []map[string]interface{}
			if err := db.Table(table).Select(before.columns).Order("id").Find(&after).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.rows, after) {
				t.Fatal("upgrade changed historical rows", table)
			}
		}
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	m := NewManagerWithStore(key, &DBStore{db: db}, true)
	ctx := database.WithUserID(context.Background(), publishedOAuthOwner)
	if err := m.LoadUserCredentials(ctx, publishedOAuthOwner); err != nil {
		t.Fatal(err)
	}
	return m, db, ctx
}

func TestPublishedOAuthRecovery(t *testing.T) {
	for _, release := range []string{"0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		for _, variant := range []string{"pkce", "client_credentials", "tokens_only", "unreadable"} {
			t.Run(release+"/"+variant, func(t *testing.T) {
				m, db, ctx := publishedOAuthFixture(t, release, variant)
				rows := func() []database.CredentialEntry {
					t.Helper()
					var rows []database.CredentialEntry
					if err := db.Order("id").Find(&rows).Error; err != nil {
						t.Fatal(err)
					}
					return rows
				}
				before := rows()
				// AEP-0110 deliberately refuses source-less generic resolution.
				// Verify cryptographic readability separately, without inferring a source.
				for _, row := range before {
					if row.ID != "fixture-client-row" && row.ID != "fixture-token-row" {
						continue
					}
					if row.Source != "" {
						t.Fatal("upgrade silently invented source")
					}
					if auth, err := m.GetByPatternWithContext(ctx, row.Pattern); err == nil || auth != nil {
						t.Fatal("source-less credential resolved")
					}
					values := map[string]string{}
					if row.ID == "fixture-client-row" && variant != "unreadable" {
						values[row.ClientIDEnc] = "fixture-client"
						values[row.ClientSecretEnc] = "fixture-secret"
					}
					if row.ID == "fixture-token-row" {
						values[row.TokenEnc] = "fixture-access"
						values[row.RefreshTokenEnc] = "fixture-refresh"
						if row.ExpiresAt != 1790038800 {
							t.Fatal("historical expiry changed")
						}
					}
					for encrypted, want := range values {
						got, err := m.decrypt(encrypted)
						if err != nil || got != want {
							t.Fatal("historical ciphertext unreadable", err)
						}
					}
				}
				dir := filepath.Join(t.TempDir(), "recovery")
				info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "fixture-server")
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, rows()) {
					t.Fatal("capture changed historical rows")
				}
				raw, err := os.ReadFile(info.Location)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(raw, []byte("fixture-")) {
					t.Fatal("snapshot contains plaintext fixture data")
				}
				plain, err := m.decrypt(string(raw))
				if err != nil {
					t.Fatal(err)
				}
				var snapshot legacySnapshot
				if err := json.Unmarshal([]byte(plain), &snapshot); err != nil {
					t.Fatal(err)
				}
				wantAuth, wantSchema := "oauth2_pkce", "legacy-pkce-v1"
				if variant == "client_credentials" {
					wantAuth, wantSchema = "oauth2_client_credentials", "legacy-client-credentials-v1"
				}
				if snapshot.Schema != wantSchema || snapshot.Consumer.AuthType != wantAuth {
					t.Fatal("snapshot changed grant or recovery schema")
				}
				var expected []database.CredentialEntry
				for _, row := range before {
					if row.ID == "fixture-client-row" || row.ID == "fixture-token-row" {
						expected = append(expected, row)
					}
				}
				if len(snapshot.Credentials) != len(expected) {
					t.Fatal("snapshot includes unrelated rows or loses partial data")
				}
				for _, row := range expected {
					found := false
					for _, captured := range snapshot.Credentials {
						if reflect.DeepEqual(row, captured.Entry) {
							found = true
						}
					}
					if !found {
						t.Fatal("snapshot did not preserve original encrypted row", row.ID)
					}
				}
				other := database.WithUserID(context.Background(), "018f0000-0000-7000-8000-000000000002")
				if err := m.RestoreLegacyOAuthSnapshot(other, dir, info.ID, nil); !errors.Is(err, ErrSnapshot) {
					t.Fatal("cross-user recovery accepted", err)
				}
				if err := m.ClearLegacyOAuth(ctx, "published", "fixture-server", ""); err != nil {
					t.Fatal(err)
				}
				afterClear := rows()
				err = m.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil)
				if variant == "unreadable" {
					if !errors.Is(err, ErrSnapshot) || !reflect.DeepEqual(afterClear, rows()) {
						t.Fatal("unreadable recovery was not atomic", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var consumer database.MCPServer
				if err := db.First(&consumer, "id = ?", "fixture-server").Error; err != nil {
					t.Fatal(err)
				}
				if consumer.AuthType != wantAuth {
					t.Fatal("recovery changed grant")
				}
				wantConfiguredID := "fixture-client"
				if variant == "client_credentials" {
					wantConfiguredID = ""
				}
				if consumer.OAuth2ClientID != wantConfiguredID {
					t.Fatal("recovery changed persisted client ID")
				}
				if consumer.Enabled || consumer.AutoConnect || consumer.OAuth2CallbackPort != 3128 || consumer.OAuth2CallbackHost != "localhost" || consumer.OAuth2Scopes != `["read","offline_access"]` || !consumer.PreferBridge {
					t.Fatal("consumer settings were not recovered safely")
				}
				if consumer.OAuth2AuthURL != "https://identity.example/authorize" || consumer.OAuth2TokenURL != "https://identity.example/token" || consumer.OAuth2RegistrationURL != "https://identity.example/register" || consumer.OAuth2DeviceAuthURL != "https://identity.example/device" {
					t.Fatal("OAuth endpoints changed")
				}
				recovered := rows()
				clientCount, tokenCount := 0, 0
				for _, row := range recovered {
					if row.UserID != publishedOAuthOwner {
						continue
					}
					if row.Pattern == "mcp-client:published" {
						clientCount++
						if row.Source != "static" || row.AuthType != "oauth2" || row.LegacyOAuthControlEnc != "" || row.TokenEnc != "" || row.RefreshTokenEnc != "" {
							t.Fatal("invalid recovered registration")
						}
					}
					if row.Pattern == "mcp-tokens:published" {
						tokenCount++
						if row.TokenEnc != "" || row.RefreshTokenEnc != "" {
							t.Fatal("old tokens reactivated")
						}
						var marker legacyOAuthControl
						plain, err := m.decrypt(row.LegacyOAuthControlEnc)
						if err != nil || json.Unmarshal([]byte(plain), &marker) != nil || !marker.Pending || marker.ConsumerID != consumer.ID {
							t.Fatal("PKCE recovery lacks pending marker")
						}
					}
				}
				wantClients, wantTokens := 1, 1
				if variant == "tokens_only" {
					wantClients = 0
				}
				if variant == "client_credentials" {
					wantTokens = 0
				}
				if clientCount != wantClients || tokenCount != wantTokens {
					t.Fatal("incorrect recovered credential topology", clientCount, tokenCount)
				}
				for _, original := range before {
					if original.ID == "fixture-client-row" || original.ID == "fixture-token-row" {
						continue
					}
					found := false
					for _, row := range recovered {
						if reflect.DeepEqual(row, original) {
							found = true
						}
					}
					if !found {
						t.Fatal("unrelated credential changed or disappeared", original.ID)
					}
				}
				if err := m.LoadUserCredentials(ctx, publishedOAuthOwner); err != nil {
					t.Fatal(err)
				}
				client, err := m.GetByPatternWithContext(ctx, "mcp-client:published")
				if err != nil {
					t.Fatal(err)
				}
				if variant == "tokens_only" {
					if client != nil {
						t.Fatal("invented missing client")
					}
				} else if client == nil || client.ClientID != "fixture-client" || client.ClientSecret != "fixture-secret" {
					t.Fatal("registration lost after reload")
				}
				if variant != "client_credentials" {
					if control := readLegacyControlFixture(t, m, ctx, "published"); !control.Pending || control.ConsumerID != "fixture-server" {
						t.Fatal("PKCE did not require reauthorization")
					}
				}
				if err := m.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
					t.Fatal("repeated restore was accepted", err)
				}
			})
		}
	}
}
