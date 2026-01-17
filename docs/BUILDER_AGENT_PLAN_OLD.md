# Builder Agent - Plano de Implementação

## 1. Visão Geral

### Objetivo
Criar um **Agent Builder** inteligente que permita criar e gerenciar outros agentes (HTTP e MCP) através de linguagem natural, seguindo exatamente o mesmo padrão arquitetural dos agentes existentes (FileAgent, MemoryAgent, FAQAgent).

### Princípios de Design
1. **Zero wrappers desnecessários** - Sem camadas de adaptação que não agregam valor
2. **Type safety onde faz sentido** - Usar tipos concretos exceto onde há import cycle
3. **Código idiomático Go** - Seguir convenções da linguagem
4. **Consistência com agentes existentes** - Mesmo padrão de FAQAgent/MemoryAgent/FileAgent

---

## 2. Análise da Arquitetura Existente

### 2.1. Padrão de Agentes Internos

**Estrutura comum (FileAgent, MemoryAgent, FAQAgent):**

```go
type XAgent struct {
    BaseAgent            // Name, DisplayName, Model, SystemPrompt, LLM, etc.
    provider  Provider   // Acesso a dados/recursos (faq.Provider, memory.Provider, etc.)
}

// Constructor
func NewXAgent(provider Provider, llmClient LLMClient, model string) *XAgent {
    return &XAgent{
        BaseAgent: BaseAgent{
            Name: "x",
            DisplayName: "X Manager",
            Model: model,
            SystemPrompt: "...",  // Prompt especializado
            LLM: llmClient,
        },
        provider: provider,
    }
}

// Execute: recebe tarefa em linguagem natural
func (a *XAgent) Execute(ctx context.Context, task string) (string, error) {
    // Chama LLM com suas tools
    result, err := a.LLM.ChatWithTools(ctx, a.Model, a.SystemPrompt, task, a.GetTools(), a.ExecuteTool)
    return result, err
}

// GetTools: define as tools que o agent pode usar
func (a *XAgent) GetTools() []Tool { /* ... */ }

// ExecuteTool: executa as tools
func (a *XAgent) ExecuteTool(toolCall ToolCall) (string, error) {
    switch toolCall.Function.Name {
    case "tool1": return a.executeTool1(args)
    case "tool2": return a.executeTool2(args)
    }
}
```

### 2.2. Funções do App Disponíveis

**Parsers:**
- `ParseOpenAPISpec(content string) (*OpenAPIImportResult, error)` - em `importers.go`
- `ParsePostmanCollection(content string) (*OpenAPIImportResult, error)` - em `importers.go`

**HTTP Agents (tools.go):**
- `CreateHTTPAgentFull(...)` - Cria AgentConfig + HTTPAgent + retorna struct completo
- `CreateHTTPEndpoint(...)` - Adiciona endpoint a HTTPAgent existente
- `TestHTTPEndpoint(...)` - Testa um endpoint com parâmetros

**MCP Agents (tools.go):**
- `CreateMCPAgentFull(...)` - Cria AgentConfig + MCPAgent completo

**Database (db.go):**
- `GetAllAgentConfigs() ([]AgentConfig, error)`
- `GetAgentConfigByID(id uint) (*AgentConfig, error)`
- `GetHTTPAgentByConfigID(id uint) (*HTTPAgent, error)`
- `UpdateAgentConfig(...)`
- `DeleteAgentConfig(id uint) error`

**LLM (llm.go):**
- `ChatCompletion(ctx, model, systemPrompt, userMessage) (string, error)` - Para análises

### 2.3. Import Cycle Problem

**O problema original:**
- `main` (App) importa `internal/agents`
- Se `BuilderAgent` importar tipos de `main`, temos cycle

**Solução definitiva: Provider Pattern (mesmo padrão de FAQ e Memory)**

Observação importante: **já existe lógica de gerenciamento de agentes em `tools.go`** E **já existe um padrão estabelecido nos outros agentes!**

**Padrão existente (FAQ e Memory):**

