package agentmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/database"
	"assistente/internal/importers"

	"gorm.io/gorm"
)

// manager implementa a interface Manager (privado, como store em faq/memory)
type manager struct {
	db *gorm.DB
}

// New cria um novo Manager (como faq.NewStore, memory.NewStore)
func New(db *gorm.DB) Manager {
	return &manager{
		db: db,
	}
}

// ==================== Conversores (database.Models → Data) ====================

func toAgentData(m *database.AgentConfig) *AgentData {
	if m == nil {
		return nil
	}
	return &AgentData{
		ID:           m.ID,
		Name:         m.Name,
		DisplayName:  m.DisplayName,
		Description:  m.Description,
		AgentType:    m.AgentType,
		Model:        m.Model,
		SystemPrompt: m.SystemPrompt,
		Config:       m.Config,
		Enabled:      m.Enabled,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func toHTTPAgentData(m *database.HTTPAgent) *HTTPAgentData {
	if m == nil {
		return nil
	}

	endpoints := make([]HTTPEndpointData, len(m.Endpoints))
	for i, ep := range m.Endpoints {
		endpoints[i] = HTTPEndpointData{
			ID:               ep.ID,
			HTTPAgentID:      ep.HTTPAgentID,
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       ep.Parameters,
			ResponseTemplate: ep.ResponseTemplate,
		}
	}

	return &HTTPAgentData{
		ID:             m.ID,
		AgentConfigID:  m.AgentConfigID,
		BaseURL:        m.BaseURL,
		AuthType:       m.AuthType,
		AuthConfig:     m.AuthConfig,
		DefaultHeaders: m.DefaultHeaders,
		TimeoutSeconds: m.TimeoutSeconds,
		RetryCount:     m.RetryCount,
		Endpoints:      endpoints,
	}
}

func toMCPAgentData(m *database.MCPAgentDB) *MCPAgentData {
	if m == nil {
		return nil
	}
	return &MCPAgentData{
		ID:            m.ID,
		AgentConfigID: m.AgentConfigID,
		TransportType: m.TransportType,
		ServerCommand: m.ServerCommand,
		ServerArgs:    m.ServerArgs,
		ServerEnv:     m.ServerEnv,
		WorkingDir:    m.WorkingDir,
		ServerURL:     m.ServerURL,
		AuthType:      m.AuthType,
		AuthValue:     m.AuthValue,
		HTTPHeaders:   m.HTTPHeaders,
		ExecutionMode: m.ExecutionMode,
		AutoConnect:   m.AutoConnect,
	}
}

// ==================== Parsers ====================

func (m *manager) ParseOpenAPISpec(content string) (*OpenAPIImportResult, error) {
	parser := importers.NewOpenAPIParser()
	result, err := parser.Parse(content)
	if err != nil {
		return nil, err
	}

	// Converte para agentmanager types
	endpoints := make([]ImportedEndpoint, 0, len(result.Endpoints))
	for _, ep := range result.Endpoints {
		endpoints = append(endpoints, ImportedEndpoint{
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       ep.Parameters,
			ResponseTemplate: ep.ResponseTemplate,
		})
	}

	return &OpenAPIImportResult{
		DisplayName:    result.DisplayName,
		Description:    result.Description,
		BaseURL:        result.BaseURL,
		AuthType:       result.AuthType,
		AuthConfig:     result.AuthConfig,
		DefaultHeaders: result.DefaultHeaders,
		Endpoints:      endpoints,
	}, nil
}

func (m *manager) ParsePostmanCollection(content string) (*OpenAPIImportResult, error) {
	parser := importers.NewPostmanParser()
	result, err := parser.Parse(content)
	if err != nil {
		return nil, err
	}

	// Converte para agentmanager types
	endpoints := make([]ImportedEndpoint, 0, len(result.Endpoints))
	for _, ep := range result.Endpoints {
		endpoints = append(endpoints, ImportedEndpoint{
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       ep.Parameters,
			ResponseTemplate: ep.ResponseTemplate,
		})
	}

	return &OpenAPIImportResult{
		DisplayName:    result.DisplayName,
		Description:    result.Description,
		BaseURL:        result.BaseURL,
		AuthType:       result.AuthType,
		AuthConfig:     result.AuthConfig,
		DefaultHeaders: result.DefaultHeaders,
		Endpoints:      endpoints,
	}, nil
}

// ==================== HTTP Agents ====================

func (m *manager) CreateHTTPAgent(req CreateHTTPAgentRequest) (*AgentData, *HTTPAgentData, error) {
	tx := m.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. Criar AgentConfig
	agentConfig := &database.AgentConfig{
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		AgentType:    "http",
		Model:        req.Model,
		SystemPrompt: req.SystemPrompt,
		Enabled:      req.Enabled,
	}

	if err := tx.Create(agentConfig).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("erro ao criar AgentConfig: %w", err)
	}

	// 2. Criar HTTPAgent
	httpAgent := &database.HTTPAgent{
		AgentConfigID:  agentConfig.ID,
		BaseURL:        req.BaseURL,
		AuthType:       req.AuthType,
		AuthConfig:     req.AuthConfig,
		DefaultHeaders: req.DefaultHeaders,
		TimeoutSeconds: req.TimeoutSeconds,
		RetryCount:     req.RetryCount,
	}

	if err := tx.Create(httpAgent).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("erro ao criar HTTPAgent: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, nil, fmt.Errorf("erro ao commit: %w", err)
	}

	return toAgentData(agentConfig), toHTTPAgentData(httpAgent), nil
}

func (m *manager) CreateHTTPEndpoint(httpAgentID uint, req CreateEndpointRequest) (*HTTPEndpointData, error) {
	endpoint := &database.HTTPEndpoint{
		HTTPAgentID:      httpAgentID,
		Name:             req.Name,
		Description:      req.Description,
		Method:           req.Method,
		PathTemplate:     req.PathTemplate,
		QueryTemplate:    req.QueryTemplate,
		HeadersJSON:      req.HeadersJSON,
		BodyTemplate:     req.BodyTemplate,
		Parameters:       req.Parameters,
		ResponseTemplate: req.ResponseTemplate,
	}

	if err := m.db.Create(endpoint).Error; err != nil {
		return nil, fmt.Errorf("erro ao criar endpoint: %w", err)
	}

	return &HTTPEndpointData{
		ID:               endpoint.ID,
		HTTPAgentID:      endpoint.HTTPAgentID,
		Name:             endpoint.Name,
		Description:      endpoint.Description,
		Method:           endpoint.Method,
		PathTemplate:     endpoint.PathTemplate,
		QueryTemplate:    endpoint.QueryTemplate,
		HeadersJSON:      endpoint.HeadersJSON,
		BodyTemplate:     endpoint.BodyTemplate,
		Parameters:       endpoint.Parameters,
		ResponseTemplate: endpoint.ResponseTemplate,
	}, nil
}

func (m *manager) GetHTTPEndpoint(id uint) (*HTTPEndpointData, error) {
	var endpoint database.HTTPEndpoint
	if err := m.db.First(&endpoint, id).Error; err != nil {
		return nil, fmt.Errorf("endpoint não encontrado: %w", err)
	}

	return &HTTPEndpointData{
		ID:               endpoint.ID,
		HTTPAgentID:      endpoint.HTTPAgentID,
		Name:             endpoint.Name,
		Description:      endpoint.Description,
		Method:           endpoint.Method,
		PathTemplate:     endpoint.PathTemplate,
		QueryTemplate:    endpoint.QueryTemplate,
		HeadersJSON:      endpoint.HeadersJSON,
		BodyTemplate:     endpoint.BodyTemplate,
		Parameters:       endpoint.Parameters,
		ResponseTemplate: endpoint.ResponseTemplate,
	}, nil
}

func (m *manager) UpdateHTTPEndpoint(id uint, req CreateEndpointRequest) (*HTTPEndpointData, error) {
	var endpoint database.HTTPEndpoint
	if err := m.db.First(&endpoint, id).Error; err != nil {
		return nil, fmt.Errorf("endpoint não encontrado: %w", err)
	}

	endpoint.Name = req.Name
	endpoint.Description = req.Description
	endpoint.Method = req.Method
	endpoint.PathTemplate = req.PathTemplate
	endpoint.QueryTemplate = req.QueryTemplate
	endpoint.HeadersJSON = req.HeadersJSON
	endpoint.BodyTemplate = req.BodyTemplate
	endpoint.Parameters = req.Parameters
	endpoint.ResponseTemplate = req.ResponseTemplate

	if err := m.db.Save(&endpoint).Error; err != nil {
		return nil, fmt.Errorf("erro ao atualizar endpoint: %w", err)
	}

	return &HTTPEndpointData{
		ID:               endpoint.ID,
		HTTPAgentID:      endpoint.HTTPAgentID,
		Name:             endpoint.Name,
		Description:      endpoint.Description,
		Method:           endpoint.Method,
		PathTemplate:     endpoint.PathTemplate,
		QueryTemplate:    endpoint.QueryTemplate,
		HeadersJSON:      endpoint.HeadersJSON,
		BodyTemplate:     endpoint.BodyTemplate,
		Parameters:       endpoint.Parameters,
		ResponseTemplate: endpoint.ResponseTemplate,
	}, nil
}

func (m *manager) DeleteHTTPEndpoint(id uint) error {
	if err := m.db.Delete(&database.HTTPEndpoint{}, id).Error; err != nil {
		return fmt.Errorf("erro ao deletar endpoint: %w", err)
	}
	return nil
}

func (m *manager) GetHTTPEndpointsByAgentID(httpAgentID uint) ([]HTTPEndpointData, error) {
	var endpoints []database.HTTPEndpoint
	if err := m.db.Where("http_agent_id = ?", httpAgentID).Find(&endpoints).Error; err != nil {
		return nil, fmt.Errorf("erro ao buscar endpoints: %w", err)
	}

	data := make([]HTTPEndpointData, len(endpoints))
	for i, ep := range endpoints {
		data[i] = HTTPEndpointData{
			ID:               ep.ID,
			HTTPAgentID:      ep.HTTPAgentID,
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       ep.Parameters,
			ResponseTemplate: ep.ResponseTemplate,
		}
	}
	return data, nil
}

func (m *manager) TestHTTPEndpoint(httpAgentID uint, endpointName string, paramsJSON string) (string, error) {
	// Busca o HTTPAgent
	var httpAgent database.HTTPAgent
	if err := m.db.Preload("Endpoints").Where("id = ?", httpAgentID).First(&httpAgent).Error; err != nil {
		return "", fmt.Errorf("HTTPAgent não encontrado: %w", err)
	}

	// Parseia parâmetros
	var params map[string]interface{}
	if paramsJSON != "" {
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			return "", fmt.Errorf("erro ao parsear parâmetros: %w", err)
		}
	} else {
		params = make(map[string]interface{})
	}

	// Encontra o endpoint
	var endpoint *database.HTTPEndpoint
	for i := range httpAgent.Endpoints {
		if httpAgent.Endpoints[i].Name == endpointName {
			endpoint = &httpAgent.Endpoints[i]
			break
		}
	}

	if endpoint == nil {
		return "", fmt.Errorf("endpoint não encontrado: %s", endpointName)
	}

	// Busca o AgentConfig para pegar o nome
	var agentConfig database.AgentConfig
	if err := m.db.Where("id = ?", httpAgent.AgentConfigID).First(&agentConfig).Error; err != nil {
		return "", fmt.Errorf("AgentConfig não encontrado: %w", err)
	}

	agentName := agentConfig.Name
	displayName := agentConfig.DisplayName

	// Converte headers para map
	var defaultHeaders map[string]string
	if httpAgent.DefaultHeaders != "" {
		json.Unmarshal([]byte(httpAgent.DefaultHeaders), &defaultHeaders)
	}

	var authConfig map[string]string
	if httpAgent.AuthConfig != "" {
		json.Unmarshal([]byte(httpAgent.AuthConfig), &authConfig)
	}

	// Cria o executor
	executor := NewHTTPExecutor(HTTPExecutorConfig{
		TimeoutSeconds: httpAgent.TimeoutSeconds,
		RetryCount:     httpAgent.RetryCount,
	})

	// Monta a request
	req := HTTPRequest{
		Method:         endpoint.Method,
		BaseURL:        httpAgent.BaseURL,
		PathTemplate:   endpoint.PathTemplate,
		QueryTemplate:  endpoint.QueryTemplate,
		HeadersJSON:    endpoint.HeadersJSON,
		BodyTemplate:   endpoint.BodyTemplate,
		DefaultHeaders: defaultHeaders,
		AuthType:       httpAgent.AuthType,
		AuthConfig:     authConfig,
		EnvVars:        getEnvVars(),
	}

	// Executa
	resp, err := executor.Execute(context.Background(), req, params, agentName, displayName)
	if err != nil {
		return "", err
	}

	if resp.Error != "" {
		return "", fmt.Errorf("%s", resp.Error)
	}

	// Formata resposta
	if endpoint.ResponseTemplate != "" {
		return executor.FormatResponse(resp, endpoint.ResponseTemplate, params, agentName, displayName)
	}

	// Retorna JSON formatado
	if resp.JSON != nil {
		jsonBytes, _ := json.MarshalIndent(resp.JSON, "", "  ")
		return string(jsonBytes), nil
	}

	return resp.Body, nil
}

