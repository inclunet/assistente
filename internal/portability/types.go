package portability

import "time"

const (
	FormatJSON = "json"
	FormatHTML = "html"
	FormatPDF  = "pdf"
)

type ExportOptions struct {
	IncludeAudio       bool `json:"includeAudio"`
	IncludeCredentials bool `json:"includeCredentials"`
}

type CredentialCipher struct {
	Mode       string `json:"mode"`
	Algorithm  string `json:"algorithm"`
	Version    int    `json:"version"`
	Salt       string `json:"salt"`
	Ciphertext string `json:"ciphertext"`
}

type MessageExport struct {
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"`
	Media            string    `json:"media,omitempty"`
	Audio            string    `json:"audio,omitempty"`
	AudioMimeType    string    `json:"audioMimeType,omitempty"`
	ToolCalls        string    `json:"toolCalls,omitempty"`
	ToolCallID       string    `json:"toolCallId,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	Model            string    `json:"model,omitempty"`
	Source           string    `json:"source,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	ParentIndex      *int      `json:"parentIndex,omitempty"`
	TurnIndex        *int      `json:"turnIndex,omitempty"`
}

type ConversationExport struct {
	Title     string          `json:"title"`
	Channel   string          `json:"channel,omitempty"`
	ContactID string          `json:"contactId,omitempty"`
	Summary   string          `json:"summary,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	Messages  []MessageExport `json:"messages"`
}

type CredentialExport struct {
	Pattern      string            `json:"pattern"`
	AuthType     string            `json:"authType"`
	Token        string            `json:"token,omitempty"`
	Username     string            `json:"username,omitempty"`
	Password     string            `json:"password,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	ExpiresAt    int64             `json:"expiresAt,omitempty"`
	RefreshURL   string            `json:"refreshUrl,omitempty"`
	ClientID     string            `json:"clientId,omitempty"`
	ClientSecret string            `json:"clientSecret,omitempty"`
}

type ExportResources struct {
	Conversations []ConversationExport `json:"conversations,omitempty"`
	Credentials   any                  `json:"credentials,omitempty"`
}

type ExportFile struct {
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exportedAt"`
	AppVersion string          `json:"appVersion,omitempty"`
	Options    ExportOptions   `json:"options"`
	Resources  ExportResources `json:"resources"`
}

type ExportRequest struct {
	All                      bool     `json:"all"`
	ConversationIDs          []string `json:"conversationIds,omitempty"`
	ProviderIDs              []string `json:"providerIds,omitempty"`
	ProfileSlugs             []string `json:"profileSlugs,omitempty"`
	SkillSlugs               []string `json:"skillSlugs,omitempty"`
	AllowlistSlugs           []string `json:"allowlistSlugs,omitempty"`
	MCPServerSlugs           []string `json:"mcpServerSlugs,omitempty"`
	JobIDs                   []string `json:"jobIds,omitempty"`
	TaskListIDs              []string `json:"taskListIds,omitempty"`
	ChannelNames             []string `json:"channelNames,omitempty"`
	IncludeContacts          bool     `json:"includeContacts"`
	IncludeWorkspace         bool     `json:"includeWorkspace"`
	IncludeAudio             bool     `json:"includeAudio"`
	IncludeCredentials       bool     `json:"includeCredentials"`
	CredentialExportPassword string   `json:"credentialExportPassword,omitempty"`
	OutputFormat             string   `json:"outputFormat,omitempty"`
}

type ImportResult struct {
	Success  bool     `json:"success"`
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
	Message  string   `json:"message"`
}
