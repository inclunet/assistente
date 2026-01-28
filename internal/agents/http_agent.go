package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/agentmanager"
)

// HTTPAgentConfig configuração de um HTTP Agent
type HTTPAgentConfig struct {
	ID             uint
	Name           string
	DisplayName    string
	Description    string
	Model          string
	SystemPrompt   string
	Enabled        bool
	BaseURL        string
	AuthType       string
	AuthConfig     map[string]string
	DefaultHeaders map[string]string
	TimeoutSeconds int
	RetryCount     int
	Endpoints      []HTTPEndpointConfig
}

// HTTPEndpointConfig configuração de um endpoint
type HTTPEndpointConfig struct {
	ID               uint
	Name             string
	Description      string
	Method           string
	PathTemplate     string
	QueryTemplate    string
	HeadersJSON      string
	BodyTemplate     string
	Parameters       map[string]interface{} // JSON Schema
	ResponseTemplate string
}

// HTTPAgent implementa um agente que chama APIs REST
type HTTPAgent struct {
	BaseAgent
	config   HTTPAgentConfig
	executor *agentmanager.HTTPExecutor
	envVars  map[string]string
}

// NewHTTPAgent cria um novo HTTP Agent
func NewHTTPAgent(config HTTPAgentConfig, llm LLMClient) *HTTPAgent {
	// Carrega variáveis de ambiente
	envVars := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	return &HTTPAgent{
		BaseAgent: BaseAgent{
			Name:         config.Name,
			DisplayName:  config.DisplayName,
			Description:  config.Description,
			AgentType:    "http",
			Model:        config.Model,
			SystemPrompt: config.SystemPrompt,
			Enabled:      config.Enabled,
			LLM:          llm,
		},
		config: config,
		executor: agentmanager.NewHTTPExecutor(agentmanager.HTTPExecutorConfig{
			TimeoutSeconds: config.TimeoutSeconds,
			RetryCount:     config.RetryCount,
		}),
		envVars: envVars,
	}
}

// GetTools retorna as tools (endpoints) disponíveis para o LLM do agente
func (a *HTTPAgent) GetTools() []Tool {
	tools := make([]Tool, 0, len(a.config.Endpoints))

	for _, endpoint := range a.config.Endpoints {
		tool := Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        endpoint.Name,
				Description: endpoint.Description,
				Parameters:  endpoint.Parameters,
			},
		}
		tools = append(tools, tool)
	}

	return tools
}

// CanHandle verifica se o agente pode executar uma tool
func (a *HTTPAgent) CanHandle(toolName string) bool {
	for _, endpoint := range a.config.Endpoints {
		if endpoint.Name == toolName {
			return true
		}
	}
	return false
}

// ExecuteTool executa uma tool (endpoint HTTP)
func (a *HTTPAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	// Encontra o endpoint
	var endpoint *HTTPEndpointConfig
	for i := range a.config.Endpoints {
		if a.config.Endpoints[i].Name == toolCall.Function.Name {
			endpoint = &a.config.Endpoints[i]
			break
		}
	}

	if endpoint == nil {
		return "", fmt.Errorf("endpoint não encontrado: %s", toolCall.Function.Name)
	}

	// Parseia os argumentos
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &params); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %w", err)
	}

	// Executa a requisição HTTP
	return a.executeEndpoint(context.Background(), endpoint, params)
}

// Execute executa uma tarefa em linguagem natural
func (a *HTTPAgent) Execute(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM não configurado para o agente %s", a.Name)
	}

	executor := func(tc ToolCall) (string, error) {
		return a.ExecuteTool(tc)
	}

	// Usa o método com saver se disponível
	var result string
	var err error
	if a.MessageSaver != nil {
		result, err = a.LLM.ChatWithToolsAndSaver(
			ctx,
			a.Model,
			a.buildSystemPrompt(),
			task,
			a.GetTools(),
			executor,
			a.Name,
			a.MessageSaver,
		)
	} else {
		result, err = a.LLM.ChatWithTools(
			ctx,
			a.Model,
			a.buildSystemPrompt(),
			task,
			a.GetTools(),
			executor,
		)
	}

	if err != nil {
		return "", fmt.Errorf("erro ao executar agente HTTP: %w", err)
	}

	return result, nil
}

