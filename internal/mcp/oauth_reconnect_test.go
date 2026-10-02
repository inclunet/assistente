package mcp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

func TestReconnectMigrationSuccessFailureAndRetry(t *testing.T) {
	for _, outcome := range []string{"success", "dcr", "denied", "commit_failure", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			var exchanges atomic.Int32
			var registrations atomic.Int32
			var callback string
			clientID, clientSecret := "client", "secret"
			if outcome == "dcr" {
				clientID, clientSecret = "registered", ""
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/register" {
					registrations.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"client_id":"registered"}`)
					return
				}
				if r.URL.Path != "/token" {
					http.NotFound(w, r)
					return
				}
				exchanges.Add(1)
				_ = r.ParseForm()
				if r.Form.Get("client_id") != clientID || r.Form.Get("client_secret") != clientSecret || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("redirect_uri") != callback {
					t.Error("incorrect grant request")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"NEW-ACCESS","refresh_token":"NEW-REFRESH","token_type":"Bearer","expires_in":3600,"scope":"read"}`)
			}))
			defer server.Close()
			a, b, ctx, cfg := legacyWALManagers(t, server.URL)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			a.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
				return d.IPs, true, nil
			})
			cfg.OAuth2AuthURL = server.URL + "/authorize"
			cfg.OAuth2Scopes = []string{"read"}
			if outcome == "dcr" {
				occupied, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = occupied.Close() }()
				cfg.OAuth2CallbackHost = "127.0.0.1"
				cfg.OAuth2CallbackPort = occupied.Addr().(*net.TCPAddr).Port
				cfg.OAuth2RegistrationURL = server.URL + "/register"
			}
			if err := a.SaveConfig(cfg.Slug, cfg); err != nil {
				t.Fatal(err)
			}
			if err := a.credMgr.RegisterPatternWithContext(ctx, clientCredPattern(cfg.Slug), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
				t.Fatal(err)
			}
			a.snapshotRoot = t.TempDir()
			b.snapshotRoot = a.snapshotRoot
			info, err := a.CreateOAuthSnapshot(ctx, cfg.ID)
			if err != nil {
				t.Fatal(err)
			}
			db := a.repository().(*DBRepository).db
			var before database.CredentialEntry
			if err := db.First(&before, "user_id = ? AND pattern = ?", "owner", userTokensPattern(cfg.Slug)).Error; err != nil {
				t.Fatal(err)
			}
			old := browserOpen
			defer func() { browserOpen = old }()
			browserOpen = func(raw string) error {
				u, _ := url.Parse(raw)
				q := u.Query()
				callback = q.Get("redirect_uri")
				var during database.CredentialEntry
				if err := db.First(&during, "id = ?", before.ID).Error; err != nil || during.TokenEnc != before.TokenEnc || during.RefreshTokenEnc != before.RefreshTokenEnc {
					t.Error("old tokens changed during consent")
				}
				if strings.Contains(during.LegacyOAuthControlEnc, "secret") {
					t.Error("staging not encrypted")
				}
				if op, _, err := b.credMgr.BeginLegacyOAuth(ctx, cfg.Slug, cfg.ID, false, false, nil); err == nil {
					op.End()
					t.Error("concurrent refresh accepted")
				}
				if err := b.ReconnectOAuthSnapshot(ctx, info.ID, "client_secret_post"); err == nil {
					t.Error("concurrent reconnection accepted")
				}
				if outcome == "cancelled" {
					cancel()
					return nil
				}
				result := "&code=CODE"
				if outcome == "denied" {
					result = "&error=access_denied"
				}
				resp, err := http.Get(callback + "?state=" + url.QueryEscape(q.Get("state")) + result)
				if err != nil {
					return err
				}
				_ = resp.Body.Close()
				return nil
			}
			if outcome == "commit_failure" {
				if err := db.Callback().Create().Before("gorm:create").Register("reject_reconnect", func(tx *gorm.DB) {
					if tx.Statement.Table == "credential_entries" {
						_ = tx.AddError(errors.New("injected"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = db.Callback().Create().Remove("reject_reconnect") }()
			}
			err = a.ReconnectOAuthSnapshot(ctx, info.ID, "client_secret_post")
			if outcome != "success" && outcome != "dcr" {
				if err == nil {
					t.Fatal("failure accepted")
				}
				var after database.CredentialEntry
				if err := db.First(&after, "id = ?", before.ID).Error; err != nil || after.TokenEnc != before.TokenEnc || after.RefreshTokenEnc != before.RefreshTokenEnc || after.LegacyOAuthControlEnc != before.LegacyOAuthControlEnc {
					t.Fatal("failed migration changed old grant", err)
				}
				var row database.MCPServer
				_ = db.First(&row, "id = ?", cfg.ID).Error
				if row.OAuthManaged || row.OAuthAuthorizationID != "" {
					t.Fatal("failure switched consumer")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_, _, r := loadManaged(t, a, ctx, cfg.Slug)
			if r.Tokens.Access != "NEW-ACCESS" || r.Tokens.Refresh != "NEW-REFRESH" || r.Client.Secret != clientSecret || r.Client.ID != clientID || r.Callback.Port == 0 || len(r.GrantedScopes) != 1 || r.GrantedScopes[0] != "read" {
				t.Fatal("new grant metadata lost")
			}
			if err := b.ReconnectOAuthSnapshot(ctx, info.ID, "client_secret_post"); err != nil {
				t.Fatal("repeat", err)
			}
			if exchanges.Load() != 1 {
				t.Fatal("repeat authorized again")
			}
			if outcome == "dcr" && (registrations.Load() != 1 || r.Client.AuthMethod != "none") {
				t.Fatal("DCR fallback not covered")
			}
			if err := b.ReconnectOAuthSnapshot(ctx, info.ID, "none"); err == nil {
				t.Fatal("retry ignored changed choice")
			}
			if _, err := os.Stat(info.Location); err != nil {
				t.Fatal("snapshot removed", err)
			}
			var count int64
			db.Model(&database.CredentialEntry{}).Where("pattern IN ?", []string{clientCredPattern(cfg.Slug), userTokensPattern(cfg.Slug)}).Count(&count)
			if count != 0 {
				t.Fatal("legacy pair retained")
			}
			if _, err := b.credMgr.ReadLegacyOAuthToken(ctx, cfg.Slug, cfg.ID); err == nil {
				t.Fatal("old runtime still resolves")
			}
		})
	}
}
