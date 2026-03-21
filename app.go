package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/channels"
	"assistente/internal/config"
	"assistente/internal/configdir"
	"assistente/internal/contacts"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/messaging/signal"
	"assistente/internal/messaging/slack"
	"assistente/internal/messaging/telegram"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
	"assistente/internal/skills"
	"assistente/internal/speech"
	"assistente/internal/terminal"
	"assistente/internal/tools"
	"assistente/internal/tools/editor"
	"assistente/internal/tools/filesystem"
	"assistente/internal/tools/history"
	msgtool "assistente/internal/tools/messaging"
	questiontool "assistente/internal/tools/questionnaire"
	"assistente/internal/tools/shell"
	tasklisttool "assistente/internal/tools/tasklist"
	"assistente/internal/tools/web"
	"assistente/internal/updater"
	"assistente/internal/workspace"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	// AppVersion é a versão do aplicativo
	// Será injetada automaticamente pelo Wails a partir de wails.json info.productVersion
	// Em dev, permanece como "dev"
	AppVersion = "dev"
)

// Request structs for LLM Provider Management
type CreateLLMProviderRequest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key,omitempty"`
}

type TestLLMProviderRequest struct {
	Type       string `json:"type"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
}

type UpdateLLMProviderRequest struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
}

// App struct
type App struct {
	ctx                   context.Context
	llmClient             *llm.SyncClient
	llmStreamClient       *llm.Client
	llmRegistry           *llm.ProviderRegistry // Registro de provedores LLM
	speechManager         *speech.SpeechManager
	hotkeyManager         *hotkey.Manager
	profileManager        *profiles.Manager
	toolRegistry          *tools.Registry             // Registro de ferramentas disponíveis
	toolExecutor          *tools.Executor             // Executor de ferramentas com paralelismo e timeout
	terminalMgr           *terminal.Manager           // Gerenciador de sessões PTY (pool compartilhado LLM + usuário)
	questionnaireMgr      *questionnaire.Manager      // Gerenciador de questionários (coleta estruturada)
	allowlistMgr          *allowlist.Manager          // Gerenciador de allowlists de comandos
	mcpMgr                *mcpmgr.Manager             // Gerenciador de servidores MCP
	skillMgr              *skills.Manager             // Gerenciador de skills
	responseNotifier      *messaging.ResponseNotifier // Notificador de respostas para mensageiros
	msgGateway            *messaging.Gateway          // Gateway de mensageria (Telegram, etc.)
	updater               *updater.Updater            // Gerenciador de atualizações automáticas
	voiceHotkeyID         int
	currentConversationID uint // ID da conversa atual

	credMgr   *credentials.Manager
	credStore credentials.Store

	// Throttle para hotkeys - evita disparo repetido quando segura a tecla
	hotkeyLastFired  map[uint]time.Time
	hotkeyThrottleMs int64 // tempo mínimo entre disparos (em ms)

	// Watcher de arquivos do editor (mudanças externas)
	editorWatchMu    sync.Mutex
	editorDirWatches map[string]*editorDirWatch

	// Workspace manager (unified tabs)
	workspaceMgr *workspace.Manager
}

// ==================== Tipos para Threads ====================

// EnrichedMessage é ChatMessage + campos derivados calculados no backend
type EnrichedMessage struct {
	ID               string    `json:"id"`
	ConversationID   uint      `json:"conversationId"`
	ParentID         *string   `json:"parentId,omitempty"`
	TurnID           *uint     `json:"turnId,omitempty"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"`
	Media            string    `json:"media,omitempty"`
	ToolCalls        string    `json:"toolCalls,omitempty"`
	ToolCallID       string    `json:"toolCallId,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	Model            string    `json:"model,omitempty"`
	Source           string    `json:"source,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	Timestamp        int64     `json:"timestamp"`
	IsStreaming      bool      `json:"isStreaming"`
	Internal         bool      `json:"internal"`
}

// MessageNode representa uma mensagem com seus filhos na hierarquia
type MessageNode struct {
	Message    EnrichedMessage `json:"message"`
	Children   []MessageNode   `json:"children,omitempty"`
	Level      int             `json:"level"`
	ChildCount int             `json:"childCount"`
}

// ConversationWithThreads representa uma conversa com mensagens organizadas em árvore
type ConversationWithThreads struct {
	ID      uint          `json:"id"`
	Title   string        `json:"title"`
	Threads []MessageNode `json:"threads"`
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
		hotkeyLastFired:  make(map[uint]time.Time),
		hotkeyThrottleMs: 1000,
		profileManager:   profiles.NewManager(),
		llmRegistry:      llm.NewProviderRegistry(),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Inicializa o banco de dados
	if err := InitDatabase(); err != nil {
		log.Printf("Erro ao inicializar banco de dados: %v", err)
	}
	if err := a.cleanupEditorOrphanDraftsOnStartup(); err != nil {
		log.Printf("Erro ao limpar drafts órfãos do editor no startup: %v", err)
	}

	// Instala/atualiza perfis embutidos em ~/.assistente/profiles/
	a.installBuiltinProfiles()

	// Garante que o diretório de perfis existe
	if err := a.profileManager.EnsureDefaults(); err != nil {
		log.Printf("Erro ao garantir diretório de perfis: %v", err)
	}

	// Inicializa Credential Manager PRIMEIRO (antes de qualquer uso)
	a.initCredentialManager()

	// Inicializa os provedores LLM (Provider Registry) ANTES do client
	a.initLLMProviders()

	// Inicializa o cliente LLM (usa credMgr + registry já populado)
	a.initLLMClient()

	// Migra config.json legado para novo sistema (se necessário)
	a.migrateLegacyConfig()

	// Inicializa managers de terminal, confirmação e allowlists
	a.initTerminalAndAllowlists()

	// Inicializa o registro de ferramentas (tool calling)
	a.initToolRegistry()

	// Inicializa o gerenciador de skills
	a.initSkills()

	// Garante que o diretório de memória existe no home
	a.initMemoryDir()

	// Inicializa o gerenciador de servidores MCP (após tool registry)
	a.initMCP()

	// Inicializa o gateway de mensageria (Telegram, etc.)
	a.initMessaging()

	// Inicializa hotkeys globais
	a.initGlobalHotkeys()

	// Registra hotkeys do perfil ativo
	a.registerActiveProfileHotkeys()

	// Inicializa o workspace manager
	a.initWorkspace()

	// Inicializa o updater
	a.initUpdater()

	// Verifica atualizações no startup (não bloqueante)
	go a.checkForUpdatesOnStartup()

	// Restaura foco da janela no startup (resolve bug do Wails no Windows)
	// Deixa 400ms para garantir que a janela está completamente pronta
	go func() {
		time.Sleep(400 * time.Millisecond)
		runtime.WindowShow(a.ctx)
		log.Printf("[App] WindowShow chamado após startup")
	}()
}

// initLLMClient inicializa o cliente LLM usando o provider do perfil ativo
func (a *App) initLLMClient() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Erro ao carregar config para LLM: %v", err)
		return
	}

	// Load active profile to get provider
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		log.Printf("Erro ao carregar perfil ativo: %v", err)
		return
	}

	// Get provider from registry
	provider := a.llmRegistry.Get(activeProfile.Chat.LLMProvider)
	if provider == nil {
		log.Printf("Provedor LLM não encontrado: %s", activeProfile.Chat.LLMProvider)
		return
	}

	// Create clients with provider config
	a.llmClient = llm.NewSyncClient(provider, a.credMgr)
	a.llmStreamClient = llm.NewClient(provider, cfg, a.credMgr)
	log.Printf("LLM Client inicializado com provedor: %s", provider.Name)
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
}

// getClientForProvider creates a new LLM client for a specific provider.
// Used to ensure requests are routed to the correct provider endpoint
// when the active profile differs from the global default.
func (a *App) getClientForProvider(providerID string) (*llm.Client, error) {
	if a.llmRegistry == nil {
		return nil, fmt.Errorf("registro de provedores não inicializado")
	}

	provider := a.llmRegistry.Get(providerID)
	if provider == nil {
		return nil, fmt.Errorf("provedor LLM não encontrado: %s", providerID)
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar config: %w", err)
	}

	return llm.NewClient(provider, cfg, a.credMgr), nil
}

// initLLMProviders inicializa o registro de provedores LLM com os provedores padrão
func (a *App) initLLMProviders() {
	// Tentar carregar provedores do SQLite
	if err := a.loadLLMProviders(); err == nil {
		return
	}

	// Se não houver provedores, verificar se já passou pelo wizard
	// Se não passou, o wizard criará o primeiro provedor
	count, err := database.CountLLMProviders()
	if err != nil || count == 0 {
		log.Printf("Nenhum provedor encontrado. Configure um provedor nas configurações ou crie um perfil.")
	}
}

// CreateDefaultLLMProvider cria o primeiro provedor durante o wizard
func (a *App) CreateDefaultLLMProvider(providerType, apiKey string) error {
	var provider *llm.ProviderConfig

	switch providerType {
	case "openai":
		provider = &llm.ProviderConfig{
			ID:                "openai-default",
			Name:              "OpenAI",
			Type:              llm.ProviderOpenAI,
			BaseURL:           "https://api.openai.com/v1",
			Model:             "gpt-4o-mini",
			Timeout:           180,
			CredentialPattern: "api.openai.com",
		}
	case "claude":
		provider = &llm.ProviderConfig{
			ID:                "anthropic-claude",
			Name:              "Claude (Anthropic)",
			Type:              llm.ProviderClaude,
			BaseURL:           "https://api.anthropic.com/v1",
			Model:             "claude-3-7-sonnet-20250219",
			Timeout:           180,
			CredentialPattern: "api.anthropic.com",
		}
	case "google":
		provider = &llm.ProviderConfig{
			ID:                "google-gemini",
			Name:              "Google (Gemini)",
			Type:              llm.ProviderOpenAI,
			BaseURL:           "https://generativelanguage.googleapis.com/v1beta/openai/",
			Model:             "gemini-2.0-flash",
			Timeout:           180,
			CredentialPattern: "generativelanguage.googleapis.com",
		}
	case "openrouter":
		provider = &llm.ProviderConfig{
			ID:                "openrouter-default",
			Name:              "OpenRouter",
			Type:              llm.ProviderOpenAI,
			BaseURL:           "https://openrouter.ai/api/v1",
			Model:             "openai/gpt-4o-mini",
			Timeout:           180,
			CredentialPattern: "openrouter.ai",
		}
	case "mistral":
		provider = &llm.ProviderConfig{
			ID:                "mistral-default",
			Name:              "Mistral AI",
			Type:              llm.ProviderMistral,
			BaseURL:           "https://api.mistral.ai/v1",
			Model:             "mistral-large-latest",
			Timeout:           180,
			CredentialPattern: "api.mistral.ai",
		}
	case "groq":
		provider = &llm.ProviderConfig{
			ID:                "groq-default",
			Name:              "Groq",
			Type:              llm.ProviderGroq,
			BaseURL:           "https://api.groq.com/openai/v1",
			Model:             "llama-3.3-70b-versatile",
			Timeout:           180,
			CredentialPattern: "api.groq.com",
		}
	case "together":
		provider = &llm.ProviderConfig{
			ID:                "together-default",
			Name:              "Together AI",
			Type:              llm.ProviderTogether,
			BaseURL:           "https://api.together.xyz/v1",
			Model:             "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Timeout:           180,
			CredentialPattern: "api.together.xyz",
		}
	case "fireworks":
		provider = &llm.ProviderConfig{
			ID:                "fireworks-default",
			Name:              "Fireworks AI",
			Type:              llm.ProviderFireworks,
			BaseURL:           "https://api.fireworks.ai/inference/v1",
			Model:             "accounts/fireworks/models/llama-v3p3-70b-instruct",
			Timeout:           180,
			CredentialPattern: "api.fireworks.ai",
		}
	case "perplexity":
		provider = &llm.ProviderConfig{
			ID:                "perplexity-default",
			Name:              "Perplexity",
			Type:              llm.ProviderPerplexity,
			BaseURL:           "https://api.perplexity.ai",
			Model:             "sonar",
			Timeout:           180,
			CredentialPattern: "api.perplexity.ai",
		}
	case "deepseek":
		provider = &llm.ProviderConfig{
			ID:                "deepseek-default",
			Name:              "DeepSeek",
			Type:              llm.ProviderDeepSeek,
			BaseURL:           "https://api.deepseek.com/v1",
			Model:             "deepseek-chat",
			Timeout:           180,
			CredentialPattern: "api.deepseek.com",
		}
	case "grok":
		provider = &llm.ProviderConfig{
			ID:                "xai-grok",
			Name:              "xAI (Grok)",
			Type:              llm.ProviderGrok,
			BaseURL:           "https://api.x.ai/v1",
			Model:             "grok-2",
			Timeout:           180,
			CredentialPattern: "api.x.ai",
		}
	case "ollama":
		provider = &llm.ProviderConfig{
			ID:                "ollama-local",
			Name:              "Ollama (Local)",
			Type:              llm.ProviderOllama,
			BaseURL:           "http://localhost:11434/api",
			Model:             "llama2",
			Timeout:           300,
			CredentialPattern: "",
		}
	default:
		return fmt.Errorf("tipo de provedor inválido: %s", providerType)
	}

	// Registrar no registry
	if err := a.llmRegistry.Register(provider); err != nil {
		return fmt.Errorf("erro ao registrar provedor: %w", err)
	}

	// Salvar API key se fornecida
	if apiKey != "" && provider.CredentialPattern != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: apiKey,
		}
		if err := a.credMgr.RegisterPatternWithContext(a.ctx, provider.CredentialPattern, authCfg); err != nil {
			return fmt.Errorf("erro ao salvar credencial: %w", err)
		}
	}

	// Salvar no SQLite
	if err := a.saveLLMProviders(); err != nil {
		return fmt.Errorf("erro ao salvar provedor: %w", err)
	}

	log.Printf("[Wizard] Provedor '%s' criado com sucesso", provider.ID)
	return nil
}

// initCredentialManager inicializa o gerenciador de credenciais com persistência
func (a *App) initCredentialManager() {
	a.credStore = credentials.NewDBStore()
	persist := true
	dek, err := credentials.LoadDEKFromKeychain()
	if err != nil {
		if !credentials.IsKeychainNotFound(err) {
			log.Printf("[Credentials] Erro ao acessar keychain: %v", err)
		}
		persist = false
		dek = nil
	}

	a.credMgr = credentials.NewManagerWithStore(dek, a.credStore, persist)
	if err := a.credMgr.LoadFromStore(context.Background()); err != nil {
		log.Printf("[Credentials] Erro ao carregar credenciais persistidas: %v", err)
	}
	a.registerEnvCredentials(a.credMgr)
}

// migrateLegacyConfig detecta config.json com campos legados e migra para novo sistema
// Migração:
// 1. Se APIKey existir → registra como credencial no credentials.Manager
// 2. Se APIKey existir → garante que provider default está usando as credenciais
// 3. Limpa campos legados do config.json
func (a *App) migrateLegacyConfig() {
	cfg, err := config.Load()
	if err != nil {
		// Sem config, sem migração necessária
		return
	}

	needsMigration := false
	migratedFields := []string{}

	// Verificar se tem APIKey (campo principal legado)
	if cfg.APIKey != "" {
		needsMigration = true
		migratedFields = append(migratedFields, "APIKey")

		// Extrair domínio do BaseURL
		baseURL := cfg.APIBaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}

		// Determinar pattern baseado no baseURL
		// Usa extractHostname para padrão exato, consistente com CreateLLMProvider
		pattern := ""
		if extractedHost, hostErr := extractHostname(baseURL); hostErr == nil && extractedHost != "" {
			pattern = extractedHost
		} else if strings.Contains(baseURL, "anthropic") {
			pattern = "api.anthropic.com"
		} else if strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") {
			pattern = "" // local, sem pattern
		} else {
			pattern = "api.openai.com" // fallback para OpenAI
		}

		// Registrar credencial no credentials.Manager
		if pattern != "" {
			authCfg := &credentials.AuthConfig{
				Type:  "bearer",
				Token: cfg.APIKey,
			}
			if err := a.credMgr.RegisterPatternWithContext(a.ctx, pattern, authCfg); err != nil {
				log.Printf("[Migration] Erro ao registrar credencial do config.json: %v", err)
			} else {
				log.Printf("[Migration] ✓ APIKey migrado para credentials.Manager (pattern: %s)", pattern)
			}
		}
	}

	// Verificar outros campos legados
	if cfg.APIBaseURL != "" && cfg.APIBaseURL != "https://api.openai.com/v1" {
		migratedFields = append(migratedFields, "APIBaseURL")
	}
	if cfg.DefaultModel != "" && cfg.DefaultModel != "gpt-4o-mini" {
		migratedFields = append(migratedFields, "DefaultModel")
	}
	if cfg.ResponseTimeout != 0 && cfg.ResponseTimeout != 180 {
		migratedFields = append(migratedFields, "ResponseTimeout")
	}
	if cfg.ActiveProfile != "" && cfg.ActiveProfile != "padrao" {
		migratedFields = append(migratedFields, "ActiveProfile")
	}

	if needsMigration {
		log.Printf("[Migration] Config.json legado detectado — campos migrados: %v", migratedFields)
		log.Printf("[Migration] Novas configurações devem ser feitas via Perfis e Provider Registry")
		log.Printf("[Migration] Os campos legados em config.json não serão mais usados")

		// Não vamos deletar o config.json ainda, apenas marcar como migrado
		// Isso permite que usuários vejam o arquivo e entendam a mudança
	}
}