```
internal/faq/                    internal/memory/
├── types.go                     ├── types.go
│   ├── Data                     │   ├── Data  
│   └── Provider interface       │   └── Provider interface
└── store.go                     └── store.go
    └── Implementa Provider          └── Implementa Provider

internal/agents/                 internal/agents/
├── faq_agent.go                 ├── memory_agent.go
│   ├── FAQAgent struct          │   ├── MemoryAgent struct
│   ├── provider faq.Provider    │   ├── provider memory.Provider
│   └── Execute/GetTools         │   └── Execute/GetTools

app.go:                          app.go:
├── faqStore = faq.NewStore()    ├── memoryStore = memory.NewStore()
├── faqAgent = NewFAQAgent(...)  ├── memoryAgent = NewMemoryAgent(...)
└── UI methods → faqStore        └── UI methods → memoryStore
```

**Mesmo padrão para Agent Management:**

```
internal/agentmanager/
├── types.go
│   ├── AgentData, HTTPAgentData, MCPAgentData
│   ├── OpenAPIImportResult, ImportedEndpoint
│   └── Manager interface (Provider pattern)
└── manager.go
    └── Implementa Manager interface

internal/agents/
├── builder_agent.go
│   ├── BuilderAgent struct
│   ├── manager agentmanager.Manager
│   └── Execute/GetTools

app.go:
├── agentManager = agentmanager.New(db, llm)
├── builderAgent = NewBuilderAgent(agentManager, ...)
└── tools.go → agentManager
```

**Por que esse padrão é perfeito:**
- ✅ **Consistente**: Exatamente como FAQ, Memory, FileManager
- ✅ **Zero import cycle**: Manager está em internal/
- ✅ **Type safety**: Manager define tipos próprios
- ✅ **DRY**: Mesma lógica para UI e LLM
- ✅ **Testável**: Mock Manager interface
- ✅ **Idiomático Go**: Provider pattern

**Fluxo:**
```
UI (Wails)         → tools.go      → agentManager → database
LLM (BuilderAgent) → builderagent → agentManager → database
                           ↓
                  Mesma lógica, zero duplicação!
```

---

## 3. Arquitetura do Builder Agent

### 3.1. Estrutura com Dependency Injection

```go
// builder_agent.go

// BuilderAgent com funções injetadas (DI pattern)
type BuilderAgent struct {
    BaseAgent
    
    // Funções injetadas do App - trabalham com JSON/maps
    parseOpenAPISpec        func(content string) (map[string]interface{}, error)
    parsePostmanCollection  func(content string) (map[string]interface{}, error)
    createHTTPAgentFull     func(name, displayName, description, model, systemPrompt string, enabled bool,
                                 baseURL, authType, authConfig, defaultHeaders string, 
                                 timeoutSeconds, retryCount int) (map[string]interface{}, error)
    createHTTPEndpoint      func(httpAgentID uint, name, description, method, pathTemplate, 
                                 queryTemplate, headersJSON, bodyTemplate, parameters, 
                                 responseTemplate string) (map[string]interface{}, error)
    testHTTPEndpoint        func(httpAgentID uint, endpointName string, paramsJSON string) (string, error)
    createMCPAgentFull      func(name, displayName, description, model, systemPrompt, transportType,
                                 serverCommand, serverArgs, serverEnv, workingDir, serverURL,
                                 authType, authValue, httpHeaders, executionMode string,
                                 autoConnect, enabled bool) (map[string]interface{}, error)
    getAllAgentConfigs      func() ([]map[string]interface{}, error)
    getAgentConfigByID      func(id uint) (map[string]interface{}, error)
    updateAgentConfig       func(id uint, displayName, description, model, systemPrompt, 
                                 config string, enabled bool) (map[string]interface{}, error)
    deleteAgentConfig       func(id uint) error
    getHTTPAgentByConfigID  func(agentConfigID uint) (map[string]interface{}, error)
    chatCompletion          func(ctx context.Context, model, systemPrompt, userMessage string) (string, error)
}

// Constructor com DI explícito
func NewBuilderAgent(
    llmClient LLMClient,
    model string,
    parseOpenAPI func(string) (map[string]interface{}, error),
    parsePostman func(string) (map[string]interface{}, error),
    createHTTPAgent func(...) (map[string]interface{}, error),
    createHTTPEndpoint func(...) (map[string]interface{}, error),
    testHTTPEndpoint func(...) (string, error),
    createMCPAgent func(...) (map[string]interface{}, error),
    getAllConfigs func() ([]map[string]interface{}, error),
    getConfigByID func(uint) (map[string]interface{}, error),
    updateConfig func(...) (map[string]interface{}, error),
    deleteConfig func(uint) error,
    getHTTPAgent func(uint) (map[string]interface{}, error),
    chatCompletion func(context.Context, string, string, string) (string, error),
) *BuilderAgent {
    return &BuilderAgent{
        BaseAgent: BaseAgent{
            Name: "builder",
            DisplayName: "Agent Builder",
            Model: model,
            SystemPrompt: builderSystemPrompt,
            LLM: llmClient,
        },
        parseOpenAPISpec: parseOpenAPI,
        parsePostmanCollection: parsePostman,
        // ... etc
    }
}
```

