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
	manager            agentmanager.Manager
	reloadHTTPCallback func(agentConfigID uint) error
	reloadMCPCallback  func(mcpAgentID uint) error
}

// builderAgentDescription returns structured description for orchestrator delegation
func builderAgentDescription() string {
	return NewDelegationDescription("Agent Builder", "Specialist in creating and managing HTTP and MCP agents").
		Capabilities(
			"Create HTTP agents that connect to external REST APIs",
			"Create MCP (Model Context Protocol) agents for tools",
			"Import OpenAPI/Swagger specifications automatically",
			"Import Postman collections as agents",
			"Manage HTTP endpoints (full CRUD)",
			"Test endpoints with real parameters",
			"Hot reload: changes available IMMEDIATELY",
		).
		DelegateWhen(
			"User wants to create a new agent for external API",
			"User wants to connect to an MCP server",
			"User wants to import OpenAPI, Swagger, or Postman",
			"User wants to list, update, or delete existing agents",
			"User wants to create or manage HTTP endpoints",
			"User mentions 'create agent', 'connect API', 'MCP'",
		).
		DontDelegateWhen(
			"Only queries about APIs (without creating agents)",
			"User wants to USE an existing agent (delegate to that specific agent)",
			"Questions about code or local files (use FileAgent)",
		).
		Build()
}

// builderAgentSystemPrompt returns the reduced system prompt
func builderAgentSystemPrompt() string {
	return `You are the Agent Builder, specialist in creating and managing HTTP and MCP agents.

## CRITICAL RULE: AVOID DUPLICATES
ALWAYS list existing agents BEFORE creating new ones:
- HTTP: builder_list_http_agents → builder_get_http_agent to see endpoints
- MCP: builder_list_mcp_agents

If you find a duplicate or lack information: ASK the user.
NEVER assume or invent URLs, commands, or configurations.

## HOT RELOAD
Created/updated agents are available IMMEDIATELY without restart.

## NAMING
- name: snake_case (e.g., github_api, weather_api)
- display_name: readable (e.g., "GitHub API")
- endpoint name: clear action (e.g., get_user, create_issue)

## AUTH CONFIG
- bearer: {"token": "ENV:TOKEN"}
- api-key: {"key": "ENV:KEY", "header": "X-API-Key"}
- basic: {"username": "user", "password": "ENV:PASS"}
Always prefer ENV:VAR for sensitive tokens.

## JSON SCHEMA FOR ENDPOINTS
Required format:
{"type": "object", "properties": {...}, "required": [...]}

Each tool has detailed instructions on when to use.`
}

// GetDelegationDescription implements DelegationDescriptionProvider
func (a *BuilderAgent) GetDelegationDescription() string {
	return builderAgentDescription()
}