// getEnvVars retorna as variáveis de ambiente
func getEnvVars() map[string]string {
	envVars := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}
	return envVars
}

func (m *manager) ImportOpenAPIAgent(content, name, model string) (*AgentData, *HTTPAgentData, error) {
	// 1. Parse OpenAPI
	result, err := m.ParseOpenAPISpec(content)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao parse OpenAPI: %w", err)
	}

	// 2. Criar agente
	req := CreateHTTPAgentRequest{
		Name:           name,
		DisplayName:    result.DisplayName,
		Description:    result.Description,
		Model:          model,
		SystemPrompt:   fmt.Sprintf("Agente criado a partir de especificação OpenAPI: %s", result.Description),
		Enabled:        true,
		BaseURL:        result.BaseURL,
		AuthType:       result.AuthType,
		AuthConfig:     "", // TODO: converter result.AuthConfig para string
		DefaultHeaders: "", // TODO: converter result.DefaultHeaders para string
		TimeoutSeconds: 30,
		RetryCount:     3,
	}

	agentData, httpData, err := m.CreateHTTPAgent(req)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao criar agente: %w", err)
	}

	// 3. Criar endpoints
	for _, ep := range result.Endpoints {
		// Converte Parameters para JSON string
		paramsJSON := ""
		if len(ep.Parameters) > 0 {
			if b, err := json.Marshal(ep.Parameters); err == nil {
				paramsJSON = string(b)
			}
		}

		epReq := CreateEndpointRequest{
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       paramsJSON,
			ResponseTemplate: ep.ResponseTemplate,
		}
		if _, err := m.CreateHTTPEndpoint(httpData.ID, epReq); err != nil {
			fmt.Printf("⚠️ Erro ao criar endpoint %s: %v\n", ep.Name, err)
		}
	}

	// 4. Recarregar com endpoints
	httpData, err = m.GetHTTPAgent(agentData.ID)
	if err != nil {
		fmt.Printf("⚠️ Erro ao recarregar agent: %v\n", err)
	}

	return agentData, httpData, nil
}

