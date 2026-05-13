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
)

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

// setupMessagingTest prepara um diretório temporário isolado para
// channels.* (que escrevem em disco). Sem isso, testes paralelos no
// mesmo workspace contaminariam o estado uns dos outros.
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

	// Garante que o diretório channels existe e está limpo.
	_ = os.RemoveAll(filepath.Join(tempDir, ".assistente", "channels"))

	t.Cleanup(func() {
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
		BotToken:    "secret-de-A",
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

	// Confirma que o canal não foi alterado (token de A intacto).
	persisted, err := channels.Load("telegram")
	if err != nil {
		t.Fatalf("load após tentativa de roubo: %v", err)
	}
	if persisted.OwnerUserID != "user-a" {
		t.Fatalf("OwnerUserID foi alterado para %q (esperava user-a)", persisted.OwnerUserID)
	}
	if persisted.BotToken != "secret-de-A" {
		t.Fatalf("BotToken sobrescrito (esperava secret-de-A): %q", persisted.BotToken)
	}
}

// TestGetChannelConfig_RejectsCrossUser cobre B6 — User B não pode
// sequer LER o cfg de canal alheio (que vazaria tokens, refs etc.).
func TestGetChannelConfig_RejectsCrossUser(t *testing.T) {
	setupMessagingTest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
		BotToken:    "secret-de-A",
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
// dono é visível ("legado") mas tokens em texto plano não vazam.
func TestGetChannelConfig_RedactsLegacyTokens(t *testing.T) {
	setupMessagingTest(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:  true,
		BotToken: "legacy-token",
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

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, OwnerUserID: "user-a"}); err != nil {
		t.Fatalf("setup A: %v", err)
	}
	if err := channels.Save("signal", &channels.ChannelConfig{Enabled: true, OwnerUserID: "user-b", APIToken: "tok-de-b"}); err != nil {
		t.Fatalf("setup B: %v", err)
	}
	if err := channels.Save("slack", &channels.ChannelConfig{Enabled: true, BotToken: "legacy-tok"}); err != nil {
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
	} else if cfg.APIToken != "tok-de-b" {
		t.Fatalf("APIToken do próprio canal foi indevidamente redactado: %q", cfg.APIToken)
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
