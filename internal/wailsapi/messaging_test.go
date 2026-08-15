package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/controllers"
	"assistente/internal/channels"
	"assistente/internal/configdir"
	"assistente/internal/contacts"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMessagingNotWired(t *testing.T) {
	t.Parallel()
	api := NewMessaging()
	if _, err := api.GetMessagingStatus(); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetMessagingStatus: got %v", err)
	}
	if _, err := api.GetChannelConfig("telegram"); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetChannelConfig: got %v", err)
	}
	if err := api.SaveChannelConfig("telegram", nil); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("SaveChannelConfig: got %v", err)
	}
	if err := api.RestartChannel("telegram"); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("RestartChannel: got %v", err)
	}
	if _, err := api.GetAllChannelConfigs(); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetAllChannelConfigs: got %v", err)
	}
	if _, err := api.GetChannelTemplates(); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetChannelTemplates: got %v", err)
	}
	if err := api.CreateChannelFromTemplate("telegram", nil); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("CreateChannelFromTemplate: got %v", err)
	}
	if _, err := api.GetChannelConfigAsMap("telegram"); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetChannelConfigAsMap: got %v", err)
	}
	if err := api.AuthorizeMessagingContactFull("telegram", "1", "n", "u"); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("AuthorizeMessagingContactFull: got %v", err)
	}
	if err := api.RemoveAuthorizedContact("telegram", "1"); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("RemoveAuthorizedContact: got %v", err)
	}
	if _, err := api.GetAuthorizedContacts(); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetAuthorizedContacts: got %v", err)
	}
	if _, err := api.GetAvailableChannels(); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetAvailableChannels: got %v", err)
	}
	if err := api.AssignConversationToChannel("c", "telegram", "1"); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("AssignConversationToChannel: got %v", err)
	}
	if err := api.UnassignConversationFromChannel("c"); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("UnassignConversationFromChannel: got %v", err)
	}
	if _, _, err := api.GetConversationChannel("c"); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetConversationChannel: got %v", err)
	}
}

func TestMessagingNilControllerIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewMessaging()
	AttachMessaging(api, stubSession{}, nil)
	if _, err := api.GetMessagingStatus(); !errors.Is(err, ErrMessagingNotWired) {
		t.Fatalf("GetMessagingStatus com ctrl nil: got %v", err)
	}
}

func TestMessagingUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewMessaging()
	AttachMessaging(
		api,
		stubSession{err: semAuth},
		controllers.NewMessagingController(controllers.MessagingControllerConfig{}),
	)

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"GetMessagingStatus", func() error {
			_, err := api.GetMessagingStatus()
			return err
		}},
		{"GetChannelConfig", func() error {
			_, err := api.GetChannelConfig("telegram")
			return err
		}},
		{"SaveChannelConfig", func() error {
			return api.SaveChannelConfig("telegram", &channels.ChannelConfig{})
		}},
		{"RestartChannel", func() error {
			return api.RestartChannel("telegram")
		}},
		{"GetAllChannelConfigs", func() error {
			_, err := api.GetAllChannelConfigs()
			return err
		}},
		{"GetChannelTemplates", func() error {
			_, err := api.GetChannelTemplates()
			return err
		}},
		{"CreateChannelFromTemplate", func() error {
			return api.CreateChannelFromTemplate("telegram", nil)
		}},
		{"GetChannelConfigAsMap", func() error {
			_, err := api.GetChannelConfigAsMap("telegram")
			return err
		}},
		{"AuthorizeMessagingContactFull", func() error {
			return api.AuthorizeMessagingContactFull("telegram", "1", "n", "u")
		}},
		{"RemoveAuthorizedContact", func() error {
			return api.RemoveAuthorizedContact("telegram", "1")
		}},
		{"GetAuthorizedContacts", func() error {
			_, err := api.GetAuthorizedContacts()
			return err
		}},
		{"GetAvailableChannels", func() error {
			_, err := api.GetAvailableChannels()
			return err
		}},
		{"AssignConversationToChannel", func() error {
			return api.AssignConversationToChannel("c", "telegram", "1")
		}},
		{"UnassignConversationFromChannel", func() error {
			return api.UnassignConversationFromChannel("c")
		}},
		{"GetConversationChannel", func() error {
			_, _, err := api.GetConversationChannel("c")
			return err
		}},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
		})
	}
}

func TestMessagingUsesWithUserNotRequireAuthSource(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "messaging.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("messaging.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("messaging.go deve chamar WithUser(session,")
	}
}

func messagingUserSession(userID string) Session {
	return stubSession{ctx: database.WithUserID(context.Background(), userID)}
}

func newTestMessagingController(t *testing.T) *controllers.MessagingController {
	t.Helper()
	return controllers.NewMessagingController(controllers.MessagingControllerConfig{
		Ctx: context.Background(),
	})
}

