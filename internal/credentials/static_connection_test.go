package credentials

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestStaticConnectionMigrationSerializesPendingCachePublication(t *testing.T) {
	for _, operation := range []string{"register", "reload"} {
		t.Run(operation, func(t *testing.T) {
			setupScopedCredentialStoreTestDB(t)
			m := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), NewDBStore(), true)
			ctx := database.WithUserID(context.Background(), "owner")
			const pattern = "channel:slack:bot_token"
			if err := m.RegisterPatternWithContext(ctx, pattern, &AuthConfig{Source: "static", Type: "secret", Token: "before"}); err != nil {
				t.Fatal(err)
			}
			type pauseKey struct{}
			paused, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var intercepted atomic.Bool
			db := database.DB()
			if err := db.Callback().Query().After("gorm:query").Register("pause_static_publication", func(tx *gorm.DB) {
				if tx.Statement.Table == "credential_entries" && tx.Statement.Context.Value(pauseKey{}) == true && intercepted.CompareAndSwap(false, true) {
					// The store has read the persisted ID, but the cache is not published.
					close(paused)
					<-release
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove("pause_static_publication") })
			writerDone := make(chan error, 1)
			go func() {
				pendingCtx := context.WithValue(ctx, pauseKey{}, true)
				if operation == "reload" {
					writerDone <- m.LoadUserCredentials(pendingCtx, "owner")
				} else {
					writerDone <- m.RegisterPatternWithContext(pendingCtx, pattern, &AuthConfig{Source: "static", Type: "secret", Token: "after"})
				}
			}()
			select {
			case <-paused:
			case <-time.After(5 * time.Second):
				t.Fatal("writer did not reach publication gap")
			}
			if m.mutationMu.TryLock() {
				m.mutationMu.Unlock()
				t.Fatal("migration can overtake pending cache publication")
			}
			var id string
			migrationDone := make(chan error, 1)
			go func() {
				migrationDone <- m.UpdateStaticConnection(ctx, func(*gorm.DB) (StaticConnectionUpdate, error) {
					return StaticConnectionUpdate{Integration: "slack", ConsumerID: "channel", Legacy: map[SecretRole]string{RoleBotToken: pattern}, Commit: func(_ *gorm.DB, next string, _ map[SecretRole]bool) error { id = next; return nil }}, nil
				})
			}()
			unblock()
			for _, done := range []<-chan error{writerDone, migrationDone} {
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("publication/migration did not finish")
				}
			}
			want := "after"
			if operation == "reload" {
				want = "before"
			}
			got, err := m.ResolveStaticComponent(ctx, id, "slack", "channel", RoleBotToken)
			if err != nil || got != want {
				t.Fatal("migration lost latest token", err)
			}
			visible, err := m.ListVisibleCredentialsWithContext(ctx)
			if err != nil || len(visible) != 0 {
				t.Fatal("historical credential resurrected in visible cache", err)
			}
			var count int64
			if err := db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatal("migration left historical row", err)
			}
		})
	}
}

func TestStaticConnectionMigratesAtomicRolesAndPreservesIsolation(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	key := bytes.Repeat([]byte{7}, 32)
	m := NewManagerWithStore(key, NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "owner")
	for role, secret := range map[SecretRole]string{RoleBotToken: "bot-secret", RoleAppToken: "app-secret"} {
		if err := m.RegisterPatternWithContext(ctx, "channel:slack:"+string(role), &AuthConfig{Source: "static", Type: "secret", Token: secret}); err != nil {
			t.Fatal(err)
		}
	}
	var id string
	fail := true
	update := func(tx *gorm.DB) (StaticConnectionUpdate, error) {
		return StaticConnectionUpdate{Integration: "slack", ConsumerID: "channel-id", Legacy: map[SecretRole]string{RoleBotToken: "channel:slack:bot_token", RoleAppToken: "channel:slack:app_token"}, Commit: func(tx *gorm.DB, next string, present map[SecretRole]bool) error {
			if !present[RoleBotToken] || !present[RoleAppToken] {
				t.Fatal("lost role")
			}
			if fail {
				return errors.New("injected")
			}
			id = next
			return nil
		}}, nil
	}
	if err := m.UpdateStaticConnection(ctx, update); err == nil {
		t.Fatal("rollback failure accepted")
	}
	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("partial conversion", err)
	}
	fail = false
	if err := m.UpdateStaticConnection(ctx, update); err != nil {
		t.Fatal(err)
	}
	var row database.CredentialEntry
	if err := database.DB().First(&row, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if row.Source != "static" || row.OAuthEnc != "" || strings.Contains(row.TokenEnc, "secret") {
		t.Fatal("not an encrypted static connection")
	}
	if err := database.DB().Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("split credential persisted", err)
	}
	for _, manager := range []*Manager{m, NewManagerWithStore(key, NewDBStore(), true)} {
		for role, want := range map[SecretRole]string{RoleBotToken: "bot-secret", RoleAppToken: "app-secret"} {
			got, err := manager.ResolveStaticComponent(ctx, id, "slack", "channel-id", role)
			if err != nil || got != want {
				t.Fatal("role unavailable after migration/restart", err)
			}
		}
	}
	for _, tc := range []struct {
		ctx                   context.Context
		integration, consumer string
		role                  SecretRole
	}{
		{database.WithUserID(context.Background(), "other"), "slack", "channel-id", RoleBotToken},
		{ctx, "other", "channel-id", RoleBotToken}, {ctx, "slack", "other", RoleBotToken}, {ctx, "slack", "channel-id", "unknown"},
	} {
		if _, err := m.ResolveStaticComponent(tc.ctx, id, tc.integration, tc.consumer, tc.role); err == nil {
			t.Fatal("unbound component read")
		}
	}
	if auth, err := m.GetByPatternWithContext(ctx, StaticConnectionPattern(id)); err == nil || auth != nil {
		t.Fatal("whole record exposed as HTTP secret")
	}
	visible, err := m.ListVisibleCredentialsWithContext(ctx)
	if err != nil || len(visible) != 0 {
		t.Fatal("managed connection exposed to editor/export", err)
	}
}

