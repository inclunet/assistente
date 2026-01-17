# Builder Agent - Plano de Implementação

## 1. Visão Geral

Criar um **Agent Builder** inteligente seguindo **exatamente o mesmo padrão** de FAQAgent e MemoryAgent:
- Package `internal/agentmanager` com Provider interface
- `BuilderAgent` usa o Provider
- `App` também usa o Provider
- Lógica compartilhada, zero duplicação

---

## 2. Análise do Padrão Existente

### 2.1. Como FAQAgent funciona

```go
// internal/faq/types.go
type Data struct {
    ID       uint   `json:"id"`
    Question string `json:"question"`
    Answer   string `json:"answer"`
    Tags     string `json:"tags,omitempty"`
}

type Provider interface {
    Create(question, answer, tags string) (*Data, error)
    Get(id uint) (*Data, error)
    GetAll() ([]Data, error)
    Update(id uint, question, answer, tags string) (*Data, error)
    Delete(id uint) error
    Search(query string) ([]Data, error)
}

// internal/faq/store.go
type store struct {
    db *gorm.DB
}

func NewStore(db *gorm.DB) Provider {
    return &store{db: db}
}

// Implementa todas as funções da interface
func (s *store) Create(...) (*Data, error) { /* ... */ }
```

```go
// internal/agents/faq_agent.go
type FAQAgent struct {
    BaseAgent
    provider faq.Provider  // ← Usa o Provider!
}

func NewFAQAgent(provider faq.Provider, llmClient LLMClient, model string) *FAQAgent {
    return &FAQAgent{
        BaseAgent: BaseAgent{
            Name: "faq",
            DisplayName: "FAQ Manager",
            Model: model,
            SystemPrompt: "...",
            LLM: llmClient,
        },
        provider: provider,
    }
}

// Tools chamam o provider
func (a *FAQAgent) toolFAQCreate(args map[string]interface{}) (string, error) {
    data, err := a.provider.Create(question, answer, tags)
    // ...
}
```

```go
// app.go
type App struct {
    // ...
    faqStore faq.Provider  // ← App também tem o Provider!
}

func (a *App) startup(ctx context.Context) {
    // ...
    a.faqStore = faq.NewStore(database.DB())
    
    faqAgent := agents.NewFAQAgent(a.faqStore, a.llmClient, agentModel)
    a.registry.Register(faqAgent)
}

// UI methods chamam o provider diretamente
func (a *App) CreateFAQ(question, answer, tags string) (*faq.Data, error) {
    return a.faqStore.Create(question, answer, tags)
}
```

**PADRÃO CLARO:**
- ✅ Provider interface em `internal/` (sem import cycle)
- ✅ Provider define seus próprios Data types (não expõe database.Models)
- ✅ Agent usa Provider
- ✅ App usa Provider
- ✅ Lógica centralizada, zero duplicação

---

## 3. AgentManager - Mesmo Padrão

### 3.1. Estrutura

```
internal/agentmanager/
├── types.go      # Manager interface + Data types
└── manager.go    # Implementação (como store.go)

internal/agents/
└── builder_agent.go  # Usa Manager (como FAQAgent usa Provider)

app.go
└── agentManager Manager  # App usa Manager (como faqStore)
```

### 3.2. Types (internal/agentmanager/types.go)

