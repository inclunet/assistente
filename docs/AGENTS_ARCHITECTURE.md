# Arquitetura de Agentes Inteligentes

## Visão Geral

O sistema utiliza uma arquitetura de **agentes inteligentes** onde cada agente possui seu próprio LLM e é especialista em um domínio. O LLM principal atua como **orquestrador**, delegando tarefas para os agentes apropriados.

```
┌─────────────────────────────────────────────────────────────┐
│                     LLM PRINCIPAL                           │
│                    (Orquestrador)                           │
│                                                             │
│  Conhece apenas os AGENTES, não as tools individuais       │
│                                                             │
│  Tools:                                                     │
│  - delegate_to_faq(task: "...")                            │
│  - delegate_to_memory(task: "...")                         │
│  - delegate_to_weather(task: "...")                        │
└─────────────────────┬───────────────────────────────────────┘
                      │ delega tarefa
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│  FAQ Agent   │ │ Memory Agent │ │ HTTP Agent   │
│  (LLM + Tools)│ │ (LLM + Tools)│ │ (LLM + Tools)│
└──────────────┘ └──────────────┘ └──────────────┘
```

## Por que Agentes Inteligentes?

### Problema com abordagem tradicional (Single-LLM)

| Problema | Impacto |
|----------|---------|
| Limite de tools | ~20-50 tools máximo antes de degradar |
| Context bloat | Cada tool consome 200-500 tokens |
| Confusão | LLM fica confuso com muitas opções |
| Escalabilidade | Não escala para sistemas complexos |

### Solução: Cada agente com seu LLM

| Benefício | Descrição |
|-----------|-----------|
| Escalável | LLM principal conhece só N agentes |
| Especializado | Cada agente é expert no seu domínio |
| Flexível | Cada agente pode usar modelo diferente |
| Isolado | Problemas em um agente não afetam outros |

---

## Tipos de Agentes

### 1. Internal Agent (FAQ, Memory)
Agentes com lógica em Go + LLM próprio para decidir qual tool usar.

```go
type InternalAgent struct {
    Name        string
    Model       string           // Modelo do agente
    SystemPrompt string          // Prompt especializado
    Tools       []Tool           // Tools internas
    Provider    DataProvider     // Acesso a dados
}
```

**Fluxo:**
```
Orquestrador: delegate_to_faq("busque FAQs sobre deploy")
                    │
                    ▼
            ┌──────────────┐
            │  FAQ Agent   │
            │              │
            │ LLM decide:  │ ← "Vou usar faq_search"
            │ faq_search   │
            │ ("deploy")   │
            │              │
            │ Executa tool │
            │              │
            │ Formata      │
            │ resposta     │
            └──────┬───────┘
                   │
                   ▼
            Retorna para Orquestrador
```

### 2. HTTP Agent
Agentes que interagem com APIs REST + LLM para decidir endpoints.

```go
type HTTPAgent struct {
    Name        string
    Model       string
    BaseURL     string
    Endpoints   []Endpoint       // GET /weather, POST /search, etc.
    Auth        AuthConfig
}
```

### 3. MCP Agent
Agentes que se conectam a servidores MCP.

```go
type MCPAgent struct {
    Name          string
    Model         string
    ServerCommand string
    ServerArgs    []string
}
```

---

## Modelo de Dados

### Tabela: agents

```sql
CREATE TABLE agents (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,           -- "faq", "memory", "weather"
    display_name TEXT NOT NULL,          -- "FAQ Manager"
    type TEXT NOT NULL,                  -- "internal", "http", "mcp"
    description TEXT,                    -- Descrição para o orquestrador
    model TEXT,                          -- Modelo do agente (gpt-4o-mini, etc.)
    system_prompt TEXT,                  -- System prompt especializado
    config TEXT,                         -- JSON com config específica
    enabled INTEGER DEFAULT 1,
    created_at DATETIME,
    updated_at DATETIME
);
```

### Tabela: agent_tools

```sql
CREATE TABLE agent_tools (
    id INTEGER PRIMARY KEY,
    agent_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    parameters TEXT,                     -- JSON Schema
    
    -- Para HTTP Agent
    http_endpoint TEXT,
    http_method TEXT,
    http_headers TEXT,
    http_body_template TEXT,
    
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);
```

---

## Interface Go

```go
// Agent define a interface para agentes inteligentes
type Agent interface {
    // Identificação
    GetName() string
    GetDisplayName() string
    GetDescription() string  // Descrição para o orquestrador
    GetType() string
    
    // Configuração
    GetModel() string        // Modelo LLM do agente
    IsEnabled() bool
    
    // Execução inteligente
    // Recebe tarefa em linguagem natural, retorna resultado
    Execute(ctx context.Context, task string) (string, error)
}

// AgentRegistry gerencia agentes
type AgentRegistry struct {
    agents map[string]Agent
    app    *App  // Para chamar LLM
}

// GetDelegationTools retorna tools de delegação para o orquestrador
func (r *AgentRegistry) GetDelegationTools() []Tool {
    // Retorna: delegate_to_faq, delegate_to_memory, etc.
}

// ExecuteDelegation executa delegação para um agente
func (r *AgentRegistry) ExecuteDelegation(agentName, task string) (string, error)
```

---

## Fluxo Detalhado

### Exemplo: "Quantas FAQs temos sobre Kubernetes?"

```
1. USUÁRIO
   └─> "Quantas FAQs temos sobre Kubernetes?"

2. LLM ORQUESTRADOR
   ├─ System Prompt: "Você coordena agentes especializados..."
   ├─ Tools: [delegate_to_faq, delegate_to_memory, ...]
   └─> Decide: delegate_to_faq(task="conte FAQs sobre Kubernetes")

3. FAQ AGENT (LLM próprio)
   ├─ System Prompt: "Você gerencia FAQs. Suas tools são..."
   ├─ Tools: [faq_search, faq_list, faq_create, ...]
   ├─ Recebe: "conte FAQs sobre Kubernetes"
   ├─> Decide: faq_search(query="Kubernetes")
   ├─ Executa tool
   ├─ Resultado: [{id:1, question:"..."}, {id:2, ...}]
   └─> Formata: "Encontrei 5 FAQs sobre Kubernetes: ..."

4. LLM ORQUESTRADOR
   ├─ Recebe resposta do FAQ Agent
   └─> Formata resposta final para usuário

5. USUÁRIO
   └─ "Encontrei 5 FAQs sobre Kubernetes: 1. Como instalar..."
```

