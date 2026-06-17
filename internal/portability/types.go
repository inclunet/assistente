package portability

import "time"

const (
	FormatJSON     = "json"
	FormatHTML     = "html"
	FormatPDF      = "pdf"
	FormatMarkdown = "md"
	FormatMCPJSON  = "mcp-json"
	ExportVersion  = 2
)

type ExportOptions struct {
	IncludeAudio       bool `json:"includeAudio"`
	IncludeCredentials bool `json:"includeCredentials"`
	IncludeTimestamps  bool `json:"includeTimestamps"`
	IncludeReasoning   bool `json:"includeReasoning"`
	IncludeMetadata    bool `json:"includeMetadata"`
}

// ContentExportOptions controla quais blocos de conteúdo entram nas
// exportações ricas (HTML, PDF e Markdown). É o contrato usado pela UI ao
// escolher os toggles de exportação por conversa.
type ContentExportOptions struct {
	IncludeTimestamps bool `json:"includeTimestamps"`
	IncludeReasoning  bool `json:"includeReasoning"`
	IncludeMetadata   bool `json:"includeMetadata"`
}

type CredentialCipher struct {
	Mode       string `json:"mode"`
	Algorithm  string `json:"algorithm"`
	Version    int    `json:"version"`
	Salt       string `json:"salt"`
	Ciphertext string `json:"ciphertext"`
}

type MessageExport struct {
	ID               string    `json:"id"`
	ConversationID   string    `json:"conversationId,omitempty"`
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
	ParentID         string    `json:"parentId,omitempty"`
	TurnID           string    `json:"turnId,omitempty"`
	ParentIndex      *int      `json:"parentIndex,omitempty"`
	TurnIndex        *int      `json:"turnIndex,omitempty"`
}

type ConversationExport struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Channel   string          `json:"channel,omitempty"`
	ContactID string          `json:"contactId,omitempty"`
	Summary   string          `json:"summary,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	Messages  []MessageExport `json:"messages"`
}

type ProviderExport struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	APIFormat         string    `json:"apiFormat,omitempty"`
	BaseURL           string    `json:"baseUrl"`
	Model             string    `json:"model,omitempty"`
	DefaultModel      string    `json:"defaultModel,omitempty"`
	IsDefault         bool      `json:"isDefault,omitempty"`
	Timeout           int       `json:"timeout,omitempty"`
	CredentialPattern string    `json:"credentialPattern,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}