// initTerminalAndAllowlists inicializa os managers de terminal, questionário e allowlists.
func (a *App) initTerminalAndAllowlists() {
	// Callback para emitir eventos Wails a partir dos managers
	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)
	}

	// Terminal Manager (pool compartilhado LLM + usuário)
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), emitEvent)

	// Questionnaire Manager (coleta de respostas estruturadas)
	a.questionnaireMgr = questionnaire.NewManager(emitEvent)

	// Allowlist Manager (CRUD de allowlists)
	a.allowlistMgr = allowlist.NewManager()
	if err := a.allowlistMgr.EnsureDefaults(); err != nil {
		log.Printf("[Allowlist] Erro ao garantir allowlist padrão: %v", err)
	}

	log.Printf("[Terminal] Managers de terminal, questionário e allowlist inicializados")
}

// initMCP inicializa o gerenciador de servidores MCP.
// Deve ser chamado após initToolRegistry (precisa do registry para registrar tools MCP).
func (a *App) initMCP() {
	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)
	}

	a.mcpMgr = mcpmgr.NewManager(a.toolRegistry, a.credMgr, emitEvent)

	// Carrega configs e auto-conecta servidores habilitados
	if err := a.mcpMgr.LoadConfigs(); err != nil {
		log.Printf("[MCP] Erro ao carregar configurações: %v", err)
	}

	// Observa mudanças externas nos arquivos de config
	go a.mcpMgr.WatchConfigs()

	log.Printf("[MCP] Manager inicializado")
}

// initMessaging inicializa o gateway de mensageria usando configs de .assistente/channels/.
func (a *App) initMessaging() {
	// ResponseNotifier — permite ao gateway capturar respostas para reenvio
	a.responseNotifier = messaging.NewResponseNotifier()

	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)
	}

	// Função TTS para sintetizar respostas em áudio para canais externos.
	// Resolve o perfil do canal e usa ChannelResponseMode para decidir se gera TTS:
	//   - "mirror" (padrão): áudio→áudio, texto→texto
	//   - "always_text":     nunca gera TTS
	//   - "always_audio":    sempre gera TTS
	// Retorna (nil, nil) se não deve gerar áudio (gateway enviará texto).
	synthesizeTTS := messaging.SynthesizeTTSFunc(func(text string, channel string, incomingIsAudio bool) ([]byte, error) {
		// Resolve o perfil do canal
		var profile *profiles.Profile
		if chCfg, _ := channels.Load(channel); chCfg != nil && chCfg.Profile != "" {
			if p, err := a.profileManager.Get(chCfg.Profile); err == nil {
				profile = p
			}
		}
		if profile == nil {
			if p, err := a.profileManager.GetActive(); err == nil {
				profile = p
			}
		}

		// Consulta ChannelResponseMode do perfil para decidir se gera áudio
		if profile != nil {
			if !profile.ShouldRespondWithAudio(incomingIsAudio) {
				log.Printf("[TTS-Channel] Modo '%s': não gerar áudio para canal %s (incoming_audio=%v)",
					profile.GetChannelResponseMode(), channel, incomingIsAudio)
				return nil, nil
			}
		} else {
			// Sem perfil: fallback para mirror
			if !incomingIsAudio {
				return nil, nil
			}
		}

		// Verifica se o provider de voz suporta canais externos
		if profile != nil {
			if profile.Voice.Provider == "disabled" || profile.Voice.Provider == "" {
				log.Printf("[TTS-Channel] Voz desabilitada no perfil para canal %s — respondendo com texto", channel)
				return nil, nil
			}
			// WebSpeech e SAPI5 são providers locais do desktop — não funcionam para canais externos
			if profile.Voice.Provider == "webspeech" || profile.Voice.Provider == "sapi5" {
				log.Printf("[TTS-Channel] Provider '%s' é local e não suporta canais externos — respondendo com texto", profile.Voice.Provider)
				return nil, nil
			}
		}

		if !a.ensureSpeechManager() {
			return nil, fmt.Errorf("speech manager indisponível para TTS")
		}

		// Usa a voz do perfil se especificada, senão usa Synthesize padrão
		var result *speech.SynthesisResult
		var err error
		if profile != nil && profile.Voice.VoiceID != "" {
			result, err = a.speechManager.SynthesizeWithVoice(text, profile.Voice.VoiceID)
		} else {
			result, err = a.speechManager.Synthesize(text)
		}
		if err != nil {
			return nil, err
		}
		audioBytes, err := base64.StdEncoding.DecodeString(result.AudioBase64)
		if err != nil {
			return nil, fmt.Errorf("erro ao decodificar áudio TTS: %w", err)
		}
		return audioBytes, nil
	})

	// Cria o gateway
	approveContactFn := func(ctx context.Context, channel, displayName, contactID, username string) (bool, error) {
		if a.questionnaireMgr == nil {
			return false, fmt.Errorf("questionnaire manager não inicializado")
		}
		name := displayName
		if name == "" {
			name = "desconhecido"
		}
		identifier := contactID
		if identifier == "" {
			identifier = username
		}
		message := fmt.Sprintf("O contato %s enviou uma mensagem via %s.\n\nIdentificador: %s\n\nPara autorizar este contato, digite o código de pareamento que foi enviado para a pessoa.",
			name, channel, identifier)
		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       "Novo contato - Pareamento requerido",
			Description: message,
			AllowCancel: true,
			SubmitLabel: "Autorizar",
			CancelLabel: "Recusar",
			Questions: []questionnaire.Question{
				{
					ID:       "pairing_code",
					Type:     "text",
					Prompt:   "Código de pareamento (6 dígitos):",
					Required: true,
				},
			},
		})
		if err != nil {
			return false, err
		}
		if resp.Cancelled {
			return false, nil
		}

		// Valida o código fornecido
		providedCode, ok := resp.Answers["pairing_code"].(string)
		if !ok {
			return false, fmt.Errorf("código de pareamento inválido")
		}

		// Valida usando a função que retorna (bool, error)
		valid, validateErr := contacts.ValidatePairingCode(channel, contactID, providedCode)
		if !valid {
			if validateErr != nil {
				return false, fmt.Errorf("pareamento falhou: %v", validateErr)
			}
			return false, fmt.Errorf("código de pareamento incorreto")
		}

		return true, nil
	}

	a.msgGateway = messaging.NewGateway(
		a.responseNotifier,
		a.SendMessageFromChannel,
		emitEvent,
		approveContactFn,
		synthesizeTTS,
		database.SaveMessageAudio,
	)

	// Carrega canais habilitados de .assistente/channels/
	enabledChannels, err := channels.LoadEnabled()
	if err != nil {
		log.Printf("[Messaging] Erro ao carregar canais: %v", err)
	}

	// Telegram
	if cfg, ok := enabledChannels["telegram"]; ok {
		botToken := cfg.BotToken
		if botToken == "" && cfg.BotTokenRef != "" {
			botToken = a.resolveCredentialRef(cfg.BotTokenRef)
		}
		if botToken == "" {
			log.Printf("[Messaging] Telegram não configurado ou desabilitado")
		} else {
			adapter := telegram.NewAdapter(botToken)
			a.msgGateway.Register("telegram", adapter)
			go func() {
				if err := adapter.Connect(a.ctx); err != nil {
					log.Printf("[Messaging] Erro ao conectar Telegram: %v", err)
				}
			}()
			log.Printf("[Messaging] Telegram habilitado")
		}
	}

	// Signal (via signal-cli-rest-api HTTP + WebSocket)
	if cfg, ok := enabledChannels["signal"]; ok && cfg.Account != "" && cfg.APIURL != "" {
		adapter := signal.NewAdapter(cfg.APIURL, cfg.Account, a.credMgr)
		a.msgGateway.Register("signal", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar Signal: %v", err)
			}
		}()
		log.Printf("[Messaging] Signal habilitado (api=%s, account=%s)", cfg.APIURL, maskIdentifier(cfg.Account))
	} else {
		log.Printf("[Messaging] Signal não configurado ou desabilitado")
	}

	// Slack (Socket Mode)
	if cfg, ok := enabledChannels["slack"]; ok {
		botToken := cfg.BotToken
		appToken := cfg.AppToken
		if botToken == "" && cfg.BotTokenRef != "" {
			botToken = a.resolveCredentialRef(cfg.BotTokenRef)
		}
		if appToken == "" && cfg.AppTokenRef != "" {
			appToken = a.resolveCredentialRef(cfg.AppTokenRef)
		}
		if botToken == "" || appToken == "" {
			log.Printf("[Messaging] Slack não configurado ou desabilitado")
		} else {
			adapter := slack.NewAdapter(botToken, appToken)
			a.msgGateway.Register("slack", adapter)
			go func() {
				if err := adapter.Connect(a.ctx); err != nil {
					log.Printf("[Messaging] Erro ao conectar Slack: %v", err)
				}
			}()
			log.Printf("[Messaging] Slack habilitado")
		}
	}

	// Registra a tool send_message no registry de ferramentas
	if a.toolRegistry != nil {
		sendMsgTool := msgtool.NewSendMessageTool(a.msgGateway)
		a.toolRegistry.MustRegister(sendMsgTool)
		log.Printf("[Messaging] Tool 'send_message' registrada")

		// Registra a tool validate_pairing_code
		pairingTool := msgtool.NewValidatePairingCodeTool()
		a.toolRegistry.MustRegister(pairingTool)
		log.Printf("[Messaging] Tool 'validate_pairing_code' registrada")
	}

	log.Printf("[Messaging] Gateway inicializado")
}

// GetMessagingStatus retorna o status de todos os mensageiros conectados.
func (a *App) GetMessagingStatus() map[string]string {
	if a.msgGateway == nil {
		return map[string]string{}
	}
	status := a.msgGateway.GetStatus()
	result := make(map[string]string, len(status))
	for k, v := range status {
		result[k] = string(v)
	}
	return result
}

// GetChannelConfig retorna a configuração de um canal de mensageria.
func (a *App) GetChannelConfig(channelName string) (*channels.ChannelConfig, error) {
	cfg, err := channels.Load(channelName)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &channels.ChannelConfig{}, nil
	}
	return cfg, nil
}

// ListCredentials retorna credenciais registradas (sem valores sensíveis).
func (a *App) ListCredentials() ([]CredentialSummary, error) {
	if a.credMgr == nil {
		return []CredentialSummary{}, nil
	}

	list, err := a.credMgr.ListCredentials()
	if err != nil {
		return nil, err
	}

	result := make([]CredentialSummary, 0, len(list))
	for _, entry := range list {
		if entry.Auth == nil {
			continue
		}
		result = append(result, CredentialSummary{
			Pattern: entry.Pattern,
			Type:    entry.Auth.Type,
			Masked:  summarizeAuth(entry.Auth),
			Managed: credentials.IsManagedPattern(entry.Pattern),
		})
	}

	return result, nil
}

// UpsertCredential cria ou atualiza uma credencial no credential manager.
func (a *App) UpsertCredential(input CredentialInput) error {
	if a.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	if !a.credMgr.CanPersist() {
		return fmt.Errorf("cofre de credenciais indisponível: configure a senha mestre")
	}

	pattern := strings.TrimSpace(input.Pattern)
	if pattern == "" {
		return fmt.Errorf("pattern é obrigatório")
	}

	if credentials.IsManagedPattern(pattern) {
		return fmt.Errorf("credencial '%s' é gerenciada pelo sistema e não pode ser editada manualmente", pattern)
	}

	auth := &credentials.AuthConfig{Type: strings.TrimSpace(input.Type)}
	switch auth.Type {
	case "bearer", "oauth2", "secret":
		if strings.TrimSpace(input.Token) == "" {
			return fmt.Errorf("token é obrigatório")
		}
		auth.Token = input.Token
	case "basic":
		if strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.Password) == "" {
			return fmt.Errorf("usuário e senha são obrigatórios")
		}
		auth.Username = input.Username
		auth.Password = input.Password
	case "custom":
		if strings.TrimSpace(input.HeaderName) == "" || strings.TrimSpace(input.HeaderValue) == "" {
			return fmt.Errorf("header e valor são obrigatórios")
		}
		auth.Headers = map[string]string{input.HeaderName: input.HeaderValue}
	default:
		return fmt.Errorf("tipo de credencial inválido")
	}

	return a.credMgr.RegisterPatternWithContext(context.Background(), pattern, auth)
}

// DeleteCredential remove uma credencial pelo padrão.
func (a *App) DeleteCredential(pattern string) error {
	if a.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	if credentials.IsManagedPattern(pattern) {
		return fmt.Errorf("credencial '%s' é gerenciada pelo sistema e não pode ser removida manualmente", pattern)
	}
	return a.credMgr.DeletePattern(context.Background(), pattern)
}

// SaveChannelConfig salva a configuração de um canal e reconecta automaticamente.
func (a *App) SaveChannelConfig(channelName string, cfg *channels.ChannelConfig) error {
	if err := a.persistChannelCredentials(channelName, cfg); err != nil {
		return err
	}
	if err := channels.Save(channelName, cfg); err != nil {
		return err
	}
	// Reconecta o canal com a nova configuração
	a.restartChannel(channelName, cfg)
	return nil
}

func (a *App) persistChannelCredentials(channelName string, cfg *channels.ChannelConfig) error {
	if cfg == nil || a.credMgr == nil || !a.credMgr.CanPersist() {
		return nil
	}

	ctx := context.Background()

	switch channelName {
	case "telegram":
		if cfg.BotTokenRef == "" && cfg.BotToken != "" {
			cfg.BotTokenRef = fmt.Sprintf("channel:%s:bot_token", channelName)
		}
		if cfg.BotTokenRef != "" && cfg.BotToken != "" {
			if err := a.credMgr.RegisterPatternWithContext(ctx, cfg.BotTokenRef, &credentials.AuthConfig{
				Type:  "secret",
				Token: cfg.BotToken,
			}); err != nil {
				return err
			}
			cfg.BotToken = ""
		}
	case "slack":
		if cfg.BotTokenRef == "" && cfg.BotToken != "" {
			cfg.BotTokenRef = fmt.Sprintf("channel:%s:bot_token", channelName)
		}
		if cfg.AppTokenRef == "" && cfg.AppToken != "" {
			cfg.AppTokenRef = fmt.Sprintf("channel:%s:app_token", channelName)
		}
		if cfg.BotTokenRef != "" && cfg.BotToken != "" {
			if err := a.credMgr.RegisterPatternWithContext(ctx, cfg.BotTokenRef, &credentials.AuthConfig{
				Type:  "secret",
				Token: cfg.BotToken,
			}); err != nil {
				return err
			}
			cfg.BotToken = ""
		}
		if cfg.AppTokenRef != "" && cfg.AppToken != "" {
			if err := a.credMgr.RegisterPatternWithContext(ctx, cfg.AppTokenRef, &credentials.AuthConfig{
				Type:  "secret",
				Token: cfg.AppToken,
			}); err != nil {
				return err
			}
			cfg.AppToken = ""
		}
	case "signal":
		if cfg.APITokenRef == "" && cfg.APIToken != "" {
			cfg.APITokenRef = fmt.Sprintf("channel:%s:api_token", channelName)
		}
		if cfg.APITokenRef != "" && cfg.APIToken != "" {
			if err := a.credMgr.RegisterPatternWithContext(ctx, cfg.APITokenRef, &credentials.AuthConfig{
				Type:  "secret",
				Token: cfg.APIToken,
			}); err != nil {
				return err
			}
			cfg.APIToken = ""
		}
	}

	return nil
}

// RestartChannel reconecta um canal de mensageria (exposto ao frontend).
func (a *App) RestartChannel(channelName string) error {
	cfg, err := channels.Load(channelName)
	if err != nil {
		return fmt.Errorf("erro ao carregar config do canal %s: %w", channelName, err)
	}
	if cfg == nil {
		return fmt.Errorf("canal %s não configurado", channelName)
	}
	a.restartChannel(channelName, cfg)
	return nil
}

// restartChannel desconecta o canal anterior (se houver) e reconecta com a nova config.
func (a *App) restartChannel(channelName string, cfg *channels.ChannelConfig) {
	if a.msgGateway == nil {
		log.Printf("[Messaging] Gateway não inicializado, ignorando restart de %s", channelName)
		return
	}

	// Desconecta o adapter anterior
	a.msgGateway.Unregister(channelName)

	if !cfg.Enabled {
		log.Printf("[Messaging] Canal %s desabilitado", channelName)
		return
	}

	switch channelName {
	case "telegram":
		botToken := cfg.BotToken
		if botToken == "" && cfg.BotTokenRef != "" {
			botToken = a.resolveCredentialRef(cfg.BotTokenRef)
		}
		if botToken == "" {
			log.Printf("[Messaging] Telegram: bot token vazio, não conectando")
			return
		}
		adapter := telegram.NewAdapter(botToken)
		a.msgGateway.Register("telegram", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar Telegram: %v", err)
			}
		}()
		log.Printf("[Messaging] Telegram reconectado")

	case "signal":
		if cfg.Account == "" || cfg.APIURL == "" {
			log.Printf("[Messaging] Signal: conta ou URL da API vazios, não conectando")
			return
		}
		adapter := signal.NewAdapter(cfg.APIURL, cfg.Account, a.credMgr)
		a.msgGateway.Register("signal", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar Signal: %v", err)
			}
		}()
		log.Printf("[Messaging] Signal reconectado (api=%s, account=%s)", cfg.APIURL, maskIdentifier(cfg.Account))

	case "slack":
		botToken := cfg.BotToken
		appToken := cfg.AppToken
		if botToken == "" && cfg.BotTokenRef != "" {
			botToken = a.resolveCredentialRef(cfg.BotTokenRef)
		}
		if appToken == "" && cfg.AppTokenRef != "" {
			appToken = a.resolveCredentialRef(cfg.AppTokenRef)
		}
		if botToken == "" || appToken == "" {
			log.Printf("[Messaging] Slack: bot/app token vazios, não conectando")
			return
		}
		adapter := slack.NewAdapter(botToken, appToken)
		a.msgGateway.Register("slack", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar Slack: %v", err)
			}
		}()
		log.Printf("[Messaging] Slack reconectado")

	default:
		log.Printf("[Messaging] Canal desconhecido: %s", channelName)
	}
}

