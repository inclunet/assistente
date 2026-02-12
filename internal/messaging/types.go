package messaging

import (
	"context"
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

	// Timestamp é o momento em que a mensagem foi enviada.
	Timestamp time.Time

	// Channel identifica a plataforma de origem ("telegram", "signal", etc.).
	Channel string
}

// OutgoingMessage representa uma mensagem a ser enviada via mensageiro.
type OutgoingMessage struct {
	// ChatID é o identificador do chat/contato de destino.
	ChatID string

	// Text é o conteúdo textual da mensagem.
	Text string

	// ReplyToMessageID é opcional — ID da mensagem a ser respondida.
	ReplyToMessageID string
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
