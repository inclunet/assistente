package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"assistente/internal/agentmanager"
)

// BuilderAgent é um agente inteligente que cria e gerencia HTTP e MCP Agents
type BuilderAgent struct {
	BaseAgent
	manager agentmanager.Manager
	reloadHTTPCallback func(agentConfigID uint) error
	reloadMCPCallback  func(mcpAgentID uint) error
}

// NewBuilderAgent cria um novo BuilderAgent inteligente
func NewBuilderAgent(manager agentmanager.Manager, llmClient LLMClient, model string) *BuilderAgent {
	if model == "" {
		model = "gpt-4o-mini"
	}

	return &BuilderAgent{
		BaseAgent: BaseAgent{
			Name:        "builder",
			DisplayName: "Agent Builder",
			Description: "🔨 Especialista em criar e gerenciar agentes HTTP e MCP. Cria agentes completos a partir de especificações OpenAPI/Postman ou instruções naturais. Gerencia todo o ciclo de vida: criação, configuração de endpoints, atualização e testes. SEMPRE lista agentes existentes antes de criar novos para evitar duplicação.",
			AgentType:   "internal",
			Model:       model,
			SystemPrompt: `Você é o **Agent Builder**, um especialista em criar e gerenciar agentes HTTP e MCP com suporte a hot reload.

## 🎯 OBJETIVO PRINCIPAL
Você é capaz de criar agentes inteligentes que conectam este sistema a APIs externas (HTTP) ou ferramentas MCP (Model Context Protocol). Cada agente que você cria se torna automaticamente disponível para o usuário conversar e usar.

## ⚡ HOT RELOAD
**IMPORTANTE**: Quando você cria, atualiza ou deleta um agente:
- O agente é automaticamente recarregado no sistema SEM REINICIAR O SERVIDOR
- As mudanças ficam disponíveis IMEDIATAMENTE para o usuário
- Isso significa que você pode criar um agente e ele já estará pronto para uso na próxima mensagem
- Endpoints criados/atualizados ficam disponíveis instantaneamente

## ⚠️ REGRAS CRÍTICAS DE VALIDAÇÃO

### 1. EVITAR DUPLICIDADE - REGRA MAIS IMPORTANTE:

**ANTES DE CRIAR QUALQUER AGENTE HTTP:**
a) Execute builder_list_http_agents para ver TODOS os agentes HTTP existentes
b) Para cada agente relevante, use builder_get_http_agent para ver TODOS os endpoints
c) Analise cuidadosamente se já existe:
   - Agente com a mesma API base (mesmo base_url)
   - Endpoint que use o mesmo caminho HTTP (mesmo path_template)
   - Endpoint com funcionalidade similar (mesmo propósito)

**ANTES DE CRIAR QUALQUER AGENTE MCP:**
a) Execute builder_list_mcp_agents para ver TODOS os agentes MCP existentes
b) Analise cuidadosamente se já existe:
   - Agente conectando ao mesmo servidor MCP (mesmo command/args ou server_url)
   - Agente com mesma finalidade ou ferramentas similares
   - Múltiplas conexões ao mesmo servidor MCP

**SE ENCONTRAR DUPLICIDADE (HTTP ou MCP):**
- NÃO crie novo agente se já existe um para a mesma API/servidor
- NÃO crie novo endpoint HTTP se já existe um igual ou similar
- Em vez disso, informe ao usuário sobre o existente e pergunte se deve:
  - Usar o existente
  - Atualizar o existente
  - Ou se realmente precisa de um novo (e por quê)

**SE FALTAR INFORMAÇÃO PARA DECIDIR:**
- NÃO ASSUMA ou INVENTE informações (URLs, comandos, argumentos, etc)
- Responda ao usuário explicando:
  - O que você encontrou (agentes/endpoints/servidores existentes)
  - Qual informação está faltando
  - Pergunte claramente o que o usuário deseja fazer

**EXEMPLOS DE RESPOSTAS QUANDO FALTA INFORMAÇÃO:**

HTTP: "Encontrei um agente 'weather_api' que já acessa a API OpenWeather com um endpoint 
'get_weather' que busca clima atual. Você quer:
1. Usar este endpoint existente
2. Adicionar um novo endpoint para previsão de 5 dias
3. Criar um agente completamente separado (preciso saber o motivo)"

MCP: "Encontrei um agente MCP 'filesystem' conectado ao servidor @modelcontextprotocol/server-filesystem. 
Você quer conectar ao mesmo servidor ou a um diferente? Se for o mesmo, já está disponível. Se for 
diferente, me informe qual servidor MCP você quer usar."

### 2. VALIDAÇÃO RIGOROSA DE JSON SCHEMA:
Os parâmetros de endpoints DEVEM ser um JSON Schema válido.

FORMATO CORRETO (exemplo):
{
  "type": "object",
  "properties": {
    "city": {"type": "string", "description": "Nome da cidade"},
    "country_code": {"type": "string", "description": "Código do país"},
    "units": {"type": "string", "enum": ["metric", "imperial"]}
  },
  "required": ["city"]
}

ERROS COMUNS A EVITAR:
- Esquecer o campo "type": "object" no nível raiz
- Usar array em vez de objeto para "properties"
- Esquecer o campo "type" dentro de cada propriedade
- Colocar "required" fora do objeto ou com formato errado

### 3. REGRAS DE NOMEAÇÃO:
- **Nome do agente** (name): snake_case, descritivo, único
  - Bom: "github_api", "openweather_api", "crm_customer_api"
  - Ruim: "api", "agent1", "minha_api"
- **Display Name**: Amigável, espaços permitidos
  - Bom: "GitHub API", "OpenWeather", "CRM Customer Management"
- **Nome de endpoint**: snake_case, ação clara
  - Bom: "get_weather", "create_user", "search_repos"
  - Ruim: "endpoint1", "api_call"

### 4. CONFIGURAÇÃO DE AUTENTICAÇÃO:
- bearer: {"token": "ENV:GITHUB_TOKEN"} ou {"token": "sk-..."}
- api-key: {"key": "ENV:API_KEY", "header": "X-API-Key"}
- basic: {"username": "user", "password": "ENV:PASSWORD"}
- none: {}

Sempre prefira usar ENV:NOME_VAR para tokens sensíveis.

## 🛠️ SUAS CAPACIDADES COMPLETAS

### HTTP Agents (APIs REST):

**Gestão de Agentes:**
- builder_create_http_agent: Cria agente HTTP (base_url, auth, headers)
- builder_get_http_agent: Obtém detalhes completos incluindo todos os endpoints
- builder_list_http_agents: Lista TODOS os agentes HTTP existentes
- builder_update_http_agent: Atualiza configurações (url, auth, timeout, etc)
- builder_delete_http_agent: Remove agente e todos seus endpoints

**Gestão de Endpoints (Tools):**
- builder_create_endpoint: Adiciona novo endpoint a um agente existente
- builder_update_endpoint: Modifica endpoint existente
- builder_delete_endpoint: Remove endpoint
- builder_test_endpoint: Testa endpoint com parâmetros reais

**Importação Automática:**
- builder_import_openapi: Cria agente completo a partir de especificação OpenAPI/Swagger
- builder_import_postman: Cria agente a partir de coleção Postman

### MCP Agents (Model Context Protocol):
- builder_create_mcp_agent: Cria agente MCP (stdio ou SSE)
- builder_list_mcp_agents: Lista todos os agentes MCP
- builder_update_mcp_agent: Atualiza configuração
- builder_delete_mcp_agent: Remove agente MCP

## 📋 FLUXO DE TRABALHO OBRIGATÓRIO

### Para Criar Novo Agente HTTP:
1. **Listar TODOS**: Execute builder_list_http_agents
2. **Analisar Duplicidade**: Para cada agente similar, use builder_get_http_agent
3. **Verificar Conflitos**:
   - Mesma API (base_url)?
   - Endpoints duplicados?
   - Funcionalidade já existe?
4. **Decidir**:
   - Se existe duplicado: PERGUNTE ao usuário o que fazer
   - Se falta info: PERGUNTE ao usuário
   - Se não existe: Prossiga com criação
5. **Criar**: Use builder_create_http_agent com configurações básicas
6. **Endpoints**: Use builder_create_endpoint para cada funcionalidade NOVA
7. **Testar**: Use builder_test_endpoint para validar
8. **Confirmar**: Informe ao usuário que está pronto

### Para Criar Novo Agente MCP:
1. **Listar TODOS**: Execute builder_list_mcp_agents
2. **Analisar Duplicidade**: Verifique se já existe conexão ao mesmo servidor
3. **Verificar Informações**:
   - Tem o nome/pacote do servidor MCP?
   - Sabe se é stdio ou SSE?
   - Tem comando e argumentos (se stdio)?
   - Tem URL do servidor (se SSE)?
4. **Decidir**:
   - Se existe duplicado: INFORME o usuário
   - Se falta info: PERGUNTE ao usuário
   - Se não existe: Prossiga com criação
5. **Criar**: Use builder_create_mcp_agent com todas as configurações
6. **Confirmar**: Informe ao usuário que está pronto e conectado

### Para Adicionar Endpoint HTTP:
1. **Obter Agente**: Use builder_get_http_agent para ver endpoints existentes
2. **Verificar Duplicidade**: Compare path_template e funcionalidade
3. **Decidir**:
   - Se endpoint igual/similar existe: INFORME o usuário e PERGUNTE
   - Se é realmente novo: Prossiga
4. **Criar**: Use builder_create_endpoint
5. **Testar**: Valide com builder_test_endpoint
6. **Confirmar**: Avise que mudança está ativa

### Para Importar OpenAPI/Postman:
1. **Listar Existentes**: Verifique agentes atuais
2. **Importar**: Use builder_import_openapi ou builder_import_postman
3. **Verificar Resultado**: Use builder_get_http_agent
4. **Analisar Duplicidade**: Compare com agentes existentes
5. **Se houver conflito**: PERGUNTE ao usuário se deve:
   - Manter ambos
   - Deletar o antigo
   - Mesclar endpoints
6. **Testar**: Valide endpoints críticos
7. **Confirmar**: Informe ao usuário

## 💡 EXEMPLOS DE SITUAÇÕES COMUNS

Exemplo 1: Usuário pede endpoint que já existe
- Usuário: "Cria um endpoint para buscar clima"
- Você: [lista agentes, encontra weather_api com get_weather]
- Resposta: "Já existe um agente OpenWeather com endpoint get_weather que busca clima por 
  cidade. Você quer usar este existente ou precisa de algo diferente? Se sim, me diga o que 
  seria diferente."

Exemplo 2: Usuário quer criar agente para API que já tem
- Usuário: "Cria um agente para a API do GitHub"
- Você: [lista agentes, encontra github_api]
- Resposta: "Já existe um agente GitHub API conectado a api.github.com. Ele tem os endpoints: 
  get_repo, list_issues, create_issue. Você quer adicionar novos endpoints neste agente ou 
  precisa de algo específico diferente?"

Exemplo 3: Falta informação crítica
- Usuário: "Cria um agente de CRM"
- Você: Não sabe qual CRM, qual URL, quais endpoints
- Resposta: "Para criar o agente CRM, preciso de algumas informações:
  1. Qual sistema CRM? (Salesforce, HubSpot, outro?)
  2. Qual a URL base da API?
  3. Que funcionalidades você precisa? (listar clientes, criar contatos, etc.)
  4. Tipo de autenticação necessária?
  Me forneça essas informações para eu criar o agente corretamente."

Exemplo 4: Importação com duplicidade
- Você: [importa OpenAPI, descobre que já existe agente similar]
- Resposta: "Importei a especificação e ela define 15 endpoints. Porém, já existe um agente 
  payment_api que acessa a mesma API (api.payment.com) com 8 endpoints. Você quer:
  1. Manter os dois agentes separados
  2. Adicionar os 7 endpoints novos ao agente existente
  3. Substituir o agente antigo por este novo completo
  Qual opção você prefere?"

Exemplo 5: Usuário pede servidor MCP que já existe
- Usuário: "Conecta ao servidor MCP do filesystem"
- Você: [lista MCPs, encontra filesystem já conectado]
- Resposta: "Já existe um agente MCP Filesystem conectado ao servidor 
  @modelcontextprotocol/server-filesystem. Ele está ativo e as ferramentas já estão disponíveis. 
  Você quer usar este existente ou precisa configurar um diferente?"

Exemplo 6: Falta informação sobre servidor MCP
- Usuário: "Cria um agente MCP para buscar dados do Brave"
- Você: Não sabe qual servidor MCP usar, comando, argumentos
- Resposta: "Para criar o agente MCP do Brave, preciso saber:
  1. Qual servidor MCP específico? (Ex: @modelcontextprotocol/server-brave-search)
  2. É stdio ou SSE?
  3. Se stdio: qual o comando? (Ex: npx, node)
  4. Quais argumentos necessários?
  5. Precisa de API key? Se sim, qual variável de ambiente?
  Me forneça essas informações ou indique onde posso encontrar a documentação do servidor."

## 🎯 COMUNICAÇÃO COM O USUÁRIO

**SEMPRE:**
- Informe sobre agentes/endpoints/servidores existentes antes de criar novos
- Pergunte quando houver dúvida ou falta de informação
- Seja específico sobre o que foi encontrado
- Apresente opções claras ao usuário
- Confirme que mudanças estão ativas (hot reload)
- Liste funcionalidades disponíveis após criação

**NUNCA:**
- Assuma informações que não tem (URLs, comandos, servidores MCP)
- Crie duplicatas sem avisar o usuário
- Invente URLs, comandos ou configurações
- Prossiga quando houver ambiguidade

## 🔍 CHECKLIST ANTES DE CRIAR

Antes de criar QUALQUER agente (HTTP ou MCP) ou endpoint, responda mentalmente:

**Para HTTP Agents:**
✓ Já listei TODOS os agentes HTTP existentes?
✓ Verifiquei os endpoints de agentes similares?
✓ Confirmei que não existe agente com mesmo base_url?
✓ Confirmei que não existe endpoint com mesmo path_template?
✓ Se existe similar, perguntei ao usuário?

**Para MCP Agents:**
✓ Já listei TODOS os agentes MCP existentes?
✓ Confirmei que não existe conexão ao mesmo servidor MCP?
✓ Tenho o nome/pacote correto do servidor MCP?
✓ Sei se é stdio ou SSE?
✓ Tenho comando/argumentos (stdio) ou URL (SSE)?
✓ Se existe similar, perguntei ao usuário?

**Para Ambos:**
✓ Tenho TODAS as informações necessárias?
✓ Se falta informação, perguntei ao usuário?
✓ O JSON Schema está correto (HTTP endpoints)?
✓ Os nomes são descritivos e únicos?

Se alguma resposta for NÃO: PARE e pergunte ao usuário.

Você tem autonomia para criar e gerenciar agentes, MAS deve sempre evitar duplicidade e pedir informações quando necessário. Seja proativo em verificar, mas conservador em assumir.`,
			Enabled: true,
			LLM:     llmClient,
		},
		manager: manager,
	}
}

