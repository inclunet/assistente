package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"assistente/internal/agents"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/faq"
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	"assistente/internal/memory"
	"assistente/internal/speech"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	libhotkey "golang.design/x/hotkey"
)

// App struct
type App struct {
	ctx               context.Context
	registry          *agents.Registry
	llmClient         *llm.SyncClient
	embeddingsService *llm.EmbeddingsService
	speechManager     *speech.SpeechManager
	hotkeyManager     *hotkey.Manager
	voiceHotkeyID     int
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		registry: agents.NewRegistry(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Inicializa o banco de dados
	if err := InitDatabase(); err != nil {
		log.Printf("Erro ao inicializar banco de dados: %v", err)
	}

	// Inicializa o cliente LLM para os agentes
	a.initLLMClient()

	// Inicializa os agentes
	a.initAgents()

	// Inicializa hotkeys globais
	a.initGlobalHotkeys()
}

// initLLMClient inicializa o cliente LLM usado pelos agentes
func (a *App) initLLMClient() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Erro ao carregar config para LLM: %v", err)
		return
	}

	if cfg.APIKey == "" {
		log.Printf("API Key não configurada - agentes não poderão usar LLM")
		return
	}

	a.llmClient = llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey)
	log.Printf("LLM Client inicializado para agentes")

	// Inicializa serviço de embeddings
	embeddingsModel := cfg.EmbeddingsModel
	if embeddingsModel == "" {
		embeddingsModel = "text-embedding-3-small" // Padrão OpenAI
	}

	a.embeddingsService = llm.NewEmbeddingsService(llm.EmbeddingsConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.APIBaseURL,
		Model:   embeddingsModel,
	})

	// Configura o gerador de embeddings no database para busca semântica
	database.SetEmbeddingGenerator(a.embeddingsService)

	log.Printf("Embeddings Service inicializado (modelo: %s)", embeddingsModel)
}

// initAgents registra todos os agentes disponíveis
func (a *App) initAgents() {
	// Modelo padrão para agentes (mais barato que o principal)
	agentModel := "gpt-4o-mini"

	// Agente FAQ
	faqStore := faq.NewStore()
	faqAgent := agents.NewFAQAgent(faqStore, a.llmClient, agentModel)
	a.applyAgentConfig(faqAgent)
	a.registry.Register(faqAgent)

	// Agente Memory
	memoryStore := memory.NewStore()
	memoryAgent := agents.NewMemoryAgent(memoryStore, a.llmClient, agentModel)
	a.applyAgentConfig(memoryAgent)
	a.registry.Register(memoryAgent)

	// Agente de Geração de Imagens (DALL-E)
	// Usa as credenciais do llmClient
	if a.llmClient != nil {
		imageAgent := agents.NewImageAgent(a.llmClient.APIKey, a.llmClient.BaseURL, a.llmClient)
		a.applyAgentConfig(imageAgent)
		a.registry.Register(imageAgent)
	}

	// Carrega MCP Agents salvos no banco
	a.loadSavedMCPAgents()

	log.Printf("Agentes registrados: %d", len(a.registry.GetAll()))
}

// loadSavedMCPAgents carrega e conecta MCP Agents salvos no banco
func (a *App) loadSavedMCPAgents() {
	mcpAgents, err := a.GetAllMCPAgents()
	if err != nil {
		log.Printf("Erro ao carregar MCP Agents do banco: %v", err)
		return
	}

	for _, mcp := range mcpAgents {
		// Busca a configuração do agente
		agentConfig, err := a.GetAgentConfigByID(mcp.AgentConfigID)
		if err != nil {
			log.Printf("Erro ao buscar config do MCP Agent %d: %v", mcp.ID, err)
			continue
		}

		// Só carrega se estiver habilitado e com auto_connect
		if !agentConfig.Enabled {
			log.Printf("MCP Agent %s desabilitado, pulando", agentConfig.Name)
			continue
		}

		if !mcp.AutoConnect {
			log.Printf("MCP Agent %s sem auto_connect, pulando", agentConfig.Name)
			continue
		}

		// Registra e conecta em background para não bloquear a inicialização
		go func(ac *AgentConfig, m MCPAgentDB) {
			log.Printf("Registrando MCP Agent: %s", ac.Name)
			if err := a.registerMCPAgentInRegistry(ac, &m); err != nil {
				// O agente foi registrado, mas a conexão falhou
				// A conexão será tentada novamente quando o agente for usado
				log.Printf("MCP Agent %s registrado (conexão pendente: %v)", ac.Name, err)
			} else {
				log.Printf("MCP Agent %s registrado e conectado com sucesso", ac.Name)
			}
		}(agentConfig, mcp)
	}
}

