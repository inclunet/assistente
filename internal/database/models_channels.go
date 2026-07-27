package database

import "time"

// Channel armazena a configuração persistente de um canal de mensageria
// (Telegram, Signal, Slack, …). Segredos ficam apenas no CredManager via refs.
type Channel struct {
	UUIDModel
	UserID      string `json:"userId" gorm:"index;uniqueIndex:ux_channels_user_slug"`
	Type        string `json:"type" gorm:"not null;index"` // telegram|signal|slack
	Slug        string `json:"slug" gorm:"not null;index;uniqueIndex:ux_channels_user_slug"`
	DisplayName string `json:"displayName" gorm:"not null"`
	Enabled     bool   `json:"enabled" gorm:"not null;default:false;index"`
	Profile     string `json:"profile,omitempty"`
	MaxHistory  int    `json:"maxHistory,omitempty"`
	MaxContacts int    `json:"maxContacts,omitempty"`
	Settings    string `json:"settings,omitempty" gorm:"type:text"` // JSON: api_url, account, reply_chat_ids, …
	BotTokenRef string `json:"botTokenRef,omitempty"`
	AppTokenRef string `json:"appTokenRef,omitempty"`
	APITokenRef string `json:"apiTokenRef,omitempty"`
	User        *User  `json:"-" gorm:"foreignKey:UserID"`
}

func (Channel) TableName() string { return "channels" }

// ChannelContact é um contato autorizado vinculado a um canal.
type ChannelContact struct {
	UUIDModel
	UserID       string     `json:"userId" gorm:"index"`
	ChannelID    string     `json:"channelId" gorm:"not null;index;uniqueIndex:ux_channel_contacts_channel_external"`
	ExternalID   string     `json:"externalId" gorm:"not null;uniqueIndex:ux_channel_contacts_channel_external"`
	DisplayName  string     `json:"displayName"`
	Username     string     `json:"username,omitempty"`
	AuthorizedAt *time.Time `json:"authorizedAt,omitempty"`
	Channel      *Channel   `json:"-" gorm:"foreignKey:ChannelID"`
	User         *User      `json:"-" gorm:"foreignKey:UserID"`
}

func (ChannelContact) TableName() string { return "channel_contacts" }

// ChannelContactConversation mapeia contact_external_id → conversation_id por canal.
type ChannelContactConversation struct {
	UUIDModel
	ChannelID         string   `json:"channelId" gorm:"not null;index;uniqueIndex:ux_channel_contact_conversations"`
	ContactExternalID string   `json:"contactExternalId" gorm:"not null;uniqueIndex:ux_channel_contact_conversations"`
	ConversationID    string   `json:"conversationId" gorm:"not null;index"`
	Channel           *Channel `json:"-" gorm:"foreignKey:ChannelID"`
}

func (ChannelContactConversation) TableName() string { return "channel_contact_conversations" }
