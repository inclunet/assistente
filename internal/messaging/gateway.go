package messaging

import (
	"assistente/internal/logging"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"assistente/internal/channels"
	"assistente/internal/contacts"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/textutil"
)

// SendMessageFunc é a assinatura do callback usado pelo gateway para enviar
// uma mensagem entrante de canal ao pipeline de chat. Recebe o ctx do
// gateway (que já carrega o OwnerUserID do canal via WithUserID — AEP-0052),
// conversationID (não vazio), conteúdo, mídia, params e source.
// Retorna o conversationID efetivamente usado.
//
// O caller é responsável por injetar o userID antes de chamar — gateway
// faz isso a partir de channelCfg.OwnerUserID. O destinatário NÃO deve
// substituir o ctx por um derivado de currentUserID (sessão Wails da UI),
// porque mensagens de canal precisam funcionar mesmo com a UI fechada/sem
// login: o owner do canal é a fonte de verdade, não o usuário ativo na UI.
type SendMessageFunc func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error)

// emitFunc é a callback para emitir eventos Wails.
type emitFunc func(event string, data any)

// SynthesizeTTSFunc é a assinatura da função que sintetiza texto em áudio (MP3).
// Recebe o texto, o canal e se a mensagem original era áudio.
// Resolve o perfil do canal e o ChannelResponseMode para decidir se gera TTS.
// Retorna (nil, nil) se não deve gerar áudio (o gateway enviará texto).
type SynthesizeTTSFunc func(ctx context.Context, text string, channel string, incomingIsAudio bool) ([]byte, error)

// SaveAudioFunc é a assinatura da função que salva áudio no DB.
type SaveAudioFunc func(ctx context.Context, messageID string, audioBase64 string, mimeType string) error

// ApproveContactFunc é a assinatura da função que solicita aprovação para autorizar um contato.
// Retorna true se aprovado, false caso contrário.
type ApproveContactFunc func(ctx context.Context, channel, displayName, contactID, username string) (bool, error)

// Gateway é o router central de mensageria. Conecta os adapters dos mensageiros
// ao processamento do assistente (via App.SendMessage).
//
// Fluxo:
//  1. Adapter recebe mensagem → chama handler do Gateway
//  2. Gateway verifica allowlist
//  3. Gateway registra callback no Notifier (para capturar resposta)
//  4. Gateway chama SendMessage (mesma função do Wails)
//  5. Quando resposta fica pronta, Notifier dispara callback
//  6. Gateway reenvia resposta ao mensageiro de origem
type Gateway struct {
	mu             sync.RWMutex
	messengers     map[string]Messenger
	notifier       *ResponseNotifier
	ttsBroker      *TTSBroker
	sendMessage    SendMessageFunc
	emitEvent      emitFunc
	approveContact ApproveContactFunc
	synthesizeTTS  SynthesizeTTSFunc // Opcional: sintetiza áudio para respostas em modo áudio
	saveAudio      SaveAudioFunc     // Opcional: salva áudio no DB
	// cancelStream cancela LLM em andamento (barge-in) antes de novo turno de canal.
	cancelStream func(conversationID string)
	// reconcileMu serializa ReconcilePending (boot + reload pós-login).
	reconcileMu sync.Mutex
	// reconcileRetrySem limita goroutines de retry no startup (M14).
	reconcileRetrySem chan struct{}
}

// SetCancelStream configura barge-in ao receber nova mensagem no mesmo conv.
func (g *Gateway) SetCancelStream(fn func(conversationID string)) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.cancelStream = fn
	g.mu.Unlock()
}

// NewGateway cria um novo Gateway de mensageria.
func NewGateway(
	notifier *ResponseNotifier,
	sendMessage SendMessageFunc,
	emitEvent emitFunc,
	approveContact ApproveContactFunc,
	synthesizeTTS SynthesizeTTSFunc,
	saveAudio SaveAudioFunc,
) *Gateway {
	return &Gateway{
		messengers:        make(map[string]Messenger),
		notifier:          notifier,
		ttsBroker:         NewTTSBroker(),
		sendMessage:       sendMessage,
		emitEvent:         emitEvent,
		approveContact:    approveContact,
		synthesizeTTS:     synthesizeTTS,
		saveAudio:         saveAudio,
		reconcileRetrySem: make(chan struct{}, 8),
	}
}