// applyAgentConfig aplica configurações salvas no banco ao agente
func (a *App) applyAgentConfig(agent agents.Agent) {
	config, err := a.GetAgentConfig(agent.GetName())
	if err != nil {
		// Sem configuração salva, usa padrão do código
		return
	}

	// Aplica configurações do banco se existirem
	if config.Model != "" {
		agent.SetModel(config.Model)
	}
	if config.SystemPrompt != "" {
		agent.SetSystemPrompt(config.SystemPrompt)
	}
	if config.DisplayName != "" {
		agent.SetDisplayName(config.DisplayName)
	}
	if config.Description != "" {
		agent.SetDescription(config.Description)
	}
	agent.SetEnabled(config.Enabled)

	log.Printf("Configuração do banco aplicada ao agente: %s", agent.GetName())
}

// GetAgentRegistry retorna o registry de agentes (para uso interno)
func (a *App) GetAgentRegistry() *agents.Registry {
	return a.registry
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
	// Reinicializa os agentes com o novo cliente
	a.registry = agents.NewRegistry()
	a.initAgents()
}

// shutdown é chamado quando o app fecha
func (a *App) shutdown(ctx context.Context) {
	// Desconecta todos os MCP agents
	if a.registry != nil {
		a.registry.DisconnectMCPAgents()
	}

	// Para hotkeys globais
	if a.hotkeyManager != nil {
		a.hotkeyManager.Stop()
	}
}

// initGlobalHotkeys configura os hotkeys globais
func (a *App) initGlobalHotkeys() {
	if !hotkey.IsSupported() {
		log.Println("Hotkeys globais não suportados neste sistema")
		return
	}

	a.hotkeyManager = hotkey.GetManager()

	// Registra hotkey padrão: Ctrl+Shift+A para ativar voz
	id, err := a.hotkeyManager.Register(
		[]libhotkey.Modifier{libhotkey.ModCtrl, libhotkey.ModShift},
		libhotkey.KeyA,
		func() {
			log.Println("Hotkey global acionado: Ctrl+Shift+A")
			// Traz a janela para frente
			runtime.WindowShow(a.ctx)
			runtime.WindowSetAlwaysOnTop(a.ctx, true)
			runtime.WindowSetAlwaysOnTop(a.ctx, false) // Remove always on top mas mantém foco

			// Emite evento para o frontend ativar o microfone
			runtime.EventsEmit(a.ctx, "global:hotkey:voice")
		},
	)

	if err != nil {
		log.Printf("Erro ao registrar hotkey global: %v", err)
		return
	}

	a.voiceHotkeyID = id
	log.Printf("Hotkey global registrado: Ctrl+Shift+A (ID=%d)", id)
}

// ============================================================================
// Global Hotkey API
// ============================================================================