// buildSystemPrompt constrói o system prompt para o agente
func (a *HTTPAgent) buildSystemPrompt() string {
	if a.SystemPrompt != "" {
		return a.SystemPrompt
	}

	// Gera um prompt baseado nos endpoints disponíveis
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Você é o agente %s, especializado em interagir com a API %s.\n\n", a.DisplayName, a.config.BaseURL))
	sb.WriteString("Você tem acesso aos seguintes endpoints:\n\n")

	for _, endpoint := range a.config.Endpoints {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", endpoint.Name, endpoint.Description))
		sb.WriteString(fmt.Sprintf("  Método: %s %s\n", endpoint.Method, endpoint.PathTemplate))
	}

	sb.WriteString("\nAnalise a tarefa do usuário e use o endpoint apropriado para atendê-la.")
	sb.WriteString("\nRetorne uma resposta clara e útil baseada nos dados obtidos da API.")

	return sb.String()
}

// executeEndpoint executa um endpoint HTTP
func (a *HTTPAgent) executeEndpoint(ctx context.Context, endpoint *HTTPEndpointConfig, params map[string]interface{}) (string, error) {
	// Constrói a requisição
	req := agentmanager.HTTPRequest{
		Method:         endpoint.Method,
		BaseURL:        a.config.BaseURL,
		PathTemplate:   endpoint.PathTemplate,
		QueryTemplate:  endpoint.QueryTemplate,
		HeadersJSON:    endpoint.HeadersJSON,
		BodyTemplate:   endpoint.BodyTemplate,
		DefaultHeaders: a.config.DefaultHeaders,
		AuthType:       a.config.AuthType,
		AuthConfig:     a.config.AuthConfig,
		EnvVars:        a.envVars,
	}

	// Executa a requisição
	resp, err := a.executor.Execute(ctx, req, params, a.Name, a.DisplayName)
	if err != nil {
		return "", err
	}

	// Verifica erros na resposta
	if resp.Error != "" {
		return "", fmt.Errorf("erro na API: %s", resp.Error)
	}

	// Formata a resposta se houver template
	if endpoint.ResponseTemplate != "" {
		fmt.Printf("  📝 [HTTP Agent] ResponseTemplate: %q\n", endpoint.ResponseTemplate)
		result, err := a.executor.FormatResponse(resp, endpoint.ResponseTemplate, params, a.Name, a.DisplayName)
		if err != nil {
			fmt.Printf("  ❌ [HTTP Agent] Erro ao formatar resposta: %v\n", err)
			return "", err
		}
		fmt.Printf("  📤 [HTTP Agent] Retornando para LLM (template): %s\n", result)
		return result, nil
	}

	// Retorna o JSON formatado ou o body raw
	if resp.JSON != nil {
		jsonBytes, err := json.MarshalIndent(resp.JSON, "", "  ")
		if err == nil {
			result := string(jsonBytes)
			fmt.Printf("  📤 [HTTP Agent] Retornando para LLM: %s\n", result)
			return result, nil
		}
	}

	fmt.Printf("  📤 [HTTP Agent] Retornando para LLM (raw): %s\n", resp.Body)
	return resp.Body, nil
}

// TestEndpoint testa um endpoint específico (para o playground)
func (a *HTTPAgent) TestEndpoint(ctx context.Context, endpointName string, params map[string]interface{}) (string, error) {
	// Encontra o endpoint
	var endpoint *HTTPEndpointConfig
	for i := range a.config.Endpoints {
		if a.config.Endpoints[i].Name == endpointName {
			endpoint = &a.config.Endpoints[i]
			break
		}
	}

	if endpoint == nil {
		return "", fmt.Errorf("endpoint não encontrado: %s", endpointName)
	}

	return a.executeEndpoint(ctx, endpoint, params)
}

// GetEndpoints retorna os endpoints configurados
func (a *HTTPAgent) GetEndpoints() []HTTPEndpointConfig {
	return a.config.Endpoints
}

// UpdateConfig atualiza a configuração do agente
func (a *HTTPAgent) UpdateConfig(config HTTPAgentConfig) {
	a.config = config
	a.BaseAgent.DisplayName = config.DisplayName
	a.BaseAgent.Description = config.Description
	a.BaseAgent.Model = config.Model
	a.BaseAgent.SystemPrompt = config.SystemPrompt
	a.BaseAgent.Enabled = config.Enabled

	// Recria o executor com novos timeouts
	a.executor = agentmanager.NewHTTPExecutor(agentmanager.HTTPExecutorConfig{
		TimeoutSeconds: config.TimeoutSeconds,
		RetryCount:     config.RetryCount,
	})
}