// SetReloadCallbacks configura os callbacks de reload de agentes
func (a *BuilderAgent) SetReloadCallbacks(reloadHTTP func(uint) error, reloadMCP func(uint) error) {
	a.reloadHTTPCallback = reloadHTTP
	a.reloadMCPCallback = reloadMCP
}

// Execute recebe uma tarefa em linguagem natural e usa o LLM para decidir como resolver
func (a *BuilderAgent) Execute(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client não configurado para o agente %s", a.Name)
	}

	fmt.Printf("🔨 [Builder Agent] Recebeu tarefa: %s\n", task)

	var result string
	var err error
	if a.MessageSaver != nil {
		result, err = a.LLM.ChatWithToolsAndSaver(
			ctx,
			a.Model,
			a.SystemPrompt,
			task,
			a.GetTools(),
			a.ExecuteTool,
			a.Name,
			a.MessageSaver,
		)
	} else {
		result, err = a.LLM.ChatWithTools(
			ctx,
			a.Model,
			a.SystemPrompt,
			task,
			a.GetTools(),
			a.ExecuteTool,
		)
	}

	if err != nil {
		return "", fmt.Errorf("erro na execução do Builder Agent: %w", err)
	}

	return result, nil
}

// CanHandle verifica se o agente pode executar uma tool
// BuilderAgent não manipula tools externas diretamente
func (a *BuilderAgent) CanHandle(toolName string) bool {
	return false
}

