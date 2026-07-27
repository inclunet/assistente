package channels

import (
	"encoding/json"
	"strings"

	"assistente/internal/database"
)

// channelSettings is the JSON payload stored in database.Channel.Settings.
type channelSettings struct {
	APIURL       string            `json:"api_url,omitempty"`
	Account      string            `json:"account,omitempty"`
	ReplyChatIDs map[string]string `json:"reply_chat_ids,omitempty"`
}

// ConfigToRow maps a ChannelConfig DTO into a database.Channel row (without conversations).
func ConfigToRow(slug string, cfg *ChannelConfig) database.Channel {
	if cfg == nil {
		cfg = &ChannelConfig{}
	}
	slug = strings.TrimSpace(slug)
	typ := strings.TrimSpace(cfg.Type)
	if typ == "" {
		typ = slug
	}
	display := strings.TrimSpace(cfg.DisplayName)
	if display == "" {
		display = defaultDisplayName(typ)
	}
	settings := channelSettings{
		APIURL:       cfg.APIURL,
		Account:      cfg.Account,
		ReplyChatIDs: cfg.ReplyChatIDs,
	}
	raw, _ := json.Marshal(settings)
	return database.Channel{
		UserID:      strings.TrimSpace(cfg.OwnerUserID),
		Type:        typ,
		Slug:        slug,
		DisplayName: display,
		Enabled:     cfg.Enabled,
		Profile:     cfg.Profile,
		MaxHistory:  cfg.MaxHistory,
		MaxContacts: cfg.MaxContacts,
		Settings:    string(raw),
		BotTokenRef: cfg.BotTokenRef,
		AppTokenRef: cfg.AppTokenRef,
		APITokenRef: cfg.APITokenRef,
	}
}

// RowToConfig maps a database.Channel (+ optional conversation map) into ChannelConfig.
// Plaintext tokens are never stored on the row; callers may still set them in-memory.
func RowToConfig(row *database.Channel, conversations map[string]string) *ChannelConfig {
	if row == nil {
		return nil
	}
	var settings channelSettings
	if strings.TrimSpace(row.Settings) != "" {
		_ = json.Unmarshal([]byte(row.Settings), &settings)
	}
	cfg := &ChannelConfig{
		Enabled:       row.Enabled,
		BotTokenRef:   row.BotTokenRef,
		AppTokenRef:   row.AppTokenRef,
		APITokenRef:   row.APITokenRef,
		Account:       settings.Account,
		APIURL:        settings.APIURL,
		Profile:       row.Profile,
		MaxHistory:    row.MaxHistory,
		MaxContacts:   row.MaxContacts,
		OwnerUserID:   row.UserID,
		Conversations: conversations,
		ReplyChatIDs:  settings.ReplyChatIDs,
		Type:          row.Type,
		DisplayName:   row.DisplayName,
	}
	if len(cfg.Conversations) == 0 {
		cfg.Conversations = nil
	}
	if len(cfg.ReplyChatIDs) == 0 {
		cfg.ReplyChatIDs = nil
	}
	return cfg
}

func defaultDisplayName(typ string) string {
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "telegram":
		return "Telegram"
	case "signal":
		return "Signal"
	case "slack":
		return "Slack"
	case "":
		return "Channel"
	default:
		runes := []rune(typ)
		if len(runes) == 0 {
			return "Channel"
		}
		return strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
}
