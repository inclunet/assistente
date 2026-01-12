package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/agents"
	"assistente/internal/database"
	"assistente/internal/filemanager"
	"assistente/internal/llm"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ToolResult representa o resultado de uma execução de ferramenta
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Role       string `json:"role"` // sempre "tool"
	Content    string `json:"content"`
}

// GetToolsForAPI retorna as tools de delegação para o orquestrador
func (a *App) GetToolsForAPI() []llm.Tool {
	if a.registry == nil {
		return []llm.Tool{}
	}
	// Converte agents.Tool para llm.Tool
	agentTools := a.registry.GetDelegationTools()
	result := make([]llm.Tool, len(agentTools))
	for i, t := range agentTools {
		result[i] = llm.Tool{
			Type: t.Type,
			Function: llm.ToolFunction{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		}
	}
	return result
}

// ExecuteTool executa uma tool de delegação
func (a *App) ExecuteTool(toolCall llm.ToolCall) (string, error) {
	toolName := toolCall.Function.Name

	// Verifica se é uma tool de delegação (delegate_to_*)
	if strings.HasPrefix(toolName, "delegate_to_") {
		agentName := strings.TrimPrefix(toolName, "delegate_to_")

		// Extrai a tarefa dos argumentos
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
		}

		task, ok := args["task"].(string)
		if !ok || task == "" {
			return "", fmt.Errorf("tarefa não especificada para o agente %s", agentName)
		}

		fmt.Printf("📤 [ORQUESTRADOR] Delegando para agente '%s': %s\n", agentName, truncateStr(task, 80))
		fmt.Printf("📤 [ORQUESTRADOR] currentDelegationID=%d, conversationID=%d\n", a.currentDelegationID, a.currentConversationID)

		// Configura o saver para salvar mensagens internas do agente
		if a.currentConversationID > 0 && a.currentDelegationID > 0 {
			a.registry.SetAgentConversationContext(agentName, a.currentConversationID, a.createAgentMessageSaver())
		} else {
			fmt.Printf("⚠️ [ORQUESTRADOR] Saver não criado: conversationID=%d, delegationID=%d\n",
				a.currentConversationID, a.currentDelegationID)
		}

		// Executa delegação para o agente
		result, err := a.registry.ExecuteDelegation(context.Background(), agentName, task)
		if err != nil {
			return "", fmt.Errorf("erro na delegação para %s: %v", agentName, err)
		}

		fmt.Printf("📥 [ORQUESTRADOR] Resposta do agente '%s': %s\n", agentName, truncateStr(result, 80))
		return result, nil
	}

	// Fallback para tools legadas (compatibilidade durante transição)
	agentToolCall := agents.ToolCall{
		ID:   toolCall.ID,
		Type: toolCall.Type,
		Function: agents.FunctionCall{
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		},
	}
	return a.registry.ExecuteTool(agentToolCall)
}