```go
package agentmanager

import (
    "context"
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

type HTTPAgentData struct {
    ID             uint                `json:"id"`
    AgentConfigID  uint                `json:"agent_config_id"`
    BaseURL        string              `json:"base_url"`
    AuthType       string              `json:"auth_type"`
    AuthConfig     string              `json:"auth_config"`
    DefaultHeaders string              `json:"default_headers"`
    TimeoutSeconds int                 `json:"timeout_seconds"`
    RetryCount     int                 `json:"retry_count"`
    Endpoints      []HTTPEndpointData  `json:"endpoints,omitempty"`
}

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

type OpenAPIImportResult struct {
    DisplayName    string                 `json:"display_name"`
    Description    string                 `json:"description"`
    BaseURL        string                 `json:"base_url"`
    AuthType       string                 `json:"auth_type"`
    AuthConfig     map[string]string      `json:"auth_config"`
    DefaultHeaders map[string]string      `json:"default_headers"`
    Endpoints      []ImportedEndpoint     `json:"endpoints"`
}

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

type UpdateAgentRequest struct {
    DisplayName  string
    Description  string
    Model        string
    SystemPrompt string
    Config       string
    Enabled      bool
}

// ==================== Manager Interface (Provider Pattern) ====================

type Manager interface {
    // Parsers
    ParseOpenAPISpec(content string) (*OpenAPIImportResult, error)
    ParsePostmanCollection(content string) (*OpenAPIImportResult, error)
    
    // HTTP Agents
    CreateHTTPAgent(req CreateHTTPAgentRequest) (*AgentData, *HTTPAgentData, error)
    CreateHTTPEndpoint(req CreateEndpointRequest) (*HTTPEndpointData, error)
    TestHTTPEndpoint(httpAgentID uint, endpointName string, paramsJSON string) (string, error)
    ImportOpenAPIAgent(content, name, model string) (*AgentData, *HTTPAgentData, error)
    ImportPostmanAgent(content, name, model string) (*AgentData, *HTTPAgentData, error)
    
    // MCP Agents
    CreateMCPAgent(req CreateMCPAgentRequest) (*AgentData, *MCPAgentData, error)
    
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
```

### 3.3. Manager Implementation (internal/agentmanager/manager.go)