// Register registra um adapter de mensageiro e configura seu handler.
func (g *Gateway) Register(name string, m Messenger) {
	g.mu.Lock()
	g.messengers[name] = m
	g.mu.Unlock()

	m.SetHandler(g.handleIncoming)
	logging.Infof(context.Background(), "messaging.gateway", "[Gateway] Messenger '%s' registrado", name)
}

// Unregister desconecta e remove um messenger pelo nome.
//
// B7: cancela todos os callbacks pendentes do canal removido. Sem isso,
// um adapter que sai de cena (ex.: usuário desabilitou Telegram em
// settings) deixava callbacks órfãos pendurados — a próxima vez que
// alguém chamasse Notify para a conversa correspondente, dispararia
// callback contra um messenger inexistente (Send falharia silenciosamente)
// ou ficaria pendente para sempre se Notify nunca fosse chamado.
func (g *Gateway) Unregister(name string) {
	g.mu.Lock()
	if m, ok := g.messengers[name]; ok {
		logging.Errorf(context.Background(), "messaging.gateway", "[Gateway] Desconectando '%s'...", name)
		if err := m.Disconnect(); err != nil {
			logging.Errorf(context.Background(), "messaging.gateway", "[Gateway] Erro ao desconectar '%s': %v", name, err)
		}
		delete(g.messengers, name)
	}
	g.mu.Unlock()

	if g.notifier != nil {
		if cancelled := g.notifier.CancelByChannel(name); cancelled > 0 {
			logging.Infof(context.Background(), "messaging.gateway", "[Gateway] %d callback(s) cancelado(s) ao remover canal '%s'", cancelled, name)
		}
	}
}

// Shutdown desconecta todos os messengers e cancela callbacks pendentes (B7).
func (g *Gateway) Shutdown() {
	g.mu.Lock()
	channelNames := make([]string, 0, len(g.messengers))
	for name, m := range g.messengers {
		logging.Errorf(context.Background(), "messaging.gateway", "[Gateway] Desconectando '%s'...", name)
		if err := m.Disconnect(); err != nil {
			logging.Errorf(context.Background(), "messaging.gateway", "[Gateway] Erro ao desconectar '%s': %v", name, err)
		}
		channelNames = append(channelNames, name)
	}
	g.messengers = make(map[string]Messenger)
	g.mu.Unlock()

	if g.notifier != nil {
		for _, name := range channelNames {
			g.notifier.CancelByChannel(name)
		}
	}
}

// GetStatus retorna o status de todos os messengers registrados.
func (g *Gateway) GetStatus() map[string]ConnectionStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()

	status := make(map[string]ConnectionStatus, len(g.messengers))
	for name, m := range g.messengers {
		status[name] = m.Status()
	}
	return status
}

// GetMessenger retorna um messenger pelo nome (para uso pela tool send_message).
func (g *Gateway) GetMessenger(name string) (Messenger, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	m, ok := g.messengers[name]
	return m, ok
}

