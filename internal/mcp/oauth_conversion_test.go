package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

func TestClientConversionAtomicIdempotentAndRestart(t *testing.T) {
	for _, method := range []string{"client_secret_basic", "client_secret_post"} {
		t.Run(method, func(t *testing.T) {
			var grants atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					grants.Add(1)
					_ = r.ParseForm()
					id, secret, basic := r.BasicAuth()
					if method == "client_secret_post" {
						id, secret = r.Form.Get("client_id"), r.Form.Get("client_secret")
						if basic {
							t.Error("unexpected basic")
						}
					} else if !basic || r.Form.Get("client_id") != "" {
						t.Error("wrong basic encoding")
					}
					if id != "client" || secret != "secret" {
						t.Error("lost credentials")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"converted-token","token_type":"Bearer","expires_in":3600}`)
					return
				}
				if r.Header.Get("Authorization") != "Bearer converted-token" {
					t.Error("wrong token")
				}
				_, _ = io.WriteString(w, "ok")
			}))
			defer server.Close()
			a, b, ctx, cfg := legacyClientFixture(t, server.URL)
			if method == "client_secret_basic" {
				if err := a.credMgr.RegisterPatternWithContext(ctx, clientCredPattern(cfg.Slug), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientSecret: "secret"}); err != nil {
					t.Fatal(err)
				}
			} else {
				cfg.OAuth2ClientID = ""
				if err := a.SaveConfig(cfg.Slug, cfg); err != nil {
					t.Fatal(err)
				}
			}
			a.snapshotRoot = t.TempDir()
			b.snapshotRoot = a.snapshotRoot
			old := b.buildAuthHTTPClient(ctx, cfg.Slug, cfg)
			if err := a.credMgr.RegisterPatternWithContext(ctx, "shared.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "shared-token"}); err != nil {
				t.Fatal(err)
			}
			info, err := a.CreateOAuthSnapshot(ctx, cfg.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.ConvertOAuthClientSnapshot(ctx, info.ID, method); err != nil {
				t.Fatal(err)
			}
			if grants.Load() != 0 {
				t.Fatal("conversion contacted provider")
			}
			legacyCtx, legacyCancel := context.WithCancel(ctx)
			defer legacyCancel()
			b.connections[cfg.Slug] = &serverConnection{cancelSession: legacyCancel}
			if err = b.ConvertOAuthClientSnapshot(ctx, info.ID, method); err != nil {
				t.Fatal("repeat failed", err)
			}
			if legacyCtx.Err() == nil {
				t.Fatal("repeat kept legacy connection")
			}
			otherMethod := "client_secret_post"
			if method == otherMethod {
				otherMethod = "client_secret_basic"
			}
			if err = b.ConvertOAuthClientSnapshot(ctx, info.ID, otherMethod); err == nil {
				t.Fatal("repeat silently ignored changed method")
			}
			attemptCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			b.managedAttempts = make(map[string]context.CancelFunc)
			b.managedAttempts[cfg.Slug] = cancel
			b.connections[cfg.Slug] = &serverConnection{cancelSession: cancel, oauthAuthorizationID: "managed"}
			if err = b.ConvertOAuthClientSnapshot(ctx, info.ID, method); err != nil || attemptCtx.Err() != nil {
				t.Fatal("repeat interrupted managed attempt", err)
			}
			db := a.repository().(*DBRepository).db
			var row database.MCPServer
			if err := db.First(&row, "id = ?", cfg.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !row.OAuthManaged || row.OAuthAuthorizationID == "" || row.OAuth2ClientID != "" || row.OAuth2TokenURL != "" {
				t.Fatal("legacy config retained")
			}
			var count int64
			if err := db.Model(&database.CredentialEntry{}).Where("pattern IN ?", []string{clientCredPattern(cfg.Slug), userTokensPattern(cfg.Slug)}).Count(&count).Error; err != nil || count != 0 {
				t.Fatal("legacy residue", err)
			}
			if _, err = os.Stat(info.Location); err != nil {
				t.Fatal("snapshot removed")
			}
			if err = clientGrantGet(old, server.URL); err == nil {
				t.Fatal("old transport survived cutover")
			}
			if err = b.credMgr.LoadUserCredentials(ctx, cfg.UserID); err != nil {
				t.Fatal(err)
			}
			store, err := b.credMgr.OAuthStore(ctx)
			if err != nil {
				t.Fatal(err)
			}
			record, err := store.Load(ctx, row.OAuthAuthorizationID)
			if err != nil || record.Client.Secret != "secret" || record.Tokens.Access != "" || record.Client.AuthMethod != method {
				t.Fatal("wrong record", err)
			}
			managed, err := serverModelToConfig(row)
			if err != nil {
				t.Fatal(err)
			}
			if err = clientGrantGet(b.managedHTTPClient(ctx, managed), server.URL); err != nil {
				t.Fatal("managed connection failed", err)
			}
			if grants.Load() != 1 {
				t.Fatal("unexpected grant count", grants.Load())
			}
			if token, ok := b.resolveNativeAuthToken(ctx, nativeMCPCandidate{slug: managed.Slug, name: managed.Name, url: managed.URL, authType: managed.AuthType, managedConfig: managed}); !ok || token != "converted-token" {
				t.Fatal("native resolution after conversion failed")
			}
			auth, err := b.credMgr.GetByPatternWithContext(ctx, "shared.example")
			if err != nil || auth == nil || auth.Token != "shared-token" {
				t.Fatal("hostname changed", err)
			}
			if err := a.RestoreOAuthSnapshot(ctx, info.ID); err == nil {
				t.Fatal("restore overwrote converted consumer")
			}
			if err := db.AutoMigrate(&database.ToolCatalog{}); err != nil {
				t.Fatal(err)
			}
			if err := a.DeleteConfig(cfg.Slug); err != nil {
				t.Fatal(err)
			}
			if err := a.RestoreOAuthSnapshot(ctx, info.ID); err != nil {
				t.Fatal("recovery after removal failed", err)
			}
			restored, err := a.GetConfig(cfg.Slug)
			if err != nil || restored.OAuthManaged || restored.Enabled || restored.AutoConnect {
				t.Fatal("unsafe recovery", err)
			}
			recovered, err := a.credMgr.GetByPatternWithContext(ctx, clientCredPattern(cfg.Slug))
			if err != nil || recovered == nil || recovered.ClientSecret != "secret" {
				t.Fatal("recovery lost secret", err)
			}
		})
	}
}

func TestClientConversionRefusesChangedIncompleteAndActiveRecords(t *testing.T) {
	for _, scenario := range []string{"changed_client", "changed_consumer", "access_residue", "refresh_residue", "active", "missing_client", "invalid_method", "other_user"} {
		t.Run(scenario, func(t *testing.T) {
			a, b, ctx, cfg := legacyClientFixture(t, "https://service.example")
			a.snapshotRoot = t.TempDir()
			if scenario == "access_residue" || scenario == "refresh_residue" {
				auth := &credentials.AuthConfig{Source: "static", Type: "oauth2"}
				if scenario == "access_residue" {
					auth.Token = "residual"
				} else {
					auth.RefreshURL = "residual"
				}
				if err := a.credMgr.RegisterPatternWithContext(ctx, userTokensPattern(cfg.Slug), auth); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "missing_client" {
				if err := a.credMgr.DeletePattern(ctx, clientCredPattern(cfg.Slug)); err != nil {
					t.Fatal(err)
				}
			}
			info, err := a.CreateOAuthSnapshot(ctx, cfg.ID)
			if err != nil {
				t.Fatal(err)
			}
			method := "client_secret_post"
			switch scenario {
			case "changed_client":
				err = b.credMgr.RegisterPatternWithContext(ctx, clientCredPattern(cfg.Slug), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "new"})
			case "changed_consumer":
				cfg.URL = "https://different.example"
				err = b.SaveConfig(cfg.Slug, cfg)
			case "active":
				op, _, beginErr := b.credMgr.BeginLegacyClientGrant(ctx, cfg.Slug, cfg.ID, nil)
				if beginErr != nil {
					t.Fatal(beginErr)
				}
				defer op.End()
			case "invalid_method":
				method = ""
			case "other_user":
				ctx = database.WithUserID(context.Background(), "different-user")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := a.ConvertOAuthClientSnapshot(ctx, info.ID, method); err == nil {
				t.Fatal("unsafe conversion accepted")
			}
			var count int64
			if err := a.repository().(*DBRepository).db.Model(&database.CredentialEntry{}).Where("source = ?", "oauth").Count(&count).Error; err != nil || count != 0 {
				t.Fatal("partial conversion", err)
			}
			if _, err := os.Stat(info.Location); err != nil {
				t.Fatal("snapshot lost", err)
			}
		})
	}
}

func TestClientConversionRollsBackAndKeepsSnapshot(t *testing.T) {
	a, _, ctx, cfg := legacyClientFixture(t, "https://service.example")
	a.snapshotRoot = t.TempDir()
	info, err := a.CreateOAuthSnapshot(ctx, cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	db := a.repository().(*DBRepository).db
	if err := db.Callback().Delete().Before("gorm:delete").Register("conversion_fail", func(tx *gorm.DB) { _ = tx.AddError(errors.New("injected")) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Delete().Remove("conversion_fail") })
	if err := a.ConvertOAuthClientSnapshot(ctx, info.ID, "client_secret_post"); err == nil {
		t.Fatal("commit should fail")
	}
	var row database.MCPServer
	if err := db.First(&row, "id = ?", cfg.ID).Error; err != nil || row.OAuthManaged || row.OAuthAuthorizationID != "" {
		t.Fatal("partial consumer", err)
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("source = ?", "oauth").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial authorization", err)
	}
	auth, err := a.credMgr.GetByPatternWithContext(ctx, clientCredPattern(cfg.Slug))
	if err != nil || auth == nil || auth.ClientSecret != "secret" {
		t.Fatal("client lost", err)
	}
	data, err := os.ReadFile(info.Location)
	if err != nil || strings.Contains(string(data), "secret") {
		t.Fatal("snapshot lost or plaintext", err)
	}
}

func TestClientConversionWaitsForLegacyCleanupBeforeNewConnection(t *testing.T) {
	a, _, ctx, cfg := legacyClientFixture(t, "https://service.example")
	a.snapshotRoot = t.TempDir()
	info, err := a.CreateOAuthSnapshot(ctx, cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacyCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	healthDone := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(healthDone) }) }
	defer release()
	a.connections[cfg.Slug] = &serverConnection{cancelSession: cancel, healthDone: healthDone}
	done := make(chan error, 1)
	go func() { done <- a.ConvertOAuthClientSnapshot(ctx, info.ID, "client_secret_post") }()
	select {
	case <-legacyCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("legacy not retired")
	}
	if err := a.connectWithContext(ctx, cfg.Slug); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatal("new connection started before cleanup", err)
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not finish")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.retiringLegacy[cfg.Slug] {
		t.Fatal("retirement barrier retained")
	}
}