```go
package agentmanager

import (
    "context"
    "assistente/internal/database"
    "gorm.io/gorm"
)

type manager struct {
    db  *gorm.DB
    llm LLMClient // Para GenerateJSONSchema
}

// LLMClient interface mínima
type LLMClient interface {
    ChatCompletion(ctx context.Context, model, systemPrompt, userMessage string) (string, error)
}

// New cria um novo Manager (como faq.NewStore, memory.NewStore)
func New(db *gorm.DB, llm LLMClient) Manager {
    return &manager{
        db:  db,
        llm: llm,
    }
}

// ==================== Conversores (database.Models → Data) ====================

func toAgentData(m *database.AgentConfig) *AgentData {
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

// ==================== Parsers (mover de importers.go) ====================

func (m *manager) ParseOpenAPISpec(content string) (*OpenAPIImportResult, error) {
    // Lógica atual de importers.go
    // Retornar OpenAPIImportResult
}

func (m *manager) ParsePostmanCollection(content string) (*OpenAPIImportResult, error) {
    // Lógica atual de importers.go
}

// ==================== HTTP Agents (mover de tools.go) ====================

func (m *manager) CreateHTTPAgent(req CreateHTTPAgentRequest) (*AgentData, *HTTPAgentData, error) {
    // Lógica atual de CreateHTTPAgentFull em tools.go
    
    tx := m.db.Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()
    
    // Criar AgentConfig
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
        return nil, nil, err
    }
    
    // Criar HTTPAgent
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
        return nil, nil, err
    }
    
    if err := tx.Commit().Error; err != nil {
        return nil, nil, err
    }
    
    return toAgentData(agentConfig), toHTTPAgentData(httpAgent), nil
}

func (m *manager) CreateHTTPEndpoint(req CreateEndpointRequest) (*HTTPEndpointData, error) {
    // Lógica atual de tools.go
    
    endpoint := &database.HTTPEndpoint{
        HTTPAgentID:      req.HTTPAgentID,
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
        return nil, err
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

func (m *manager) TestHTTPEndpoint(httpAgentID uint, endpointName string, paramsJSON string) (string, error) {
    // Lógica atual de tools.go
}

func (m *manager) ImportOpenAPIAgent(content, name, model string) (*AgentData, *HTTPAgentData, error) {
    // Parse
    result, err := m.ParseOpenAPISpec(content)
    if err != nil {
        return nil, nil, err
    }
    
    // Create agent
    req := CreateHTTPAgentRequest{
        Name:           name,
        DisplayName:    result.DisplayName,
        Description:    result.Description,
        Model:          model,
        SystemPrompt:   "Agent criado a partir de especificação OpenAPI",
        Enabled:        true,
        BaseURL:        result.BaseURL,
        AuthType:       result.AuthType,
        // ... preencher com result
    }
    
    agentData, httpData, err := m.CreateHTTPAgent(req)
    if err != nil {
        return nil, nil, err
    }
    
    // Create endpoints
    for _, ep := range result.Endpoints {
        epReq := CreateEndpointRequest{
            HTTPAgentID:      httpData.ID,
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
        m.CreateHTTPEndpoint(epReq)
    }
    
    // Reload com endpoints
    httpData, _ = m.GetHTTPAgent(agentData.ID)
    
    return agentData, httpData, nil
}

func (m *manager) ImportPostmanAgent(content, name, model string) (*AgentData, *HTTPAgentData, error) {
    // Similar ao ImportOpenAPIAgent
}

// ==================== MCP Agents ====================

func (m *manager) CreateMCPAgent(req CreateMCPAgentRequest) (*AgentData, *MCPAgentData, error) {
    // Lógica atual de tools.go
}

// ==================== Management ====================

func (m *manager) GetAllAgents() ([]AgentData, error) {
    var models []database.AgentConfig
    if err := m.db.Find(&models).Error; err != nil {
        return nil, err
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
        return nil, err
    }
    return toAgentData(&model), nil
}

func (m *manager) GetHTTPAgent(agentConfigID uint) (*HTTPAgentData, error) {
    var model database.HTTPAgent
    if err := m.db.Preload("Endpoints").Where("agent_config_id = ?", agentConfigID).First(&model).Error; err != nil {
        return nil, err
    }
    return toHTTPAgentData(&model), nil
}

func (m *manager) GetMCPAgent(agentConfigID uint) (*MCPAgentData, error) {
    var model database.MCPAgentDB
    if err := m.db.Where("agent_config_id = ?", agentConfigID).First(&model).Error; err != nil {
        return nil, err
    }
    return toMCPAgentData(&model), nil
}

func (m *manager) UpdateAgent(id uint, req UpdateAgentRequest) (*AgentData, error) {
    var model database.AgentConfig
    if err := m.db.First(&model, id).Error; err != nil {
        return nil, err
    }
    
    model.DisplayName = req.DisplayName
    model.Description = req.Description
    model.Model = req.Model
    model.SystemPrompt = req.SystemPrompt
    model.Config = req.Config
    model.Enabled = req.Enabled
    
    if err := m.db.Save(&model).Error; err != nil {
        return nil, err
    }
    
    return toAgentData(&model), nil
}

func (m *manager) DeleteAgent(id uint) error {
    return m.db.Delete(&database.AgentConfig{}, id).Error
}

// ==================== Utilities ====================

func (m *manager) GenerateJSONSchema(ctx context.Context, exampleJSON, description string) (string, error) {
    prompt := fmt.Sprintf(`Gere um JSON Schema válido baseado neste exemplo:

%s

Descrição: %s

Retorne apenas o JSON Schema, sem explicações.`, exampleJSON, description)
    
    return m.llm.ChatCompletion(ctx, "gpt-4o-mini", "Você é um especialista em JSON Schema.", prompt)
}
```

### 3.4. BuilderAgent (internal/agents/builder_agent.go)

