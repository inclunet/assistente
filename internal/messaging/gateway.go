package messaging

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"

	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/llm"
)

// SendMessageFunc é a assinatura da função App.SendMessage (ou wrapper).
// Recebe conversationID (0=criar nova), conteúdo, mídia, params e source.
// Retorna o conversationID usado.
type SendMessageFunc func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error)

// emitFunc é a callback para emitir eventos Wails.
type emitFunc func(event string, data any)

// SynthesizeTTSFunc é a assinatura da função que sintetiza texto em áudio (MP3).
// Retorna os bytes do áudio MP3. Usada para responder em áudio quando a mensagem original era áudio.
type SynthesizeTTSFunc func(text string) ([]byte, error)

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
	config         *config.MessagingConfig
	sendMessage    SendMessageFunc
	emitEvent      emitFunc
	synthesizeTTS  SynthesizeTTSFunc // Opcional: sintetiza áudio para respostas em modo áudio
}

// NewGateway cria um novo Gateway de mensageria.
func NewGateway(
	notifier *ResponseNotifier,
	msgConfig *config.MessagingConfig,
	sendMessage SendMessageFunc,
	emitEvent emitFunc,
	synthesizeTTS SynthesizeTTSFunc,
) *Gateway {
	return &Gateway{
		messengers:    make(map[string]Messenger),
		notifier:      notifier,
		config:        msgConfig,
		sendMessage:   sendMessage,
		emitEvent:     emitEvent,
		synthesizeTTS: synthesizeTTS,
	}
}

// Register registra um adapter de mensageiro e configura seu handler.
func (g *Gateway) Register(name string, m Messenger) {
	g.mu.Lock()
	g.messengers[name] = m
	g.mu.Unlock()

	m.SetHandler(g.handleIncoming)
	fmt.Printf("[Gateway] Messenger '%s' registrado\n", name)
}