// HotkeyInfo informações sobre um hotkey
type HotkeyInfo struct {
	ID          int    `json:"id"`
	Modifiers   string `json:"modifiers"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// IsGlobalHotkeySupported verifica se hotkeys globais são suportados
func (a *App) IsGlobalHotkeySupported() bool {
	return hotkey.IsSupported()
}

// GetGlobalHotkeys retorna os hotkeys globais configurados
func (a *App) GetGlobalHotkeys() []HotkeyInfo {
	return []HotkeyInfo{
		{
			ID:          a.voiceHotkeyID,
			Modifiers:   "Ctrl+Shift",
			Key:         "A",
			Description: "Ativar captura de voz",
			Enabled:     a.voiceHotkeyID > 0,
		},
	}
}

// SetVoiceHotkey altera o hotkey de voz
func (a *App) SetVoiceHotkey(modifiers string, key string) error {
	if a.hotkeyManager == nil {
		return fmt.Errorf("hotkey manager not initialized")
	}

	// Remove hotkey anterior
	if a.voiceHotkeyID > 0 {
		a.hotkeyManager.Unregister(a.voiceHotkeyID)
	}

	// Parse dos modificadores e tecla
	mods := hotkey.ParseModifiersString(modifiers)
	vk, err := hotkey.ParseKeyString(key)
	if err != nil {
		return err
	}

	// Registra novo hotkey
	id, err := a.hotkeyManager.Register(mods, vk, func() {
		log.Printf("Hotkey global acionado: %s+%s", modifiers, key)
		runtime.WindowShow(a.ctx)
		runtime.WindowSetAlwaysOnTop(a.ctx, true)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
		runtime.EventsEmit(a.ctx, "global:hotkey:voice")
	})

	if err != nil {
		return err
	}

	a.voiceHotkeyID = id
	log.Printf("Novo hotkey de voz configurado: %s+%s (ID=%d)", modifiers, key, id)
	return nil
}

// DisableVoiceHotkey desativa o hotkey de voz
func (a *App) DisableVoiceHotkey() error {
	if a.hotkeyManager == nil || a.voiceHotkeyID == 0 {
		return nil
	}

	err := a.hotkeyManager.Unregister(a.voiceHotkeyID)
	if err != nil {
		return err
	}

	a.voiceHotkeyID = 0
	log.Println("Hotkey de voz desativado")
	return nil
}

// EnableVoiceHotkey reativa o hotkey de voz com configuração padrão
func (a *App) EnableVoiceHotkey() error {
	return a.SetVoiceHotkey("Ctrl+Shift", "A")
}

// ==================== MCP Agent Functions ====================

// CreateMCPAgentFull cria um MCP Agent com sua configuração completa
func (a *App) CreateMCPAgentFull(name, displayName, description, model, systemPrompt, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode string, autoConnect, enabled bool) (map[string]interface{}, error) {
	// Primeiro, cria o AgentConfig
	agentConfig, err := a.CreateAgentConfig(name, displayName, description, "mcp", model, systemPrompt, "", enabled)
	if err != nil {
		return nil, err
	}

	// Depois, cria o MCPAgentDB
	mcpAgent, err := a.CreateMCPAgent(agentConfig.ID, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode, autoConnect)
	if err != nil {
		// Rollback: deleta o agent config
		a.DeleteAgentConfig(agentConfig.ID)
		return nil, err
	}

	// Se autoConnect, registra e conecta o agente em background (não bloqueia)
	if autoConnect && enabled {
		go func() {
			if err := a.registerMCPAgentInRegistry(agentConfig, mcpAgent); err != nil {
				log.Printf("Erro ao conectar MCP agent %s: %v", name, err)
			} else {
				log.Printf("MCP agent %s conectado com sucesso", name)
			}
		}()
	}

	return map[string]interface{}{
		"id":           mcpAgent.ID,
		"agent_config": agentConfig,
		"mcp_config":   mcpAgent,
	}, nil
}

// UpdateMCPAgentFull atualiza um MCP Agent completo
func (a *App) UpdateMCPAgentFull(mcpAgentID uint, displayName, description, model, systemPrompt, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode string, autoConnect, enabled bool) (map[string]interface{}, error) {
	// Busca o MCP agent existente
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return nil, err
	}

	// Busca o agent config
	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return nil, err
	}

	// Desconecta o agente antigo se estiver conectado
	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent != nil {
		existingAgent.Disconnect()
		a.registry.Unregister(agentConfig.Name)
	}

	// Atualiza o AgentConfig
	agentConfig, err = a.UpdateAgentConfig(agentConfig.ID, displayName, description, model, systemPrompt, "", enabled)
	if err != nil {
		return nil, err
	}

	// Atualiza o MCPAgentDB
	mcpAgent, err = a.UpdateMCPAgent(mcpAgentID, transportType, serverCommand, serverArgs, serverEnv, workingDir, serverURL, authType, authValue, httpHeaders, executionMode, autoConnect)
	if err != nil {
		return nil, err
	}

	// Se autoConnect e enabled, reconecta em background (não bloqueia)
	if autoConnect && enabled {
		go func() {
			if err := a.registerMCPAgentInRegistry(agentConfig, mcpAgent); err != nil {
				log.Printf("Erro ao reconectar MCP agent %s: %v", agentConfig.Name, err)
			} else {
				log.Printf("MCP agent %s reconectado com sucesso", agentConfig.Name)
			}
		}()
	}

	return map[string]interface{}{
		"id":           mcpAgent.ID,
		"agent_config": agentConfig,
		"mcp_config":   mcpAgent,
	}, nil
}

// DeleteMCPAgentFull deleta um MCP Agent e sua configuração
func (a *App) DeleteMCPAgentFull(mcpAgentID uint) error {
	// Busca o MCP agent
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return err
	}

	// Busca o agent config
	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err == nil {
		// Desconecta e remove do registry
		existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
		if existingAgent != nil {
			existingAgent.Disconnect()
			a.registry.Unregister(agentConfig.Name)
		}
	}

	// Deleta o MCP agent
	if err := a.DeleteMCPAgent(mcpAgentID); err != nil {
		return err
	}

	// Deleta o agent config
	if agentConfig != nil {
		return a.DeleteAgentConfig(agentConfig.ID)
	}

	return nil
}

// ConnectMCPAgent conecta um MCP Agent pelo ID
func (a *App) ConnectMCPAgent(mcpAgentID uint) error {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return err
	}

	// Verifica se já está conectado
	if existing := a.registry.GetMCPAgent(agentConfig.Name); existing != nil {
		return nil // Já conectado
	}

	return a.registerMCPAgentInRegistry(agentConfig, mcpAgent)
}

// DisconnectMCPAgent desconecta um MCP Agent pelo ID
func (a *App) DisconnectMCPAgent(mcpAgentID uint) error {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent != nil {
		existingAgent.Disconnect()
		a.registry.Unregister(agentConfig.Name)
	}

	return nil
}

// GetMCPAgentStatus retorna o status de conexão de um MCP Agent
func (a *App) GetMCPAgentStatus(mcpAgentID uint) (map[string]interface{}, error) {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return nil, err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return nil, err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	connected := existingAgent != nil && existingAgent.IsConnected()

	result := map[string]interface{}{
		"connected": connected,
		"name":      agentConfig.Name,
	}

	if existingAgent != nil {
		// Verifica estado detalhado
		if connected {
			serverInfo := existingAgent.GetServerInfo()
			if serverInfo != nil {
				result["server_info"] = map[string]interface{}{
					"name":             serverInfo.Name,
					"version":          serverInfo.Version,
					"protocol_version": serverInfo.ProtocolVersion,
				}
			}
			result["tools"] = existingAgent.GetMCPTools()

			// Tenta ping para verificar conexão real
			if err := existingAgent.Ping(); err != nil {
				result["ping_status"] = "failed"
				result["ping_error"] = err.Error()
			} else {
				result["ping_status"] = "ok"
			}
		}

		// Informações de diagnóstico (mesmo quando desconectado)
		result["transport_type"] = string(existingAgent.TransportType)
	}

	return result, nil
}

// TestMCPAgent testa um MCP Agent com uma tarefa
func (a *App) TestMCPAgent(mcpAgentID uint, task string) (string, error) {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return "", err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return "", err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent == nil {
		return "", fmt.Errorf("MCP agent não está conectado")
	}

	return existingAgent.Execute(a.ctx, task)
}

// ==================== MCP Resources API ====================

// MCPResourceInfo representa um resource MCP para a UI
type MCPResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mime_type"`
}

