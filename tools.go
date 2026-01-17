package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"assistente/internal/agentmanager"
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

// ReloadHTTPAgent recarrega um HTTP Agent no registry (hot reload)
func (a *App) ReloadHTTPAgent(agentConfigID uint) error {
	log.Printf("[ReloadHTTPAgent] 🔄 Iniciando reload do agente ID: %d", agentConfigID)
	
	// Busca a config completa
	full, err := a.GetHTTPAgentFull(agentConfigID)
	if err != nil {
		return fmt.Errorf("erro ao buscar agente: %w", err)
	}

	log.Printf("[ReloadHTTPAgent] 📦 Agente carregado: %s com %d endpoints", full.DisplayName, len(full.Endpoints))
	for i, ep := range full.Endpoints {
		log.Printf("[ReloadHTTPAgent]   - Endpoint %d: %s (%s %s)", i+1, ep.Name, ep.Method, ep.PathTemplate)
	}

	// Remove do registry se já estiver registrado
	a.registry.Unregister(full.Name)
	log.Printf("[ReloadHTTPAgent] 🗑️ Agente %s removido do registry", full.Name)

	// Só recarrega se estiver habilitado
	if !full.Enabled {
		log.Printf("[ReloadHTTPAgent] ⚠️ Agente %s está desabilitado, não será carregado", full.Name)
		return nil
	}

	// Registra novamente
	if err := a.registerHTTPAgentInRegistry(full); err != nil {
		return fmt.Errorf("erro ao registrar no registry: %w", err)
	}

	log.Printf("[ReloadHTTPAgent] ✅ Agente %s recarregado com sucesso", full.Name)
	
	// Verifica se foi registrado corretamente
	agent := a.registry.Get(full.Name)
	if agent != nil {
		tools := agent.GetTools()
		log.Printf("[ReloadHTTPAgent] 🔧 Agente %s agora tem %d tools disponíveis", full.Name, len(tools))
	}
	
	return nil
}

// ReloadMCPAgent recarrega um MCP Agent no registry (hot reload)
func (a *App) ReloadMCPAgent(mcpAgentID uint) error {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return fmt.Errorf("erro ao buscar MCP agent: %w", err)
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return fmt.Errorf("erro ao buscar config: %w", err)
	}

	// Remove do registry se já estiver conectado
	existing := a.registry.GetMCPAgent(agentConfig.Name)
	if existing != nil {
		existing.Disconnect()
		a.registry.Unregister(agentConfig.Name)
	}

	// Só recarrega se estiver habilitado e autoConnect
	if !agentConfig.Enabled || !mcpAgent.AutoConnect {
		log.Printf("[ReloadMCPAgent] Agente %s não será carregado (enabled=%v, autoConnect=%v)",
			agentConfig.Name, agentConfig.Enabled, mcpAgent.AutoConnect)
		return nil
	}

	// Registra e conecta
	if err := a.registerMCPAgentInRegistry(agentConfig, mcpAgent); err != nil {
		return fmt.Errorf("erro ao registrar no registry: %w", err)
	}

	log.Printf("[ReloadMCPAgent] ✅ Agente %s recarregado com sucesso", agentConfig.Name)
	return nil
}