---

## Fases de Implementação

### Fase 1: Refatorar para Agentes Inteligentes ✅ Concluída
**Objetivo:** Transformar FAQ e Memory em agentes com LLM próprio.

**Tarefas:**
- [x] Criar interface `Agent` com método `Execute(task string)`
- [x] Refatorar `FAQAgent` para ter LLM próprio
- [x] Refatorar `MemoryAgent` para ter LLM próprio
- [x] Criar `GetDelegationTools()` no Registry
- [x] Atualizar orquestrador para usar delegação
- [x] Testes de integração

**Arquivos:**
```
internal/agents/
├── agent.go              # Interface Agent
├── registry.go           # Registry com delegação
├── faq_agent.go          # FAQAgent inteligente
├── memory_agent.go       # MemoryAgent inteligente
└── llm_client.go         # Cliente LLM para agentes
```

### Fase 2: UI de Gerenciamento ✅ Concluída
**Objetivo:** Interface para configurar agentes.

**Tarefas:**
- [x] CRUD de agentes no backend
- [x] UI para listar agentes
- [x] UI para configurar modelo/prompt de cada agente
- [x] Playground para testar agentes

**Arquivos criados:**
```
database.go              # Adicionado: AgentConfig model + CRUD
tools.go                 # Adicionado: GetRegisteredAgents, TestAgent
frontend/src/components/AgentManager.svelte  # UI de gerenciamento
```

### Fase 3: HTTP Agents ✅ Concluída
**Objetivo:** Agentes que chamam APIs REST com templates flexíveis.

**Tarefas Backend:**
- [x] Modelo de dados: `http_agents` e `http_endpoints` no banco
- [x] CRUD de HTTP Agents e Endpoints
- [x] Engine de templates (Go template) com funções customizadas
- [x] Executor HTTP com retry, timeout, auth
- [x] Integração com o sistema de agentes existente

**Tarefas Frontend:**
- [x] Instalar e configurar Monaco Editor
- [x] Componente `MonacoTemplate.svelte` com autocomplete
- [x] Componente `SchemaBuilder.svelte` (builder visual)
- [x] Componente `HTTPAgentEditor.svelte` (tela principal)
- [x] Componente `EndpointEditor.svelte` (config de endpoint)
- [x] Headers editor integrado no HTTPAgentEditor
- [x] Auth config integrado no HTTPAgentEditor
- [x] Playground para testar endpoints

**Melhorias Avançadas (implementadas):**
- [x] Descoberta automática via OpenAPI/Swagger
- [x] Importar coleções do Postman
- [x] OAuth 2.0 flow completo (Client Credentials, Authorization Code)

**Arquivos criados:**
```
internal/agents/
├── http_agent.go           # HTTPAgent implementation
├── http_template.go        # Template engine com funções
├── http_template_test.go   # Testes
├── http_executor.go        # Executor HTTP
├── openapi_parser.go       # Parser de OpenAPI/Swagger
├── openapi_parser_test.go  # Testes do parser OpenAPI
├── postman_parser.go       # Parser de Postman Collections
├── postman_parser_test.go  # Testes do parser Postman
├── oauth2_client.go        # Cliente OAuth 2.0

database.go               # HTTPAgent e HTTPEndpoint models + CRUD
tools.go                  # API para frontend (Create/Get/Update/Delete/Test/Import)

frontend/src/components/
├── HTTPAgentEditor.svelte  # Editor principal (com OAuth 2.0)
├── EndpointEditor.svelte   # Editor de endpoint
├── SchemaBuilder.svelte    # Builder visual de JSON Schema
├── MonacoTemplate.svelte   # Monaco Editor com autocomplete
├── ImportSpecModal.svelte  # Modal de importação OpenAPI/Postman
```

---

## Design Detalhado: HTTP Agents

### Modelo de Dados

```sql
-- Configuração base do agente HTTP
CREATE TABLE http_agents (
    id INTEGER PRIMARY KEY,
    agent_id INTEGER NOT NULL,          -- FK para agents
    base_url TEXT NOT NULL,             -- https://api.example.com/v1
    
    -- Autenticação
    auth_type TEXT,                     -- none, api_key, bearer, basic, oauth2
    auth_config TEXT,                   -- JSON com config de auth
    
    -- Headers globais (aplicados a todas as requests)
    default_headers TEXT,               -- JSON: {"Content-Type": "application/json"}
    
    -- Configurações
    timeout_seconds INTEGER DEFAULT 30,
    retry_count INTEGER DEFAULT 3,
    
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);

-- Endpoints/Tools do agente HTTP
CREATE TABLE http_endpoints (
    id INTEGER PRIMARY KEY,
    http_agent_id INTEGER NOT NULL,
    
    -- Identificação
    name TEXT NOT NULL,                 -- get_weather, create_user
    description TEXT,                   -- Descrição para o LLM
    
    -- Request
    method TEXT NOT NULL,               -- GET, POST, PUT, DELETE, PATCH
    path_template TEXT NOT NULL,        -- /users/{{.user_id}}/posts
    query_template TEXT,                -- page={{.page}}&limit={{.limit}}
    headers_template TEXT,              -- JSON com headers específicos
    body_template TEXT,                 -- Template do corpo (para POST/PUT)
    
    -- Parâmetros (para o LLM saber o que pedir)
    parameters TEXT NOT NULL,           -- JSON Schema dos parâmetros
    
    -- Response
    response_template TEXT,             -- Template para formatar resposta
    
    FOREIGN KEY (http_agent_id) REFERENCES http_agents(id)
);
```

### Go Template Engine

Usamos `text/template` do Go com funções auxiliares:

