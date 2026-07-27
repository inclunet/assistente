package telegram

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
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

	logging.Infof(ctx, "messaging.telegram.adapter", "[Telegram] Conectado como @%s", bot.Self.UserName)

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
	logging.Println(context.Background(), "messaging.telegram.adapter", "[Telegram] Desconectado")
	return nil
}

// Send envia uma mensagem (texto e/ou attachments) para um chat do Telegram.
// Mensagens longas são automaticamente divididas.
//
// IdempotencyKey é no-op: a Bot API do Telegram não expõe chave de dedup
// nativa em sendMessage/sendDocument — a janela residual Send→MarkDelivered
// (M14) permanece at-least-once neste canal.
func (t *TelegramAdapter) Send(ctx context.Context, msg messaging.OutgoingMessage) error {
	_ = msg.IdempotencyKey // API sem chave nativa — ver comentário acima.

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

	// Envia attachments primeiro (se houver)
	for _, att := range msg.Attachments {
		if err := t.sendAttachment(chatID, att, msg.ReplyToMessageID); err != nil {
			logging.Errorf(ctx, "messaging.telegram.adapter", "[Telegram] Erro ao enviar attachment %s: %v", att.Filename, err)
		}
	}

	// Envia texto (se houver)
	if msg.Text != "" {
		parts := SplitMessage(msg.Text)
		for _, part := range parts {
			teleMsg := tgbotapi.NewMessage(chatID, part)
			teleMsg.ParseMode = "Markdown"

			if msg.ReplyToMessageID != "" {
				if replyID, err := strconv.Atoi(msg.ReplyToMessageID); err == nil {
					teleMsg.ReplyToMessageID = replyID
				}
			}

			if _, err := bot.Send(teleMsg); err != nil {
				teleMsg.ParseMode = ""
				if _, err2 := bot.Send(teleMsg); err2 != nil {
					return fmt.Errorf("erro ao enviar mensagem para %d: %w", chatID, err2)
				}
			}
		}
	}

	return nil
}

// sendAttachment envia um attachment individual via Telegram Bot API.
func (t *TelegramAdapter) sendAttachment(chatID int64, att messaging.Attachment, replyTo string) error {
	fileBytes := tgbotapi.FileBytes{Name: att.Filename, Bytes: att.Data}

	var chattable tgbotapi.Chattable

	if att.IsAudio() {
		// Áudio: envia como voice (aparece como player inline no Telegram)
		voice := tgbotapi.NewVoice(chatID, fileBytes)
		if replyTo != "" {
			if replyID, err := strconv.Atoi(replyTo); err == nil {
				voice.ReplyToMessageID = replyID
			}
		}
		chattable = voice
	} else if att.IsImage() {
		photo := tgbotapi.NewPhoto(chatID, fileBytes)
		if replyTo != "" {
			if replyID, err := strconv.Atoi(replyTo); err == nil {
				photo.ReplyToMessageID = replyID
			}
		}
		chattable = photo
	} else if att.IsVideo() {
		video := tgbotapi.NewVideo(chatID, fileBytes)
		if replyTo != "" {
			if replyID, err := strconv.Atoi(replyTo); err == nil {
				video.ReplyToMessageID = replyID
			}
		}
		chattable = video
	} else {
		doc := tgbotapi.NewDocument(chatID, fileBytes)
		if replyTo != "" {
			if replyID, err := strconv.Atoi(replyTo); err == nil {
				doc.ReplyToMessageID = replyID
			}
		}
		chattable = doc
	}

	_, err := t.bot.Send(chattable)
	return err
}

// downloadFile baixa um arquivo do Telegram via Bot API (getFile + download).
func (t *TelegramAdapter) downloadFile(fileID string) ([]byte, string, error) {
	t.mu.RLock()
	bot := t.bot
	t.mu.RUnlock()

	if bot == nil {
		return nil, "", fmt.Errorf("telegram não está conectado")
	}

	fileConfig := tgbotapi.FileConfig{FileID: fileID}
	file, err := bot.GetFile(fileConfig)
	if err != nil {
		return nil, "", fmt.Errorf("erro ao obter info do arquivo: %w", err)
	}

	fileURL := file.Link(bot.Token)

	resp, err := http.Get(fileURL) //nolint:gosec
	if err != nil {
		return nil, "", fmt.Errorf("erro ao baixar arquivo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("erro ao ler arquivo: %w", err)
	}

	// Tenta detectar MIME pelo Content-Type ou extensão
	mime := resp.Header.Get("Content-Type")
	if mime == "" || mime == "application/octet-stream" {
		mime = mimeFromFilename(file.FilePath)
	}

	return data, mime, nil
}

// mimeFromFilename detecta o MIME type a partir da extensão do arquivo.
func mimeFromFilename(filename string) string {
	ext := path.Ext(filename)
	mimes := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
		".gif": "image/gif", ".webp": "image/webp",
		".mp3": "audio/mpeg", ".ogg": "audio/ogg", ".oga": "audio/ogg",
		".wav": "audio/wav", ".aac": "audio/aac", ".m4a": "audio/mp4",
		".mp4": "video/mp4", ".webm": "video/webm",
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	}
	if m, ok := mimes[ext]; ok {
		return m
	}
	return "application/octet-stream"
}