// MCPResourceTemplateInfo representa um template de resource para a UI
type MCPResourceTemplateInfo struct {
	URITemplate string `json:"uri_template"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mime_type"`
}

// MCPResourceContentInfo representa o conteúdo de um resource para a UI
type MCPResourceContentInfo struct {
	URI      string `json:"uri"`
	MimeType string `json:"mime_type"`
	Text     string `json:"text"`
	IsBlob   bool   `json:"is_blob"`
}

// GetMCPResources retorna os resources disponíveis de um MCP Agent
func (a *App) GetMCPResources(mcpAgentID uint) ([]MCPResourceInfo, error) {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return nil, err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return nil, err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent == nil {
		return nil, fmt.Errorf("MCP agent não está conectado")
	}

	transport := existingAgent.GetTransport()
	if transport == nil {
		return nil, fmt.Errorf("transporte MCP não disponível")
	}

	resources, err := transport.ListResources()
	if err != nil {
		return nil, err
	}

	result := make([]MCPResourceInfo, len(resources))
	for i, r := range resources {
		result[i] = MCPResourceInfo{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
		}
	}

	return result, nil
}

// GetMCPResourceTemplates retorna os templates de resources de um MCP Agent
func (a *App) GetMCPResourceTemplates(mcpAgentID uint) ([]MCPResourceTemplateInfo, error) {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return nil, err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return nil, err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent == nil {
		return nil, fmt.Errorf("MCP agent não está conectado")
	}

	transport := existingAgent.GetTransport()
	if transport == nil {
		return nil, fmt.Errorf("transporte MCP não disponível")
	}

	templates, err := transport.ListResourceTemplates()
	if err != nil {
		return nil, err
	}

	result := make([]MCPResourceTemplateInfo, len(templates))
	for i, t := range templates {
		result[i] = MCPResourceTemplateInfo{
			URITemplate: t.URITemplate,
			Name:        t.Name,
			Description: t.Description,
			MimeType:    t.MimeType,
		}
	}

	return result, nil
}

