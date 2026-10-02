package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"assistente/internal/oauthsnapshot"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestOAuthSnapshotMissingConsumerRemainsDisabled(t *testing.T) {
	a, _, db, ctx, id := legacyOperationFixture(t)
	dir := filepath.Join(t.TempDir(), "recovery")
	info, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", id).Delete(&database.MCPServer{}).Error; err != nil {
		t.Fatal(err)
	}
	var published database.MCPServer
	if err := a.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, func(row database.MCPServer) { published = row }); err != nil {
		t.Fatal(err)
	}
	var stored database.MCPServer
	if err := db.First(&stored, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Enabled || stored.AutoConnect || published.Enabled || published.AutoConnect {
		t.Fatal("GORM defaults enabled restored consumer")
	}
}

func TestClientCredentialsSnapshotRestoresOnlyRegistration(t *testing.T) {
	for _, removeConsumer := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "removed"}[removeConsumer], func(t *testing.T) {
			a, b, db, ctx, id := legacyOperationFixture(t)
			if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Update("auth_type", "oauth2_client_credentials").Error; err != nil {
				t.Fatal(err)
			}
			if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret", Token: "misplaced-access", RefreshURL: "misplaced-refresh"}); err != nil {
				t.Fatal(err)
			}
			if err := a.RegisterPatternWithContext(ctx, "shared.example", &AuthConfig{Source: "static", Type: "bearer", Token: "shared"}); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "recovery")
			info, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(info.Location)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("secret")) || bytes.Contains(data, []byte("misplaced")) {
				t.Fatal("plaintext snapshot")
			}
			if err := b.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
				t.Fatalf("overwrote current credentials: %v", err)
			}
			if err := a.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
				t.Fatal(err)
			}
			if removeConsumer {
				if err := db.Where("id = ?", id).Delete(&database.MCPServer{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := b.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); err != nil {
				t.Fatal(err)
			}
			var consumer database.MCPServer
			if err := db.First(&consumer, "id = ?", id).Error; err != nil || consumer.Enabled || consumer.AutoConnect {
				t.Fatalf("consumer not disabled: %v", err)
			}
			var rows []database.CredentialEntry
			if err := db.Where("pattern IN ?", []string{"mcp-client:legacy", "mcp-tokens:legacy"}).Find(&rows).Error; err != nil || len(rows) != 1 {
				t.Fatalf("unexpected recovered rows: %d %v", len(rows), err)
			}
			if rows[0].TokenEnc != "" || rows[0].RefreshTokenEnc != "" || rows[0].LegacyOAuthControlEnc != "" {
				t.Fatal("restored stale tokens or interactive marker")
			}
			client, err := b.GetByPatternWithContext(ctx, "mcp-client:legacy")
			if err != nil || client == nil || client.ClientID != "client" || client.ClientSecret != "secret" {
				t.Fatalf("recovered client unavailable before restart: %v", err)
			}
			if err := a.LoadUserCredentials(ctx, "owner"); err != nil {
				t.Fatal(err)
			}
			client, err = a.GetByPatternWithContext(ctx, "mcp-client:legacy")
			if err != nil || client == nil || client.ClientSecret != "secret" {
				t.Fatalf("recovered client unavailable after reload: %v", err)
			}
			shared, err := a.GetByPatternWithContext(ctx, "shared.example")
			if err != nil || shared == nil || shared.Token != "shared" {
				t.Fatal("changed shared hostname credential")
			}
			if err := b.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
				t.Fatalf("repeated restore replaced registration: %v", err)
			}
		})
	}
}

func TestClientCredentialsSnapshotRestoreRollsBackCacheAndDatabase(t *testing.T) {
	a, _, db, ctx, id := legacyOperationFixture(t)
	if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Update("auth_type", "oauth2_client_credentials").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "recovery")
	info, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("fail_cc_restore", func(tx *gorm.DB) { _ = tx.AddError(errors.New("injected")) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("fail_cc_restore") })
	called := false
	if err := a.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, func(database.MCPServer) { called = true }); err == nil {
		t.Fatal("expected rollback")
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 0 || called {
		t.Fatal("partial recovery")
	}
	if auth, err := a.GetByPatternWithContext(ctx, "mcp-client:legacy"); err != nil || auth != nil {
		t.Fatal("uncommitted registration published to cache")
	}
}

func TestClientCredentialsSnapshotRejectsUnreadableRegistration(t *testing.T) {
	a, _, db, ctx, id := legacyOperationFixture(t)
	if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Update("auth_type", "oauth2_client_credentials").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "mcp-client:legacy").Update("client_secret_enc", "unreadable").Error; err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "recovery")
	info, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	if err := a.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("unreadable registration accepted: %v", err)
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("unreadable recovery modified database")
	}
}