// GetTools retorna as ferramentas disponíveis para o agente
func (a *BuilderAgent) GetTools() []Tool {
	return []Tool{
		// HTTP Agent CRUD
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_create_http_agent",
				Description: "Cria um novo agente HTTP. ATENÇÃO: Use builder_list_http_agents PRIMEIRO para verificar se já existe agente para a mesma API (mesmo base_url). Se existir, adicione endpoints ao agente existente em vez de criar duplicado.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Nome interno do agente (snake_case, ex: github_api)",
						},
						"display_name": map[string]interface{}{
							"type":        "string",
							"description": "Nome de exibição (ex: GitHub API)",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Descrição do que o agente faz",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Modelo LLM (padrão: gpt-4o-mini)",
						},
						"base_url": map[string]interface{}{
							"type":        "string",
							"description": "URL base da API (ex: https://api.github.com)",
						},
						"auth_type": map[string]interface{}{
							"type":        "string",
							"description": "Tipo de auth: bearer, api-key, basic, none",
							"enum":        []string{"bearer", "api-key", "basic", "none"},
						},
						"auth_config": map[string]interface{}{
							"type":        "string",
							"description": "JSON com config de auth (ex: {\"token\": \"ENV:TOKEN\"})",
						},
						"default_headers": map[string]interface{}{
							"type":        "string",
							"description": "JSON com headers padrão",
						},
						"timeout_seconds": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout em segundos (padrão: 30)",
						},
						"retry_count": map[string]interface{}{
							"type":        "integer",
							"description": "Tentativas em erro (padrão: 3)",
						},
					},
					"required": []string{"name", "display_name", "base_url"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_get_http_agent",
				Description: "Obtém detalhes completos de um agente HTTP incluindo endpoints",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do agente HTTP",
						},
					},
					"required": []string{"agent_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_list_http_agents",
				Description: "Lista todos os agentes HTTP cadastrados",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_update_http_agent",
				Description: "Atualiza configurações de um agente HTTP existente",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do agente HTTP",
						},
						"display_name": map[string]interface{}{
							"type":        "string",
							"description": "Novo nome de exibição",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Nova descrição",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Novo modelo LLM",
						},
						"base_url": map[string]interface{}{
							"type":        "string",
							"description": "Nova URL base",
						},
						"auth_type": map[string]interface{}{
							"type":        "string",
							"description": "Novo tipo de auth",
						},
						"auth_config": map[string]interface{}{
							"type":        "string",
							"description": "Nova config de auth (JSON)",
						},
						"default_headers": map[string]interface{}{
							"type":        "string",
							"description": "Novos headers padrão (JSON)",
						},
						"timeout_seconds": map[string]interface{}{
							"type":        "integer",
							"description": "Novo timeout",
						},
						"retry_count": map[string]interface{}{
							"type":        "integer",
							"description": "Novo retry count",
						},
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "Habilitar/desabilitar",
						},
					},
					"required": []string{"agent_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_delete_http_agent",
				Description: "Deleta um agente HTTP",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do agente HTTP",
						},
					},
					"required": []string{"agent_id"},
				},
			},
		},

		// HTTP Endpoints
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_create_endpoint",
				Description: "Cria um endpoint em agente HTTP existente. CRÍTICO: Use builder_get_http_agent PRIMEIRO para ver endpoints existentes e evitar duplicidade. NÃO crie endpoint se já existe um com mesmo path_template ou funcionalidade similar - pergunte ao usuário.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do agente HTTP",
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Nome do endpoint (ex: get_user)",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Descrição do que o endpoint faz",
						},
						"method": map[string]interface{}{
							"type":        "string",
							"description": "Método HTTP (GET, POST, PUT, DELETE, PATCH)",
							"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
						},
						"path_template": map[string]interface{}{
							"type":        "string",
							"description": "Template do path (ex: /users/{{.user_id}})",
						},
						"query_template": map[string]interface{}{
							"type":        "string",
							"description": "Template de query params (ex: page={{.page}})",
						},
						"headers_json": map[string]interface{}{
							"type":        "string",
							"description": "Headers específicos (JSON)",
						},
						"body_template": map[string]interface{}{
							"type":        "string",
							"description": "Template do body para POST/PUT",
						},
						"parameters": map[string]interface{}{
							"type":        "string",
							"description": "JSON Schema dos parâmetros",
						},
						"response_template": map[string]interface{}{
							"type":        "string",
							"description": "Template para formatar resposta",
						},
					},
					"required": []string{"agent_id", "name", "method", "path_template"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_update_endpoint",
				Description: "Atualiza um endpoint existente",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"endpoint_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do endpoint",
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Novo nome",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Nova descrição",
						},
						"method": map[string]interface{}{
							"type":        "string",
							"description": "Novo método HTTP",
						},
						"path_template": map[string]interface{}{
							"type":        "string",
							"description": "Novo path template",
						},
						"query_template": map[string]interface{}{
							"type":        "string",
							"description": "Novo query template",
						},
						"headers_json": map[string]interface{}{
							"type":        "string",
							"description": "Novos headers",
						},
						"body_template": map[string]interface{}{
							"type":        "string",
							"description": "Novo body template",
						},
						"parameters": map[string]interface{}{
							"type":        "string",
							"description": "Novo JSON Schema",
						},
						"response_template": map[string]interface{}{
							"type":        "string",
							"description": "Novo response template",
						},
					},
					"required": []string{"endpoint_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_delete_endpoint",
				Description: "Deleta um endpoint",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"endpoint_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do endpoint",
						},
					},
					"required": []string{"endpoint_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_test_endpoint",
				Description: "Testa um endpoint com parâmetros",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do agente HTTP",
						},
						"endpoint_name": map[string]interface{}{
							"type":        "string",
							"description": "Nome do endpoint a testar",
						},
						"params_json": map[string]interface{}{
							"type":        "string",
							"description": "JSON com parâmetros (ex: {\"user_id\": 123})",
						},
					},
					"required": []string{"agent_id", "endpoint_name"},
				},
			},
		},

		// Import
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_import_openapi",
				Description: "Importa agente completo de especificação OpenAPI (YAML ou JSON)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Conteúdo da especificação OpenAPI (YAML ou JSON)",
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Nome para o agente",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Modelo LLM (padrão: gpt-4o-mini)",
						},
					},
					"required": []string{"content", "name"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_import_postman",
				Description: "Importa agente de coleção Postman (JSON)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]interface{}{
							"type":        "string",
							"description": "JSON da coleção Postman",
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Nome para o agente",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Modelo LLM (padrão: gpt-4o-mini)",
						},
					},
					"required": []string{"content", "name"},
				},
			},
		},

		// MCP Agents
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_create_mcp_agent",
				Description: "Cria um agente MCP (stdio ou SSE). ATENÇÃO: Use builder_list_mcp_agents PRIMEIRO para verificar se já existe conexão ao mesmo servidor MCP. Não crie múltiplas conexões ao mesmo servidor sem consultar o usuário. Se faltar informação (comando, URL, args), pergunte ao usuário em vez de assumir.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Nome interno (snake_case)",
						},
						"display_name": map[string]interface{}{
							"type":        "string",
							"description": "Nome de exibição",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Descrição",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Modelo LLM",
						},
						"transport_type": map[string]interface{}{
							"type":        "string",
							"description": "Tipo de transporte: stdio ou http",
							"enum":        []string{"stdio", "http"},
						},
						"server_command": map[string]interface{}{
							"type":        "string",
							"description": "Comando do servidor (para stdio)",
						},
						"server_args": map[string]interface{}{
							"type":        "string",
							"description": "Argumentos do servidor (JSON array)",
						},
						"server_env": map[string]interface{}{
							"type":        "string",
							"description": "Variáveis de ambiente (JSON)",
						},
						"working_dir": map[string]interface{}{
							"type":        "string",
							"description": "Diretório de trabalho",
						},
						"server_url": map[string]interface{}{
							"type":        "string",
							"description": "URL do servidor (para http)",
						},
						"auth_type": map[string]interface{}{
							"type":        "string",
							"description": "Tipo de auth para http",
						},
						"auth_value": map[string]interface{}{
							"type":        "string",
							"description": "Valor de auth",
						},
						"http_headers": map[string]interface{}{
							"type":        "string",
							"description": "Headers HTTP (JSON)",
						},
						"auto_connect": map[string]interface{}{
							"type":        "boolean",
							"description": "Conectar automaticamente",
						},
					},
					"required": []string{"name", "display_name", "transport_type"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_list_mcp_agents",
				Description: "Lista todos os agentes MCP cadastrados",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_update_mcp_agent",
				Description: "Atualiza configurações de um agente MCP",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do agente MCP",
						},
						"display_name": map[string]interface{}{
							"type":        "string",
							"description": "Novo nome de exibição",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Nova descrição",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Novo modelo LLM",
						},
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "Habilitar/desabilitar",
						},
						"server_command": map[string]interface{}{
							"type":        "string",
							"description": "Novo comando (stdio)",
						},
						"server_args": map[string]interface{}{
							"type":        "string",
							"description": "Novos argumentos",
						},
						"server_env": map[string]interface{}{
							"type":        "string",
							"description": "Novas variáveis de ambiente",
						},
						"server_url": map[string]interface{}{
							"type":        "string",
							"description": "Nova URL (http)",
						},
						"auth_type": map[string]interface{}{
							"type":        "string",
							"description": "Novo tipo de auth",
						},
						"auth_value": map[string]interface{}{
							"type":        "string",
							"description": "Novo valor de auth",
						},
						"http_headers": map[string]interface{}{
							"type":        "string",
							"description": "Novos headers",
						},
						"auto_connect": map[string]interface{}{
							"type":        "boolean",
							"description": "Auto conectar",
						},
					},
					"required": []string{"agent_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "builder_delete_mcp_agent",
				Description: "Deleta um agente MCP",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID do agente MCP",
						},
					},
					"required": []string{"agent_id"},
				},
			},
		},
	}
}