// GetAllChannelConfigs retorna as configurações de todos os canais.
func (a *App) GetAllChannelConfigs() (map[string]*channels.ChannelConfig, error) {
	return channels.ListAll()
}

// GetChannelTemplates retorna todos os templates disponíveis para criar novos canais.
func (a *App) GetChannelTemplates() []channels.ChannelTemplate {
	all := channels.GetAvailableTemplates()
	supported := a.getSupportedChannelTypes()
	filtered := make([]channels.ChannelTemplate, 0, len(all))
	for _, t := range all {
		if _, ok := supported[t.Type]; ok {
			t.Supported = true
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// getSupportedChannelTypes retorna os tipos de canais suportados pelo backend.
// Mantém este mapa alinhado com os adapters disponíveis (initMessaging/restartChannel).
func (a *App) getSupportedChannelTypes() map[string]struct{} {
	return map[string]struct{}{
		"telegram": {},
		"signal":   {},
		"slack":    {},
	}
}

// CreateChannelFromTemplate cria um novo canal a partir de um template.
// templateType: "telegram", "signal", "whatsapp", "slack", "teams", "email"
// values: mapa com os valores dos campos (ex: {"bot_token": "xxx", "max_contacts": 5})
func (a *App) CreateChannelFromTemplate(templateType string, values map[string]interface{}) error {
	if err := channels.CreateFromTemplate(templateType, values); err != nil {
		return err
	}

	// Emite evento para atualizar UI
	runtime.EventsEmit(a.ctx, "channel:created", map[string]string{"type": templateType})

	return nil
}

// GetChannelConfigAsMap retorna a configuração de um canal como mapa para exibir na UI.
func (a *App) GetChannelConfigAsMap(channelName string) (map[string]interface{}, error) {
	return channels.GetChannelConfigAsMap(channelName)
}

// SignalRegister inicia o registro de uma conta Signal via signal-cli-rest-api.
// mode: "sms" (padrão) ou "voice" para receber o código por ligação.
// captcha: token de verificação exigido pelo Signal (signalcaptcha://...).
func (a *App) SignalRegister(apiURL, number, mode, captcha, apiToken string) error {
	return signal.Register(apiURL, number, mode, captcha, apiToken)
}

// SignalVerify verifica o código recebido via SMS/ligação.
func (a *App) SignalVerify(apiURL, number, code, apiToken string) error {
	return signal.Verify(apiURL, number, code, apiToken)
}

// SignalLink gera o QR code para vincular como dispositivo secundário.
// Retorna a imagem QR code em base64 (PNG).
func (a *App) SignalLink(apiURL, deviceName, apiToken string) (string, error) {
	return signal.GetLinkQRCode(apiURL, deviceName, apiToken)
}

// SignalLinkRaw gera a URI texto para vincular como dispositivo secundário.
// Alternativa acessível ao QR code.
func (a *App) SignalLinkRaw(apiURL, deviceName, apiToken string) (string, error) {
	return signal.GetLinkRawURI(apiURL, deviceName, apiToken)
}

// SignalUnregister remove uma conta da signal-cli-rest-api.
func (a *App) SignalUnregister(apiURL, number string, deleteLocalData bool, apiToken string) error {
	return signal.Unregister(apiURL, number, deleteLocalData, apiToken)
}

// SignalCheckAPI verifica se a signal-cli-rest-api está acessível na URL informada.
func (a *App) SignalCheckAPI(apiURL, apiToken string) (map[string]interface{}, error) {
	return signal.CheckAPI(apiURL, apiToken)
}

// SignalListAccounts retorna as contas já registradas na signal-cli-rest-api.
func (a *App) SignalListAccounts(apiURL, apiToken string) ([]string, error) {
	return signal.ListAccounts(apiURL, apiToken)
}

// AuthorizeMessagingContactFull autoriza um contato em um canal.
// Respeita o limite max_contacts configurado no canal.
func (a *App) AuthorizeMessagingContactFull(channel, contactID, displayName, username string) error {
	if channel == "" || contactID == "" {
		return fmt.Errorf("canal e ID do contato são obrigatórios")
	}

	// Obtém max_contacts do config do canal
	maxContacts := 1
	if chCfg, _ := channels.Load(channel); chCfg != nil {
		maxContacts = chCfg.GetMaxContacts()
	}

	if err := contacts.Authorize(channel, contactID, displayName, username, maxContacts); err != nil {
		return fmt.Errorf("erro ao autorizar: %w", err)
	}
	log.Printf("[Contacts] Contato %s (%s) autorizado no canal %s", displayName, contactID, channel)
	return nil
}

// RemoveAuthorizedContact remove um contato específico de um canal.
func (a *App) RemoveAuthorizedContact(channel, contactID string) error {
	if channel == "" || contactID == "" {
		return fmt.Errorf("canal e ID do contato são obrigatórios")
	}
	if err := contacts.Remove(channel, contactID); err != nil {
		return fmt.Errorf("erro ao remover contato: %w", err)
	}
	log.Printf("[Contacts] Contato %s removido do canal %s", contactID, channel)
	return nil
}

// GetAuthorizedContacts retorna todos os contatos autorizados (mapa canal → lista).
func (a *App) GetAuthorizedContacts() (contacts.ContactsFile, error) {
	return contacts.GetAll()
}

// registerChannelBridge verifica se uma conversa pertence a um canal externo (Signal, Telegram).
// Se sim, registra um callback no ResponseNotifier para que a resposta do assistente seja
// reenviada ao mensageiro de origem — permitindo bridge bidirecional (Wails ↔ Messenger).
func (a *App) registerChannelBridge(conversationID uint) {
	conv, err := database.GetConversationInfo(conversationID)
	if err != nil || conv == nil || conv.Channel == "" || conv.ContactID == "" {
		return // Conversa local do Wails, não precisa de bridge
	}

	messenger, ok := a.msgGateway.GetMessenger(conv.Channel)
	if !ok {
		return // Messenger não registrado
	}

	log.Printf("[Bridge] Registrando bridge Wails→%s para conversa %d (contato: %s)", conv.Channel, conversationID, conv.ContactID)

	a.responseNotifier.Register(conversationID, messaging.ResponseCallback{
		Channel: conv.Channel,
		ChatID:  conv.ContactID,
		Callback: func(response string, assistantMsgID uint) {
			err := messenger.Send(context.Background(), messaging.OutgoingMessage{
				ChatID: conv.ContactID,
				Text:   response,
			})
			if err != nil {
				log.Printf("[Bridge] Erro ao reenviar resposta para %s/%s: %v", conv.Channel, conv.ContactID, err)
			} else {
				log.Printf("[Bridge] Resposta reenviada para %s/%s", conv.Channel, conv.ContactID)
			}
		},
	})
}

// ChannelInfo descreve um canal de mensageria disponível para atribuição.
type ChannelInfo struct {
	Name        string                        `json:"name"`        // "signal", "telegram"
	Connected   bool                          `json:"connected"`   // se está conectado e funcional
	Contacts    []*contacts.AuthorizedContact `json:"contacts"`    // contatos autorizados
	MaxContacts int                           `json:"maxContacts"` // limite de contatos
}

// GetAvailableChannels retorna os canais habilitados, seu status e contatos autorizados.
func (a *App) GetAvailableChannels() []ChannelInfo {
	enabledChannels, err := channels.LoadEnabled()
	if err != nil {
		return nil
	}

	authorizedContacts, _ := contacts.GetAll()

	var result []ChannelInfo

	var status map[string]messaging.ConnectionStatus
	if a.msgGateway != nil {
		status = a.msgGateway.GetStatus()
	}

	for name, cfg := range enabledChannels {
		connected := false
		if s, ok := status[name]; ok {
			connected = s == messaging.StatusConnected
		}
		result = append(result, ChannelInfo{
			Name:        name,
			Connected:   connected,
			Contacts:    authorizedContacts[name],
			MaxContacts: cfg.GetMaxContacts(),
		})
	}

	return result
}

// AssignConversationToChannel vincula uma conversa existente a um canal externo.
// A partir deste momento, respostas do assistente nessa conversa também são enviadas ao canal.
func (a *App) AssignConversationToChannel(conversationID uint, channel, contactID string) error {
	if channel == "" || contactID == "" {
		return fmt.Errorf("canal e contato são obrigatórios")
	}

	conv, err := database.GetConversationInfo(conversationID)
	if err != nil {
		return fmt.Errorf("conversa %d não encontrada: %w", conversationID, err)
	}

	// Atualiza os campos de canal na conversa
	conv.Channel = channel
	conv.ContactID = contactID
	if err := database.UpdateConversationChannel(conversationID, channel, contactID); err != nil {
		return fmt.Errorf("erro ao atualizar conversa: %w", err)
	}

	log.Printf("[Bridge] Conversa %d atribuída ao canal %s (contato: %s)", conversationID, channel, contactID)
	return nil
}

// UnassignConversationFromChannel remove a vinculação de uma conversa com um canal externo.
func (a *App) UnassignConversationFromChannel(conversationID uint) error {
	if err := database.UpdateConversationChannel(conversationID, "", ""); err != nil {
		return fmt.Errorf("erro ao remover canal da conversa: %w", err)
	}

	log.Printf("[Bridge] Conversa %d desvinculada de canal externo", conversationID)
	return nil
}

// GetConversationChannel retorna o canal e contato vinculados a uma conversa.
func (a *App) GetConversationChannel(conversationID uint) (string, string, error) {
	conv, err := database.GetConversationInfo(conversationID)
	if err != nil {
		return "", "", err
	}
	return conv.Channel, conv.ContactID, nil
}

// AudioResult é o resultado de busca/geração de áudio para o frontend.
type AudioResult struct {
	Audio    string `json:"audio"`
	MimeType string `json:"mimeType"`
}

// CredentialSummary descreve uma credencial para exibição (sem dados sensíveis).
type CredentialSummary struct {
	Pattern string `json:"pattern"`
	Type    string `json:"type"`
	Masked  string `json:"masked"`
	Managed bool   `json:"managed"`
}

// CredentialInput descreve a entrada para criar/atualizar credenciais.
type CredentialInput struct {
	Pattern     string `json:"pattern"`
	Type        string `json:"type"`
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"headerName,omitempty"`
	HeaderValue string `json:"headerValue,omitempty"`
}

// ExternalSourceSuggestion representa uma sugestão de fonte externa para autocomplete.
type ExternalSourceSuggestion struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ListExternalSources lista fontes externas disponíveis para autocomplete.
// prefix deve ser "keyring://" ou "env://".
func (a *App) ListExternalSources(prefix string) ([]ExternalSourceSuggestion, error) {
	switch prefix {
	case "keyring://":
		return a.listKeyringEntries()
	case "env://":
		return a.listEnvVars()
	default:
		return []ExternalSourceSuggestion{}, nil
	}
}

func (a *App) listEnvVars() ([]ExternalSourceSuggestion, error) {
	envs := os.Environ()
	suggestions := make([]ExternalSourceSuggestion, 0, len(envs))

	skipPrefixes := []string{"PROCESSOR_", "SYSTEM", "WINDOWS", "COMMON"}
	skipExact := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"TEMP": true, "TMP": true, "OS": true,
		"HOMEDRIVE": true, "HOMEPATH": true,
		"USERDOMAIN": true, "USERNAME": true,
		"LOCALAPPDATA": true, "APPDATA": true,
		"PROGRAMFILES": true, "PROGRAMDATA": true,
		"WINDIR": true, "SYSTEMROOT": true,
		"COMPUTERNAME": true, "NUMBER_OF_PROCESSORS": true,
		"PROGRAMFILES(X86)": true, "PSMODULEPATH": true,
		"PUBLIC": true, "SESSIONNAME": true,
		"USERPROFILE": true, "ALLUSERSPROFILE": true,
	}

	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		name := parts[0]
		upper := strings.ToUpper(name)

		if skipExact[upper] {
			continue
		}
		skip := false
		for _, p := range skipPrefixes {
			if strings.HasPrefix(upper, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		suggestions = append(suggestions, ExternalSourceSuggestion{
			Value: "env://" + name,
			Label: name,
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Label < suggestions[j].Label
	})
	return suggestions, nil
}

func (a *App) listKeyringEntries() ([]ExternalSourceSuggestion, error) {
	entries, err := credentials.ListKeyringEntries()
	if err != nil {
		return nil, err
	}

	suggestions := make([]ExternalSourceSuggestion, 0, len(entries))
	for _, e := range entries {
		ref := "keyring://" + e.Target
		suggestions = append(suggestions, ExternalSourceSuggestion{
			Value: ref,
			Label: e.Target,
		})
	}
	return suggestions, nil
}

// GetMessageAudio retorna o áudio base64 e MIME type de uma mensagem.
func (a *App) GetMessageAudio(messageID uint) (*AudioResult, error) {
	audio, mime, err := database.GetMessageAudio(messageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar áudio: %w", err)
	}
	if audio == "" {
		return nil, nil
	}
	return &AudioResult{Audio: audio, MimeType: mime}, nil
}

// SaveMessageAudio salva áudio (base64) numa mensagem existente.
// Usado pelo frontend para persistir áudio gerado via TTS OpenAI.
func (a *App) SaveMessageAudio(messageID uint, audioBase64 string, mimeType string) error {
	return database.SaveMessageAudio(messageID, audioBase64, mimeType)
}

// GenerateAndSaveMessageAudio gera áudio TTS para uma mensagem e salva no DB.
// Retorna o áudio base64 e MIME type. Usado pelo frontend para gerar+salvar+tocar.
func (a *App) GenerateAndSaveMessageAudio(messageID uint, text string) (*AudioResult, error) {
	if !a.ensureSpeechManager() {
		return nil, fmt.Errorf("speech manager indisponível")
	}

	result, err := a.speechManager.Synthesize(text)
	if err != nil {
		return nil, fmt.Errorf("erro ao sintetizar TTS: %w", err)
	}

	mimeType := "audio/mpeg"
	// Salva no DB
	if err := database.SaveMessageAudio(messageID, result.AudioBase64, mimeType); err != nil {
		log.Printf("[TTS] Erro ao salvar áudio no DB: %v", err)
		// Retorna o áudio mesmo se falhar ao salvar
	}

	return &AudioResult{Audio: result.AudioBase64, MimeType: mimeType}, nil
}

// appTaskListManager adapta o App para a interface tasklisttool.TaskListManager
type appTaskListManager struct{}

func (m *appTaskListManager) CreateTaskList(title, description string, conversationID *uint, templateWorkflow *database.TaskListWorkflow) (*database.TaskList, error) {
	return database.CreateTaskList(title, description, conversationID != nil, conversationID, templateWorkflow)
}

func (m *appTaskListManager) GetTaskList(id uint) (*database.TaskList, error) {
	return database.GetTaskList(id)
}

func (m *appTaskListManager) GetAllTaskLists() ([]database.TaskList, error) {
	return database.GetAllTaskLists()
}

func (m *appTaskListManager) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	return database.GetTaskListStats(taskListID)
}

func (m *appTaskListManager) CreateTask(taskListID uint, title, description string, parentID *uint) (*database.Task, error) {
	return database.CreateTask(taskListID, title, description, parentID)
}

func (m *appTaskListManager) GetTask(id uint) (*database.Task, error) {
	return database.GetTask(id)
}

func (m *appTaskListManager) UpdateTask(id uint, title, description string) error {
	return database.UpdateTask(id, title, description)
}

func (m *appTaskListManager) UpdateTaskStatus(id uint, newStatusID int) error {
	return database.UpdateTaskStatus(id, newStatusID)
}

func (m *appTaskListManager) DeleteTask(id uint) error {
	return database.DeleteTask(id)
}

func (m *appTaskListManager) GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error) {
	return database.GetWorkflow(taskListID)
}
// initToolRegistry inicializa o registro de ferramentas disponíveis
func (a *App) initToolRegistry() {
	a.toolRegistry = tools.NewRegistry()
	a.toolExecutor = tools.NewExecutor(a.toolRegistry, tools.DefaultExecutorConfig())

	// Determina diretório de trabalho para as tools de filesystem
	workDir, err := os.Getwd()
	if err != nil {
		log.Printf("[Tools] Erro ao obter diretório de trabalho: %v", err)
		workDir = "."
	}

	// Registra ferramentas de filesystem
	a.toolRegistry.MustRegister(filesystem.NewReadFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewListDirectory(workDir))
	a.toolRegistry.MustRegister(filesystem.NewSearchFiles(workDir))
	a.toolRegistry.MustRegister(filesystem.NewGrepSearch(workDir))
	a.toolRegistry.MustRegister(filesystem.NewWriteFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewEditFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewMoveFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewCopyFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewDeleteFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewMakeDirectory(workDir))

	// Registra ferramentas web (credMgr já foi inicializado antes)
	a.toolRegistry.MustRegister(web.NewWebFetch(a.credMgr)) // GET simples, foco em leitura

	// HTTPRequest com CredentialManager (autenticação automática por domínio)
	httpReqTool := web.NewHTTPRequest(a.credMgr)

	// Confirmação para operações destrutivas
	httpReqTool.SetConfirmFunc(func(ctx context.Context, method, url, body string) (bool, error) {
		if a.questionnaireMgr == nil {
			return false, fmt.Errorf("questionnaire manager não inicializado")
		}
		bodyPreview := body
		if bodyPreview == "" {
			bodyPreview = "(sem body)"
		}
		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       fmt.Sprintf("Confirmar operação %s", method),
			Description: fmt.Sprintf("O assistente quer executar:\n\n%s %s\n\nBody:\n%s", method, url, bodyPreview),
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Negar",
			Questions: []questionnaire.Question{
				{
					ID:       "approve",
					Type:     "boolean",
					Prompt:   fmt.Sprintf("Permitir esta operação %s?", method),
					Required: true,
				},
			},
		})
		if err != nil {
			return false, err
		}
		if resp.Cancelled {
			return false, nil
		}
		approved, ok := resp.Answers["approve"].(bool)
		if !ok {
			return false, fmt.Errorf("resposta inválida para aprovação")
		}
		return approved, nil
	})
	a.toolRegistry.MustRegister(httpReqTool)

	a.toolRegistry.MustRegister(web.NewWebSearch(a.credMgr))

	// Registra ferramenta de shell (run_command)
	confirmFn := func(ctx context.Context, cmd, wd string) (bool, error) {
		if a.questionnaireMgr == nil {
			return false, fmt.Errorf("questionnaire manager não inicializado")
		}
		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       "Confirmar execução de comando",
			Description: fmt.Sprintf("O assistente quer executar:\n\n%s\n\nem: %s", cmd, wd),
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Negar",
			Questions: []questionnaire.Question{
				{
					ID:       "approve",
					Type:     "boolean",
					Prompt:   "Permitir a execução deste comando?",
					Required: true,
				},
			},
		})
		if err != nil {
			return false, err
		}
		if resp.Cancelled {
			return false, nil
		}
		approved, ok := resp.Answers["approve"].(bool)
		if !ok {
			return false, fmt.Errorf("resposta inválida para aprovação de comando")
		}
		return approved, nil
	}
	getAllowlistFn := func() *allowlist.Allowlist {
		activeProfile, err := a.profileManager.GetActive()
		if err != nil || activeProfile == nil {
			// Sem perfil ativo: usa allowlist padrão
			al, err := a.allowlistMgr.Get("padrao")
			if err != nil {
				return nil // sem allowlist = tudo requer confirmação
			}
			return al
		}
		if activeProfile.Chat.CommandAllowlist == "" {
			// Perfil sem allowlist configurada: usa a padrão
			al, err := a.allowlistMgr.Get("padrao")
			if err != nil {
				return nil
			}
			return al
		}
		al, err := a.allowlistMgr.Get(activeProfile.Chat.CommandAllowlist)
		if err != nil {
			log.Printf("[Tools] Allowlist '%s' não encontrada, usando confirmação para tudo", activeProfile.Chat.CommandAllowlist)
			return nil
		}
		return al
	}
	a.toolRegistry.MustRegister(shell.NewRunCommand(a.terminalMgr, confirmFn, getAllowlistFn, workDir))

	// Registra ferramenta de questionário (collect_responses)
	a.toolRegistry.MustRegister(questiontool.NewCollectResponses(a.questionnaireMgr))

	// Registra ferramenta de edição de texto (opt-in: só disponível em perfis que a listam explicitamente)
	a.toolRegistry.MustRegisterOptIn(editor.NewTextEdit(a.questionnaireMgr))

	// Registra ferramenta de busca no histórico de conversas
	a.toolRegistry.MustRegister(history.NewSearchConversations())

	// Registra ferramentas de gerenciamento de task lists
	tlMgr := &appTaskListManager{}
	a.toolRegistry.MustRegister(tasklisttool.NewCreateTaskList(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewListTaskLists(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewGetTaskList(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewGetTaskListStatus(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewUpsertTask(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewBulkUpsertTasks(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewDeleteTask(tlMgr))

	log.Printf("[Tools] Registry inicializado com %d ferramentas: %v", a.toolRegistry.Count(), a.toolRegistry.Names())
}

func (a *App) registerEnvCredentials(credMgr *credentials.Manager) {
	if credMgr == nil {
		return
	}

	// GITHUB_TOKEN -> *.github.com, github.com
	if ghToken := os.Getenv("GITHUB_TOKEN"); ghToken != "" {
		_ = credMgr.RegisterPattern("*.github.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: ghToken,
		})
		_ = credMgr.RegisterPattern("github.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: ghToken,
		})
	}

	// GITLAB_TOKEN -> *.gitlab.com, gitlab.com
	if glToken := os.Getenv("GITLAB_TOKEN"); glToken != "" {
		_ = credMgr.RegisterPattern("*.gitlab.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: glToken,
		})
		_ = credMgr.RegisterPattern("gitlab.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: glToken,
		})
	}

	// BITBUCKET_TOKEN -> *.bitbucket.org, bitbucket.org
	if bbToken := os.Getenv("BITBUCKET_TOKEN"); bbToken != "" {
		_ = credMgr.RegisterPattern("*.bitbucket.org", &credentials.AuthConfig{
			Type:  "bearer",
			Token: bbToken,
		})
		_ = credMgr.RegisterPattern("bitbucket.org", &credentials.AuthConfig{
			Type:  "bearer",
			Token: bbToken,
		})
	}

	// API genérica - X_API_KEY para qualquer host (fallback)
	// Usar com cuidado - preferir padrões específicos acima
	if apiKey := os.Getenv("GENERIC_API_KEY"); apiKey != "" {
		_ = credMgr.RegisterPattern("*", &credentials.AuthConfig{
			Type: "custom",
			Headers: map[string]string{
				"X-API-Key": apiKey,
			},
		})
	}
}

func (a *App) configureCredentialManager(dek []byte, persist bool) {
	if a.credStore == nil {
		a.credStore = credentials.NewDBStore()
	}
	if a.credMgr == nil {
		a.credMgr = credentials.NewManagerWithStore(dek, a.credStore, persist)
	} else {
		a.credMgr.Reset(dek, persist)
	}

	if err := a.credMgr.LoadFromStore(context.Background()); err != nil {
		log.Printf("[Credentials] Erro ao carregar credenciais persistidas: %v", err)
	}
	a.registerEnvCredentials(a.credMgr)
}

// ToolInfo é um resumo de uma ferramenta para listagem no frontend.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GetAvailableTools retorna a lista de ferramentas registradas no registry.
// Usado pelo frontend para exibir checkboxes no editor de perfis.
func (a *App) GetAvailableTools() []ToolInfo {
	if a.toolRegistry == nil {
		return []ToolInfo{}
	}

	allTools := a.toolRegistry.All()
	result := make([]ToolInfo, len(allTools))
	for i, t := range allTools {
		result[i] = ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
		}
	}
	return result
}

// shutdown é chamado quando o app fecha
func (a *App) shutdown(_ context.Context) {
	a.stopAllEditorWatches()

	if a.hotkeyManager != nil {
		a.hotkeyManager.Stop()
	}

	// Encerra todos os servidores MCP
	if a.mcpMgr != nil {
		a.mcpMgr.CloseAll()
	}

	// Encerra todas as sessões de terminal
	if a.terminalMgr != nil {
		a.terminalMgr.CloseAll()
	}

	// Encerra todos os mensageiros
	if a.msgGateway != nil {
		a.msgGateway.Shutdown()
	}
}

// ============================================================================
// Terminal Management API (sessões PTY compartilhadas LLM + usuário)
// ============================================================================

// ListTerminalSessions retorna todas as sessões de terminal ativas.
func (a *App) ListTerminalSessions() []terminal.SessionInfo {
	if a.terminalMgr == nil {
		return []terminal.SessionInfo{}
	}
	return a.terminalMgr.List()
}

// CreateTerminalSession cria uma nova sessão de terminal.
func (a *App) CreateTerminalSession(name string) (*terminal.SessionInfo, error) {
	if a.terminalMgr == nil {
		return nil, fmt.Errorf("terminal manager não inicializado")
	}

	workDir, _ := os.Getwd()
	session, err := a.terminalMgr.Create(name, workDir)
	if err != nil {
		return nil, err
	}

	info := session.Info()
	return &info, nil
}

// CloseTerminalSession encerra uma sessão de terminal.
func (a *App) CloseTerminalSession(sessionID string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.Close(sessionID)
}

// GetTerminalHistory retorna o histórico de comandos de uma sessão.
func (a *App) GetTerminalHistory(sessionID string) ([]terminal.HistoryEntry, error) {
	if a.terminalMgr == nil {
		return nil, fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.GetHistory(sessionID)
}

// RunTerminalCommand executa um comando com markers em uma sessão de terminal.
// Mantido para compatibilidade — usado internamente pelo LLM.
func (a *App) RunTerminalCommand(sessionID string, command string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}

	// Executa em goroutine para não bloquear o binding
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		_, err := a.terminalMgr.RunCommand(ctx, sessionID, command, 0, "user")
		if err != nil {
			log.Printf("[Terminal] Erro ao executar comando: %v", err)
		}
	}()

	return nil
}

// SendTerminalInput envia input raw para uma sessão de terminal (modo interativo).
// Diferente de RunTerminalCommand, não usa markers — o input vai direto ao PTY.
// Suporta comandos interativos (wsl, python, ssh, etc.) e input para programas em execução.
func (a *App) SendTerminalInput(sessionID string, input string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}

	_, err := a.terminalMgr.SendInput(sessionID, input)
	if err != nil {
		log.Printf("[Terminal] Erro ao enviar input: %v", err)
		return err
	}
	return nil
}

// saveLLMProviders salva os provedores no SQLite
func (a *App) saveLLMProviders() error {
	providers := a.llmRegistry.List()

	for _, p := range providers {
		dbProvider := &database.LLMProvider{
			ID:                p.ID,
			Name:              p.Name,
			Type:              string(p.Type),
			BaseURL:           p.BaseURL,
			Model:             p.Model,
			Timeout:           p.Timeout,
			CredentialPattern: p.CredentialPattern,
		}
		if err := database.SaveLLMProvider(dbProvider); err != nil {
			log.Printf("Erro ao salvar provedor %s: %v", p.ID, err)
			return err
		}
	}

	return nil
}

// loadLLMProviders carrega provedores do SQLite
func (a *App) loadLLMProviders() error {
	providers, err := database.GetLLMProviders()
	if err != nil {
		return err
	}

	if len(providers) == 0 {
		return fmt.Errorf("nenhum provedor encontrado")
	}

	for _, dbProvider := range providers {
		p := &llm.ProviderConfig{
			ID:                dbProvider.ID,
			Name:              dbProvider.Name,
			Type:              llm.ProviderType(dbProvider.Type),
			BaseURL:           dbProvider.BaseURL,
			Model:             dbProvider.Model,
			Timeout:           dbProvider.Timeout,
			CredentialPattern: dbProvider.CredentialPattern,
		}
		if err := a.llmRegistry.Register(p); err != nil {
			log.Printf("Erro ao registrar provedor %s: %v", p.ID, err)
		}
	}

	log.Printf("Provedores LLM carregados do SQLite: %d", len(providers))
	return nil
}

// InterruptTerminalCommand envia Ctrl+C para uma sessão de terminal.
func (a *App) InterruptTerminalCommand(sessionID string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.Interrupt(sessionID)
}

// GetTerminalStats retorna estatísticas do gerenciador de terminais.
func (a *App) GetTerminalStats() *terminal.ManagerStats {
	if a.terminalMgr == nil {
		return &terminal.ManagerStats{}
	}
	stats := a.terminalMgr.Stats()
	return &stats
}

// RespondQuestionnaire responde a uma solicitação de questionário.
// Chamado pelo frontend quando o usuário envia ou cancela o questionário.
func (a *App) RespondQuestionnaire(requestID string, answers map[string]any, cancelled bool) error {
	if a.questionnaireMgr == nil {
		return fmt.Errorf("questionnaire manager não inicializado")
	}
	return a.questionnaireMgr.Respond(requestID, answers, cancelled)
}

// ============================================================================
// Allowlist Management API
// ============================================================================

// GetAllowlists retorna a lista de allowlists disponíveis.
func (a *App) GetAllowlists() ([]allowlist.AllowlistInfo, error) {
	if a.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.List()
}

// GetAllowlist retorna uma allowlist pelo slug.
func (a *App) GetAllowlist(slug string) (*allowlist.Allowlist, error) {
	if a.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Get(slug)
}

// CreateAllowlist cria uma nova allowlist.
func (a *App) CreateAllowlist(al allowlist.Allowlist) (string, error) {
	if a.allowlistMgr == nil {
		return "", fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Create(&al)
}

// UpdateAllowlist atualiza uma allowlist existente.
func (a *App) UpdateAllowlist(slug string, al allowlist.Allowlist) error {
	if a.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Update(slug, &al)
}

// DeleteAllowlist exclui uma allowlist.
func (a *App) DeleteAllowlist(slug string) error {
	if a.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Delete(slug)
}

// GetAllowlistSearchPaths retorna os caminhos de busca de allowlists.
func (a *App) GetAllowlistSearchPaths() []string {
	if a.allowlistMgr == nil {
		return []string{}
	}
	return a.allowlistMgr.GetSearchPaths()
}

// ============================================================================
// Skills Management API
// ============================================================================

// initSkills inicializa o gerenciador de skills
func (a *App) initSkills() {
	a.skillMgr = skills.NewManager()
	if err := a.skillMgr.EnsureDir(); err != nil {
		log.Printf("[Skills] Erro ao garantir diretório de skills: %v", err)
	}

	a.installBuiltinSkills()

	list, err := a.skillMgr.List()
	if err != nil {
		log.Printf("[Skills] Erro ao listar skills: %v", err)
	} else {
		log.Printf("[Skills] Manager inicializado com %d skills", len(list))
	}
}

// initMemoryDir garante que o diretório memory/ existe no home (~/.assistente/memory/)
// e cria o arquivo memory.md inicial se não existir.
func (a *App) initMemoryDir() {
	resolver := configdir.NewResolver("memory")

	if err := resolver.EnsureHomeDir(); err != nil {
		log.Printf("[Memory] Erro ao criar diretório de memória: %v", err)
		return
	}

	// Cria memory.md se não existir em nenhum diretório
	if !resolver.Exists("memory.md") {
		initial := []byte("## Sobre o Usuário\n\n(Ainda não há memórias salvas. Quando o usuário compartilhar informações pessoais ou pedir para lembrar algo, registre aqui.)\n")
		if err := resolver.Create("memory.md", initial); err != nil {
			log.Printf("[Memory] Erro ao criar memory.md: %v", err)
		} else {
			log.Printf("[Memory] memory.md criado em ~/.assistente/memory/")
		}
	} else {
		log.Printf("[Memory] memory.md encontrado")
	}

	// Garante que os subdiretórios de memória temporal existem no home
	homeDir := resolver.GetHomeDir()
	if homeDir != "" {
		for _, sub := range []string{"daily", "weekly", "monthly", "yearly"} {
			subPath := homeDir + string(os.PathSeparator) + sub
			if err := os.MkdirAll(subPath, 0755); err != nil {
				log.Printf("[Memory] Erro ao criar %s: %v", sub, err)
			}
		}
	}
}

// GetSkills retorna a lista de skills disponíveis (metadados apenas).
func (a *App) GetSkills() ([]skills.SkillInfo, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.List()
}

// GetSkill retorna um skill completo pelo slug.
func (a *App) GetSkill(slug string) (*skills.Skill, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.Get(slug)
}

// SkillCreateRequest é o payload para criar/atualizar um skill via frontend.
// Contém a SkillMetadata completa conforme spec + conteúdo Markdown.
type SkillCreateRequest struct {
	skills.SkillMetadata `json:",inline"`
	Content              string `json:"content"`
}

// CreateSkill cria um novo skill.
func (a *App) CreateSkill(req SkillCreateRequest) (string, error) {
	if a.skillMgr == nil {
		return "", fmt.Errorf("skill manager não inicializado")
	}

	meta := req.SkillMetadata
	slug, err := a.skillMgr.Create(&meta, req.Content)
	if err != nil {
		return "", err
	}

	runtime.EventsEmit(a.ctx, "skill:created", map[string]interface{}{
		"slug": slug,
		"name": req.Name,
	})

	return slug, nil
}

// DuplicateSkill cria uma copia de um skill existente.
func (a *App) DuplicateSkill(slug string) (string, error) {
	if a.skillMgr == nil {
		return "", fmt.Errorf("skill manager não inicializado")
	}

	newSlug, err := a.skillMgr.Duplicate(slug)
	if err != nil {
		return "", err
	}

	name := ""
	if copied, err := a.skillMgr.Get(newSlug); err == nil && copied != nil {
		name = copied.Name
	}

	runtime.EventsEmit(a.ctx, "skill:created", map[string]interface{}{
		"slug": newSlug,
		"name": name,
	})

	return newSlug, nil
}

// UpdateSkill atualiza um skill existente.
func (a *App) UpdateSkill(slug string, req SkillCreateRequest) error {
	if a.skillMgr == nil {
		return fmt.Errorf("skill manager não inicializado")
	}

	meta := req.SkillMetadata
	if err := a.skillMgr.Update(slug, &meta, req.Content); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "skill:updated", map[string]interface{}{
		"slug": slug,
		"name": req.Name,
	})

	return nil
}

// DeleteSkill exclui um skill.
func (a *App) DeleteSkill(slug string) error {
	if a.skillMgr == nil {
		return fmt.Errorf("skill manager não inicializado")
	}

	if err := a.skillMgr.Delete(slug); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "skill:deleted", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// GetUserInvocableSkills retorna skills que o usuário pode invocar via /slash.
func (a *App) GetUserInvocableSkills() ([]skills.SkillInfo, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.GetUserInvocableSkills()
}

// GetSkillSearchPaths retorna os caminhos de busca de skills.
func (a *App) GetSkillSearchPaths() []string {
	if a.skillMgr == nil {
		return []string{}
	}
	return a.skillMgr.GetSearchPaths()
}

// ============================================================================
// MCP Server Management API
// ============================================================================

// ListMCPServers retorna informações de todos os servidores MCP configurados.
func (a *App) ListMCPServers() []mcpmgr.ServerInfo {
	if a.mcpMgr == nil {
		return []mcpmgr.ServerInfo{}
	}
	return a.mcpMgr.List()
}

// ConnectMCPServer conecta a um servidor MCP pelo slug.
func (a *App) ConnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Connect(slug)
}

// DisconnectMCPServer desconecta de um servidor MCP.
func (a *App) DisconnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Disconnect(slug)
}

// ReconnectMCPServer reconecta a um servidor MCP.
func (a *App) ReconnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Reconnect(slug)
}

// SaveMCPServer salva (cria ou atualiza) a configuração de um servidor MCP.
func (a *App) SaveMCPServer(slug string, cfg mcpmgr.ServerConfig) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SaveConfig(slug, cfg)
}

// DuplicateMCPServer cria uma copia da configuracao de um servidor MCP.
func (a *App) DuplicateMCPServer(slug string) (string, error) {
	if a.mcpMgr == nil {
		return "", fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.DuplicateConfig(slug)
}

// DeleteMCPServer remove a configuração de um servidor MCP.
func (a *App) DeleteMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.DeleteConfig(slug)
}

// GetMCPServerTools retorna as ferramentas de um servidor MCP específico.
func (a *App) GetMCPServerTools(slug string) []mcpmgr.MCPToolInfo {
	if a.mcpMgr == nil {
		return []mcpmgr.MCPToolInfo{}
	}
	return a.mcpMgr.GetTools(slug)
}

// GetMCPServerConfig retorna a configuração de um servidor MCP.
func (a *App) GetMCPServerConfig(slug string) (*mcpmgr.ServerConfig, error) {
	if a.mcpMgr == nil {
		return nil, fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.GetConfig(slug)
}

// ReadMCPResource lê o conteúdo de um resource MCP.
func (a *App) ReadMCPResource(slug, uri string) (string, error) {
	if a.mcpMgr == nil {
		return "", fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.ReadResource(slug, uri)
}

// GetMCPPrompt executa um prompt MCP e retorna as mensagens geradas.
func (a *App) GetMCPPrompt(slug, name string, arguments map[string]string) ([]string, error) {
	if a.mcpMgr == nil {
		return nil, fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.GetPrompt(slug, name, arguments)
}

// GetNativeMCPServers retorna informações dos servidores MCP para uso nativo por modelos.
func (a *App) GetNativeMCPServers() []map[string]any {
	if a.mcpMgr == nil {
		return []map[string]any{}
	}
	return a.mcpMgr.GetNativeServerInfo()
}

// LLMSettings contém configurações da API LLM
type LLMSettings struct {
	APIKey  string
	BaseURL string
}

// ============================================================================
// Token Stats API
// ============================================================================

// ToolUsageBreakdownResult contém informações de uso de uma ferramenta
type ToolUsageBreakdownResult struct {
	ToolName              string `json:"toolName"`
	CallCount             int    `json:"callCount"`
	TotalPromptTokens     int    `json:"totalPromptTokens"`
	TotalCompletionTokens int    `json:"totalCompletionTokens"`
	TotalTokens           int    `json:"totalTokens"`
}

// TokenStatsResult representa estatísticas de tokens para o frontend
type TokenStatsResult struct {
	ConversationID   uint    `json:"conversationId"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	MessageCount     int     `json:"messageCount"`
	Model            string  `json:"model"`
	MostUsedModel    string  `json:"mostUsedModel"`
	ContextUsage     float64 `json:"contextUsage"` // Porcentagem de uso do contexto (0-100)
	ContextLimit     int     `json:"contextLimit"` // Limite de tokens do modelo
	IsNearLimit      bool    `json:"isNearLimit"`  // True se >= 80% do limite
	IsCritical       bool    `json:"isCritical"`   // True se >= 95% do limite

	// Breakdown detalhado de tokens
	SystemPromptEstimatedTokens int                        `json:"systemPromptEstimatedTokens"`
	SummaryTokens               int                        `json:"summaryTokens"`
	MessagesInContextCount      int                        `json:"messagesInContextCount"`
	MessagesInContextTokens     int                        `json:"messagesInContextTokens"`
	MessagesOutOfContextCount   int                        `json:"messagesOutOfContextCount"`
	MessagesOutOfContextTokens  int                        `json:"messagesOutOfContextTokens"`
	ToolsUsedCount              int                        `json:"toolsUsedCount"`
	ToolBreakdown               []ToolUsageBreakdownResult `json:"toolBreakdown"`
}

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa
func (a *App) GetConversationTokenStats(conversationID uint) (*TokenStatsResult, error) {
	// Recuperar summaryUpToMessageID para cálculo de mensagens in/out of context
	summaryUpToMessageID := uint(0)
	summary, upToID, _ := database.GetConversationSummary(conversationID)
	if summary != "" {
		summaryUpToMessageID = upToID
	}

	detailedStats, err := database.GetDetailedTokenStats(conversationID, summaryUpToMessageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar estatísticas de tokens: %w", err)
	}

	// Map tool usage breakdown
	toolBreakdown := make([]ToolUsageBreakdownResult, len(detailedStats.ToolBreakdown))
	for i, tool := range detailedStats.ToolBreakdown {
		toolBreakdown[i] = ToolUsageBreakdownResult{
			ToolName:              tool.ToolName,
			CallCount:             tool.CallCount,
			TotalPromptTokens:     tool.TotalPromptTokens,
			TotalCompletionTokens: tool.TotalCompletionTokens,
			TotalTokens:           tool.TotalTokens,
		}
	}

	result := &TokenStatsResult{
		ConversationID:              conversationID,
		PromptTokens:                detailedStats.PromptTokens,
		CompletionTokens:            detailedStats.CompletionTokens,
		TotalTokens:                 detailedStats.TotalTokens,
		MessageCount:                detailedStats.MessageCount,
		Model:                       detailedStats.Model,
		MostUsedModel:               detailedStats.Model,
		SystemPromptEstimatedTokens: detailedStats.SystemPromptEstimatedTokens,
		SummaryTokens:               detailedStats.SummaryTokens,
		MessagesInContextCount:      detailedStats.MessagesInContextCount,
		MessagesInContextTokens:     detailedStats.MessagesInContextTokens,
		MessagesOutOfContextCount:   detailedStats.MessagesOutOfContextCount,
		MessagesOutOfContextTokens:  detailedStats.MessagesOutOfContextTokens,
		ToolsUsedCount:              detailedStats.ToolsUsedCount,
		ToolBreakdown:               toolBreakdown,
	}

	// Busca informações do perfil ativo para obter o limite de contexto
	profile, err := a.profileManager.GetActive()
	if err == nil && profile != nil && profile.Chat.ContextWindow > 0 {
		contextLimit := profile.Chat.ContextWindow
		percentage, _, err := database.GetContextWindowUsage(conversationID, contextLimit)
		if err == nil {
			result.ContextUsage = percentage
			result.ContextLimit = contextLimit
			result.IsNearLimit = percentage >= 80.0
			result.IsCritical = percentage >= 95.0
		}
	}

	return result, nil
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico
func (a *App) GetTurnTokenStats(conversationID uint, turnID uint) (*TokenStatsResult, error) {
	stats, err := database.GetTurnTokenStats(conversationID, turnID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar estatísticas do turno: %w", err)
	}

	return &TokenStatsResult{
		PromptTokens:     stats.PromptTokens,
		CompletionTokens: stats.CompletionTokens,
		TotalTokens:      stats.TotalTokens,
		MessageCount:     stats.MessageCount,
	}, nil
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes
// Útil para estimar quanto contexto será enviado na próxima requisição
func (a *App) GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error) {
	return database.GetRecentMessagesTokenCount(conversationID, messageLimit)
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto
// Retorna true e a porcentagem se estiver acima do threshold (padrão 80%)
func (a *App) CheckContextWindowThreshold(conversationID uint, threshold float64) (bool, float64, error) {
	if threshold <= 0 {
		threshold = 80.0 // Padrão: 80%
	}

	// Busca informações do perfil ativo para obter o limite de contexto
	profile, err := a.profileManager.GetActive()
	if err != nil {
		return false, 0, fmt.Errorf("erro ao obter perfil ativo: %w", err)
	}

	if profile == nil || profile.Chat.ContextWindow <= 0 {
		return false, 0, fmt.Errorf("limite de contexto não configurado no perfil")
	}

	contextLimit := profile.Chat.ContextWindow
	percentage, totalTokens, err := database.GetContextWindowUsage(conversationID, contextLimit)
	if err != nil {
		return false, 0, fmt.Errorf("erro ao calcular uso do contexto: %w", err)
	}

	log.Printf("[TokenStats] Conversa %d: %d tokens de %d (%0.1f%%)",
		conversationID, totalTokens, contextLimit, percentage)

	return percentage >= threshold, percentage, nil
}

// GetLLMSettings retorna as configurações atuais da API LLM
func (a *App) GetLLMSettings() (*LLMSettings, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar config: %w", err)
	}

	return &LLMSettings{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.APIBaseURL,
	}, nil
}

// TestMCPNativeSupport testa se o modelo configurado no perfil suporta MCP nativo.
// Faz chamada real à API. Deve ser chamado ao configurar perfil pela primeira vez.
// Retorna (suporta, mensagemErro, erro)
func (a *App) TestMCPNativeSupport(profileSlug string) (bool, string, error) {
	// Carregar perfil
	profile, err := a.profileManager.Get(profileSlug)
	if err != nil {
		return false, "", fmt.Errorf("erro ao carregar perfil: %w", err)
	}

	// Obter configurações da API do LLM settings atual
	settings, err := a.GetLLMSettings()
	if err != nil {
		return false, "", fmt.Errorf("erro ao obter configurações: %w", err)
	}

	// Fazer teste
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	supported, errMsg, err := profiles.TestMCPNativeSupport(
		ctx,
		settings.APIKey,
		settings.BaseURL,
		profile.Chat.Model,
	)

	if err != nil {
		return false, errMsg, err
	}

	// Salvar resultado no perfil
	profile.SetMCPNativeSupport(supported)
	if err := a.profileManager.Update(profileSlug, profile); err != nil {
		return supported, "", fmt.Errorf("erro ao salvar perfil: %w", err)
	}

	return supported, "", nil
}

// ClearMCPTest limpa resultado do teste MCP de um perfil para forçar re-teste.
func (a *App) ClearMCPTest(profileSlug string) error {
	profile, err := a.profileManager.Get(profileSlug)
	if err != nil {
		return fmt.Errorf("erro ao carregar perfil: %w", err)
	}

	profile.ClearMCPTest()

	if err := a.profileManager.Update(profileSlug, profile); err != nil {
		return fmt.Errorf("erro ao salvar perfil: %w", err)
	}

	return nil
}

// SetMCPWorkspaceRoots configura os diretórios raiz do workspace para servidores MCP.
func (a *App) SetMCPWorkspaceRoots(roots []mcpmgr.Root) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SetWorkspaceRoots(roots)
}

// GetMCPWorkspaceRoots retorna os workspace roots configurados.
func (a *App) GetMCPWorkspaceRoots() []mcpmgr.Root {
	if a.mcpMgr == nil {
		return []mcpmgr.Root{}
	}
	return a.mcpMgr.GetWorkspaceRoots()
}

// SubscribeToMCPResource inscreve para receber notificações de um resource.
func (a *App) SubscribeToMCPResource(slug, uri string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SubscribeToResource(slug, uri)
}

// UnsubscribeFromMCPResource cancela inscrição de um resource.
func (a *App) UnsubscribeFromMCPResource(slug, uri string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.UnsubscribeFromResource(slug, uri)
}

// SaveMCPServerAuth salva credenciais de autenticação para um servidor MCP.
// As credenciais são armazenadas de forma segura no credential manager,
// usando o hostname da URL do servidor como padrão de resolução.
func (a *App) SaveMCPServerAuth(slug, authType, token, username, password, clientSecret string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	if a.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	return a.mcpMgr.SaveServerAuth(slug, authType, token, username, password, clientSecret)
}

// DeleteMCPServerAuth remove credenciais de autenticação de um servidor MCP.
func (a *App) DeleteMCPServerAuth(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.DeleteServerAuth(slug)
}

// GetMCPServerAuthInfo retorna informações sobre a autenticação de um servidor MCP
// (tipo e se existe, sem expor valores sensíveis).
func (a *App) GetMCPServerAuthInfo(slug string) (map[string]any, error) {
	if a.mcpMgr == nil {
		return nil, fmt.Errorf("MCP manager não inicializado")
	}
	authType, hasAuth, err := a.mcpMgr.GetServerAuthInfo(slug)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"hasAuth":  hasAuth,
		"authType": authType,
	}, nil
}

// DiscoverMCPServerAuth consulta os endpoints well-known de um servidor MCP
// para auto-discovery de configuração OAuth.
func (a *App) DiscoverMCPServerAuth(serverURL string) mcpmgr.OAuthDiscoveryResult {
	return mcpmgr.DiscoverOAuth(serverURL)
}

// initGlobalHotkeys inicializa o gerenciador de hotkeys
func (a *App) initGlobalHotkeys() {
	if !hotkey.IsSupported() {
		log.Println("[Hotkey] Hotkeys globais não suportados neste sistema")
		return
	}

	a.hotkeyManager = hotkey.GetManager()
	log.Println("[Hotkey] Manager inicializado. Hotkeys serão registrados pelos triggers dos perfis.")
}

// registerActiveProfileHotkeys registra os hotkeys do perfil ativo
func (a *App) registerActiveProfileHotkeys() {
	if a.hotkeyManager == nil {
		return
	}

	activeProfile, err := a.profileManager.GetActive()
	if err != nil {
		log.Printf("[Hotkey] Erro ao obter perfil ativo: %v", err)
		return
	}

	// Remove todos os hotkeys anteriores
	a.hotkeyManager.UnregisterAllProfileHotkeys()

	if activeProfile == nil || len(activeProfile.Interaction.Triggers) == 0 {
		return
	}

	hotkeyCount := 0
	for _, trigger := range activeProfile.Interaction.Triggers {
		if !trigger.Enabled || trigger.Hotkey == "" {
			continue
		}
		hotkeyCount++

		t := trigger // Captura variável para closure

		log.Printf("[Hotkey] Registrando hotkey '%s' para trigger tipo %s...", t.Hotkey, t.Type)
		_, err := a.hotkeyManager.RegisterProfileHotkey(
			1, // Profile ID fixo (perfil global)
			t.Hotkey,
			t.Type == profiles.TriggerTypeHotkey,
			t.HotkeyBringToFront,
			func() {
				// Throttle: ignora se disparou recentemente
				now := time.Now()
				triggerKey := uint(hotkeyCount) // Usa index como key
				if lastFired, ok := a.hotkeyLastFired[triggerKey]; ok {
					elapsed := now.Sub(lastFired).Milliseconds()
					if elapsed < a.hotkeyThrottleMs {
						return
					}
				}
				a.hotkeyLastFired[triggerKey] = now

				log.Printf("[Hotkey] HOTKEY ACIONADA! Trigger tipo %s", t.Type)
				runtime.EventsEmit(a.ctx, "interaction:hotkey:triggered", map[string]interface{}{
					"triggerType":  t.Type,
					"bringToFront": t.HotkeyBringToFront,
				})

				if t.HotkeyGlobal && t.HotkeyBringToFront {
					runtime.WindowShow(a.ctx)
				}
			},
		)
		if err != nil {
			log.Printf("[Hotkey] ERRO ao registrar hotkey '%s': %v", t.Hotkey, err)
		} else {
			log.Printf("[Hotkey] Hotkey '%s' registrada com sucesso", t.Hotkey)
		}
	}

	log.Printf("[Hotkey] Total: %d hotkeys registradas para perfil ativo", hotkeyCount)
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
// DEPRECATED: Use InitSpeechManagerFromProfile() que usa providers do perfil
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

	a.speechManager = speech.NewSpeechManager(config, a.credMgr)
	log.Printf("Speech Manager inicializado (STT: whisper, TTS: openai)")
	return nil
}

// InitSpeechManagerFromProfile inicializa o gerenciador de speech usando providers do perfil ativo
// Permite TTS e STT usarem providers diferentes do LLM (ex: Claude para chat, OpenAI para vozes)
func (a *App) InitSpeechManagerFromProfile() error {
	// Carregar perfil ativo
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		return fmt.Errorf("perfil ativo não encontrado: %w", err)
	}

	// Provider para TTS (se habilitado e usar OpenAI)
	var ttsProviderConfig *llm.ProviderConfig
	if activeProfile.Voice.Provider == "openai" && activeProfile.Voice.LLMProviderID != "" {
		ttsProviderConfig = a.llmRegistry.Get(activeProfile.Voice.LLMProviderID)
		if ttsProviderConfig == nil {
			log.Printf("[Speech] TTS provider não encontrado: %s, usando fallback", activeProfile.Voice.LLMProviderID)
		}
	}

	// Provider para STT (se usar whisper_api)
	var sttProviderConfig *llm.ProviderConfig
	if activeProfile.Interaction.STTProvider == "whisper_api" && activeProfile.Interaction.LLMProviderID != "" {
		sttProviderConfig = a.llmRegistry.Get(activeProfile.Interaction.LLMProviderID)
		if sttProviderConfig == nil {
			log.Printf("[Speech] STT provider não encontrado: %s, usando fallback", activeProfile.Interaction.LLMProviderID)
		}
	}

	// Usar provider de TTS se disponível, senão fallback para config global (migração)
	apiKey := ""
	apiBaseURL := ""
	if ttsProviderConfig != nil {
		apiBaseURL = ttsProviderConfig.BaseURL
		// Credentials serão resolvidas automaticamente pelo httpclient via credMgr
	} else if sttProviderConfig != nil {
		apiBaseURL = sttProviderConfig.BaseURL
	} else {
		// Fallback: carregar da config global (compatibilidade)
		cfg, _ := config.Load()
		if cfg != nil {
			apiKey = cfg.APIKey
			apiBaseURL = cfg.APIBaseURL
		}
	}

	// Configurar speech manager
	config := speech.SpeechConfig{
		STTProvider:      speech.STTProvider(activeProfile.Interaction.STTProvider),
		TTSProvider:      speech.TTSProvider(activeProfile.Voice.Provider),
		OpenAIAPIKey:     apiKey, // Usado apenas em fallback legacy
		OpenAIAPIBaseURL: apiBaseURL,
		WhisperModel:     "whisper-1",
		WhisperLanguage:  activeProfile.Interaction.Language,
		TTSModel:         "tts-1",
		TTSVoice:         activeProfile.Voice.VoiceID,
	}

	a.speechManager = speech.NewSpeechManager(config, a.credMgr)

	ttsInfo := "disabled"
	if ttsProviderConfig != nil {
		ttsInfo = fmt.Sprintf("%s (%s)", activeProfile.Voice.Provider, ttsProviderConfig.Name)
	} else if activeProfile.Voice.Provider != "disabled" {
		ttsInfo = activeProfile.Voice.Provider
	}

	sttInfo := activeProfile.Interaction.STTProvider
	if sttProviderConfig != nil {
		sttInfo = fmt.Sprintf("%s (%s)", sttInfo, sttProviderConfig.Name)
	}

	log.Printf("[Speech] Manager inicializado | TTS: %s | STT: %s", ttsInfo, sttInfo)
	return nil
}

// ensureSpeechManager garante que o speechManager está inicializado.
// Tenta inicializar a partir do perfil ativo.
// Retorna true se disponível, false caso contrário.
func (a *App) ensureSpeechManager() bool {
	if a.speechManager != nil {
		return true
	}

	// Tentar inicializar do perfil ativo
	if err := a.InitSpeechManagerFromProfile(); err != nil {
		log.Printf("[Speech] Erro ao inicializar speechManager do perfil: %v", err)
		return false
	}

	return a.speechManager != nil
}

func maskIdentifier(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	visible := value[len(value)-4:]
	return strings.Repeat("*", len(value)-4) + visible
}

func maskCredentialValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}

