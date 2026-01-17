package database

import (
	"encoding/json"
	"time"
)

// ==================== Conversation & Messages ====================

// ChatPreferences representa as preferências locais de uma conversa
type ChatPreferences struct {
	// Chat
	Model                string  `json:"model,omitempty"`
	Temperature          float64 `json:"temperature,omitempty"`
	MaxTokens            int     `json:"max_tokens,omitempty"`
	TopP                 float64 `json:"top_p,omitempty"`
	UseTools             *bool   `json:"use_tools,omitempty"`
	ShowInternalMessages *bool   `json:"show_internal_messages,omitempty"`
	// Voz TTS
	Voice       string `json:"voice,omitempty"`
	AutoSpeak   *bool  `json:"auto_speak,omitempty"`
	VoiceVolume int    `json:"voice_volume,omitempty"`
	VoiceRate   int    `json:"voice_rate,omitempty"`
	// STT/Transcrição
	STTProvider   string `json:"stt_provider,omitempty"`
	RecordingMode string `json:"recording_mode,omitempty"`
}

// Conversation representa uma conversa
type Conversation struct {
	ID          uint          `json:"id" gorm:"primaryKey"`
	Title       string        `json:"title"`
	Preferences string        `json:"preferences,omitempty" gorm:"type:text"` // JSON das preferências locais
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
	Messages    []ChatMessage `json:"messages,omitempty" gorm:"foreignKey:ConversationID"`
}

// GetPreferences retorna as preferências da conversa deserializadas
func (c *Conversation) GetPreferences() *ChatPreferences {
	if c.Preferences == "" {
		return nil
	}
	var prefs ChatPreferences
	if err := json.Unmarshal([]byte(c.Preferences), &prefs); err != nil {
		return nil
	}
	return &prefs
}

// SetPreferences define as preferências da conversa
func (c *Conversation) SetPreferences(prefs *ChatPreferences) {
	if prefs == nil {
		c.Preferences = ""
		return
	}
	data, err := json.Marshal(prefs)
	if err != nil {
		return
	}
	c.Preferences = string(data)
}

// ChatMessage representa uma mensagem na conversa
// A hierarquia é definida pelo ParentID:
//   - ParentID=null: mensagem de nível 0 (user/assistant principal)
//   - ParentID=ID_delegação: mensagem de nível 1 (agente respondendo ao orquestrador)
//   - ParentID=ID_agente_tool: mensagem de nível 2 (tool respondendo ao agente)
type ChatMessage struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	ConversationID   uint      `json:"conversation_id" gorm:"index"`
	ParentID         *uint     `json:"parent_id,omitempty" gorm:"index"` // ID da mensagem pai (define hierarquia)
	Role             string    `json:"role"`                             // user, assistant, tool, system
	Content          string    `json:"content"`
	Media            string    `json:"media,omitempty"`             // JSON com mídias (imagens, áudio, etc) em base64
	ToolCalls        string    `json:"tool_calls,omitempty"`        // JSON serializado
	ToolResults      string    `json:"tool_results,omitempty"`      // JSON serializado (deprecated, usar hierarquia)
	ToolCallID       string    `json:"tool_call_id,omitempty"`      // ID da tool call (para role="tool")
	AgentName        string    `json:"agent_name,omitempty"`        // Nome do agente que processou (file_manager, faq, etc)
	PromptTokens     int       `json:"prompt_tokens,omitempty"`     // Tokens de entrada
	CompletionTokens int       `json:"completion_tokens,omitempty"` // Tokens de saída
	TotalTokens      int       `json:"total_tokens,omitempty"`      // Total de tokens
	Model            string    `json:"model,omitempty"`             // Modelo usado
	CreatedAt        time.Time `json:"created_at"`
}

// ==================== Chat Tabs ====================