// ExecuteTool executa uma ferramenta específica
func (a *BuilderAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %w", err)
	}

	toolName := toolCall.Function.Name

	switch toolName {
	// HTTP Agent CRUD
	case "builder_create_http_agent":
		return a.createHTTPAgent(args)
	case "builder_get_http_agent":
		return a.getHTTPAgent(args)
	case "builder_list_http_agents":
		return a.listHTTPAgents(args)
	case "builder_update_http_agent":
		return a.updateHTTPAgent(args)
	case "builder_delete_http_agent":
		return a.deleteHTTPAgent(args)

	// HTTP Endpoints
	case "builder_create_endpoint":
		return a.createEndpoint(args)
	case "builder_update_endpoint":
		return a.updateEndpoint(args)
	case "builder_delete_endpoint":
		return a.deleteEndpoint(args)
	case "builder_test_endpoint":
		return a.testEndpoint(args)

	// Import
	case "builder_import_openapi":
		return a.importOpenAPI(args)
	case "builder_import_postman":
		return a.importPostman(args)

	// MCP Agents
	case "builder_create_mcp_agent":
		return a.createMCPAgent(args)
	case "builder_list_mcp_agents":
		return a.listMCPAgents(args)
	case "builder_update_mcp_agent":
		return a.updateMCPAgent(args)
	case "builder_delete_mcp_agent":
		return a.deleteMCPAgent(args)

	default:
		return "", fmt.Errorf("ferramenta desconhecida: %s", toolName)
	}
}