// handleIncoming é chamado quando uma mensagem chega de qualquer messenger.
func (g *Gateway) handleIncoming(ctx context.Context, msg IncomingMessage) {
	traceID := uuid.NewString()

	// 1. Verifica contato autorizado (contacts.json centralizado + max_contacts do canal)
	//    Carrega o config uma única vez e reusa para owner/profile abaixo.
	channelCfg, _ := channels.Load(msg.Channel)
	// GetMaxContacts é nil-safe (omitido/nil → 1).
	maxContacts := channelCfg.GetMaxContacts()

	// AEP-0052: propaga o dono do canal (definido em SaveChannelConfig com o
	// userID autenticado) no contexto. FindOrCreateChannelConversation usa
	// esse userID como dono da conversa criada — sem isso, mensagens
	// recebidas em canais criariam conversas órfãs (user_id="").
	//
	// Caminho legado: configs pré-AEP-0052 podem chegar com OwnerUserID="".
	// Em vez de criar conversas órfãs (que ficariam invisíveis a todos os
	// usuários), rejeitamos a mensagem com log explícito. O fluxo correto é
	// o usuário reabrir as settings do canal e salvar de novo (que carimba
	// OwnerUserID via App.SaveChannelConfig), ou rodar AdoptLegacyData no
	// primeiro login pós-upgrade. Sem essa migração o canal fica em modo
	// degradado mas nada vaza para outro usuário.
	if channelCfg == nil || channelCfg.OwnerUserID == "" {
		// M8: era silent failure — sem feedback ao remetente, sem evento
		// ao frontend, sem métrica. Agora:
		//   1. Loga estruturado (canal=legacy_owner_missing) para
		//      contagem em logs.
		//   2. Emite evento para o frontend (UI pode mostrar banner de
		//      "canal legado precisa ser reativado").
		//   3. Manda uma resposta humana ao remetente externo dizendo
		//      que o canal está em modo legado — sem isso a pessoa
		//      do outro lado fica falando com vácuo.
		logging.Warnf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s status=legacy_owner_missing mensagem rejeitada (config pré-AEP-0052; reabra settings do canal para reatribuir)",
			traceID, msg.Channel)
		if g.emitEvent != nil {
			g.emitEvent("messaging:legacy_channel_dropped", map[string]any{
				"channel":   msg.Channel,
				"from":      msg.From.DisplayName,
				"fromId":    msg.From.ID,
				"messageId": msg.ID,
				"reason":    "owner_missing",
			})
		}
		if messenger, ok := g.GetMessenger(msg.Channel); ok && msg.OutboundChatID() != "" {
			outMsg := OutgoingMessage{
				ChatID: msg.OutboundChatID(),
				Text:   "Este canal está em modo legado e aguarda reativação pelo administrador da instância. Sua mensagem não será processada.",
			}
			if err := messenger.Send(ctx, outMsg); err != nil {
				logging.Warnf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s erro ao enviar aviso de canal legado: %v",
					traceID, msg.Channel, err)
			}
		}
		return
	}
	// Locais após o guard acima: staticcheck SA5011 não prova non-nil
	// quando há checks `channelCfg != nil` mais abaixo.
	ownerUserID := channelCfg.OwnerUserID
	channelProfile := channelCfg.Profile
	channelMaxHistory := channelCfg.MaxHistory
	ctx = database.WithUserID(ctx, ownerUserID)

	hasContacts, isAllowed := contacts.IsAuthorized(msg.Channel, maxContacts, msg.From.ID, msg.From.Username)

	if hasContacts && !isAllowed {
		// Limite de contatos atingido e este não está na lista — rejeita silenciosamente
		logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=? channel=%s contact=%s name=%s msg=%s rejeitada (limite de contatos)",
			traceID, msg.Channel, maskIdentifier(msg.From.ID), msg.From.DisplayName, msg.ID)
		return
	}

	outboundChatID := msg.OutboundChatID()

	if !hasContacts {
		// Contato novo / vaga disponível — pareamento pelo próprio mensageiro
		// (o contato responde com o código). Não bloqueia em questionnaire/UI.
		if pending := contacts.GetPairingCode(msg.Channel, msg.From.ID); pending != nil {
			codeAttempt := strings.TrimSpace(msg.Text)
			// Só valida/consome tentativas quando a entrada parece um código
			// (6 dígitos). Mensagens livres ("oi") não devem esgotar o limite.
			if !isPairingCodeAttempt(codeAttempt) {
				logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s aguardando código de pareamento (entrada ignorada)",
					traceID, msg.Channel)
				if messenger, ok := g.GetMessenger(msg.Channel); ok {
					_ = messenger.Send(ctx, OutgoingMessage{
						ChatID: outboundChatID,
						Text:   "Aguardando o código de pareamento de 6 dígitos enviado anteriormente.",
					})
				}
				return
			}
			valid, validateErr := contacts.ValidatePairingCode(msg.Channel, msg.From.ID, codeAttempt)
			if !valid {
				logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s pareamento inválido: %v",
					traceID, msg.Channel, validateErr)
				if messenger, ok := g.GetMessenger(msg.Channel); ok {
					errText := "Código de pareamento inválido. Verifique e tente novamente."
					if validateErr != nil {
						errText = validateErr.Error()
					}
					_ = messenger.Send(ctx, OutgoingMessage{ChatID: outboundChatID, Text: errText})
				}
				return
			}
			if err := contacts.Authorize(msg.Channel, msg.From.ID, msg.From.DisplayName, msg.From.Username, maxContacts); err != nil {
				logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s erro ao autorizar contato: %v", traceID, msg.Channel, err)
				if messenger, ok := g.GetMessenger(msg.Channel); ok {
					_ = messenger.Send(ctx, OutgoingMessage{
						ChatID: outboundChatID,
						Text:   "Não foi possível concluir o pareamento (limite de contatos ou erro interno). Peça ao administrador para verificar a configuração do canal.",
					})
				}
				return
			}
			logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s contato autorizado via código: %s",
				traceID, msg.Channel, maskIdentifier(msg.From.ID))
			if g.emitEvent != nil {
				g.emitEvent("messaging:contact_authorized", map[string]any{
					"channel":     msg.Channel,
					"from":        msg.From.DisplayName,
					"fromId":      msg.From.ID,
					"username":    msg.From.Username,
					"messageId":   msg.ID,
				})
			}
			if messenger, ok := g.GetMessenger(msg.Channel); ok {
				_ = messenger.Send(ctx, OutgoingMessage{
					ChatID: outboundChatID,
					Text:   "Pareamento concluído! Você está autorizado. Envie sua mensagem.",
				})
			}
			// Não processa o código como prompt do LLM.
			return
		}

		logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=? channel=%s contact=%s username=%s name=%s msg=%s aguardando pareamento",
			traceID, msg.Channel, maskIdentifier(msg.From.ID), maskIdentifier(msg.From.Username), msg.From.DisplayName, msg.ID)

		pairingCode := contacts.GeneratePairingCode(msg.Channel, msg.From.ID)
		logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s contact=%s código de pareamento gerado: %s",
			traceID, msg.Channel, maskIdentifier(msg.From.ID), pairingCode)

		if messenger, ok := g.GetMessenger(msg.Channel); ok {
			pairingMsg := fmt.Sprintf(
				"Bem-vindo! Para autorizar seu acesso, responda ao assistente com o seguinte código de pareamento:\n\nCódigo: %s",
				pairingCode,
			)
			if err := messenger.Send(ctx, OutgoingMessage{ChatID: outboundChatID, Text: pairingMsg}); err != nil {
				logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s erro ao enviar código: %v", traceID, msg.Channel, err)
				contacts.CancelPairingCode(msg.Channel, msg.From.ID)
				return
			}
		} else {
			logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s channel=%s messenger ausente ao enviar código", traceID, msg.Channel)
			contacts.CancelPairingCode(msg.Channel, msg.From.ID)
			return
		}
		if g.emitEvent != nil {
			g.emitEvent("messaging:pairing_pending", map[string]any{
				"channel":   msg.Channel,
				"from":      msg.From.DisplayName,
				"fromId":    msg.From.ID,
				"username":  msg.From.Username,
				"messageId": msg.ID,
			})
		}
		return
	}

	// 2. Busca (ou cria) a conversa dedicada para este canal+contato.
	//    Primeiro verifica o config do canal (persistido entre reinícios),
	//    depois busca no DB por channel+contactID. O ctx já carrega o
	//    OwnerUserID do canal (injetado acima) — FindOrCreateChannelConversation
	//    o usa como dono da conversa criada.
	conv, created, err := database.FindOrCreateChannelConversationWithContext(
		ctx, msg.Channel, msg.From.ID, msg.From.DisplayName,
	)
	if err != nil {
		logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=? channel=%s contact=%s erro ao buscar/criar conversa: %v",
			traceID, msg.Channel, maskIdentifier(msg.From.ID), err)
		return
	}
	conversationID := conv.ID

	if created {
		logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s contact=%s nova conversa criada",
			traceID, conversationID, msg.Channel, maskIdentifier(msg.From.ID))
		// Persiste o mapeamento contactID → conversationID no config do canal
		if err := channels.SaveConversationID(msg.Channel, msg.From.ID, conversationID); err != nil {
			logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s erro ao persistir conversa no config: %v",
				traceID, conversationID, msg.Channel, err)
		}
	}
	// Só persiste override quando o destino outbound difere do contactID
	// (ex.: Slack user ≠ channel). Telegram/Signal evitam I/O por mensagem.
	if outboundChatID != "" && outboundChatID != msg.From.ID {
		if err := channels.SaveReplyChatID(msg.Channel, msg.From.ID, outboundChatID); err != nil {
			logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s erro ao persistir reply chat: %v",
				traceID, conversationID, msg.Channel, err)
		}
	}
	logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s contact=%s msg=%s recebida",
		traceID, conversationID, msg.Channel, maskIdentifier(msg.From.ID), msg.ID)

	// 3. Converte attachments em media JSON (mesmo formato que o frontend)
	mediaJSON := ""
	if len(msg.Attachments) > 0 {
		mediaJSON = attachmentsToMediaJSON(msg.Attachments)
		logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s attachments=%d convertidos para media JSON", traceID, conversationID, len(msg.Attachments))
	}

	// 4. Emite evento para o frontend
	hasAttachments := len(msg.Attachments) > 0
	if g.emitEvent != nil {
		g.emitEvent("messaging:incoming", map[string]any{
			"channel":         msg.Channel,
			"from":            msg.From.DisplayName,
			"fromId":          msg.From.ID,
			"text":            msg.Text,
			"messageId":       msg.ID,
			"conversationId":  conversationID,
			"newConversation": created,
			"hasAttachments":  hasAttachments,
			"audioOnly":       msg.IsAudioOnly(),
		})
	}

	// 6. Registra callback no Notifier para capturar a resposta e reenviar ao mensageiro.
	//    O ChannelResponseMode do perfil decide se a resposta será áudio ou texto.
	// Cancela streaming anterior (barge-in) para o Notify atrasado do turno
	// antigo não consumir o callback do turno novo.
	g.mu.RLock()
	cancelStream := g.cancelStream
	g.mu.RUnlock()
	if cancelStream != nil {
		cancelStream(conversationID)
	}
	incomingIsAudio := msg.IsAudioOnly()
	g.notifier.Register(conversationID, ResponseCallback{
		Channel:      msg.Channel,
		ChatID:       outboundChatID,
		OwnerUserID:  ownerUserID,
		AudioOnly:    incomingIsAudio, // hint para o notifier (mantém compatibilidade)
		ReplyToMsgID: msg.ID,
		TraceID:      traceID,
		Callback: func(response string, assistantMsgID string) {
			if err := g.deliverChannelResponse(ctx, msg.Channel, outboundChatID, response, assistantMsgID, incomingIsAudio, msg.ID, traceID, conversationID); err != nil {
				logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s erro ao enviar resposta: %v",
					traceID, conversationID, msg.Channel, err)
			}
		},
	})

	// 7. Chama o mesmo SendMessage que o Wails usa (com o conversationID dedicado)
	//    Usa o perfil do canal (se configurado) em vez do perfil ativo global.
	params := llm.ChatParams{}
	if channelProfile != "" {
		params.ProfileSlug = channelProfile
		logging.Infof(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s usando perfil=%s", traceID, conversationID, msg.Channel, channelProfile)
	}
	if channelMaxHistory > 0 {
		params.MaxContextMessages = channelMaxHistory
	}
	sendCtx := WithChannelTraceID(ctx, traceID)
	_, err = g.sendMessage(sendCtx, conversationID, msg.Text, mediaJSON, params, msg.Channel)
	if err != nil {
		logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s erro ao processar mensagem: %v", traceID, conversationID, msg.Channel, err)
		// B7: o callback deste turno nunca seria invocado porque
		// sendMessage falhou antes do agentic loop chegar a saveAndFinish
		// (que dispara Notify). CancelTrace (não Cancel) evita apagar a
		// pendência M14 de um turno mais novo na mesma conversa.
		g.notifier.CancelTrace(conversationID, traceID)
		g.mu.RLock()
		messenger, ok := g.messengers[msg.Channel]
		g.mu.RUnlock()
		if ok {
			outMsg := OutgoingMessage{
				ChatID: outboundChatID,
				Text:   "Não foi possível processar a mensagem. Tente novamente em instantes.",
			}
			_ = messenger.Send(ctx, outMsg)
		}
	}
}