// createAgentMessageSaver cria um callback para salvar mensagens internas dos agentes
// NOVA ARQUITETURA v2:
// - n0: user/assistant
// - n1: assistant chamando agent (parentId=userMessage)
// - n2: agent interagindo com tools (parentId=mensagem de tool_calls do assistant)
func (a *App) createAgentMessageSaver() llm.MessageSaver {
	// NÃO captura parentID aqui - usa sempre o valor atual de currentDelegationID
	// Isso garante que múltiplas interações usem o parentID correto

	fmt.Printf("📝 [SAVER CRIADO] delegationID atual: %d\n", a.currentDelegationID)

	return func(msg llm.AgentMessage) error {
		// Usa o valor ATUAL de currentDelegationID (não capturado)
		parentID := a.currentDelegationID

		if a.currentConversationID == 0 || parentID == 0 {
			fmt.Printf("⚠️ [AGENT SAVER] Sem contexto: conversationID=%d, delegationID=%d\n",
				a.currentConversationID, parentID)
			return nil
		}

		// Serializa tool calls se houver
		var toolCallsJSON string
		if len(msg.ToolCalls) > 0 {
			if data, err := json.Marshal(msg.ToolCalls); err == nil {
				toolCallsJSON = string(data)
			}
		}

		// Para mensagens de tool_calls (agente chamando tools),
		// enriquece o conteúdo com os parâmetros para facilitar debug
		content := msg.Content
		if len(msg.ToolCalls) > 0 && content == "" {
			var toolDescriptions []string
			for _, tc := range msg.ToolCalls {
				toolDesc := fmt.Sprintf("📤 %s(%s)", tc.Function.Name, tc.Function.Arguments)
				toolDescriptions = append(toolDescriptions, toolDesc)
			}
			content = strings.Join(toolDescriptions, "\n")
		}

		fmt.Printf("💾 [AGENT SAVER] Salvando: role=%s, agent=%s, parentID=%d (nível 2)\n",
			msg.Role, msg.AgentName, parentID)

		// Salva como filho da mensagem de tool_calls do assistant (nível 2)
		savedMsg, err := database.AddChildMessage(
			a.currentConversationID,
			parentID,
			msg.Role,
			content,
			toolCallsJSON,
			msg.ToolCallID,
			msg.AgentName,
			msg.Model,
		)
		if err != nil {
			fmt.Printf("❌ [AGENT SAVER] Erro ao salvar: %v\n", err)
			return err
		}
		fmt.Printf("✅ [AGENT SAVER] Salvo: ID=%d, parentID=%d\n", savedMsg.ID, parentID)

		// Emite evento em tempo real para o frontend
		runtime.EventsEmit(a.ctx, "chat:agent_message", map[string]interface{}{
			"id":         savedMsg.ID,
			"parentId":   parentID,
			"role":       msg.Role,
			"content":    content,
			"agentName":  msg.AgentName,
			"toolCalls":  toolCallsJSON,
			"toolCallId": msg.ToolCallID,
		})

		return nil
	}
}