// ==================== HTTP Agent CRUD ====================

func (a *BuilderAgent) createHTTPAgent(args map[string]interface{}) (string, error) {
	name := getStringArg(args, "name", "")
	displayName := getStringArg(args, "display_name", "")
	description := getStringArg(args, "description", "")
	model := getStringArg(args, "model", "gpt-4o-mini")
	baseURL := getStringArg(args, "base_url", "")
	authType := getStringArg(args, "auth_type", "none")
	authConfig := getStringArg(args, "auth_config", "")
	defaultHeaders := getStringArg(args, "default_headers", "")
	timeoutSeconds := getIntArg(args, "timeout_seconds", 30)
	retryCount := getIntArg(args, "retry_count", 3)

	if name == "" || displayName == "" || baseURL == "" {
		return "", fmt.Errorf("name, display_name e base_url são obrigatórios")
	}

	systemPrompt := fmt.Sprintf("Você é um agente HTTP que acessa %s. Use os endpoints disponíveis para ajudar o usuário.", displayName)

	req := agentmanager.CreateHTTPAgentRequest{
		Name:           name,
		DisplayName:    displayName,
		Description:    description,
		Model:          model,
		SystemPrompt:   systemPrompt,
		Enabled:        true,
		BaseURL:        baseURL,
		AuthType:       authType,
		AuthConfig:     authConfig,
		DefaultHeaders: defaultHeaders,
		TimeoutSeconds: timeoutSeconds,
		RetryCount:     retryCount,
	}

	agentData, httpData, err := a.manager.CreateHTTPAgent(req)
	if err != nil {
		return "", fmt.Errorf("erro ao criar agente: %w", err)
	}

	// Hot reload: registrar o agente no registry se habilitado
	if req.Enabled && a.reloadHTTPCallback != nil {
		if err := a.reloadHTTPCallback(agentData.ID); err != nil {
			return "", fmt.Errorf("agente criado mas falhou ao registrar no registry: %w", err)
		}
	}

	result := fmt.Sprintf(`✅ Agente HTTP criado com sucesso!

**ID do Agente**: %d
**ID HTTP Agent**: %d (use este para criar endpoints)
**Nome**: %s
**Display Name**: %s
**Base URL**: %s
**Auth Type**: %s

O agente está pronto. Adicione endpoints com builder_create_endpoint.`,
		agentData.ID, httpData.ID, name, displayName, baseURL, authType)

	return result, nil
}

func (a *BuilderAgent) getHTTPAgent(args map[string]interface{}) (string, error) {
	agentID := getIntArg(args, "agent_id", 0)
	if agentID == 0 {
		return "", fmt.Errorf("agent_id é obrigatório")
	}

	agentData, err := a.manager.GetAgentByID(uint(agentID))
	if err != nil {
		return "", fmt.Errorf("erro ao buscar agente: %w", err)
	}

	httpData, err := a.manager.GetHTTPAgent(uint(agentID))
	if err != nil {
		return "", fmt.Errorf("erro ao buscar dados HTTP: %w", err)
	}

	result := fmt.Sprintf(`📋 **%s** (ID: %d)
**Tipo**: HTTP Agent
**Descrição**: %s
**Base URL**: %s
**Auth**: %s
**Habilitado**: %v
**HTTP Agent ID**: %d
**Endpoints** (%d):`,
		agentData.DisplayName, agentData.ID, agentData.Description,
		httpData.BaseURL, httpData.AuthType, agentData.Enabled,
		httpData.ID, len(httpData.Endpoints))

	if len(httpData.Endpoints) == 0 {
		result += "\n(Nenhum endpoint cadastrado)"
	} else {
		for _, ep := range httpData.Endpoints {
			result += fmt.Sprintf("\n- **%s** (%s %s) - %s",
				ep.Name, ep.Method, ep.PathTemplate, ep.Description)
		}
	}

	return result, nil
}

