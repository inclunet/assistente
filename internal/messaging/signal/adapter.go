package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"assistente/internal/messaging"

	"github.com/gorilla/websocket"
)

// SignalAdapter implementa messaging.Messenger para o Signal via signal-cli-rest-api HTTP.
// Conecta a uma instância da signal-cli-rest-api (bbernhard) rodando como serviço
// (Docker, Kubernetes, etc.) e comunica via REST API + WebSocket.
//
// Endpoints usados:
//   - POST /v2/send — envia mensagens
//   - GET  /v1/receive/{number} — WebSocket para receber mensagens em tempo real
//   - GET  /v1/about — health check
type SignalAdapter struct {
	baseURL string // URL base da API (ex: "http://signal-api:8080")
	account string // número de telefone da conta Signal (ex: "+5511999999999")

	handler messaging.IncomingMessageHandler
	status  messaging.ConnectionStatus

	wsConn *websocket.Conn
	client *http.Client

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// NewAdapter cria um novo adapter para o Signal via REST API.
// baseURL é a URL base da signal-cli-rest-api (ex: "http://signal-api:8080").
// account é o número de telefone vinculado (ex: "+5511999999999").
func NewAdapter(baseURL, account string) *SignalAdapter {
	// Remove trailing slash
	baseURL = strings.TrimRight(baseURL, "/")

	return &SignalAdapter{
		baseURL: baseURL,
		account: account,
		status:  messaging.StatusDisconnected,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name retorna o identificador da plataforma.
func (s *SignalAdapter) Name() string {
	return "signal"
}

// Connect verifica a conexão com a API e inicia o WebSocket para receber mensagens.
func (s *SignalAdapter) Connect(ctx context.Context) error {
	s.mu.Lock()
	s.status = messaging.StatusConnecting
	s.mu.Unlock()

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Verifica se a API está acessível
	if err := s.healthCheck(); err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("signal-cli-rest-api não acessível em %s: %w", s.baseURL, err)
	}

	// Conecta o WebSocket para receber mensagens
	if err := s.connectWebSocket(); err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("erro ao conectar WebSocket Signal: %w", err)
	}

	s.setStatus(messaging.StatusConnected)
	fmt.Printf("[Signal] Conectado à API %s (account=%s)\n", s.baseURL, s.account)

	// Inicia o loop de leitura do WebSocket
	go s.wsReadLoop()

	return nil
}

// Disconnect encerra a conexão WebSocket.
func (s *SignalAdapter) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	if s.wsConn != nil {
		s.wsConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		s.wsConn.Close()
		s.wsConn = nil
	}
	s.status = messaging.StatusDisconnected
	fmt.Println("[Signal] Desconectado")
	return nil
}