func summarizeAuth(auth *credentials.AuthConfig) string {
	if auth == nil {
		return ""
	}
	switch auth.Type {
	case "bearer", "oauth2", "secret":
		if credentials.IsExternalRef(auth.Token) {
			return auth.Token
		}
		return maskCredentialValue(auth.Token)
	case "basic":
		if auth.Username == "" && auth.Password == "" {
			return ""
		}
		pwd := maskCredentialValue(auth.Password)
		if credentials.IsExternalRef(auth.Password) {
			pwd = auth.Password
		}
		return fmt.Sprintf("%s:%s", auth.Username, pwd)
	case "custom":
		if len(auth.Headers) == 0 {
			return ""
		}
		keys := make([]string, 0, len(auth.Headers))
		for k := range auth.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		first := keys[0]
		val := maskCredentialValue(auth.Headers[first])
		if credentials.IsExternalRef(auth.Headers[first]) {
			val = auth.Headers[first]
		}
		return fmt.Sprintf("%s: %s", first, val)
	default:
		return ""
	}
}

func resolveSecretFromAuth(auth *credentials.AuthConfig) string {
	if auth == nil {
		return ""
	}
	if auth.Token != "" {
		return auth.Token
	}
	if auth.Password != "" {
		return auth.Password
	}
	if len(auth.Headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(auth.Headers))
	for k := range auth.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return auth.Headers[keys[0]]
}