func TestStaticConnectionRepairsLegacyOnlyWithExplicitChange(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		for _, replacement := range []string{"replacement", ""} {
			t.Run(fmt.Sprintf("corrupt=%v/remove=%v", corrupt, replacement == ""), func(t *testing.T) {
				setupScopedCredentialStoreTestDB(t)
				m := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), NewDBStore(), true)
				ctx := database.WithUserID(context.Background(), "owner")
				if corrupt {
					row := database.CredentialEntry{UserID: "owner", Pattern: "channel:slack:bot_token", Source: "static", AuthType: "secret", TokenEnc: "invalid-ciphertext"}
					if err := database.DB().Create(&row).Error; err != nil {
						t.Fatal(err)
					}
				}
				var id string
				changes := map[SecretRole]*string{}
				prepare := func(*gorm.DB) (StaticConnectionUpdate, error) {
					return StaticConnectionUpdate{Integration: "slack", ConsumerID: "channel-id", Legacy: map[SecretRole]string{RoleBotToken: "channel:slack:bot_token"}, Changes: changes, Commit: func(_ *gorm.DB, next string, _ map[SecretRole]bool) error { id = next; return nil }}, nil
				}
				if err := m.UpdateStaticConnection(ctx, prepare); err == nil {
					t.Fatal("missing/illegible secret silently preserved")
				}
				changes[RoleBotToken] = &replacement
				if err := m.UpdateStaticConnection(ctx, prepare); err != nil {
					t.Fatal(err)
				}
				got, err := m.ResolveStaticComponent(ctx, id, "slack", "channel-id", RoleBotToken)
				if replacement == "" {
					if err == nil {
						t.Fatal("removed role found")
					}
				} else if err != nil || got != replacement {
					t.Fatal("replacement not recovered", err)
				}
			})
		}
	}
}

func TestStaticConnectionConcurrentReadKeepsPairTogether(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	m := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "owner")
	var id string
	save := func(value string) error {
		return m.UpdateStaticConnection(ctx, func(*gorm.DB) (StaticConnectionUpdate, error) {
			return StaticConnectionUpdate{ID: id, Integration: "slack", ConsumerID: "channel-id", Changes: map[SecretRole]*string{RoleBotToken: &value, RoleAppToken: &value}, Commit: func(_ *gorm.DB, next string, _ map[SecretRole]bool) error { id = next; return nil }}, nil
		})
	}
	if err := save("initial"); err != nil {
		t.Fatal(err)
	}
	stableID := id
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			if err := save(fmt.Sprint(i)); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i := 0; i < 30; i++ {
		pair, err := m.ResolveStaticComponents(ctx, stableID, "slack", "channel-id", RoleBotToken, RoleAppToken)
		if err != nil || pair[RoleBotToken] != pair[RoleAppToken] {
			t.Error("mixed credential revisions", err)
			break
		}
	}
	wg.Wait()
}

func TestStaticConnectionPartialUpdateAndRemoval(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	m := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "owner")
	bot, app := "bot", "app"
	var id string
	save := func(changes map[SecretRole]*string) error {
		return m.UpdateStaticConnection(ctx, func(*gorm.DB) (StaticConnectionUpdate, error) {
			return StaticConnectionUpdate{ID: id, Integration: "slack", ConsumerID: "channel-id", Changes: changes, Commit: func(_ *gorm.DB, next string, _ map[SecretRole]bool) error { id = next; return nil }}, nil
		})
	}
	if err := save(map[SecretRole]*string{RoleBotToken: &bot, RoleAppToken: &app}); err != nil {
		t.Fatal(err)
	}
	bot = "replacement"
	if err := save(map[SecretRole]*string{RoleBotToken: &bot}); err != nil {
		t.Fatal(err)
	}
	if got, err := m.ResolveStaticComponent(ctx, id, "slack", "channel-id", RoleAppToken); err != nil || got != app {
		t.Fatal("partial update lost other role", err)
	}
	empty := ""
	if err := save(map[SecretRole]*string{RoleBotToken: &empty}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ResolveStaticComponent(ctx, id, "slack", "channel-id", RoleBotToken); err == nil {
		t.Fatal("removed role remained")
	}
	if got, err := m.ResolveStaticComponent(ctx, id, "slack", "channel-id", RoleAppToken); err != nil || got != app {
		t.Fatal("remove lost other role", err)
	}
}
