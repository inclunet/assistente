package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/channels"
	"assistente/internal/config"
	"assistente/internal/configdir"
	"assistente/internal/contacts"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/messaging/signal"
	"assistente/internal/messaging/telegram"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
	"assistente/internal/skills"
	"assistente/internal/speech"
	"assistente/internal/terminal"
	"assistente/internal/tools"
	"assistente/internal/tools/filesystem"
	msgtool "assistente/internal/tools/messaging"
	questiontool "assistente/internal/tools/questionnaire"
	"assistente/internal/tools/shell"
	"assistente/internal/tools/web"
	"assistente/internal/updater"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	// AppVersion é a versão do aplicativo
	// Será injetada automaticamente pelo Wails a partir de wails.json info.productVersion
	// Em dev, permanece como "dev"
	AppVersion = "dev"
)

// App struct
type App struct {
	ctx                   context.Context
	llmClient             *llm.SyncClient
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

	// Throttle para hotkeys - evita disparo repetido quando segura a tecla
	hotkeyLastFired  map[uint]time.Time
	hotkeyThrottleMs int64 // tempo mínimo entre disparos (em ms)
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
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Inicializa o banco de dados
	if err := InitDatabase(); err != nil {
		log.Printf("Erro ao inicializar banco de dados: %v", err)
	}

	// Instala/atualiza perfis embutidos em ~/.assistente/profiles/
	a.installBuiltinProfiles()

	// Garante que o diretório de perfis existe
	if err := a.profileManager.EnsureDefaults(); err != nil {
		log.Printf("Erro ao garantir diretório de perfis: %v", err)
	}

	// Inicializa o cliente LLM
	a.initLLMClient()

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

	// Inicializa o updater
	a.initUpdater()

	// Verifica atualizações no startup (não bloqueante)
	go a.checkForUpdatesOnStartup()
}

// initLLMClient inicializa o cliente LLM
func (a *App) initLLMClient() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Erro ao carregar config para LLM: %v", err)
		return
	}

	llm.ConfigureResponseTimeout(cfg.GetResponseTimeout())
	log.Printf("HTTP Response Timeout configurado para %d segundos", cfg.GetResponseTimeout())

	if cfg.APIKey == "" {
		log.Printf("API Key não configurada")
		return
	}

	a.llmClient = llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey)
	log.Printf("LLM Client inicializado")
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
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

	a.mcpMgr = mcpmgr.NewManager(a.toolRegistry, emitEvent)

	// Carrega configs e auto-conecta servidores habilitados
	if err := a.mcpMgr.LoadConfigs(); err != nil {
		log.Printf("[MCP] Erro ao carregar configurações: %v", err)
	}

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
		message := fmt.Sprintf("O contato %s enviou uma mensagem via %s, mas nenhum contato está autorizado para este canal.\n\nIdentificador: %s\n\nDeseja definir este contato como o contato autorizado do %s? (Apenas um contato é permitido por canal.)",
			name, channel, identifier, channel)
		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       "Contato não autorizado",
			Description: message,
			AllowCancel: true,
			SubmitLabel: "Autorizar",
			CancelLabel: "Ignorar",
			Questions: []questionnaire.Question{
				{
					ID:       "approve",
					Type:     "boolean",
					Prompt:   fmt.Sprintf("Autorizar este contato no canal %s?", channel),
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
			return false, fmt.Errorf("resposta inválida para autorização de contato")
		}
		return approved, nil
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
	if cfg, ok := enabledChannels["telegram"]; ok && cfg.BotToken != "" {
		adapter := telegram.NewAdapter(cfg.BotToken)
		a.msgGateway.Register("telegram", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar Telegram: %v", err)
			}
		}()
		log.Printf("[Messaging] Telegram habilitado")
	} else {
		log.Printf("[Messaging] Telegram não configurado ou desabilitado")
	}

	// Signal (via signal-cli-rest-api HTTP + WebSocket)
	if cfg, ok := enabledChannels["signal"]; ok && cfg.Account != "" && cfg.APIURL != "" {
		adapter := signal.NewAdapter(cfg.APIURL, cfg.Account)
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

	// Registra a tool send_message no registry de ferramentas
	if a.toolRegistry != nil {
		sendMsgTool := msgtool.NewSendMessageTool(a.msgGateway)
		a.toolRegistry.MustRegister(sendMsgTool)
		log.Printf("[Messaging] Tool 'send_message' registrada")
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

// SaveChannelConfig salva a configuração de um canal e reconecta automaticamente.
func (a *App) SaveChannelConfig(channelName string, cfg *channels.ChannelConfig) error {
	if err := channels.Save(channelName, cfg); err != nil {
		return err
	}
	// Reconecta o canal com a nova configuração
	a.restartChannel(channelName, cfg)
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
		if cfg.BotToken == "" {
			log.Printf("[Messaging] Telegram: bot token vazio, não conectando")
			return
		}
		adapter := telegram.NewAdapter(cfg.BotToken)
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
		adapter := signal.NewAdapter(cfg.APIURL, cfg.Account)
		a.msgGateway.Register("signal", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar Signal: %v", err)
			}
		}()
		log.Printf("[Messaging] Signal reconectado (api=%s, account=%s)", cfg.APIURL, maskIdentifier(cfg.Account))

	default:
		log.Printf("[Messaging] Canal desconhecido: %s", channelName)
	}
}

