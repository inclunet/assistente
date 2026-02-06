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
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/faq"
	"assistente/internal/filemanager"
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	"assistente/internal/memory"
	"assistente/internal/skills"
	"assistente/internal/speech"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx                   context.Context
	registry              *agents.Registry
	skillsRegistry        *skills.Registry // Registry de skills declarativas
	llmClient             *llm.SyncClient
	embeddingsService     *llm.EmbeddingsService
	summaryService        *llm.SummaryService
	speechManager         *speech.SpeechManager
	hotkeyManager         *hotkey.Manager
	agentManager          agentmanager.Manager // NOVO - Manager para agentes HTTP/MCP
	voiceHotkeyID         int
	InitialWorkDir        string // Diretório de trabalho inicial (passado via --workdir ou pwd)
	currentConversationID uint   // ID da conversa atual (para passar aos agentes)
	currentDelegationID   uint   // ID da mensagem de delegação atual (para ParentID)

	// Throttle para hotkeys - evita disparo repetido quando segura a tecla
	hotkeyLastFired  map[uint]time.Time
	hotkeyThrottleMs int64 // tempo mínimo entre disparos (em ms)
}

// ==================== Tipos para Threads ====================

// EnrichedMessage é ChatMessage + campos derivados calculados no backend
// Todos os campos são definidos explicitamente para evitar conflitos de embedding
type EnrichedMessage struct {
	ID               string    `json:"id"` // String para JS safety (números grandes)
	ConversationID   uint      `json:"conversationId"`
	ParentID         *string   `json:"parentId,omitempty"` // String para evitar undefined no TypeScript
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"` // Reasoning/thinking do modelo (DeepSeek, Claude, o1)
	Media            string    `json:"media,omitempty"`
	ToolCalls        string    `json:"toolCalls,omitempty"`
	ToolResults      string    `json:"toolResults,omitempty"`
	ToolCallID       string    `json:"toolCallId,omitempty"`
	AgentName        string    `json:"agentName,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	Model            string    `json:"model,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	Timestamp        int64     `json:"timestamp"`          // Milliseconds desde epoch
	IsStreaming      bool      `json:"isStreaming"`        // Sempre false do DB
	ToolName         string    `json:"toolName,omitempty"` // Nome do tool (extraído de toolCalls)
	Internal         bool      `json:"internal"`           // Se tem parentId (é resposta de thread)
}

// MessageNode representa uma mensagem com seus filhos na hierarquia
type MessageNode struct {
	Message    EnrichedMessage `json:"message"` // Mensagem enriquecida
	Children   []MessageNode   `json:"children,omitempty"`
	Level      int             `json:"level"`
	ChildCount int             `json:"childCount"` // Para lazy loading
}

// ConversationWithThreads representa uma conversa com mensagens organizadas em árvore
type ConversationWithThreads struct {
	ID          uint                      `json:"id"`
	Title       string                    `json:"title"`
	Preferences *database.ChatPreferences `json:"preferences,omitempty"`
	Threads     []MessageNode             `json:"threads"`
}

