package database

import "time"

// ==================== MCP Servers & Tool Catalog ====================

// MCPServer armazena a configuração persistente de um servidor MCP.
// Estado volátil (sessão SDK, goroutines, conexão ativa) permanece no mcp.Manager.
type MCPServer struct {
	UUIDModel
	UserID string `json:"userId" gorm:"not null;index;uniqueIndex:ux_mcp_servers_user_slug"`
	Slug   string `json:"slug" gorm:"not null;index;uniqueIndex:ux_mcp_servers_user_slug"`

	Name        string `json:"name" gorm:"not null"`
	Description string `json:"description,omitempty" gorm:"type:text"`
	Transport   string `json:"transport" gorm:"not null;index"`
	Command     string `json:"command,omitempty" gorm:"type:text"`
	Args        string `json:"args,omitempty" gorm:"type:text"` // JSON array
	Env         string `json:"env,omitempty" gorm:"type:text"`  // JSON object
	URL         string `json:"url,omitempty" gorm:"type:text"`

	AuthType              string        `json:"authType,omitempty" gorm:"not null;default:'none';index"`
	OAuth2ClientID        string        `json:"oauth2ClientId,omitempty"`
	OAuth2AuthURL         string        `json:"oauth2AuthUrl,omitempty" gorm:"type:text"`
	OAuth2TokenURL        string        `json:"oauth2TokenUrl,omitempty" gorm:"type:text"`
	OAuth2Scopes          string        `json:"oauth2Scopes,omitempty" gorm:"type:text"` // JSON array
	OAuth2CallbackPort    int           `json:"oauth2CallbackPort,omitempty"`
	OAuth2CallbackHost    string        `json:"oauth2CallbackHost,omitempty"`
	OAuth2RegistrationURL string        `json:"oauth2RegistrationUrl,omitempty" gorm:"type:text"`
	OAuth2DeviceAuthURL   string        `json:"oauth2DeviceAuthUrl,omitempty" gorm:"type:text"`
	DisableSSE            bool          `json:"disableSse,omitempty" gorm:"default:false"`
	PreferBridge          bool          `json:"preferBridge,omitempty" gorm:"default:false"`
	Enabled               bool          `json:"enabled" gorm:"not null;default:true;index"`
	AutoConnect           bool          `json:"autoConnect" gorm:"not null;default:true;index"`
	LastConnectedAt       *time.Time    `json:"lastConnectedAt,omitempty"`
	LastDiscoveredAt      *time.Time    `json:"lastDiscoveredAt,omitempty"`
	LastError             string        `json:"lastError,omitempty" gorm:"type:text"`
	User                  *User         `json:"-" gorm:"foreignKey:UserID"`
	Tools                 []ToolCatalog `json:"-" gorm:"foreignKey:MCPServerID"`
}

// MCPServerLog registra eventos observáveis do lifecycle de um servidor MCP.
type MCPServerLog struct {
	UUIDModel
	ServerID  string     `json:"serverId" gorm:"not null;index"`
	Timestamp time.Time  `json:"timestamp" gorm:"not null;index"`
	Type      string     `json:"type" gorm:"not null;index"`
	Message   string     `json:"message,omitempty" gorm:"type:text"`
	Data      string     `json:"data,omitempty" gorm:"type:text"` // JSON object
	Server    *MCPServer `json:"-" gorm:"foreignKey:ServerID"`
}

// ToolCatalog é o catálogo persistido de capabilities de tools.
// Builtins usam UserID/MCPServerID nulos; MCP tools herdam escopo via MCPServerID.
type ToolCatalog struct {
	UUIDModel
	UserID      *string `json:"userId,omitempty" gorm:"index"`
	MCPServerID *string `json:"mcpServerId,omitempty" gorm:"index"`

	Name        string `json:"name" gorm:"not null;index"`
	DisplayName string `json:"displayName" gorm:"not null"`
	Description string `json:"description,omitempty" gorm:"type:text"`
	Origin      string `json:"origin" gorm:"not null;index"`
	Category    string `json:"category,omitempty" gorm:"index"`
	Class       string `json:"class,omitempty" gorm:"index"`
	Package     string `json:"package,omitempty" gorm:"column:package;index"`
	Risk        string `json:"risk,omitempty" gorm:"index"`
	Schema      string `json:"schema,omitempty" gorm:"type:text"`
	SchemaHash  string `json:"schemaHash,omitempty" gorm:"index"`
	SchemaBytes int    `json:"schemaBytes,omitempty"`
	Tags        string `json:"tags,omitempty" gorm:"type:text"` // JSON array

	AvailabilityStatus string     `json:"availabilityStatus" gorm:"not null;default:'available';index"`
	AvailabilityReason string     `json:"availabilityReason,omitempty" gorm:"type:text"`
	LastSeenAt         *time.Time `json:"lastSeenAt,omitempty" gorm:"index"`
	LastAvailableAt    *time.Time `json:"lastAvailableAt,omitempty"`
	LastUnavailableAt  *time.Time `json:"lastUnavailableAt,omitempty"`
	LastTestedAt       *time.Time `json:"lastTestedAt,omitempty"`
	LastTestStatus     string     `json:"lastTestStatus,omitempty" gorm:"index"`
	LastTestError      string     `json:"lastTestError,omitempty" gorm:"type:text"`

	MCPServer *MCPServer `json:"-" gorm:"foreignKey:MCPServerID"`
}

func (ToolCatalog) TableName() string {
	return "tool_catalog"
}