// Send envia uma mensagem de texto via POST /v2/send.
func (s *SignalAdapter) Send(ctx context.Context, msg messaging.OutgoingMessage) error {
	payload := sendMessageV2{
		Message:    msg.Text,
		Number:     s.account,
		Recipients: []string{msg.ChatID},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("erro ao serializar mensagem: %w", err)
	}

	reqURL := s.baseURL + "/v2/send"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem Signal para %s: %w", msg.ChatID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("signal-cli-rest-api retornou %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SetHandler define o callback para mensagens recebidas.
func (s *SignalAdapter) SetHandler(handler messaging.IncomingMessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

// Status retorna o estado atual da conexão.
func (s *SignalAdapter) Status() messaging.ConnectionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// ==================== Internal Methods ====================

// healthCheck verifica se a API está acessível via GET /v1/about.
func (s *SignalAdapter) healthCheck() error {
	reqURL := s.baseURL + "/v1/about"
	req, err := http.NewRequestWithContext(s.ctx, "GET", reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	fmt.Printf("[Signal] Health check OK (%s)\n", reqURL)
	return nil
}

// connectWebSocket conecta ao endpoint WebSocket /v1/receive/{number}.
func (s *SignalAdapter) connectWebSocket() error {
	// Converte HTTP URL para WS URL
	wsURL := s.baseURL + "/v1/receive/" + url.PathEscape(s.account)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(s.ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("erro ao conectar WebSocket %s: %w", wsURL, err)
	}

	s.mu.Lock()
	s.wsConn = conn
	s.mu.Unlock()

	fmt.Printf("[Signal] WebSocket conectado: %s\n", wsURL)
	return nil
}

// wsReadLoop lê mensagens do WebSocket continuamente.
func (s *SignalAdapter) wsReadLoop() {
	defer func() {
		s.mu.RLock()
		conn := s.wsConn
		s.mu.RUnlock()
		if conn != nil {
			conn.Close()
		}
	}()

	for {
		select {
		case <-s.ctx.Done():
			fmt.Println("[Signal] WebSocket read loop encerrado (contexto cancelado)")
			return
		default:
		}

		s.mu.RLock()
		conn := s.wsConn
		s.mu.RUnlock()

		if conn == nil {
			fmt.Println("[Signal] WebSocket desconectado, tentando reconectar...")
			s.reconnectWebSocket()
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				fmt.Println("[Signal] WebSocket fechado normalmente")
				return
			}
			if s.ctx.Err() != nil {
				return // Contexto cancelado
			}
			fmt.Printf("[Signal] Erro ao ler WebSocket: %v\n", err)
			s.reconnectWebSocket()
			continue
		}

		s.handleWSMessage(message)
	}
}

// handleWSMessage processa uma mensagem recebida via WebSocket.
func (s *SignalAdapter) handleWSMessage(data []byte) {
	var envelope wsEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		fmt.Printf("[Signal] Erro ao parsear mensagem WS: %v (dados: %s)\n", err, truncate(string(data), 200))
		return
	}

	if envelope.Envelope == nil {
		return
	}

	env := envelope.Envelope

	// Extrai texto da mensagem (dataMessage)
	var text string
	if env.DataMessage != nil && env.DataMessage.Message != "" {
		text = env.DataMessage.Message
	} else if env.SyncMessage != nil && env.SyncMessage.SentMessage != nil {
		// Mensagens de sync são enviadas por nós de outro dispositivo — ignoramos
		return
	}

	if text == "" {
		return // Ignora mensagens sem texto (delivery receipts, typing indicators, etc.)
	}

	s.mu.RLock()
	handler := s.handler
	ctx := s.ctx
	s.mu.RUnlock()

	if handler == nil {
		return
	}

	// Usa sourceNumber como ID do contato
	contactID := env.SourceNumber
	if contactID == "" {
		contactID = env.Source
	}

	msg := messaging.IncomingMessage{
		ID:        fmt.Sprintf("%d", env.Timestamp),
		From: messaging.Contact{
			ID:          contactID,
			DisplayName: env.SourceName,
			Username:    contactID,
		},
		Text:      text,
		Timestamp: timestampToTime(env.Timestamp),
		Channel:   "signal",
	}

	fmt.Printf("[Signal] Mensagem de %s (%s): %s\n", msg.From.DisplayName, msg.From.ID, truncate(msg.Text, 100))

	// Processa em goroutine para não bloquear o reader loop
	go handler(ctx, msg)
}

// reconnectWebSocket tenta reconectar ao WebSocket com backoff exponencial.
func (s *SignalAdapter) reconnectWebSocket() {
	s.mu.Lock()
	if s.wsConn != nil {
		s.wsConn.Close()
		s.wsConn = nil
	}
	s.mu.Unlock()

	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(backoff):
		}

		fmt.Printf("[Signal] Tentando reconectar WebSocket (backoff=%s)...\n", backoff)

		if err := s.connectWebSocket(); err != nil {
			fmt.Printf("[Signal] Reconexão falhou: %v\n", err)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		fmt.Println("[Signal] WebSocket reconectado com sucesso")
		s.setStatus(messaging.StatusConnected)
		return
	}
}

// setStatus atualiza o status da conexão de forma thread-safe.
func (s *SignalAdapter) setStatus(status messaging.ConnectionStatus) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

// truncate encurta uma string para exibição em logs.
func truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen] + "..."
}
