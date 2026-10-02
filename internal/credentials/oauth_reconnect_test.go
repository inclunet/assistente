package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

func TestReconnectSessionChangeCannotCommitGrant(t *testing.T) {
	m, db, ctx := publishedOAuthFixture(t, "0.5.0", "")
	dir := t.TempDir()
	info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "fixture-server")
	if err != nil {
		t.Fatal(err)
	}
	authorize := func(ctx context.Context, store oauthflow.Store, r oauthflow.Record, _ database.MCPServer) error {
		s, err := oauthflow.NewConfigured(r, nil)
		if err != nil {
			return err
		}
		_, err = s.AuthorizeUsing(ctx, store, r.ID, func(_ context.Context, r oauthflow.Record) (oauthflow.Record, error) {
			m.Reset(bytes.Repeat([]byte{9}, 32), true)
			r.Tokens = oauthflow.Tokens{Access: "late-token", Type: "Bearer"}
			r.GrantedScopes = []string{"read"}
			return r, nil
		})
		return err
	}
	if err := m.ReconnectLegacyOAuth(ctx, dir, info.ID, "client_secret_post", reconnectTestPrepare, authorize, reconnectTestProject, nil); err == nil {
		t.Fatal("old session committed")
	}
	var row database.MCPServer
	if err := db.First(&row, "id = ?", "fixture-server").Error; err != nil {
		t.Fatal(err)
	}
	if row.OAuthManaged || row.OAuthAuthorizationID != "" {
		t.Fatal("old session switched consumer")
	}
	var count int64
	db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", publishedOAuthOwner, []string{"mcp-client:published", "mcp-tokens:published"}).Count(&count)
	if count != 2 {
		t.Fatal("old session removed legacy pair")
	}
}

func reconnectTestPrepare(row database.MCPServer, auth *AuthConfig, id string) (oauthflow.Record, error) {
	return oauthflow.Record{Version: 1, Revision: 1, ID: id, UserID: row.UserID, ConsumerID: row.ID, Integration: "mcp", GrantType: "authorization_code", State: "pending", Resource: row.URL, RequestedScopes: []string{"read"},
		Client: oauthflow.ClientRegistration{ID: auth.ClientID, Secret: auth.ClientSecret, AuthMethod: "client_secret_post", Method: "manual"}, Endpoints: oauthflow.Endpoints{Token: row.OAuth2TokenURL}, Callback: oauthflow.CallbackConfig{Host: row.OAuth2CallbackHost, Port: row.OAuth2CallbackPort, Path: "/callback", PortPolicy: "fixed"}}, nil
}
func reconnectTestProject(row database.MCPServer, r oauthflow.Record) (database.MCPServer, error) {
	row.OAuthManaged, row.OAuthAuthorizationID = true, r.ID
	return row, nil
}
func reconnectTestAuthorize(ctx context.Context, store oauthflow.Store, r oauthflow.Record, _ database.MCPServer) error {
	s, err := oauthflow.NewConfigured(r, nil)
	if err != nil {
		return err
	}
	_, err = s.AuthorizeUsing(ctx, store, r.ID, func(_ context.Context, r oauthflow.Record) (oauthflow.Record, error) {
		r.Tokens = oauthflow.Tokens{Access: "new", Refresh: "fresh", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
		r.GrantedScopes = []string{"read"}
		return r, nil
	})
	return err
}

func TestPublishedPKCEReconnectKeepsClientAndReplacesGrant(t *testing.T) {
	for _, release := range []string{"0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(release, func(t *testing.T) {
			m, db, ctx := publishedOAuthFixture(t, release, "")
			dir := t.TempDir()
			info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "fixture-server")
			if err != nil {
				t.Fatal(err)
			}
			run := func(ctx context.Context) error {
				return m.ReconnectLegacyOAuth(ctx, dir, info.ID, "client_secret_post", reconnectTestPrepare, reconnectTestAuthorize, reconnectTestProject, nil)
			}
			if err := run(database.WithUserID(ctx, "another-user")); err == nil {
				t.Fatal("cross-user accepted")
			}
			if err := run(ctx); err != nil {
				t.Fatal(err)
			}
			if err := run(ctx); err != nil {
				t.Fatal("repeat", err)
			}
			var row database.MCPServer
			if err := db.First(&row, "id = ?", "fixture-server").Error; err != nil {
				t.Fatal(err)
			}
			store, _ := m.OAuthStore(ctx)
			r, err := store.Load(ctx, row.OAuthAuthorizationID)
			if err != nil || r.Tokens.Access != "new" || r.Tokens.Refresh != "fresh" || r.Client.ID != "fixture-client" || r.Client.Secret != "fixture-secret" || r.Callback.Port != 3128 {
				t.Fatal("migration lost client or new grant", err)
			}
			var count int64
			db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", publishedOAuthOwner, []string{"mcp-client:published", "mcp-tokens:published"}).Count(&count)
			if count != 0 {
				t.Fatal("legacy pair retained")
			}
			if err := m.ReconnectLegacyOAuth(ctx, dir, info.ID, "none", reconnectTestPrepare, reconnectTestAuthorize, reconnectTestProject, nil); err == nil {
				t.Fatal("repeat ignored new method")
			}
		})
	}
}