// GetAllChannelConfigs retorna as configurações de todos os canais.
func (a *App) GetAllChannelConfigs() (map[string]*channels.ChannelConfig, error) {
	return channels.ListAll()
}

// SignalRegister inicia o registro de uma conta Signal via signal-cli-rest-api.
// mode: "sms" (padrão) ou "voice" para receber o código por ligação.
// captcha: token de verificação exigido pelo Signal (signalcaptcha://...).
func (a *App) SignalRegister(apiURL, number, mode, captcha string) error {
	return signal.Register(apiURL, number, mode, captcha)
}

// SignalVerify verifica o código recebido via SMS/ligação.
func (a *App) SignalVerify(apiURL, number, code string) error {
	return signal.Verify(apiURL, number, code)
}

// SignalLink gera o QR code para vincular como dispositivo secundário.
// Retorna a imagem QR code em base64 (PNG).
func (a *App) SignalLink(apiURL, deviceName string) (string, error) {
	return signal.GetLinkQRCode(apiURL, deviceName)
}

// SignalLinkRaw gera a URI texto para vincular como dispositivo secundário.
// Alternativa acessível ao QR code.
func (a *App) SignalLinkRaw(apiURL, deviceName string) (string, error) {
	return signal.GetLinkRawURI(apiURL, deviceName)
}

// SignalUnregister remove uma conta da signal-cli-rest-api.
func (a *App) SignalUnregister(apiURL, number string, deleteLocalData bool) error {
	return signal.Unregister(apiURL, number, deleteLocalData)
}

// SignalCheckAPI verifica se a signal-cli-rest-api está acessível na URL informada.
func (a *App) SignalCheckAPI(apiURL string) (map[string]interface{}, error) {
	return signal.CheckAPI(apiURL)
}