### 3.2. Modificações no App

**tools.go - Criar funções adaptadoras que retornam JSON:**

```go
// Adapters para BuilderAgent - convertem tipos internos para JSON-friendly maps
func (a *App) parseOpenAPISpecForBuilder(content string) (map[string]interface{}, error) {
    result, err := a.ParseOpenAPISpec(content)
    if err != nil {
        return nil, err
    }
    // Marshal e Unmarshal para converter para map genérico
    data, _ := json.Marshal(result)
    var m map[string]interface{}
    json.Unmarshal(data, &m)
    return m, nil
}

func (a *App) getAllAgentConfigsForBuilder() ([]map[string]interface{}, error) {
    configs, err := a.GetAllAgentConfigs()
    if err != nil {
        return nil, err
    }
    // Converter para []map[string]interface{}
    data, _ := json.Marshal(configs)
    var result []map[string]interface{}
    json.Unmarshal(data, &result)
    return result, nil
}

// ... outras funções similares
```

**app.go - initAgents():**

```go
func (a *App) initAgents() {
    // ...
    
    // Builder Agent com DI
    builderAgent := agents.NewBuilderAgent(
        a.llmClient,
        agentModel,
        a.parseOpenAPISpecForBuilder,
        a.parsePostmanCollectionForBuilder,
        a.createHTTPAgentFullForBuilder,
        a.createHTTPEndpointForBuilder,
        a.testHTTPEndpointForBuilder,
        a.createMCPAgentFullForBuilder,
        a.getAllAgentConfigsForBuilder,
        a.getAgentConfigByIDForBuilder,
        a.updateAgentConfigForBuilder,
        a.deleteAgentConfigForBuilder,
        a.getHTTPAgentByConfigIDForBuilder,
        a.chatCompletionForBuilder,
    )
    a.applyAgentConfig(builderAgent)
    a.registry.Register(builderAgent)
}
```

**Vantagens desta abordagem:**
1. ✅ **Zero import cycle** - BuilderAgent não conhece tipos do App
2. ✅ **Testável** - Mock functions facilmente
3. ✅ **Explícito** - Fica claro quais dependências o agent precisa
4. ✅ **Sem conversões complexas** - Tudo é JSON/maps
5. ✅ **Go idiomático** - Dependency Injection com funções é padrão Go

**Alternativa ainda mais limpa (Option pattern):**

```go
type BuilderAgentConfig struct {
    ParseOpenAPI       func(string) (map[string]interface{}, error)
    ParsePostman       func(string) (map[string]interface{}, error)
    CreateHTTPAgent    func(...) (map[string]interface{}, error)
    // ... etc
}

func NewBuilderAgent(llmClient LLMClient, model string, config BuilderAgentConfig) *BuilderAgent {
    // ...
}.parseOpenAPISpec() (função injetada)
  - Resultado já é map[string]interface{}
  - Marshal para JSON e retornar
builderAgent := agents.NewBuilderAgent(a.llmClient, agentModel, agents.BuilderAgentConfig{
    ParseOpenAPI: a.parseOpenAPISpecForBuilder,
    ParsePostman: a.parsePostmanCollectionForBuilder,
    // ...
})
```

---

## 4. Tools do Builder Agent

### 4.1. Análise e Planejamento

**1. analyze_openapi**
```yaml
Entrada: { spec_content: string }
Saída: Análise estruturada (endpoints, auth, base_url)
Implementação:
  - Chama appContext.ParseOpenAPISpec()
  - Type assertion para ParseResult
  - Retorna JSON formatado para o LLM
```

**2. analyze_postman**
```yaml
Entrada: { collection_json: string }
Saída: Análise estruturada
Implementação: Similar ao analyze_openapi
```

**3. extract_requirements**
```yaml
Entrada: { user_instructions: string }
Saída: Requisitos estruturados para criar agente
Implementação:
  - Usa appContext.ChatCompletion() para analisar
  - Prompt específico: extrair nome, base_url, endpoints, auth
```

