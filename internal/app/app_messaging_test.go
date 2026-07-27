package app

import (
	"context"
	"os"
	"path/filepath"
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

// TestBindMessagingDatabase_FailClosedWhenDBNil garante o cutover AEP-0083:
// sem database.DB() no boot, não omitir UseDatabase silenciosamente.
func TestBindMessagingDatabase_FailClosedWhenDBNil(t *testing.T) {
	prev := database.DB()
	database.SetDB(nil)
	t.Cleanup(func() { database.SetDB(prev) })

	err := bindMessagingDatabase()
	if err == nil {
		t.Fatal("esperava erro fail-closed quando database.DB() == nil")
	}
	if !strings.Contains(err.Error(), "AEP-0083") {
		t.Fatalf("erro deveria citar AEP-0083: %v", err)
	}
	if !strings.Contains(err.Error(), "filesystem") {
		t.Fatalf("erro deveria mencionar fallback filesystem: %v", err)
	}
}

// TestBindMessagingDatabase_EnablesStoresWhenDBAvailable cobre o caminho feliz do boot.
func TestBindMessagingDatabase_EnablesStoresWhenDBAvailable(t *testing.T) {
	prev := database.DB()
	channels.UseDatabase(nil)
	contacts.UseDatabase(nil)
	t.Cleanup(func() {
		database.SetDB(prev)
		channels.UseDatabase(nil)
		contacts.UseDatabase(nil)
	})

	db, err := gorm.Open(sqlite.Open("file:bind_messaging?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	database.SetDB(db)

	if err := bindMessagingDatabase(); err != nil {
		t.Fatalf("bindMessagingDatabase: %v", err)
	}
	if channels.DB() == nil {
		t.Fatal("channels.UseDatabase não foi chamado")
	}
}

// newTestMessagingController cria um MessagingController mínimo (sem
// Init) suficiente para chamadas read-only que vão direto a channels.*
// e contacts.*. Operações que dependem de gateway/messengers vão
// panicar — usar apenas para filtragem/listagem.
func newTestMessagingController(t *testing.T) *controllers.MessagingController {
	t.Helper()
	return controllers.NewMessagingController(controllers.MessagingControllerConfig{
		Ctx: context.Background(),
	})
}

// setupMessagingTest prepara HOME temporário + SQLite com UseDatabase
// (AEP-0083: runtime exige DB; sem fallback FS).
func setupMessagingTest(t *testing.T) {
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
}

// TestSaveChannelConfig_RejectsCrossUserOverwrite cobre M12 do review da
// Fatia 2: usuário B não pode sobrescrever (e portanto roubar) canal cujo
// OwnerUserID é A. Antes, o handler simplesmente carimbava OwnerUserID =
// currentUserID, transferindo a posse silenciosamente.
func TestSaveChannelConfig_RejectsCrossUserOverwrite(t *testing.T) {
	setupMessagingTest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
		BotTokenRef: "channel:telegram:bot_token",
		Profile:     "perfil-de-A",
	}); err != nil {
		t.Fatalf("setup channel de user-a: %v", err)
	}

	app := &App{ctx: context.Background()}
	app.setCurrentUserID("user-b")

	err := app.SaveChannelConfig("telegram", &channels.ChannelConfig{Enabled: true})
	if err == nil {
		t.Fatal("user-b conseguiu sobrescrever canal de user-a — vetor de roubo aberto")
	}
	if !strings.Contains(err.Error(), "telegram") {
		t.Fatalf("erro deveria mencionar canal: %v", err)
	}

	// Confirma que o canal não foi alterado.
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

// TestGetChannelConfig_RejectsCrossUser cobre B6 — User B não pode
// sequer LER o cfg de canal alheio (que vazaria tokens, refs etc.).
func TestGetChannelConfig_RejectsCrossUser(t *testing.T) {
	setupMessagingTest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
		BotTokenRef: "channel:telegram:bot_token",
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	app := &App{ctx: context.Background()}
	app.msgCtrl = nil // não deve ser chamado — rejeição vem antes
	app.setCurrentUserID("user-b")

	_, err := app.GetChannelConfig("telegram")
	if err == nil {
		t.Fatal("GetChannelConfig vazou config de canal alheio")
	}
}

// TestGetChannelConfig_RedactsLegacyTokens cobre B6 + B10. Canal sem
// dono é visível ("legado"); plaintext nunca vem do DB e redact garante
// BotToken vazio mesmo se o DTO tivesse valor em memória.
func TestGetChannelConfig_RedactsLegacyTokens(t *testing.T) {
	setupMessagingTest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		BotTokenRef: "channel:telegram:bot_token",
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	app := &App{ctx: context.Background()}
	app.setCurrentUserID("user-b")

	cfg, err := app.GetChannelConfig("telegram")
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

// TestRestartChannel_RejectsCrossUser cobre B6. Restart é vetor de DoS:
// usuário B não pode derrubar canal de A.
func TestRestartChannel_RejectsCrossUser(t *testing.T) {
	setupMessagingTest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	app := &App{ctx: context.Background()}
	app.setCurrentUserID("user-b")

	err := app.RestartChannel("telegram")
	if err == nil {
		t.Fatal("RestartChannel cross-user deveria falhar")
	}
}

// TestGetAllChannelConfigs_FiltersByOwner cobre B6. User B só vê seus
// canais + legados sem dono — nunca canais de A.
func TestGetAllChannelConfigs_FiltersByOwner(t *testing.T) {
	setupMessagingTest(t)

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

	app := &App{ctx: context.Background()}
	app.msgCtrl = newTestMessagingController(t)
	app.setCurrentUserID("user-b")

	all, err := app.GetAllChannelConfigs()
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

// TestUnauthenticatedBindingsReject cobre B6 — todas as bindings devem
// falhar quando não há sessão autenticada (currentUserID == "").
func TestUnauthenticatedBindingsReject(t *testing.T) {
	setupMessagingTest(t)
	app := &App{ctx: context.Background()} // sem setCurrentUserID

	cases := []struct {
		name string
		fn   func() error
	}{
		{"GetMessagingStatus", func() error {
			_, err := app.GetMessagingStatus()
			return err
		}},
		{"GetChannelConfig", func() error {
			_, err := app.GetChannelConfig("telegram")
			return err
		}},
		{"SaveChannelConfig", func() error {
			return app.SaveChannelConfig("telegram", &channels.ChannelConfig{})
		}},
		{"RestartChannel", func() error {
			return app.RestartChannel("telegram")
		}},
		{"GetAllChannelConfigs", func() error {
			_, err := app.GetAllChannelConfigs()
			return err
		}},
		{"GetChannelTemplates", func() error {
			_, err := app.GetChannelTemplates()
			return err
		}},
		{"CreateChannelFromTemplate", func() error {
			return app.CreateChannelFromTemplate("telegram", nil)
		}},
		{"GetChannelConfigAsMap", func() error {
			_, err := app.GetChannelConfigAsMap("telegram")
			return err
		}},
		{"AuthorizeMessagingContactFull", func() error {
			return app.AuthorizeMessagingContactFull("telegram", "1", "n", "u")
		}},
		{"RemoveAuthorizedContact", func() error {
			return app.RemoveAuthorizedContact("telegram", "1")
		}},
		{"GetAuthorizedContacts", func() error {
			_, err := app.GetAuthorizedContacts()
			return err
		}},
		{"GetAvailableChannels", func() error {
			_, err := app.GetAvailableChannels()
			return err
		}},
		{"AssignConversationToChannel", func() error {
			return app.AssignConversationToChannel("c", "telegram", "1")
		}},
		{"UnassignConversationFromChannel", func() error {
			return app.UnassignConversationFromChannel("c")
		}},
		{"GetConversationChannel", func() error {
			_, _, err := app.GetConversationChannel("c")
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatalf("%s deveria rejeitar caller sem sessão", tc.name)
			}
		})
	}
}