// SignalListAccounts retorna as contas já registradas na signal-cli-rest-api.
func (a *App) SignalListAccounts(apiURL string) ([]string, error) {
	return signal.ListAccounts(apiURL)
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

	// Registra ferramentas web
	a.toolRegistry.MustRegister(web.NewWebFetch())
	a.toolRegistry.MustRegister(web.NewWebSearch())

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

	log.Printf("[Tools] Registry inicializado com %d ferramentas: %v", a.toolRegistry.Count(), a.toolRegistry.Names())
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
func (a *App) shutdown(ctx context.Context) {
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

// ensureSpeechManager garante que o speechManager está inicializado.
// Se não estiver, tenta inicializar a partir da config.
// Retorna true se disponível, false caso contrário.
func (a *App) ensureSpeechManager() bool {
	if a.speechManager != nil {
		return true
	}
	cfg, err := config.Load()
	if err != nil {
		log.Printf("[Speech] Erro ao carregar config para inicializar speechManager: %v", err)
		return false
	}
	if cfg.APIKey == "" {
		log.Printf("[Speech] API key não configurada — speechManager indisponível")
		return false
	}
	a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")
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

// TranscribeWhisper transcreve áudio usando OpenAI Whisper
func (a *App) TranscribeWhisper(audioBase64 string, filename string) (*TranscriptionResultInfo, error) {
	if a.speechManager == nil {
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
func (a *App) CreateTab(title, icon string, setAsActive bool) (*database.ChatTab, error) {
	tab, err := database.CreateTab(title, icon, setAsActive)
	if err != nil {
		return nil, err
	}

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

	runtime.EventsEmit(a.ctx, "tab_activated", map[string]interface{}{
		"id": id,
	})

	return nil
}

// UpdateTabTitle atualiza o título de uma aba e da conversa associada
func (a *App) UpdateTabTitle(id uint, title string) error {
	tab, err := database.GetTab(id)
	if err != nil {
		return err
	}

	err = database.UpdateTabTitle(id, title)
	if err != nil {
		return err
	}

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

	conv, err := database.GetConversation(conversationId)
	if err != nil {
		return err
	}

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

	runtime.EventsEmit(a.ctx, "tab_cleared", map[string]interface{}{
		"id": id,
	})

	return nil
}

// ReorderTabs reordena as abas
func (a *App) ReorderTabs(orderedIds []uint) error {
	return database.ReorderTabs(orderedIds)
}

// ==================== Auto Update ====================

// initUpdater inicializa o gerenciador de atualizações
func (a *App) initUpdater() {
	// AppVersion é injetada via ldflags durante o build
	// Em dev, permanece como "dev"
	a.updater = updater.New(AppVersion)

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

// NeedsWelcomeWizard verifica se o assistente precisa do wizard de boas-vindas
// Retorna true se não houver provedor ou token configurado
func (a *App) NeedsWelcomeWizard() bool {
	cfg, err := config.Load()
	if err != nil {
		return true
	}

	// Verifica se tem API key e base URL configurados
	hasConfig := cfg.APIKey != "" && cfg.APIBaseURL != ""
	return !hasConfig
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
	
	// Controle de navegação entre etapas
	currentStep := 1
	
	for currentStep > 0 {
		switch currentStep {
		case 1: // Etapa 1: Escolher provedor
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
				baseURL = "https://generativelanguage.googleapis.com/v1beta"
			case "Azure OpenAI":
				baseURL = "" // Usuário precisará fornecer
			case "Ollama (Local)":
				baseURL = "http://localhost:11434/v1"
			case "LiteLLM":
				baseURL = "" // Usuário precisará fornecer
			case "Outro (URL personalizada)":
				baseURL = "" // Usuário precisará fornecer
			}
			
			currentStep = 2
			
		case 2: // Etapa 2: URL personalizada (se necessário)
			needsCustomURL := provider == "Outro (URL personalizada)" || provider == "Azure OpenAI" || provider == "LiteLLM"
			
			if !needsCustomURL {
				// Pula para próxima etapa se não precisa de URL customizada
				currentStep = 3
				continue
			}
			
			// Ajusta placeholder e exemplo baseado no provedor
			placeholderURL := "http://localhost:11434/v1"
			if provider == "LiteLLM" {
				placeholderURL = "http://localhost:4000"
			} else if provider == "Azure OpenAI" {
				placeholderURL = "https://your-resource.openai.azure.com"
			}
			
			urlResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Configuração do Servidor",
				Description: "Informe a URL do servidor OpenAI-compatible.",
				Questions: []questionnaire.Question{
					{
						ID:          "baseURL",
						Type:        "text",
						Prompt:      "URL do servidor",
						Required:    true,
						Placeholder: placeholderURL,
						Default:     baseURL, // Mantém valor anterior se voltar
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
				currentStep = 1 // Volta para etapa anterior
				continue
			}

			baseURL = urlResp.Answers["baseURL"].(string)
			currentStep = 3
			
		case 3: // Etapa 3: API Key
			keyDescription := "Informe sua chave de API. Deixe em branco se o servidor não requer autenticação."
			if provider == "Ollama (Local)" {
				keyDescription = "Ollama local geralmente não precisa de chave. Você pode deixar em branco."
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
						Default:     apiKey, // Mantém valor anterior se voltar
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
				// Volta para etapa anterior (URL customizada ou provedor)
				needsCustomURL := provider == "Outro (URL personalizada)" || provider == "Azure OpenAI" || provider == "LiteLLM"
				if needsCustomURL {
					currentStep = 2
				} else {
					currentStep = 1
				}
				continue
			}

			if keyResp.Answers["apiKey"] != nil {
				apiKey = keyResp.Answers["apiKey"].(string)
			}
			
			currentStep = 4
			
		case 4: // Etapa 4: Listar e escolher modelo
			// Salva configuração temporária para testar modelos
			tempCfg := &config.Config{
				APIKey:     apiKey,
				APIBaseURL: baseURL,
			}

			models, err := llm.GetModels(tempCfg)
			if err != nil {
				// Se falhou ao listar modelos, pergunta se quer continuar mesmo assim
				errorResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
					Title:       "Erro ao Listar Modelos",
					Description: fmt.Sprintf("Não foi possível listar os modelos disponíveis: %v\n\nVocê pode configurar um modelo padrão manualmente ou voltar para verificar suas credenciais.", err),
					Questions: []questionnaire.Question{
						{
							ID:          "defaultModel",
							Type:        "text",
							Prompt:      "Nome do modelo (ex: gpt-4o-mini, claude-3-5-sonnet-20241022)",
							Required:    true,
							Placeholder: "gpt-4o-mini",
							Default:     defaultModel,
						},
					},
					AllowCancel: true,
					SubmitLabel: "Salvar Configuração",
					CancelLabel: "Voltar",
				})

				if err != nil {
					return false, err
				}
				
				if errorResp.Cancelled {
					currentStep = 3 // Volta para API key
					continue
				}

				defaultModel = errorResp.Answers["defaultModel"].(string)

				// Salva configuração
				if err := a.saveWelcomeConfig(baseURL, apiKey, defaultModel); err != nil {
					return false, err
				}

				return true, nil
			}

			// Se conseguiu listar modelos, mostra seleção
			if len(models) == 0 {
				models = []string{"gpt-4o-mini", "claude-3-5-sonnet-20241022", "gemini-pro"}
			}
			
			// Mantém modelo selecionado anteriormente se houver
			modelDefault := ""
			if defaultModel != "" {
				modelDefault = defaultModel
			} else {
				modelDefault = models[0]
			}

			modelResp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
				Title:       "Escolha o Modelo Padrão",
				Description: "Selecione o modelo que será usado como padrão. Você pode alterar isso depois.",
				Questions: []questionnaire.Question{
					{
						ID:       "model",
						Type:     "single_choice",
						Prompt:   "Modelo padrão:",
						Required: true,
						Options:  models,
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
				currentStep = 3 // Volta para API key
				continue
			}

			defaultModel = modelResp.Answers["model"].(string)

			// Salva configuração
			if err := a.saveWelcomeConfig(baseURL, apiKey, defaultModel); err != nil {
				return false, err
			}

			// Atualiza modelo em todos os perfis
			if err := a.updateAllProfilesModel(defaultModel); err != nil {
				log.Printf("[Wizard] Aviso: erro ao atualizar modelo nos perfis: %v", err)
			}

			// Reinicializa cliente LLM
			a.initLLMClient()

			// Verifica se há atualizações disponíveis após configuração
			go a.checkForUpdatesAfterWizard()

			return true, nil
		}
	}
	
	return false, nil
}

// saveWelcomeConfig salva a configuração do wizard
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

// updateAllProfilesModel atualiza o modelo em todos os perfis
func (a *App) updateAllProfilesModel(model string) error {
	profiles, err := a.profileManager.List()
	if err != nil {
		return err
	}

	for _, profileInfo := range profiles {
		profile, err := a.profileManager.Get(profileInfo.Name)
		if err != nil {
			log.Printf("[Wizard] Erro ao carregar perfil %s: %v", profileInfo.Name, err)
			continue
		}

		profile.Chat.Model = model
		if err := a.profileManager.Update(profileInfo.Name, profile); err != nil {
			log.Printf("[Wizard] Erro ao salvar perfil %s: %v", profileInfo.Name, err)
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