**4. plan_agent**
```yaml
Entrada: { type: "http"|"mcp", analysis: string }
Saída: Plano de criação detalhado
Implementação:
  - Usa appContext.ChatCompletion()
  - Gera plano estruturado antes de criar
  - Permite revisão/confirmação do usuário
```

### 4.2. Criação de Agentes HTTP

**5. create_http_agent**
```yaml
Entrada: {
  name: string,
  display_name: string,
  description: string,
  base_url: string,
  auth_type?: string,
  auth_config?: string,
  default_headers?: string,
  model?: string
}
Saída: AgentConfig criada.getHTTPAgentByConfigID()
  - Extrai http_agent_id do map retornado
  - Chama a.cppContext.CreateHTTPAgentFull()
  - Retorna ID e detalhes
```

**6. add_http_endpoint**
```yaml
Entrada: {
  agent_config_id: number,
  name: string,
  description: string,
  method: "GET"|"POST"|"PUT"|"DELETE"|...,
  path_template: string,
  query_template?: string,
  body_template?: string,
  parameters: object,
  response_template?: string
}
Saída: Endpoint criado
Implementação:
  - httpAgent, err := a.app.GetHTTPAgentByConfigID(agentConfigID)
  - endpoint, err := a.app.CreateHTTPEndpoint(httpAgent.ID, ...)
  - endpoint é *dto.HTTPEndpoint (type safe!)
  - json.MarshalIndent(endpoint)
```

**7. test_http_endpoint**
```yaml
Entrada: {
  http_agent_id: number,
  endpoint_name: string,
  parameters: object
}
Saída: Resposta da API
Implementação:
  - Chama appContext.TestHTTPEndpoint()
  - Retorna resultado ou erro
```

**8. import_openapi_agent**
```yaml
Entrada: {
  spec_content: string,
  name?: string,
  model?: string
}
Saída: Agente completo criado
Implementação:
  - Analisa spec com ParseOpenAPISpec()
  - Cria HTTPAgent
  - Adiciona todos os endpoints
  - Retorna summary
```

**9. import_postman_agent**
```yaml
Similar ao import_openapi_agent
```

### 4.3. Criação de Agentes MCP

**10. create_mcp_agent**
```yaml
Entrada: {
  name: string,
  display_name: string,
  description: string,
  transport_type: "stdio"|"http",
  # Para stdio:
  server_command?: string,
  server_args?: string,
  server_env?: string,
  # Para http:
  server_ua.getAllAgentConfigs()
  - Resultado já é []map[string]interface{}
  auth_value?: string,
  # Comum:
  model?: string,
  auto_connect?: bool
}
Saída: MCP Agent criado
Implementação:
  - Valida parâmetros baseado no transport_type
  - Chama appContext.CreateMCPAgentFull()
  - Retorna detalhes
```

### 4.4. Gerenciamento

**11. list_agents**
```yaml
Entrada: { filter?: string }
Saída: Lista de agentes
Implementação:
  - Chama GetAllAgentConfigs()
  - Type assertion para []AgentConfigInfo
  - Marshal para JSON
```

**12. get_agent_details**
```yaml
Entrada: { agent_id: number }
Saída: Detalhes completos
Implementação:
  - config, err := a.app.GetAgentConfigByID(id)
  - Se config.AgentType == "http":
      httpAgent, _ := a.app.GetHTTPAgentByConfigID(id)
  - Criar struct combinado com ambos
  - json.MarshalIndent
```

**13. update_agent**
```yaml
Entrada: {
  agent_id: number,
  display_name?: string,
  description?: string,
  model?: string,
  system_prompt?: string,
  enabled?: bool
}
Saídconfig, err := a.app.UpdateAgentConfig(id, displayName, ...)
  - config é *dto.AgentConfig (type safe!)
  - json.MarshalIndent(config)ntConfig()
  - Resultado já é map[string]interface{}
  - Marshal para JSON
```

**14. delete_agent**
```yaml
Entrada: { agent_id: number }
Saída: Confirmação
Implementaa.deleteAgentConfig()
  - Cascade delete automático (HTTPAgent/MCPAgent)
  - Retorna mensagem de sucesso
  - Cascade delete automático (HTTPAgent/MCPAgent)
```

