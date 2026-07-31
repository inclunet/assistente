package controllers

import (
	"assistente/internal/channels"
	"assistente/internal/chat"
	"assistente/internal/contacts"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/messaging"
	"assistente/internal/messaging/signal"
	"assistente/internal/messaging/slack"
	"assistente/internal/messaging/telegram"
	"assistente/internal/profiles"
	"assistente/internal/speech"
	"assistente/internal/textutil"
	"assistente/internal/tools"
	msgtool "assistente/internal/tools/messaging"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MessagingControllerConfig agrupa todas as dependências do MessagingController.
type MessagingControllerConfig struct {
	Ctx          context.Context
	ProfileMgr   *profiles.Manager
	CredMgr      *credentials.Manager
	SpeechSvc    *speech.Service
	AudioRepo    speech.AudioRepository
	ToolRegistry *tools.Registry
	Emitter      ports.Emitter
	ConvSvc      chat.ConversationRepository
	SendMessageFn messaging.SendMessageFunc
}

// MessagingController é o Inbound Adapter para canais de mensageria externos
// (Telegram, Signal, Slack, etc.). Gerencia o gateway, conexões e contatos.
type MessagingController struct {
	ctx           context.Context
	profileMgr    *profiles.Manager
	credMgr       *credentials.Manager
	speechSvc     *speech.Service
	audioRepo     speech.AudioRepository
	toolRegistry  *tools.Registry
	emitter       ports.Emitter
	convSvc       chat.ConversationRepository
	sendMessageFn messaging.SendMessageFunc

	// criados por Init()
	msgGateway       *messaging.Gateway
	responseNotifier *messaging.ResponseNotifier

	// credUserID escopo para resolver/persistir channel:* no CredManager
	// (user-scoped). Sem isso GetByPattern sem usuário ignora vault do login.
	credMu     sync.RWMutex
	credUserID string
}

// NewMessagingController cria o MessagingController com as dependências fornecidas.
// Chame Init() para inicializar o gateway e StartAdapters() após o ChatController.
func NewMessagingController(cfg MessagingControllerConfig) *MessagingController {
	return &MessagingController{
		ctx:           cfg.Ctx,
		profileMgr:    cfg.ProfileMgr,
		credMgr:       cfg.CredMgr,
		speechSvc:     cfg.SpeechSvc,
		audioRepo:     cfg.AudioRepo,
		toolRegistry:  cfg.ToolRegistry,
		emitter:       cfg.Emitter,
		convSvc:       cfg.ConvSvc,
		sendMessageFn: cfg.SendMessageFn,
	}
}

// Gateway retorna o gateway de mensageria. Disponível após Init().
func (c *MessagingController) Gateway() *messaging.Gateway {
	return c.msgGateway
}

// ResponseNotifier retorna o notificador de respostas. Disponível após Init().
func (c *MessagingController) ResponseNotifier() *messaging.ResponseNotifier {
	return c.responseNotifier
}

// SetCredentialUserID define o usuário autenticado usado ao resolver/persistir
// refs channel:{slug}:* no CredManager. Chamar no login e antes de Save/Restart.
func (c *MessagingController) SetCredentialUserID(userID string) {
	if c == nil {
		return
	}
	c.credMu.Lock()
	c.credUserID = strings.TrimSpace(userID)
	c.credMu.Unlock()
}

func (c *MessagingController) credentialUserID() string {
	if c == nil {
		return ""
	}
	c.credMu.RLock()
	defer c.credMu.RUnlock()
	return c.credUserID
}

func (c *MessagingController) credentialContext() context.Context {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if uid := c.credentialUserID(); uid != "" {
		return database.WithUserID(ctx, uid)
	}
	return ctx
}

// profileForChannel resolve o perfil que rege as respostas de um canal: o
// perfil configurado no canal quando existir, com fallback para o ativo.
// Texto entregue e áudio sintetizado precisam sair do mesmo perfil.
func (c *MessagingController) profileForChannel(channel string) *profiles.Profile {
	if c == nil || c.profileMgr == nil {
		return nil
	}
	if chCfg, _ := channels.Load(channel); chCfg != nil && chCfg.Profile != "" {
		if p, err := c.profileMgr.Get(chCfg.Profile); err == nil {
			return p
		}
	}
	if p, err := c.profileMgr.GetActive(); err == nil {
		return p
	}
	return nil
}

// Init inicializa o gateway e registra as tools de mensageria no ToolRegistry.
// Não conecta adapters — chame StartAdapters() após o ChatController existir.
func (c *MessagingController) Init() {
	c.responseNotifier = messaging.NewResponseNotifier()

	emitEvent := func(event string, data any) {
		c.emitter.Emit(event, data)
	}

	// synthesizeTTS — converte texto em áudio para canais externos.
	// Respeita ChannelResponseMode do perfil:
	//   "mirror" (padrão): áudio→áudio, texto→texto
	//   "always_text":     nunca gera TTS
	//   "always_audio":    sempre gera TTS
	// Retorna (nil, nil) se não deve gerar áudio (gateway enviará texto).
	synthesizeTTS := messaging.SynthesizeTTSFunc(func(ctx context.Context, text string, channel string, incomingIsAudio bool) ([]byte, error) {
		// Respeita cancelamento/timeout do gateway
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		profile := c.profileForChannel(channel)

		if profile != nil {
			if !profile.ShouldRespondWithAudio(incomingIsAudio) {
				logging.Infof(context.Background(), "controllers.messaging-controller", "[TTS-Channel] Modo '%s': não gerar áudio para canal %s (incoming_audio=%v)",
					profile.GetChannelResponseMode(), channel, incomingIsAudio)
				return nil, nil
			}
		} else {
			// Sem perfil: fallback para mirror
			if !incomingIsAudio {
				return nil, nil
			}
		}

		if profile != nil {
			if profile.Voice.Assistant.Provider == "disabled" || profile.Voice.Assistant.Provider == "" {
				logging.Infof(context.Background(), "controllers.messaging-controller", "[TTS-Channel] Voz desabilitada no perfil para canal %s — respondendo com texto", channel)
				return nil, nil
			}
			// WebSpeech e SAPI5 são providers locais do desktop — não funcionam para canais externos
			if profile.Voice.Assistant.Provider == "webspeech" || profile.Voice.Assistant.Provider == "sapi5" {
				logging.Infof(context.Background(), "controllers.messaging-controller", "[TTS-Channel] Provider '%s' é local e não suporta canais externos — respondendo com texto", profile.Voice.Assistant.Provider)
				return nil, nil
			}
		}

		if !c.speechSvc.EnsureSpeechManager(ctx) {
			return nil, fmt.Errorf("speech manager indisponível para TTS")
		}

		speechLanguage := ""
		if profile != nil {
			speechLanguage = profile.Input.Language
		}
		plain := textutil.StripMarkdownForSpeechLabeled(text, textutil.CodeBlockSpeechLabel(speechLanguage))
		if strings.TrimSpace(plain) == "" {
			plain = strings.TrimSpace(text)
		}
		if plain == "" {
			return nil, nil
		}

		var result *speech.SynthesisResult
		var err error
		if profile != nil && profile.Voice.Assistant.VoiceID != "" {
			result, err = c.speechSvc.SynthesizeWithVoice(ctx, plain, profile.Voice.Assistant.VoiceID)
		} else {
			result, err = c.speechSvc.Synthesize(ctx, plain)
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

	var saveAudio messaging.SaveAudioFunc
	if c.audioRepo != nil {
		saveAudio = c.audioRepo.SaveMessageAudio
	}

	// Pareamento é feito pelo próprio contato (responde com o código no
	// mensageiro). Não bloqueamos mais em questionnaire/UI.
	c.msgGateway = messaging.NewGateway(
		c.responseNotifier,
		c.sendMessageFn,
		emitEvent,
		nil,
		synthesizeTTS,
		saveAudio,
	)

	c.msgGateway.SetSpeechLanguage(func(channel string) string {
		p := c.profileForChannel(channel)
		if p == nil {
			return ""
		}
		return p.Input.Language
	})

	if c.toolRegistry != nil {
		sendMsgTool := msgtool.NewSendMessageTool(c.msgGateway)
		c.toolRegistry.MustRegister(sendMsgTool)
		logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Tool 'send_message' registrada")

		pairingTool := msgtool.NewValidatePairingCodeTool()
		c.toolRegistry.MustRegister(pairingTool)
		logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Tool 'validate_pairing_code' registrada")
	}

	logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Gateway inicializado")
}

// StartAdapters conecta os messengers habilitados. Deve ser chamado somente
// depois que o ChatController existir — Init() cria o gateway cedo (agent/
// notifier dependem dele), mas Connect antes de chatCtrl gera NPE em
// SendMessageFromChannel se uma mensagem chegar no startup.
//
// ownerUserID não vazio (pós-login): carrega só canais desse usuário + órfãos,
// evitando conectar adapters de outros donos no mesmo SQLite. Vazio (boot
// pré-login / sem escopo de usuário): usa LoadEnabled global (FS ou DB,
// conforme o store ativo) — sem filtro por dono.
func (c *MessagingController) StartAdapters(ownerUserID string) {
	if c == nil || c.msgGateway == nil {
		logging.Warnf(context.Background(), "controllers.messaging-controller", "[Messaging] StartAdapters ignorado: gateway não inicializado")
		return
	}

	c.SetCredentialUserID(ownerUserID)

	var (
		enabledChannels map[string]*channels.ChannelConfig
		err             error
	)
	if strings.TrimSpace(ownerUserID) != "" {
		enabledChannels, err = channels.LoadEnabledForUser(ownerUserID)
	} else {
		enabledChannels, err = channels.LoadEnabled()
	}
	if err != nil {
		logging.Errorf(context.Background(), "controllers.messaging-controller", "[Messaging] Erro ao carregar canais: %v", err)
		return
	}

	if c.responseNotifier != nil {
		// Persistência M14 antes de Connect: mensagem que chega durante o
		// handshake já encontra store pronto no Register do gateway.
		c.responseNotifier.SetPendingStore(messaging.NewDBChannelPendingStore())
	}

	// Unregister antes de Connect: StartAdapters roda no boot e de novo
	// pós-login (após import legado). Sem Disconnect, loops de polling
	// órfãos ficariam em paralelo e mensagens poderiam ser processadas 2x.
	for _, name := range []string{"telegram", "signal", "slack"} {
		if cfg, ok := enabledChannels[name]; ok {
			c.restartChannel(name, cfg)
			continue
		}
		c.msgGateway.Unregister(name)
		if name == "signal" {
			logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Signal não configurado ou desabilitado")
		}
	}

	if c.msgGateway != nil {
		// Best-effort e assíncrono: DB lento/travado não pode bloquear o
		// Connect dos adapters. O timeout limita só a fase inicial do
		// ReconcilePending (List/find/send síncronos); retries agendados
		// seguem best-effort em background com timeout próprio por tentativa.
		// Falha parcial deixa pending no store para o próximo restart.
		go func(gw *messaging.Gateway) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			gw.ReconcilePending(ctx, messaging.DefaultFindAssistantAfter)
		}(c.msgGateway)
	}
}

// ============================================================================
// API pública — exposta ao frontend via Wails
// ============================================================================

// GetMessagingStatus retorna o status de todos os mensageiros conectados.
func (c *MessagingController) GetMessagingStatus() map[string]string {
	if c.msgGateway == nil {
		return map[string]string{}
	}
	status := c.msgGateway.GetStatus()
	result := make(map[string]string, len(status))
	for k, v := range status {
		result[k] = string(v)
	}
	return result
}

// GetChannelConfig retorna a configuração de um canal de mensageria.
func (c *MessagingController) GetChannelConfig(channelName string) (*channels.ChannelConfig, error) {
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
func (c *MessagingController) SaveChannelConfig(channelName string, cfg *channels.ChannelConfig) error {
	if err := c.persistChannelCredentials(channelName, cfg); err != nil {
		return err
	}
	if err := channels.Save(channelName, cfg); err != nil {
		return err
	}
	c.restartChannel(channelName, cfg)
	return nil
}

// RestartChannel reconecta um canal de mensageria (exposto ao frontend).
func (c *MessagingController) RestartChannel(channelName string) error {
	cfg, err := channels.Load(channelName)
	if err != nil {
		return fmt.Errorf("erro ao carregar config do canal %s: %w", channelName, err)
	}
	if cfg == nil {
		return fmt.Errorf("canal %s não configurado", channelName)
	}
	c.restartChannel(channelName, cfg)
	return nil
}

// GetAllChannelConfigs retorna as configurações de todos os canais.
func (c *MessagingController) GetAllChannelConfigs() (map[string]*channels.ChannelConfig, error) {
	return channels.ListAll()
}

// GetChannelTemplates retorna todos os templates disponíveis para criar novos canais.
func (c *MessagingController) GetChannelTemplates() []channels.ChannelTemplate {
	all := channels.GetAvailableTemplates()
	supported := c.getSupportedChannelTypes()
	filtered := make([]channels.ChannelTemplate, 0, len(all))
	for _, t := range all {
		if _, ok := supported[t.Type]; ok {
			t.Supported = true
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// CreateChannelFromTemplate cria um novo canal a partir de um template.
// templateType: "telegram", "signal", "whatsapp", "slack", "teams", "email"
func (c *MessagingController) CreateChannelFromTemplate(templateType string, values map[string]interface{}) error {
	if err := channels.CreateFromTemplate(templateType, values); err != nil {
		return err
	}
	c.emitter.Emit("channel:created", map[string]string{"type": templateType})
	return nil
}

// GetChannelConfigAsMap retorna a configuração de um canal como mapa para exibir na UI.
func (c *MessagingController) GetChannelConfigAsMap(channelName string) (map[string]interface{}, error) {
	return channels.GetChannelConfigAsMap(channelName)
}

// AuthorizeMessagingContactFull autoriza um contato em um canal.
// Respeita o limite max_contacts configurado no canal.
func (c *MessagingController) AuthorizeMessagingContactFull(channel, contactID, displayName, username string) error {
	if channel == "" || contactID == "" {
		return fmt.Errorf("canal e ID do contato são obrigatórios")
	}
	chCfg, _ := channels.Load(channel)
	// GetMaxContacts é nil-safe (config ausente → 1).
	maxContacts := chCfg.GetMaxContacts()
	if err := contacts.Authorize(channel, contactID, displayName, username, maxContacts); err != nil {
		return fmt.Errorf("erro ao autorizar: %w", err)
	}
	logging.Infof(context.Background(), "controllers.messaging-controller", "[Contacts] Contato %s (%s) autorizado no canal %s", displayName, contactID, channel)
	return nil
}

// RemoveAuthorizedContact remove um contato específico de um canal.
func (c *MessagingController) RemoveAuthorizedContact(channel, contactID string) error {
	if channel == "" || contactID == "" {
		return fmt.Errorf("canal e ID do contato são obrigatórios")
	}
	if err := contacts.Remove(channel, contactID); err != nil {
		return fmt.Errorf("erro ao remover contato: %w", err)
	}
	logging.Infof(context.Background(), "controllers.messaging-controller", "[Contacts] Contato %s removido do canal %s", contactID, channel)
	return nil
}

// GetAuthorizedContacts retorna todos os contatos autorizados (mapa canal → lista).
func (c *MessagingController) GetAuthorizedContacts() (contacts.ContactsFile, error) {
	return contacts.GetAll()
}

// ChannelInfo descreve um canal de mensageria disponível para atribuição.
type ChannelInfo struct {
	Name        string                        `json:"name"`
	Connected   bool                          `json:"connected"`
	Contacts    []*contacts.AuthorizedContact `json:"contacts"`
	MaxContacts int                           `json:"maxContacts"`
}

// GetAvailableChannels retorna os canais habilitados, seu status e contatos autorizados.
func (c *MessagingController) GetAvailableChannels() []ChannelInfo {
	enabledChannels, err := channels.LoadEnabled()
	if err != nil {
		return nil
	}
	authorizedContacts, _ := contacts.GetAll()
	var result []ChannelInfo
	var status map[string]messaging.ConnectionStatus
	if c.msgGateway != nil {
		status = c.msgGateway.GetStatus()
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
//
// AEP-0052 / B6: o ctx deve carregar o userID autenticado (via
// database.WithUserID). O DBConversationStore exige RequireUserID e
// rejeita ctx sem escopo — sem isso, qualquer conversa visível seria
// alterável por qualquer caller.
func (c *MessagingController) AssignConversationToChannel(ctx context.Context, conversationID string, channel, contactID string) error {
	if channel == "" || contactID == "" {
		return fmt.Errorf("canal e contato são obrigatórios")
	}
	if _, err := c.convSvc.GetConversationInfo(ctx, conversationID); err != nil {
		return fmt.Errorf("conversa %s não encontrada: %w", conversationID, err)
	}
	if err := c.convSvc.UpdateConversationChannel(ctx, conversationID, channel, contactID); err != nil {
		return fmt.Errorf("erro ao atualizar conversa: %w", err)
	}
	logging.Infof(ctx, "controllers.messaging-controller", "[Bridge] Conversa %s atribuída ao canal %s (contato: %s)", conversationID, channel, contactID)
	return nil
}

// UnassignConversationFromChannel remove a vinculação de uma conversa com um canal externo.
// O ctx deve carregar userID autenticado (AEP-0052 / B6).
func (c *MessagingController) UnassignConversationFromChannel(ctx context.Context, conversationID string) error {
	if err := c.convSvc.UpdateConversationChannel(ctx, conversationID, "", ""); err != nil {
		return fmt.Errorf("erro ao remover canal da conversa: %w", err)
	}
	logging.Infof(ctx, "controllers.messaging-controller", "[Bridge] Conversa %s desvinculada de canal externo", conversationID)
	return nil
}

// GetConversationChannel retorna o canal e contato vinculados a uma conversa.
// O ctx deve carregar userID autenticado (AEP-0052 / B6).
func (c *MessagingController) GetConversationChannel(ctx context.Context, conversationID string) (string, string, error) {
	conv, err := c.convSvc.GetConversationInfo(ctx, conversationID)
	if err != nil {
		return "", "", err
	}
	return conv.Channel, conv.ContactID, nil
}

// ============================================================================
// Métodos privados de infraestrutura
// ============================================================================

// restartChannel desconecta o adapter anterior (se houver) e reconecta com a nova config.
func (c *MessagingController) restartChannel(channelName string, cfg *channels.ChannelConfig) {
	if c.msgGateway == nil {
		logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Gateway não inicializado, ignorando restart de %s", channelName)
		return
	}
	c.msgGateway.Unregister(channelName)
	if !cfg.Enabled {
		logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Canal %s desabilitado", channelName)
		return
	}
	switch channelName {
	case "telegram":
		c.connectTelegram(cfg)
	case "signal":
		c.connectSignal(cfg)
	case "slack":
		c.connectSlack(cfg)
	default:
		logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Canal desconhecido: %s", channelName)
	}
}

// persistChannelCredentials extrai tokens sensíveis do config e os armazena no CredMgr,
// substituindo os valores em texto plano por refs.
func (c *MessagingController) persistChannelCredentials(channelName string, cfg *channels.ChannelConfig) error {
	if cfg == nil || c.credMgr == nil || !c.credMgr.CanPersist() {
		return nil
	}
	ctx := c.credentialContext()
	switch channelName {
	case "telegram":
		if cfg.BotTokenRef == "" && cfg.BotToken != "" {
			cfg.BotTokenRef = fmt.Sprintf("channel:%s:bot_token", channelName)
		}
		if cfg.BotTokenRef != "" && cfg.BotToken != "" {
			if err := c.credMgr.RegisterPatternWithContext(ctx, cfg.BotTokenRef, &credentials.AuthConfig{
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
			if err := c.credMgr.RegisterPatternWithContext(ctx, cfg.BotTokenRef, &credentials.AuthConfig{
				Type:  "secret",
				Token: cfg.BotToken,
			}); err != nil {
				return err
			}
			cfg.BotToken = ""
		}
		if cfg.AppTokenRef != "" && cfg.AppToken != "" {
			if err := c.credMgr.RegisterPatternWithContext(ctx, cfg.AppTokenRef, &credentials.AuthConfig{
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
			if err := c.credMgr.RegisterPatternWithContext(ctx, cfg.APITokenRef, &credentials.AuthConfig{
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

// connectTelegram cria e registra o adapter do Telegram.
// Token: BotToken plaintext ou BotTokenRef via resolveCredentialRef (user-scoped).
func (c *MessagingController) connectTelegram(cfg *channels.ChannelConfig) {
	botToken := cfg.BotToken
	if botToken == "" && cfg.BotTokenRef != "" {
		botToken = c.resolveCredentialRef(cfg.BotTokenRef)
	}
	if botToken == "" {
		logging.Errorf(context.Background(), "controllers.messaging-controller", "[Messaging] Telegram não configurado (bot token ausente)")
		return
	}
	adapter := telegram.NewAdapter(botToken)
	c.msgGateway.Register("telegram", adapter)
	go func() {
		if err := adapter.Connect(c.ctx); err != nil {
			logging.Errorf(context.Background(), "controllers.messaging-controller", "[Messaging] Erro ao conectar Telegram: %v", err)
		}
	}()
	logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Telegram conectado")
}

// connectSignal cria e registra o adapter do Signal (via signal-cli-rest-api).
// APIToken opcional: plaintext ou APITokenRef via resolveCredentialRef (mesmo
// escopo de usuário que Telegram/Slack).
func (c *MessagingController) connectSignal(cfg *channels.ChannelConfig) {
	if cfg.Account == "" || cfg.APIURL == "" {
		logging.Errorf(context.Background(), "controllers.messaging-controller", "[Messaging] Signal não configurado (conta ou URL da API ausente)")
		return
	}
	apiToken := cfg.APIToken
	if apiToken == "" && cfg.APITokenRef != "" {
		apiToken = c.resolveCredentialRef(cfg.APITokenRef)
	}
	adapter := signal.NewAdapter(cfg.APIURL, cfg.Account, c.credMgr, apiToken)
	c.msgGateway.Register("signal", adapter)
	go func() {
		if err := adapter.Connect(c.ctx); err != nil {
			logging.Errorf(context.Background(), "controllers.messaging-controller", "[Messaging] Erro ao conectar Signal: %v", err)
		}
	}()
	logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Signal conectado (api=%s, account=%s)", cfg.APIURL, credentials.MaskIdentifier(cfg.Account))
}

// connectSlack cria e registra o adapter do Slack (Socket Mode).
// BotToken/AppToken: plaintext ou *TokenRef via resolveCredentialRef (user-scoped).
func (c *MessagingController) connectSlack(cfg *channels.ChannelConfig) {
	botToken := cfg.BotToken
	appToken := cfg.AppToken
	if botToken == "" && cfg.BotTokenRef != "" {
		botToken = c.resolveCredentialRef(cfg.BotTokenRef)
	}
	if appToken == "" && cfg.AppTokenRef != "" {
		appToken = c.resolveCredentialRef(cfg.AppTokenRef)
	}
	if botToken == "" || appToken == "" {
		logging.Errorf(context.Background(), "controllers.messaging-controller", "[Messaging] Slack não configurado (bot/app token ausente)")
		return
	}
	adapter := slack.NewAdapter(botToken, appToken)
	c.msgGateway.Register("slack", adapter)
	go func() {
		if err := adapter.Connect(c.ctx); err != nil {
			logging.Errorf(context.Background(), "controllers.messaging-controller", "[Messaging] Erro ao conectar Slack: %v", err)
		}
	}()
	logging.Infof(context.Background(), "controllers.messaging-controller", "[Messaging] Slack conectado")
}

// getSupportedChannelTypes retorna os tipos de canais suportados.
// Mantém alinhado com os adapters disponíveis (connectTelegram/Signal/Slack).
func (c *MessagingController) getSupportedChannelTypes() map[string]struct{} {
	return map[string]struct{}{
		"telegram": {},
		"signal":   {},
		"slack":    {},
	}
}

// resolveCredentialRef resolve uma referência de credencial para o valor secreto.
// Usa o userID de SetCredentialUserID/StartAdapters — GetByPattern sem escopo
// ignora credenciais user-scoped (channel:* no vault pós-login).
func (c *MessagingController) resolveCredentialRef(ref string) string {
	if ref == "" || c.credMgr == nil {
		return ""
	}
	auth, err := c.credMgr.GetByPatternWithContext(c.credentialContext(), ref)
	if err != nil {
		logging.Errorf(context.Background(), "controllers.messaging-controller", "[Credentials] Erro ao resolver referência %s: %v", ref, err)
		return ""
	}
	if auth == nil {
		uid := c.credentialUserID()
		if uid == "" {
			uid = "<sem usuario>"
		}
		logging.Warnf(context.Background(), "controllers.messaging-controller", "[Credentials] referência %s não encontrada no vault (user=%s)", ref, uid)
		return ""
	}
	return credentials.ResolveSecretFromAuth(auth)
}