func (m *manager) ImportPostmanAgent(content, name, model string) (*AgentData, *HTTPAgentData, error) {
	// 1. Parse Postman
	result, err := m.ParsePostmanCollection(content)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao parse Postman: %w", err)
	}

	// 2. Criar agente
	req := CreateHTTPAgentRequest{
		Name:           name,
		DisplayName:    result.DisplayName,
		Description:    result.Description,
		Model:          model,
		SystemPrompt:   fmt.Sprintf("Agente criado a partir de coleção Postman: %s", result.Description),
		Enabled:        true,
		BaseURL:        result.BaseURL,
		AuthType:       result.AuthType,
		AuthConfig:     "", // TODO: converter result.AuthConfig para string
		DefaultHeaders: "", // TODO: converter result.DefaultHeaders para string
		TimeoutSeconds: 30,
		RetryCount:     3,
	}

	agentData, httpData, err := m.CreateHTTPAgent(req)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao criar agente: %w", err)
	}

	// 3. Criar endpoints (Postman)
	for _, ep := range result.Endpoints {
		// Converte Parameters para JSON string
		paramsJSON := ""
		if len(ep.Parameters) > 0 {
			if b, err := json.Marshal(ep.Parameters); err == nil {
				paramsJSON = string(b)
			}
		}

		epReq := CreateEndpointRequest{
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       paramsJSON,
			ResponseTemplate: ep.ResponseTemplate,
		}
		if _, err := m.CreateHTTPEndpoint(httpData.ID, epReq); err != nil {
			fmt.Printf("⚠️ Erro ao criar endpoint %s: %v\n", ep.Name, err)
		}
	}

	// 4. Recarregar com endpoints
	httpData, err = m.GetHTTPAgent(agentData.ID)
	if err != nil {
		fmt.Printf("⚠️ Erro ao recarregar agent: %v\n", err)
	}

	return agentData, httpData, nil
}