func (a *BuilderAgent) listHTTPAgents(args map[string]interface{}) (string, error) {
	agents, err := a.manager.GetAllAgents()
	if err != nil {
		return "", fmt.Errorf("erro ao listar agentes: %w", err)
	}

	httpAgents := []agentmanager.AgentData{}
	for _, agent := range agents {
		if agent.AgentType == "http" {
			httpAgents = append(httpAgents, agent)
		}
	}

	if len(httpAgents) == 0 {
		return "Nenhum agente HTTP cadastrado.", nil
	}

	result := fmt.Sprintf("📋 **%d Agentes HTTP**:\n\n", len(httpAgents))
	for _, agent := range httpAgents {
		status := "✅"
		if !agent.Enabled {
			status = "❌"
		}
		result += fmt.Sprintf("%s **%s** (ID: %d)\n   %s\n\n", status, agent.DisplayName, agent.ID, agent.Description)
	}

	return result, nil
}

func (a *BuilderAgent) updateHTTPAgent(args map[string]interface{}) (string, error) {
	agentID := getIntArg(args, "agent_id", 0)
	if agentID == 0 {
		return "", fmt.Errorf("agent_id é obrigatório")
	}

	current, err := a.manager.GetAgentByID(uint(agentID))
	if err != nil {
		return "", fmt.Errorf("agente não encontrado: %w", err)
	}

	httpCurrent, _ := a.manager.GetHTTPAgent(uint(agentID))

	req := agentmanager.UpdateAgentRequest{
		DisplayName:    getStringArg(args, "display_name", current.DisplayName),
		Description:    getStringArg(args, "description", current.Description),
		Model:          getStringArg(args, "model", current.Model),
		SystemPrompt:   current.SystemPrompt,
		Enabled:        getBoolArg(args, "enabled", current.Enabled),
		BaseURL:        getStringArg(args, "base_url", httpCurrent.BaseURL),
		AuthType:       getStringArg(args, "auth_type", httpCurrent.AuthType),
		AuthConfig:     getStringArg(args, "auth_config", httpCurrent.AuthConfig),
		DefaultHeaders: getStringArg(args, "default_headers", httpCurrent.DefaultHeaders),
		TimeoutSeconds: getIntArg(args, "timeout_seconds", httpCurrent.TimeoutSeconds),
		RetryCount:     getIntArg(args, "retry_count", httpCurrent.RetryCount),
	}

	agentData, err := a.manager.UpdateAgent(uint(agentID), req)
	if err != nil {
		return "", fmt.Errorf("erro ao atualizar agente: %w", err)
	}

	// Hot reload: atualizar agente no registry
	if a.reloadHTTPCallback != nil {
		if err := a.reloadHTTPCallback(agentData.ID); err != nil {
			return "", fmt.Errorf("agente atualizado mas falhou ao recarregar no registry: %w", err)
		}
	}

	return fmt.Sprintf("✅ Agente **%s** (ID: %d) atualizado com sucesso!", agentData.DisplayName, agentData.ID), nil
}

func (a *BuilderAgent) deleteHTTPAgent(args map[string]interface{}) (string, error) {
	agentID := getIntArg(args, "agent_id", 0)
	if agentID == 0 {
		return "", fmt.Errorf("agent_id é obrigatório")
	}

	agent, _ := a.manager.GetAgentByID(uint(agentID))
	name := "Agente"
	if agent != nil {
		name = agent.DisplayName
	}

	if err := a.manager.DeleteAgent(uint(agentID)); err != nil {
		return "", fmt.Errorf("erro ao deletar agente: %w", err)
	}

	return fmt.Sprintf("✅ Agente **%s** (ID: %d) deletado com sucesso!", name, agentID), nil
}

// ==================== HTTP Endpoints ====================

func (a *BuilderAgent) createEndpoint(args map[string]interface{}) (string, error) {
	agentID := getIntArg(args, "agent_id", 0)
	name := getStringArg(args, "name", "")
	method := getStringArg(args, "method", "GET")
	pathTemplate := getStringArg(args, "path_template", "")
	parameters := getStringArg(args, "parameters", "")

	if agentID == 0 || name == "" || pathTemplate == "" {
		return "", fmt.Errorf("agent_id, name e path_template são obrigatórios")
	}

	// Validar JSON Schema dos parâmetros
	if err := agentmanager.ValidateJSONSchema(parameters); err != nil {
		return "", fmt.Errorf("❌ Schema de parâmetros inválido: %w\n\nExemplo de schema válido:\n{\n  \"type\": \"object\",\n  \"properties\": {\n    \"param_name\": {\"type\": \"string\", \"description\": \"Descrição\"}\n  },\n  \"required\": [\"param_name\"]\n}", err)
	}

	req := agentmanager.CreateEndpointRequest{
		Name:             name,
		Description:      getStringArg(args, "description", ""),
		Method:           method,
		PathTemplate:     pathTemplate,
		QueryTemplate:    getStringArg(args, "query_template", ""),
		HeadersJSON:      getStringArg(args, "headers_json", ""),
		BodyTemplate:     getStringArg(args, "body_template", ""),
		Parameters:       parameters,
		ResponseTemplate: getStringArg(args, "response_template", ""),
	}

	endpointData, err := a.manager.CreateHTTPEndpoint(uint(agentID), req)
	if err != nil {
		return "", fmt.Errorf("erro ao criar endpoint: %w", err)
	}

	result := fmt.Sprintf(`✅ Endpoint criado com sucesso!

**ID**: %d
**Nome**: %s
**Método**: %s %s
**Descrição**: %s

O endpoint está pronto para uso no agente HTTP.`,
		endpointData.ID, name, method, pathTemplate, endpointData.Description)

	return result, nil
}

func (a *BuilderAgent) updateEndpoint(args map[string]interface{}) (string, error) {
	endpointID := getIntArg(args, "endpoint_id", 0)
	if endpointID == 0 {
		return "", fmt.Errorf("endpoint_id é obrigatório")
	}

	current, err := a.manager.GetHTTPEndpoint(uint(endpointID))
	if err != nil {
		return "", fmt.Errorf("endpoint não encontrado: %w", err)
	}

	// Validar JSON Schema se fornecido
	parameters := getStringArg(args, "parameters", current.Parameters)
	if parameters != current.Parameters {
		if err := agentmanager.ValidateJSONSchema(parameters); err != nil {
			return "", fmt.Errorf("❌ Schema de parâmetros inválido: %w\n\nExemplo de schema válido:\n{\n  \"type\": \"object\",\n  \"properties\": {\n    \"param_name\": {\"type\": \"string\", \"description\": \"Descrição\"}\n  },\n  \"required\": [\"param_name\"]\n}", err)
		}
	}

	req := agentmanager.CreateEndpointRequest{
		Name:             getStringArg(args, "name", current.Name),
		Description:      getStringArg(args, "description", current.Description),
		Method:           getStringArg(args, "method", current.Method),
		PathTemplate:     getStringArg(args, "path_template", current.PathTemplate),
		QueryTemplate:    getStringArg(args, "query_template", current.QueryTemplate),
		HeadersJSON:      getStringArg(args, "headers_json", current.HeadersJSON),
		BodyTemplate:     getStringArg(args, "body_template", current.BodyTemplate),
		Parameters:       parameters,
		ResponseTemplate: getStringArg(args, "response_template", current.ResponseTemplate),
	}

	endpointData, err := a.manager.UpdateHTTPEndpoint(uint(endpointID), req)
	if err != nil {
		return "", fmt.Errorf("erro ao atualizar endpoint: %w", err)
	}

	return fmt.Sprintf("✅ Endpoint **%s** (ID: %d) atualizado com sucesso!", endpointData.Name, endpointData.ID), nil
}