// ChatTab representa uma aba de chat aberta na interface
type ChatTab struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	ConversationID *uint     `json:"conversation_id,omitempty" gorm:"index"`
	Title          string    `json:"title" gorm:"default:'Nova conversa'"`
	Icon           string    `json:"icon" gorm:"default:'💬'"`
	Position       int       `json:"position" gorm:"index;default:0"`
	IsActive       bool      `json:"is_active" gorm:"index;default:false"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Relacionamento
	Conversation *Conversation `json:"conversation,omitempty" gorm:"foreignKey:ConversationID"`
}

// ==================== Memory ====================

// Memory representa uma memória persistente do assistente sobre o usuário
type Memory struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Title     string    `json:"title" gorm:"type:text"`
	Content   string    `json:"content" gorm:"type:text"`
	Category  string    `json:"category,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Implementa providers.MemoryRecord
func (m *Memory) GetID() uint         { return m.ID }
func (m *Memory) GetTitle() string    { return m.Title }
func (m *Memory) GetContent() string  { return m.Content }
func (m *Memory) GetCategory() string { return m.Category }

// ==================== FAQ ====================

// FAQ representa uma pergunta e resposta do FAQ
type FAQ struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Question  string    `json:"question" gorm:"type:text"`
	Answer    string    `json:"answer" gorm:"type:text"`
	Tags      string    `json:"tags,omitempty"`
	Embedding string    `json:"-" gorm:"type:text"` // Embedding JSON (não expõe na API)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Implementa providers.FAQRecord
func (f *FAQ) GetID() uint         { return f.ID }
func (f *FAQ) GetQuestion() string { return f.Question }
func (f *FAQ) GetAnswer() string   { return f.Answer }
func (f *FAQ) GetTags() string     { return f.Tags }

// GetEmbedding retorna o embedding como slice de float32
func (f *FAQ) GetEmbedding() []float32 {
	if f.Embedding == "" {
		return nil
	}
	var embedding []float32
	json.Unmarshal([]byte(f.Embedding), &embedding)
	return embedding
}

// SetEmbedding define o embedding a partir de um slice de float32
func (f *FAQ) SetEmbedding(embedding []float32) {
	data, _ := json.Marshal(embedding)
	f.Embedding = string(data)
}

// ==================== Agent Config ====================

// AgentConfig representa a configuração persistente de um agente
type AgentConfig struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"uniqueIndex;not null"`
	DisplayName  string    `json:"display_name" gorm:"type:text"`
	Description  string    `json:"description" gorm:"type:text"`
	AgentType    string    `json:"agent_type" gorm:"type:text"`
	Model        string    `json:"model" gorm:"type:text"`
	SystemPrompt string    `json:"system_prompt" gorm:"type:text"`
	Config       string    `json:"config,omitempty" gorm:"type:text"`
	Enabled      bool      `json:"enabled" gorm:"default:true"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ==================== HTTP Agent ====================

// HTTPAgent representa a configuração de um agente HTTP
type HTTPAgent struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	AgentConfigID  uint           `json:"agent_config_id" gorm:"index"`
	BaseURL        string         `json:"base_url" gorm:"type:text;not null"`
	AuthType       string         `json:"auth_type" gorm:"type:text"`
	AuthConfig     string         `json:"auth_config" gorm:"type:text"`
	DefaultHeaders string         `json:"default_headers" gorm:"type:text"`
	TimeoutSeconds int            `json:"timeout_seconds" gorm:"default:30"`
	RetryCount     int            `json:"retry_count" gorm:"default:3"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Endpoints      []HTTPEndpoint `json:"endpoints,omitempty" gorm:"foreignKey:HTTPAgentID"`
}

// HTTPEndpoint representa um endpoint/função de um agente HTTP
type HTTPEndpoint struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	HTTPAgentID      uint      `json:"http_agent_id" gorm:"index;not null"`
	Name             string    `json:"name" gorm:"type:text;not null"`
	Description      string    `json:"description" gorm:"type:text"`
	Method           string    `json:"method" gorm:"type:text;not null"`
	PathTemplate     string    `json:"path_template" gorm:"type:text;not null"`
	QueryTemplate    string    `json:"query_template" gorm:"type:text"`
	HeadersJSON      string    `json:"headers_json" gorm:"type:text"`
	BodyTemplate     string    `json:"body_template" gorm:"type:text"`
	Parameters       string    `json:"parameters" gorm:"type:text;not null"`
	ResponseTemplate string    `json:"response_template" gorm:"type:text"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ==================== MCP Agent ====================