// ==================== MCP Agents ====================

func (m *manager) CreateMCPAgent(req CreateMCPAgentRequest) (*AgentData, *MCPAgentData, error) {
	tx := m.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. Criar AgentConfig
	agentConfig := &database.AgentConfig{
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		AgentType:    "mcp",
		Model:        req.Model,
		SystemPrompt: req.SystemPrompt,
		Enabled:      req.Enabled,
	}

	if err := tx.Create(agentConfig).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("erro ao criar AgentConfig: %w", err)
	}

	// 2. Criar MCPAgent
	mcpAgent := &database.MCPAgentDB{
		AgentConfigID: agentConfig.ID,
		TransportType: req.TransportType,
		ServerCommand: req.ServerCommand,
		ServerArgs:    req.ServerArgs,
		ServerEnv:     req.ServerEnv,
		WorkingDir:    req.WorkingDir,
		ServerURL:     req.ServerURL,
		AuthType:      req.AuthType,
		AuthValue:     req.AuthValue,
		HTTPHeaders:   req.HTTPHeaders,
		ExecutionMode: req.ExecutionMode,
		AutoConnect:   req.AutoConnect,
	}

	if err := tx.Create(mcpAgent).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("erro ao criar MCPAgent: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, nil, fmt.Errorf("erro ao commit: %w", err)
	}

	return toAgentData(agentConfig), toMCPAgentData(mcpAgent), nil
}