func TestOAuthSnapshotPublicationRefusesEndedSession(t *testing.T) {
	a, _, _, ctx, _ := legacyOperationFixture(t)
	s, err := a.snapshotSession(ctx, filepath.Join(t.TempDir(), "recovery"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	files, err := oauthsnapshot.Open(s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	id := uuid.NewString()
	go func() {
		done <- files.WriteGuarded(id, []byte("opaque"), func(publish func() error) error {
			close(ready)
			<-release
			return s.store.WithSession(ctx, publish)
		})
	}()
	<-ready // file write and Sync completed, publication has not begun
	reset := make(chan struct{})
	go func() { a.Reset(bytes.Repeat([]byte{7}, 32), true); close(reset) }()
	select {
	case <-reset:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("file I/O blocked vault reset")
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("published after logout/key reset")
	}
	ids, err := files.List()
	if err != nil || len(ids) != 0 {
		t.Fatal("stale snapshot became visible")
	}
}

func TestOAuthSnapshotRecoveryNeverReplaysStoredRefresh(t *testing.T) {
	a, b, db, ctx, id := legacyOperationFixture(t)
	dir := filepath.Join(t.TempDir(), "recovery")
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	info, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(info.Location)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"old-refresh", "secret", "Legacy", "ClientSecretEnc"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("snapshot leaks plaintext")
		}
	}
	if info.RetainUntil.Sub(info.CreatedAt) != 30*24*time.Hour {
		t.Fatal("missing finite retention")
	}
	listed, err := b.ListLegacyOAuthSnapshots(ctx, dir)
	if err != nil || len(listed) != 1 || listed[0].ID != info.ID {
		t.Fatalf("restart list: %v", err)
	}
	if err := b.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
		t.Fatalf("live credentials overwritten: %v", err)
	}
	if err := a.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	var published database.MCPServer
	if err := b.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, func(row database.MCPServer) { published = row }); err != nil {
		t.Fatal(err)
	}
	if published.ID != id || published.AutoConnect || published.Enabled {
		t.Fatal("restore enabled remote connection")
	}
	if _, err := a.ReadLegacyOAuthToken(ctx, "legacy", id, nil); !errors.Is(err, oauthflow.ErrReauthorize) {
		t.Fatalf("old refresh became usable: %v", err)
	}
	var tokens database.CredentialEntry
	if err := db.Where("pattern = ?", "mcp-tokens:legacy").First(&tokens).Error; err != nil {
		t.Fatal(err)
	}
	if tokens.TokenEnc != "" || tokens.RefreshTokenEnc != "" {
		t.Fatal("old tokens restored")
	}
	if err := b.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
		t.Fatalf("restore repeated over pending grant: %v", err)
	}
	if err := a.DiscardLegacyOAuthSnapshot(ctx, dir, info.ID, false); !errors.Is(err, ErrSnapshotConflict) {
		t.Fatal("discard without confirmation")
	}
	if err := a.DiscardLegacyOAuthSnapshot(ctx, dir, info.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Location); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("discard retained file")
	}
}

func TestOAuthSnapshotRejectsActiveOperationsSourcesAndWrongKey(t *testing.T) {
	a, b, _, ctx, id := legacyOperationFixture(t)
	dir := filepath.Join(t.TempDir(), "recovery")
	op, _, err := a.BeginLegacyOAuth(ctx, "legacy", id, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateLegacyOAuthSnapshot(ctx, dir, id); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("captured active operation: %v", err)
	}
	op.End()
	info, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	b.Reset(bytes.Repeat([]byte{9}, 32), true)
	if _, err := b.ListLegacyOAuthSnapshots(ctx, dir); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("wrong key accepted: %v", err)
	}
	other := database.WithUserID(context.Background(), "another-user")
	if err := a.RestoreLegacyOAuthSnapshot(other, dir, info.ID, nil); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("other user restored: %v", err)
	}
	if err := a.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "command", Type: "oauth2", SourceConfig: &SourceConfig{Command: "must-not-run"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("command source accepted: %v", err)
	}
}

func TestOAuthSnapshotRestoreIsAtomicAndRejectsEdits(t *testing.T) {
	a, _, db, ctx, id := legacyOperationFixture(t)
	dir := filepath.Join(t.TempDir(), "recovery")
	info, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Update("name", "new edit").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
		t.Fatalf("new edit overwritten: %v", err)
	}
	if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Update("name", "Legacy").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("fail_snapshot_restore", func(tx *gorm.DB) { _ = tx.AddError(errors.New("injected failure")) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("fail_snapshot_restore") })
	called := false
	if err := a.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, func(database.MCPServer) { called = true }); err == nil {
		t.Fatal("expected rollback")
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 0 || called {
		t.Fatal("partial recovery published")
	}
}

func TestOAuthSnapshotTamperAndRetention(t *testing.T) {
	a, _, _, ctx, id := legacyOperationFixture(t)
	dir := filepath.Join(t.TempDir(), "recovery")
	info, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(info.Location)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := a.decrypt(string(data))
	if err != nil {
		t.Fatal(err)
	}
	var payload legacySnapshot
	if err := json.Unmarshal([]byte(plain), &payload); err != nil {
		t.Fatal(err)
	}
	payload.RetainUntil = time.Now().Add(-time.Minute)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := a.encrypt(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(info.Location, []byte(encrypted), 0600); err != nil {
		t.Fatal(err)
	}
	listed, err := a.ListLegacyOAuthSnapshots(ctx, dir)
	if err != nil || len(listed) != 1 || !listed[0].Expired {
		t.Fatalf("expired snapshot hidden: %v", err)
	}
	if err := a.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshotConflict) {
		t.Fatal("expired snapshot restored")
	}
	if err := os.WriteFile(info.Location, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.RestoreLegacyOAuthSnapshot(ctx, dir, info.ID, nil); !errors.Is(err, ErrSnapshot) {
		t.Fatal("tampered snapshot accepted")
	}
}