// StreamEvent representa um evento de streaming simplificado
type StreamEvent struct {
	MessageID      uint   `json:"messageId"`
	ConversationId uint   `json:"conversationId"`
	Content        string `json:"content"`
	Done           bool   `json:"done"`
	FullResponse   string `json:"fullResponse,omitempty"`
	Error          string `json:"error,omitempty"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		registry:         agents.NewRegistry(),
		skillsRegistry:   skills.NewRegistry(),
		hotkeyLastFired:  make(map[uint]time.Time),
		hotkeyThrottleMs: 1000, // 1000ms entre disparos (1 segundo)
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

	// Carrega skills declarativas
	a.loadSkills()

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

	// Configura o timeout de resposta HTTP baseado na config
	llm.ConfigureResponseTimeout(cfg.GetResponseTimeout())
	log.Printf("HTTP Response Timeout configurado para %d segundos", cfg.GetResponseTimeout())

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

	// Inicializa serviço de resumo (para embeddings de conversas)
	summaryModel := "gpt-4o-mini" // Modelo rápido e barato para resumos
	a.summaryService = llm.NewSummaryService(llm.SummaryConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.APIBaseURL,
		Model:   summaryModel,
	})

	// Configura o gerador de resumos no database
	database.SetSummaryGenerator(&summaryGeneratorAdapter{service: a.summaryService})

	log.Printf("Summary Service inicializado (modelo: %s)", summaryModel)
}

// summaryGeneratorAdapter adapta llm.SummaryService para database.SummaryGenerator
type summaryGeneratorAdapter struct {
	service *llm.SummaryService
}

func (a *summaryGeneratorAdapter) GenerateSummary(messages []database.ChatMessage) (string, error) {
	// Converte database.ChatMessage para llm.ChatMessage
	llmMessages := make([]llm.ChatMessage, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return a.service.GenerateSummary(llmMessages)
}

// loadSkills carrega skills declarativas do diretório ~/.assistente/skills/
func (a *App) loadSkills() {
	dir, err := skills.GetSkillsDir()
	if err != nil {
		log.Printf("[Skills] Erro ao obter diretório de skills: %v", err)
		return
	}

	if err := a.skillsRegistry.LoadFromDir(dir); err != nil {
		log.Printf("[Skills] Erro ao carregar skills: %v", err)
	}
}

// GetSkills retorna todas as skills carregadas (binding para frontend)
func (a *App) GetSkills() []*skills.Skill {
	return a.skillsRegistry.GetAll()
}

// GetSkillContent retorna o conteúdo de uma skill pelo nome (binding para frontend)
func (a *App) GetSkillContent(name string) (string, error) {
	skill := a.skillsRegistry.Get(name)
	if skill == nil {
		return "", fmt.Errorf("skill não encontrada: %s", name)
	}
	return skill.Content, nil
}

// ReloadSkills recarrega as skills do filesystem (binding para frontend)
func (a *App) ReloadSkills() error {
	return a.skillsRegistry.Reload()
}

// initAgents registra todos os agentes disponíveis
func (a *App) initAgents() {
	// Modelo padrão para agentes (mais barato que o principal)
	agentModel := "gpt-4o-mini"

	// Criar AgentManager (como faqStore, memoryStore)
	a.agentManager = agentmanager.New(database.DB())

	// Agente FAQ
	faqStore := faq.NewStore()
	faqAgent := agents.NewFAQAgent(faqStore, a.llmClient, agentModel)
	a.applyAgentConfig(faqAgent)
	a.registry.Register(faqAgent)

	// Agente Memory (também busca em abas e histórico)
	memoryStore := memory.NewStore()
	memoryAgent := agents.NewMemoryAgent(memoryStore, a.llmClient, agentModel)
	memoryAgent.SetContextSearcher(a) // Permite buscar em abas e histórico
	a.applyAgentConfig(memoryAgent)
	a.registry.Register(memoryAgent)

	// Agente Chat Manager (navegação, gerenciamento de abas e conversas)
	chatManagerAgent := agents.NewChatManagerAgent(a, a.llmClient, agentModel)
	a.applyAgentConfig(chatManagerAgent)
	a.registry.Register(chatManagerAgent)

	// Agente de Geração de Imagens (DALL-E)
	// Usa as credenciais do llmClient
	if a.llmClient != nil {
		imageAgent := agents.NewImageAgent(a.llmClient.APIKey, a.llmClient.BaseURL, a.llmClient)
		a.applyAgentConfig(imageAgent)
		a.registry.Register(imageAgent)
	}

	// Agente de Gerenciamento de Arquivos
	fileAgent := agents.NewFileAgent(a.llmClient, agentModel)
	a.applyAgentConfig(fileAgent)
	a.loadFileAgentAuthorizedPaths(fileAgent)
	// Configura diretório de trabalho inicial (se passado via --workdir ou detectado do terminal)
	if a.InitialWorkDir != "" {
		if err := fileAgent.SetWorkingDirectory(a.InitialWorkDir); err != nil {
			log.Printf("Aviso: Não foi possível definir diretório de trabalho inicial: %v", err)
		} else {
			log.Printf("FileAgent: Diretório de trabalho inicial: %s", a.InitialWorkDir)
		}
	}
	// Configura Google Docs se houver conexão OAuth ativa
	a.configureFileAgentGoogleDocs(fileAgent)
	a.registry.Register(fileAgent)

	// Agente de Navegação Web
	webAgentCfg := agents.WebAgentConfig{}
	webAgent := agents.NewWebAgentWithConfig(a.llmClient, agentModel, webAgentCfg)
	a.applyAgentConfig(webAgent)
	a.registry.Register(webAgent)

	// Agente de Busca Web (usa modelos de busca da OpenAI)
	if cfg, err := config.Load(); err == nil && cfg.APIKey != "" {
		webSearchCfg := agents.WebSearchAgentConfig{
			APIKey:     cfg.APIKey,
			APIBaseURL: cfg.APIBaseURL,
			Model:      cfg.WebSearchModel, // Modelo de busca configurável
		}
		webSearchAgent := agents.NewWebSearchAgent(a.llmClient, webSearchCfg)
		a.applyAgentConfig(webSearchAgent)
		a.registry.Register(webSearchAgent)
	}

	// Agente Builder (Cria e gerencia HTTP e MCP Agents)
	builderAgent := agents.NewBuilderAgent(a.agentManager, a.llmClient, agentModel)
	a.applyAgentConfig(builderAgent)
	// Configurar callbacks de hot reload
	builderAgent.SetReloadCallbacks(
		func(agentConfigID uint) error {
			return a.ReloadHTTPAgent(agentConfigID)
		},
		func(mcpAgentID uint) error {
			return a.ReloadMCPAgent(mcpAgentID)
		},
	)
	a.registry.Register(builderAgent)

	// Agente Profile (Gerencia Voice e Interaction Profiles)
	profileAgent := agents.NewProfileAgent(a.llmClient, agentModel)
	a.applyAgentConfig(profileAgent)
	// Configurar callbacks para ativação/desativação de perfis
	profileAgent.SetCallbacks(
		func(profileID uint) error {
			return a.RegisterInteractionProfileHotkeys(profileID)
		},
		func(profileID uint) error {
			return a.UnregisterInteractionProfileHotkeys(profileID)
		},
		func(event string, data interface{}) {
			runtime.EventsEmit(a.ctx, event, data)
		},
		func(conversationID, profileID uint) error {
			return a.SetConversationVoiceProfile(conversationID, profileID)
		},
	)
	a.registry.Register(profileAgent)

	// Carrega agentes personalizados salvos no banco
	a.loadSavedHTTPAgents()
	a.loadSavedMCPAgents()

	log.Printf("Agentes registrados: %d", len(a.registry.GetAll()))
}

// loadSavedHTTPAgents carrega e registra HTTP Agents salvos no banco
func (a *App) loadSavedHTTPAgents() {
	httpAgents, err := a.GetAllHTTPAgentsFull()
	if err != nil {
		log.Printf("Erro ao carregar HTTP Agents do banco: %v", err)
		return
	}

	for _, httpFull := range httpAgents {
		// Só carrega se estiver habilitado
		if !httpFull.Enabled {
			log.Printf("HTTP Agent %s desabilitado, pulando", httpFull.Name)
			continue
		}

		// Registra em background para não bloquear a inicialização
		go func(hf HTTPAgentFullConfig) {
			log.Printf("Registrando HTTP Agent: %s", hf.Name)
			if err := a.registerHTTPAgentInRegistry(&hf); err != nil {
				log.Printf("Erro ao registrar HTTP Agent %s: %v", hf.Name, err)
			} else {
				log.Printf("HTTP Agent %s registrado com sucesso", hf.Name)
			}
		}(httpFull)
	}
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

// configureFileAgentGoogleDocs configura o suporte a Google Docs no FileAgent
func (a *App) configureFileAgentGoogleDocs(fileAgent *agents.FileAgent) {
	// Cria uma função que obtém o token do Google via OAuth
	tokenProvider := func() (string, error) {
		return a.GetOAuthAccessTokenForProvider("google")
	}

	// Verifica se há uma conexão ativa do Google
	conn, err := a.GetActiveOAuthConnectionForProvider("google")
	if err != nil || conn == nil {
		log.Printf("FileAgent: Google Docs não configurado (sem conexão OAuth ativa)")
		return
	}

	// Verifica se a conexão tem os scopes necessários para Drive/Docs
	scopes := conn.Scopes
	hasAccess := strings.Contains(scopes, "drive") ||
		strings.Contains(scopes, "documents") ||
		strings.Contains(scopes, "spreadsheets")

	if !hasAccess {
		log.Printf("FileAgent: Conexão Google existe mas não tem scopes de Drive/Docs")
		return
	}

	fileAgent.SetGoogleTokenProvider(tokenProvider)
	log.Printf("FileAgent: Google Docs habilitado (conta: %s)", conn.UserEmail)
}

// loadFileAgentAuthorizedPaths carrega as pastas autorizadas para o FileAgent
func (a *App) loadFileAgentAuthorizedPaths(fileAgent *agents.FileAgent) {
	paths, err := database.GetAllFileAgentAuthorizedPaths()
	if err != nil {
		log.Printf("Erro ao carregar pastas autorizadas do FileAgent: %v", err)
		return
	}

	// Converte para o formato esperado pelo FileAgent
	var authorizedPaths []filemanager.AuthorizedPath
	for _, p := range paths {
		authorizedPaths = append(authorizedPaths, filemanager.AuthorizedPath{
			ID:          p.ID,
			Path:        p.Path,
			AllowDelete: p.AllowDelete,
			AllowWrite:  p.AllowWrite,
			Recursive:   p.Recursive,
		})
	}

	fileAgent.SetAuthorizedPaths(authorizedPaths)
	log.Printf("FileAgent: %d pastas autorizadas carregadas", len(authorizedPaths))

	// Configura callback para persistir mudanças nas autorizações
	agents.OnAuthorizationChange = func(paths []filemanager.AuthorizedPath) {
		a.syncFileAgentAuthorizedPaths(paths)
	}
}

// syncFileAgentAuthorizedPaths sincroniza as pastas autorizadas com o banco
func (a *App) syncFileAgentAuthorizedPaths(paths []filemanager.AuthorizedPath) {
	// Obtém paths atuais do banco
	existingPaths, err := database.GetAllFileAgentAuthorizedPaths()
	if err != nil {
		log.Printf("Erro ao obter pastas do banco: %v", err)
		return
	}

	// Cria mapa das paths existentes
	existingMap := make(map[string]database.FileAgentAuthorizedPath)
	for _, p := range existingPaths {
		existingMap[p.Path] = p
	}

	// Cria mapa das novas paths
	newMap := make(map[string]filemanager.AuthorizedPath)
	for _, p := range paths {
		newMap[p.Path] = p
	}

	// Remove paths que não existem mais
	for path, existing := range existingMap {
		if _, found := newMap[path]; !found {
			if err := database.DeleteFileAgentAuthorizedPath(existing.ID); err != nil {
				log.Printf("Erro ao remover autorização %s: %v", path, err)
			} else {
				log.Printf("FileAgent: Autorização removida: %s", path)
			}
		}
	}

	// Adiciona novas paths
	for path, newPath := range newMap {
		if _, found := existingMap[path]; !found {
			_, err := database.CreateFileAgentAuthorizedPath(
				newPath.Path,
				newPath.AllowDelete,
				newPath.AllowWrite,
				newPath.Recursive,
			)
			if err != nil {
				log.Printf("Erro ao criar autorização %s: %v", path, err)
			} else {
				log.Printf("FileAgent: Autorização criada: %s", path)
			}
		}
	}
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

// initGlobalHotkeys inicializa o gerenciador de hotkeys
// Os hotkeys são registrados pelos triggers dos perfis de interação
func (a *App) initGlobalHotkeys() {
	if !hotkey.IsSupported() {
		log.Println("[Hotkey] Hotkeys globais não suportados neste sistema")
		return
	}

	a.hotkeyManager = hotkey.GetManager()
	log.Println("[Hotkey] Manager inicializado. Hotkeys serão registrados pelos triggers dos perfis.")
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
// DEPRECATED: Use os triggers dos perfis de interação para configurar hotkeys
func (a *App) GetGlobalHotkeys() []HotkeyInfo {
	// Hotkeys são agora gerenciados pelos triggers dos perfis de interação
	return []HotkeyInfo{}
}

// SetVoiceHotkey altera o hotkey de voz
// DEPRECATED: Use os triggers dos perfis de interação para configurar hotkeys
func (a *App) SetVoiceHotkey(modifiers string, key string) error {
	return fmt.Errorf("deprecated: use interaction profile triggers to configure hotkeys")
}

// DisableVoiceHotkey desativa o hotkey de voz
// DEPRECATED: Use os triggers dos perfis de interação para configurar hotkeys
func (a *App) DisableVoiceHotkey() error {
	return nil
}

// EnableVoiceHotkey reativa o hotkey de voz com configuração padrão
// DEPRECATED: Use os triggers dos perfis de interação para configurar hotkeys
func (a *App) EnableVoiceHotkey() error {
	return fmt.Errorf("deprecated: use interaction profile triggers to configure hotkeys")
}

// ==================== Interaction Profile Hotkeys ====================

// RegisterInteractionProfileHotkeys registra os hotkeys de um perfil de interação
// Itera pelos triggers do perfil e registra hotkeys para triggers que possuem
func (a *App) RegisterInteractionProfileHotkeys(profileID uint) error {
	if a.hotkeyManager == nil {
		log.Printf("[Hotkey] Manager não inicializado!")
		return fmt.Errorf("hotkey manager not initialized")
	}

	// Busca o perfil com triggers
	profile, err := database.GetInteractionProfile(profileID)
	if err != nil {
		log.Printf("[Hotkey] Perfil %d não encontrado: %v", profileID, err)
		return fmt.Errorf("profile not found: %w", err)
	}

	log.Printf("[Hotkey] Perfil %d (%s) tem %d triggers", profileID, profile.Name, len(profile.Triggers))

	// Remove hotkeys anteriores deste perfil
	a.hotkeyManager.UnregisterProfileHotkeys(int(profileID))

	// Registra hotkeys para cada trigger que possui hotkey configurada
	hotkeyCount := 0
	for _, trigger := range profile.Triggers {
		log.Printf("[Hotkey] Trigger %d: type=%s, enabled=%v, hotkey='%s'", trigger.ID, trigger.Type, trigger.Enabled, trigger.Hotkey)
		if !trigger.Enabled || trigger.Hotkey == "" {
			log.Printf("[Hotkey] Trigger %d ignorado (enabled=%v, hotkey vazio=%v)", trigger.ID, trigger.Enabled, trigger.Hotkey == "")
			continue
		}
		hotkeyCount++

		// Captura variáveis para closure
		t := trigger

		log.Printf("[Hotkey] Registrando hotkey '%s' para trigger %d...", t.Hotkey, t.ID)
		_, err := a.hotkeyManager.RegisterProfileHotkey(
			int(profileID),
			t.Hotkey,
			t.Type == database.TriggerTypeHotkey, // isPrimary: só hotkey direto é "principal"
			t.HotkeyBringToFront,
			func() {
				// Throttle: ignora se disparou recentemente (evita loop quando segura tecla)
				now := time.Now()
				if lastFired, ok := a.hotkeyLastFired[t.ID]; ok {
					elapsed := now.Sub(lastFired).Milliseconds()
					if elapsed < a.hotkeyThrottleMs {
						log.Printf("[Hotkey] BLOQUEADO por throttle: trigger %d, elapsed=%dms < %dms", t.ID, elapsed, a.hotkeyThrottleMs)
						return // Ignora - muito rápido
					}
				}
				a.hotkeyLastFired[t.ID] = now

				log.Printf("[Hotkey] HOTKEY ACIONADA! Trigger %d, perfil %d (throttle OK)", t.ID, profileID)
				// Emite evento para frontend com informações do trigger
				runtime.EventsEmit(a.ctx, "interaction:hotkey:triggered", map[string]interface{}{
					"triggerId":    t.ID,
					"profileId":    profileID,
					"triggerType":  t.Type,
					"bringToFront": t.HotkeyBringToFront,
				})

				// Se deve trazer janela para frente
				if t.HotkeyGlobal && t.HotkeyBringToFront {
					runtime.WindowShow(a.ctx)
				}
			},
		)
		if err != nil {
			log.Printf("[Hotkey] ERRO ao registrar hotkey do trigger %d (perfil %d): %v", t.ID, profileID, err)
			// Continua para os outros triggers
		} else {
			log.Printf("[Hotkey] Hotkey '%s' registrada com sucesso para trigger %d", t.Hotkey, t.ID)
		}
	}

	log.Printf("[Hotkey] Total: %d hotkeys registradas para perfil %d", hotkeyCount, profileID)
	return nil
}

// UnregisterInteractionProfileHotkeys remove os hotkeys de um perfil
func (a *App) UnregisterInteractionProfileHotkeys(profileID uint) error {
	if a.hotkeyManager == nil {
		return nil
	}
	return a.hotkeyManager.UnregisterProfileHotkeys(int(profileID))
}

// GetActiveInteractionProfile retorna o perfil de interação atualmente ativo
func (a *App) GetActiveInteractionProfile() *database.InteractionProfile {
	profile, err := database.GetActiveInteractionProfile()
	if err != nil {
		log.Printf("[App] Erro ao buscar perfil ativo: %v", err)
		return nil
	}
	return profile
}

// SetActiveInteractionProfile define e ativa um perfil de interação
// Persiste no banco, registra os hotkeys do perfil e emite evento
func (a *App) SetActiveInteractionProfile(profileID uint) error {
	log.Printf("[SetActiveInteractionProfile] Ativando perfil %d", profileID)

	// Persiste no banco
	if err := database.SetActiveInteractionProfile(profileID); err != nil {
		log.Printf("[SetActiveInteractionProfile] Erro ao persistir: %v", err)
		return err
	}

	// Desregistra hotkeys de todos os perfis
	if a.hotkeyManager != nil {
		log.Printf("[SetActiveInteractionProfile] Desregistrando todas hotkeys...")
		a.hotkeyManager.UnregisterAllProfileHotkeys()
	} else {
		log.Printf("[SetActiveInteractionProfile] AVISO: hotkeyManager é nil!")
	}

	// Registra hotkeys do novo perfil (se não for 0 = desativado)
	if profileID > 0 {
		log.Printf("[SetActiveInteractionProfile] Registrando hotkeys do perfil %d...", profileID)
		if err := a.RegisterInteractionProfileHotkeys(profileID); err != nil {
			log.Printf("[SetActiveInteractionProfile] Erro ao registrar hotkeys: %v", err)
			return err
		}
	} else {
		log.Printf("[SetActiveInteractionProfile] Perfil 0 = desativado, não registrando hotkeys")
	}

	// Emite evento de mudança de perfil
	runtime.EventsEmit(a.ctx, "interaction:profile:activated", map[string]interface{}{
		"profileId": profileID,
	})

	return nil
}

// GetActiveProfileHotkeys retorna os hotkeys registrados para um perfil
func (a *App) GetActiveProfileHotkeys(profileID uint) []map[string]interface{} {
	hotkeys := hotkey.GetProfileHotkeys(int(profileID))
	result := make([]map[string]interface{}, 0, len(hotkeys))

	for _, hk := range hotkeys {
		result = append(result, map[string]interface{}{
			"profileId":    hk.ProfileID,
			"isPrimary":    hk.IsPrimary,
			"combination":  hk.Combination,
			"bringToFront": hk.BringToFront,
			"hotkeyId":     hk.HotkeyID,
		})
	}

	return result
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

// registerHTTPAgentInRegistry cria e registra um HTTP Agent no registry
func (a *App) registerHTTPAgentInRegistry(httpFull *HTTPAgentFullConfig) error {
	// Parse auth config
	var authConfig map[string]string
	if httpFull.AuthConfig != "" {
		if err := json.Unmarshal([]byte(httpFull.AuthConfig), &authConfig); err != nil {
			authConfig = map[string]string{}
		}
	}

	// Parse default headers
	var defaultHeaders map[string]string
	if httpFull.DefaultHeaders != "" {
		if err := json.Unmarshal([]byte(httpFull.DefaultHeaders), &defaultHeaders); err != nil {
			defaultHeaders = map[string]string{}
		}
	}

	// Converte endpoints
	endpoints := make([]agents.HTTPEndpointConfig, 0, len(httpFull.Endpoints))
	for _, ep := range httpFull.Endpoints {
		// Parse parameters JSON Schema
		var params map[string]interface{}
		if ep.Parameters != "" {
			if err := json.Unmarshal([]byte(ep.Parameters), &params); err != nil {
				params = map[string]interface{}{}
			}
		}

		endpoints = append(endpoints, agents.HTTPEndpointConfig{
			ID:               ep.ID,
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       params,
			ResponseTemplate: ep.ResponseTemplate,
		})
	}

	// Cria o HTTP Agent
	config := agents.HTTPAgentConfig{
		ID:             httpFull.ID,
		Name:           httpFull.Name,
		DisplayName:    httpFull.DisplayName,
		Description:    httpFull.Description,
		Model:          httpFull.Model,
		SystemPrompt:   httpFull.SystemPrompt,
		Enabled:        httpFull.Enabled,
		BaseURL:        httpFull.BaseURL,
		AuthType:       httpFull.AuthType,
		AuthConfig:     authConfig,
		DefaultHeaders: defaultHeaders,
		TimeoutSeconds: httpFull.TimeoutSeconds,
		RetryCount:     httpFull.RetryCount,
		Endpoints:      endpoints,
	}

	agent := agents.NewHTTPAgent(config, a.llmClient)

	// Registra o agente
	a.registry.Register(agent)
	return nil
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

// TTSStreamEvent evento de streaming de TTS (interface unificada para todos os provedores)
type TTSStreamEvent struct {
	SessionID   string `json:"sessionId"`   // Identificador único da sessão
	ChunkBase64 string `json:"chunkBase64"` // Chunk de áudio em base64 (apenas em tts:stream:chunk)
	Format      string `json:"format"`      // Formato do áudio (mp3, opus, etc)
	Done        bool   `json:"done"`        // True quando streaming terminou
	Error       string `json:"error"`       // Mensagem de erro (apenas em tts:stream:error)
}

// SynthesizeOpenAIStream sintetiza texto usando OpenAI TTS com streaming
// Emite eventos Wails conforme recebe chunks de áudio:
// - "tts:stream:start"  -> { sessionId, format }
// - "tts:stream:chunk"  -> { sessionId, chunkBase64, format }
// - "tts:stream:done"   -> { sessionId, done: true }
// - "tts:stream:error"  -> { sessionId, error }
// IMPORTANTE: Este método retorna imediatamente e executa o streaming em background
func (a *App) SynthesizeOpenAIStream(text string, voice string, sessionID string) error {
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     "speech manager not initialized",
			})
			return fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     "API key not configured",
			})
			return fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voice, "tts-1")
	}

	// Verifica se o provedor suporta streaming
	if !a.speechManager.SupportsStreaming() {
		// Fallback em goroutine separada
		go func() {
			result, err := a.speechManager.SynthesizeWithVoice(text, voice)
			if err != nil {
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
				return
			}

			// Emite como streaming de um único chunk
			runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
				SessionID: sessionID,
				Format:    result.Format,
			})
			runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
				SessionID:   sessionID,
				ChunkBase64: result.AudioBase64,
				Format:      result.Format,
			})
			runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
				SessionID: sessionID,
				Done:      true,
			})
		}()
		return nil
	}

	// Executa streaming em goroutine separada para não bloquear
	go func() {
		// Emite evento de início
		runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
			SessionID: sessionID,
			Format:    "mp3",
		})

		// Inicia streaming com callbacks
		callbacks := speech.StreamCallbacks{
			OnChunk: func(chunkBase64 string) {
				runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
					SessionID:   sessionID,
					ChunkBase64: chunkBase64,
					Format:      "mp3",
				})
			},
			OnDone: func() {
				runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
					SessionID: sessionID,
					Done:      true,
				})
			},
			OnError: func(err error) {
				log.Printf("[TTS] Stream error: %v", err)
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
			},
		}

		// Usa contexto com timeout
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := a.speechManager.SynthesizeStream(ctx, text, voice, callbacks)
		if err != nil {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     err.Error(),
			})
		}
	}()

	return nil
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

// ==================== Interaction Profiles ====================

// GetInteractionProfiles retorna todos os perfis de interação
func (a *App) GetInteractionProfiles() ([]database.InteractionProfile, error) {
	return database.GetAllInteractionProfiles()
}

// GetInteractionProfile retorna um perfil de interação por ID
func (a *App) GetInteractionProfile(id uint) (*database.InteractionProfile, error) {
	return database.GetInteractionProfile(id)
}

// GetDefaultInteractionProfile retorna o perfil de interação padrão
func (a *App) GetDefaultInteractionProfile() (*database.InteractionProfile, error) {
	return database.GetDefaultInteractionProfile()
}

// CreateInteractionProfile cria um novo perfil de interação
func (a *App) CreateInteractionProfile(profile database.InteractionProfile) (*database.InteractionProfile, error) {
	created, err := database.CreateInteractionProfile(&profile)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:profile:created", map[string]interface{}{
		"id":   created.ID,
		"name": created.Name,
	})

	return created, nil
}

// UpdateInteractionProfile atualiza um perfil de interação
func (a *App) UpdateInteractionProfile(id uint, profile database.InteractionProfile) (*database.InteractionProfile, error) {
	updated, err := database.UpdateInteractionProfile(id, &profile)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:profile:updated", map[string]interface{}{
		"id":   updated.ID,
		"name": updated.Name,
	})

	return updated, nil
}

// DeleteInteractionProfile deleta um perfil de interação
func (a *App) DeleteInteractionProfile(id uint) error {
	err := database.DeleteInteractionProfile(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:profile:deleted", map[string]interface{}{
		"id": id,
	})

	return nil
}

// SetDefaultInteractionProfile define um perfil como padrão
func (a *App) SetDefaultInteractionProfile(id uint) error {
	err := database.SetDefaultInteractionProfile(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:profile:default_changed", map[string]interface{}{
		"id": id,
	})

	return nil
}

// SearchInteractionProfiles busca perfis por nome ou descrição
func (a *App) SearchInteractionProfiles(query string) ([]database.InteractionProfile, error) {
	return database.SearchInteractionProfiles(query)
}

// ==================== Interaction Triggers ====================

// GetTriggersByProfile retorna todos os triggers de um perfil
func (a *App) GetTriggersByProfile(profileID uint) ([]database.InteractionTrigger, error) {
	return database.GetTriggersByProfile(profileID)
}

// GetInteractionTrigger retorna um trigger por ID
func (a *App) GetInteractionTrigger(id uint) (*database.InteractionTrigger, error) {
	return database.GetInteractionTrigger(id)
}

// CreateInteractionTrigger cria um novo trigger
func (a *App) CreateInteractionTrigger(trigger database.InteractionTrigger) (*database.InteractionTrigger, error) {
	created, err := database.CreateInteractionTrigger(&trigger)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:trigger:created", map[string]interface{}{
		"id":        created.ID,
		"profileId": created.ProfileID,
		"type":      created.Type,
	})

	return created, nil
}

// UpdateInteractionTrigger atualiza um trigger
func (a *App) UpdateInteractionTrigger(id uint, trigger database.InteractionTrigger) (*database.InteractionTrigger, error) {
	updated, err := database.UpdateInteractionTrigger(id, &trigger)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:trigger:updated", map[string]interface{}{
		"id":        updated.ID,
		"profileId": updated.ProfileID,
		"type":      updated.Type,
	})

	return updated, nil
}

// DeleteInteractionTrigger deleta um trigger
func (a *App) DeleteInteractionTrigger(id uint) error {
	// Busca trigger para obter profileId antes de deletar
	trigger, err := database.GetInteractionTrigger(id)
	if err != nil {
		return err
	}

	err = database.DeleteInteractionTrigger(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:trigger:deleted", map[string]interface{}{
		"id":        id,
		"profileId": trigger.ProfileID,
	})

	return nil
}

// ==================== Chat Profiles ====================

// GetChatProfiles retorna todos os perfis de conversa
func (a *App) GetChatProfiles() ([]database.ChatProfile, error) {
	return database.GetAllChatProfiles()
}

// GetChatProfile retorna um perfil de conversa por ID
func (a *App) GetChatProfile(id uint) (*database.ChatProfile, error) {
	return database.GetChatProfile(id)
}

// GetDefaultChatProfile retorna o perfil de conversa padrão
func (a *App) GetDefaultChatProfile() (*database.ChatProfile, error) {
	return database.GetDefaultChatProfile()
}

// CreateChatProfile cria um novo perfil de conversa
func (a *App) CreateChatProfile(profile database.ChatProfile) (*database.ChatProfile, error) {
	created, err := database.CreateChatProfile(&profile)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:created", created)

	return created, nil
}

// UpdateChatProfile atualiza um perfil de conversa
func (a *App) UpdateChatProfile(id uint, profile database.ChatProfile) (*database.ChatProfile, error) {
	updated, err := database.UpdateChatProfile(id, &profile)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:updated", updated)

	return updated, nil
}

// DeleteChatProfile deleta um perfil de conversa
func (a *App) DeleteChatProfile(id uint) error {
	err := database.DeleteChatProfile(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:deleted", id)

	return nil
}

// SetDefaultChatProfile define um perfil como padrão
func (a *App) SetDefaultChatProfile(id uint) error {
	err := database.SetDefaultChatProfile(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:default_changed", id)

	return nil
}

// SetConversationChatProfile define o perfil de conversa para uma conversa
func (a *App) SetConversationChatProfile(conversationID uint, profileID uint) error {
	err := database.SetConversationChatProfile(conversationID, profileID)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:conversation_changed", map[string]interface{}{
		"conversation_id": conversationID,
		"profile_id":      profileID,
	})

	return nil
}

// ClearConversationChatProfile remove o perfil customizado de uma conversa
func (a *App) ClearConversationChatProfile(conversationID uint) error {
	err := database.ClearConversationChatProfile(conversationID)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:conversation_changed", map[string]interface{}{
		"conversation_id": conversationID,
		"profile_id":      0, // 0 indica usar padrão
	})

	return nil
}

// GetEffectiveChatProfile retorna o perfil efetivo de uma conversa
func (a *App) GetEffectiveChatProfile(conversationID uint) (*database.ChatProfile, error) {
	return database.GetEffectiveChatProfile(conversationID)
}

// ==================== Chat Tabs ====================

// GetAllTabs retorna todas as abas de chat
func (a *App) GetAllTabs() ([]database.ChatTab, error) {
	return database.GetAllTabs()
}

// GetActiveTab retorna a aba ativa
func (a *App) GetActiveTab() (*database.ChatTab, error) {
	return database.GetActiveTab()
}

// CreateTab cria uma nova aba de chat
// setAsActive: se true, marca a nova aba como ativa; se false, mantém a aba atual ativa
func (a *App) CreateTab(title, icon string, setAsActive bool) (*database.ChatTab, error) {
	tab, err := database.CreateTab(title, icon, setAsActive)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "tab_created", map[string]interface{}{
		"id":       tab.ID,
		"title":    tab.Title,
		"icon":     tab.Icon,
		"position": tab.Position,
		"isActive": tab.IsActive,
	})

	return tab, nil
}

// CloseTab fecha uma aba
func (a *App) CloseTab(id uint) error {
	err := database.CloseTab(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "tab_closed", map[string]interface{}{
		"id": id,
	})

	return nil
}

// SetActiveTab define a aba ativa
func (a *App) SetActiveTab(id uint) error {
	err := database.SetActiveTab(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "tab_activated", map[string]interface{}{
		"id": id,
	})

	return nil
}

// UpdateTabTitle atualiza o título de uma aba e da conversa associada
func (a *App) UpdateTabTitle(id uint, title string) error {
	// Busca a tab para verificar se tem conversa associada
	tab, err := database.GetTab(id)
	if err != nil {
		return err
	}

	err = database.UpdateTabTitle(id, title)
	if err != nil {
		return err
	}

	// Se há conversa associada, emite evento unificado para atualizar todas as referências
	if tab.ConversationID != nil && *tab.ConversationID > 0 {
		runtime.EventsEmit(a.ctx, "conversation:renamed", map[string]interface{}{
			"conversation_id": *tab.ConversationID,
			"new_title":       title,
		})
	}

	return nil
}

// LoadConversationInTab carrega uma conversa em uma aba
func (a *App) LoadConversationInTab(tabId, conversationId uint) error {
	err := database.LoadConversationInTab(tabId, conversationId)
	if err != nil {
		return err
	}

	// Obtém a conversa para emitir evento completo
	conv, err := database.GetConversation(conversationId)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "conversation_loaded_in_tab", map[string]interface{}{
		"tabId":          tabId,
		"conversationId": conv.ID,
		"title":          conv.Title,
	})

	return nil
}

// ClearTab limpa a conversa de uma aba
func (a *App) ClearTab(id uint) error {
	err := database.ClearTab(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "tab_cleared", map[string]interface{}{
		"id": id,
	})

	return nil
}

// ReorderTabs reordena as abas
func (a *App) ReorderTabs(orderedIds []uint) error {
	return database.ReorderTabs(orderedIds)
}