// ReadMCPResource lê o conteúdo de um resource de um MCP Agent
func (a *App) ReadMCPResource(mcpAgentID uint, uri string) (*MCPResourceContentInfo, error) {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return nil, err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return nil, err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent == nil {
		return nil, fmt.Errorf("MCP agent não está conectado")
	}

	transport := existingAgent.GetTransport()
	if transport == nil {
		return nil, fmt.Errorf("transporte MCP não disponível")
	}

	content, err := transport.ReadResource(uri)
	if err != nil {
		return nil, err
	}

	return &MCPResourceContentInfo{
		URI:      content.URI,
		MimeType: content.MimeType,
		Text:     content.Text,
		IsBlob:   content.Blob != "",
	}, nil
}

// ==================== MCP Prompts API ====================

// MCPPromptInfo representa um prompt MCP para a UI
type MCPPromptInfo struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Arguments   []MCPPromptArgInfo `json:"arguments"`
}

// MCPPromptArgInfo representa um argumento de prompt para a UI
type MCPPromptArgInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// MCPPromptResultInfo representa o resultado de um prompt expandido
type MCPPromptResultInfo struct {
	Description string                 `json:"description"`
	Messages    []MCPPromptMessageInfo `json:"messages"`
}

// MCPPromptMessageInfo representa uma mensagem de prompt para a UI
type MCPPromptMessageInfo struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GetMCPPrompts retorna os prompts disponíveis de um MCP Agent
func (a *App) GetMCPPrompts(mcpAgentID uint) ([]MCPPromptInfo, error) {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return nil, err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return nil, err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent == nil {
		return nil, fmt.Errorf("MCP agent não está conectado")
	}

	transport := existingAgent.GetTransport()
	if transport == nil {
		return nil, fmt.Errorf("transporte MCP não disponível")
	}

	prompts, err := transport.ListPrompts()
	if err != nil {
		return nil, err
	}

	result := make([]MCPPromptInfo, len(prompts))
	for i, p := range prompts {
		args := make([]MCPPromptArgInfo, len(p.Arguments))
		for j, arg := range p.Arguments {
			args[j] = MCPPromptArgInfo{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
			}
		}
		result[i] = MCPPromptInfo{
			Name:        p.Name,
			Description: p.Description,
			Arguments:   args,
		}
	}

	return result, nil
}