// Shutdown desconecta todos os messengers.
func (g *Gateway) Shutdown() {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for name, m := range g.messengers {
		fmt.Printf("[Gateway] Desconectando '%s'...\n", name)
		if err := m.Disconnect(); err != nil {
			fmt.Printf("[Gateway] Erro ao desconectar '%s': %v\n", name, err)
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

// UpdateConfig atualiza a configuração de mensageria em memória.
// Usado após autorizar um contato, por exemplo.
func (g *Gateway) UpdateConfig(cfg *config.MessagingConfig) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.config = cfg
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
	// 1. Verifica allowlist
	channelConfig := g.getChannelConfig(msg.Channel)
	if !config.IsContactAllowed(channelConfig, msg.From.ID, msg.From.Username) {
		fmt.Printf("[Gateway] Mensagem de %s (%s / %s) rejeitada — contato não autorizado\n",
			msg.From.DisplayName, msg.From.ID, msg.From.Username)

		// Emite evento para o frontend oferecer autorização manual
		if g.emitEvent != nil {
			g.emitEvent("messaging:contact_blocked", map[string]any{
				"channel":     msg.Channel,
				"displayName": msg.From.DisplayName,
				"contactId":   msg.From.ID,
				"username":    msg.From.Username,
			})
		}
		return
	}

	// 2. Busca (ou cria) a conversa dedicada para este canal+contato.
	//    Cada contato externo tem sua própria conversa — não compartilha com a aba ativa.
	conv, created, err := database.FindOrCreateChannelConversation(
		msg.Channel, msg.From.ID, msg.From.DisplayName,
	)
	if err != nil {
		fmt.Printf("[Gateway] Erro ao buscar/criar conversa para %s/%s: %v\n", msg.Channel, msg.From.ID, err)
		return
	}
	conversationID := conv.ID

	if created {
		fmt.Printf("[Gateway] Nova conversa %d criada para %s via %s\n", conversationID, msg.From.DisplayName, msg.Channel)
	}
	fmt.Printf("[Gateway] Mensagem de %s via %s → conversa %d\n",
		msg.From.DisplayName, msg.Channel, conversationID)

	// 3. Garante que existe uma aba para essa conversa no Wails
	channelIcons := map[string]string{"signal": "📡", "telegram": "✈️"}
	icon := channelIcons[msg.Channel]
	if icon == "" {
		icon = "💬"
	}
	tabTitle := fmt.Sprintf("[%s] %s", msg.Channel, msg.From.DisplayName)
	tab, tabCreated, tabErr := database.FindOrCreateTabForConversation(conversationID, tabTitle, icon)

	var tabID uint
	if tabErr == nil {
		tabID = tab.ID
		if tabCreated {
			fmt.Printf("[Gateway] Nova aba %d criada para conversa %d\n", tabID, conversationID)
		}
	} else {
		fmt.Printf("[Gateway] Erro ao criar aba para conversa %d: %v\n", conversationID, tabErr)
	}

	// 4. Converte attachments em media JSON (mesmo formato que o frontend)
	mediaJSON := ""
	if len(msg.Attachments) > 0 {
		mediaJSON = attachmentsToMediaJSON(msg.Attachments)
		fmt.Printf("[Gateway] %d attachment(s) convertidos para media JSON\n", len(msg.Attachments))
	}

	// 5. Emite evento para o frontend (com conversationID + info da aba)
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
			"tabId":           tabID,
			"tabCreated":      tabCreated,
			"tabTitle":        tabTitle,
			"tabIcon":         icon,
			"hasAttachments":  hasAttachments,
			"audioOnly":       msg.IsAudioOnly(),
		})
	}

	// 6. Registra callback no Notifier para capturar a resposta e reenviar ao mensageiro.
	//    Se a mensagem original era audio-only, o callback também sintetizará TTS.
	audioOnly := msg.IsAudioOnly()
	g.notifier.Register(conversationID, ResponseCallback{
		Channel:   msg.Channel,
		ChatID:    msg.From.ID,
		AudioOnly: audioOnly,
		Callback: func(response string) {
			g.mu.RLock()
			messenger, ok := g.messengers[msg.Channel]
			g.mu.RUnlock()

			if !ok {
				fmt.Printf("[Gateway] Messenger '%s' não encontrado para resposta\n", msg.Channel)
				return
			}

			outMsg := OutgoingMessage{
				ChatID:           msg.From.ID,
				Text:             response,
				ReplyToMessageID: msg.ID,
			}

			// Se a mensagem original era áudio e temos TTS, sintetiza e envia áudio
			if audioOnly && g.synthesizeTTS != nil {
				audioData, err := g.synthesizeTTS(response)
				if err == nil && len(audioData) > 0 {
					outMsg.Attachments = []Attachment{{
						Filename: "resposta.mp3",
						MIMEType: "audio/mpeg",
						Data:     audioData,
					}}
					// Em modo áudio, não envia texto (só o áudio)
					outMsg.Text = ""
					fmt.Printf("[Gateway] Resposta TTS gerada (%d bytes) para %s\n", len(audioData), msg.From.DisplayName)
				} else if err != nil {
					fmt.Printf("[Gateway] Erro ao gerar TTS: %v (enviando texto)\n", err)
				}
			}

			err := messenger.Send(ctx, outMsg)
			if err != nil {
				fmt.Printf("[Gateway] Erro ao enviar resposta via %s: %v\n", msg.Channel, err)
			} else {
				fmt.Printf("[Gateway] Resposta enviada via %s para %s\n", msg.Channel, msg.From.DisplayName)
			}
		},
	})

	// 7. Chama o mesmo SendMessage que o Wails usa (com o conversationID dedicado)
	params := llm.ChatParams{}
	_, err = g.sendMessage(conversationID, msg.Text, mediaJSON, params, msg.Channel)
	if err != nil {
		fmt.Printf("[Gateway] Erro ao processar mensagem: %v\n", err)
		g.mu.RLock()
		messenger, ok := g.messengers[msg.Channel]
		g.mu.RUnlock()
		if ok {
			messenger.Send(ctx, OutgoingMessage{
				ChatID: msg.From.ID,
				Text:   fmt.Sprintf("Erro ao processar mensagem: %v", err),
			})
		}
	}
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
		return ""
	}
	return string(data)
}

// getChannelConfig retorna a configuração de um canal específico.
func (g *Gateway) getChannelConfig(channel string) *config.ChannelConfig {
	if g.config == nil {
		return nil
	}
	switch channel {
	case "telegram":
		return g.config.Telegram
	case "signal":
		return g.config.Signal
	default:
		return nil
	}
}
