package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestHostnameSnapshotRestoresSecretsWithoutChangingConsumers(t *testing.T) {
	for _, scheme := range []string{"bearer", "oauth2"} {
		for _, source := range []string{"static", ""} {
			t.Run(scheme+"/"+source, func(t *testing.T) {
				m, other, db, ctx, _ := legacyOperationFixture(t)
				if err := db.Model(&database.MCPServer{}).Where("slug = ?", "legacy").Update("url", "https://shared.example/one").Error; err != nil {
					t.Fatal(err)
				}
				second := database.MCPServer{UserID: "owner", Slug: "second", Name: "Second", Transport: "streamable", URL: "https://shared.example/two", AuthType: "oauth2_pkce"}
				if err := db.Create(&second).Error; err != nil {
					t.Fatal(err)
				}
				var consumers []database.MCPServer
				if err := db.Order("id").Find(&consumers).Error; err != nil {
					t.Fatal(err)
				}
				expected := &AuthConfig{Source: "static", Type: scheme, Token: "copied-token", RefreshURL: "copied-refresh", ExpiresAt: 1234, ClientID: "client", ClientSecret: "secret", ClientGrantType: "manual", Username: "name", Password: "password", Headers: map[string]string{"X-Test": "header-secret"}}
				if err := m.RegisterPatternWithContext(ctx, "shared.example", expected); err != nil {
					t.Fatal(err)
				}
				if source == "" {
					if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "shared.example").Update("source", "").Error; err != nil {
						t.Fatal(err)
					}
				}
				var original database.CredentialEntry
				if err := db.Where("pattern = ?", "shared.example").First(&original).Error; err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join(t.TempDir(), "recovery")
				info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "credential:shared.example")
				if err != nil {
					t.Fatal(err)
				}
				if err := other.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
					t.Fatal("overwrote live credential", err)
				}
				if err := other.DeletePattern(ctx, "shared.example"); err != nil {
					t.Fatal(err)
				}
				called := false
				if err := m.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, func(database.MCPServer) { called = true }, func(database.MCPServer) error { called = true; return nil }); err != nil {
					t.Fatal(err)
				}
				if called {
					t.Fatal("hostname recovery published a consumer")
				}
				var restored database.CredentialEntry
				if err := db.Where("pattern = ?", "shared.example").First(&restored).Error; err != nil {
					t.Fatal(err)
				}
				original.Source = "static"
				if !restored.CreatedAt.Equal(original.CreatedAt) || !restored.UpdatedAt.Equal(original.UpdatedAt) {
					t.Fatal("recovery changed timestamps")
				}
				restored.CreatedAt, restored.UpdatedAt = original.CreatedAt, original.UpdatedAt
				if !reflect.DeepEqual(restored, original) {
					t.Fatal("recovery changed encrypted secrets or expiry")
				}
				for _, reload := range []bool{false, true} {
					if reload {
						if err := m.LoadUserCredentials(ctx, "owner"); err != nil {
							t.Fatal(err)
						}
					}
					auth, err := m.GetByPatternWithContext(ctx, "shared.example")
					if err != nil || !reflect.DeepEqual(auth, expected) {
						t.Fatal("restored secrets unavailable", err)
					}
					foreign := database.WithUserID(context.Background(), "foreign")
					if auth, err := m.GetByPatternWithContext(foreign, "shared.example"); err != nil || auth != nil {
						t.Fatal("cross-user token exposure")
					}
				}
				var after []database.MCPServer
				if err := db.Order("id").Find(&after).Error; err != nil || !reflect.DeepEqual(consumers, after) {
					t.Fatal("changed shared consumers", err)
				}
				if err := m.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
					t.Fatal("repeated restore overwrote token", err)
				}
			})
		}
	}
}