// firstNonEmpty retorna o primeiro valor não vazio da lista.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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
			return
		case update, ok := <-updates:
			if !ok {
				logging.Println(context.Background(), "messaging.telegram.adapter", "[Telegram] Canal de updates fechado")
				return
			}
			t.handleUpdate(update)
		}
	}
}

// handleUpdate processa um update recebido do Telegram.
func (t *TelegramAdapter) handleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	m := update.Message

	// Extrai texto (pode ser text ou caption de uma mídia)
	text := m.Text
	if text == "" {
		text = m.Caption
	}

	// Extrai attachments (fotos, áudio, voz, documentos, vídeo)
	var attachments []messaging.Attachment

	// Voz (voice messages — .ogg opus)
	if m.Voice != nil {
		if data, mime, err := t.downloadFile(m.Voice.FileID); err == nil {
			attachments = append(attachments, messaging.Attachment{
				Filename: "voice.ogg",
				MIMEType: firstNonEmpty(m.Voice.MimeType, mime, "audio/ogg"),
				Data:     data,
				Size:     int64(m.Voice.FileSize),
			})
		} else {
			logging.Errorf(context.Background(), "messaging.telegram.adapter", "[Telegram] Erro ao baixar voice: %v", err)
		}
	}

	// Áudio (arquivos de áudio — mp3, etc.)
	if m.Audio != nil {
		filename := m.Audio.FileName
		if filename == "" {
			filename = "audio.mp3"
		}
		if data, mime, err := t.downloadFile(m.Audio.FileID); err == nil {
			attachments = append(attachments, messaging.Attachment{
				Filename: filename,
				MIMEType: firstNonEmpty(m.Audio.MimeType, mime, "audio/mpeg"),
				Data:     data,
				Size:     int64(m.Audio.FileSize),
			})
		} else {
			logging.Errorf(context.Background(), "messaging.telegram.adapter", "[Telegram] Erro ao baixar audio: %v", err)
		}
	}

	// Foto (pega a maior resolução — último item do array)
	if len(m.Photo) > 0 {
		photo := m.Photo[len(m.Photo)-1] // Maior resolução
		if data, _, err := t.downloadFile(photo.FileID); err == nil {
			attachments = append(attachments, messaging.Attachment{
				Filename: "photo.jpg",
				MIMEType: "image/jpeg",
				Data:     data,
				Size:     int64(photo.FileSize),
			})
		} else {
			logging.Errorf(context.Background(), "messaging.telegram.adapter", "[Telegram] Erro ao baixar foto: %v", err)
		}
	}

	// Documento (PDF, DOCX, etc.)
	if m.Document != nil {
		filename := m.Document.FileName
		if filename == "" {
			filename = "document"
		}
		if data, mime, err := t.downloadFile(m.Document.FileID); err == nil {
			attachments = append(attachments, messaging.Attachment{
				Filename: filename,
				MIMEType: firstNonEmpty(m.Document.MimeType, mime, "application/octet-stream"),
				Data:     data,
				Size:     int64(m.Document.FileSize),
			})
		} else {
			logging.Errorf(context.Background(), "messaging.telegram.adapter", "[Telegram] Erro ao baixar documento: %v", err)
		}
	}

	// Vídeo
	if m.Video != nil {
		filename := m.Video.FileName
		if filename == "" {
			filename = "video.mp4"
		}
		if data, mime, err := t.downloadFile(m.Video.FileID); err == nil {
			attachments = append(attachments, messaging.Attachment{
				Filename: filename,
				MIMEType: firstNonEmpty(m.Video.MimeType, mime, "video/mp4"),
				Data:     data,
				Size:     int64(m.Video.FileSize),
			})
		} else {
			logging.Errorf(context.Background(), "messaging.telegram.adapter", "[Telegram] Erro ao baixar vídeo: %v", err)
		}
	}

	// Video note (mensagens circulares)
	if m.VideoNote != nil {
		if data, _, err := t.downloadFile(m.VideoNote.FileID); err == nil {
			attachments = append(attachments, messaging.Attachment{
				Filename: "videonote.mp4",
				MIMEType: "video/mp4",
				Data:     data,
				Size:     int64(m.VideoNote.FileSize),
			})
		} else {
			logging.Errorf(context.Background(), "messaging.telegram.adapter", "[Telegram] Erro ao baixar video note: %v", err)
		}
	}

	// Sticker (como imagem)
	if m.Sticker != nil && !m.Sticker.IsAnimated {
		if data, _, err := t.downloadFile(m.Sticker.FileID); err == nil {
			attachments = append(attachments, messaging.Attachment{
				Filename: "sticker.webp",
				MIMEType: "image/webp",
				Data:     data,
				Size:     int64(m.Sticker.FileSize),
			})
		}
	}

	// Ignora se não tem texto nem attachments
	if text == "" && len(attachments) == 0 {
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
		ID:          strconv.Itoa(m.MessageID),
		From:        contactFromTelegram(m),
		Text:        text,
		Attachments: attachments,
		Timestamp:   m.Time(),
		Channel:     "telegram",
	}

	// Envia typing indicator
	t.SendTypingAction(m.Chat.ID)

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
