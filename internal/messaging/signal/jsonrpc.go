package signal

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

// ==================== JSON-RPC 2.0 Types ====================

// Contadores globais para gerar IDs únicos de requisições
var requestIDCounter atomic.Int64

// nextRequestID gera um ID único para cada requisição JSON-RPC.
func nextRequestID() string {
	return fmt.Sprintf("%d", requestIDCounter.Add(1))
}

// jsonRPCRequest é uma requisição JSON-RPC 2.0 enviada via stdin ao signal-cli.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      string      `json:"id"`
}

// jsonRPCResponse é uma resposta JSON-RPC 2.0 recebida via stdout do signal-cli.
// Pode ser uma resposta a uma requisição (tem ID) ou uma notificação (sem ID).
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	ID      *string         `json:"id,omitempty"` // nil = notificação (mensagem recebida)
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCError representa um erro JSON-RPC 2.0.
type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// IsNotification retorna true se a resposta é uma notificação (mensagem recebida)
// e não uma resposta a uma requisição.
func (r *jsonRPCResponse) IsNotification() bool {
	return r.ID == nil && r.Method != ""
}

// ==================== Signal-specific Types ====================

// sendParams são os parâmetros para o método "send" do signal-cli.
type sendParams struct {
	Recipient []string `json:"recipient,omitempty"` // Número(s) de telefone destino
	GroupID   string   `json:"groupId,omitempty"`   // ID do grupo (alternativa a recipient)
	Message   string   `json:"message"`             // Texto da mensagem
}

// sendResult é o resultado do método "send".
type sendResult struct {
	Timestamp int64 `json:"timestamp"`
}

// receiveNotification é o payload de uma notificação "receive" do signal-cli.
// Mensagens recebidas chegam como notificações JSON-RPC (sem campo "id").
type receiveNotification struct {
	Envelope signalEnvelope `json:"envelope"`
	Account  string         `json:"account,omitempty"` // Presente em modo multi-account
}

// signalEnvelope é o envelope de uma mensagem Signal.
type signalEnvelope struct {
	Source       string `json:"source"`       // Número do remetente
	SourceNumber string `json:"sourceNumber"` // Número do remetente (redundante)
	SourceUUID   string `json:"sourceUuid"`   // UUID do remetente
	SourceName   string `json:"sourceName"`   // Nome do remetente
	SourceDevice int    `json:"sourceDevice"` // ID do dispositivo

	Timestamp   int64        `json:"timestamp"`
	DataMessage *dataMessage `json:"dataMessage,omitempty"` // Mensagem de dados (texto, mídia, etc.)
	SyncMessage *syncMessage `json:"syncMessage,omitempty"` // Mensagem de sync (quando enviamos de outro dispositivo)
}

// dataMessage é uma mensagem de texto/dados do Signal.
type dataMessage struct {
	Timestamp        int64  `json:"timestamp"`
	Message          string `json:"message"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
	ViewOnce         bool   `json:"viewOnce"`
}

// syncMessage é uma mensagem de sincronização (quando enviamos de outro dispositivo).
type syncMessage struct {
	SentMessage *sentMessage `json:"sentMessage,omitempty"`
}

// sentMessage é uma mensagem enviada por nós a partir de outro dispositivo.
type sentMessage struct {
	Destination       string `json:"destination"`
	DestinationNumber string `json:"destinationNumber"`
	Timestamp         int64  `json:"timestamp"`
	Message           string `json:"message"`
}

// ==================== Helper Functions ====================

// newSendRequest cria uma requisição JSON-RPC para enviar uma mensagem.
func newSendRequest(recipient, message string) jsonRPCRequest {
	return jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "send",
		Params: sendParams{
			Recipient: []string{recipient},
			Message:   message,
		},
		ID: nextRequestID(),
	}
}

// timestampToTime converte um timestamp do Signal (milissegundos) em time.Time.
func timestampToTime(ts int64) time.Time {
	return time.UnixMilli(ts)
}
