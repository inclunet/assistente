package database

import (
	"encoding/json"
	"fmt"
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
	Voice          string `json:"voice,omitempty"`
	AutoSpeak      *bool  `json:"auto_speak,omitempty"`
	VoiceVolume    int    `json:"voice_volume,omitempty"`
	VoiceRate      int    `json:"voice_rate,omitempty"`
	VoiceProfileID *uint  `json:"voice_profile_id,omitempty"` // ID do perfil de voz (nil = usar padrão)
	// STT/Transcrição
	STTProvider   string `json:"stt_provider,omitempty"`
	RecordingMode string `json:"recording_mode,omitempty"`
}

// Conversation representa uma conversa
type Conversation struct {
	ID            uint          `json:"id" gorm:"primaryKey"`
	Title         string        `json:"title"`
	ChatProfileID *uint         `json:"chat_profile_id,omitempty" gorm:"index"` // FK → ChatProfile (nil = usar padrão)
	Preferences   string        `json:"preferences,omitempty" gorm:"type:text"` // JSON das preferências locais (override)
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Messages      []ChatMessage `json:"messages,omitempty" gorm:"foreignKey:ConversationID"`
	MessageCount  int           `json:"message_count" gorm:"-"` // Campo calculado, não persiste no banco

	// Campos para busca semântica
	Summary               string `json:"summary,omitempty" gorm:"type:text"` // Resumo gerado pelo LLM
	Embedding             string `json:"-" gorm:"type:text"`                 // Embedding do resumo (não expõe na API)
	EmbeddingMessageCount int    `json:"embedding_message_count"`            // Qtd de msgs quando gerou o embedding

	// Relacionamento
	ChatProfile *ChatProfile `json:"chat_profile,omitempty" gorm:"foreignKey:ChatProfileID"`
}

// GetEmbedding retorna o embedding como slice de float32
func (c *Conversation) GetEmbedding() []float32 {
	if c.Embedding == "" {
		return nil
	}
	var embedding []float32
	json.Unmarshal([]byte(c.Embedding), &embedding)
	return embedding
}

// SetEmbedding define o embedding a partir de um slice de float32
func (c *Conversation) SetEmbedding(embedding []float32) {
	data, _ := json.Marshal(embedding)
	c.Embedding = string(data)
}

