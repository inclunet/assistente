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

type TaskListWorkflowStatusExport struct {
	ID    int    `json:"id"`
	Order int    `json:"order"`
	Label string `json:"label"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

type TaskListWorkflowExport struct {
	Statuses           []TaskListWorkflowStatusExport `json:"statuses"`
	AllowedTransitions map[int][]int                  `json:"allowedTransitions"`
	InitialStatusID    int                            `json:"initialStatusId"`
}

type TaskNoteExport struct {
	Type              int        `json:"type"`
	Content           string     `json:"content"`
	AuthorName        string     `json:"authorName,omitempty"`
	AuthorID          string     `json:"authorId,omitempty"`
	Source            string     `json:"source,omitempty"`
	ExternalID        string     `json:"externalId,omitempty"`
	ExternalParentID  string     `json:"externalParentId,omitempty"`
	ExternalUpdatedAt *time.Time `json:"externalUpdatedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type TaskExport struct {
	Title        string           `json:"title"`
	Description  string           `json:"description,omitempty"`
	Code         string           `json:"code,omitempty"`
	Link         string           `json:"link,omitempty"`
	StatusID     int              `json:"statusId"`
	Order        int              `json:"order"`
	AssigneeName string           `json:"assigneeName,omitempty"`
	AssigneeID   string           `json:"assigneeId,omitempty"`
	CreatorName  string           `json:"creatorName,omitempty"`
	CreatorID    string           `json:"creatorId,omitempty"`
	DueDate      *time.Time       `json:"dueDate,omitempty"`
	CompletedAt  *time.Time       `json:"completedAt,omitempty"`
	CreatedAt    time.Time        `json:"createdAt"`
	Notes        []TaskNoteExport `json:"notes,omitempty"`
	Children     []TaskExport     `json:"children,omitempty"`
}

type TaskListExport struct {
	Title             string                 `json:"title"`
	Slug              string                 `json:"slug,omitempty"`
	Description       string                 `json:"description,omitempty"`
	PreferredViewMode string                 `json:"preferredViewMode,omitempty"`
	ValidationPolicy  string                 `json:"validationPolicy,omitempty"`
	CreatedAt         time.Time              `json:"createdAt"`
	Workflow          TaskListWorkflowExport `json:"workflow"`
	Tasks             []TaskExport           `json:"tasks,omitempty"`
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
	TaskLists     []TaskListExport     `json:"taskLists,omitempty"`
	Credentials   *CredentialCipher    `json:"credentials,omitempty"`
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
	Success                     bool     `json:"success"`
	Imported                    int      `json:"imported"`
	Skipped                     int      `json:"skipped"`
	Failed                      int      `json:"failed"`
	SkippedEmptyConversations   int      `json:"skippedEmptyConversations"`
	SkippedConversationConflict int      `json:"skippedConversationConflict"`
	SkippedTaskListConflict     int      `json:"skippedTaskListConflict"`
	SkippedCredentialConflict   int      `json:"skippedCredentialConflict"`
	SkippedOther                int      `json:"skippedOther"`
	UnsupportedResourceTypes    []string `json:"unsupportedResourceTypes,omitempty"`
	Warnings                    []string `json:"warnings,omitempty"`
	Errors                      []string `json:"errors,omitempty"`
	Message                     string   `json:"message"`
}

type ImportConflict struct {
	ResourceType string `json:"resourceType"`
	Identifier   string `json:"identifier"`
	Reason       string `json:"reason"`
}

type ImportAnalysis struct {
	Version                    int              `json:"version"`
	AppVersion                 string           `json:"appVersion,omitempty"`
	ConversationCount          int              `json:"conversationCount"`
	MessageCount               int              `json:"messageCount"`
	TaskListCount              int              `json:"taskListCount"`
	TaskCount                  int              `json:"taskCount"`
	TaskNoteCount              int              `json:"taskNoteCount"`
	IncludesCredentials        bool             `json:"includesCredentials"`
	RequiresCredentialPassword bool             `json:"requiresCredentialPassword"`
	CredentialCount            int              `json:"credentialCount"`
	ConflictCount              int              `json:"conflictCount"`
	ConversationConflicts      []ImportConflict `json:"conversationConflicts,omitempty"`
	TaskListConflicts          []ImportConflict `json:"taskListConflicts,omitempty"`
	CredentialConflicts        []ImportConflict `json:"credentialConflicts,omitempty"`
	UnsupportedResourceTypes   []string         `json:"unsupportedResourceTypes,omitempty"`
	Warnings                   []string         `json:"warnings,omitempty"`
	CredentialAnalysisError    string           `json:"credentialAnalysisError,omitempty"`
}