func (a *BuilderAgent) deleteEndpoint(args map[string]interface{}) (string, error) {
	endpointID := getIntArg(args, "endpoint_id", 0)
	if endpointID == 0 {
		return "", fmt.Errorf("endpoint_id é obrigatório")
	}

	endpoint, _ := a.manager.GetHTTPEndpoint(uint(endpointID))
	name := "Endpoint"
	if endpoint != nil {
		name = endpoint.Name
	}

	if err := a.manager.DeleteHTTPEndpoint(uint(endpointID)); err != nil {
		return "", fmt.Errorf("erro ao deletar endpoint: %w", err)
	}

	return fmt.Sprintf("✅ Endpoint **%s** (ID: %d) deletado com sucesso!", name, endpointID), nil
}

func (a *BuilderAgent) testEndpoint(args map[string]interface{}) (string, error) {
	agentID := getIntArg(args, "agent_id", 0)
	endpointName := getStringArg(args, "endpoint_name", "")
	paramsJSON := getStringArg(args, "params_json", "{}")

	if agentID == 0 || endpointName == "" {
		return "", fmt.Errorf("agent_id e endpoint_name são obrigatórios")
	}

	result, err := a.manager.TestHTTPEndpoint(uint(agentID), endpointName, paramsJSON)
	if err != nil {
		return "", fmt.Errorf("erro ao testar endpoint: %w", err)
	}

	return fmt.Sprintf(`✅ **Teste do Endpoint: %s**

**Resultado**:
%s`, endpointName, result), nil
}

// ==================== Import ====================

func (a *BuilderAgent) importOpenAPI(args map[string]interface{}) (string, error) {
	content := getStringArg(args, "content", "")
	name := getStringArg(args, "name", "")
	model := getStringArg(args, "model", "gpt-4o-mini")

	if content == "" || name == "" {
		return "", fmt.Errorf("content e name são obrigatórios")
	}

	agentData, httpData, err := a.manager.ImportOpenAPIAgent(content, name, model)
	if err != nil {
		return "", fmt.Errorf("erro ao importar OpenAPI: %w", err)
	}

	result := fmt.Sprintf(`✅ **Agente importado do OpenAPI com sucesso!**

**ID do Agente**: %d
**HTTP Agent ID**: %d
**Nome**: %s
**Display Name**: %s
**Base URL**: %s
**Endpoints criados**: %d

O agente está pronto para uso!`,
		agentData.ID, httpData.ID, agentData.Name, agentData.DisplayName,
		httpData.BaseURL, len(httpData.Endpoints))

	if len(httpData.Endpoints) > 0 {
		result += "\n\n**Endpoints**:"
		for i, ep := range httpData.Endpoints {
			if i < 10 {
				result += fmt.Sprintf("\n- %s %s", ep.Method, ep.PathTemplate)
			}
		}
		if len(httpData.Endpoints) > 10 {
			result += fmt.Sprintf("\n- ... e mais %d endpoints", len(httpData.Endpoints)-10)
		}
	}

	return result, nil
}

func (a *BuilderAgent) importPostman(args map[string]interface{}) (string, error) {
	content := getStringArg(args, "content", "")
	name := getStringArg(args, "name", "")
	model := getStringArg(args, "model", "gpt-4o-mini")

	if content == "" || name == "" {
		return "", fmt.Errorf("content e name são obrigatórios")
	}

	agentData, httpData, err := a.manager.ImportPostmanAgent(content, name, model)
	if err != nil {
		return "", fmt.Errorf("erro ao importar Postman: %w", err)
	}

	result := fmt.Sprintf(`✅ **Agente importado do Postman com sucesso!**

**ID do Agente**: %d
**HTTP Agent ID**: %d
**Nome**: %s
**Display Name**: %s
**Base URL**: %s
**Endpoints criados**: %d

O agente está pronto para uso!`,
		agentData.ID, httpData.ID, agentData.Name, agentData.DisplayName,
		httpData.BaseURL, len(httpData.Endpoints))

	if len(httpData.Endpoints) > 0 {
		result += "\n\n**Endpoints**:"
		for i, ep := range httpData.Endpoints {
			if i < 10 {
				result += fmt.Sprintf("\n- %s %s", ep.Method, ep.PathTemplate)
			}
		}
		if len(httpData.Endpoints) > 10 {
			result += fmt.Sprintf("\n- ... e mais %d endpoints", len(httpData.Endpoints)-10)
		}
	}

	return result, nil
}

// ==================== MCP Agents ====================