```go
// Funções disponíveis nos templates
var templateFuncs = template.FuncMap{
    // Encoding
    "urlEncode":    url.QueryEscape,
    "jsonEncode":   jsonEncode,
    "base64Encode": base64Encode,
    
    // Strings
    "lower":    strings.ToLower,
    "upper":    strings.ToUpper,
    "trim":     strings.TrimSpace,
    "replace":  strings.ReplaceAll,
    
    // Condicionais
    "default":  defaultValue,  // {{.value | default "fallback"}}
    "required": required,      // Erro se vazio
    
    // Data
    "now":      time.Now,
    "formatDate": formatDate,  // {{now | formatDate "2006-01-02"}}
}
```

### Exemplos de Configuração

#### Exemplo 1: API de Clima

```json
{
  "name": "weather",
  "display_name": "Weather API",
  "type": "http",
  "base_url": "https://api.openweathermap.org/data/2.5",
  "auth_type": "api_key",
  "auth_config": {
    "param_name": "appid",
    "location": "query"
  },
  "endpoints": [
    {
      "name": "get_current_weather",
      "description": "Obtém o clima atual de uma cidade",
      "method": "GET",
      "path_template": "/weather",
      "query_template": "q={{.city | urlEncode}}&units={{.units | default \"metric\"}}",
      "parameters": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string",
            "description": "Nome da cidade (ex: São Paulo, BR)"
          },
          "units": {
            "type": "string",
            "enum": ["metric", "imperial"],
            "description": "Unidade de medida"
          }
        },
        "required": ["city"]
      },
      "response_template": "Clima em {{.name}}: {{.main.temp}}°C, {{.weather[0].description}}"
    }
  ]
}
```

#### Exemplo 2: API REST com POST

```json
{
  "name": "crm",
  "display_name": "CRM API",
  "type": "http",
  "base_url": "https://api.crm.example.com/v2",
  "auth_type": "bearer",
  "auth_config": {
    "token_env": "CRM_API_TOKEN"
  },
  "default_headers": {
    "Content-Type": "application/json",
    "X-Client-Id": "assistente-app"
  },
  "endpoints": [
    {
      "name": "get_customer",
      "description": "Busca um cliente por ID",
      "method": "GET",
      "path_template": "/customers/{{.customer_id}}",
      "parameters": {
        "type": "object",
        "properties": {
          "customer_id": {
            "type": "string",
            "description": "ID do cliente"
          }
        },
        "required": ["customer_id"]
      }
    },
    {
      "name": "create_customer",
      "description": "Cria um novo cliente",
      "method": "POST",
      "path_template": "/customers",
      "body_template": {
        "name": "{{.name}}",
        "email": "{{.email}}",
        "phone": "{{.phone | default \"\"}}",
        "metadata": {
          "source": "assistente",
          "created_at": "{{now | formatDate \"2006-01-02T15:04:05Z07:00\"}}"
        }
      },
      "parameters": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "description": "Nome completo"},
          "email": {"type": "string", "description": "E-mail"},
          "phone": {"type": "string", "description": "Telefone (opcional)"}
        },
        "required": ["name", "email"]
      }
    },
    {
      "name": "search_customers",
      "description": "Busca clientes com filtros",
      "method": "GET",
      "path_template": "/customers",
      "query_template": "q={{.query | urlEncode}}&page={{.page | default 1}}&limit={{.limit | default 10}}",
      "parameters": {
        "type": "object",
        "properties": {
          "query": {"type": "string", "description": "Termo de busca"},
          "page": {"type": "integer", "description": "Página (default: 1)"},
          "limit": {"type": "integer", "description": "Itens por página (default: 10)"}
        },
        "required": ["query"]
      }
    }
  ]
}
```

#### Exemplo 3: API com Path Variables

```json
{
  "name": "github",
  "display_name": "GitHub API",
  "type": "http",
  "base_url": "https://api.github.com",
  "auth_type": "bearer",
  "auth_config": {
    "token_env": "GITHUB_TOKEN"
  },
  "default_headers": {
    "Accept": "application/vnd.github+json",
    "X-GitHub-Api-Version": "2022-11-28"
  },
  "endpoints": [
    {
      "name": "get_repo",
      "description": "Obtém informações de um repositório",
      "method": "GET",
      "path_template": "/repos/{{.owner}}/{{.repo}}",
      "parameters": {
        "type": "object",
        "properties": {
          "owner": {"type": "string", "description": "Dono do repositório"},
          "repo": {"type": "string", "description": "Nome do repositório"}
        },
        "required": ["owner", "repo"]
      }
    },
    {
      "name": "create_issue",
      "description": "Cria uma issue em um repositório",
      "method": "POST",
      "path_template": "/repos/{{.owner}}/{{.repo}}/issues",
      "body_template": {
        "title": "{{.title}}",
        "body": "{{.body | default \"\"}}",
        "labels": {{.labels | jsonEncode | default "[]"}}
      },
      "parameters": {
        "type": "object",
        "properties": {
          "owner": {"type": "string"},
          "repo": {"type": "string"},
          "title": {"type": "string", "description": "Título da issue"},
          "body": {"type": "string", "description": "Descrição"},
          "labels": {"type": "array", "items": {"type": "string"}}
        },
        "required": ["owner", "repo", "title"]
      }
    }
  ]
}
```

### Tipos de Autenticação

| Tipo | Descrição | Config |
|------|-----------|--------|
| `none` | Sem autenticação | - |
| `api_key` | API Key em header ou query | `header_name`, `param_name`, `location` (header/query), `value_env` |
| `bearer` | Bearer token | `token_env` ou `token_value` |
| `basic` | HTTP Basic Auth | `username_env`, `password_env` |
| `oauth2` | OAuth 2.0 (futuro) | `client_id`, `client_secret`, `token_url`, `scopes` |

### Fluxo de Execução