// GetMCPPrompt obtém um prompt expandido de um MCP Agent
func (a *App) GetMCPPrompt(mcpAgentID uint, promptName string, arguments map[string]string) (*MCPPromptResultInfo, error) {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return nil, err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return nil, err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent == nil {
		return nil, fmt.Errorf("MCP agent não está conectado")
	}

	transport := existingAgent.GetTransport()
	if transport == nil {
		return nil, fmt.Errorf("transporte MCP não disponível")
	}

	promptResult, err := transport.GetPrompt(promptName, arguments)
	if err != nil {
		return nil, err
	}

	messages := make([]MCPPromptMessageInfo, len(promptResult.Messages))
	for i, m := range promptResult.Messages {
		messages[i] = MCPPromptMessageInfo{
			Role:    m.Role,
			Content: m.Content.Text,
		}
	}

	return &MCPPromptResultInfo{
		Description: promptResult.Description,
		Messages:    messages,
	}, nil
}

// ==================== MCP Sampling API ====================

// MCPSamplingRequestInfo representa uma requisição de sampling para a UI
type MCPSamplingRequestInfo struct {
	Messages     []MCPSamplingMessageInfo `json:"messages"`
	SystemPrompt string                   `json:"system_prompt"`
	MaxTokens    int                      `json:"max_tokens"`
	Temperature  *float64                 `json:"temperature"`
}

// MCPSamplingMessageInfo representa uma mensagem de sampling para a UI
type MCPSamplingMessageInfo struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MCPSamplingResultInfo representa o resultado de sampling para a UI
type MCPSamplingResultInfo struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
}

// CreateMCPMessage solicita ao servidor MCP que crie uma mensagem via seu LLM
func (a *App) CreateMCPMessage(mcpAgentID uint, request MCPSamplingRequestInfo) (*MCPSamplingResultInfo, error) {
	mcpAgent, err := a.GetMCPAgent(mcpAgentID)
	if err != nil {
		return nil, err
	}

	agentConfig, err := a.GetAgentConfigByID(mcpAgent.AgentConfigID)
	if err != nil {
		return nil, err
	}

	existingAgent := a.registry.GetMCPAgent(agentConfig.Name)
	if existingAgent == nil {
		return nil, fmt.Errorf("MCP agent não está conectado")
	}

	transport := existingAgent.GetTransport()
	if transport == nil {
		return nil, fmt.Errorf("transporte MCP não disponível")
	}

	// Converte mensagens para formato MCP
	messages := make([]agents.MCPSamplingMessage, len(request.Messages))
	for i, m := range request.Messages {
		messages[i] = agents.MCPSamplingMessage{
			Role: m.Role,
			Content: agents.MCPContent{
				Type: "text",
				Text: m.Content,
			},
		}
	}

	samplingRequest := &agents.MCPSamplingRequest{
		Messages:     messages,
		SystemPrompt: request.SystemPrompt,
		MaxTokens:    request.MaxTokens,
		Temperature:  request.Temperature,
	}

	result, err := transport.CreateMessage(samplingRequest)
	if err != nil {
		return nil, err
	}

	return &MCPSamplingResultInfo{
		Role:       result.Role,
		Content:    result.Content.Text,
		Model:      result.Model,
		StopReason: result.StopReason,
	}, nil
}

