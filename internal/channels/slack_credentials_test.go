package channels

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestSlackComposedCredentialExplicitReconstruction(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%v", missing), func(t *testing.T) {
			db := setupChannelsDB(t)
			if err := db.AutoMigrate(&database.CredentialEntry{}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(database.SetDB(db))
			mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true)
			ctx := database.WithUserID(context.Background(), "user-ana")
			cfg := &ChannelConfig{OwnerUserID: "user-ana", BotToken: "old-bot", AppToken: "old-app"}
			if err := SaveSlackWithCredentials(ctx, cfg, mgr); err != nil {
				t.Fatal(err)
			}
			id := cfg.CredentialID
			if missing {
				if err := db.Delete(&database.CredentialEntry{}, "id = ?", id).Error; err != nil {
					t.Fatal(err)
				}
			} else if err := db.Model(&database.CredentialEntry{}).Where("id = ?", id).Update("token_enc", "unreadable").Error; err != nil {
				t.Fatal(err)
			}
			cfg.BotToken = "replacement-bot"
			if err := SaveSlackWithCredentials(ctx, cfg, mgr); err == nil {
				t.Fatal("partial reconstruction discarded lost app token")
			}
			cfg.AppToken = "replacement-app"
			if err := SaveSlackWithCredentials(database.WithUserID(context.Background(), "other"), cfg, mgr); err == nil {
				t.Fatal("foreign reconstruction accepted")
			}
			if err := SaveSlackWithCredentials(ctx, cfg, mgr); err != nil {
				t.Fatal(err)
			}
			pair, err := mgr.ResolveStaticComponents(ctx, id, "slack", cfg.ID)
			if err != nil || pair[credentials.RoleBotToken] != "replacement-bot" || pair[credentials.RoleAppToken] != "replacement-app" || cfg.CredentialID != id {
				t.Fatal("reconstruction failed", err)
			}
			var count int64
			if err := db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatal("reconstruction created duplicate", err)
			}
		})
	}
}

func TestSlackComposedCredentialSaveMigrationAndPartialUpdate(t *testing.T) {
	db := setupChannelsDB(t)
	if err := db.AutoMigrate(&database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.SetDB(db))
	mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "user-ana")
	for role, secret := range map[string]string{"bot_token": "old-bot", "app_token": "old-app"} {
		if err := mgr.RegisterPatternWithContext(ctx, "channel:slack:"+role, &credentials.AuthConfig{Source: "static", Type: "secret", Token: secret}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &ChannelConfig{OwnerUserID: "user-ana", Enabled: true, BotTokenRef: "channel:slack:bot_token", AppTokenRef: "channel:slack:app_token", Conversations: map[string]string{"contact": "conv-uuid"}, ReplyChatIDs: map[string]string{"contact": "slack-channel"}}
	if err := Save("slack", cfg); err != nil {
		t.Fatal(err)
	}
	if err := SaveSlackWithCredentials(ctx, cfg, mgr); err != nil {
		t.Fatal(err)
	}
	id := cfg.CredentialID
	if err := mgr.RegisterPatternWithContext(ctx, "channel:slack:bot_token", &credentials.AuthConfig{Source: "static", Type: "secret", Token: "late"}); err == nil {
		t.Fatal("late writer recreated legacy pair")
	}
	if id == "" || cfg.BotToken != "" || cfg.AppToken != "" || cfg.BotTokenRef != credentials.StaticConnectionPattern(id) || cfg.AppTokenRef != cfg.BotTokenRef {
		t.Fatal("invalid composed projection")
	}
	if err := SaveSlackWithCredentials(ctx, cfg, mgr); err != nil || cfg.CredentialID != id {
		t.Fatal("migration not idempotent", err)
	}
	update := &ChannelConfig{OwnerUserID: "user-ana", Enabled: true, BotToken: "new-bot", Profile: "updated"}
	if err := SaveSlackWithCredentials(ctx, update, mgr); err != nil {
		t.Fatal(err)
	}
	for role, want := range map[credentials.SecretRole]string{credentials.RoleBotToken: "new-bot", credentials.RoleAppToken: "old-app"} {
		got, err := mgr.ResolveStaticComponent(ctx, id, "slack", update.ID, role)
		if err != nil || got != want {
			t.Fatal("role lost", err)
		}
	}
	loaded, err := Load("slack")
	if err != nil || loaded.Conversations["contact"] != "conv-uuid" || loaded.ReplyChatIDs["contact"] != "slack-channel" || loaded.CredentialID != id {
		t.Fatal("runtime metadata lost", err)
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("legacy pair remains", err)
	}
	// An explicit component removal preserves the other token.
	if err := SaveConversationID("slack", "new-contact", "conv-2"); err != nil {
		t.Fatal(err)
	}
	if err := SaveReplyChatID("slack", "new-contact", "new-destination"); err != nil {
		t.Fatal(err)
	}
	loaded.Conversations, loaded.ReplyChatIDs = nil, nil
	loaded.Enabled, loaded.RemoveBotToken = false, true
	if err := SaveSlackWithCredentials(ctx, loaded, mgr); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ResolveStaticComponent(ctx, id, "slack", loaded.ID, credentials.RoleBotToken); err == nil {
		t.Fatal("removed bot remains")
	}
	if got, err := mgr.ResolveStaticComponent(ctx, id, "slack", loaded.ID, credentials.RoleAppToken); err != nil || got != "old-app" {
		t.Fatal("app removed with bot", err)
	}
	fresh, err := Load("slack")
	if err != nil || fresh.Conversations["new-contact"] != "conv-2" || fresh.ReplyChatIDs["new-contact"] != "new-destination" {
		t.Fatal("removal erased concurrent runtime maps", err)
	}
}

func TestSlackComposedCredentialRollbackPreservesPairAndConfiguration(t *testing.T) {
	db := setupChannelsDB(t)
	if err := db.AutoMigrate(&database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.SetDB(db))
	mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "user-ana")
	for _, role := range []string{"bot_token", "app_token"} {
		if err := mgr.RegisterPatternWithContext(ctx, "channel:slack:"+role, &credentials.AuthConfig{Source: "static", Type: "secret", Token: role}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &ChannelConfig{OwnerUserID: "user-ana", BotTokenRef: "channel:slack:bot_token", AppTokenRef: "channel:slack:app_token"}
	if err := Save("slack", cfg); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("reject-slack", func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			_ = tx.AddError(errors.New("injected"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("reject-slack") })
	if err := SaveSlackWithCredentials(ctx, cfg, mgr); err == nil {
		t.Fatal("save failure hidden")
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("pair removed before commit", err)
	}
	loaded, err := Load("slack")
	if err != nil || loaded.CredentialID != "" || cfg.CredentialID != "" {
		t.Fatal("partial references published", err)
	}
}