```
1. LLM recebe tarefa
   └─> "Qual o clima em São Paulo?"

2. LLM escolhe tool
   └─> get_current_weather(city: "São Paulo, BR")

3. HTTPAgent processa
   ├─ Resolve path_template: /weather
   ├─ Resolve query_template: q=S%C3%A3o%20Paulo%2C%20BR&units=metric
   ├─ Adiciona auth: &appid=xxxxx
   └─ Monta URL final: https://api.openweathermap.org/data/2.5/weather?q=...

4. Executa request
   └─> GET https://api.openweathermap.org/data/2.5/weather?q=...

5. Processa response
   ├─ Se response_template: aplica template
   └─> "Clima em São Paulo: 25°C, céu limpo"

6. Retorna para LLM
   └─> LLM formula resposta final
```

### Variáveis Especiais nos Templates

| Variável | Descrição |
|----------|-----------|
| `{{.param_name}}` | Parâmetro passado pelo LLM |
| `{{.env.VAR_NAME}}` | Variável de ambiente |
| `{{.agent.name}}` | Nome do agente |
| `{{.request_id}}` | ID único da request |

### Contexto Unificado do Template

Todas as variáveis estão disponíveis em **todos os campos de template** (path, query, headers, body, response):

```go
// Contexto disponível nos templates
{
    // Parâmetros do schema (passados pelo LLM)
    "customer_id": "123",
    "name": "João",
    "email": "joao@email.com",
    
    // Variáveis de ambiente
    "env": {
        "API_TOKEN": "xxx",
        "APP_ENV": "production"
    },
    
    // Metadados do agent
    "agent": {
        "name": "crm",
        "display_name": "CRM API"
    },
    
    // Request info
    "request_id": "uuid-xxx",
    "timestamp": "2024-01-15T10:30:00Z"
}
```

### Funções Disponíveis nos Templates

| Função | Descrição | Exemplo |
|--------|-----------|---------|
| `urlEncode` | Escapa para URL | `{{.query \| urlEncode}}` |
| `jsonEncode` | Converte para JSON | `{{.tags \| jsonEncode}}` |
| `base64Encode` | Codifica em Base64 | `{{.data \| base64Encode}}` |
| `lower` | Minúsculas | `{{.name \| lower}}` |
| `upper` | Maiúsculas | `{{.code \| upper}}` |
| `trim` | Remove espaços | `{{.input \| trim}}` |
| `replace` | Substitui texto | `{{.text \| replace " " "_"}}` |
| `default` | Valor padrão | `{{.page \| default 1}}` |
| `required` | Erro se vazio | `{{.id \| required}}` |
| `now` | Data/hora atual | `{{now}}` |
| `formatDate` | Formata data | `{{now \| formatDate "2006-01-02"}}` |

### Compartilhamento: Auth e Headers

```
┌─────────────────────────────────────────────────────────────┐
│  HTTP AGENT                                                 │
│  ├─ base_url: "https://api.example.com/v2"                 │
│  ├─ auth_type: "bearer"  ─────────────┐                    │
│  ├─ auth_config: { token_env: "TOKEN" }│ COMPARTILHADO     │
│  ├─ default_headers: {                 │ entre todos       │
│  │    "Content-Type": "application/json" endpoints         │
│  │    "X-Client": "assistente"        │                    │
│  │  }  ───────────────────────────────┘                    │
│  │                                                          │
│  ├─ ENDPOINT 1: get_customer                               │
│  │   headers: {}  (usa apenas os default_headers)          │
│  │                                                          │
│  ├─ ENDPOINT 2: upload_file                                │
│  │   headers: { "Content-Type": "multipart/form-data" }    │
│  │   (MERGE: default_headers + headers do endpoint)        │
│  │   Resultado: X-Client + Content-Type override           │
│  │                                                          │
│  └─ ENDPOINT 3: webhook                                    │
│      headers: { "X-Webhook-Secret": "{{.env.SECRET}}" }    │
│      (headers também suportam templates!)                  │
└─────────────────────────────────────────────────────────────┘
```

---

## UI para HTTP Agents

### Monaco Editor com Autocomplete

Usamos **Monaco Editor** (o editor do VS Code) para edição de templates com:

1. **Autocomplete de variáveis do schema**
2. **Autocomplete de funções do template**
3. **Validação em tempo real**
4. **Syntax highlighting para Go templates**

#### Fluxo de Autocomplete

```
1. Usuário define parâmetros no Schema Builder:
   ┌──────────────────────────────────────┐
   │ customer_id  │ string  │ ✓ required │
   │ page         │ integer │ ○ optional │
   └──────────────────────────────────────┘

2. Monaco recebe o schema e configura autocomplete

3. Ao editar path_template:
   
   /customers/{{.|
                 ↓ cursor aqui
         ┌─────────────────────────┐
         │ 📦 customer_id (string) │ ← do schema
         │ 📦 page (integer)       │
         │ ─────────────────────── │
         │ 🌍 env.API_TOKEN        │ ← globais
         │ 🌍 env.APP_ENV          │
         │ 🔧 request_id           │
         │ 🔧 timestamp            │
         └─────────────────────────┘

4. Ao digitar pipe para funções:
   
   {{.customer_id | |
                    ↓
         ┌─────────────────────────────────┐
         │ urlEncode   → Escapa para URL   │
         │ upper       → MAIÚSCULAS        │
         │ lower       → minúsculas        │
         │ default     → Valor padrão      │
         │ required    → Erro se vazio     │
         └─────────────────────────────────┘
```

#### Validação em Tempo Real

```
path_template: /users/{{.user_idd}}/posts
                          ~~~~~~~~
                          ⚠️ Variável 'user_idd' não definida no schema.
                             Sugestão: 'user_id'
```

### Schema Builder Visual

Interface visual para criar schemas sem escrever JSON:

```
┌─────────────────────────────────────────────────────────────────┐
│  📋 Parâmetros do Endpoint: create_customer                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌────────────┬──────────┬───────────┬───────────────────────┐  │
│  │ Nome       │ Tipo     │ Required  │ Descrição             │  │
│  ├────────────┼──────────┼───────────┼───────────────────────┤  │
│  │ name       │ string ▼ │ ☑         │ Nome completo         │  │
│  │ email      │ string ▼ │ ☑         │ E-mail do cliente     │  │
│  │ phone      │ string ▼ │ ☐         │ Telefone (opcional)   │  │
│  │ tags       │ array  ▼ │ ☐         │ Tags do cliente       │  │
│  ├────────────┴──────────┴───────────┴───────────────────────┤  │
│  │ [+ Adicionar Parâmetro]                                   │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  📝 JSON Schema (gerado automaticamente):          [Copiar]     │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ {                                                         │  │
│  │   "type": "object",                                       │  │
│  │   "properties": {                                         │  │
│  │     "name": {"type": "string", "description": "Nome..."},│  │
│  │     "email": {"type": "string", "description": "E-ma..."}, │  │
│  │     "phone": {"type": "string", "description": "Tele..."},│  │
│  │     "tags": {"type": "array", "items": {"type": "string"}}│  │
│  │   },                                                      │  │
│  │   "required": ["name", "email"]                           │  │
│  │ }                                                         │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Tela de Configuração do HTTP Agent

```
┌─────────────────────────────────────────────────────────────────┐
│  🌐 Configurar HTTP Agent                              [Salvar] │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─ Informações Básicas ──────────────────────────────────────┐ │
│  │ Nome:         [crm_api____________]                        │ │
│  │ Display Name: [CRM API____________]                        │ │
│  │ Descrição:    [Gerencia clientes e pedidos do CRM_______]  │ │
│  │ Modelo LLM:   [gpt-4o-mini________▼]                       │ │
│  │ ☑ Habilitado                                               │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ Conexão ──────────────────────────────────────────────────┐ │
│  │ Base URL:     [https://api.crm.example.com/v2__________]   │ │
│  │               (suporta template: {{.env.API_URL}})         │ │
│  │                                                            │ │
│  │ Autenticação: [Bearer Token_______▼]                       │ │
│  │ Token:        [{{.env.CRM_TOKEN}}__] ou [*************]    │ │
│  │                                                            │ │
│  │ Timeout:      [30___] segundos                             │ │
│  │ Retries:      [3____]                                      │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ Headers Padrão ───────────────────────────────────────────┐ │
│  │ ┌──────────────────┬───────────────────────────────┐       │ │
│  │ │ Header           │ Valor                         │       │ │
│  │ ├──────────────────┼───────────────────────────────┤       │ │
│  │ │ Content-Type     │ application/json              │       │ │
│  │ │ X-Client-Id      │ assistente-app                │       │ │
│  │ │ [+ Adicionar]                                    │       │ │
│  │ └──────────────────────────────────────────────────┘       │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ Endpoints (Funções) ──────────────────────────────────────┐ │
│  │                                                            │ │
│  │  ┌─────────────────────────────────────────────────────┐   │ │
│  │  │ 📗 GET  get_customer    "Busca cliente por ID"      │   │ │
│  │  │        /customers/{{.customer_id}}                  │   │ │
│  │  └─────────────────────────────────────────────────────┘   │ │
│  │                                                            │ │
│  │  ┌─────────────────────────────────────────────────────┐   │ │
│  │  │ 📘 POST create_customer "Cria novo cliente"         │   │ │
│  │  │        /customers                                   │   │ │
│  │  └─────────────────────────────────────────────────────┘   │ │
│  │                                                            │ │
│  │  ┌─────────────────────────────────────────────────────┐   │ │
│  │  │ 📗 GET  search_customers "Busca com filtros"        │   │ │
│  │  │        /customers?q={{.query}}                      │   │ │
│  │  └─────────────────────────────────────────────────────┘   │ │
│  │                                                            │ │
│  │  [+ Novo Endpoint]                                         │ │
│  │                                                            │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Tela de Configuração de Endpoint

```
┌─────────────────────────────────────────────────────────────────┐
│  📝 Configurar Endpoint: create_customer               [Salvar] │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─ Identificação ────────────────────────────────────────────┐ │
│  │ Nome:       [create_customer_____]                         │ │
│  │ Descrição:  [Cria um novo cliente no CRM________________]  │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ Request ──────────────────────────────────────────────────┐ │
│  │ Método: [POST▼]                                            │ │
│  │                                                            │ │
│  │ Path Template:                                  [Monaco]   │ │
│  │ ┌────────────────────────────────────────────────────────┐ │ │
│  │ │ /customers                                             │ │ │
│  │ └────────────────────────────────────────────────────────┘ │ │
│  │                                                            │ │
│  │ Query Template (opcional):                      [Monaco]   │ │
│  │ ┌────────────────────────────────────────────────────────┐ │ │
│  │ │                                                        │ │ │
│  │ └────────────────────────────────────────────────────────┘ │ │
│  │                                                            │ │
│  │ Body Template:                                  [Monaco]   │ │
│  │ ┌────────────────────────────────────────────────────────┐ │ │
│  │ │ {                                                      │ │ │
│  │ │   "name": "{{.name}}",                                 │ │ │
│  │ │   "email": "{{.email}}",                               │ │ │
│  │ │   "phone": "{{.phone | default \"\"}}"                 │ │ │
│  │ │ }                                                      │ │ │
│  │ └────────────────────────────────────────────────────────┘ │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ Parâmetros (Schema Builder) ──────────────────────────────┐ │
│  │ [Interface visual do Schema Builder - ver acima]           │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ Headers Específicos (opcional) ───────────────────────────┐ │
│  │ ┌──────────────────┬───────────────────────────────┐       │ │
│  │ │ Header           │ Valor                         │       │ │
│  │ ├──────────────────┼───────────────────────────────┤       │ │
│  │ │ [+ Adicionar]                                    │       │ │
│  │ └──────────────────────────────────────────────────┘       │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ Response Template (opcional) ─────────────────────────────┐ │
│  │ ┌────────────────────────────────────────────────────────┐ │ │
│  │ │ Cliente {{.name}} criado com ID {{.id}}                │ │ │
│  │ └────────────────────────────────────────────────────────┘ │ │
│  │ (Formata a resposta da API para o LLM)                     │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ Testar Endpoint ──────────────────────────────────────────┐ │
│  │ [▶ Testar com dados de exemplo]                            │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Arquivos de Implementação

```
frontend/
├── src/
│   ├── components/
│   │   ├── HTTPAgentEditor.svelte      # Editor principal do agent
│   │   ├── EndpointEditor.svelte       # Editor de endpoint
│   │   ├── SchemaBuilder.svelte        # Builder visual de schema
│   │   ├── MonacoTemplate.svelte       # Monaco com autocomplete
│   │   ├── HeadersEditor.svelte        # Editor de headers
│   │   └── AuthConfig.svelte           # Configuração de auth
│   └── lib/
│       └── monaco-gotemplate.js        # Config do Monaco para Go templates

internal/agents/
├── http_agent.go           # Implementação do HTTPAgent
├── http_template.go        # Engine de templates com funções
├── http_executor.go        # Executor HTTP com retry/timeout
└── http_agent_test.go      # Testes
```

### Fase 4: MCP Agents ✅ Concluída
**Objetivo:** Agentes que usam Model Context Protocol.

**Tarefas:**
- [x] Cliente MCP em Go (JSON-RPC sobre stdio)
- [x] Implementar `MCPAgent` com integração ao LLM
- [x] Modelo de dados MCP no banco (mcp_agents)
- [x] Integração com o registry de agentes
- [x] UI para configurar servidores MCP
- [x] Suporte a conexão/desconexão dinâmica
- [x] Playground para testar MCP agents

**Arquivos criados:**
```
internal/agents/
├── mcp_client.go         # Cliente JSON-RPC para servidores MCP
├── mcp_agent.go          # MCPAgent implementation

database.go               # MCPAgentDB model + CRUD
app.go                    # Funções para frontend (Create/Connect/Disconnect/Test)

frontend/src/components/
├── MCPAgentEditor.svelte # Editor de MCP Agents
├── AgentManager.svelte   # Atualizado para suportar MCP Agents
```

---

## Design Detalhado: MCP Agents

### O que é MCP (Model Context Protocol)?

O MCP é um protocolo aberto da Anthropic para conectar LLMs a ferramentas e fontes de dados externas de forma padronizada. O protocolo usa JSON-RPC 2.0 sobre transporte stdio.

### Como funciona

```
┌─────────────────────────────────────────────────────────────┐
│  ASSISTENTE (Cliente MCP)                                   │
│                                                             │
│  1. Inicia servidor MCP via comando                         │
│  2. Faz handshake (initialize)                              │
│  3. Descobre ferramentas (tools/list)                       │
│  4. Executa ferramentas (tools/call)                        │
└─────────────────────┬───────────────────────────────────────┘
                      │ stdin/stdout (JSON-RPC)
                      ▼
┌─────────────────────────────────────────────────────────────┐
│  SERVIDOR MCP (processo filho)                              │
│                                                             │
│  - npx @modelcontextprotocol/server-xxx                    │
│  - python server.py                                         │
│  - Qualquer executável que implemente MCP                   │
└─────────────────────────────────────────────────────────────┘
```

### Modelo de Dados

```sql
CREATE TABLE mcp_agents (
    id INTEGER PRIMARY KEY,
    agent_config_id INTEGER NOT NULL,    -- FK para agents
    server_command TEXT NOT NULL,         -- npx, python, node, etc
    server_args TEXT,                     -- JSON array de argumentos
    server_env TEXT,                      -- JSON array de env vars
    working_dir TEXT,                     -- Diretório de trabalho
    auto_connect INTEGER DEFAULT 0,       -- Conectar ao iniciar
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY (agent_config_id) REFERENCES agents(id)
);
```

### Tipos de Transporte

| Transporte | Descrição | Quando usar |
|------------|-----------|-------------|
| `stdio` | Processo local via stdin/stdout | Servidores MCP locais (npx, python, node) |
| `http` | HTTP/SSE para servidor remoto | Servidores MCP hospedados na nuvem |

### Exemplos de Configuração

#### Servidor MCP Local (stdio) - Filesystem

```json
{
  "name": "filesystem",
  "display_name": "Acesso ao Sistema de Arquivos",
  "transport_type": "stdio",
  "server_command": "npx",
  "server_args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/dir"],
  "auto_connect": true
}
```

#### Servidor MCP Local (stdio) - Python Customizado

```json
{
  "name": "meu_mcp",
  "display_name": "Meu Servidor MCP",
  "transport_type": "stdio",
  "server_command": "python",
  "server_args": ["server.py"],
  "server_env": ["API_KEY=xxx", "DEBUG=true"],
  "working_dir": "C:\\projetos\\meu-mcp"
}
```

#### Servidor MCP Remoto (HTTP/SSE)

```json
{
  "name": "cloud_mcp",
  "display_name": "Servidor MCP na Nuvem",
  "transport_type": "http",
  "server_url": "https://mcp.example.com",
  "auth_type": "bearer",
  "auth_value": "seu_token_aqui",
  "http_headers": {"X-Client-Id": "assistente-app"},
  "auto_connect": true
}
```

#### Servidor MCP Remoto com API Key

```json
{
  "name": "api_mcp",
  "display_name": "API MCP Externa",
  "transport_type": "http",
  "server_url": "https://api.mcpservice.com/v1",
  "auth_type": "api_key",
  "auth_value": "sk-xxxxxxxx",
  "execution_mode": "convert"
}
```

### Modos de Execução

O MCPAgent suporta três modos de execução:

| Modo | Descrição | Quando usar |
|------|-----------|-------------|
| `convert` | Converte tools MCP para formato OpenAI | Padrão. Compatível com qualquer modelo. |
| `native` | Passa tools MCP no formato nativo | Para modelos com suporte nativo a MCP (Claude). |
| `passthrough` | Envia tarefa direto ao servidor MCP | Quando o servidor já tem um LLM embutido. |

#### Modo Convert (Padrão)
```
Tarefa → LLM (formato OpenAI) → Tool Call → Servidor MCP → Resultado → LLM → Resposta
```

#### Modo Native
```
Tarefa → LLM (formato MCP nativo) → Tool Call → Servidor MCP → Resultado → LLM → Resposta
```

#### Modo Passthrough
```
Tarefa → Servidor MCP (processa internamente) → Resposta
```

### Fluxo de Execução

```
1. Usuário pergunta algo que requer MCP
   └─> "Liste os arquivos no diretório docs"

