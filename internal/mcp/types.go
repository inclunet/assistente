package mcp

import (
	"encoding/json"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TransportType define o tipo de transporte do servidor MCP.
type TransportType string

const (
	TransportStdio      TransportType = "stdio"
	TransportSSE        TransportType = "sse"
	TransportStreamable TransportType = "streamable"
)

// ConnectionStatus representa o estado de conexão de um servidor MCP.
type ConnectionStatus string

const (
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusConnecting   ConnectionStatus = "connecting"
	StatusConnected    ConnectionStatus = "connected"
	StatusError        ConnectionStatus = "error"
)

// AuthType identifica o mecanismo de autenticação de um servidor MCP.
type AuthType string

const (
	AuthNone                    AuthType = "none"
	AuthBearer                  AuthType = "bearer"
	AuthBasic                   AuthType = "basic"
	AuthOAuth2ClientCredentials AuthType = "oauth2_client_credentials"
	AuthOAuth2PKCE              AuthType = "oauth2_pkce"
)

// ServerConfig é a configuração de um servidor MCP, armazenada em JSON.
// Cada arquivo em .assistente/mcp/ representa um servidor.
//
// Segredos (tokens, client_secret) são armazenados no credential manager
// centralizado, não neste arquivo. Os campos OAuth2 aqui são apenas
// configuração não-sensível (client_id, URLs, scopes).
type ServerConfig struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Transport   TransportType     `json:"transport" yaml:"transport"`
	Command     string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	URL         string            `json:"url,omitempty" yaml:"url,omitempty"`
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	AutoConnect bool              `json:"auto_connect" yaml:"auto_connect"`

	// Auth config (não-sensível). Segredos ficam no credential manager.
	AuthType               AuthType `json:"auth_type,omitempty"`
	OAuth2ClientID         string   `json:"oauth2_client_id,omitempty"`
	OAuth2TokenURL         string   `json:"oauth2_token_url,omitempty"`
	OAuth2AuthURL          string   `json:"oauth2_auth_url,omitempty"`
	OAuth2Scopes           []string `json:"oauth2_scopes,omitempty"`
	OAuth2RegistrationURL  string   `json:"oauth2_registration_url,omitempty"`
	OAuth2CallbackPort     int      `json:"oauth2_callback_port,omitempty"`
	OAuth2CallbackHost     string   `json:"oauth2_callback_host,omitempty"`
}

// ServerStatus é o estado runtime de um servidor MCP (não persistido).
type ServerStatus struct {
	Slug        string            `json:"slug"`
	Config      ServerConfig      `json:"config"`
	Status      ConnectionStatus  `json:"status"`
	Error       string            `json:"error,omitempty"`
	Tools       []MCPToolInfo     `json:"tools"`
	Resources   []MCPResourceInfo `json:"resources"`
	Prompts     []MCPPromptInfo   `json:"prompts"`
	ConnectedAt *time.Time        `json:"connectedAt,omitempty"`
	LastPing    *time.Time        `json:"lastPing,omitempty"`
	RetryCount  int               `json:"retryCount,omitempty"`
	Roots       []Root            `json:"roots,omitempty"`
	Capabilities ServerCapabilities `json:"capabilities"`
	HasAuth     bool              `json:"hasAuth"`
}

// ServerInfo é a versão exportada para o frontend (sem campos sensíveis como env).
type ServerInfo struct {
	Slug          string            `json:"slug"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Transport     TransportType     `json:"transport"`
	Status        ConnectionStatus  `json:"status"`
	Error         string            `json:"error,omitempty"`
	ToolCount     int               `json:"toolCount"`
	Tools         []MCPToolInfo     `json:"tools"`
	ResourceCount int               `json:"resourceCount"`
	Resources     []MCPResourceInfo `json:"resources"`
	PromptCount   int               `json:"promptCount"`
	Prompts       []MCPPromptInfo   `json:"prompts"`
	Enabled       bool              `json:"enabled"`
	AutoConnect   bool              `json:"autoConnect"`
	ConnectedAt   string            `json:"connectedAt,omitempty"`
	LastPing      string            `json:"lastPing,omitempty"`
	// Campos de config visíveis (sem env)
	Command  string   `json:"command,omitempty"`
	Args     []string `json:"args,omitempty"`
	URL      string   `json:"url,omitempty"`
	AuthType AuthType `json:"authType,omitempty"`
	HasAuth  bool     `json:"hasAuth"`
}

// MCPToolInfo contém informações sobre uma tool exposta por um servidor MCP.
type MCPToolInfo struct {
	Name        string          `json:"name"`        // nome original da tool no servidor MCP
	FullName    string          `json:"fullName"`     // nome registrado no registry (namespaced)
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`       // JSON Schema dos parâmetros
	ServerSlug  string          `json:"serverSlug"`
}

// MCPResourceInfo contém informações sobre um resource exposto por um servidor MCP.
type MCPResourceInfo struct {
	URI         string `json:"uri"`         // URI do resource
	Name        string `json:"name"`        // nome do resource
	Description string `json:"description"` // descrição do resource
	MIMEType    string `json:"mimeType"`    // tipo MIME do resource
	ServerSlug  string `json:"serverSlug"`
}

