package messaging

import (
	"context"
	"fmt"
	"sync"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// SendMessageFunc é a assinatura da função App.SendMessage (ou wrapper).
// Recebe conversationID (0=criar nova), conteúdo, mídia, params e source.
// Retorna o conversationID usado.
type SendMessageFunc func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error)

// emitFunc é a callback para emitir eventos Wails.
type emitFunc func(event string, data any)

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
	mu          sync.RWMutex
	messengers  map[string]Messenger
	notifier    *ResponseNotifier
	config      *Config
	sendMessage SendMessageFunc
	emitEvent   emitFunc
}

// NewGateway cria um novo Gateway de mensageria.
func NewGateway(
	notifier *ResponseNotifier,
	config *Config,
	sendMessage SendMessageFunc,
	emitEvent emitFunc,
) *Gateway {
	return &Gateway{
		messengers:  make(map[string]Messenger),
		notifier:    notifier,
		config:      config,
		sendMessage: sendMessage,
		emitEvent:   emitEvent,
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
	if !IsContactAllowed(channelConfig, msg.From.ID) {
		fmt.Printf("[Gateway] Mensagem de %s (%s) rejeitada — contato não autorizado\n",
			msg.From.DisplayName, msg.From.ID)
		return
	}

	// 2. Pega a conversa ativa do Wails (ou 0 para criar nova)
	conversationID := g.getActiveConversationID()
	fmt.Printf("[Gateway] Mensagem de %s via %s → conversa %d\n",
		msg.From.DisplayName, msg.Channel, conversationID)

	// 3. Emite evento para o frontend (notificação visual)
	if g.emitEvent != nil {
		g.emitEvent("messaging:incoming", map[string]any{
			"channel":     msg.Channel,
			"from":        msg.From.DisplayName,
			"fromId":      msg.From.ID,
			"text":        msg.Text,
			"messageId":   msg.ID,
		})
	}

	// 4. Registra callback no Notifier para capturar a resposta
	g.notifier.Register(conversationID, ResponseCallback{
		Channel: msg.Channel,
		ChatID:  msg.From.ID,
		Callback: func(response string) {
			g.mu.RLock()
			messenger, ok := g.messengers[msg.Channel]
			g.mu.RUnlock()

			if !ok {
				fmt.Printf("[Gateway] Messenger '%s' não encontrado para resposta\n", msg.Channel)
				return
			}

			err := messenger.Send(ctx, OutgoingMessage{
				ChatID:           msg.From.ID,
				Text:             response,
				ReplyToMessageID: msg.ID,
			})
			if err != nil {
				fmt.Printf("[Gateway] Erro ao enviar resposta via %s: %v\n", msg.Channel, err)
			} else {
				fmt.Printf("[Gateway] Resposta enviada via %s para %s\n", msg.Channel, msg.From.DisplayName)
			}
		},
	})

	// 5. Chama o mesmo SendMessage que o Wails usa
	params := llm.ChatParams{}
	resultConvID, err := g.sendMessage(conversationID, msg.Text, "", params, msg.Channel)
	if err != nil {
		fmt.Printf("[Gateway] Erro ao processar mensagem: %v\n", err)
		// Tenta notificar o usuário do erro
		g.mu.RLock()
		messenger, ok := g.messengers[msg.Channel]
		g.mu.RUnlock()
		if ok {
			messenger.Send(ctx, OutgoingMessage{
				ChatID: msg.From.ID,
				Text:   fmt.Sprintf("Erro ao processar mensagem: %v", err),
			})
		}
		return
	}

	// Se a conversa foi criada (conversationID era 0), re-registra o callback
	// com o ID real da conversa
	if conversationID == 0 && resultConvID > 0 {
		fmt.Printf("[Gateway] Conversa criada: %d\n", resultConvID)
		// O notifier já foi registrado com 0, mas o saveAndFinish vai usar resultConvID.
		// Precisamos re-registrar com o ID correto.
		g.notifier.Register(resultConvID, ResponseCallback{
			Channel: msg.Channel,
			ChatID:  msg.From.ID,
			Callback: func(response string) {
				g.mu.RLock()
				messenger, ok := g.messengers[msg.Channel]
				g.mu.RUnlock()

				if !ok {
					return
				}
				messenger.Send(ctx, OutgoingMessage{
					ChatID: msg.From.ID,
					Text:   response,
				})
			},
		})
	}
}

// getActiveConversationID retorna o ID da conversa ativa no Wails.
// Se não há conversa ativa, retorna 0 (SendMessage criará uma nova).
func (g *Gateway) getActiveConversationID() uint {
	tab, err := database.GetActiveTab()
	if err != nil || tab == nil || tab.ConversationID == nil {
		return 0
	}
	return *tab.ConversationID
}

// getChannelConfig retorna a configuração de um canal específico.
func (g *Gateway) getChannelConfig(channel string) *ChannelConfig {
	if g.config == nil {
		return nil
	}
	switch channel {
	case "telegram":
		return g.config.Telegram
	default:
		return nil
	}
}
