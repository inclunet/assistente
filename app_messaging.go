package main

import (
	msgtool "assistente/internal/tools/messaging"

	"assistente/internal/channels"
	"assistente/internal/contacts"
	"assistente/internal/credentials"
	"assistente/internal/messaging"
	"assistente/internal/messaging/signal"
	"assistente/internal/messaging/sip"
	"assistente/internal/messaging/slack"
	"assistente/internal/messaging/telegram"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
	"assistente/internal/speech"
	"context"
	"encoding/base64"
	"fmt"
	"log"

)

func (a *App) initMessaging() {
	// ResponseNotifier — permite ao gateway capturar respostas para reenvio
	a.responseNotifier = messaging.NewResponseNotifier()

	emitEvent := func(event string, data any) {
		a.emitter.Emit( event, data)
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

		if !a.ensureSpeechManager() {
			return nil, fmt.Errorf("speech manager indisponível para TTS")
		}

		// Usa a voz do perfil se especificada, senão usa Synthesize padrão
		var result *speech.SynthesisResult
		var err error
		if profile != nil && profile.Voice.Assistant.VoiceID != "" {
			result, err = a.speechManager.SynthesizeWithVoice(text, profile.Voice.Assistant.VoiceID)
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
		a.audioSvc.SaveMessageAudio,
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
		log.Printf("[Messaging] Signal habilitado (api=%s, account=%s)", cfg.APIURL, credentials.MaskIdentifier(cfg.Account))
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

	// SIP (Telefonia)
	if cfg, ok := enabledChannels["sip"]; ok && cfg.SIPServer != "" && cfg.SIPUser != "" {
		sipPassword := cfg.SIPPassword
		if sipPassword == "" && cfg.SIPPasswordRef != "" {
			sipPassword = a.resolveCredentialRef(cfg.SIPPasswordRef)
		}
		if sipPassword == "" {
			log.Printf("[Messaging] SIP: senha vazia, não conectando")
		} else {
			sipCfg := sip.SIPConfig{
				Server:      cfg.SIPServer,
				Port:        cfg.SIPPort,
				Transport:   cfg.SIPTransport,
				User:        cfg.SIPUser,
				Password:    sipPassword,
				DisplayName: cfg.SIPDisplayName,
				LocalIP:     cfg.SIPLocalIP,
			}
			adapter := sip.NewAdapter(sipCfg)
			adapter.CancelStreamingForContact = a.cancelStreamingForContact
			a.msgGateway.Register("sip", adapter)
			go func() {
				if err := adapter.Connect(a.ctx); err != nil {
					log.Printf("[Messaging] Erro ao conectar SIP: %v", err)
				}
			}()
			log.Printf("[Messaging] SIP habilitado (%s@%s:%d)", cfg.SIPUser, cfg.SIPServer, sipCfg.GetPort())
		}
	} else {
		log.Printf("[Messaging] SIP não configurado ou desabilitado")
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
	case "sip":
		if cfg.SIPPasswordRef == "" && cfg.SIPPassword != "" {
			cfg.SIPPasswordRef = fmt.Sprintf("channel:%s:sip_password", channelName)
		}
		if cfg.SIPPasswordRef != "" && cfg.SIPPassword != "" {
			if err := a.credMgr.RegisterPatternWithContext(ctx, cfg.SIPPasswordRef, &credentials.AuthConfig{
				Type:  "secret",
				Token: cfg.SIPPassword,
			}); err != nil {
				return err
			}
			cfg.SIPPassword = ""
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
		log.Printf("[Messaging] Signal reconectado (api=%s, account=%s)", cfg.APIURL, credentials.MaskIdentifier(cfg.Account))

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

	case "sip":
		if cfg.SIPServer == "" || cfg.SIPUser == "" {
			log.Printf("[Messaging] SIP: servidor ou usuário vazios, não conectando")
			return
		}
		sipPassword := cfg.SIPPassword
		if sipPassword == "" && cfg.SIPPasswordRef != "" {
			sipPassword = a.resolveCredentialRef(cfg.SIPPasswordRef)
		}
		if sipPassword == "" {
			log.Printf("[Messaging] SIP: senha vazia, não conectando")
			return
		}
		sipCfg := sip.SIPConfig{
			Server:      cfg.SIPServer,
			Port:        cfg.SIPPort,
			Transport:   cfg.SIPTransport,
			User:        cfg.SIPUser,
			Password:    sipPassword,
			DisplayName: cfg.SIPDisplayName,
			LocalIP:     cfg.SIPLocalIP,
		}
		adapter := sip.NewAdapter(sipCfg)
		adapter.CancelStreamingForContact = a.cancelStreamingForContact
		if cfg.Profile != "" {
			if p, err := a.profileManager.Get(cfg.Profile); err == nil {
				sm := a.createSpeechManagerForProfile(p)
				if sm != nil {
					adapter.SetSpeechManager(sm)
				}
				if p.Voice.Assistant.VoiceID != "" {
					adapter.SetVoiceID(p.Voice.Assistant.VoiceID)
				}
			}
		} else if a.speechManager != nil {
			adapter.SetSpeechManager(a.speechManager)
		}
		a.msgGateway.Register("sip", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar SIP: %v", err)
			}
		}()
		log.Printf("[Messaging] SIP reconectado (%s@%s:%d)", cfg.SIPUser, cfg.SIPServer, sipCfg.GetPort())

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
		"sip":      {},
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
	a.emitter.Emit( "channel:created", map[string]string{"type": templateType})

	return nil
}

// GetChannelConfigAsMap retorna a configuração de um canal como mapa para exibir na UI.
func (a *App) GetChannelConfigAsMap(channelName string) (map[string]interface{}, error) {
	return channels.GetChannelConfigAsMap(channelName)
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
	conv, err := a.convSvc.GetConversationInfo(conversationID)
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

	conv, err := a.convSvc.GetConversationInfo(conversationID)
	if err != nil {
		return fmt.Errorf("conversa %d não encontrada: %w", conversationID, err)
	}

	// Atualiza os campos de canal na conversa
	conv.Channel = channel
	conv.ContactID = contactID
	if err := a.convSvc.UpdateConversationChannel(conversationID, channel, contactID); err != nil {
		return fmt.Errorf("erro ao atualizar conversa: %w", err)
	}

	log.Printf("[Bridge] Conversa %d atribuída ao canal %s (contato: %s)", conversationID, channel, contactID)
	return nil
}

// UnassignConversationFromChannel remove a vinculação de uma conversa com um canal externo.
func (a *App) UnassignConversationFromChannel(conversationID uint) error {
	if err := a.convSvc.UpdateConversationChannel(conversationID, "", ""); err != nil {
		return fmt.Errorf("erro ao remover canal da conversa: %w", err)
	}

	log.Printf("[Bridge] Conversa %d desvinculada de canal externo", conversationID)
	return nil
}

// GetConversationChannel retorna o canal e contato vinculados a uma conversa.
func (a *App) GetConversationChannel(conversationID uint) (string, string, error) {
	conv, err := a.convSvc.GetConversationInfo(conversationID)
	if err != nil {
		return "", "", err
	}
	return conv.Channel, conv.ContactID, nil
}