func (a *App) resolveCredentialRef(ref string) string {
	if ref == "" || a.credMgr == nil {
		return ""
	}
	auth, err := a.credMgr.GetByPattern(ref)
	if err != nil {
		log.Printf("[Credentials] Erro ao resolver referência %s: %v", ref, err)
		return ""
	}
	return resolveSecretFromAuth(auth)
}

// TranscribeWhisper transcreve áudio usando OpenAI Whisper
func (a *App) TranscribeWhisper(audioBase64 string, filename string) (*TranscriptionResultInfo, error) {
	if !a.ensureSpeechManager() {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
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
	if !a.ensureSpeechManager() {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
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
	if !a.ensureSpeechManager() {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
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

// TTSStreamEvent evento de streaming de TTS
type TTSStreamEvent struct {
	SessionID   string `json:"sessionId"`
	ChunkBase64 string `json:"chunkBase64"`
	Format      string `json:"format"`
	Done        bool   `json:"done"`
	Error       string `json:"error"`
}

// SynthesizeOpenAIStream sintetiza texto usando OpenAI TTS com streaming
func (a *App) SynthesizeOpenAIStream(text string, voice string, sessionID string) error {
	if !a.ensureSpeechManager() {
		runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
			SessionID: sessionID,
			Error:     "speech manager não disponível - configure um provedor no perfil",
		})
		return fmt.Errorf("speech manager não disponível")
	}

	if !a.speechManager.SupportsStreaming() {
		go func() {
			result, err := a.speechManager.SynthesizeWithVoice(text, voice)
			if err != nil {
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
				return
			}

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

	go func() {
		runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
			SessionID: sessionID,
			Format:    "mp3",
		})

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

// ============================================================================
// Unified Profile API (arquivo JSON via configdir)
// ============================================================================

// GetProfiles retorna todos os perfis disponíveis
func (a *App) GetProfiles() ([]profiles.ProfileInfo, error) {
	return a.profileManager.List()
}

// GetProfile retorna um perfil pelo slug
func (a *App) GetProfile(slug string) (*profiles.Profile, error) {
	return a.profileManager.Get(slug)
}

// GetActiveProfile retorna o perfil ativo global
func (a *App) GetActiveProfile() (*profiles.Profile, error) {
	return a.profileManager.GetActive()
}

// GetActiveProfileSlug retorna o slug do perfil ativo
func (a *App) GetActiveProfileSlug() string {
	return a.profileManager.GetActiveSlug()
}

// SetActiveProfile define o perfil ativo e re-registra hotkeys
func (a *App) SetActiveProfile(slug string) error {
	if err := a.profileManager.SetActive(slug); err != nil {
		return err
	}

	// Recarrega o cliente LLM para usar o provedor do novo perfil ativo
	a.initLLMClient()

	// Recarrega o speech manager com os providers do novo perfil (TTS/STT podem ser independentes do LLM)
	if err := a.InitSpeechManagerFromProfile(); err != nil {
		log.Printf("[Profile] Erro ao inicializar speech manager para perfil %s: %v", slug, err)
	}

	// Re-registra hotkeys do novo perfil
	a.registerActiveProfileHotkeys()

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "profile:changed", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// CreateProfile cria um novo perfil
func (a *App) CreateProfile(profile profiles.Profile) (string, error) {
	slug, err := a.profileManager.Create(&profile)
	if err != nil {
		return "", err
	}

	runtime.EventsEmit(a.ctx, "profile:created", map[string]interface{}{
		"slug": slug,
		"name": profile.Name,
	})

	return slug, nil
}

// DuplicateProfile cria uma copia de um perfil existente.
func (a *App) DuplicateProfile(slug string) (string, error) {
	newSlug, err := a.profileManager.Duplicate(slug)
	if err != nil {
		return "", err
	}

	profile, err := a.profileManager.Get(newSlug)
	if err == nil && profile != nil {
		runtime.EventsEmit(a.ctx, "profile:created", map[string]interface{}{
			"slug": newSlug,
			"name": profile.Name,
		})
	}

	return newSlug, nil
}

// UpdateProfile atualiza um perfil existente
func (a *App) UpdateProfile(slug string, profile profiles.Profile) error {
	if err := a.profileManager.Update(slug, &profile); err != nil {
		return err
	}

	// Se for o perfil ativo, re-registra hotkeys
	if slug == a.profileManager.GetActiveSlug() {
		a.registerActiveProfileHotkeys()
	}

	runtime.EventsEmit(a.ctx, "profile:updated", map[string]interface{}{
		"slug": slug,
		"name": profile.Name,
	})

	return nil
}

// DeleteProfile deleta um perfil
func (a *App) DeleteProfile(slug string) error {
	// Não permite deletar o perfil ativo
	if slug == a.profileManager.GetActiveSlug() {
		return fmt.Errorf("não é possível deletar o perfil ativo")
	}

	if err := a.profileManager.Delete(slug); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "profile:deleted", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// GetProfileSearchPaths retorna os caminhos de busca dos perfis
func (a *App) GetProfileSearchPaths() []string {
	return a.profileManager.GetSearchPaths()
}

// ============================================================================
// LLM Provider API
// ============================================================================

// GetLLMProviders retorna todos os provedores LLM disponíveis
func (a *App) GetLLMProviders() []*llm.ProviderConfig {
	if a.llmRegistry == nil {
		return []*llm.ProviderConfig{}
	}
	return a.llmRegistry.List()
}

// GetLLMProvider retorna um provedor LLM pelo ID
func (a *App) GetLLMProvider(id string) *llm.ProviderConfig {
	if a.llmRegistry == nil {
		return nil
	}
	return a.llmRegistry.Get(id)
}

// GetActiveProviderInfo retorna informações sobre o provedor LLM ativo
// (baseado no perfil ativo)
func (a *App) GetActiveProviderInfo() map[string]interface{} {
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		return map[string]interface{}{
			"error": "perfil ativo não encontrado",
		}
	}

	provider := a.llmRegistry.Get(activeProfile.Chat.LLMProvider)
	if provider == nil {
		return map[string]interface{}{
			"error":      "provedor não encontrado",
			"providerID": activeProfile.Chat.LLMProvider,
		}
	}

	return map[string]interface{}{
		"id":       provider.ID,
		"name":     provider.Name,
		"type":     provider.Type,
		"base_url": provider.BaseURL,
		"model":    provider.Model,
	}
}

// extractDomainPattern extrai o pattern de domínio de uma base URL
func extractHostname(baseURL string) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("base_url vazio")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("base_url inválido: %w", err)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return "", fmt.Errorf("host não encontrado no base_url")
	}

	return host, nil
}

// TestLLMProvider testa a conexão com um provider LLM.
// Quando provider_id é informado e api_key está vazio, busca a credencial existente
// no credential manager (resolve o problema de testes falharem durante edição).
func (a *App) TestLLMProvider(req TestLLMProviderRequest) (bool, error) {
	if req.BaseURL == "" {
		return false, fmt.Errorf("base_url é obrigatório")
	}

	parsedURL, err := url.Parse(req.BaseURL)
	if err != nil {
		return false, fmt.Errorf("URL inválida: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false, fmt.Errorf("URL deve começar com http:// ou https://")
	}

	if parsedURL.Host == "" {
		return false, fmt.Errorf("URL deve conter um endereço de servidor válido")
	}

	apiKey := req.APIKey

	// Se não tem API key mas tem provider_id, busca credencial existente no credManager
	if apiKey == "" && req.ProviderID != "" && a.llmRegistry != nil && a.credMgr != nil {
		if provider := a.llmRegistry.Get(req.ProviderID); provider != nil && provider.CredentialPattern != "" {
			if auth, err := a.credMgr.GetByPattern(provider.CredentialPattern); err == nil && auth.Token != "" {
				apiKey = auth.Token
				log.Printf("[TestLLMProvider] Usando credencial existente para provider '%s' (pattern: %s)", req.ProviderID, provider.CredentialPattern)
			}
		}
	}

	modelsEndpoint := strings.TrimSuffix(req.BaseURL, "/") + "/models"

	client := &http.Client{Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()

	httpReq, err := http.NewRequestWithContext(a.ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return false, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	if apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("erro ao conectar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return false, fmt.Errorf("servidor retornou erro: %d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return false, fmt.Errorf("API Key inválida ou não autorizada")
	}

	if resp.StatusCode == http.StatusForbidden {
		return false, fmt.Errorf("acesso negado (403). A API Key pode não ter permissões suficientes")
	}

	return true, nil
}

// CreateLLMProvider cria um novo provider com auto-salvamento de credenciais
func (a *App) CreateLLMProvider(req CreateLLMProviderRequest) (map[string]interface{}, error) {
	// Validação
	if req.ID == "" || req.Name == "" || req.BaseURL == "" {
		return nil, fmt.Errorf("campos obrigatórios faltando (id, name, base_url)")
	}

	// Verificar se já existe
	if a.llmRegistry.Get(req.ID) != nil {
		return nil, fmt.Errorf("provider com ID '%s' já existe", req.ID)
	}

	// Extrair hostname exato do base_url
	hostname, err := extractHostname(req.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao extrair hostname: %w", err)
	}

	// Salvar API key se fornecida
	credConfigured := false
	if req.APIKey != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: req.APIKey,
		}
		err = a.credMgr.RegisterPatternWithContext(a.ctx, hostname, authCfg)
		if err != nil {
			return nil, fmt.Errorf("erro ao salvar credencial: %w", err)
		}
		credConfigured = true
	}

	// Timeout default
	timeout := 180

	// Criar provider config
	provider := &llm.ProviderConfig{
		ID:                req.ID,
		Name:              req.Name,
		Type:              llm.ProviderType(req.Type),
		BaseURL:           req.BaseURL,
		Model:             "", // Modelo não é definido ao criar provider
		Timeout:           timeout,
		CredentialPattern: hostname, // Armazena hostname exato
	}

	// Registrar no registry
	err = a.llmRegistry.Register(provider)
	if err != nil {
		return nil, fmt.Errorf("erro ao registrar provider: %w", err)
	}

	// Salvar provedores em disco
	if err := a.saveLLMProviders(); err != nil {
		log.Printf("[ProviderManager] Erro ao salvar provedores: %v", err)
	}

	log.Printf("[ProviderManager] Provider '%s' criado com hostname '%s'", req.ID, hostname)

	return map[string]interface{}{
		"id":                    provider.ID,
		"name":                  provider.Name,
		"type":                  string(provider.Type),
		"base_url":              provider.BaseURL,
		"model":                 provider.Model,
		"timeout":               provider.Timeout,
		"credential_pattern":    hostname,
		"credential_configured": credConfigured,
	}, nil
}

// UpdateLLMProvider atualiza um provider existente
func (a *App) UpdateLLMProvider(id string, req UpdateLLMProviderRequest) (map[string]interface{}, error) {
	// Buscar provider existente
	existing := a.llmRegistry.Get(id)
	if existing == nil {
		return nil, fmt.Errorf("provider '%s' não encontrado", id)
	}

	// Atualizar campos fornecidos
	updated := &llm.ProviderConfig{
		ID:                existing.ID,
		Name:              existing.Name,
		Type:              existing.Type,
		BaseURL:           existing.BaseURL,
		Model:             existing.Model,
		Timeout:           existing.Timeout,
		CredentialPattern: existing.CredentialPattern,
	}

	if req.Name != "" {
		updated.Name = req.Name
	}
	if req.Type != "" {
		updated.Type = llm.ProviderType(req.Type)
	}
	if req.BaseURL != "" {
		updated.BaseURL = req.BaseURL
		// Re-extrair hostname se base_url mudou
		hostname, err := extractHostname(req.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("erro ao extrair hostname: %w", err)
		}
		updated.CredentialPattern = hostname
		log.Printf("[UpdateLLMProvider] Base URL mudou, novo hostname: '%s'", hostname)
	}

	// Atualizar credencial se fornecida
	credConfigured := false
	if req.APIKey != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: req.APIKey,
		}
		err := a.credMgr.RegisterPatternWithContext(a.ctx, updated.CredentialPattern, authCfg)
		if err != nil {
			return nil, fmt.Errorf("erro ao atualizar credencial: %w", err)
		}
		credConfigured = true
	} else {
		// Verificar se credencial já existe (GetByPattern retorna nil,nil se não encontrada)
		auth, err := a.credMgr.GetByPattern(updated.CredentialPattern)
		credConfigured = (err == nil && auth != nil)
	}

	// Remover provider antigo e registrar atualizado
	a.llmRegistry.Remove(id)
	err := a.llmRegistry.Register(updated)
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar provider: %w", err)
	}

	// Salvar provedores em disco
	if err := a.saveLLMProviders(); err != nil {
		log.Printf("[ProviderManager] Erro ao salvar provedores: %v", err)
	}

	log.Printf("[ProviderManager] Provider '%s' atualizado", id)

	return map[string]interface{}{
		"id":                    updated.ID,
		"name":                  updated.Name,
		"type":                  string(updated.Type),
		"base_url":              updated.BaseURL,
		"model":                 updated.Model,
		"timeout":               updated.Timeout,
		"credential_pattern":    updated.CredentialPattern,
		"credential_configured": credConfigured,
	}, nil
}

// DeleteLLMProvider remove um provider do registry
func (a *App) DeleteLLMProvider(ctx context.Context, id string) error {
	provider := a.llmRegistry.Get(id)
	if provider == nil {
		return fmt.Errorf("provider '%s' não encontrado", id)
	}

	// Remover do registry
	err := a.llmRegistry.Remove(id)
	if err != nil {
		return fmt.Errorf("erro ao remover provider: %w", err)
	}

	// Nota: Não removemos a credencial do credentials.Manager pois pode ser usada por outros providers
	// Se quiser remover, adicionar: a.credMgr.DeletePattern(provider.CredentialPattern)

	log.Printf("[ProviderManager] Provider '%s' removido", id)
	return nil
}

// GetLLMProvidersWithStatus retorna todos os providers com status de credencial
func (a *App) GetLLMProvidersWithStatus() []map[string]interface{} {
	providers := a.GetLLMProviders()
	result := make([]map[string]interface{}, 0, len(providers))

	for _, p := range providers {
		// Verificar se credencial está configurada (GetByPattern retorna nil,nil se não encontrada)
		credConfigured := false
		if p.CredentialPattern != "" {
			auth, err := a.credMgr.GetByPattern(p.CredentialPattern)
			credConfigured = (err == nil && auth != nil)
		}

		result = append(result, map[string]interface{}{
			"id":                    p.ID,
			"name":                  p.Name,
			"type":                  string(p.Type),
			"base_url":              p.BaseURL,
			"model":                 p.Model,
			"timeout":               p.Timeout,
			"credential_pattern":    p.CredentialPattern,
			"credential_configured": credConfigured,
		})
	}

	return result
}

// PreviewVoiceSettings reproduz um texto de teste com configurações ad-hoc
func (a *App) PreviewVoiceSettings(provider, voiceID string, rate, pitch, volume float64, sampleText string) error {
	if sampleText == "" {
		sampleText = "Este é um teste das configurações de voz"
	}

	log.Printf("[PreviewVoiceSettings] provider=%s, voiceID=%s, rate=%.2f", provider, voiceID, rate)

	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("erro ao carregar config: %w", err)
		}
		if cfg.APIKey == "" {
			return fmt.Errorf("API key não configurada")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voiceID, "tts-1")
	}

	if provider == "openai" {
		a.speechManager.SetTTSVoice(voiceID)
	}

	result, err := a.speechManager.SynthesizeWithVoice(sampleText, voiceID)
	if err != nil {
		return fmt.Errorf("erro ao sintetizar: %w", err)
	}

	runtime.EventsEmit(a.ctx, "voice_profile:preview", map[string]interface{}{
		"audio_base64": result.AudioBase64,
		"format":       result.Format,
	})

	return nil
}

