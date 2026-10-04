package mcp

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"bytes"
	"errors"
	"gorm.io/gorm"
	"testing"
)

func TestMCPCredentialSaveAtomicFailure(t *testing.T) {
	for _, mode := range []string{"create", "edit", "oauth"} {
		t.Run(mode, func(t *testing.T) {
			m, repo, ctx := managedFixture(t)
			cfg := ServerConfig{Name: "Before", URL: "https://example.com/mcp", Transport: TransportStreamable, AuthType: AuthBearer, Enabled: true}
			if mode == "edit" {
				if err := m.SaveConfig("test", cfg); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "oauth" {
				if err := m.SaveConfigWithOAuthSecret("test", managedConfig(cfg.URL), "client-secret"); err != nil {
					t.Fatal(err)
				}
			}
			old, oldErr := repo.GetServer(ctx, "test")
			if err := m.credMgr.RegisterPatternWithContext(ctx, "example.com", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "before"}); err != nil {
				t.Fatal(err)
			}
			var beforeRow database.CredentialEntry
			if err := repo.db.Where("pattern = ?", "example.com").First(&beforeRow).Error; err != nil {
				t.Fatal(err)
			}
			for _, failTable := range []string{"credential_entries", "mcp_servers"} {
				sentinel := errors.New("injected persistence failure")
				hook := "test:atomic-fail"
				fail := func(tx *gorm.DB) {
					if tx.Statement.Table == failTable {
						_ = tx.AddError(sentinel)
					}
				}
				if err := repo.db.Callback().Create().Before("gorm:create").Register(hook, fail); err != nil {
					t.Fatal(err)
				}
				if err := repo.db.Callback().Update().Before("gorm:update").Register(hook, fail); err != nil {
					t.Fatal(err)
				}
				cfg.Name = "After"
				err := m.SaveConfigWithCredential(ctx, "test", cfg, "example.com", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "after"})
				_ = repo.db.Callback().Create().Remove(hook)
				_ = repo.db.Callback().Update().Remove(hook)
				if !errors.Is(err, sentinel) {
					t.Fatalf("%s failure in %s: got %v, want injected persistence error", mode, failTable, err)
				}
				got, getErr := repo.GetServer(ctx, "test")
				if oldErr != nil {
					if !errors.Is(getErr, gorm.ErrRecordNotFound) {
						t.Fatal("created partial server")
					}
				} else if getErr != nil || got.Name != old.Name || got.OAuthAuthorizationID != old.OAuthAuthorizationID {
					t.Fatalf("server changed on failure: %#v %v", got, getErr)
				}
				resolved, err := m.credMgr.GetByPatternWithContext(ctx, "example.com")
				if err != nil || resolved.Token != "before" {
					t.Fatal("cache changed on failure")
				}
				var afterRow database.CredentialEntry
				if err := repo.db.Where("id = ?", beforeRow.ID).First(&afterRow).Error; err != nil {
					t.Fatal(err)
				}
				if afterRow.TokenEnc != beforeRow.TokenEnc || afterRow.SourceConfigEnc != beforeRow.SourceConfigEnc {
					t.Fatal("persisted credential changed on rollback")
				}
				if mode == "oauth" {
					store, err := m.credMgr.OAuthStore(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = store.Load(ctx, old.OAuthAuthorizationID); err != nil {
						t.Fatal("old grant lost on failure")
					}
				}
			}
			if err := m.SaveConfigWithCredential(ctx, "test", cfg, "example.com", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "after"}); err != nil {
				t.Fatal(err)
			}
			got, err := m.GetConfig("test")
			if err != nil || got.Name != "After" || got.OAuthAuthorizationID != "" || got.AuthType != AuthBearer {
				t.Fatalf("new config missing: %#v %v", got, err)
			}
			resolved, err := m.credMgr.GetByPatternWithContext(ctx, "example.com")
			if err != nil || resolved.Token != "after" {
				t.Fatal("new credential missing")
			}
			if mode == "oauth" {
				var count int64
				repo.db.Model(&database.CredentialEntry{}).Where("id = ?", old.OAuthAuthorizationID).Count(&count)
				if count != 0 {
					t.Fatal("old grant survived replacement")
				}
			}
		})
	}
}

func TestMCPCredentialSaveRejectsSessionChangeDuringPreflight(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	changed := false
	if err := repo.db.Callback().Query().Before("gorm:query").Register("test:change-vault", func(tx *gorm.DB) {
		if !changed && tx.Statement.Table == "mcp_servers" {
			changed = true
			m.credMgr.Reset(bytes.Repeat([]byte{9}, 32), true)
		}
	}); err != nil {
		t.Fatal(err)
	}
	cfg := ServerConfig{Name: "Test", URL: "https://example.com", Transport: TransportStreamable, AuthType: AuthBearer}
	err := m.SaveConfigWithCredential(ctx, "test", cfg, "example.com", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "old-session-input"})
	_ = repo.db.Callback().Query().Remove("test:change-vault")
	if !changed || !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("stale session accepted: changed=%v err=%v", changed, err)
	}
	var count int64
	repo.db.Model(&database.CredentialEntry{}).Count(&count)
	if count != 0 {
		t.Fatal("stale credential persisted")
	}
	repo.db.Model(&database.MCPServer{}).Count(&count)
	if count != 0 {
		t.Fatal("stale server persisted")
	}
	if len(m.servers) != 0 {
		t.Fatal("stale server published")
	}
}