2. Orquestrador delega para MCP Agent
   └─> delegate_to_filesystem(task="liste arquivos em docs")

3. MCP Agent (modo convert)
   ├─ LLM decide usar ferramenta: read_directory
   ├─ Chama servidor MCP via JSON-RPC
   ├─ Recebe resultado
   └─> Formata resposta para usuário

4. Retorna ao orquestrador
```

### Protocolo JSON-RPC (Conformidade MCP)

A implementação segue a especificação oficial do MCP:
- **Versão do protocolo**: `2024-11-05`
- **Transporte**: stdio (JSON-RPC 2.0 sobre stdin/stdout)
- **Referência**: https://spec.modelcontextprotocol.io/

#### Initialize (handshake)
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "clientInfo": { "name": "assistente-mcp-client", "version": "1.0.0" },
    "capabilities": { "tools": {} }
  }
}
```

#### Initialized (notificação - sem ID)
```json
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
```

#### List Tools
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list"
}
```

#### Call Tool
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "read_file",
    "arguments": { "path": "/docs/readme.md" }
  }
}
```

#### Funcionalidades Implementadas

| Feature | Status | Descrição |
|---------|--------|-----------|
| Handshake | ✅ | `initialize` + `notifications/initialized` |
| Tools Discovery | ✅ | `tools/list` |
| Tools Execution | ✅ | `tools/call` |
| Server Notifications | ✅ | Recebe mas ignora (log futuro) |
| Transporte stdio | ✅ | Processo local via stdin/stdout |
| Transporte HTTP/SSE | ✅ | Servidor remoto via HTTP + SSE |
| Resources | ✅ | `resources/list`, `resources/read` |
| Prompts | ✅ | `prompts/list`, `prompts/get` |
| Sampling | ✅ | `sampling/createMessage` |