**15. generate_json_schema**
```yaml
Entrada: { example_json?: string, description?: string }
Saída: Ja.chatCompletion() para gerar schema
  - Prompt específico: criar JSON Schema válido
Implementação:
  - Usa ChatCompletion() para gerar schema
  - Útil para criar parameters de endpoints
```

---

## 5. System Prompt

```markdown
Você é o **Agent Builder**, especialista em criar e gerenciar agentes HTTP e MCP.

## Suas Capacidades

### 1. Análise de Documentação
- **analyze_openapi**: Analisa especificações OpenAPI/Swagger
- **analyze_postman**: Analisa coleções Postman
- **extract_requirements**: Extrai requisitos de instruções em linguagem natural

### 2. Planejamento
- **plan_agent**: Cria plano detalhado antes de implementar
- Sempre planeje antes de criar para revisar com o usuário

### 3. Criação de Agentes HTTP
- **create_http_agent**: Cria novo agente HTTP
- **add_http_endpoint**: Adiciona endpoints a um agente
- **test_http_endpoint**: Testa endpoints com parâmetros
- **import_openapi_agent**: Importa direto de spec OpenAPI
- **import_postman_agent**: Importa direto de coleção Postman

### 4. Criação de Agentes MCP
- **create_mcp_agent**: Cria agente MCP (stdio ou http)
  - **stdio**: Para servers locais (ex: npx -y @modelcontextprotocol/server-github)
  - **http**: Para servers remotos com URL

### 5. Gerenciamento
- **list_agents**: Lista todos os agentes
- **get_agent_details**: Detalhes de um agente
- **update_agent**: Atualiza configuração
- **delete_agent**: Remove agente

### 6. Utilitários
- **generate_json_schema**: Gera JSON Schema para parameters

## Workflows Recomendados

### Criar Agente de OpenAPI:
1. **analyze_openapi** com o conteúdo do spec
2. **plan_agent** para revisar estrutura
3. **import_openapi_agent** para criar completo
OU criar manualmente:
3. **create_http_agent** com config base
4. **add_http_endpoint** para cada endpoint

### Criar Agente de Postman:
Similar ao OpenAPI, usando **analyze_postman** e **import_postman_agent**

### Criar Agente de Linguagem Natural:
1. **extract_requirements** das instruções do usuário
2. **plan_agent** para estruturar
3. **create_http_agent** + múltiplos **add_http_endpoint**
4. **test_http_endpoint** para validar

### Criar Agente MCP:
1. Confirmar tipo (stdio vs http)
2. **create_mcp_agent** com configuração
3. Testar conexão

## Templates de Sintaxe

### Path Templates
```
/users/{user_id}/posts
/api/v1/{resource}
```

### Query Templates
```
page={{.Params.page}}&limit={{.Params.limit}}
q={{.Params.query}}&filter={{.Params.filter}}
```

### Body Templates
```json
{
  "name": "{{.Params.name}}",
  "email": "{{.Params.email}}",
  "metadata": {{.Params.metadata}}
}
```

### Headers (JSON)
```json
{
  "X-API-Key": "{{.Env.API_KEY}}",
  "Content-Type": "application/json"
}
```

## Tipos de Auth

- **none**: Sem autenticação
- **bearer**: Bearer token (use {{.Env.TOKEN}})
- **api_key**: API Key em header (auth_config: {"header": "X-API-Key", "value": "{{.Env.KEY}}"})
- **oauth2**: OAuth2 (auth_config: JSON com client_id, token_url, etc.)

## Boas Práticas

1. **Sempre planejar** antes de criar
2. **Testar endpoints** após criar
3. **Usar variáveis de ambiente** para credenciais ({{.Env.VAR}})
4. **Descrições claras** para LLM saber quando usar cada endpoint
5. **Parameters estruturados** com type, description, required
6. **Response templates** para formatar saídas

## Exemplos de Parameters

```json
{
  "user_id": {
    "type": "string",
    "description": "ID do usuário",
    "required": true
  },
  "page": {
    "type": "integer",
    "description": "Número da página",
    "default": 1
  }
}
```

Seja proativo: sugira melhorias, valide configurações, teste antes de confirmar.
```

---

## 6. Estrutura de Código