func TestHostnameSnapshotRecoveryFailureIsAtomic(t *testing.T) {
	for _, failure := range []string{"unreadable", "rollback", "replacement", "wrong_key", "wrong_user", "expired"} {
		t.Run(failure, func(t *testing.T) {
			m, _, db, ctx, _ := legacyOperationFixture(t)
			if err := m.RegisterPatternWithContext(ctx, "shared.example", &AuthConfig{Source: "static", Type: "bearer", Token: "saved"}); err != nil {
				t.Fatal(err)
			}
			if failure == "unreadable" {
				if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "shared.example").Update("token_enc", "corrupt").Error; err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(t.TempDir(), "recovery")
			info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "credential:shared.example")
			if err != nil {
				t.Fatal(err)
			}
			if err := m.DeletePattern(ctx, "shared.example"); err != nil {
				t.Fatal(err)
			}
			want := ErrSnapshot
			switch failure {
			case "rollback":
				if err := db.Callback().Create().Before("gorm:create").Register("fail_hostname_restore", func(tx *gorm.DB) { _ = tx.AddError(errors.New("injected")) }); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Callback().Create().Remove("fail_hostname_restore") })
			case "replacement":
				if err := m.RegisterPatternWithContext(ctx, "shared.example", &AuthConfig{Source: "static", Type: "bearer", Token: "newer"}); err != nil {
					t.Fatal(err)
				}
				want = ErrSnapshotConflict
			case "wrong_key":
				m.Reset(bytes.Repeat([]byte{8}, 32), true)
			case "wrong_user":
				ctx = database.WithUserID(context.Background(), "foreign")
			case "expired":
				s, err := m.snapshotSession(ctx, dir)
				if err != nil {
					t.Fatal(err)
				}
				defer s.close()
				files, err := s.open()
				if err != nil {
					t.Fatal(err)
				}
				defer files.Close()
				p, err := s.read(ctx, files, info.ID)
				if err != nil {
					t.Fatal(err)
				}
				p.RetainUntil = time.Now().Add(-time.Hour)
				raw, err := json.Marshal(p)
				if err != nil {
					t.Fatal(err)
				}
				enc, err := m.encrypt(string(raw))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(info.Location, []byte(enc), 0600); err != nil {
					t.Fatal(err)
				}
				want = ErrSnapshotConflict
			}
			err = m.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil)
			if err == nil || (failure != "rollback" && !errors.Is(err, want)) {
				t.Fatal("unsafe restore accepted", err)
			}
			var count int64
			if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "shared.example").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if failure == "replacement" {
				if count != 1 {
					t.Fatal("replacement removed")
				}
				auth, err := m.GetByPatternWithContext(ctx, "shared.example")
				if err != nil || auth.Token != "newer" {
					t.Fatal("replacement changed")
				}
			} else if count != 0 {
				t.Fatal("partial restore")
			}
			if failure == "rollback" || failure == "unreadable" {
				if auth, err := m.GetByPatternWithContext(ctx, "shared.example"); err != nil || auth != nil {
					t.Fatal("failed recovery published cache")
				}
			}
		})
	}
}

func TestHostnameSnapshotConcurrentRestore(t *testing.T) {
	m, other, _, ctx, _ := legacyOperationFixture(t)
	if err := m.RegisterPatternWithContext(ctx, "shared.example", &AuthConfig{Source: "static", Type: "bearer", Token: "saved"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "recovery")
	info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "credential:shared.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.DeletePattern(ctx, "shared.example"); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, manager := range []*Manager{m, other} {
		wg.Add(1)
		go func(manager *Manager) {
			defer wg.Done()
			<-start
			results <- manager.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil)
		}(manager)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("concurrent restores both succeeded or failed", success)
	}
}

func TestHostnameSnapshotResolvesIPv6AndCaseAfterRestore(t *testing.T) {
	for _, sample := range []struct{ pattern, url string }{
		{"2001:DB8::1", "https://[2001:db8::1]/mcp"},
		{"2001:db8::1", "https://[2001:DB8::1]:8443/mcp"},
		{"SHARED.EXAMPLE", "https://shared.example:8443/mcp"},
		{"*.EXAMPLE", "https://SHARED.example/mcp"},
	} {
		t.Run(sample.url, func(t *testing.T) {
			m, _, _, ctx, _ := legacyOperationFixture(t)
			if err := m.RegisterPatternWithContext(ctx, sample.pattern, &AuthConfig{Source: "static", Type: "bearer", Token: "copied"}); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "recovery")
			info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "credential:"+sample.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.DeletePattern(ctx, sample.pattern); err != nil {
				t.Fatal(err)
			}
			if err := m.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); err != nil {
				t.Fatal(err)
			}
			for _, reload := range []bool{false, true} {
				if reload {
					if err := m.LoadUserCredentials(ctx, "owner"); err != nil {
						t.Fatal(err)
					}
				}
				auth, err := m.ResolveForURLWithContext(ctx, sample.url)
				if err != nil || auth == nil || auth.Token != "copied" {
					t.Fatal("restored pattern not resolved", err)
				}
				if auth, err := m.ResolveForURLWithContext(ctx, "https://unrelated.invalid"); err != nil || auth != nil {
					t.Fatal("unrelated host matched", err)
				}
			}
		})
	}
}