// registerMCPAgentInRegistry cria e registra um MCP Agent no registry
func (a *App) registerMCPAgentInRegistry(agentConfig *AgentConfig, mcpAgentDB *MCPAgentDB) error {
	// Parse dos argumentos (para stdio)
	var serverArgs []string
	if mcpAgentDB.ServerArgs != "" {
		if err := json.Unmarshal([]byte(mcpAgentDB.ServerArgs), &serverArgs); err != nil {
			serverArgs = []string{}
		}
	}

	// Parse das variáveis de ambiente (para stdio)
	var serverEnv []string
	if mcpAgentDB.ServerEnv != "" {
		if err := json.Unmarshal([]byte(mcpAgentDB.ServerEnv), &serverEnv); err != nil {
			serverEnv = []string{}
		}
	}

	// Parse dos headers HTTP
	var httpHeaders map[string]string
	if mcpAgentDB.HTTPHeaders != "" {
		if err := json.Unmarshal([]byte(mcpAgentDB.HTTPHeaders), &httpHeaders); err != nil {
			httpHeaders = map[string]string{}
		}
	}

	// Cria o MCP Agent
	config := agents.MCPAgentConfig{
		Name:          agentConfig.Name,
		DisplayName:   agentConfig.DisplayName,
		Description:   agentConfig.Description,
		Model:         agentConfig.Model,
		SystemPrompt:  agentConfig.SystemPrompt,
		TransportType: agents.MCPTransportType(mcpAgentDB.TransportType),
		ServerCommand: mcpAgentDB.ServerCommand,
		ServerArgs:    serverArgs,
		ServerEnv:     serverEnv,
		ServerURL:     mcpAgentDB.ServerURL,
		AuthType:      mcpAgentDB.AuthType,
		AuthValue:     mcpAgentDB.AuthValue,
		HTTPHeaders:   httpHeaders,
		ExecutionMode: agents.MCPExecutionMode(mcpAgentDB.ExecutionMode),
	}

	agent := agents.NewMCPAgent(config, a.llmClient)

	// Registra e conecta
	return a.registry.RegisterMCPAgent(agent)
}

// ============================================================================
// SAPI5 Voice Methods (Windows only)
// ============================================================================

