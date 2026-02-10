package mcp

import (
	"encoding/json"
	"time"
)

// TransportType define o tipo de transporte do servidor MCP.
type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportSSE   TransportType = "sse"
)

// ConnectionStatus representa o estado de conexão de um servidor MCP.
type ConnectionStatus string

const (
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusConnecting   ConnectionStatus = "connecting"
	StatusConnected    ConnectionStatus = "connected"
	StatusError        ConnectionStatus = "error"
)

// ServerConfig é a configuração de um servidor MCP, armazenada em YAML.
// Cada arquivo em .assistente/mcp/ representa um servidor.
type ServerConfig struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Transport   TransportType     `json:"transport" yaml:"transport"`
	Command     string            `json:"command,omitempty" yaml:"command,omitempty"`     // apenas stdio
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`           // apenas stdio
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`             // variáveis de ambiente
	URL         string            `json:"url,omitempty" yaml:"url,omitempty"`             // apenas sse
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	AutoConnect bool              `json:"auto_connect" yaml:"auto_connect"`
}

// ServerStatus é o estado runtime de um servidor MCP (não persistido).
type ServerStatus struct {
	Slug        string           `json:"slug"`
	Config      ServerConfig     `json:"config"`
	Status      ConnectionStatus `json:"status"`
	Error       string           `json:"error,omitempty"`
	Tools       []MCPToolInfo    `json:"tools"`
	ConnectedAt *time.Time       `json:"connectedAt,omitempty"`
}

// ServerInfo é a versão exportada para o frontend (sem campos sensíveis como env).
type ServerInfo struct {
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Transport   TransportType    `json:"transport"`
	Status      ConnectionStatus `json:"status"`
	Error       string           `json:"error,omitempty"`
	ToolCount   int              `json:"toolCount"`
	Tools       []MCPToolInfo    `json:"tools"`
	Enabled     bool             `json:"enabled"`
	AutoConnect bool             `json:"autoConnect"`
	ConnectedAt string           `json:"connectedAt,omitempty"`
	// Campos de config visíveis (sem env)
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	URL     string   `json:"url,omitempty"`
}

// MCPToolInfo contém informações sobre uma tool exposta por um servidor MCP.
type MCPToolInfo struct {
	Name        string          `json:"name"`        // nome original da tool no servidor MCP
	FullName    string          `json:"fullName"`     // nome registrado no registry (namespaced)
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`       // JSON Schema dos parâmetros
	ServerSlug  string          `json:"serverSlug"`
}

// toServerInfo converte ServerStatus para ServerInfo (versão frontend-safe).
func (s *ServerStatus) toServerInfo() ServerInfo {
	info := ServerInfo{
		Slug:        s.Slug,
		Name:        s.Config.Name,
		Description: s.Config.Description,
		Transport:   s.Config.Transport,
		Status:      s.Status,
		Error:       s.Error,
		ToolCount:   len(s.Tools),
		Tools:       s.Tools,
		Enabled:     s.Config.Enabled,
		AutoConnect: s.Config.AutoConnect,
		Command:     s.Config.Command,
		Args:        s.Config.Args,
		URL:         s.Config.URL,
	}
	if s.ConnectedAt != nil {
		info.ConnectedAt = s.ConnectedAt.Format(time.RFC3339)
	}
	if info.Tools == nil {
		info.Tools = []MCPToolInfo{}
	}
	return info
}