// ==================== Auto Update ====================

// initUpdater inicializa o gerenciador de atualizações
func (a *App) initUpdater() {
	// AppVersion é injetada via ldflags durante o build
	// Em dev, permanece como "dev"
	a.updater = updater.New(AppVersion, a.credMgr)

	// Configura callback de progresso
	a.updater.SetProgressCallback(func(bytesDownloaded, totalBytes int64, phase string) {
		if a.ctx == nil {
			return
		}

		var percentage float64
		if totalBytes > 0 {
			percentage = float64(bytesDownloaded) / float64(totalBytes) * 100
		}

		runtime.EventsEmit(a.ctx, "update:progress", map[string]interface{}{
			"phase":           phase,
			"bytesDownloaded": bytesDownloaded,
			"totalBytes":      totalBytes,
			"percentage":      percentage,
		})
	})

	// Configura callback de elevação (solicita permissão ao usuário)
	a.updater.SetElevationCallback(func() bool {
		if a.questionnaireMgr == nil {
			log.Printf("[Updater] Questionnaire manager não disponível para solicitar elevação")
			return false
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       "Permissão Necessária",
			Description: "Para atualizar o aplicativo, precisamos de permissões de administrador para substituir o arquivo executável.\n\nDeseja permitir?",
			Questions: []questionnaire.Question{
				{
					ID:       "allow",
					Type:     "boolean",
					Prompt:   "Permitir atualização com privilégios de administrador?",
					Required: true,
					Default:  true,
				},
			},
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Cancelar",
		})

		if err != nil {
			log.Printf("[Updater] Erro ao solicitar confirmação de elevação: %v", err)
			return false
		}

		if resp.Cancelled {
			log.Printf("[Updater] Usuário cancelou a solicitação de elevação")
			return false
		}

		if allow, ok := resp.Answers["allow"].(bool); ok && allow {
			log.Printf("[Updater] Usuário autorizou elevação")
			return true
		}

		return false
	})

	log.Printf("[Updater] Inicializado (versão atual: %s)", AppVersion)
}