// TestHotReload cria um agente de teste temporário para validar o hot reload
func (a *App) TestHotReload() (string, error) {
	agentName := fmt.Sprintf("test_hotreload_%d", time.Now().Unix())
	
	log.Printf("🧪 [TestHotReload] Criando agente de teste: %s", agentName)
	
	// Cria agente de teste
	agent, err := a.CreateHTTPAgentFull(
		agentName,
		"Hot Reload Test Agent",
		"Agente temporário para testar hot reload",
		"gpt-4o-mini",
		"Você é um agente de teste para validar hot reload",
		true,
		"https://httpbin.org",
		"none",
		"{}",
		`{"Content-Type": "application/json"}`,
		10,
		1,
	)
	if err != nil {
		return "", fmt.Errorf("erro ao criar agente de teste: %w", err)
	}
	
	log.Printf("✅ [TestHotReload] Agente criado com ID: %d", agent.ID)
	
	// Cria endpoint de teste
	endpoint, err := a.CreateHTTPEndpoint(
		agent.HTTPAgentID,
		"test_get",
		"Endpoint de teste que retorna informações",
		"GET",
		"/get",
		"",
		"",
		"",
		`{"type":"object","properties":{"test_param":{"type":"string","description":"Parâmetro de teste"}},"required":[]}`,
		"",
	)
	if err != nil {
		return "", fmt.Errorf("erro ao criar endpoint: %w", err)
	}
	
	log.Printf("✅ [TestHotReload] Endpoint criado: %s", endpoint.Name)
	
	// Força reload para testar
	time.Sleep(500 * time.Millisecond) // Aguarda propagação
	
	if err := a.ReloadHTTPAgent(agent.ID); err != nil {
		return "", fmt.Errorf("erro no hot reload: %w", err)
	}
	
	// Verifica se o agente está no registry
	registeredAgent := a.registry.Get(agentName)
	if registeredAgent == nil {
		return "", fmt.Errorf("agente não foi registrado no registry após reload")
	}
	
	tools := registeredAgent.GetTools()
	if len(tools) == 0 {
		return "", fmt.Errorf("agente registrado mas sem tools disponíveis")
	}
	
	result := fmt.Sprintf(`✅ Hot Reload Validado com Sucesso!

Agente: %s (ID: %d)
Status: Registrado no registry
Tools disponíveis: %d
Endpoint de teste: %s

O hot reload está funcionando corretamente:
- Agente foi criado no banco de dados
- Endpoint foi adicionado
- ReloadHTTPAgent foi executado
- Agente foi registrado no registry
- Tools ficaram disponíveis imediatamente

Você pode deletar este agente de teste usando o Agent Builder.`,
		agentName, agent.ID, len(tools), endpoint.Name)
	
	log.Printf("✅ [TestHotReload] Validação concluída com sucesso")
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
	log.Printf("[CreateHTTPAgent] Criando agente: %s", name)

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
		return nil, fmt.Errorf("erro ao criar agente: %w", err)
	}

	// Converte para HTTPAgentFullConfig (tipo UI)
	endpoints := make([]HTTPEndpointInfo, 0, len(httpData.Endpoints))
	for _, ep := range httpData.Endpoints {
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

	result := &HTTPAgentFullConfig{
		ID:             agentData.ID,
		Name:           agentData.Name,
		DisplayName:    agentData.DisplayName,
		Description:    agentData.Description,
		Model:          agentData.Model,
		SystemPrompt:   agentData.SystemPrompt,
		Enabled:        agentData.Enabled,
		HTTPAgentID:    httpData.ID,
		BaseURL:        httpData.BaseURL,
		AuthType:       httpData.AuthType,
		AuthConfig:     httpData.AuthConfig,
		DefaultHeaders: httpData.DefaultHeaders,
		TimeoutSeconds: httpData.TimeoutSeconds,
		RetryCount:     httpData.RetryCount,
		Endpoints:      endpoints,
	}

	// Hot reload: carrega no registry automaticamente se habilitado
	if enabled {
		go func() {
			if err := a.ReloadHTTPAgent(agentData.ID); err != nil {
				log.Printf("[CreateHTTPAgent] Erro ao carregar no registry: %v", err)
			} else {
				log.Printf("[CreateHTTPAgent] ✅ Carregado no registry: %s", name)
			}
		}()
	}

	return result, nil
}