// MCPAgentDB representa a configuração persistente de um agente MCP
type MCPAgentDB struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	AgentConfigID uint      `json:"agent_config_id" gorm:"index"`
	TransportType string    `json:"transport_type" gorm:"type:text;default:'stdio'"`
	ServerCommand string    `json:"server_command" gorm:"type:text"`
	ServerArgs    string    `json:"server_args" gorm:"type:text"`
	ServerEnv     string    `json:"server_env" gorm:"type:text"`
	WorkingDir    string    `json:"working_dir" gorm:"type:text"`
	ServerURL     string    `json:"server_url" gorm:"type:text"`
	AuthType      string    `json:"auth_type" gorm:"type:text"`
	AuthValue     string    `json:"auth_value" gorm:"type:text"`
	HTTPHeaders   string    `json:"http_headers" gorm:"type:text"`
	ExecutionMode string    `json:"execution_mode" gorm:"type:text;default:'convert'"`
	AutoConnect   bool      `json:"auto_connect" gorm:"default:false"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName define o nome da tabela
func (MCPAgentDB) TableName() string {
	return "mcp_agents"
}

// ==================== Model Capability ====================

// ModelCapability representa as capacidades aprendidas de um modelo
type ModelCapability struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	ModelName         string    `json:"model_name" gorm:"uniqueIndex;not null"`
	SupportsVision    *bool     `json:"supports_vision"`
	SupportsAudio     *bool     `json:"supports_audio"`
	SupportsVideo     *bool     `json:"supports_video"`
	SupportsDocuments *bool     `json:"supports_documents"`
	SupportsTools     *bool     `json:"supports_tools"`
	SupportsStreaming *bool     `json:"supports_streaming"`
	SupportsJSON      *bool     `json:"supports_json"`
	LastTested        time.Time `json:"last_tested"`
	TimesUsed         int       `json:"times_used" gorm:"default:0"`
	LastError         string    `json:"last_error,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ==================== OAuth Connection ====================

// OAuthConnection representa uma conexão OAuth salva
type OAuthConnection struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProviderID   string    `json:"provider_id" gorm:"type:text;not null;index"`
	ProviderName string    `json:"provider_name" gorm:"type:text"`
	UserEmail    string    `json:"user_email" gorm:"type:text"`
	UserName     string    `json:"user_name" gorm:"type:text"`
	UserID       string    `json:"user_id" gorm:"type:text"`
	AccessToken  string    `json:"-" gorm:"type:text"`
	RefreshToken string    `json:"-" gorm:"type:text"`
	TokenType    string    `json:"token_type" gorm:"type:text"`
	Scopes       string    `json:"scopes" gorm:"type:text"`
	ExpiresAt    time.Time `json:"expires_at"`
	IsActive     bool      `json:"is_active" gorm:"default:true"`
	LastUsedAt   time.Time `json:"last_used_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// IsExpired verifica se o token expirou
func (c *OAuthConnection) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// NeedsRefresh verifica se o token precisa ser renovado (30s antes de expirar)
func (c *OAuthConnection) NeedsRefresh() bool {
	return time.Now().Add(30 * time.Second).After(c.ExpiresAt)
}

// ==================== File Agent ====================

// FileAgentAuthorizedPath representa uma pasta autorizada para operações de arquivo
type FileAgentAuthorizedPath struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Path        string    `json:"path" gorm:"type:text;not null"`
	AllowDelete bool      `json:"allow_delete" gorm:"default:true"`
	AllowWrite  bool      `json:"allow_write" gorm:"default:true"`
	Recursive   bool      `json:"recursive" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