// deliverChannelResponse monta e envia a OutgoingMessage (texto/TTS/thread)
// usada tanto no callback normal quanto no reconcile pós-crash.
func (g *Gateway) deliverChannelResponse(ctx context.Context, channel, chatID, response, assistantMsgID string, audioOnly bool, replyToMsgID, traceID, conversationID string) error {
	messenger, ok := g.GetMessenger(channel)
	if !ok {
		logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s messenger não encontrado para resposta",
			traceID, conversationID, channel)
		return fmt.Errorf("messenger %s ausente", channel)
	}

	// Texto falável / legível sem sintaxe Markdown (TTS + outbound texto).
	// O conteúdo no chat permanece em Markdown; só o que sai para canais/fala.
	plainResponse := textutil.StripMarkdownForSpeech(response)

	outMsg := OutgoingMessage{
		ChatID:           chatID,
		Text:             plainResponse,
		ReplyToMessageID: replyToMsgID,
		// TraceID do pending — NÃO DeliveredAssistantID (msgID muda entre
		// tentativas; TraceID é estável no turno e encolhe a janela residual
		// Send→MarkDelivered nas plataformas com dedup nativo).
		IdempotencyKey: traceID,
	}

	if g.synthesizeTTS != nil && assistantMsgID != "" {
		g.ttsBroker.Prepare(assistantMsgID)
		go func() {
			ttsCtx, ttsCancel := context.WithTimeout(ctx, 5*time.Second)
			defer ttsCancel()
			audioData, ttsErr := g.synthesizeTTS(ttsCtx, plainResponse, channel, audioOnly)
			if ttsErr != nil {
				logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s erro ao gerar TTS: %v",
					traceID, conversationID, channel, ttsErr)
				g.ttsBroker.Cancel(assistantMsgID)
				return
			}
			if len(audioData) == 0 {
				g.ttsBroker.Cancel(assistantMsgID)
				return
			}
			g.ttsBroker.Publish(assistantMsgID, audioData, "audio/mpeg")
		}()

		payload, ok := g.ttsBroker.Wait(assistantMsgID, 5*time.Second)
		if ok && len(payload.Data) > 0 {
			outMsg.Attachments = []Attachment{{
				Filename: "resposta.mp3",
				MIMEType: payload.MIMEType,
				Data:     payload.Data,
			}}
			outMsg.Text = ""
			logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s TTS gerado bytes=%d",
				traceID, conversationID, channel, len(payload.Data))
			if g.saveAudio != nil {
				if err := g.saveAudio(ctx, assistantMsgID, base64.StdEncoding.EncodeToString(payload.Data), payload.MIMEType); err != nil {
					logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s msgID=%s erro ao salvar áudio TTS no DB: %v",
						traceID, conversationID, assistantMsgID, err)
				}
			}
		} else {
			logging.Errorf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s TTS não disponível (timeout ou não aplicável)",
				traceID, conversationID, channel)
		}
	}

	if err := messenger.Send(ctx, outMsg); err != nil {
		return err
	}
	logging.Debugf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s channel=%s resposta enviada", traceID, conversationID, channel)
	// M14: após Send OK, SEMPRE MarkDelivered antes do Delete — inclusive com
	// assistantMsgID vazio (sentinel delivered:<traceID>). Pular a marca quando
	// o ID vinha vazio deixava pending sem DeliveredAssistantID e o reconcile
	// reenviava. Janela residual (crash entre Send e MarkDelivered) ainda pode
	// reenviar — at-least-once intencional; marcar antes do Send causaria perda
	// silenciosa se o crash fosse entre Mark e Send. Após MarkDelivered,
	// reconcile/retry (pendingSendGate) só limpam — sem segundo Send ao contato.
	// Slack reduz essa janela via IdempotencyKey→client_msg_id; Telegram/Signal
	// não têm chave nativa (residual permanece).
	if g.notifier != nil {
		if store := g.notifier.pendingStore(); store != nil && conversationID != "" {
			// Background: ctx do adapter pode cancelar no shutdown após Send OK.
			storeCtx := context.Background()
			markID := pendingDeliveredMarkID(assistantMsgID, traceID)
			if err := store.MarkDelivered(storeCtx, conversationID, traceID, markID); err != nil {
				logging.Warnf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s falha ao marcar pending entregue: %v",
					traceID, conversationID, err)
			}
			if err := store.DeleteIfTrace(storeCtx, conversationID, traceID); err != nil {
				logging.Warnf(ctx, "messaging.gateway", "[Gateway] trace=%s conv=%s falha ao remover pending após send: %v",
					traceID, conversationID, err)
			}
		}
	}
	return nil
}