```go
package agents

import (
    "context"
    "encoding/json"
    "assistente/internal/agentmanager"
)

type BuilderAgent struct {
    BaseAgent
    manager agentmanager.Manager  // ← Igual a faq.Provider, memory.Provider
}

func NewBuilderAgent(manager agentmanager.Manager, llmClient LLMClient, model string) *BuilderAgent {
    if model == "" {
        model = "gpt-4o"  // Builder precisa de modelo mais potente
    }
    
    return &BuilderAgent{
        BaseAgent: BaseAgent{
            Name:        "builder",
            DisplayName: "Agent Builder",
            Description: "Cria e gerencia agentes HTTP e MCP. Use para criar agentes a partir de OpenAPI, Postman, ou instruções em linguagem natural.",
            AgentType:   "internal",
            Model:       model,
            SystemPrompt: builderSystemPrompt,
            Enabled:     true,
            LLM:         llmClient,
        },
        manager: manager,
    }
}

// Execute - padrão BaseAgent
func (a *BuilderAgent) Execute(ctx context.Context, task string) (string, error) {
    return a.LLM.ChatWithTools(ctx, a.Model, a.SystemPrompt, task, a.GetTools(), a.ExecuteTool)
}

// GetTools - define as 15 tools
func (a *BuilderAgent) GetTools() []Tool {
    return []Tool{
        // Análise
        {Name: "analyze_openapi", /* ... */},
        {Name: "analyze_postman", /* ... */},
        {Name: "extract_requirements", /* ... */},
        {Name: "plan_agent", /* ... */},
        
        // HTTP
        {Name: "create_http_agent", /* ... */},
        {Name: "add_http_endpoint", /* ... */},
        {Name: "test_http_endpoint", /* ... */},
        {Name: "import_openapi_agent", /* ... */},
        {Name: "import_postman_agent", /* ... */},
        
        // MCP
        {Name: "create_mcp_agent", /* ... */},
        
        // Management
        {Name: "list_agents", /* ... */},
        {Name: "get_agent_details", /* ... */},
        {Name: "update_agent", /* ... */},
        {Name: "delete_agent", /* ... */},
        {Name: "generate_json_schema", /* ... */},
    }
}

// ExecuteTool - switch case
func (a *BuilderAgent) ExecuteTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
    switch toolName {
    case "analyze_openapi":
        return a.toolAnalyzeOpenAPI(args)
    case "create_http_agent":
        return a.toolCreateHTTPAgent(args)
    case "import_openapi_agent":
        return a.toolImportOpenAPIAgent(args)
    // ... etc
    default:
        return "", fmt.Errorf("unknown tool: %s", toolName)
    }
}

// ==================== Tool Implementations ====================

func (a *BuilderAgent) toolAnalyzeOpenAPI(args map[string]interface{}) (string, error) {
    content := args["content"].(string)
    
    result, err := a.manager.ParseOpenAPISpec(content)
    if err != nil {
        return "", err
    }
    
    jsonResult, _ := json.MarshalIndent(result, "", "  ")
    return string(jsonResult), nil
}

func (a *BuilderAgent) toolCreateHTTPAgent(args map[string]interface{}) (string, error) {
    req := agentmanager.CreateHTTPAgentRequest{
        Name:           args["name"].(string),
        DisplayName:    args["display_name"].(string),
        Description:    args["description"].(string),
        Model:          getStringOrDefault(args, "model", "gpt-4o-mini"),
        SystemPrompt:   getStringOrDefault(args, "system_prompt", ""),
        Enabled:        getBoolOrDefault(args, "enabled", true),
        BaseURL:        args["base_url"].(string),
        AuthType:       getStringOrDefault(args, "auth_type", "none"),
        AuthConfig:     getStringOrDefault(args, "auth_config", ""),
        DefaultHeaders: getStringOrDefault(args, "default_headers", ""),
        TimeoutSeconds: getIntOrDefault(args, "timeout_seconds", 30),
        RetryCount:     getIntOrDefault(args, "retry_count", 3),
    }
    
    agentData, httpData, err := a.manager.CreateHTTPAgent(req)
    if err != nil {
        return "", err
    }
    
    jsonResult, _ := json.MarshalIndent(map[string]interface{}{
        "agent": agentData,
        "http_config": httpData,
    }, "", "  ")
    
    return string(jsonResult), nil
}

func (a *BuilderAgent) toolImportOpenAPIAgent(args map[string]interface{}) (string, error) {
    content := args["content"].(string)
    name := args["name"].(string)
    model := getStringOrDefault(args, "model", "gpt-4o-mini")
    
    agentData, httpData, err := a.manager.ImportOpenAPIAgent(content, name, model)
    if err != nil {
        return "", err
    }
    
    jsonResult, _ := json.MarshalIndent(map[string]interface{}{
        "agent": agentData,
        "http_config": httpData,
    }, "", "  ")
    
    return string(jsonResult), nil
}

// ... outras 12 tools similarmente
```