// NeedsEmbeddingUpdate verifica se o embedding precisa ser atualizado
func (c *Conversation) NeedsEmbeddingUpdate() bool {
	return c.Embedding == "" || c.EmbeddingMessageCount < c.MessageCount
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
	ConversationID   uint      `json:"conversationId" gorm:"index"`
	ParentID         *uint     `json:"parentId,omitempty" gorm:"index"` // ID da mensagem pai (define hierarquia)
	Role             string    `json:"role"`                            // user, assistant, tool, system
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"`        // Reasoning/thinking do modelo (DeepSeek, Claude, o1, etc)
	Media            string    `json:"media,omitempty"`            // JSON com mídias (imagens, áudio, etc) em base64
	ToolCalls        string    `json:"toolCalls,omitempty"`        // JSON serializado
	ToolResults      string    `json:"toolResults,omitempty"`      // JSON serializado (deprecated, usar hierarquia)
	ToolCallID       string    `json:"toolCallId,omitempty"`       // ID da tool call (para role="tool")
	AgentName        string    `json:"agentName,omitempty"`        // Nome do agente que processou (file_manager, faq, etc)
	PromptTokens     int       `json:"promptTokens,omitempty"`     // Tokens de entrada
	CompletionTokens int       `json:"completionTokens,omitempty"` // Tokens de saída
	TotalTokens      int       `json:"totalTokens,omitempty"`      // Total de tokens
	Model            string    `json:"model,omitempty"`            // Modelo usado
	CreatedAt        time.Time `json:"createdAt"`
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
	Embedding string    `json:"-" gorm:"type:text"` // Embedding JSON (não expõe na API)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Implementa providers.MemoryRecord
func (m *Memory) GetID() uint         { return m.ID }
func (m *Memory) GetTitle() string    { return m.Title }
func (m *Memory) GetContent() string  { return m.Content }
func (m *Memory) GetCategory() string { return m.Category }

// GetEmbedding retorna o embedding como slice de float32
func (m *Memory) GetEmbedding() []float32 {
	if m.Embedding == "" {
		return nil
	}
	var embedding []float32
	json.Unmarshal([]byte(m.Embedding), &embedding)
	return embedding
}

// SetEmbedding define o embedding a partir de um slice de float32
func (m *Memory) SetEmbedding(embedding []float32) {
	data, _ := json.Marshal(embedding)
	m.Embedding = string(data)
}

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

// ==================== Voice Profile ====================

// VoiceProfile representa um perfil de configuração de voz TTS
type VoiceProfile struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	Name            string    `json:"name" gorm:"uniqueIndex;not null"`
	Description     string    `json:"description" gorm:"type:text"`
	Provider        string    `json:"provider" gorm:"type:text;not null"`     // disabled, webspeech, sapi5, openai
	VoiceID         string    `json:"voice_id" gorm:"type:text"`              // ID da voz (ex: nova, alloy, Microsoft Maria) - vazio se disabled
	Rate            float64   `json:"rate" gorm:"default:1.0"`                // Velocidade (0.25-4.0 para OpenAI, -10 a 10 para SAPI5)
	Pitch           float64   `json:"pitch" gorm:"default:1.0"`               // Tom (apenas WebSpeech)
	Volume          float64   `json:"volume" gorm:"default:1.0"`              // Volume (0-1)
	EnabledForAgent bool      `json:"enabled_for_agent" gorm:"default:false"` // Ativa TTS para mensagens do assistente
	EnabledForUser  bool      `json:"enabled_for_user" gorm:"default:false"`  // Ativa TTS para mensagens do usuário (lê mensagens enviadas)
	IsDefault       bool      `json:"is_default" gorm:"default:false"`        // Se é o perfil padrão
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ShouldUseAriaLiveForAgent retorna se deve usar aria-live para mensagens do assistente
// (quando TTS do assistente está desativado)
func (v *VoiceProfile) ShouldUseAriaLiveForAgent() bool {
	return v.Provider == "disabled" || !v.EnabledForAgent
}

// ShouldUseAriaLiveForUser retorna se deve usar aria-live para mensagens do usuário
// (quando TTS do usuário está desativado)
func (v *VoiceProfile) ShouldUseAriaLiveForUser() bool {
	return v.Provider == "disabled" || !v.EnabledForUser
}

// Validate valida os campos do perfil de voz
func (v *VoiceProfile) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("name is required")
	}
	if v.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	validProviders := []string{"disabled", "webspeech", "sapi5", "openai"}
	isValidProvider := false
	for _, p := range validProviders {
		if v.Provider == p {
			isValidProvider = true
			break
		}
	}
	if !isValidProvider {
		return fmt.Errorf("provider must be one of: disabled, webspeech, sapi5, openai")
	}
	// VoiceID é obrigatório quando TTS está ativado para assistente ou usuário
	if v.Provider != "disabled" && v.VoiceID == "" && (v.EnabledForAgent || v.EnabledForUser) {
		return fmt.Errorf("voice_id is required when TTS is enabled for agent or user")
	}
	if v.Rate < 0.25 || v.Rate > 4.0 {
		return fmt.Errorf("rate must be between 0.25 and 4.0")
	}
	if v.Pitch < 0.5 || v.Pitch > 2.0 {
		return fmt.Errorf("pitch must be between 0.5 and 2.0")
	}
	if v.Volume < 0 || v.Volume > 1 {
		return fmt.Errorf("volume must be between 0 and 1")
	}
	return nil
}

// IsDisabled retorna true se o perfil não usa TTS
func (v *VoiceProfile) IsDisabled() bool {
	return v.Provider == "disabled" || (!v.EnabledForAgent && !v.EnabledForUser)
}

// ==================== Interaction Profile ====================