### Servidores MCP Disponíveis

Alguns servidores MCP oficiais e da comunidade:

| Servidor | Comando | Descrição |
|----------|---------|-----------|
| Filesystem | `npx -y @modelcontextprotocol/server-filesystem` | Acesso a arquivos |
| GitHub | `npx -y @modelcontextprotocol/server-github` | API do GitHub |
| Google Drive | `npx -y @modelcontextprotocol/server-gdrive` | Arquivos do Google Drive |
| Slack | `npx -y @modelcontextprotocol/server-slack` | Integração com Slack |
| PostgreSQL | `npx -y @modelcontextprotocol/server-postgres` | Acesso a banco PostgreSQL |

---

## Configuração de Agentes

### FAQ Agent
```json
{
    "name": "faq",
    "display_name": "FAQ Manager",
    "model": "gpt-4o-mini",
    "system_prompt": "Você é um especialista em gerenciar FAQs. Você pode criar, buscar, listar, atualizar e deletar perguntas frequentes. Sempre retorne respostas úteis e formatadas.",
    "tools": ["faq_create", "faq_search", "faq_list", "faq_get", "faq_update", "faq_delete"]
}
```

### Memory Agent
```json
{
    "name": "memory",
    "display_name": "Memory Manager", 
    "model": "gpt-4o-mini",
    "system_prompt": "Você gerencia memórias persistentes sobre o usuário. Salve informações importantes, busque contexto relevante, e ajude a lembrar de coisas.",
    "tools": ["memory_save", "memory_search", "memory_list", "memory_delete"]
}
```

### HTTP Agent
Veja a seção **"Design Detalhado: HTTP Agents"** acima para exemplos completos de configuração com templates.

---

## System Prompts

### Orquestrador (LLM Principal)
```
Você é um assistente pessoal que coordena agentes especializados.

Você tem acesso aos seguintes agentes:
- FAQ Agent: Gerencia perguntas frequentes. Use para criar, buscar ou listar FAQs.
- Memory Agent: Gerencia memórias sobre o usuário. Use para lembrar ou buscar informações.

Quando o usuário pedir algo, delegue para o agente apropriado.
Se for uma conversa geral, responda diretamente sem delegar.

Para delegar, use: delegate_to_<agent>(task="descrição clara do que fazer")
```

### FAQ Agent
```
Você é um especialista em gerenciamento de FAQs.

Suas capacidades:
- faq_create: Criar nova FAQ
- faq_search: Buscar FAQs por termo
- faq_list: Listar todas as FAQs
- faq_get: Obter FAQ por ID
- faq_update: Atualizar FAQ existente
- faq_delete: Deletar FAQ

Receba a tarefa, execute as tools necessárias, e retorne uma resposta clara e útil.
```

---

## Testes de Agentes