// attachmentsToMediaJSON converte []Attachment para o formato media JSON
// usado pelo pipeline LLM (mesmo formato que o frontend envia).
// Formato: [{"name":"file.jpg","type":"image/jpeg","data":"base64...","size":1234}]
func attachmentsToMediaJSON(attachments []Attachment) string {
	var parts []map[string]interface{}
	for _, att := range attachments {
		parts = append(parts, map[string]interface{}{
			"name": att.Filename,
			"type": att.MIMEType,
			"data": base64.StdEncoding.EncodeToString(att.Data),
			"size": att.Size,
		})
	}
	data, err := json.Marshal(parts)
	if err != nil {
		// Mi7: improvável com map[string]interface{}, mas registra para
		// não silenciar diagnóstico em caso patológico (ex.: sob fuzzing
		// ou se a estrutura mudar e introduzir um valor não-serializável).
		logging.Errorf(context.Background(), "messaging.gateway", "[Gateway] erro ao serializar attachments para media JSON: %v", err)
		return ""
	}
	return string(data)
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

// isPairingCodeAttempt reporta se o texto parece um código de 6 dígitos.
// Entradas livres não devem consumir tentativas de ValidatePairingCode.
func isPairingCodeAttempt(text string) bool {
	if len(text) != 6 {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}