// NewBuilderAgent creates a new intelligent BuilderAgent
func NewBuilderAgent(manager agentmanager.Manager, llmClient LLMClient, model string) *BuilderAgent {
	if model == "" {
		model = "gpt-4o-mini"
	}

	return &BuilderAgent{
		BaseAgent: BaseAgent{
			Name:         "builder",
			DisplayName:  "Agent Builder",
			Description:  "Specialist in creating and managing HTTP and MCP agents. Creates agents from OpenAPI/Postman or natural instructions. ALWAYS checks for duplicates before creating.",
			AgentType:    "internal",
			Model:        model,
			SystemPrompt: builderAgentSystemPrompt(),
			Enabled:      true,
			LLM:          llmClient,
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

// GetTools returns the available tools for the agent
func (a *BuilderAgent) GetTools() []Tool {
	return []Tool{
		// ==================== HTTP Agent CRUD ====================
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_list_http_agents",
				Description: NewToolDescription("Lists all HTTP agents registered in the system").
					WhenToUse(
						"ALWAYS as FIRST STEP before creating any HTTP agent",
						"To check if an agent for a specific API already exists",
						"To find the ID of an existing agent",
						"To see which APIs are already connected",
					).
					WhenNotToUse(
						"If already listed recently in the same conversation",
					).
					Returns("List of HTTP agents with ID, name, description, and status (enabled/disabled)").
					Notes(
						"Use this list to avoid creating duplicate agents",
						"After listing, use builder_get_http_agent to see endpoints of specific agents",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_get_http_agent",
				Description: NewToolDescription("Gets complete details of an HTTP agent including all its endpoints").
					WhenToUse(
						"Before creating endpoint, to see existing endpoints",
						"To check if a specific endpoint already exists",
						"To see current configuration (base_url, auth, etc)",
						"To identify duplicate functionality",
					).
					Returns("Agent details: name, base_url, auth, and list of all endpoints with path_template").
					Notes(
						"ALWAYS use this tool before builder_create_endpoint",
						"Check path_template to avoid duplicate endpoints",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"agent_id": JSONSchemaInt(
						NewParamDescription("HTTP agent ID (obtained via builder_list_http_agents)").
							Examples("1", "5", "12").Build(),
					),
				}, []string{"agent_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_create_http_agent",
				Description: NewToolDescription("Creates a new HTTP agent to connect to an external REST API").
					WhenToUse(
						"After confirming with builder_list_http_agents that no similar agent exists",
						"When user provided the API base_url",
						"When you have sufficient information (URL, auth if needed)",
					).
					WhenNotToUse(
						"NEVER use without first listing existing agents",
						"If an agent with the same base_url exists - add endpoints to the existing one",
						"If critical information is missing (URL, auth type) - ask user first",
					).
					Returns("Created agent ID and hot reload confirmation").
					Notes(
						"After creating, use builder_create_endpoint to add functionality",
						"Agent becomes available IMMEDIATELY (hot reload)",
						"name must be unique snake_case (e.g., github_api, weather_api)",
						"Use ENV:VAR for sensitive tokens in auth_config",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"name": JSONSchemaString(
						NewParamDescription("Unique internal agent name").
							Formats("snake_case").
							Examples("github_api", "openweather_api", "stripe_payments").Build(),
					),
					"display_name": JSONSchemaString(
						NewParamDescription("User-friendly display name").
							Examples("GitHub API", "OpenWeather", "Stripe Payments").Build(),
					),
					"description": JSONSchemaString(
						NewParamDescription("Description of what the agent does").Build(),
					),
					"base_url": JSONSchemaString(
						NewParamDescription("API base URL (without trailing slash)").
							Formats("https://...").
							Examples("https://api.github.com", "https://api.openweathermap.org").Build(),
					),
					"auth_type": JSONSchemaStringEnum(
						NewParamDescription("Authentication type").
							Default("none").Build(),
						[]string{"bearer", "api-key", "basic", "none"},
					),
					"auth_config": JSONSchemaString(
						NewParamDescription("JSON with auth configuration").
							Examples(
								`{"token": "ENV:GITHUB_TOKEN"}`,
								`{"key": "ENV:API_KEY", "header": "X-API-Key"}`,
								`{"username": "user", "password": "ENV:PASS"}`,
							).Build(),
					),
					"default_headers": JSONSchemaString(
						NewParamDescription("JSON with default headers for all requests").
							Examples(`{"Accept": "application/json"}`).Build(),
					),
					"model": JSONSchemaString(
						NewParamDescription("LLM model for the agent").Default("gpt-4o-mini").Build(),
					),
					"timeout_seconds": JSONSchemaInt(
						NewParamDescription("Request timeout in seconds").Default("30").Build(),
					),
					"retry_count": JSONSchemaInt(
						NewParamDescription("Number of retries on error").Default("3").Build(),
					),
				}, []string{"name", "display_name", "base_url"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_update_http_agent",
				Description: NewToolDescription("Updates configuration of an existing HTTP agent").
					WhenToUse(
						"To change base_url, auth, or headers of existing agent",
						"To enable/disable an agent",
						"To fix authentication configuration",
					).
					Returns("Update confirmation and hot reload").
					Notes(
						"Changes become active IMMEDIATELY",
						"Only pass fields you want to change",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"agent_id": JSONSchemaInt(
						NewParamDescription("ID of HTTP agent to update").Build(),
					),
					"display_name":    JSONSchemaString(NewParamDescription("New display name").Build()),
					"description":     JSONSchemaString(NewParamDescription("New description").Build()),
					"model":           JSONSchemaString(NewParamDescription("New LLM model").Build()),
					"base_url":        JSONSchemaString(NewParamDescription("New base URL").Build()),
					"auth_type":       JSONSchemaStringEnum(NewParamDescription("New auth type").Build(), []string{"bearer", "api-key", "basic", "none"}),
					"auth_config":     JSONSchemaString(NewParamDescription("New auth config (JSON)").Build()),
					"default_headers": JSONSchemaString(NewParamDescription("New default headers (JSON)").Build()),
					"timeout_seconds": JSONSchemaInt(NewParamDescription("New timeout in seconds").Build()),
					"retry_count":     JSONSchemaInt(NewParamDescription("New retry count").Build()),
					"enabled":         JSONSchemaBool(NewParamDescription("Enable or disable the agent").Build()),
				}, []string{"agent_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_delete_http_agent",
				Description: NewToolDescription("Deletes an HTTP agent and all its endpoints").
					WhenToUse(
						"When user confirms they want to remove the agent",
						"To clean up duplicate or unused agents",
					).
					WhenNotToUse(
						"Without user confirmation - this action is irreversible",
					).
					Returns("Deletion confirmation").
					Notes(
						"IRREVERSIBLE ACTION - all endpoints are also deleted",
						"Ask for user confirmation before executing",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"agent_id": JSONSchemaInt(NewParamDescription("ID of HTTP agent to delete").Build()),
				}, []string{"agent_id"}),
			},
		},

		// ==================== HTTP Endpoints ====================
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_create_endpoint",
				Description: NewToolDescription("Adds a new endpoint to an existing HTTP agent").
					WhenToUse(
						"After verifying with builder_get_http_agent that endpoint doesn't exist",
						"When user asks for new functionality for existing API",
						"When you have complete information (method, path, params)",
					).
					WhenNotToUse(
						"NEVER use without first checking existing endpoints",
						"If endpoint with same path_template already exists",
						"If similar functionality is already available - ask user",
					).
					Returns("Created endpoint ID and confirmation").
					Notes(
						"path_template uses Go templates: /users/{{.user_id}}",
						"parameters MUST be valid JSON Schema",
						"Format: {\"type\":\"object\",\"properties\":{...},\"required\":[...]}",
						"name must be descriptive snake_case: get_user, create_issue",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"agent_id": JSONSchemaInt(
						NewParamDescription("ID of HTTP agent that will receive the endpoint").Build(),
					),
					"name": JSONSchemaString(
						NewParamDescription("Unique endpoint name").
							Formats("snake_case").
							Examples("get_user", "create_issue", "search_repos", "delete_comment").Build(),
					),
					"description": JSONSchemaString(
						NewParamDescription("Clear description of what the endpoint does").Build(),
					),
					"method": JSONSchemaStringEnum(
						NewParamDescription("HTTP method").Build(),
						[]string{"GET", "POST", "PUT", "DELETE", "PATCH"},
					),
					"path_template": JSONSchemaString(
						NewParamDescription("Path template with variables in Go template format").
							Examples("/users/{{.user_id}}", "/repos/{{.owner}}/{{.repo}}/issues", "/search").Build(),
					),
					"query_template": JSONSchemaString(
						NewParamDescription("Query params template").
							Examples("page={{.page}}&per_page={{.per_page}}", "q={{.query}}").Build(),
					),
					"headers_json": JSONSchemaString(
						NewParamDescription("Endpoint-specific headers (JSON)").Build(),
					),
					"body_template": JSONSchemaString(
						NewParamDescription("Body template for POST/PUT/PATCH (JSON with Go templates)").
							Examples(`{"title": "{{.title}}", "body": "{{.body}}"}`).Build(),
					),
					"parameters": JSONSchemaString(
						NewParamDescription("JSON Schema for accepted parameters").
							Examples(
								`{"type":"object","properties":{"user_id":{"type":"string","description":"User ID"}},"required":["user_id"]}`,
							).Build(),
					),
					"response_template": JSONSchemaString(
						NewParamDescription("Template to format the response").Build(),
					),
				}, []string{"agent_id", "name", "method", "path_template"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_update_endpoint",
				Description: NewToolDescription("Updates configuration of an existing endpoint").
					WhenToUse(
						"To fix path_template or parameters",
						"To improve endpoint description",
						"To adjust body_template or query_template",
					).
					Returns("Update confirmation").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"endpoint_id":       JSONSchemaInt(NewParamDescription("ID of endpoint to update").Build()),
					"name":              JSONSchemaString(NewParamDescription("New name").Build()),
					"description":       JSONSchemaString(NewParamDescription("New description").Build()),
					"method":            JSONSchemaStringEnum(NewParamDescription("New method").Build(), []string{"GET", "POST", "PUT", "DELETE", "PATCH"}),
					"path_template":     JSONSchemaString(NewParamDescription("New path template").Build()),
					"query_template":    JSONSchemaString(NewParamDescription("New query template").Build()),
					"headers_json":      JSONSchemaString(NewParamDescription("New headers (JSON)").Build()),
					"body_template":     JSONSchemaString(NewParamDescription("New body template").Build()),
					"parameters":        JSONSchemaString(NewParamDescription("New parameter JSON Schema").Build()),
					"response_template": JSONSchemaString(NewParamDescription("New response template").Build()),
				}, []string{"endpoint_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_delete_endpoint",
				Description: NewToolDescription("Removes an endpoint from an HTTP agent").
					WhenToUse(
						"When user confirms removal",
						"To clean up duplicate endpoints",
					).
					WhenNotToUse(
						"Without user confirmation",
					).
					Returns("Deletion confirmation").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"endpoint_id": JSONSchemaInt(NewParamDescription("ID of endpoint to delete").Build()),
				}, []string{"endpoint_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_test_endpoint",
				Description: NewToolDescription("Tests an endpoint with real parameters to validate it works").
					WhenToUse(
						"After creating or updating endpoint",
						"To verify configuration is correct",
						"To show user it works",
					).
					Returns("HTTP call result (success or detailed error)").
					Notes(
						"Provides diagnosis if there's an error",
						"Use to validate before confirming to user",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"agent_id": JSONSchemaInt(NewParamDescription("HTTP agent ID").Build()),
					"endpoint_name": JSONSchemaString(
						NewParamDescription("Name of endpoint to test").
							Examples("get_user", "search_repos").Build(),
					),
					"params_json": JSONSchemaString(
						NewParamDescription("JSON with parameters for the test").
							Examples(`{"user_id": "octocat"}`, `{"query": "rust"}`).Build(),
					),
				}, []string{"agent_id", "endpoint_name"}),
			},
		},

		// ==================== Import ====================
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_import_openapi",
				Description: NewToolDescription("Imports a complete agent from OpenAPI/Swagger specification").
					WhenToUse(
						"When user provides OpenAPI file (YAML or JSON)",
						"To create agent with many endpoints quickly",
						"To ensure fidelity to API specification",
					).
					WhenNotToUse(
						"For APIs without OpenAPI spec - use builder_create_http_agent",
					).
					Returns("Agent created with all endpoints from specification").
					Notes(
						"Supports OpenAPI 2.0 (Swagger) and OpenAPI 3.x",
						"Accepts YAML or JSON",
						"After importing, check for duplicates with existing agents",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"content": JSONSchemaString(
						NewParamDescription("Complete OpenAPI specification content").
							Formats("YAML", "JSON").Build(),
					),
					"name": JSONSchemaString(
						NewParamDescription("Name for created agent").
							Formats("snake_case").Build(),
					),
					"model": JSONSchemaString(
						NewParamDescription("LLM model").Default("gpt-4o-mini").Build(),
					),
				}, []string{"content", "name"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_import_postman",
				Description: NewToolDescription("Imports an agent from Postman collection").
					WhenToUse(
						"When user provides Postman JSON file",
						"To convert existing Postman collection to agent",
					).
					Returns("Agent created with endpoints from collection").
					Notes(
						"Supports Postman Collection v2.x format",
						"Environment variables are converted to templates",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"content": JSONSchemaString(
						NewParamDescription("Complete Postman collection JSON").Build(),
					),
					"name": JSONSchemaString(
						NewParamDescription("Name for created agent").
							Formats("snake_case").Build(),
					),
					"model": JSONSchemaString(
						NewParamDescription("LLM model").Default("gpt-4o-mini").Build(),
					),
				}, []string{"content", "name"}),
			},
		},

		// ==================== MCP Agents ====================
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_list_mcp_agents",
				Description: NewToolDescription("Lists all MCP agents registered").
					WhenToUse(
						"ALWAYS as FIRST STEP before creating MCP agent",
						"To check if connection to desired server already exists",
						"To find ID of existing MCP agent",
					).
					Returns("List of MCP agents with transport type (stdio/http) and configuration").
					Notes(
						"Use to avoid creating multiple connections to same server",
						"Check server_command or server_url to identify duplicates",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_create_mcp_agent",
				Description: NewToolDescription("Creates a new MCP agent to connect to a Model Context Protocol server").
					WhenToUse(
						"After confirming with builder_list_mcp_agents that no similar connection exists",
						"When user provided MCP server information",
						"When you have: type (stdio/http), command/URL, arguments if needed",
					).
					WhenNotToUse(
						"NEVER use without first listing existing MCP agents",
						"If connection to same server exists",
						"If critical information is missing - ask user first",
						"NEVER assume or invent MCP server commands/URLs",
					).
					Returns("Created agent ID and confirmation").
					Notes(
						"stdio: requires server_command and optionally server_args",
						"http: requires server_url",
						"Example stdio commands: npx, node, python",
						"If auto_connect=true, connects immediately",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"name": JSONSchemaString(
						NewParamDescription("Unique internal name").
							Formats("snake_case").
							Examples("filesystem_mcp", "brave_search_mcp", "postgres_mcp").Build(),
					),
					"display_name": JSONSchemaString(
						NewParamDescription("Display name").
							Examples("FileSystem MCP", "Brave Search", "PostgreSQL").Build(),
					),
					"description": JSONSchemaString(
						NewParamDescription("Description of MCP server capabilities").Build(),
					),
					"transport_type": JSONSchemaStringEnum(
						NewParamDescription("MCP server transport type").Build(),
						[]string{"stdio", "http"},
					),
					"server_command": JSONSchemaString(
						NewParamDescription("Command to start server (stdio)").
							Examples("npx", "node", "python", "uvx").Build(),
					),
					"server_args": JSONSchemaString(
						NewParamDescription("Command arguments (JSON array)").
							Examples(
								`["-y", "@modelcontextprotocol/server-filesystem", "/path"]`,
								`["server.js"]`,
							).Build(),
					),
					"server_env": JSONSchemaString(
						NewParamDescription("Environment variables (JSON)").
							Examples(`{"API_KEY": "ENV:MCP_API_KEY"}`).Build(),
					),
					"working_dir": JSONSchemaString(
						NewParamDescription("Working directory for the server").Build(),
					),
					"server_url": JSONSchemaString(
						NewParamDescription("Server URL (http)").
							Examples("http://localhost:3000/mcp", "https://mcp.example.com").Build(),
					),
					"auth_type":    JSONSchemaString(NewParamDescription("Auth type for http").Build()),
					"auth_value":   JSONSchemaString(NewParamDescription("Auth value").Build()),
					"http_headers": JSONSchemaString(NewParamDescription("Additional HTTP headers (JSON)").Build()),
					"model": JSONSchemaString(
						NewParamDescription("LLM model").Default("gpt-4o-mini").Build(),
					),
					"auto_connect": JSONSchemaBool(
						NewParamDescription("Connect automatically on creation").Default("false").Build(),
					),
				}, []string{"name", "display_name", "transport_type"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_update_mcp_agent",
				Description: NewToolDescription("Updates configuration of an existing MCP agent").
					WhenToUse(
						"To change command, arguments, or server URL",
						"To enable/disable agent",
						"To fix configuration",
					).
					Returns("Update confirmation and hot reload").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"agent_id":       JSONSchemaInt(NewParamDescription("ID of MCP agent to update").Build()),
					"display_name":   JSONSchemaString(NewParamDescription("New display name").Build()),
					"description":    JSONSchemaString(NewParamDescription("New description").Build()),
					"model":          JSONSchemaString(NewParamDescription("New LLM model").Build()),
					"enabled":        JSONSchemaBool(NewParamDescription("Enable or disable").Build()),
					"server_command": JSONSchemaString(NewParamDescription("New command (stdio)").Build()),
					"server_args":    JSONSchemaString(NewParamDescription("New arguments (JSON array)").Build()),
					"server_env":     JSONSchemaString(NewParamDescription("New environment variables (JSON)").Build()),
					"server_url":     JSONSchemaString(NewParamDescription("New URL (http)").Build()),
					"auth_type":      JSONSchemaString(NewParamDescription("New auth type").Build()),
					"auth_value":     JSONSchemaString(NewParamDescription("New auth value").Build()),
					"http_headers":   JSONSchemaString(NewParamDescription("New headers (JSON)").Build()),
					"auto_connect":   JSONSchemaBool(NewParamDescription("Auto connect").Build()),
				}, []string{"agent_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "builder_delete_mcp_agent",
				Description: NewToolDescription("Removes an MCP agent from the system").
					WhenToUse(
						"When user confirms removal",
						"To clean up duplicate or unused agents",
					).
					WhenNotToUse(
						"Without user confirmation - irreversible action",
					).
					Returns("Deletion confirmation").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"agent_id": JSONSchemaInt(NewParamDescription("ID of MCP agent to delete").Build()),
				}, []string{"agent_id"}),
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