### Playground na UI
```
┌─────────────────────────────────────────────────────────────┐
│  🧪 Testar Agente                                           │
├─────────────────────────────────────────────────────────────┤
│  Agente:  [FAQ Agent              ▼]                        │
│                                                             │
│  Tarefa (linguagem natural):                                │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Busque todas as FAQs sobre deploy e me diga quantas   │ │
│  │ existem                                                │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                             │
│  [▶ Executar]                                               │
├─────────────────────────────────────────────────────────────┤
│  Execução:                                                  │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ 🔧 Agente decidiu usar: faq_search("deploy")          │ │
│  │ 📊 Resultado: 8 FAQs encontradas                       │ │
│  │                                                        │ │
│  │ ✅ Resposta do Agente:                                 │ │
│  │ "Encontrei 8 FAQs sobre deploy:                        │ │
│  │  1. Como fazer deploy em produção?                     │ │
│  │  2. O que fazer se o deploy falhar?                    │ │
│  │  ..."                                                  │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

---

## Considerações

### Performance
- Cada delegação = 1 chamada LLM adicional
- Usar modelos menores/mais baratos para agentes (gpt-4o-mini)
- Cache de respostas quando possível

### Custos
- Orquestrador: modelo principal (gpt-4o)
- Agentes: modelos menores (gpt-4o-mini, gpt-3.5-turbo)
- Balancear custo vs qualidade por agente

### Segurança
- Validar inputs antes de passar para agentes
- Sanitizar outputs de HTTP agents
- Rate limiting por agente

---

## Próximos Passos

1. ~~**Revisar este documento** - Validar se atende às expectativas~~ ✅
2. ~~**Refatorar Fase 1** - Transformar agentes atuais em inteligentes~~ ✅
3. ~~**Implementar Fase 2** - UI de Gerenciamento de Agentes~~ ✅
4. ~~**Implementar Fase 3** - HTTP Agents para integração com APIs REST~~ ✅
5. ~~**Fase 4** - MCP Agents (Model Context Protocol)~~ ✅
6. ~~**Melhorias HTTP Agents:**~~ ✅
   - ~~Importar OpenAPI/Swagger para criar endpoints automaticamente~~ ✅
   - ~~Importar coleções do Postman~~ ✅
   - ~~OAuth 2.0 flow completo~~ ✅
7. ~~**Sistema de Conexões OAuth:**~~ ✅
   - ~~Providers pré-configurados (Google, Microsoft, GitHub, etc.)~~ ✅
   - ~~Fluxo Authorization Code com callback local~~ ✅
   - ~~Gerenciamento de tokens (salvar, renovar, revogar)~~ ✅
   - ~~UI dedicada para gerenciar conexões~~ ✅
8. ~~**MCP Advanced:**~~ ✅
   - ~~Resources (`resources/list`, `resources/read`)~~ ✅
   - ~~Prompts (`prompts/list`, `prompts/get`)~~ ✅
   - ~~Sampling (`sampling/createMessage`)~~ ✅
   - ~~UI para visualizar Resources e Prompts~~ ✅
9. ~~**Busca Semântica para FAQs:**~~ ✅
   - ~~Serviço de embeddings (OpenAI text-embedding-3-small)~~ ✅
   - ~~Geração automática de embeddings ao criar/atualizar FAQ~~ ✅
   - ~~Busca por similaridade de cosseno (força bruta)~~ ✅
   - ~~Fallback para busca textual se API falhar~~ ✅
10. **Melhorias futuras:**
   - Cache de respostas de agentes
11. **Iterar** - Ajustar conforme necessário

---

## Sistema de Conexões OAuth

### Visão Geral

O sistema permite conectar com serviços externos via OAuth 2.0, facilitando automações que precisam acessar APIs de terceiros como Gmail, Google Calendar, Microsoft 365, GitHub, etc.

### Providers Suportados

| Provider | Serviços Disponíveis | Env Vars |
|----------|---------------------|----------|
| Google | Gmail, Calendar, Drive, Sheets, Docs | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` |
| Microsoft | Outlook, OneDrive, Teams, SharePoint | `MICROSOFT_CLIENT_ID`, `MICROSOFT_CLIENT_SECRET` |
| GitHub | Repos, Gists, Actions, Packages | `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` |
| Slack | Channels, Messages, Files | `SLACK_CLIENT_ID`, `SLACK_CLIENT_SECRET` |
| Discord | Identify, Email, Guilds | `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET` |
| Facebook | Profile, Pages, Ads | `FACEBOOK_CLIENT_ID`, `FACEBOOK_CLIENT_SECRET` |
| LinkedIn | Profile, Connections | `LINKEDIN_CLIENT_ID`, `LINKEDIN_CLIENT_SECRET` |
| Twitter/X | Tweets, Users | `TWITTER_CLIENT_ID`, `TWITTER_CLIENT_SECRET` |
| Notion | Pages, Databases | `NOTION_CLIENT_ID`, `NOTION_CLIENT_SECRET` |
| Spotify | Playlists, Library | `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET` |
| Dropbox | Files, Folders | `DROPBOX_CLIENT_ID`, `DROPBOX_CLIENT_SECRET` |
| Atlassian | Jira, Confluence | `ATLASSIAN_CLIENT_ID`, `ATLASSIAN_CLIENT_SECRET` |
| Salesforce | CRM, APIs | `SALESFORCE_CLIENT_ID`, `SALESFORCE_CLIENT_SECRET` |
| HubSpot | CRM, Marketing | `HUBSPOT_CLIENT_ID`, `HUBSPOT_CLIENT_SECRET` |

### Fluxo de Autorização

```
1. Usuário clica "Conectar" no provider
                  │
                  ▼
2. Sistema inicia servidor HTTP local (:porta dinâmica)
                  │
                  ▼
3. Abre navegador com URL de autorização
   (com state para CSRF protection)
                  │
                  ▼
4. Usuário autoriza no site do provider
                  │
                  ▼
5. Provider redireciona para http://127.0.0.1:porta/callback
                  │
                  ▼
6. Sistema troca código por access_token + refresh_token
                  │
                  ▼
7. Busca informações do usuário (email, nome)
                  │
                  ▼
8. Salva conexão no banco de dados
```

### Configuração

1. **Criar credenciais** no console do provider (ex: Google Cloud Console)
2. **Configurar redirect URI**: `http://127.0.0.1:{porta}/callback` (porta dinâmica)
3. **Definir variáveis de ambiente** ou compilar com defaults

### Arquivos Criados

```
internal/agents/
├── oauth_providers.go    # Providers pré-configurados
├── oauth_server.go       # Servidor HTTP callback + FlowManager
├── oauth2_client.go      # Cliente OAuth2 básico

database.go              # Modelo OAuthConnection + CRUD
tools.go                 # APIs para frontend

frontend/src/components/
├── OAuthManager.svelte  # UI de gerenciamento de conexões
```

### Uso em HTTP Agents

Os HTTP Agents podem usar tokens OAuth salvos:

```go
// No HTTPExecutor
token, err := app.GetOAuthAccessTokenForProvider("google")
req.Header.Set("Authorization", "Bearer " + token)
```

O sistema renova automaticamente tokens expirados usando refresh_token.
