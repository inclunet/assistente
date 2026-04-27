package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"

	"assistente/internal/channels"
	"assistente/internal/chat"
	"assistente/internal/contacts"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/messaging"
	"assistente/internal/messaging/signal"
	"assistente/internal/messaging/slack"
	"assistente/internal/messaging/telegram"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
	"assistente/internal/speech"
	"assistente/internal/tools"
	msgtool "assistente/internal/tools/messaging"
)

// MessagingControllerConfig agrupa todas as dependências do MessagingController.
type MessagingControllerConfig struct {
	Ctx              context.Context
	ProfileMgr       *profiles.Manager
	CredMgr          *credentials.Manager
	QuestionnaireMgr *questionnaire.Manager
	SpeechSvc        *speech.Service
	AudioRepo        speech.AudioRepository
	ToolRegistry     *tools.Registry
	Emitter          ports.Emitter
	ConvSvc          chat.ConversationRepository
	SendMessageFn    messaging.SendMessageFunc
}

// MessagingController é o Inbound Adapter para canais de mensageria externos
// (Telegram, Signal, Slack, etc.). Gerencia o gateway, conexões e contatos.
type MessagingController struct {
	ctx              context.Context
	profileMgr       *profiles.Manager
	credMgr          *credentials.Manager
	questionnaireMgr *questionnaire.Manager
	speechSvc        *speech.Service
	audioRepo        speech.AudioRepository
	toolRegistry     *tools.Registry
	emitter          ports.Emitter
	convSvc          chat.ConversationRepository
	sendMessageFn    messaging.SendMessageFunc

	// criados por Init()
	msgGateway       *messaging.Gateway
	responseNotifier *messaging.ResponseNotifier
}