// MCPPromptInfo contém informações sobre um prompt exposto por um servidor MCP.
type MCPPromptInfo struct {
	Name        string                `json:"name"`        // nome do prompt
	Description string                `json:"description"` // descrição do prompt
	Arguments   []MCPPromptArgument   `json:"arguments"`   // argumentos do prompt
	ServerSlug  string                `json:"serverSlug"`
}

// MCPPromptArgument representa um argumento de um prompt MCP.
type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// toServerInfo converte ServerStatus para ServerInfo (versão frontend-safe).
func (s *ServerStatus) toServerInfo() ServerInfo {
	info := ServerInfo{
		Slug:          s.Slug,
		Name:          s.Config.Name,
		Description:   s.Config.Description,
		Transport:     s.Config.Transport,
		Status:        s.Status,
		Error:         s.Error,
		ToolCount:     len(s.Tools),
		Tools:         s.Tools,
		ResourceCount: len(s.Resources),
		Resources:     s.Resources,
		PromptCount:   len(s.Prompts),
		Prompts:       s.Prompts,
		Enabled:       s.Config.Enabled,
		AutoConnect:   s.Config.AutoConnect,
		Command:       s.Config.Command,
		Args:          s.Config.Args,
		URL:           s.Config.URL,
		AuthType:      s.Config.AuthType,
		HasAuth:       s.HasAuth,
	}
	if s.ConnectedAt != nil {
		info.ConnectedAt = s.ConnectedAt.Format(time.RFC3339)
	}
	if s.LastPing != nil {
		info.LastPing = s.LastPing.Format(time.RFC3339)
	}
	if info.Tools == nil {
		info.Tools = []MCPToolInfo{}
	}
	if info.Resources == nil {
		info.Resources = []MCPResourceInfo{}
	}
	if info.Prompts == nil {
		info.Prompts = []MCPPromptInfo{}
	}
	return info
}

// SamplingRequest representa uma requisição de sampling do servidor MCP.
type SamplingRequest struct {
	Messages          []mcpsdk.SamplingMessage `json:"messages"`
	ModelPreferences  *ModelPreferences        `json:"modelPreferences,omitempty"`
	SystemPrompt      string                   `json:"systemPrompt,omitempty"`
	IncludeContext    string                   `json:"includeContext,omitempty"`
	Temperature       *float64                 `json:"temperature,omitempty"`
	MaxTokens         int                      `json:"maxTokens"`
	StopSequences     []string                 `json:"stopSequences,omitempty"`
	Metadata          map[string]any           `json:"metadata,omitempty"`
}

// ModelPreferences define preferências de modelo para sampling.
type ModelPreferences struct {
	Hints         []ModelHint    `json:"hints,omitempty"`
	CostPriority  *float64       `json:"costPriority,omitempty"`
	SpeedPriority *float64       `json:"speedPriority,omitempty"`
	IntelligencePriority *float64 `json:"intelligencePriority,omitempty"`
}

// ModelHint sugere um modelo específico para sampling.
type ModelHint struct {
	Name string `json:"name"`
}

// LogLevel define o nível de log do servidor MCP.
type LogLevel string

const (
	LogLevelDebug     LogLevel = "debug"
	LogLevelInfo      LogLevel = "info"
	LogLevelNotice    LogLevel = "notice"
	LogLevelWarning   LogLevel = "warning"
	LogLevelError     LogLevel = "error"
	LogLevelCritical  LogLevel = "critical"
	LogLevelAlert     LogLevel = "alert"
	LogLevelEmergency LogLevel = "emergency"
)

// LogEntry representa uma entrada de log do servidor MCP.
type LogEntry struct {
	Level     LogLevel       `json:"level"`
	Logger    string         `json:"logger,omitempty"`
	Data      any            `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
	ServerSlug string        `json:"serverSlug"`
}

// Root representa um diretório raiz do workspace.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// ProgressToken identifica uma operação em progresso.
type ProgressToken struct {
	Value any `json:"value"` // string ou número
}

// ProgressNotification representa uma notificação de progresso.
type ProgressNotification struct {
	ProgressToken ProgressToken `json:"progressToken"`
	Progress      float64       `json:"progress"`      // 0-100
	Total         *float64      `json:"total,omitempty"` // opcional
}

// ResourceUpdated representa uma notificação de atualização de resource.
type ResourceUpdated struct {
	URI string `json:"uri"`
}

// ServerCapabilities descreve as capabilities que o servidor suporta.
type ServerCapabilities struct {
	Logging           *LoggingCapability           `json:"logging,omitempty"`
	Prompts           *PromptsCapability           `json:"prompts,omitempty"`
	Resources         *ResourcesCapability         `json:"resources,omitempty"`
	Tools             *ToolsCapability             `json:"tools,omitempty"`
	Sampling          *SamplingCapability          `json:"sampling,omitempty"`
}

// LoggingCapability indica se o servidor suporta logging.
type LoggingCapability struct{}

// PromptsCapability indica se o servidor suporta prompts.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indica se o servidor suporta resources.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsCapability indica se o servidor suporta tools.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability indica se o servidor suporta sampling.
type SamplingCapability struct{}

