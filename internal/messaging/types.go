package messaging

import (
	"context"
	"strings"
	"time"
)

// Messenger é a interface que cada plataforma de mensageria (Telegram, Signal, etc.) implementa.
type Messenger interface {
	// Name retorna o identificador da plataforma (ex: "telegram", "signal").
	Name() string

	// Connect inicia a conexão com a plataforma e começa a receber mensagens.
	// Bloqueia até a conexão ser estabelecida ou ctx ser cancelado.
	Connect(ctx context.Context) error

	// Disconnect encerra a conexão e para de receber mensagens.
	Disconnect() error

	// Send envia uma mensagem para um contato/chat.
	Send(ctx context.Context, msg OutgoingMessage) error

	// SetHandler define o callback chamado quando uma mensagem chega.
	// Deve ser chamado antes de Connect.
	SetHandler(handler IncomingMessageHandler)

	// Status retorna o estado atual da conexão.
	Status() ConnectionStatus
}

// IncomingMessageHandler é chamado quando uma mensagem chega de qualquer messenger.
type IncomingMessageHandler func(ctx context.Context, msg IncomingMessage)

// IncomingMessage representa uma mensagem recebida de um mensageiro externo.
type IncomingMessage struct {
	// ID é o identificador da mensagem na plataforma de origem.
	ID string

	// From é o contato que enviou a mensagem.
	From Contact

	// Text é o conteúdo textual da mensagem.
	Text string

	// Attachments são os anexos da mensagem (áudio, imagens, documentos, etc.).
	Attachments []Attachment

	// Timestamp é o momento em que a mensagem foi enviada.
	Timestamp time.Time

	// Channel identifica a plataforma de origem ("telegram", "signal", etc.).
	Channel string

	// ReplyChatID é o destino de outbound quando diferente de From.ID
	// (ex.: Slack: From.ID = user, ReplyChatID = channel). Vazio = usar From.ID.
	ReplyChatID string
}

// OutboundChatID retorna o chatID a usar ao responder esta mensagem.
func (m IncomingMessage) OutboundChatID() string {
	if reply := strings.TrimSpace(m.ReplyChatID); reply != "" {
		return reply
	}
	return m.From.ID
}

// OutgoingMessage representa uma mensagem a ser enviada via mensageiro.
type OutgoingMessage struct {
	// ChatID é o identificador do chat/contato de destino.
	ChatID string

	// Text é o conteúdo textual da mensagem.
	Text string

	// Attachments são os anexos a serem enviados (áudio TTS, imagens, etc.).
	Attachments []Attachment

	// ReplyToMessageID é opcional — ID da mensagem a ser respondida.
	ReplyToMessageID string

	// IdempotencyKey é opcional — chave estável do turno (ex.: TraceID do
	// pending) para deduplicar reenvios na plataforma quando ela expõe
	// suporte nativo (Slack: client_msg_id). Vazio = sem dedup no adapter.
	IdempotencyKey string
}

// Attachment representa um anexo de mensagem (áudio, imagem, documento, etc.).
type Attachment struct {
	// Filename é o nome do arquivo (ex: "audio.ogg", "foto.jpg", "relatorio.pdf").
	Filename string

	// MIMEType é o tipo MIME do arquivo (ex: "audio/ogg", "image/jpeg", "application/pdf").
	MIMEType string

	// Data é o conteúdo do arquivo em bytes (já baixado).
	Data []byte

	// Size é o tamanho do arquivo em bytes (pode ser 0 se desconhecido).
	Size int64
}

// IsAudio retorna true se o attachment é um arquivo de áudio.
func (a *Attachment) IsAudio() bool {
	return strings.HasPrefix(a.MIMEType, "audio/")
}

// IsImage retorna true se o attachment é uma imagem.
func (a *Attachment) IsImage() bool {
	return strings.HasPrefix(a.MIMEType, "image/")
}

// IsDocument retorna true se o attachment é um documento (PDF, DOCX, etc.).
func (a *Attachment) IsDocument() bool {
	return !a.IsAudio() && !a.IsImage() && !strings.HasPrefix(a.MIMEType, "video/")
}

// IsVideo retorna true se o attachment é um vídeo.
func (a *Attachment) IsVideo() bool {
	return strings.HasPrefix(a.MIMEType, "video/")
}

// HasAudio retorna true se a mensagem contém pelo menos um anexo de áudio.
func (m *IncomingMessage) HasAudio() bool {
	for _, a := range m.Attachments {
		if a.IsAudio() {
			return true
		}
	}
	return false
}

// IsAudioOnly retorna true se a mensagem contém apenas áudio (sem texto).
// Usado para decidir se a resposta deve ser em áudio também.
func (m *IncomingMessage) IsAudioOnly() bool {
	return m.Text == "" && len(m.Attachments) > 0 && m.HasAudio()
}

// Contact representa um contato em um mensageiro.
type Contact struct {
	// ID é o identificador único na plataforma (chat_id do Telegram, número do Signal, etc.).
	ID string

	// DisplayName é o nome de exibição do contato.
	DisplayName string

	// Username é o nome de usuário na plataforma (ex: @usuario no Telegram).
	Username string
}

// ConnectionStatus representa o estado da conexão com um mensageiro.
type ConnectionStatus string

const (
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusConnecting   ConnectionStatus = "connecting"
	StatusConnected    ConnectionStatus = "connected"
	StatusError        ConnectionStatus = "error"
)