// truncateStr trunca uma string para exibição em logs
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// AgentInfo representa informações de um agente para a UI
type AgentInfo struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	AgentType    string `json:"agent_type"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	Enabled      bool   `json:"enabled"`
}

// GetRegisteredAgents retorna informações dos agentes registrados
func (a *App) GetRegisteredAgents() []AgentInfo {
	if a.registry == nil {
		return []AgentInfo{}
	}

	registeredAgents := a.registry.GetAll()
	result := make([]AgentInfo, 0, len(registeredAgents))

	for _, agent := range registeredAgents {
		result = append(result, AgentInfo{
			Name:         agent.GetName(),
			DisplayName:  agent.GetDisplayName(),
			Description:  agent.GetDescription(),
			AgentType:    agent.GetType(),
			Model:        agent.GetModel(),
			SystemPrompt: agent.GetSystemPrompt(),
			Enabled:      agent.IsEnabled(),
		})
	}

	return result
}

// TestAgent testa um agente com uma tarefa (playground)
func (a *App) TestAgent(agentName, task string) (string, error) {
	if a.registry == nil {
		return "", fmt.Errorf("registry não inicializado")
	}

	agent := a.registry.Get(agentName)
	if agent == nil {
		return "", fmt.Errorf("agente não encontrado: %s", agentName)
	}

	if !agent.IsEnabled() {
		return "", fmt.Errorf("agente desabilitado: %s", agentName)
	}

	fmt.Printf("🧪 [PLAYGROUND] Testando agente '%s' com: %s\n", agentName, truncateStr(task, 80))

	result, err := agent.Execute(context.Background(), task)
	if err != nil {
		return "", fmt.Errorf("erro ao executar agente: %v", err)
	}

	fmt.Printf("✅ [PLAYGROUND] Resultado: %s\n", truncateStr(result, 100))
	return result, nil
}

// ==================== HTTP Agent API ====================

// HTTPAgentFullConfig representa a configuração completa de um HTTP Agent para a UI
type HTTPAgentFullConfig struct {
	// Dados do AgentConfig
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	Enabled      bool   `json:"enabled"`

	// Dados específicos do HTTPAgent
	HTTPAgentID    uint               `json:"http_agent_id"`
	BaseURL        string             `json:"base_url"`
	AuthType       string             `json:"auth_type"`
	AuthConfig     string             `json:"auth_config"`     // JSON
	DefaultHeaders string             `json:"default_headers"` // JSON
	TimeoutSeconds int                `json:"timeout_seconds"`
	RetryCount     int                `json:"retry_count"`
	Endpoints      []HTTPEndpointInfo `json:"endpoints"`
}

// HTTPEndpointInfo representa um endpoint para a UI
type HTTPEndpointInfo struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Method           string `json:"method"`
	PathTemplate     string `json:"path_template"`
	QueryTemplate    string `json:"query_template"`
	HeadersJSON      string `json:"headers_json"`
	BodyTemplate     string `json:"body_template"`
	Parameters       string `json:"parameters"` // JSON Schema
	ResponseTemplate string `json:"response_template"`
}

// CreateHTTPAgentFull cria um HTTP Agent completo (AgentConfig + HTTPAgent)
func (a *App) CreateHTTPAgentFull(name, displayName, description, model, systemPrompt string, enabled bool,
	baseURL, authType, authConfig, defaultHeaders string, timeoutSeconds, retryCount int) (*HTTPAgentFullConfig, error) {

	// 1. Cria o AgentConfig
	agentConfig, err := a.CreateAgentConfig(name, displayName, description, "http", model, systemPrompt, "", enabled)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar AgentConfig: %w", err)
	}

	// 2. Cria o HTTPAgent
	httpAgent, err := a.CreateHTTPAgent(agentConfig.ID, baseURL, authType, authConfig, defaultHeaders, timeoutSeconds, retryCount)
	if err != nil {
		// Rollback: deleta o AgentConfig
		a.DeleteAgentConfig(agentConfig.ID)
		return nil, fmt.Errorf("erro ao criar HTTPAgent: %w", err)
	}

	return &HTTPAgentFullConfig{
		ID:             agentConfig.ID,
		Name:           agentConfig.Name,
		DisplayName:    agentConfig.DisplayName,
		Description:    agentConfig.Description,
		Model:          agentConfig.Model,
		SystemPrompt:   agentConfig.SystemPrompt,
		Enabled:        agentConfig.Enabled,
		HTTPAgentID:    httpAgent.ID,
		BaseURL:        httpAgent.BaseURL,
		AuthType:       httpAgent.AuthType,
		AuthConfig:     httpAgent.AuthConfig,
		DefaultHeaders: httpAgent.DefaultHeaders,
		TimeoutSeconds: httpAgent.TimeoutSeconds,
		RetryCount:     httpAgent.RetryCount,
		Endpoints:      []HTTPEndpointInfo{},
	}, nil
}

// GetHTTPAgentFull retorna a configuração completa de um HTTP Agent
func (a *App) GetHTTPAgentFull(agentConfigID uint) (*HTTPAgentFullConfig, error) {
	// Busca o AgentConfig
	agentConfig, err := a.GetAgentConfigByID(agentConfigID)
	if err != nil {
		return nil, fmt.Errorf("AgentConfig não encontrado: %w", err)
	}

	// Busca o HTTPAgent
	httpAgent, err := a.GetHTTPAgentByConfigID(agentConfigID)
	if err != nil {
		return nil, fmt.Errorf("HTTPAgent não encontrado: %w", err)
	}

	// Converte endpoints
	endpoints := make([]HTTPEndpointInfo, 0, len(httpAgent.Endpoints))
	for _, ep := range httpAgent.Endpoints {
		endpoints = append(endpoints, HTTPEndpointInfo{
			ID:               ep.ID,
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

	return &HTTPAgentFullConfig{
		ID:             agentConfig.ID,
		Name:           agentConfig.Name,
		DisplayName:    agentConfig.DisplayName,
		Description:    agentConfig.Description,
		Model:          agentConfig.Model,
		SystemPrompt:   agentConfig.SystemPrompt,
		Enabled:        agentConfig.Enabled,
		HTTPAgentID:    httpAgent.ID,
		BaseURL:        httpAgent.BaseURL,
		AuthType:       httpAgent.AuthType,
		AuthConfig:     httpAgent.AuthConfig,
		DefaultHeaders: httpAgent.DefaultHeaders,
		TimeoutSeconds: httpAgent.TimeoutSeconds,
		RetryCount:     httpAgent.RetryCount,
		Endpoints:      endpoints,
	}, nil
}

// GetAllHTTPAgentsFull retorna todos os HTTP Agents com config completa
func (a *App) GetAllHTTPAgentsFull() ([]HTTPAgentFullConfig, error) {
	// Busca todos os AgentConfigs do tipo HTTP
	configs, err := a.GetAllAgentConfigs()
	if err != nil {
		return nil, err
	}

	result := make([]HTTPAgentFullConfig, 0)
	for _, config := range configs {
		if config.AgentType == "http" {
			full, err := a.GetHTTPAgentFull(config.ID)
			if err == nil {
				result = append(result, *full)
			}
		}
	}

	return result, nil
}

// UpdateHTTPAgentFull atualiza um HTTP Agent completo
func (a *App) UpdateHTTPAgentFull(agentConfigID uint, displayName, description, model, systemPrompt string, enabled bool,
	baseURL, authType, authConfig, defaultHeaders string, timeoutSeconds, retryCount int) (*HTTPAgentFullConfig, error) {

	// Busca o AgentConfig existente
	existingConfig, err := a.GetAgentConfigByID(agentConfigID)
	if err != nil {
		return nil, fmt.Errorf("AgentConfig não encontrado: %w", err)
	}

	// Atualiza o AgentConfig
	_, err = a.UpdateAgentConfig(agentConfigID, displayName, description, model, systemPrompt, "", enabled)
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar AgentConfig: %w", err)
	}

	// Busca o HTTPAgent
	httpAgent, err := a.GetHTTPAgentByConfigID(agentConfigID)
	if err != nil {
		return nil, fmt.Errorf("HTTPAgent não encontrado: %w", err)
	}

	// Atualiza o HTTPAgent
	_, err = a.UpdateHTTPAgent(httpAgent.ID, baseURL, authType, authConfig, defaultHeaders, timeoutSeconds, retryCount)
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar HTTPAgent: %w", err)
	}

	// Retorna a config atualizada
	return a.GetHTTPAgentFull(existingConfig.ID)
}

// DeleteHTTPAgentFull deleta um HTTP Agent completo
func (a *App) DeleteHTTPAgentFull(agentConfigID uint) error {
	// Busca o HTTPAgent
	httpAgent, err := a.GetHTTPAgentByConfigID(agentConfigID)
	if err == nil {
		// Deleta o HTTPAgent (cascata para endpoints)
		a.DeleteHTTPAgent(httpAgent.ID)
	}

	// Deleta o AgentConfig
	return a.DeleteAgentConfig(agentConfigID)
}

// ValidateTemplate valida um template Go
func (a *App) ValidateTemplate(templateStr string) (bool, string) {
	engine := agents.NewTemplateEngine()
	err := engine.ValidateTemplate(templateStr)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

// ExtractTemplateVariables extrai as variáveis usadas em um template
func (a *App) ExtractTemplateVariables(templateStr string) []string {
	engine := agents.NewTemplateEngine()
	return engine.ExtractVariables(templateStr)
}

// TestHTTPEndpoint testa um endpoint HTTP com parâmetros
func (a *App) TestHTTPEndpoint(httpAgentID uint, endpointName string, paramsJSON string) (string, error) {
	// Busca o HTTPAgent
	httpAgent, err := a.GetHTTPAgent(httpAgentID)
	if err != nil {
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
	var endpoint *HTTPEndpoint
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
	agentConfig, _ := a.GetAgentConfigByID(httpAgent.AgentConfigID)
	agentName := "http_agent"
	displayName := "HTTP Agent"
	if agentConfig != nil {
		agentName = agentConfig.Name
		displayName = agentConfig.DisplayName
	}

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
	executor := agents.NewHTTPExecutor(agents.HTTPExecutorConfig{
		TimeoutSeconds: httpAgent.TimeoutSeconds,
		RetryCount:     httpAgent.RetryCount,
	})

	// Monta a request
	req := agents.HTTPRequest{
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

// ==================== File Agent APIs ====================

// FileAgentAuthorizedPathInfo representa info de uma pasta autorizada para a UI
type FileAgentAuthorizedPathInfo struct {
	ID          uint   `json:"id"`
	Path        string `json:"path"`
	AllowDelete bool   `json:"allow_delete"`
	AllowWrite  bool   `json:"allow_write"`
	Recursive   bool   `json:"recursive"`
}

// GetFileAgentAuthorizedPaths retorna todas as pastas autorizadas
func (a *App) GetFileAgentAuthorizedPaths() ([]FileAgentAuthorizedPathInfo, error) {
	paths, err := database.GetAllFileAgentAuthorizedPaths()
	if err != nil {
		return nil, err
	}

	result := make([]FileAgentAuthorizedPathInfo, len(paths))
	for i, p := range paths {
		result[i] = FileAgentAuthorizedPathInfo{
			ID:          p.ID,
			Path:        p.Path,
			AllowDelete: p.AllowDelete,
			AllowWrite:  p.AllowWrite,
			Recursive:   p.Recursive,
		}
	}
	return result, nil
}

// CreateFileAgentAuthorizedPath cria uma nova pasta autorizada
func (a *App) CreateFileAgentAuthorizedPath(path string, allowDelete, allowWrite, recursive bool) (*FileAgentAuthorizedPathInfo, error) {
	authPath, err := database.CreateFileAgentAuthorizedPath(path, allowDelete, allowWrite, recursive)
	if err != nil {
		return nil, err
	}

	// Atualiza o FileAgent
	a.reloadFileAgentPaths()

	return &FileAgentAuthorizedPathInfo{
		ID:          authPath.ID,
		Path:        authPath.Path,
		AllowDelete: authPath.AllowDelete,
		AllowWrite:  authPath.AllowWrite,
		Recursive:   authPath.Recursive,
	}, nil
}

// UpdateFileAgentAuthorizedPath atualiza uma pasta autorizada
func (a *App) UpdateFileAgentAuthorizedPath(id uint, path string, allowDelete, allowWrite, recursive bool) (*FileAgentAuthorizedPathInfo, error) {
	authPath, err := database.UpdateFileAgentAuthorizedPath(id, path, allowDelete, allowWrite, recursive)
	if err != nil {
		return nil, err
	}

	// Atualiza o FileAgent
	a.reloadFileAgentPaths()

	return &FileAgentAuthorizedPathInfo{
		ID:          authPath.ID,
		Path:        authPath.Path,
		AllowDelete: authPath.AllowDelete,
		AllowWrite:  authPath.AllowWrite,
		Recursive:   authPath.Recursive,
	}, nil
}

// DeleteFileAgentAuthorizedPath deleta uma pasta autorizada
func (a *App) DeleteFileAgentAuthorizedPath(id uint) error {
	err := database.DeleteFileAgentAuthorizedPath(id)
	if err != nil {
		return err
	}

	// Atualiza o FileAgent
	a.reloadFileAgentPaths()
	return nil
}

// reloadFileAgentPaths recarrega as pastas autorizadas no FileAgent
func (a *App) reloadFileAgentPaths() {
	if a.registry == nil {
		return
	}

	agent := a.registry.Get("file_manager")
	if agent == nil {
		return
	}

	fileAgent, ok := agent.(*agents.FileAgent)
	if !ok {
		return
	}

	a.loadFileAgentAuthorizedPaths(fileAgent)
}

// GetFileAgentProtectedPaths retorna as pastas protegidas (apenas leitura)
func (a *App) GetFileAgentProtectedPaths() map[string]interface{} {
	return map[string]interface{}{
		"paths":      filemanager.GetProtectedPaths(),
		"extensions": filemanager.GetProtectedExtensions(),
		"files":      filemanager.GetProtectedFiles(),
	}
}

// TestFileAgent executa o FileAgent com uma tarefa de teste
func (a *App) TestFileAgent(task string) (string, error) {
	if a.registry == nil {
		return "", fmt.Errorf("registry não inicializado")
	}

	agent := a.registry.Get("file_manager")
	if agent == nil {
		return "", fmt.Errorf("FileAgent não encontrado")
	}

	return agent.Execute(context.Background(), task)
}