type MCPServerExport struct {
	ID                    string            `json:"id,omitempty"`
	Slug                  string            `json:"slug"`
	Name                  string            `json:"name"`
	Description           string            `json:"description,omitempty"`
	Transport             string            `json:"transport"`
	Command               string            `json:"command,omitempty"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	URL                   string            `json:"url,omitempty"`
	AuthType              string            `json:"authType,omitempty"`
	OAuth2ClientID        string            `json:"oauth2ClientId,omitempty"`
	OAuth2AuthURL         string            `json:"oauth2AuthUrl,omitempty"`
	OAuth2TokenURL        string            `json:"oauth2TokenUrl,omitempty"`
	OAuth2Scopes          []string          `json:"oauth2Scopes,omitempty"`
	OAuth2CallbackPort    int               `json:"oauth2CallbackPort,omitempty"`
	OAuth2CallbackHost    string            `json:"oauth2CallbackHost,omitempty"`
	OAuth2RegistrationURL string            `json:"oauth2RegistrationUrl,omitempty"`
	OAuth2DeviceAuthURL   string            `json:"oauth2DeviceAuthUrl,omitempty"`
	DisableSSE            bool              `json:"disableSse,omitempty"`
	PreferBridge          bool              `json:"preferBridge,omitempty"`
	Enabled               bool              `json:"enabled"`
	AutoConnect           bool              `json:"autoConnect"`
	CreatedAt             time.Time         `json:"createdAt,omitempty"`
	BearerToken           string            `json:"-"`
}

type TaskListWorkflowStatusExport struct {
	ID    int    `json:"id"`
	Order int    `json:"order"`
	Label string `json:"label"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

type TaskListWorkflowExport struct {
	ID                 string                         `json:"id,omitempty"`
	TaskListID         string                         `json:"taskListId,omitempty"`
	Statuses           []TaskListWorkflowStatusExport `json:"statuses"`
	AllowedTransitions map[int][]int                  `json:"allowedTransitions"`
	InitialStatusID    int                            `json:"initialStatusId"`
}

type TaskNoteExport struct {
	ID                string     `json:"id,omitempty"`
	TaskID            string     `json:"taskId,omitempty"`
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
	ID           string           `json:"id,omitempty"`
	TaskListID   string           `json:"taskListId,omitempty"`
	ParentID     string           `json:"parentId,omitempty"`
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
	ID                string                 `json:"id,omitempty"`
	Title             string                 `json:"title"`
	Slug              string                 `json:"slug,omitempty"`
	Description       string                 `json:"description,omitempty"`
	PreferredViewMode string                 `json:"preferredViewMode,omitempty"`
	ValidationPolicy  string                 `json:"validationPolicy,omitempty"`
	CreatedAt         time.Time              `json:"createdAt"`
	Workflow          TaskListWorkflowExport `json:"workflow"`
	Tasks             []TaskExport           `json:"tasks,omitempty"`
}

type MemoryRecordExport struct {
	ID                 string     `json:"id"`
	Content            string     `json:"content"`
	Summary            string     `json:"summary,omitempty"`
	LoadPolicy         string     `json:"loadPolicy"`
	ArchivedFromPolicy string     `json:"archivedFromPolicy,omitempty"`
	Kind               string     `json:"kind"`
	Scope              string     `json:"scope"`
	ScopeRef           string     `json:"scopeRef,omitempty"`
	Tags               string     `json:"tags,omitempty"`
	Importance         int        `json:"importance"`
	Confidence         int        `json:"confidence"`
	SourceType         string     `json:"sourceType,omitempty"`
	SourceID           string     `json:"sourceId,omitempty"`
	LastUsedAt         *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

type CredentialExport struct {
	ID           string            `json:"id,omitempty"`
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
	Providers     []ProviderExport     `json:"providers,omitempty"`
	MCPServers    []MCPServerExport    `json:"mcpServers,omitempty"`
	TaskLists     []TaskListExport     `json:"taskLists,omitempty"`
	MemoryRecords []MemoryRecordExport `json:"memoryRecords,omitempty"`
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
	ExplicitSelection        bool     `json:"explicitSelection,omitempty"`
	ConversationIDs          []string `json:"conversationIds,omitempty"`
	ProviderIDs              []string `json:"providerIds,omitempty"`
	ProfileSlugs             []string `json:"profileSlugs,omitempty"`
	SkillSlugs               []string `json:"skillSlugs,omitempty"`
	AllowlistSlugs           []string `json:"allowlistSlugs,omitempty"`
	MCPServerSlugs           []string `json:"mcpServerSlugs,omitempty"`
	JobIDs                   []string `json:"jobIds,omitempty"`
	TaskListIDs              []string `json:"taskListIds,omitempty"`
	MemoryRecordIDs          []string `json:"memoryRecordIds,omitempty"`
	ChannelNames             []string `json:"channelNames,omitempty"`
	IncludeContacts          bool     `json:"includeContacts"`
	IncludeWorkspace         bool     `json:"includeWorkspace"`
	IncludeAudio             bool     `json:"includeAudio"`
	IncludeCredentials       bool     `json:"includeCredentials"`
	CredentialExportPassword string   `json:"credentialExportPassword,omitempty"`
	OutputFormat             string   `json:"outputFormat,omitempty"`
	// Toggles de conteúdo para exportações ricas. Quando nil, assumem o
	// comportamento padrão (incluído) para não quebrar exports existentes.
	IncludeTimestamps *bool `json:"includeTimestamps,omitempty"`
	IncludeReasoning  *bool `json:"includeReasoning,omitempty"`
	IncludeMetadata   *bool `json:"includeMetadata,omitempty"`
}

// ResolveContentToggle interpreta um toggle opcional de conteúdo. Quando não
// especificado (nil) o padrão é incluir o bloco, preservando o comportamento
// histórico das exportações ricas.
func ResolveContentToggle(v *bool) bool {
	return v == nil || *v
}

type ConflictResolutionStrategy string

const (
	ConflictResolutionSkip      ConflictResolutionStrategy = "skip"
	ConflictResolutionOverwrite ConflictResolutionStrategy = "overwrite"
	ConflictResolutionRename    ConflictResolutionStrategy = "rename"
)

type ImportResolution struct {
	ResourceType string                     `json:"resourceType"`
	Identifier   string                     `json:"identifier"`
	Strategy     ConflictResolutionStrategy `json:"strategy"`
	RenameValue  string                     `json:"renameValue,omitempty"`
}

type ImportRequest struct {
	JSONData                 string             `json:"jsonData"`
	CredentialExportPassword string             `json:"credentialExportPassword,omitempty"`
	Resolutions              []ImportResolution `json:"resolutions,omitempty"`
}

type ImportResult struct {
	Success                     bool     `json:"success"`
	Imported                    int      `json:"imported"`
	Skipped                     int      `json:"skipped"`
	Failed                      int      `json:"failed"`
	SkippedEmptyConversations   int      `json:"skippedEmptyConversations"`
	SkippedConversationConflict int      `json:"skippedConversationConflict"`
	SkippedProviderConflict     int      `json:"skippedProviderConflict"`
	SkippedMCPServerConflict    int      `json:"skippedMcpServerConflict"`
	SkippedTaskListConflict     int      `json:"skippedTaskListConflict"`
	SkippedCredentialConflict   int      `json:"skippedCredentialConflict"`
	SkippedOther                int      `json:"skippedOther"`
	UnsupportedResourceTypes    []string `json:"unsupportedResourceTypes,omitempty"`
	Warnings                    []string `json:"warnings,omitempty"`
	Errors                      []string `json:"errors,omitempty"`
	Message                     string   `json:"message"`
}

type ImportConflict struct {
	ResourceType        string                       `json:"resourceType"`
	Identifier          string                       `json:"identifier"`
	Reason              string                       `json:"reason"`
	SupportedStrategies []ConflictResolutionStrategy `json:"supportedStrategies,omitempty"`
}

type ImportAnalysis struct {
	Version                    int              `json:"version"`
	AppVersion                 string           `json:"appVersion,omitempty"`
	ConversationCount          int              `json:"conversationCount"`
	MessageCount               int              `json:"messageCount"`
	ProviderCount              int              `json:"providerCount"`
	MCPServerCount             int              `json:"mcpServerCount"`
	TaskListCount              int              `json:"taskListCount"`
	TaskCount                  int              `json:"taskCount"`
	TaskNoteCount              int              `json:"taskNoteCount"`
	MemoryRecordCount          int              `json:"memoryRecordCount"`
	IncludesCredentials        bool             `json:"includesCredentials"`
	RequiresCredentialPassword bool             `json:"requiresCredentialPassword"`
	CredentialCount            int              `json:"credentialCount"`
	ConflictCount              int              `json:"conflictCount"`
	ConversationConflicts      []ImportConflict `json:"conversationConflicts,omitempty"`
	ProviderConflicts          []ImportConflict `json:"providerConflicts,omitempty"`
	MCPServerConflicts         []ImportConflict `json:"mcpServerConflicts,omitempty"`
	TaskListConflicts          []ImportConflict `json:"taskListConflicts,omitempty"`
	CredentialConflicts        []ImportConflict `json:"credentialConflicts,omitempty"`
	UnsupportedResourceTypes   []string         `json:"unsupportedResourceTypes,omitempty"`
	Warnings                   []string         `json:"warnings,omitempty"`
	CredentialAnalysisError    string           `json:"credentialAnalysisError,omitempty"`
}