// NewMessagingController cria o MessagingController com as dependências fornecidas.
// Chame Init() para inicializar o gateway e conectar os canais habilitados.
func NewMessagingController(cfg MessagingControllerConfig) *MessagingController {
	return &MessagingController{
		ctx:              cfg.Ctx,
		profileMgr:       cfg.ProfileMgr,
		credMgr:          cfg.CredMgr,
		questionnaireMgr: cfg.QuestionnaireMgr,
		speechSvc:        cfg.SpeechSvc,
		audioRepo:        cfg.AudioRepo,
		toolRegistry:     cfg.ToolRegistry,
		emitter:          cfg.Emitter,
		convSvc:          cfg.ConvSvc,
		sendMessageFn:    cfg.SendMessageFn,
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

// Init inicializa o gateway, conecta os canais habilitados e registra as tools
// de mensageria no ToolRegistry. Substitui a função initMessaging() do App.
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

		var profile *profiles.Profile
		if chCfg, _ := channels.Load(channel); chCfg != nil && chCfg.Profile != "" {
			if p, err := c.profileMgr.Get(chCfg.Profile); err == nil {
				profile = p
			}
		}
		if profile == nil {
			if p, err := c.profileMgr.GetActive(); err == nil {
				profile = p
			}
		}

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

		if profile != nil {
			if profile.Voice.Assistant.Provider == "disabled" || profile.Voice.Assistant.Provider == "" {
				log.Printf("[TTS-Channel] Voz desabilitada no perfil para canal %s — respondendo com texto", channel)
				return nil, nil
			}
			// WebSpeech e SAPI5 são providers locais do desktop — não funcionam para canais externos
			if profile.Voice.Assistant.Provider == "webspeech" || profile.Voice.Assistant.Provider == "sapi5" {
				log.Printf("[TTS-Channel] Provider '%s' é local e não suporta canais externos — respondendo com texto", profile.Voice.Assistant.Provider)
				return nil, nil
			}
		}

		if !c.speechSvc.EnsureSpeechManager() {
			return nil, fmt.Errorf("speech manager indisponível para TTS")
		}

		var result *speech.SynthesisResult
		var err error
		if profile != nil && profile.Voice.Assistant.VoiceID != "" {
			result, err = c.speechSvc.SynthesizeWithVoice(text, profile.Voice.Assistant.VoiceID)
		} else {
			result, err = c.speechSvc.Synthesize(text)
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

	approveContactFn := func(ctx context.Context, channel, displayName, contactID, username string) (bool, error) {
		if c.questionnaireMgr == nil {
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
		resp, err := c.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
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

		providedCode, ok := resp.Answers["pairing_code"].(string)
		if !ok {
			return false, fmt.Errorf("código de pareamento inválido")
		}

		valid, validateErr := contacts.ValidatePairingCode(channel, contactID, providedCode)
		if !valid {
			if validateErr != nil {
				return false, fmt.Errorf("pareamento falhou: %v", validateErr)
			}
			return false, fmt.Errorf("código de pareamento incorreto")
		}
		return true, nil
	}

	var saveAudio messaging.SaveAudioFunc
	if c.audioRepo != nil {
		saveAudio = c.audioRepo.SaveMessageAudio
	}

	c.msgGateway = messaging.NewGateway(
		c.responseNotifier,
		c.sendMessageFn,
		emitEvent,
		approveContactFn,
		synthesizeTTS,
		saveAudio,
	)

	enabledChannels, err := channels.LoadEnabled()
	if err != nil {
		log.Printf("[Messaging] Erro ao carregar canais: %v", err)
	}

	if cfg, ok := enabledChannels["telegram"]; ok {
		c.connectTelegram(cfg)
	}
	if cfg, ok := enabledChannels["signal"]; ok {
		c.connectSignal(cfg)
	} else {
		log.Printf("[Messaging] Signal não configurado ou desabilitado")
	}
	if cfg, ok := enabledChannels["slack"]; ok {
		c.connectSlack(cfg)
	}

	if c.toolRegistry != nil {
		sendMsgTool := msgtool.NewSendMessageTool(c.msgGateway)
		c.toolRegistry.MustRegister(sendMsgTool)
		log.Printf("[Messaging] Tool 'send_message' registrada")

		pairingTool := msgtool.NewValidatePairingCodeTool()
		c.toolRegistry.MustRegister(pairingTool)
		log.Printf("[Messaging] Tool 'validate_pairing_code' registrada")
	}

	log.Printf("[Messaging] Gateway inicializado")
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
func (c *MessagingController) RemoveAuthorizedContact(channel, contactID string) error {
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
func (c *MessagingController) AssignConversationToChannel(conversationID string, channel, contactID string) error {
	if channel == "" || contactID == "" {
		return fmt.Errorf("canal e contato são obrigatórios")
	}
	if _, err := c.convSvc.GetConversationInfo(conversationID); err != nil {
		return fmt.Errorf("conversa %s não encontrada: %w", conversationID, err)
	}
	if err := c.convSvc.UpdateConversationChannel(conversationID, channel, contactID); err != nil {
		return fmt.Errorf("erro ao atualizar conversa: %w", err)
	}
	log.Printf("[Bridge] Conversa %s atribuída ao canal %s (contato: %s)", conversationID, channel, contactID)
	return nil
}

// UnassignConversationFromChannel remove a vinculação de uma conversa com um canal externo.
func (c *MessagingController) UnassignConversationFromChannel(conversationID string) error {
	if err := c.convSvc.UpdateConversationChannel(conversationID, "", ""); err != nil {
		return fmt.Errorf("erro ao remover canal da conversa: %w", err)
	}
	log.Printf("[Bridge] Conversa %s desvinculada de canal externo", conversationID)
	return nil
}

// GetConversationChannel retorna o canal e contato vinculados a uma conversa.
func (c *MessagingController) GetConversationChannel(conversationID string) (string, string, error) {
	conv, err := c.convSvc.GetConversationInfo(conversationID)
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
		log.Printf("[Messaging] Gateway não inicializado, ignorando restart de %s", channelName)
		return
	}
	c.msgGateway.Unregister(channelName)
	if !cfg.Enabled {
		log.Printf("[Messaging] Canal %s desabilitado", channelName)
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
		log.Printf("[Messaging] Canal desconhecido: %s", channelName)
	}
}

// persistChannelCredentials extrai tokens sensíveis do config e os armazena no CredMgr,
// substituindo os valores em texto plano por refs.
func (c *MessagingController) persistChannelCredentials(channelName string, cfg *channels.ChannelConfig) error {
	if cfg == nil || c.credMgr == nil || !c.credMgr.CanPersist() {
		return nil
	}
	ctx := context.Background()
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
func (c *MessagingController) connectTelegram(cfg *channels.ChannelConfig) {
	botToken := cfg.BotToken
	if botToken == "" && cfg.BotTokenRef != "" {
		botToken = c.resolveCredentialRef(cfg.BotTokenRef)
	}
	if botToken == "" {
		log.Printf("[Messaging] Telegram não configurado (bot token ausente)")
		return
	}
	adapter := telegram.NewAdapter(botToken)
	c.msgGateway.Register("telegram", adapter)
	go func() {
		if err := adapter.Connect(c.ctx); err != nil {
			log.Printf("[Messaging] Erro ao conectar Telegram: %v", err)
		}
	}()
	log.Printf("[Messaging] Telegram conectado")
}

// connectSignal cria e registra o adapter do Signal (via signal-cli-rest-api).
func (c *MessagingController) connectSignal(cfg *channels.ChannelConfig) {
	if cfg.Account == "" || cfg.APIURL == "" {
		log.Printf("[Messaging] Signal não configurado (conta ou URL da API ausente)")
		return
	}
	adapter := signal.NewAdapter(cfg.APIURL, cfg.Account, c.credMgr)
	c.msgGateway.Register("signal", adapter)
	go func() {
		if err := adapter.Connect(c.ctx); err != nil {
			log.Printf("[Messaging] Erro ao conectar Signal: %v", err)
		}
	}()
	log.Printf("[Messaging] Signal conectado (api=%s, account=%s)", cfg.APIURL, credentials.MaskIdentifier(cfg.Account))
}

// connectSlack cria e registra o adapter do Slack (Socket Mode).
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
		log.Printf("[Messaging] Slack não configurado (bot/app token ausente)")
		return
	}
	adapter := slack.NewAdapter(botToken, appToken)
	c.msgGateway.Register("slack", adapter)
	go func() {
		if err := adapter.Connect(c.ctx); err != nil {
			log.Printf("[Messaging] Erro ao conectar Slack: %v", err)
		}
	}()
	log.Printf("[Messaging] Slack conectado")
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
func (c *MessagingController) resolveCredentialRef(ref string) string {
	if ref == "" || c.credMgr == nil {
		return ""
	}
	auth, err := c.credMgr.GetByPattern(ref)
	if err != nil {
		log.Printf("[Credentials] Erro ao resolver referência %s: %v", ref, err)
		return ""
	}
	return credentials.ResolveSecretFromAuth(auth)
}