// GetHTTPAgentFull retorna a configuração completa de um HTTP Agent
func (a *App) GetHTTPAgentFull(agentConfigID uint) (*HTTPAgentFullConfig, error) {
	// Busca via manager
	agentData, err := a.agentManager.GetAgentByID(agentConfigID)
	if err != nil {
		return nil, fmt.Errorf("AgentConfig não encontrado: %w", err)
	}

	httpData, err := a.agentManager.GetHTTPAgent(agentConfigID)
	if err != nil {
		return nil, fmt.Errorf("HTTPAgent não encontrado: %w", err)
	}

	// Converte endpoints
	endpoints := make([]HTTPEndpointInfo, 0, len(httpData.Endpoints))
	for _, ep := range httpData.Endpoints {
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
		ID:             agentData.ID,
		Name:           agentData.Name,
		DisplayName:    agentData.DisplayName,
		Description:    agentData.Description,
		Model:          agentData.Model,
		SystemPrompt:   agentData.SystemPrompt,
		Enabled:        agentData.Enabled,
		HTTPAgentID:    httpData.ID,
		BaseURL:        httpData.BaseURL,
		AuthType:       httpData.AuthType,
		AuthConfig:     httpData.AuthConfig,
		DefaultHeaders: httpData.DefaultHeaders,
		TimeoutSeconds: httpData.TimeoutSeconds,
		RetryCount:     httpData.RetryCount,
		Endpoints:      endpoints,
	}, nil
}

// GetAllHTTPAgentsFull retorna todos os HTTP Agents com config completa
func (a *App) GetAllHTTPAgentsFull() ([]HTTPAgentFullConfig, error) {
	// Busca todos os agentes via manager
	allAgents, err := a.agentManager.GetAllAgents()
	if err != nil {
		return nil, err
	}

	result := make([]HTTPAgentFullConfig, 0)
	for _, agentData := range allAgents {
		if agentData.AgentType == "http" {
			full, err := a.GetHTTPAgentFull(agentData.ID)
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

	// Busca config antiga para saber o nome do agente
	oldConfig, err := a.agentManager.GetAgentByID(agentConfigID)
	if err == nil {
		// Remove do registry se já estiver registrado
		a.registry.Unregister(oldConfig.Name)
		log.Printf("[UpdateHTTPAgent] Removido do registry: %s", oldConfig.Name)
	}

	req := agentmanager.UpdateAgentRequest{
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

	agentData, err := a.agentManager.UpdateAgent(agentConfigID, req)
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar agente: %w", err)
	}

	// Busca a config completa atualizada
	full, err := a.GetHTTPAgentFull(agentData.ID)
	if err != nil {
		return nil, err
	}

	// Hot reload: recarrega no registry se estiver habilitado
	if enabled {
		go func() {
			if err := a.registerHTTPAgentInRegistry(full); err != nil {
				log.Printf("[UpdateHTTPAgent] Erro ao recarregar no registry: %v", err)
			} else {
				log.Printf("[UpdateHTTPAgent] Recarregado no registry: %s", full.Name)
			}
		}()
	}

	return full, nil
}

// DeleteHTTPAgentFull deleta um HTTP Agent completo
func (a *App) DeleteHTTPAgentFull(agentConfigID uint) error {
	// Busca o nome do agente para remover do registry
	agentConfig, err := a.agentManager.GetAgentByID(agentConfigID)
	if err == nil {
		a.registry.Unregister(agentConfig.Name)
		log.Printf("[DeleteHTTPAgent] Removido do registry: %s", agentConfig.Name)
	}

	return a.agentManager.DeleteAgent(agentConfigID)
}

// ValidateTemplate valida um template Go
func (a *App) ValidateTemplate(templateStr string) (bool, string) {
	engine := agentmanager.NewTemplateEngine()
	err := engine.ValidateTemplate(templateStr)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

// ExtractTemplateVariables extrai as variáveis usadas em um template
func (a *App) ExtractTemplateVariables(templateStr string) []string {
	engine := agentmanager.NewTemplateEngine()
	return engine.ExtractVariables(templateStr)
}

// TestHTTPEndpoint testa um endpoint HTTP com parâmetros
func (a *App) TestHTTPEndpoint(httpAgentID uint, endpointName string, paramsJSON string) (string, error) {
	return a.agentManager.TestHTTPEndpoint(httpAgentID, endpointName, paramsJSON)
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