### 3.5. App Integration (app.go)

```go
type App struct {
    // ...
    faqStore     faq.Provider
    memoryStore  memory.Provider
    agentManager agentmanager.Manager  // ← Nova linha!
}

func (a *App) startup(ctx context.Context) {
    // ...
    
    // Criar manager
    a.agentManager = agentmanager.New(database.DB(), a.llmClient)
    
    // Criar builder agent
    builderAgent := agents.NewBuilderAgent(a.agentManager, a.llmClient, agentModel)
    a.applyAgentConfig(builderAgent)
    a.registry.Register(builderAgent)
}
```

### 3.6. Tools.go - Delega para Manager

```go
// Manter assinaturas Wails, mas delegar para manager

func (a *App) CreateHTTPAgentFull(name, displayName, description, model, systemPrompt string, 
    enabled bool, baseURL, authType, authConfig, defaultHeaders string, 
    timeoutSeconds, retryCount int) (map[string]interface{}, error) {
    
    req := agentmanager.CreateHTTPAgentRequest{
        Name:           name,
        DisplayName:    displayName,
        Description:    description,
        Model:          model,
        SystemPrompt:   systemPrompt,
        Enabled:        enabled,
        BaseURL:        baseURL,
        AuthType:       authType,
        AuthConfig:     authConfig,
        DefaultHeaders: defaultHeaders,
        TimeoutSeconds: timeoutSeconds,
        RetryCount:     retryCount,
    }
    
    agentData, httpData, err := a.agentManager.CreateHTTPAgent(req)
    if err != nil {
        return nil, err
    }
    
    return map[string]interface{}{
        "agent_config": agentData,
        "http_agent":   httpData,
    }, nil
}

func (a *App) ParseOpenAPISpec(content string) (*agentmanager.OpenAPIImportResult, error) {
    return a.agentManager.ParseOpenAPISpec(content)
}

// ... etc
```

---

## 4. Plano de Implementação

### Fase 1: Criar Package AgentManager
- [ ] Criar `internal/agentmanager/types.go`
  - Data types: AgentData, HTTPAgentData, MCPAgentData, HTTPEndpointData
  - OpenAPIImportResult, ImportedEndpoint
  - Request types: CreateHTTPAgentRequest, etc
  - Manager interface com todos os métodos
- [ ] Criar `internal/agentmanager/manager.go`
  - Struct manager privada
  - New() retorna Manager interface
  - Conversores toAgentData(), toHTTPAgentData(), etc

### Fase 2: Migrar Lógica Existente
- [ ] Mover ParseOpenAPISpec de `importers.go` para `manager.go`
- [ ] Mover ParsePostmanCollection de `importers.go` para `manager.go`
- [ ] Mover CreateHTTPAgentFull de `tools.go` para `manager.CreateHTTPAgent()`
- [ ] Mover CreateHTTPEndpoint de `tools.go` para `manager.CreateHTTPEndpoint()`
- [ ] Mover TestHTTPEndpoint de `tools.go` para `manager.TestHTTPEndpoint()`
- [ ] Mover CreateMCPAgentFull de `tools.go` para `manager.CreateMCPAgent()`
- [ ] Implementar Get/Update/Delete methods
- [ ] Implementar ImportOpenAPIAgent, ImportPostmanAgent
- [ ] Implementar GenerateJSONSchema

