package channels

import (
	"testing"
)

func TestConfigToRowAndRowToConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	cfg := &ChannelConfig{
		Enabled:     true,
		BotTokenRef: "channel:telegram:bot_token",
		APIURL:      "http://localhost:8080",
		Account:     "+5511999999999",
		Profile:     "canais-comunicacao",
		MaxHistory:  40,
		MaxContacts: 2,
		OwnerUserID: "user-ana",
		Type:        "telegram",
		DisplayName: "Telegram",
		ReplyChatIDs: map[string]string{
			"U1": "C1",
		},
		Conversations: map[string]string{
			"U1": "conv-1",
		},
	}

	row := ConfigToRow("telegram", cfg)
	if row.Slug != "telegram" || row.Type != "telegram" {
		t.Fatalf("slug/type: %+v", row)
	}
	if row.UserID != "user-ana" {
		t.Fatalf("userID=%q", row.UserID)
	}
	if row.BotTokenRef != "channel:telegram:bot_token" {
		t.Fatalf("bot ref=%q", row.BotTokenRef)
	}
	if row.Settings == "" {
		t.Fatal("settings JSON vazio")
	}
	// Plaintext nunca na row
	if row.BotTokenRef == cfg.BotToken && cfg.BotToken != "" {
		t.Fatal("não deveria espelhar plaintext")
	}

	out := RowToConfig(&row, cfg.Conversations)
	if out == nil {
		t.Fatal("RowToConfig retornou nil")
	}
	if !out.Enabled || out.APIURL != cfg.APIURL || out.Account != cfg.Account {
		t.Fatalf("roundtrip settings: %+v", out)
	}
	if out.OwnerUserID != "user-ana" {
		t.Fatalf("owner=%q", out.OwnerUserID)
	}
	if out.Conversations["U1"] != "conv-1" {
		t.Fatalf("conversations=%v", out.Conversations)
	}
	if out.ReplyChatIDs["U1"] != "C1" {
		t.Fatalf("reply=%v", out.ReplyChatIDs)
	}
	if out.BotToken != "" || out.AppToken != "" || out.APIToken != "" {
		t.Fatalf("plaintext vazou no DTO mapeado: %+v", out)
	}
}

func TestConfigToRow_DefaultsTypeAndDisplayName(t *testing.T) {
	t.Parallel()
	row := ConfigToRow("signal", &ChannelConfig{Enabled: true})
	if row.Type != "signal" {
		t.Fatalf("type=%q", row.Type)
	}
	if row.DisplayName != "Signal" {
		t.Fatalf("display=%q", row.DisplayName)
	}
}
