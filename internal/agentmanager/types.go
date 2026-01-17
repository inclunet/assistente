package agentmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ==================== Data Types ====================

// AgentData representa um agente (como faq.Data, memory.Data)
type AgentData struct {
	ID           uint      `json:"id"`
	Name         string    `json:"name"`
	DisplayName  string    `json:"display_name"`
	Description  string    `json:"description"`
	AgentType    string    `json:"agent_type"`
	Model        string    `json:"model"`
	SystemPrompt string    `json:"system_prompt"`
	Config       string    `json:"config,omitempty"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// HTTPAgentData representa configuração de agente HTTP
type HTTPAgentData struct {
	ID             uint               `json:"id"`
	AgentConfigID  uint               `json:"agent_config_id"`
	BaseURL        string             `json:"base_url"`
	AuthType       string             `json:"auth_type"`
	AuthConfig     string             `json:"auth_config"`
	DefaultHeaders string             `json:"default_headers"`
	TimeoutSeconds int                `json:"timeout_seconds"`
	RetryCount     int                `json:"retry_count"`
	Endpoints      []HTTPEndpointData `json:"endpoints,omitempty"`
}

// HTTPEndpointData representa um endpoint HTTP
type HTTPEndpointData struct {
	ID               uint   `json:"id"`
	HTTPAgentID      uint   `json:"http_agent_id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Method           string `json:"method"`
	PathTemplate     string `json:"path_template"`
	QueryTemplate    string `json:"query_template,omitempty"`
	HeadersJSON      string `json:"headers_json,omitempty"`
	BodyTemplate     string `json:"body_template,omitempty"`
	Parameters       string `json:"parameters"`
	ResponseTemplate string `json:"response_template,omitempty"`
}

// MCPAgentData representa configuração de agente MCP
type MCPAgentData struct {
	ID            uint   `json:"id"`
	AgentConfigID uint   `json:"agent_config_id"`
	TransportType string `json:"transport_type"`
	ServerCommand string `json:"server_command,omitempty"`
	ServerArgs    string `json:"server_args,omitempty"`
	ServerEnv     string `json:"server_env,omitempty"`
	WorkingDir    string `json:"working_dir,omitempty"`
	ServerURL     string `json:"server_url,omitempty"`
	AuthType      string `json:"auth_type,omitempty"`
	AuthValue     string `json:"auth_value,omitempty"`
	HTTPHeaders   string `json:"http_headers,omitempty"`
	ExecutionMode string `json:"execution_mode"`
	AutoConnect   bool   `json:"auto_connect"`
}

// OpenAPIImportResult representa resultado da análise
type OpenAPIImportResult struct {
	DisplayName    string             `json:"display_name"`
	Description    string             `json:"description"`
	BaseURL        string             `json:"base_url"`
	AuthType       string             `json:"auth_type"`
	AuthConfig     map[string]string  `json:"auth_config"`
	DefaultHeaders map[string]string  `json:"default_headers"`
	Endpoints      []ImportedEndpoint `json:"endpoints"`
}

// ImportedEndpoint representa um endpoint importado
type ImportedEndpoint struct {
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Method           string                 `json:"method"`
	PathTemplate     string                 `json:"path_template"`
	QueryTemplate    string                 `json:"query_template"`
	HeadersJSON      string                 `json:"headers_json"`
	BodyTemplate     string                 `json:"body_template"`
	Parameters       map[string]interface{} `json:"parameters"`
	ResponseTemplate string                 `json:"response_template"`
}

// ==================== Request Types ====================

// CreateHTTPAgentRequest encapsula parâmetros para criar agente HTTP
type CreateHTTPAgentRequest struct {
	Name           string
	DisplayName    string
	Description    string
	Model          string
	SystemPrompt   string
	Enabled        bool
	BaseURL        string
	AuthType       string
	AuthConfig     string
	DefaultHeaders string
	TimeoutSeconds int
	RetryCount     int
}

// CreateEndpointRequest encapsula parâmetros para criar endpoint
type CreateEndpointRequest struct {
	HTTPAgentID      uint
	Name             string
	Description      string
	Method           string
	PathTemplate     string
	QueryTemplate    string
	HeadersJSON      string
	BodyTemplate     string
	Parameters       string
	ResponseTemplate string
}

// CreateMCPAgentRequest encapsula parâmetros para criar agente MCP
type CreateMCPAgentRequest struct {
	Name          string
	DisplayName   string
	Description   string
	Model         string
	SystemPrompt  string
	TransportType string
	ServerCommand string
	ServerArgs    string
	ServerEnv     string
	WorkingDir    string
	ServerURL     string
	AuthType      string
	AuthValue     string
	HTTPHeaders   string
	ExecutionMode string
	AutoConnect   bool
	Enabled       bool
}

// UpdateAgentRequest encapsula parâmetros para atualizar agente
type UpdateAgentRequest struct {
	DisplayName    string
	Description    string
	Model          string
	SystemPrompt   string
	Config         string
	Enabled        bool
	BaseURL        string
	AuthType       string
	AuthConfig     string
	DefaultHeaders string
	TimeoutSeconds int
	RetryCount     int
}