func (a *BuilderAgent) createMCPAgent(args map[string]interface{}) (string, error) {
	name := getStringArg(args, "name", "")
	displayName := getStringArg(args, "display_name", "")
	transportType := getStringArg(args, "transport_type", "")

	if name == "" || displayName == "" || transportType == "" {
		return "", fmt.Errorf("name, display_name e transport_type são obrigatórios")
	}

	description := getStringArg(args, "description", "")
	model := getStringArg(args, "model", "gpt-4o-mini")
	systemPrompt := fmt.Sprintf("Você é um agente MCP que conecta com %s. Use as ferramentas disponíveis para ajudar o usuário.", displayName)

	req := agentmanager.CreateMCPAgentRequest{
		Name:          name,
		DisplayName:   displayName,
		Description:   description,
		Model:         model,
		SystemPrompt:  systemPrompt,
		Enabled:       true,
		TransportType: transportType,
		ServerCommand: getStringArg(args, "server_command", ""),
		ServerArgs:    getStringArg(args, "server_args", ""),
		ServerEnv:     getStringArg(args, "server_env", ""),
		WorkingDir:    getStringArg(args, "working_dir", ""),
		ServerURL:     getStringArg(args, "server_url", ""),
		AuthType:      getStringArg(args, "auth_type", ""),
		AuthValue:     getStringArg(args, "auth_value", ""),
		HTTPHeaders:   getStringArg(args, "http_headers", ""),
		ExecutionMode: "sync",
		AutoConnect:   getBoolArg(args, "auto_connect", false),
	}

	agentData, mcpData, err := a.manager.CreateMCPAgent(req)
	if err != nil {
		return "", fmt.Errorf("erro ao criar agente MCP: %w", err)
	}

	// Hot reload: registrar o agente no registry se auto_connect está habilitado
	if req.AutoConnect && a.reloadMCPCallback != nil {
		if err := a.reloadMCPCallback(mcpData.ID); err != nil {
			return "", fmt.Errorf("agente criado mas falhou ao registrar no registry: %w", err)
		}
	}

	result := fmt.Sprintf(`✅ **Agente MCP criado com sucesso!**

**ID do Agente**: %d
**MCP Agent ID**: %d
**Nome**: %s
**Display Name**: %s
**Transport**: %s`,
		agentData.ID, mcpData.ID, name, displayName, transportType)

	if transportType == "stdio" {
		result += fmt.Sprintf("\n**Comando**: %s", mcpData.ServerCommand)
	} else {
		result += fmt.Sprintf("\n**URL**: %s", mcpData.ServerURL)
	}

	result += "\n\nO agente está pronto para uso!"
	return result, nil
}

func (a *BuilderAgent) listMCPAgents(args map[string]interface{}) (string, error) {
	agents, err := a.manager.GetAllMCPAgents()
	if err != nil {
		return "", fmt.Errorf("erro ao listar agentes MCP: %w", err)
	}

	if len(agents) == 0 {
		return "Nenhum agente MCP cadastrado.", nil
	}

	result := fmt.Sprintf("📋 **%d Agentes MCP**:\n\n", len(agents))
	for _, mcp := range agents {
		agentData, _ := a.manager.GetAgentByID(mcp.AgentConfigID)
		status := "✅"
		if agentData != nil && !agentData.Enabled {
			status = "❌"
		}

		result += fmt.Sprintf("%s **%s** (ID Config: %d, MCP ID: %d)\n", status, mcp.TransportType, mcp.AgentConfigID, mcp.ID)
		if mcp.TransportType == "stdio" {
			result += fmt.Sprintf("   Command: %s\n", mcp.ServerCommand)
		} else {
			result += fmt.Sprintf("   URL: %s\n", mcp.ServerURL)
		}
		result += "\n"
	}

	return result, nil
}

func (a *BuilderAgent) updateMCPAgent(args map[string]interface{}) (string, error) {
	agentID := getIntArg(args, "agent_id", 0)
	if agentID == 0 {
		return "", fmt.Errorf("agent_id é obrigatório")
	}

	current, err := a.manager.GetAgentByID(uint(agentID))
	if err != nil {
		return "", fmt.Errorf("agente não encontrado: %w", err)
	}

	mcpCurrent, _ := a.manager.GetMCPAgent(uint(agentID))

	req := agentmanager.UpdateAgentRequest{
		DisplayName:  getStringArg(args, "display_name", current.DisplayName),
		Description:  getStringArg(args, "description", current.Description),
		Model:        getStringArg(args, "model", current.Model),
		SystemPrompt: current.SystemPrompt,
		Enabled:      getBoolArg(args, "enabled", current.Enabled),
	}

	agentData, err := a.manager.UpdateAgent(uint(agentID), req)
	if err != nil {
		return "", fmt.Errorf("erro ao atualizar agente: %w", err)
	}

	if mcpCurrent != nil {
		mcpReq := agentmanager.CreateMCPAgentRequest{
			Name:          current.Name,
			DisplayName:   req.DisplayName,
			Description:   req.Description,
			Model:         req.Model,
			SystemPrompt:  req.SystemPrompt,
			Enabled:       req.Enabled,
			TransportType: mcpCurrent.TransportType,
			ServerCommand: getStringArg(args, "server_command", mcpCurrent.ServerCommand),
			ServerArgs:    getStringArg(args, "server_args", mcpCurrent.ServerArgs),
			ServerEnv:     getStringArg(args, "server_env", mcpCurrent.ServerEnv),
			WorkingDir:    getStringArg(args, "working_dir", mcpCurrent.WorkingDir),
			ServerURL:     getStringArg(args, "server_url", mcpCurrent.ServerURL),
			AuthType:      getStringArg(args, "auth_type", mcpCurrent.AuthType),
			AuthValue:     getStringArg(args, "auth_value", mcpCurrent.AuthValue),
			HTTPHeaders:   getStringArg(args, "http_headers", mcpCurrent.HTTPHeaders),
			ExecutionMode: mcpCurrent.ExecutionMode,
			AutoConnect:   getBoolArg(args, "auto_connect", mcpCurrent.AutoConnect),
		}
		a.manager.UpdateMCPAgent(mcpCurrent.ID, mcpReq)
	}

	// Hot reload: atualizar agente no registry
	if a.reloadMCPCallback != nil {
		if mcpCurrent != nil {
			if err := a.reloadMCPCallback(mcpCurrent.ID); err != nil {
				return "", fmt.Errorf("agente atualizado mas falhou ao recarregar no registry: %w", err)
			}
		}
	}

	return fmt.Sprintf("✅ Agente MCP **%s** (ID: %d) atualizado com sucesso!", agentData.DisplayName, agentData.ID), nil
}

func (a *BuilderAgent) deleteMCPAgent(args map[string]interface{}) (string, error) {
	agentID := getIntArg(args, "agent_id", 0)
	if agentID == 0 {
		return "", fmt.Errorf("agent_id é obrigatório")
	}

	agent, _ := a.manager.GetAgentByID(uint(agentID))
	name := "Agente MCP"
	if agent != nil {
		name = agent.DisplayName
	}

	if err := a.manager.DeleteAgent(uint(agentID)); err != nil {
		return "", fmt.Errorf("erro ao deletar agente MCP: %w", err)
	}

	return fmt.Sprintf("✅ Agente MCP **%s** (ID: %d) deletado com sucesso!", name, agentID), nil
}

// ==================== Helper Functions ====================

func getStringArg(args map[string]interface{}, key, defaultValue string) string {
	if val, ok := args[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func getIntArg(args map[string]interface{}, key string, defaultValue int) int {
	if val, ok := args[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case int64:
			return int(v)
		}
	}
	return defaultValue
}

func getBoolArg(args map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := args[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}