```
internal/agents/
├── agent.go              # Interface Agent (já existe)
├── registry.go           # Registry (já existe)
├── builder_agent.go      # NOVO - Builder Agent
└── BuilderAgent struct (com funções injetadas)
├── BuilderAgentConfig struct (option pattern)
├── NewBuilderAgent(llmClient, model, config)
├── Execute()
├── GetTools()
├── ExecuteTool()
└── Implementações das 15 tools

tools.go (App):
└── Funções *ForBuilder (14 adapters que retornam JSON/maps)

app.go:
└── initAgents() - criar e registrar BuilderAgent com DI

importers.go:
└── Wrappers *ForBuilder (2 funções)
```

---

## 7. Plano de Implementação

### Fase 1: Criar Package AgentManager
- [ ] Criar `internal/agentmanager/` package
- [ ] Criar `manager.go` com Manager struct
- [ ] Criar `types.go` com Request types e OpenAPIImportResult
- [ ] Implementar `NewManager(db, llm)`

### Fase 2: Migrar Lógica Existente para AgentManager
- [ ] `parser.go`: Mover lógica de `importers.go` (ParseOpenAPISpec, ParsePostmanCollection)
- [ ] `http_creator.go`: Mover lógica de `tools.go` (CreateHTTPAgent, CreateHTTPEndpoint)
- [ ] `mcp_creator.go`: Mover lógica de `tools.go` (CreateMCPAgent)
- [ ] `tester.go`: Mover TestHTTPEndpoint
- [ ] `importer.go`: ImportOpenAPIAgent, ImportPostmanAgent (combina parse + create)
- [ ] Implementar métodos Get/Update/Delete

### Fase 3: Atualizar App para usar AgentManager
- [ ] `app.go`: Adicionar `agentManager *agentmanager.Manager`
- [ ] `app.go`: Inicializar manager em `startup()`
- [ ] `tools.go`: Refatorar para delegar para `agentManager`
- [ ] `importers.go`: Pode ser removido ou simplificado

### Fase 4: BuilderAgent Base
- [ ] Criar `builder_agent.go`
- [ ] BuilderAgent struct com `manager *agentmanager.Manager`
- [ ] Implementar `NewBuilderAgent(manager, llmClient, model)`
- [ ] Definir system prompt completo

### Fase 5: Tools de Análise
- [ ] `analyze_openapi`
- [ ] `analyze_postman`
- [ ] `extract_requirements`
- [ ] `plan_agent`

### Fase 6: Tools HTTP
- [ ] `create_http_agent`
- [ ] `add_http_endpoint`
- [ ] `test_http_endpoint`
- [ ] `import_openapi_agent`
- [ ] `import_postman_agent`

### Fase 7: Tools MCP
- [ ] `create_mcp_agent`

### Fase 8: Tools de Gerenciamento
- [ ] `list_agents`
- [ ] `get_agent_details`
- [ ] `update_agent`
- [ ] `delete_agent`
- [ ] `generate_json_schema`

### Fase 9: Integração
- [ ] Registrar BuilderAgent em `app.go`
- [ ] Testar delegação do orquestrador
- [ ] Validar fluxos completos

### Fase 10: Testes
- [ ] Teste: criar agente de OpenAPI
- [ ] Teste: criar agente de Postman
- [ ] Teste: criar agente de linguagem natural
- [ ] Teste: criar agente MCP stdio
- [ ] Teste: criar agente MCP http
- [ ] Teste: gerenciar agentes (list, update, delete)

---

## 8. Critérios de Qualidade

### Código Limpo
- ✅ Sem wrappers desnecessários
- ✅ Comentários claros explicando decisões de design
- ✅ Type assertions centralizadas e documentadas
- ✅ Funções pequenas e focadas

### Consistência
- ✅ Segue padrão de FileAgent/MemoryAgent/FAQAgent
- ✅ Mesmo style de tools e execute
- ✅ Mesma estrutura de erros e logging

### Manutenibilidade
- ✅ Fácil adicionar novas tools
- ✅ Fácil entender onde fazer mudanças
- ✅ Import cycle resolvido de forma clara
- ✅ Documentação inline suficiente

### Funcionalidade
- ✅ Cria agentes HTTP funcionais
- ✅ Cria agentes MCP funcionais
- ✅ Gerencia agentes existentes
- ✅ Testa endpoints
- ✅ Valida configurações

---

## 9. Referências