// ==================== Manager Interface (Provider Pattern) ====================

// Manager define a interface para gerenciamento de agentes (como faq.Provider, memory.Provider)
type Manager interface {
	// Parsers
	ParseOpenAPISpec(content string) (*OpenAPIImportResult, error)
	ParsePostmanCollection(content string) (*OpenAPIImportResult, error)

	// HTTP Agents
	CreateHTTPAgent(req CreateHTTPAgentRequest) (*AgentData, *HTTPAgentData, error)
	CreateHTTPEndpoint(httpAgentID uint, req CreateEndpointRequest) (*HTTPEndpointData, error)
	GetHTTPEndpoint(id uint) (*HTTPEndpointData, error)
	UpdateHTTPEndpoint(id uint, req CreateEndpointRequest) (*HTTPEndpointData, error)
	DeleteHTTPEndpoint(id uint) error
	GetHTTPEndpointsByAgentID(httpAgentID uint) ([]HTTPEndpointData, error)
	TestHTTPEndpoint(httpAgentID uint, endpointName string, paramsJSON string) (string, error)
	ImportOpenAPIAgent(content, name, model string) (*AgentData, *HTTPAgentData, error)
	ImportPostmanAgent(content, name, model string) (*AgentData, *HTTPAgentData, error)

	// MCP Agents
	CreateMCPAgent(req CreateMCPAgentRequest) (*AgentData, *MCPAgentData, error)
	UpdateMCPAgent(id uint, req CreateMCPAgentRequest) (*MCPAgentData, error)
	DeleteMCPAgent(id uint) error
	GetAllMCPAgents() ([]MCPAgentData, error)

	// Management
	GetAllAgents() ([]AgentData, error)
	GetAgentByID(id uint) (*AgentData, error)
	GetHTTPAgent(agentConfigID uint) (*HTTPAgentData, error)
	GetMCPAgent(agentConfigID uint) (*MCPAgentData, error)
	UpdateAgent(id uint, req UpdateAgentRequest) (*AgentData, error)
	DeleteAgent(id uint) error

	// Utilities
	GenerateJSONSchema(ctx context.Context, exampleJSON, description string) (string, error)
}

// LLMClient interface mínima para o Manager
type LLMClient interface {
	ChatCompletion(ctx context.Context, model, systemPrompt, userMessage string) (string, error)
}

// ==================== Validation ====================

// ValidateJSONSchema valida se uma string contém um JSON Schema válido
func ValidateJSONSchema(schemaStr string) error {
	if schemaStr == "" {
		return nil // Schema vazio é permitido (endpoint sem parâmetros)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(schemaStr), &schema); err != nil {
		return fmt.Errorf("JSON inválido: %w", err)
	}

	// Validar campos obrigatórios básicos de JSON Schema
	schemaType, hasType := schema["type"]
	if !hasType {
		return fmt.Errorf("schema deve ter campo 'type'")
	}

	// Se type não é "object", não precisa validar properties
	if schemaType != "object" {
		return nil
	}

	// Se é object, deve ter properties
	properties, hasProperties := schema["properties"]
	if !hasProperties {
		return fmt.Errorf("schema do tipo 'object' deve ter campo 'properties'")
	}

	// Validar que properties é um map
	propertiesMap, ok := properties.(map[string]interface{})
	if !ok {
		return fmt.Errorf("campo 'properties' deve ser um objeto")
	}

	// Validar cada property
	validTypes := map[string]bool{
		"string":  true,
		"number":  true,
		"integer": true,
		"boolean": true,
		"array":   true,
		"object":  true,
		"null":    true,
	}

	for propName, propValue := range propertiesMap {
		propMap, ok := propValue.(map[string]interface{})
		if !ok {
			return fmt.Errorf("property '%s' deve ser um objeto", propName)
		}

		propType, hasType := propMap["type"]
		if !hasType {
			return fmt.Errorf("property '%s' deve ter campo 'type'", propName)
		}

		propTypeStr, ok := propType.(string)
		if !ok {
			return fmt.Errorf("type da property '%s' deve ser string", propName)
		}

		if !validTypes[propTypeStr] {
			return fmt.Errorf("property '%s' tem type inválido: '%s' (válidos: string, number, integer, boolean, array, object, null)", propName, propTypeStr)
		}
	}

	// Validar campo 'required' se existir
	if required, hasRequired := schema["required"]; hasRequired {
		requiredArray, ok := required.([]interface{})
		if !ok {
			return fmt.Errorf("campo 'required' deve ser um array")
		}

		// Validar que todos os campos required existem em properties
		for _, req := range requiredArray {
			reqStr, ok := req.(string)
			if !ok {
				return fmt.Errorf("items do campo 'required' devem ser strings")
			}

			if _, exists := propertiesMap[reqStr]; !exists {
				return fmt.Errorf("campo required '%s' não existe em properties", reqStr)
			}
		}
	}

	return nil
}