// checkForUpdatesOnStartup verifica atualizações ao iniciar (não bloqueante)
func (a *App) checkForUpdatesOnStartup() {
	// Pula verificação de updates em modo desenvolvimento
	if AppVersion == "dev" {
		log.Printf("[Updater] Modo desenvolvimento detectado (AppVersion=%s): pulando verificação de updates", AppVersion)
		return
	}

	// Aguarda 5 segundos após startup para não interferir com inicialização
	time.Sleep(5 * time.Second)

	// Só verifica atualizações se LLM estiver configurado
	cfg, err := config.Load()
	if err != nil || cfg.APIKey == "" || cfg.APIBaseURL == "" {
		log.Printf("[Updater] Pulando verificação de atualizações: LLM não configurado")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := a.updater.CheckForUpdates(ctx)
	if err != nil {
		log.Printf("[Updater] Erro ao verificar atualizações: %v", err)
		return
	}

	if !info.Available {
		log.Printf("[Updater] Aplicativo está atualizado (v%s)", info.CurrentVersion)
		return
	}

	log.Printf("[Updater] Nova versão disponível: v%s -> v%s", info.CurrentVersion, info.LatestVersion)

	// Pergunta ao usuário se deseja atualizar usando o sistema de questionário
	go a.promptForUpdate(info)
}

// promptForUpdate pergunta ao usuário se deseja atualizar
func (a *App) promptForUpdate(info *updater.UpdateInfo) {
	if a.questionnaireMgr == nil {
		log.Printf("[Updater] Questionnaire manager não disponível")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	description := fmt.Sprintf("Versão atual: %s\nNova versão: %s", info.CurrentVersion, info.LatestVersion)
	if info.ReleaseNotes != "" {
		description += "\n\nNotas da versão:\n" + info.ReleaseNotes
	}
	if info.DownloadSize > 0 {
		sizeMB := float64(info.DownloadSize) / (1024 * 1024)
		description += fmt.Sprintf("\n\nTamanho do download: %.2f MB", sizeMB)
	}

	resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       "Atualização Disponível",
		Description: description,
		Questions: []questionnaire.Question{
			{
				ID:       "confirm",
				Type:     "boolean",
				Prompt:   "Deseja atualizar agora?",
				Required: true,
				Default:  true,
			},
		},
		AllowCancel: true,
		SubmitLabel: "Atualizar",
		CancelLabel: "Mais Tarde",
	})

	if err != nil {
		log.Printf("[Updater] Erro ao solicitar confirmação: %v", err)
		return
	}

	if resp.Cancelled {
		log.Printf("[Updater] Usuário cancelou a atualização")
		return
	}

	if confirm, ok := resp.Answers["confirm"].(bool); ok && confirm {
		// Navega para página de atualização
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "navigate:update", nil)
		}
		go a.applyUpdateWithProgress()
	}
}

// CheckForUpdates verifica manualmente se há atualizações disponíveis (chamado pelo frontend)
func (a *App) CheckForUpdates() (*updater.UpdateInfo, error) {
	if a.updater == nil {
		return nil, fmt.Errorf("updater não inicializado")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return a.updater.CheckForUpdates(ctx)
}

// ApplyUpdate aplica a atualização (chamado pelo frontend)
func (a *App) ApplyUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater não inicializado")
	}

	go a.applyUpdateWithProgress()
	return nil
}

// StartUpdate inicia o processo de atualização (navega para página e inicia)
func (a *App) StartUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater não inicializado")
	}

	// Emite evento para navegar para página de atualização
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "navigate:update", nil)
	}

	// Aguarda um pouco para garantir que a navegação ocorreu
	time.Sleep(500 * time.Millisecond)

	go a.applyUpdateWithProgress()
	return nil
}

// applyUpdateWithProgress aplica a atualização com feedback de progresso
func (a *App) applyUpdateWithProgress() {
	// Emite evento de início
	runtime.EventsEmit(a.ctx, "update:started", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Printf("[Updater] Iniciando download e aplicação da atualização...")

	err := a.updater.ApplyUpdate(ctx)
	if err != nil {
		log.Printf("[Updater] Erro ao aplicar atualização: %v", err)
		runtime.EventsEmit(a.ctx, "update:error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	log.Printf("[Updater] Atualização aplicada com sucesso. Reinicie o aplicativo.")
	runtime.EventsEmit(a.ctx, "update:completed", map[string]interface{}{
		"message": "Atualização instalada com sucesso! Feche e reabra o aplicativo para aplicar as mudanças.",
	})
}

// GetAppVersion retorna a versão atual do aplicativo
func (a *App) GetAppVersion() string {
	return AppVersion
}

// ==================== Welcome Wizard ====================

// wizardValidationResult contém o resultado da validação de conexão do wizard
type wizardValidationResult struct {
	URLReachable    bool
	AuthOK          bool
	ModelsAvailable bool
	Models          []string
	ErrorType       string // "url_invalid", "url_unreachable", "auth_invalid", "auth_required", "server_error"
	ErrorDetail     string
}

// validateWizardConnection testa a URL, autenticação e lista de modelos de um provedor.
// Faz um GET direto ao endpoint /models para validação completa em uma única requisição.
func (a *App) validateWizardConnection(baseURL, apiKey string) wizardValidationResult {
	result := wizardValidationResult{}

	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		result.ErrorType = "url_invalid"
		result.ErrorDetail = "URL inválida. Deve começar com http:// ou https:// e conter um endereço válido."
		return result
	}

	modelsEndpoint := strings.TrimSuffix(baseURL, "/") + "/models"

	client := &http.Client{Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()

	httpReq, err := http.NewRequestWithContext(a.ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		result.ErrorType = "url_unreachable"
		result.ErrorDetail = "Não foi possível preparar a requisição de teste."
		return result
	}

	if apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		result.ErrorType = "url_unreachable"
		result.ErrorDetail = fmt.Sprintf("Não foi possível conectar ao servidor. Verifique se a URL está correta e o servidor está ativo.\n\nDetalhes: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.URLReachable = true
	body, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusOK:
		result.AuthOK = true
		var modelsResp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &modelsResp); err == nil && len(modelsResp.Data) > 0 {
			result.ModelsAvailable = true
			for _, m := range modelsResp.Data {
				result.Models = append(result.Models, m.ID)
			}
			sort.Strings(result.Models)
		}

	case resp.StatusCode == http.StatusUnauthorized:
		if apiKey != "" {
			result.ErrorType = "auth_invalid"
			result.ErrorDetail = "A API Key informada foi rejeitada pelo servidor (401 Unauthorized). Verifique se a chave está correta."
		} else {
			result.ErrorType = "auth_required"
			result.ErrorDetail = "Este servidor requer uma API Key para autenticação."
		}

	case resp.StatusCode == http.StatusForbidden:
		result.ErrorType = "auth_invalid"
		result.ErrorDetail = "Acesso negado (403 Forbidden). A API Key pode não ter permissões suficientes."

	case resp.StatusCode == http.StatusNotFound:
		result.AuthOK = true
		result.ModelsAvailable = false

	case resp.StatusCode >= 500:
		result.ErrorType = "server_error"
		result.ErrorDetail = fmt.Sprintf("O servidor retornou erro %d. O servidor pode estar com problemas temporários.", resp.StatusCode)

	default:
		result.AuthOK = true
		result.ModelsAvailable = false
	}

	return result
}

// validateWizardURL valida formato e alcançabilidade básica de uma URL personalizada.
// Aceita qualquer resposta HTTP (inclusive 401) — apenas rejeita se o servidor estiver inacessível.
func (a *App) validateWizardURL(baseURL string) error {
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" {
		return fmt.Errorf("URL inválida. Deve conter um endereço de servidor válido.")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL deve começar com http:// ou https://")
	}

	testURL := strings.TrimSuffix(baseURL, "/") + "/"

	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()

	httpReq, err := http.NewRequestWithContext(a.ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return fmt.Errorf("não foi possível preparar requisição de teste")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("não foi possível conectar ao servidor. Verifique se a URL está correta e o servidor está ativo")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("o servidor retornou erro %d. Pode estar com problemas temporários", resp.StatusCode)
	}

	return nil
}

// NeedsWelcomeWizard verifica se o assistente precisa do wizard de boas-vindas
// Retorna true se não houver chave mestra ou provedor configurado
func (a *App) NeedsWelcomeWizard() bool {
	cfg, err := config.Load()
	if err != nil {
		return true
	}

	// Verifica se tem API key e base URL configurados
	hasConfig := cfg.APIKey != "" && cfg.APIBaseURL != ""

	store := credentials.NewDBStore()
	hasMasterKey, err := store.HasKeyWrap(context.Background(), credentials.KeyWrapKindMaster)
	if err != nil {
		return true
	}

	return !hasConfig || !hasMasterKey
}

// RunWelcomeWizard executa o wizard de boas-vindas
// Retorna true se completou com sucesso, false se cancelado
func (a *App) RunWelcomeWizard() (bool, error) {
	ctx := a.ctx

	// Variáveis para armazenar dados entre etapas
	var provider string
	var baseURL string
	var apiKey string
	var defaultModel string
	var recoveryKey string
	var passwordError string
	var urlError string
	var keyError string
	var validatedModels []string

	store := credentials.NewDBStore()
	masterKeyConfigured, _ := store.HasKeyWrap(ctx, credentials.KeyWrapKindMaster)

	// Controle de navegação entre etapas
	currentStep := 0
	if masterKeyConfigured {
		currentStep = 2
	}

	for currentStep >= 0 {
		switch currentStep {
		case 0: // Etapa 0: Senha mestre
			description := "Defina uma senha mestre para criptografar credenciais locais. Guarde com cuidado."
			if passwordError != "" {
				description = passwordError
			}

			passwordResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Segurança: senha mestre",
				Description: description,
				Questions: []questionnaire.Question{
					{
						ID:          "masterPassword",
						Type:        "password",
						Prompt:      "Senha mestre",
						Required:    true,
						Placeholder: "Digite uma senha forte",
					},
					{
						ID:          "confirmPassword",
						Type:        "password",
						Prompt:      "Confirmar senha mestre",
						Required:    true,
						Placeholder: "Repita a senha",
					},
				},
				AllowCancel: true,
				SubmitLabel: "Continuar",
				CancelLabel: "Cancelar",
			})

			if err != nil || passwordResp.Cancelled {
				return false, err
			}

			masterPassword, _ := passwordResp.Answers["masterPassword"].(string)
			confirmPassword, _ := passwordResp.Answers["confirmPassword"].(string)
			if strings.TrimSpace(masterPassword) == "" || masterPassword != confirmPassword {
				passwordError = "As senhas não conferem. Tente novamente."
				currentStep = 0
				continue
			}

			setupResult, err := credentials.SetupMasterKey(store, masterPassword)
			if err != nil {
				return false, err
			}

			recoveryKey = setupResult.RecoveryKey
			a.configureCredentialManager(setupResult.DEK, true)
			passwordError = ""
			currentStep = 1

		case 1: // Etapa 1: Código de recuperação
			_, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Código de recuperação",
				Description: "Guarde este código em local seguro. Ele permite recuperar suas credenciais se você esquecer a senha mestre.",
				Questions: []questionnaire.Question{
					{
						ID:      "recoveryCode",
						Type:    "readonly_code",
						Prompt:  "Código de recuperação",
						Content: recoveryKey,
					},
					{
						ID:       "confirmed",
						Type:     "boolean",
						Prompt:   "Eu salvei o código de recuperação em local seguro",
						Required: true,
					},
				},
				AllowCancel: false,
				SubmitLabel: "Continuar",
			})
			if err != nil {
				return false, err
			}
			currentStep = 2

		case 2: // Etapa 2: Escolher provedor
			providerResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Bem-vindo ao Assistente!",
				Description: "Vamos configurar seu assistente em alguns passos simples.",
				Questions: []questionnaire.Question{
					{
						ID:       "provider",
						Type:     "single_choice",
						Prompt:   "Qual provedor de IA você deseja usar?",
						Required: true,
						Options: []string{
							"OpenAI",
							"Anthropic (Claude)",
							"Google (Gemini)",
							"DeepSeek",
							"xAI (Grok)",
							"OpenRouter",
							"Mistral AI",
							"Groq",
							"Together AI",
							"Fireworks AI",
							"Perplexity",
							"Azure OpenAI",
							"Ollama (Local)",
							"LiteLLM",
							"Outro (URL personalizada)",
						},
						Default: provider, // Mantém seleção anterior se voltar
					},
				},
				AllowCancel: true,
				SubmitLabel: "Próximo",
				CancelLabel: "Cancelar",
			})

			if err != nil || providerResp.Cancelled {
				return false, err
			}

			provider = providerResp.Answers["provider"].(string)

			// Mapeia provedor para base URL padrão
			switch provider {
			case "OpenAI":
				baseURL = "https://api.openai.com/v1"
			case "Anthropic (Claude)":
				baseURL = "https://api.anthropic.com/v1"
			case "Google (Gemini)":
				baseURL = "https://generativelanguage.googleapis.com/v1beta/openai/"
			case "DeepSeek":
				baseURL = "https://api.deepseek.com/v1"
			case "xAI (Grok)":
				baseURL = "https://api.x.ai/v1"
			case "OpenRouter":
				baseURL = "https://openrouter.ai/api/v1"
			case "Mistral AI":
				baseURL = "https://api.mistral.ai/v1"
			case "Groq":
				baseURL = "https://api.groq.com/openai/v1"
			case "Together AI":
				baseURL = "https://api.together.xyz/v1"
			case "Fireworks AI":
				baseURL = "https://api.fireworks.ai/inference/v1"
			case "Perplexity":
				baseURL = "https://api.perplexity.ai"
			case "Azure OpenAI":
				baseURL = "" // Usuário precisará fornecer
			case "Ollama (Local)":
				baseURL = "http://localhost:11434/v1"
			case "LiteLLM":
				baseURL = "" // Usuário precisará fornecer
			case "Outro (URL personalizada)":
				baseURL = "" // Usuário precisará fornecer
			}

			currentStep = 3

		case 3: // Etapa 3: URL personalizada (se necessário)
			needsCustomURL := provider == "Outro (URL personalizada)" || provider == "Azure OpenAI" || provider == "LiteLLM"

			if !needsCustomURL {
				currentStep = 4
				continue
			}

			placeholderURL := "http://localhost:11434/v1"
			switch provider {
			case "LiteLLM":
				placeholderURL = "http://localhost:4000"
			case "Azure OpenAI":
				placeholderURL = "https://your-resource.openai.azure.com"
			}

			urlDescription := "Informe a URL do servidor OpenAI-compatible."
			if urlError != "" {
				urlDescription = urlError
			}

			urlResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Configuração do Servidor",
				Description: urlDescription,
				Questions: []questionnaire.Question{
					{
						ID:          "baseURL",
						Type:        "text",
						Prompt:      "URL do servidor",
						Required:    true,
						Placeholder: placeholderURL,
						Default:     baseURL,
					},
				},
				AllowCancel: true,
				SubmitLabel: "Próximo",
				CancelLabel: "Voltar",
			})

			if err != nil {
				return false, err
			}

			if urlResp.Cancelled {
				currentStep = 2
				continue
			}

			baseURL = urlResp.Answers["baseURL"].(string)
			urlError = ""

			if err := a.validateWizardURL(baseURL); err != nil {
				urlError = fmt.Sprintf("⚠️ %v\n\nCorreija a URL e tente novamente.", err)
				currentStep = 3
				continue
			}

			currentStep = 4

		case 4: // Etapa 4: API Key + validação de conexão
			keyDescription := "Informe sua chave de API. Deixe em branco se o servidor não requer autenticação."
			if provider == "Ollama (Local)" {
				keyDescription = "Ollama local geralmente não precisa de chave. Você pode deixar em branco."
			}
			if keyError != "" {
				keyDescription = keyError
			}

			keyResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Chave de API",
				Description: keyDescription,
				Questions: []questionnaire.Question{
					{
						ID:          "apiKey",
						Type:        "text",
						Prompt:      "Chave de API (opcional)",
						Required:    false,
						Placeholder: "sk-...",
						Default:     apiKey,
					},
				},
				AllowCancel: true,
				SubmitLabel: "Próximo",
				CancelLabel: "Voltar",
			})

			if err != nil {
				return false, err
			}

			if keyResp.Cancelled {
				keyError = ""
				needsCustomURL := provider == "Outro (URL personalizada)" || provider == "Azure OpenAI" || provider == "LiteLLM"
				if needsCustomURL {
					currentStep = 3
				} else {
					currentStep = 2
				}
				continue
			}

			if keyResp.Answers["apiKey"] != nil {
				apiKey = keyResp.Answers["apiKey"].(string)
			}
			keyError = ""

			log.Printf("[Wizard] Validando conexão: %s (com key: %v)", baseURL, apiKey != "")
			validation := a.validateWizardConnection(baseURL, apiKey)

			needsCustomURL := provider == "Outro (URL personalizada)" || provider == "Azure OpenAI" || provider == "LiteLLM"

			switch validation.ErrorType {
			case "url_invalid", "url_unreachable":
				if needsCustomURL {
					urlError = fmt.Sprintf("⚠️ %s", validation.ErrorDetail)
					currentStep = 3
				} else {
					keyError = fmt.Sprintf("⚠️ Não foi possível conectar ao servidor do %s (%s).\n\n%s\n\nClique \"Próximo\" para tentar novamente ou \"Voltar\" para escolher outro provedor.", provider, baseURL, validation.ErrorDetail)
					currentStep = 4
				}
				continue

			case "auth_required":
				keyError = fmt.Sprintf("⚠️ %s\n\nInforme uma API Key válida para continuar.", validation.ErrorDetail)
				currentStep = 4
				continue

			case "auth_invalid":
				keyError = fmt.Sprintf("⚠️ %s\n\nVerifique sua chave e tente novamente.", validation.ErrorDetail)
				currentStep = 4
				continue

			case "server_error":
				keyError = fmt.Sprintf("⚠️ %s\n\nClique \"Próximo\" para tentar novamente ou \"Voltar\" para alterar configurações.", validation.ErrorDetail)
				currentStep = 4
				continue
			}

			validatedModels = validation.Models
			log.Printf("[Wizard] Conexão validada com sucesso. Modelos disponíveis: %d", len(validatedModels))
			currentStep = 5

		case 5: // Etapa 5: Escolher modelo (conexão já validada)
			if len(validatedModels) > 0 {
				modelDefault := ""
				if defaultModel != "" {
					modelDefault = defaultModel
				} else {
					modelDefault = validatedModels[0]
				}

				modelResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
					Title:       "Escolha o Modelo Padrão",
					Description: fmt.Sprintf("Conexão validada com sucesso! %d modelo(s) disponível(is).\n\nSelecione o modelo padrão. Você pode alterar depois nas configurações.", len(validatedModels)),
					Questions: []questionnaire.Question{
						{
							ID:       "model",
							Type:     "single_choice",
							Prompt:   "Modelo padrão:",
							Required: true,
							Options:  validatedModels,
							Default:  modelDefault,
						},
					},
					AllowCancel: true,
					SubmitLabel: "Finalizar",
					CancelLabel: "Voltar",
				})

				if err != nil {
					return false, err
				}

				if modelResp.Cancelled {
					currentStep = 4
					continue
				}

				defaultModel = modelResp.Answers["model"].(string)
			} else {
				manualResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
					Title:       "Configurar Modelo",
					Description: "Conexão validada! O servidor não suporta listagem automática de modelos.\n\nInforme o nome do modelo que deseja usar.",
					Questions: []questionnaire.Question{
						{
							ID:          "defaultModel",
							Type:        "text",
							Prompt:      "Nome do modelo",
							Required:    true,
							Placeholder: "gpt-4o-mini",
							Default:     defaultModel,
						},
					},
					AllowCancel: true,
					SubmitLabel: "Finalizar",
					CancelLabel: "Voltar",
				})

				if err != nil {
					return false, err
				}

				if manualResp.Cancelled {
					currentStep = 4
					continue
				}

				defaultModel = manualResp.Answers["defaultModel"].(string)
			}

			// Registra credencial temporária para o createWizardProvider
			wizardHostname, _ := extractHostname(baseURL)
			if apiKey != "" && wizardHostname != "" {
				wizardAuth := &credentials.AuthConfig{
					Type:  "bearer",
					Token: apiKey,
				}
				if err := a.credMgr.RegisterPatternWithContext(ctx, wizardHostname, wizardAuth); err != nil {
					log.Printf("[Wizard] Erro ao registrar credencial temporária: %v", err)
				}
			}

			providerID, err := a.createWizardProvider(provider, baseURL, apiKey, defaultModel)
			if err != nil {
				return false, fmt.Errorf("erro ao criar provedor: %w", err)
			}

			if err := a.saveWelcomeConfig(baseURL, apiKey, defaultModel); err != nil {
				return false, err
			}

			if err := a.updateAllProfilesProviderAndModel(providerID, defaultModel); err != nil {
				log.Printf("[Wizard] Aviso: erro ao atualizar perfis: %v", err)
			}

			a.initLLMClient()

			// Verificação final: confirma que o provider funciona com o modelo escolhido
			finalModels, finalErr := a.GetModelsByProvider(providerID)
			if finalErr != nil {
				log.Printf("[Wizard] Verificação final: %v (provider pode não suportar /models)", finalErr)
			} else {
				log.Printf("[Wizard] Verificação final OK: %d modelos via provider '%s'", len(finalModels), providerID)
				modelFound := false
				for _, m := range finalModels {
					if m == defaultModel {
						modelFound = true
						break
					}
				}
				if !modelFound && len(finalModels) > 0 {
					log.Printf("[Wizard] Aviso: modelo '%s' não encontrado na lista do provider (%d modelos)", defaultModel, len(finalModels))
				}
			}

			go a.checkForUpdatesAfterWizard()
			return true, nil
		}
	}

	return false, nil
}