func (m *manager) UpdateMCPAgent(id uint, req CreateMCPAgentRequest) (*MCPAgentData, error) {
	var mcpAgent database.MCPAgentDB
	if err := m.db.First(&mcpAgent, id).Error; err != nil {
		return nil, fmt.Errorf("MCPAgent não encontrado: %w", err)
	}

	mcpAgent.TransportType = req.TransportType
	mcpAgent.ServerCommand = req.ServerCommand
	mcpAgent.ServerArgs = req.ServerArgs
	mcpAgent.ServerEnv = req.ServerEnv
	mcpAgent.WorkingDir = req.WorkingDir
	mcpAgent.ServerURL = req.ServerURL
	mcpAgent.AuthType = req.AuthType
	mcpAgent.AuthValue = req.AuthValue
	mcpAgent.HTTPHeaders = req.HTTPHeaders
	mcpAgent.ExecutionMode = req.ExecutionMode
	mcpAgent.AutoConnect = req.AutoConnect

	if err := m.db.Save(&mcpAgent).Error; err != nil {
		return nil, fmt.Errorf("erro ao atualizar MCPAgent: %w", err)
	}

	return toMCPAgentData(&mcpAgent), nil
}

func (m *manager) DeleteMCPAgent(id uint) error {
	if err := m.db.Delete(&database.MCPAgentDB{}, id).Error; err != nil {
		return fmt.Errorf("erro ao deletar MCPAgent: %w", err)
	}
	return nil
}