// InteractionProfile representa um perfil de interação por voz
// Define configurações comuns compartilhadas por todos os triggers do perfil
type InteractionProfile struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"uniqueIndex;not null"`
	Description string    `json:"description" gorm:"type:text"`
	IsDefault   bool      `json:"is_default" gorm:"default:false"`
	IsActive    bool      `json:"is_active" gorm:"default:false"` // Perfil atualmente ativo (persistido)
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Configurações comuns
	STTProvider    string `json:"stt_provider" gorm:"type:text;default:'webspeech'"` // webspeech, whisper_api
	Language       string `json:"language" gorm:"type:text;default:'pt-BR'"`         // Idioma do reconhecimento
	FeedbackSounds bool   `json:"feedback_sounds" gorm:"default:true"`               // Sons de início/fim

	// Relacionamento com triggers (1:N)
	Triggers []InteractionTrigger `json:"triggers,omitempty" gorm:"foreignKey:ProfileID;constraint:OnDelete:CASCADE"`
}

// Validate valida os campos do perfil de interação
func (p *InteractionProfile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Valida STT provider
	validSTTProviders := []string{"webspeech", "whisper_api"}
	if p.STTProvider != "" && !contains(validSTTProviders, p.STTProvider) {
		return fmt.Errorf("stt_provider must be one of: webspeech, whisper_api")
	}

	return nil
}

// ==================== Interaction Trigger ====================

// TriggerType define os tipos possíveis de trigger
const (
	TriggerTypeHotkey       = "hotkey"        // Atalho de teclado (toggle)
	TriggerTypeButtonPTT    = "button_ptt"    // Botão push-to-talk
	TriggerTypeButtonToggle = "button_toggle" // Botão toggle
	TriggerTypeWakeword     = "wakeword"      // Palavra de ativação
	TriggerTypeVAD          = "vad"           // Detecção contínua de voz
)

// InteractionTrigger representa uma forma de ativar um perfil de interação
// Um perfil pode ter múltiplos triggers (hotkey, wakeword, button, etc.)
type InteractionTrigger struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ProfileID uint      `json:"profile_id" gorm:"not null;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Tipo: hotkey, button_ptt, button_toggle, wakeword, vad
	Type    string `json:"type" gorm:"type:text;not null"`
	Enabled bool   `json:"enabled" gorm:"default:true"`

	// Como terminar gravação: true=VAD automático, false=manual
	// Aplicável para hotkey e button_toggle
	AutoStop bool `json:"auto_stop" gorm:"default:false"`

	// === Hotkey ===
	// Para type=hotkey: tecla que ACIONA gravação
	// Para type=wakeword/vad: tecla que LIGA/DESLIGA a escuta
	Hotkey             string `json:"hotkey" gorm:"type:text"`                   // Ex: "Ctrl+Shift+Space"
	HotkeyGlobal       bool   `json:"hotkey_global" gorm:"default:true"`         // Global ou local
	HotkeyBringToFront bool   `json:"hotkey_bring_to_front" gorm:"default:true"` // Trazer janela (se global)

	// === Wakeword ===
	WakewordKeyword     string  `json:"wakeword_keyword" gorm:"type:text"`       // Ex: "assistente"
	WakewordProvider    string  `json:"wakeword_provider" gorm:"type:text"`      // webspeech (por enquanto só este)
	WakewordSensitivity float64 `json:"wakeword_sensitivity" gorm:"default:0.5"` // 0.0 - 1.0

	// === VAD Config ===
	// Usado quando auto_stop=true, type=wakeword ou type=vad
	VADSilenceThreshold  float64 `json:"vad_silence_threshold" gorm:"default:0.01"`  // 0-1
	VADSilenceDuration   int     `json:"vad_silence_duration" gorm:"default:1500"`   // ms
	VADActivityThreshold float64 `json:"vad_activity_threshold" gorm:"default:0.02"` // 0-1
	VADActivityDuration  int     `json:"vad_activity_duration" gorm:"default:200"`   // ms
}

// Validate valida os campos do trigger
func (t *InteractionTrigger) Validate() error {
	// Valida tipo
	validTypes := []string{TriggerTypeHotkey, TriggerTypeButtonPTT, TriggerTypeButtonToggle, TriggerTypeWakeword, TriggerTypeVAD}
	if !contains(validTypes, t.Type) {
		return fmt.Errorf("type must be one of: hotkey, button_ptt, button_toggle, wakeword, vad")
	}

	// Valida hotkey para tipo hotkey
	if t.Type == TriggerTypeHotkey && t.Hotkey == "" {
		return fmt.Errorf("hotkey is required for type hotkey")
	}

	// Valida wakeword para tipo wakeword
	if t.Type == TriggerTypeWakeword && t.WakewordKeyword == "" {
		return fmt.Errorf("wakeword_keyword is required for type wakeword")
	}

	// Valida provider wakeword
	if t.Type == TriggerTypeWakeword && t.WakewordProvider != "" {
		validProviders := []string{"webspeech"}
		if !contains(validProviders, t.WakewordProvider) {
			return fmt.Errorf("wakeword_provider must be: webspeech")
		}
	}

	return nil
}

// helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ==================== Chat Profile ====================

// ChatProfile representa um perfil de configuração de conversa
// Define modelo, parâmetros, ferramentas disponíveis e system prompt
type ChatProfile struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"uniqueIndex;not null"`
	Description string    `json:"description" gorm:"type:text"`
	Icon        string    `json:"icon" gorm:"type:text;default:'💬'"`
	IsDefault   bool      `json:"is_default" gorm:"default:false"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Configurações do Modelo
	Model           string  `json:"model" gorm:"type:text"`               // Nome do modelo (gpt-4o, gpt-oss, etc.)
	Temperature     float64 `json:"temperature" gorm:"default:0.7"`       // 0.0 a 2.0
	MaxTokens       int     `json:"max_tokens" gorm:"default:4096"`       // Limite de tokens
	TopP            float64 `json:"top_p" gorm:"default:1.0"`             // 0.0 a 1.0
	ResponseTimeout int     `json:"response_timeout" gorm:"default:180"`  // Timeout em segundos
	EnableThinking  bool    `json:"enable_thinking" gorm:"default:false"` // Habilita reasoning/thinking (Ollama: think=true)

	// Ferramentas/Agentes
	UseTools  bool   `json:"use_tools" gorm:"default:true"` // Habilitar ferramentas
	ToolsList string `json:"tools_list" gorm:"type:text"`   // JSON array de agentes selecionados

	// System Prompt
	SystemPrompt         string `json:"system_prompt" gorm:"type:text"`                // Prompt customizado
	SystemPromptPosition string `json:"system_prompt_position" gorm:"default:'after'"` // "before" ou "after" do prompt base
	IncludeCoreMemories  bool   `json:"include_core_memories" gorm:"default:true"`     // Incluir memórias core no system prompt

	// Embeddings
	EmbeddingsModel      string `json:"embeddings_model" gorm:"type:text;default:'text-embedding-3-small'"` // Modelo de embeddings
	EmbeddingsDimensions int    `json:"embeddings_dimensions" gorm:"default:0"`                             // 0 = padrão do modelo

	// Geração de Imagens
	ImageModel string `json:"image_model" gorm:"type:text;default:'dall-e-3'"` // Modelo de imagens (dall-e-3, dall-e-2, gpt-image-1)

	// UI
	ShowInternalMessages bool `json:"show_internal_messages" gorm:"default:false"` // Mostrar tool calls
}

// Validate valida os campos do perfil de conversa
func (p *ChatProfile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.Temperature < 0 || p.Temperature > 2 {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if p.MaxTokens < 1 {
		return fmt.Errorf("max_tokens must be at least 1")
	}
	if p.TopP < 0 || p.TopP > 1 {
		return fmt.Errorf("top_p must be between 0 and 1")
	}
	if p.ResponseTimeout < 10 {
		return fmt.Errorf("response_timeout must be at least 10 seconds")
	}
	if p.SystemPromptPosition != "" && p.SystemPromptPosition != "before" && p.SystemPromptPosition != "after" {
		return fmt.Errorf("system_prompt_position must be 'before' or 'after'")
	}
	return nil
}

// GetToolsList retorna a lista de ferramentas como slice
func (p *ChatProfile) GetToolsList() []string {
	if p.ToolsList == "" || p.ToolsList == "[]" {
		return []string{}
	}
	var tools []string
	if err := json.Unmarshal([]byte(p.ToolsList), &tools); err != nil {
		return []string{}
	}
	return tools
}

// SetToolsList define a lista de ferramentas a partir de um slice
func (p *ChatProfile) SetToolsList(tools []string) {
	if len(tools) == 0 {
		p.ToolsList = "[]"
		return
	}
	data, err := json.Marshal(tools)
	if err != nil {
		p.ToolsList = "[]"
		return
	}
	p.ToolsList = string(data)
}

// HasTool verifica se uma ferramenta está na lista
func (p *ChatProfile) HasTool(toolName string) bool {
	if !p.UseTools {
		return false
	}
	tools := p.GetToolsList()
	for _, t := range tools {
		if t == toolName {
			return true
		}
	}
	return false
}