// SAPI5VoiceInfo representa informações de uma voz SAPI5 para o frontend
type SAPI5VoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Gender      string `json:"gender"`
	Age         string `json:"age"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// GetSAPI5Voices retorna a lista de vozes SAPI5 instaladas
// Em sistemas não-Windows, retorna lista vazia sem erro
func (a *App) GetSAPI5Voices() ([]SAPI5VoiceInfo, error) {
	manager := speech.GetSAPI5Manager()

	if err := manager.Initialize(); err != nil {
		log.Printf("SAPI5 Initialize error (may be expected on non-Windows): %v", err)
		return []SAPI5VoiceInfo{}, nil
	}

	voices := manager.GetVoices()
	result := make([]SAPI5VoiceInfo, len(voices))

	for i, v := range voices {
		result[i] = SAPI5VoiceInfo{
			ID:          v.ID,
			Name:        v.Name,
			Language:    v.Language,
			Gender:      v.Gender,
			Age:         v.Age,
			Vendor:      v.Vendor,
			Description: v.Description,
			Source:      v.Source,
		}
	}

	return result, nil
}

// SpeakSAPI5 sintetiza texto usando uma voz SAPI5
// Em sistemas não-Windows, não faz nada
func (a *App) SpeakSAPI5(text string, voiceName string) error {
	manager := speech.GetSAPI5Manager()
	return manager.Speak(text, voiceName)
}

// StopSAPI5 para a síntese SAPI5 atual
func (a *App) StopSAPI5() error {
	manager := speech.GetSAPI5Manager()
	return manager.Stop()
}

// SetSAPI5Volume define o volume (0-100)
func (a *App) SetSAPI5Volume(volume int) error {
	manager := speech.GetSAPI5Manager()
	return manager.SetVolume(volume)
}

// SetSAPI5Rate define a velocidade (-10 a 10, 0 é normal)
func (a *App) SetSAPI5Rate(rate int) error {
	manager := speech.GetSAPI5Manager()
	return manager.SetRate(rate)
}

// IsSAPI5Speaking verifica se está falando
func (a *App) IsSAPI5Speaking() bool {
	manager := speech.GetSAPI5Manager()
	return manager.IsSpeaking()
}

// ============================================================================
// OpenAI Speech API Methods (Whisper STT + OpenAI TTS)
// ============================================================================

// OpenAITTSVoiceInfo representa uma voz OpenAI TTS para o frontend
type OpenAITTSVoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Gender      string `json:"gender"`
	Provider    string `json:"provider"`
}

// TranscriptionResultInfo resultado da transcrição para o frontend
type TranscriptionResultInfo struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Provider string  `json:"provider"`
}

// SynthesisResultInfo resultado da síntese para o frontend
type SynthesisResultInfo struct {
	AudioBase64 string `json:"audioBase64"`
	Format      string `json:"format"`
	Provider    string `json:"provider"`
}

// InitSpeechManager inicializa o gerenciador de speech com as configurações
func (a *App) InitSpeechManager(apiKey, apiBaseURL, whisperLanguage, ttsVoice, ttsModel string) error {
	config := speech.SpeechConfig{
		STTProvider:      speech.STTProviderWhisper,
		TTSProvider:      speech.TTSProviderOpenAI,
		OpenAIAPIKey:     apiKey,
		OpenAIAPIBaseURL: apiBaseURL,
		WhisperModel:     "whisper-1",
		WhisperLanguage:  whisperLanguage,
		TTSModel:         ttsModel,
		TTSVoice:         ttsVoice,
	}

	a.speechManager = speech.NewSpeechManager(config)
	log.Printf("Speech Manager inicializado (STT: whisper, TTS: openai)")
	return nil
}

// TranscribeWhisper transcreve áudio usando OpenAI Whisper
// audioBase64: áudio codificado em base64
// filename: nome do arquivo com extensão (ex: "audio.webm")
func (a *App) TranscribeWhisper(audioBase64 string, filename string) (*TranscriptionResultInfo, error) {
	if a.speechManager == nil {
		// Tenta inicializar com as configurações salvas
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")
	}

	result, err := a.speechManager.Transcribe(audioBase64, filename)
	if err != nil {
		return nil, err
	}

	return &TranscriptionResultInfo{
		Text:     result.Text,
		Language: result.Language,
		Duration: result.Duration,
		Provider: result.Provider,
	}, nil
}

// SynthesizeOpenAI sintetiza texto usando OpenAI TTS
func (a *App) SynthesizeOpenAI(text string) (*SynthesisResultInfo, error) {
	if a.speechManager == nil {
		// Tenta inicializar com as configurações salvas
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")
	}

	result, err := a.speechManager.Synthesize(text)
	if err != nil {
		return nil, err
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// SynthesizeOpenAIWithVoice sintetiza texto usando OpenAI TTS com uma voz específica
func (a *App) SynthesizeOpenAIWithVoice(text string, voice string) (*SynthesisResultInfo, error) {
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voice, "tts-1")
	}

	result, err := a.speechManager.SynthesizeWithVoice(text, voice)
	if err != nil {
		return nil, err
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// GetOpenAITTSVoices retorna as vozes disponíveis do OpenAI TTS
func (a *App) GetOpenAITTSVoices() []OpenAITTSVoiceInfo {
	voices := speech.GetAvailableVoices()
	result := make([]OpenAITTSVoiceInfo, len(voices))

	for i, v := range voices {
		result[i] = OpenAITTSVoiceInfo{
			ID:          v.ID,
			Name:        v.Name,
			Description: v.Description,
			Gender:      v.Gender,
			Provider:    v.Provider,
		}
	}

	return result
}

// SetOpenAITTSVoice altera a voz do OpenAI TTS
func (a *App) SetOpenAITTSVoice(voice string) {
	if a.speechManager != nil {
		a.speechManager.SetTTSVoice(voice)
	}
}

// SetOpenAITTSSpeed altera a velocidade do OpenAI TTS
func (a *App) SetOpenAITTSSpeed(rate int) {
	if a.speechManager != nil {
		a.speechManager.SetTTSSpeed(rate)
	}
}