// wizardProviderInfo mapeia a escolha do wizard para configuração do provedor
type wizardProviderInfo struct {
	ID   string
	Name string
	Type llm.ProviderType
}

// getWizardProviderInfo retorna ID, nome e tipo para a escolha do wizard
func getWizardProviderInfo(providerChoice string) wizardProviderInfo {
	switch providerChoice {
	case "OpenAI":
		return wizardProviderInfo{ID: "openai-default", Name: "OpenAI", Type: llm.ProviderOpenAI}
	case "Anthropic (Claude)":
		return wizardProviderInfo{ID: "anthropic-claude", Name: "Claude (Anthropic)", Type: llm.ProviderClaude}
	case "Google (Gemini)":
		return wizardProviderInfo{ID: "google-gemini", Name: "Google (Gemini)", Type: llm.ProviderOpenAI}
	case "OpenRouter":
		return wizardProviderInfo{ID: "openrouter-default", Name: "OpenRouter", Type: llm.ProviderOpenAI}
	case "Mistral AI":
		return wizardProviderInfo{ID: "mistral-default", Name: "Mistral AI", Type: llm.ProviderMistral}
	case "Groq":
		return wizardProviderInfo{ID: "groq-default", Name: "Groq", Type: llm.ProviderGroq}
	case "Together AI":
		return wizardProviderInfo{ID: "together-default", Name: "Together AI", Type: llm.ProviderTogether}
	case "Fireworks AI":
		return wizardProviderInfo{ID: "fireworks-default", Name: "Fireworks AI", Type: llm.ProviderFireworks}
	case "Perplexity":
		return wizardProviderInfo{ID: "perplexity-default", Name: "Perplexity", Type: llm.ProviderPerplexity}
	case "DeepSeek":
		return wizardProviderInfo{ID: "deepseek-default", Name: "DeepSeek", Type: llm.ProviderDeepSeek}
	case "xAI (Grok)":
		return wizardProviderInfo{ID: "xai-grok", Name: "xAI (Grok)", Type: llm.ProviderGrok}
	case "Azure OpenAI":
		return wizardProviderInfo{ID: "azure-openai", Name: "Azure OpenAI", Type: llm.ProviderOpenAI}
	case "Ollama (Local)":
		return wizardProviderInfo{ID: "ollama-local", Name: "Ollama (Local)", Type: llm.ProviderOllama}
	case "LiteLLM":
		return wizardProviderInfo{ID: "litellm", Name: "LiteLLM", Type: llm.ProviderOpenAI}
	default:
		return wizardProviderInfo{ID: "custom", Name: "Custom Provider", Type: llm.ProviderCustom}
	}
}

// createWizardProvider cria o provedor LLM escolhido durante o wizard,
// registrando-o no registry, salvando a credencial e persistindo no SQLite.
func (a *App) createWizardProvider(providerChoice, baseURL, apiKey, model string) (string, error) {
	info := getWizardProviderInfo(providerChoice)

	hostname, err := extractHostname(baseURL)
	if err != nil {
		return "", fmt.Errorf("erro ao extrair hostname de %s: %w", baseURL, err)
	}

	timeout := 180
	if info.Type == llm.ProviderOllama {
		timeout = 300
	}

	provider := &llm.ProviderConfig{
		ID:                info.ID,
		Name:              info.Name,
		Type:              info.Type,
		BaseURL:           baseURL,
		Model:             model,
		Timeout:           timeout,
		CredentialPattern: hostname,
	}

	if err := a.llmRegistry.Register(provider); err != nil {
		return "", fmt.Errorf("erro ao registrar provedor: %w", err)
	}

	if apiKey != "" && hostname != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: apiKey,
		}
		if err := a.credMgr.RegisterPatternWithContext(a.ctx, hostname, authCfg); err != nil {
			return "", fmt.Errorf("erro ao salvar credencial: %w", err)
		}
	}

	if err := a.saveLLMProviders(); err != nil {
		return "", fmt.Errorf("erro ao persistir provedor: %w", err)
	}

	log.Printf("[Wizard] Provedor '%s' (%s) criado com sucesso", info.ID, info.Name)
	return info.ID, nil
}

// saveWelcomeConfig salva a configuração legada do wizard (config.json)
func (a *App) saveWelcomeConfig(baseURL, apiKey, defaultModel string) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	cfg.APIBaseURL = baseURL
	cfg.APIKey = apiKey
	cfg.DefaultModel = defaultModel
	cfg.ChatParams.Model = defaultModel

	return config.Save(cfg)
}

// updateAllProfilesProviderAndModel atualiza o provedor e modelo em todos os perfis
func (a *App) updateAllProfilesProviderAndModel(providerID, model string) error {
	profileList, err := a.profileManager.List()
	if err != nil {
		return err
	}

	for _, profileInfo := range profileList {
		profile, err := a.profileManager.Get(profileInfo.Slug)
		if err != nil {
			log.Printf("[Wizard] Erro ao carregar perfil %s (slug: %s): %v", profileInfo.Name, profileInfo.Slug, err)
			continue
		}

		profile.Chat.LLMProvider = providerID
		profile.Chat.Model = model
		if err := a.profileManager.Update(profileInfo.Slug, profile); err != nil {
			log.Printf("[Wizard] Erro ao salvar perfil %s (slug: %s): %v", profileInfo.Name, profileInfo.Slug, err)
		}
	}

	return nil
}

// checkForUpdatesAfterWizard verifica atualizações após o wizard de configuração
func (a *App) checkForUpdatesAfterWizard() {
	// Aguarda 2 segundos para não interferir com finalização do wizard
	time.Sleep(2 * time.Second)

	if a.updater == nil {
		log.Printf("[Wizard] Updater não inicializado, pulando verificação de atualizações")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("[Wizard] Verificando atualizações disponíveis...")

	info, err := a.updater.CheckForUpdates(ctx)
	if err != nil {
		log.Printf("[Wizard] Erro ao verificar atualizações: %v", err)
		return
	}

	if !info.Available {
		log.Printf("[Wizard] Aplicativo está atualizado (v%s)", info.CurrentVersion)
		return
	}

	log.Printf("[Wizard] Nova versão disponível: v%s -> v%s", info.CurrentVersion, info.LatestVersion)

	// Pergunta ao usuário se deseja atualizar
	go a.promptForUpdate(info)
}

// ==================== Workspace ====================

func (a *App) initWorkspace() {
	homeDir := configdir.GetHomeDir()
	a.workspaceMgr = workspace.NewManager(homeDir)

	workDir := ""
	if wd, err := os.Getwd(); err == nil {
		workDir = wd
	}

	if err := a.workspaceMgr.Initialize(workDir); err != nil {
		log.Printf("Erro ao inicializar workspace: %v", err)
	} else if ws := a.workspaceMgr.Active(); ws != nil {
		log.Printf("Workspace ativo: %s (%s)", ws.Name, ws.ID)
	}
}

// GetActiveWorkspace retorna o workspace ativo.
func (a *App) GetActiveWorkspace() *workspace.Workspace {
	if a.workspaceMgr == nil {
		return nil
	}
	return a.workspaceMgr.Active()
}

// ListWorkspaces retorna todos os workspaces conhecidos.
func (a *App) ListWorkspaces() ([]workspace.WorkspaceInfo, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.List()
}

// CreateWorkspace cria um novo workspace avulso.
func (a *App) CreateWorkspace(name string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.Create(name)
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:created", ws)
	return ws, nil
}

// SwitchWorkspace alterna para outro workspace.
func (a *App) SwitchWorkspace(workspaceID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.Switch(workspaceID)
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:switched", ws)
	return ws, nil
}

// RenameWorkspace renomeia o workspace ativo.
func (a *App) RenameWorkspace(newName string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.Rename(newName); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:renamed", a.workspaceMgr.Active())
	return nil
}

// DeleteWorkspace remove um workspace (não pode ser o ativo).
func (a *App) DeleteWorkspace(workspaceID string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.Delete(workspaceID); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:deleted", workspaceID)
	return nil
}

// SetWorkspaceProfile define o perfil base do workspace ativo.
func (a *App) SetWorkspaceProfile(profileSlug string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.SetProfile(profileSlug)
}

// SaveWorkspace persiste o estado do workspace ativo.
func (a *App) SaveWorkspace() error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.Save()
}

// --- Workspace Tab APIs ---

// AddWorkspaceTab adiciona uma aba ao workspace ativo.
func (a *App) AddWorkspaceTab(tab workspace.Tab) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.AddTab(tab); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_added", ws)
	return ws, nil
}

// RemoveWorkspaceTab remove uma aba do workspace ativo.
func (a *App) RemoveWorkspaceTab(tabID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.RemoveTab(tabID); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_removed", ws)
	return ws, nil
}

// SetActiveWorkspaceTab define a aba ativa no workspace.
func (a *App) SetActiveWorkspaceTab(tabID string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:tab_activated", tabID)
	return nil
}

// UpdateWorkspaceTab atualiza campos de uma aba.
func (a *App) UpdateWorkspaceTab(tabID string, updates map[string]any) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.UpdateTab(tabID, updates)
}

// ReorderWorkspaceTabs reordena as abas do workspace.
func (a *App) ReorderWorkspaceTabs(orderedIDs []string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.ReorderTabs(orderedIDs)
}

// MoveWorkspaceTabTo move uma aba do workspace ativo para outro workspace.
func (a *App) MoveWorkspaceTabTo(tabID, targetWorkspaceID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.MoveTabToWorkspace(tabID, targetWorkspaceID); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_removed", ws)
	return ws, nil
}

// ExportWorkspace exporta o workspace ativo como YAML.
func (a *App) ExportWorkspace() (string, error) {
	if a.workspaceMgr == nil {
		return "", fmt.Errorf("workspace manager not initialized")
	}
	data, err := a.workspaceMgr.ExportWorkspace()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ImportWorkspace importa um workspace a partir de YAML.
func (a *App) ImportWorkspace(yamlData string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.ImportWorkspace([]byte(yamlData))
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:created", ws)
	return ws, nil
}
