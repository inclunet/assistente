package signal

import "time"

// ==================== signal-cli-rest-api Types ====================

// sendMessageV2 é o payload do POST /v2/send da signal-cli-rest-api.
type sendMessageV2 struct {
	Message    string   `json:"message"`              // Texto da mensagem
	Number     string   `json:"number"`               // Número remetente (conta Signal)
	Recipients []string `json:"recipients"`            // Número(s) destinatário(s)
}

// sendMessageResponse é a resposta do POST /v2/send.
type sendMessageResponse struct {
	Timestamp string `json:"timestamp,omitempty"`
}

// apiError é um erro retornado pela REST API.
type apiError struct {
	Error string `json:"error"`
}

// ==================== WebSocket Receive Types ====================

// wsEnvelope é o envelope de uma mensagem recebida via WebSocket (/v1/receive).
// Formato baseado na documentação do signal-cli-rest-api.
type wsEnvelope struct {
	Envelope *signalEnvelope `json:"envelope,omitempty"`
	Account  string          `json:"account,omitempty"`
}

// signalEnvelope é o envelope de uma mensagem Signal.
type signalEnvelope struct {
	Source       string `json:"source"`
	SourceNumber string `json:"sourceNumber"`
	SourceUUID   string `json:"sourceUuid"`
	SourceName   string `json:"sourceName"`
	SourceDevice int    `json:"sourceDevice"`

	Timestamp   int64        `json:"timestamp"`
	DataMessage *dataMessage `json:"dataMessage,omitempty"`
	SyncMessage *syncMessage `json:"syncMessage,omitempty"`
}

// dataMessage é uma mensagem de texto/dados.
type dataMessage struct {
	Timestamp        int64  `json:"timestamp"`
	Message          string `json:"message"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
	ViewOnce         bool   `json:"viewOnce"`
}

// syncMessage é uma mensagem de sincronização (enviada por outro dispositivo nosso).
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

// timestampToTime converte um timestamp do Signal (milissegundos) em time.Time.
func timestampToTime(ts int64) time.Time {
	return time.UnixMilli(ts)
}