### Fase 3: Atualizar App
- [ ] `app.go`: Adicionar `agentManager agentmanager.Manager`
- [ ] `app.go`: Inicializar `a.agentManager = agentmanager.New(db, llm)`
- [ ] `tools.go`: Refatorar para delegar para `a.agentManager`
- [ ] `importers.go`: Refatorar ou remover (lógica moveu para manager)

### Fase 4: BuilderAgent
- [ ] Criar `internal/agents/builder_agent.go`
- [ ] Struct BuilderAgent com `manager agentmanager.Manager`
- [ ] NewBuilderAgent(manager, llmClient, model)
- [ ] Definir system prompt
- [ ] GetTools() com 15 tools
- [ ] ExecuteTool() com switch case

### Fase 5: Implementar Tools de Análise
- [ ] analyze_openapi (chama manager.ParseOpenAPISpec)
- [ ] analyze_postman (chama manager.ParsePostmanCollection)
- [ ] extract_requirements (usa LLM para analisar texto)
- [ ] plan_agent (usa LLM para gerar plano)

### Fase 6: Implementar Tools HTTP
- [ ] create_http_agent (chama manager.CreateHTTPAgent)
- [ ] add_http_endpoint (chama manager.CreateHTTPEndpoint)
- [ ] test_http_endpoint (chama manager.TestHTTPEndpoint)
- [ ] import_openapi_agent (chama manager.ImportOpenAPIAgent)
- [ ] import_postman_agent (chama manager.ImportPostmanAgent)

### Fase 7: Implementar Tools MCP
- [ ] create_mcp_agent (chama manager.CreateMCPAgent)

### Fase 8: Implementar Tools de Gerenciamento
- [ ] list_agents (chama manager.GetAllAgents)
- [ ] get_agent_details (chama manager.GetAgentByID + GetHTTPAgent/GetMCPAgent)
- [ ] update_agent (chama manager.UpdateAgent)
- [ ] delete_agent (chama manager.DeleteAgent)
- [ ] generate_json_schema (chama manager.GenerateJSONSchema)

### Fase 9: Integração
- [ ] Registrar BuilderAgent em `app.go`
- [ ] Testar delegação do orquestrador
- [ ] Validar que UI continua funcionando

### Fase 10: Testes
- [ ] Criar agente via OpenAPI
- [ ] Criar agente via Postman
- [ ] Criar agente via linguagem natural
- [ ] Criar agente MCP stdio
- [ ] Criar agente MCP http
- [ ] Gerenciar agentes (list, update, delete)

---

## 5. Decisões de Design

### Por que Provider Pattern?

**Já existe no projeto:**
- `internal/faq` usa Provider pattern
- `internal/memory` usa Provider pattern
- `internal/filemanager` usa Provider pattern

**Consistência > criatividade**

### Por que Data Types próprios?

**FAQ define faq.Data, não expõe database.FAQ**
**Memory define memory.Data, não expõe database.Memory**

**Benefícios:**
- Desacoplamento: database.Models podem mudar sem afetar API
- Clareza: Tipos de contrato vs tipos de persistência
- Sem GORM tags vazando para API

### Trade-offs

**Prós:**
- ✅ Consistente com padrão existente
- ✅ Zero import cycle
- ✅ Type safety completo
- ✅ DRY (lógica compartilhada)
- ✅ Testável (mock Manager interface)

**Contras:**
- ⚠️ Conversores database.Models → Data (mas centralizados)
- ⚠️ Mais structs (mas clareza justifica)

**ROI:** Arquitetura limpa e consistente vale o investimento inicial