func (m *manager) GetAllMCPAgents() ([]MCPAgentData, error) {
	var models []database.MCPAgentDB
	if err := m.db.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("erro ao buscar MCPAgents: %w", err)
	}

	data := make([]MCPAgentData, len(models))
	for i, model := range models {
		data[i] = *toMCPAgentData(&model)
	}
	return data, nil
}

// ==================== Management ====================

func (m *manager) GetAllAgents() ([]AgentData, error) {
	var models []database.AgentConfig
	if err := m.db.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("erro ao buscar agentes: %w", err)
	}

	data := make([]AgentData, len(models))
	for i, model := range models {
		data[i] = *toAgentData(&model)
	}
	return data, nil
}

func (m *manager) GetAgentByID(id uint) (*AgentData, error) {
	var model database.AgentConfig
	if err := m.db.First(&model, id).Error; err != nil {
		return nil, fmt.Errorf("agente não encontrado: %w", err)
	}
	return toAgentData(&model), nil
}

func (m *manager) GetHTTPAgent(agentConfigID uint) (*HTTPAgentData, error) {
	var model database.HTTPAgent
	if err := m.db.Preload("Endpoints").Where("agent_config_id = ?", agentConfigID).First(&model).Error; err != nil {
		return nil, fmt.Errorf("HTTPAgent não encontrado: %w", err)
	}
	return toHTTPAgentData(&model), nil
}

func (m *manager) GetMCPAgent(agentConfigID uint) (*MCPAgentData, error) {
	var model database.MCPAgentDB
	if err := m.db.Where("agent_config_id = ?", agentConfigID).First(&model).Error; err != nil {
		return nil, fmt.Errorf("MCPAgent não encontrado: %w", err)
	}
	return toMCPAgentData(&model), nil
}

func (m *manager) UpdateAgent(id uint, req UpdateAgentRequest) (*AgentData, error) {
	var model database.AgentConfig
	if err := m.db.First(&model, id).Error; err != nil {
		return nil, fmt.Errorf("agente não encontrado: %w", err)
	}

	model.DisplayName = req.DisplayName
	model.Description = req.Description
	model.Model = req.Model
	model.SystemPrompt = req.SystemPrompt
	model.Config = req.Config
	model.Enabled = req.Enabled

	if err := m.db.Save(&model).Error; err != nil {
		return nil, fmt.Errorf("erro ao atualizar agente: %w", err)
	}

	// Se for HTTP Agent, atualiza também
	if model.AgentType == "http" {
		var httpAgent database.HTTPAgent
		if err := m.db.Where("agent_config_id = ?", id).First(&httpAgent).Error; err == nil {
			httpAgent.BaseURL = req.BaseURL
			httpAgent.AuthType = req.AuthType
			httpAgent.AuthConfig = req.AuthConfig
			httpAgent.DefaultHeaders = req.DefaultHeaders
			httpAgent.TimeoutSeconds = req.TimeoutSeconds
			httpAgent.RetryCount = req.RetryCount
			m.db.Save(&httpAgent)
		}
	}

	return toAgentData(&model), nil
}

func (m *manager) DeleteAgent(id uint) error {
	if err := m.db.Delete(&database.AgentConfig{}, id).Error; err != nil {
		return fmt.Errorf("erro ao deletar agente: %w", err)
	}
	return nil
}

// ==================== Utilities ====================

func (m *manager) GenerateJSONSchema(ctx context.Context, exampleJSON, description string) (string, error) {
	// TODO: Implementar geração de JSON Schema com LLM quando necessário
	return "", fmt.Errorf("GenerateJSONSchema não implementado ainda")
}