- `docs/AGENTS_ARCHITECTURE.md` - Arquitetura de agentes
- `internal/agents/file_agent.go` - Exemplo de agente complexo
- `internal/agents/faq_agent.go` - Exemplo de agente simples
- `internal/agents/http_agent.go` - Agentes HTTP dinâmicos
- `internal/agents/mcp_agent.go` - Agentes MCP
- `tools.go` - Funções de criação de agentes
- `importers.go` - Parsers OpenAPI/Postman

---

## 10. Decisões de Design

### Por que AgentManager centralizado?

**Problema original:** 
- `tools.go` tem lógica de criar agentes (para UI Wails)
- BuilderAgent precisaria duplicar essa lógica (para LLM)
- Duplicação = manutenção 2x, bugs 2x

**Solução: Package AgentManager**
- ✅ **DRY**: Uma única fonte de verdade
- ✅ **Testável**: Testar manager isoladamente
- ✅ **Reutilizável**: UI E LLM usam mesma base
- ✅ **Manutenível**: Mudança em um lugar reflete em ambos
- ✅ **Zero import cycle**: AgentManager quebra o ciclo main ↔ internal/agents

### Por que NÃO usar DTOs?

**Problema que DTOs resolveriam:** Import cycle entre `main` e `internal/agents`

**MAS:** AgentManager já resolve esse problema!

```
Sem AgentManager:
main → internal/agents
internal/agents → main ❌ CYCLE!

Com AgentManager:
main → internal/agentmanager → internal/database ✅
internal/agents → internal/agentmanager → internal/database ✅
```

**Por que database.Models direto é suficiente:**
- ✅ Models já têm JSON tags (funcionam para serialização)
- ✅ Type safety completo
- ✅ Zero conversões/adaptadores desnecessários
- ✅ Menos código para manter

**Quando DTOs seriam necessários:**
- Se precisássemos versionar API externa
- Se precisássemos omitir/calcular campos sensíveis
- Se schema do banco divergisse muito da API

**Para este projeto:** É uma aplicação desktop interna, não uma API pública. Models do banco servem perfeitamente como contratos de comunicação.

### Arquitetura em Camadas

```
┌─────────────────────────────────────────────┐
│          UI Layer (Wails Frontend)          │
└───────────────┬─────────────────────────────┘
                │
┌───────────────▼─────────────────────────────┐
│        App Layer (tools.go)                 │
│        - Wails bindings                     │
│        - Converte para JSON                 │
└───────────────┬─────────────────────────────┘
                │
                ├──────────────┐
                │              │
┌───────────────▼──────┐  ┌───▼──────────────────────┐
│  BuilderAgent        │  │  Other Agents            │
│  (LLM Interface)     │  │  (FileAgent, etc)        │
└───────────────┬──────┘  └──────────────────────────┘
                │
┌───────────────▼─────────────────────────────┐
│      AgentManager (Business Logic)          │
│      - CreateHTTPAgent                      │
│      - ParseOpenAPISpec                     │
│      - TestHTTPEndpoint                     │
│      - etc                                  │
└───────────────┬─────────────────────────────┘
                │
┌───────────────▼─────────────────────────────┐
│       Database Layer (GORM)                 │
│       - AgentConfig model                   │
│       - HTTPAgent model                     │
│       - Models têm JSON tags!               │
└─────────────────────────────────────────────┘
```

**Benefícios:**
1. **Separation of Concerns**: Cada layer tem responsabilidade clara
2. **Testabilidade**: Cada layer pode ser testada isoladamente
3. **Flexibilidade**: Fácil adicionar novos consumers do AgentManager
4. **Manutenibilidade**: Mudanças em business logic não afetam UI
5. **Simplicidade**: Menos layers = menos complexidade

### Trade-offs

**Prós:**
- ✅ Zero duplicação de código
- ✅ Type safety completo (database.Models são tipos concretos)
- ✅ Testável em camadas
- ✅ Escalável (fácil adicionar consumers)
- ✅ Manutenível a longo prazo
- ✅ **Simples**: Sem DTOs, conversores, ou layers extras

**Contras:**
- ⚠️ Models do banco expostos (mas OK para app interno)
- ⚠️ Mudanças no schema afetam API (mitigado: versão única desktop)

**ROI:** 
- **Curto prazo**: Implementação limpa e direta
- **Médio prazo** (3 meses): Zero overhead de conversões
- **Longo prazo**: Código limpo, fácil manutenção

---

**Próximo passo:** Implementar seguindo este plano, fase por fase, validando cada etapa antes de prosseguir.