func TestHostnameSnapshotCaptureIsPrivateAndReadOnly(t *testing.T) {
	for _, pattern := range []string{"shared.example", "*.example", "2001:db8::1"} {
		t.Run(pattern, func(t *testing.T) {
			m, _, db, ctx, _ := legacyOperationFixture(t)
			if err := m.RegisterPatternWithContext(ctx, pattern, &AuthConfig{Source: "static", Type: "oauth2", Token: "hostname-access", RefreshURL: "hostname-refresh", ClientID: "hostname-client", ClientSecret: "hostname-secret"}); err != nil {
				t.Fatal(err)
			}
			var before database.CredentialEntry
			if err := db.Where("user_id = ? AND pattern = ?", "owner", pattern).First(&before).Error; err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "recovery")
			info, err := m.CreateLegacyOAuthSnapshot(ctx, dir, "credential:"+pattern)
			if err != nil {
				t.Fatal(err)
			}
			if info.ConsumerID != "credential:"+pattern || info.Name != pattern {
				t.Fatal("incorrect hostname projection")
			}
			raw, err := os.ReadFile(info.Location)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte("hostname-")) || bytes.Contains(raw, []byte(pattern)) {
				t.Fatal("plaintext in snapshot")
			}
			s, err := m.snapshotSession(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			defer s.close()
			files, err := s.open()
			if err != nil {
				t.Fatal(err)
			}
			defer files.Close()
			p, err := s.read(ctx, files, info.ID)
			if err != nil || p.Schema != "legacy-hostname-v1" || p.HostnameCredential == nil {
				t.Fatal("hostname capture lost encrypted fields", err)
			}
			captured := p.HostnameCredential.Entry
			if !captured.CreatedAt.Equal(before.CreatedAt) || !captured.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatal("capture changed timestamps")
			}
			captured.CreatedAt, captured.UpdatedAt = before.CreatedAt, before.UpdatedAt
			if !reflect.DeepEqual(before, captured) {
				t.Fatal("capture changed persisted fields")
			}
			var after database.CredentialEntry
			if err := db.First(&after, "id = ?", before.ID).Error; err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("capture altered shared entry", err)
			}
			listed, err := m.ListLegacyOAuthSnapshots(ctx, dir)
			if err != nil || len(listed) != 1 || listed[0].ID != info.ID {
				t.Fatal("hostname snapshot not listed", err)
			}
			other := database.WithUserID(context.Background(), "other")
			if _, err := m.CreateLegacyOAuthSnapshot(other, dir, "credential:"+pattern); err == nil {
				t.Fatal("cross-user capture accepted")
			}
			if err := m.DiscardLegacyOAuthSnapshot(ctx, dir, info.ID, false); !errors.Is(err, ErrSnapshotConflict) {
				t.Fatal("discard without explicit confirmation", err)
			}
			if err := m.DiscardLegacyOAuthSnapshot(ctx, dir, info.ID, true); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHostnameSnapshotRejectsExternalAndManagedEntries(t *testing.T) {
	for _, change := range []string{"command", "managed", "control", "url", "source_config"} {
		t.Run(change, func(t *testing.T) {
			m, _, db, ctx, _ := legacyOperationFixture(t)
			pattern := "shared.example"
			auth := &AuthConfig{Source: "static", Type: "bearer", Token: "opaque"}
			switch change {
			case "command":
				auth.Source = "command"
				auth.Token = ""
				auth.SourceConfig = &SourceConfig{Command: "must-not-run"}
			case "managed":
				pattern = "mcp-client:other"
			case "url":
				pattern = "shared.example/private"
			}
			if err := m.RegisterPatternWithContext(ctx, pattern, auth); err != nil {
				t.Fatal(err)
			}
			column := ""
			if change == "control" {
				column = "legacy_oauth_control_enc"
			}
			if change == "source_config" {
				column = "source_config_enc"
			}
			if column != "" {
				if err := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern = ?", "owner", pattern).Update(column, "opaque").Error; err != nil {
					t.Fatal(err)
				}
			}
			if _, err := m.CreateLegacyOAuthSnapshot(ctx, filepath.Join(t.TempDir(), "recovery"), "credential:"+pattern); !errors.Is(err, ErrSnapshot) {
				t.Fatal("unsafe capture accepted", err)
			}
		})
	}
}