func TestReconnectCrashPreservesPendingAndRecoversMissingTokenRow(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "missing"}[missing], func(t *testing.T) {
			m, db, ctx := publishedOAuthFixture(t, "0.5.0", "")
			dir := t.TempDir()
			if missing {
				if err := db.Where("user_id = ? AND pattern = ?", publishedOAuthOwner, "mcp-tokens:published").Delete(&database.CredentialEntry{}).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				body, _ := json.Marshal(legacyOAuthControl{Version: 1, ConsumerID: "fixture-server", Pending: true})
				enc, _ := m.encrypt(string(body))
				if err := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern = ?", publishedOAuthOwner, "mcp-tokens:published").Update("legacy_oauth_control_enc", enc).Error; err != nil {
					t.Fatal(err)
				}
			}
			info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "fixture-server")
			if err != nil {
				t.Fatal(err)
			}
			crash := func(ctx context.Context, store oauthflow.Store, r oauthflow.Record, _ database.MCPServer) error {
				staged := store.(*reconnectStore)
				plain, err := m.decrypt(staged.control)
				if err != nil {
					t.Fatal(err)
				}
				var c legacyOAuthControl
				if err := json.Unmarshal([]byte(plain), &c); err != nil {
					t.Fatal(err)
				}
				if c.Pending == missing {
					t.Fatal("pending barrier changed")
				}
				c.Until = time.Now().Add(-time.Minute)
				body, _ := json.Marshal(c)
				enc, _ := m.encrypt(string(body))
				// Simulate expired durable staging left by a terminated process.
				if err := db.Model(&database.CredentialEntry{}).Where("id = ?", staged.rowID).Update("legacy_oauth_control_enc", enc).Error; err != nil {
					t.Fatal(err)
				}
				return errors.New("crash")
			}
			if err := m.ReconnectLegacyOAuth(ctx, dir, info.ID, "client_secret_post", reconnectTestPrepare, crash, reconnectTestProject, nil); err == nil {
				t.Fatal("crash accepted")
			}
			if !missing {
				if op, _, err := m.BeginLegacyOAuth(ctx, "published", "fixture-server", false, false, nil); !errors.Is(err, oauthflow.ErrReauthorize) {
					if op != nil {
						op.End()
					}
					t.Fatal("uncertain refresh released", err)
				}
			}
			if err := m.ReconnectLegacyOAuth(ctx, dir, info.ID, "client_secret_post", reconnectTestPrepare, reconnectTestAuthorize, reconnectTestProject, nil); err != nil {
				t.Fatal("crash recovery", err)
			}
		})
	}
}

func TestReconnectRejectsChangedSnapshotBeforeAuthorization(t *testing.T) {
	m, db, ctx := publishedOAuthFixture(t, "0.5.0", "")
	dir := t.TempDir()
	info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "fixture-server")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.MCPServer{}).Where("id = ?", "fixture-server").Update("name", "Changed").Error; err != nil {
		t.Fatal(err)
	}
	authorize := func(context.Context, oauthflow.Store, oauthflow.Record, database.MCPServer) error {
		t.Fatal("stale snapshot opened authorization")
		return nil
	}
	if err := m.ReconnectLegacyOAuth(ctx, dir, info.ID, "client_secret_post", reconnectTestPrepare, authorize, reconnectTestProject, nil); !errors.Is(err, ErrSnapshotConflict) {
		t.Fatal(err)
	}
}
