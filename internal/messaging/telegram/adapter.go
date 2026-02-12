package telegram

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"assistente/internal/messaging"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramAdapter implementa messaging.Messenger para o Telegram Bot API.
// Usa long polling (sem webhook) para receber mensagens.
type TelegramAdapter struct {
	bot     *tgbotapi.BotAPI
	token   string
	handler messaging.IncomingMessageHandler
	status  messaging.ConnectionStatus
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
}

// NewAdapter cria um novo adapter para o Telegram.
// O token deve ser obtido via @BotFather no Telegram.
func NewAdapter(token string) *TelegramAdapter {
	return &TelegramAdapter{
		token:  token,
		status: messaging.StatusDisconnected,
	}
}

// Name retorna o identificador da plataforma.
func (t *TelegramAdapter) Name() string {
	return "telegram"
}

// Connect inicia a conexão com o Telegram e começa o long polling.
func (t *TelegramAdapter) Connect(ctx context.Context) error {
	t.mu.Lock()
	t.status = messaging.StatusConnecting
	t.mu.Unlock()

	bot, err := tgbotapi.NewBotAPI(t.token)
	if err != nil {
		t.mu.Lock()
		t.status = messaging.StatusError
		t.mu.Unlock()
		return fmt.Errorf("erro ao conectar ao Telegram: %w", err)
	}

	t.mu.Lock()
	t.bot = bot
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.status = messaging.StatusConnected
	t.mu.Unlock()

	fmt.Printf("[Telegram] Conectado como @%s\n", bot.Self.UserName)

	// Inicia o loop de long polling em goroutine
	go t.pollLoop()

	return nil
}

// Disconnect encerra a conexão e para de receber mensagens.
func (t *TelegramAdapter) Disconnect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
	}
	if t.bot != nil {
		t.bot.StopReceivingUpdates()
	}
	t.status = messaging.StatusDisconnected
	fmt.Println("[Telegram] Desconectado")
	return nil
}

// Send envia uma mensagem para um chat do Telegram.
// Mensagens longas são automaticamente divididas.
func (t *TelegramAdapter) Send(ctx context.Context, msg messaging.OutgoingMessage) error {
	t.mu.RLock()
	bot := t.bot
	t.mu.RUnlock()

	if bot == nil {
		return fmt.Errorf("telegram não está conectado")
	}

	chatID, err := strconv.ParseInt(msg.ChatID, 10, 64)
	if err != nil {
		return fmt.Errorf("chatID inválido %q: %w", msg.ChatID, err)
	}

	// Divide mensagens longas
	parts := SplitMessage(msg.Text)

	for _, part := range parts {
		teleMsg := tgbotapi.NewMessage(chatID, part)
		teleMsg.ParseMode = "Markdown"

		// Se é reply, associa à mensagem original
		if msg.ReplyToMessageID != "" {
			if replyID, err := strconv.Atoi(msg.ReplyToMessageID); err == nil {
				teleMsg.ReplyToMessageID = replyID
			}
		}

		if _, err := bot.Send(teleMsg); err != nil {
			// Fallback: tenta sem Markdown (pode falhar por formatação inválida)
			teleMsg.ParseMode = ""
			if _, err2 := bot.Send(teleMsg); err2 != nil {
				return fmt.Errorf("erro ao enviar mensagem para %d: %w", chatID, err2)
			}
		}
	}

	return nil
}

// SetHandler define o callback para mensagens recebidas.
func (t *TelegramAdapter) SetHandler(handler messaging.IncomingMessageHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = handler
}

// Status retorna o estado atual da conexão.
func (t *TelegramAdapter) Status() messaging.ConnectionStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

// SendTypingAction envia indicador de "digitando..." para o chat.
func (t *TelegramAdapter) SendTypingAction(chatID int64) {
	t.mu.RLock()
	bot := t.bot
	t.mu.RUnlock()

	if bot == nil {
		return
	}

	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	bot.Send(action) //nolint:errcheck
}

// pollLoop é o loop de long polling que recebe mensagens do Telegram.
func (t *TelegramAdapter) pollLoop() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60 // 60 segundos de long polling

	t.mu.RLock()
	updates := t.bot.GetUpdatesChan(u)
	ctx := t.ctx
	t.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("[Telegram] Poll loop encerrado")
			return
		case update, ok := <-updates:
			if !ok {
				fmt.Println("[Telegram] Canal de updates fechado")
				return
			}
			t.handleUpdate(update)
		}
	}
}

// handleUpdate processa um update recebido do Telegram.
func (t *TelegramAdapter) handleUpdate(update tgbotapi.Update) {
	// Só processa mensagens de texto (ignora edições, callbacks, etc.)
	if update.Message == nil || update.Message.Text == "" {
		return
	}

	t.mu.RLock()
	handler := t.handler
	ctx := t.ctx
	t.mu.RUnlock()

	if handler == nil {
		return
	}

	msg := messaging.IncomingMessage{
		ID:        strconv.Itoa(update.Message.MessageID),
		From:      contactFromTelegram(update.Message),
		Text:      update.Message.Text,
		Timestamp: update.Message.Time(),
		Channel:   "telegram",
	}

	fmt.Printf("[Telegram] Mensagem de %s: %s\n", msg.From.DisplayName, truncate(msg.Text, 100))

	// Envia typing indicator
	t.SendTypingAction(update.Message.Chat.ID)

	// Processa em goroutine para não bloquear o polling
	go handler(ctx, msg)
}

// contactFromTelegram extrai um Contact a partir de uma mensagem do Telegram.
func contactFromTelegram(msg *tgbotapi.Message) messaging.Contact {
	displayName := msg.From.FirstName
	if msg.From.LastName != "" {
		displayName += " " + msg.From.LastName
	}

	return messaging.Contact{
		ID:          strconv.FormatInt(msg.Chat.ID, 10),
		DisplayName: displayName,
		Username:    msg.From.UserName,
	}
}

// truncate encurta uma string para exibição em logs.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