func setupMessagingAPITest(t *testing.T) *Messaging {
	t.Helper()
	tempDir := t.TempDir()
	oldHome, _ := os.LookupEnv("HOME")
	oldUserProfile, _ := os.LookupEnv("USERPROFILE")
	oldWd, _ := os.Getwd()

	_ = os.Setenv("HOME", tempDir)
	_ = os.Setenv("USERPROFILE", tempDir)
	_ = os.Chdir(tempDir)
	configdir.ResetForTests()

	path := filepath.Join(tempDir, "messaging.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&database.Channel{},
		&database.ChannelContact{},
		&database.ChannelContactConversation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	channels.UseDatabase(db)
	contacts.UseDatabase(db)

	t.Cleanup(func() {
		channels.UseDatabase(nil)
		contacts.UseDatabase(nil)
		_ = sqlDB.Close()
		_ = os.Chdir(oldWd)
		if oldHome == "" {
			_ = os.Unsetenv("HOME")
		} else {
			_ = os.Setenv("HOME", oldHome)
		}
		if oldUserProfile == "" {
			_ = os.Unsetenv("USERPROFILE")
		} else {
			_ = os.Setenv("USERPROFILE", oldUserProfile)
		}
		configdir.ResetForTests()
	})

	api := NewMessaging()
	AttachMessaging(api, messagingUserSession("user-b"), newTestMessagingController(t))
	return api
}

func TestSaveChannelConfig_RejectsCrossUserOverwrite(t *testing.T) {
	api := setupMessagingAPITest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
		BotTokenRef: "channel:telegram:bot_token",
		Profile:     "perfil-de-A",
	}); err != nil {
		t.Fatalf("setup channel de user-a: %v", err)
	}

	err := api.SaveChannelConfig("telegram", &channels.ChannelConfig{Enabled: true})
	if err == nil {
		t.Fatal("user-b conseguiu sobrescrever canal de user-a — vetor de roubo aberto")
	}
	if !strings.Contains(err.Error(), "telegram") {
		t.Fatalf("erro deveria mencionar canal: %v", err)
	}

	persisted, err := channels.Load("telegram")
	if err != nil {
		t.Fatalf("load após tentativa de roubo: %v", err)
	}
	if persisted.OwnerUserID != "user-a" {
		t.Fatalf("OwnerUserID foi alterado para %q (esperava user-a)", persisted.OwnerUserID)
	}
	if persisted.BotTokenRef != "channel:telegram:bot_token" {
		t.Fatalf("BotTokenRef sobrescrito: %q", persisted.BotTokenRef)
	}
	if persisted.Profile != "perfil-de-A" {
		t.Fatalf("Profile sobrescrito: %q", persisted.Profile)
	}
}

func TestGetChannelConfig_RejectsCrossUser(t *testing.T) {
	api := setupMessagingAPITest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
		BotTokenRef: "channel:telegram:bot_token",
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := api.GetChannelConfig("telegram")
	if err == nil {
		t.Fatal("GetChannelConfig vazou config de canal alheio")
	}
}

func TestGetChannelConfig_RedactsLegacyTokens(t *testing.T) {
	api := setupMessagingAPITest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		BotTokenRef: "channel:telegram:bot_token",
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := api.GetChannelConfig("telegram")
	if err != nil {
		t.Fatalf("canal legado deveria estar visível: %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg nil em canal legado")
	}
	if cfg.BotToken != "" {
		t.Fatalf("BotToken não redacted em canal legado: %q", cfg.BotToken)
	}
	if cfg.BotTokenRef != "channel:telegram:bot_token" {
		t.Fatalf("BotTokenRef deveria permanecer: %q", cfg.BotTokenRef)
	}
}

func TestRestartChannel_RejectsCrossUser(t *testing.T) {
	api := setupMessagingAPITest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := api.RestartChannel("telegram")
	if err == nil {
		t.Fatal("RestartChannel cross-user deveria falhar")
	}
}

func TestGetAllChannelConfigs_FiltersByOwner(t *testing.T) {
	api := setupMessagingAPITest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, OwnerUserID: "user-a", Type: "telegram"}); err != nil {
		t.Fatalf("setup A: %v", err)
	}
	if err := channels.Save("signal", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-b",
		Type:        "signal",
		APITokenRef: "channel:signal:api_token",
		Profile:     "perfil-b",
	}); err != nil {
		t.Fatalf("setup B: %v", err)
	}
	if err := channels.Save("slack", &channels.ChannelConfig{
		Enabled:     true,
		Type:        "slack",
		BotTokenRef: "channel:slack:bot_token",
	}); err != nil {
		t.Fatalf("setup legacy: %v", err)
	}

	all, err := api.GetAllChannelConfigs()
	if err != nil {
		t.Fatalf("GetAllChannelConfigs: %v", err)
	}
	if _, leaked := all["telegram"]; leaked {
		t.Fatal("canal de user-a vazou para user-b")
	}
	if cfg, ok := all["signal"]; !ok {
		t.Fatal("user-b não viu o próprio canal signal")
	} else if cfg.APITokenRef != "channel:signal:api_token" {
		t.Fatalf("APITokenRef do próprio canal perdido: %q", cfg.APITokenRef)
	} else if cfg.Profile != "perfil-b" {
		t.Fatalf("Profile do próprio canal perdido: %q", cfg.Profile)
	}
	if cfg, ok := all["slack"]; !ok {
		t.Fatal("canal legado deveria aparecer na lista")
	} else if cfg.BotToken != "" {
		t.Fatalf("BotToken legado não foi redacted: %q", cfg.BotToken)
	}
}
