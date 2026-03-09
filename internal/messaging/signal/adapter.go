package signal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
	"assistente/internal/messaging"

	"github.com/gorilla/websocket"
)

// base64Encode codifica bytes em base64 standard.
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// SignalAdapter implementa messaging.Messenger para o Signal via signal-cli-rest-api HTTP.
// Conecta a uma instância da signal-cli-rest-api (bbernhard) rodando como serviço
// (Docker, Kubernetes, etc.) e comunica via REST API.
//
// Endpoints usados:
//   - POST /v2/send — envia mensagens
//   - GET  /v1/receive/{number} — recebe mensagens (HTTP polling em modo native, WebSocket em json-rpc)
//   - GET  /v1/about — health check
type SignalAdapter struct {
	baseURL  string // URL base da API (ex: "http://signal-api:8080")
	account  string // número de telefone da conta Signal (ex: "+5511999999999")
	apiMode  string // modo da API: "native" ou "json-rpc"

	handler messaging.IncomingMessageHandler
	status  messaging.ConnectionStatus

	wsConn *websocket.Conn
	client *httpclient.Client

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// NewAdapter cria um novo adapter para o Signal via REST API.
// baseURL é a URL base da signal-cli-rest-api (ex: "http://signal-api:8080").
// account é o número de telefone vinculado (ex: "+5511999999999").
func NewAdapter(baseURL, account string, credMgr *credentials.Manager) *SignalAdapter {
	// Remove trailing slash
	baseURL = strings.TrimRight(baseURL, "/")

	if credMgr == nil {
		credMgr = credentials.NewManager(nil)
	}

	client := httpclient.New(&httpclient.Config{
		CredentialManager: credMgr,
		Timeout:           30 * time.Second,
	}, map[string]string{})

	return &SignalAdapter{
		baseURL: baseURL,
		account: account,
		status:  messaging.StatusDisconnected,
		client:  client,
	}
}

// Name retorna o identificador da plataforma.
func (s *SignalAdapter) Name() string {
	return "signal"
}

// Connect verifica a conexão com a API e inicia o recebimento de mensagens.
// Em modo "native", usa HTTP polling. Em modo "json-rpc", usa WebSocket.
func (s *SignalAdapter) Connect(ctx context.Context) error {
	s.mu.Lock()
	s.status = messaging.StatusConnecting
	s.mu.Unlock()

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Verifica a API e detecta o modo
	mode, err := s.healthCheckAndDetectMode()
	if err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("signal-cli-rest-api não acessível em %s: %w", s.baseURL, err)
	}
	s.apiMode = mode

	if mode == "json-rpc" {
		// Modo json-rpc: usa WebSocket para receber mensagens em tempo real
		if err := s.connectWebSocket(); err != nil {
			s.setStatus(messaging.StatusError)
			return fmt.Errorf("erro ao conectar WebSocket Signal: %w", err)
		}
		s.setStatus(messaging.StatusConnected)
		fmt.Printf("[Signal] Conectado via WebSocket à API %s (account=%s, mode=%s)\n", s.baseURL, maskIdentifier(s.account), mode)
		go s.wsReadLoop()
	} else {
		// Modo native: usa HTTP polling
		s.setStatus(messaging.StatusConnected)
		fmt.Printf("[Signal] Conectado via HTTP polling à API %s (account=%s, mode=%s)\n", s.baseURL, maskIdentifier(s.account), mode)
		go s.httpPollLoop()
	}

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

// Send envia uma mensagem (texto e/ou attachments) via POST /v2/send.
func (s *SignalAdapter) Send(ctx context.Context, msg messaging.OutgoingMessage) error {
	payload := sendMessageV2{
		Message:    msg.Text,
		Number:     s.account,
		Recipients: []string{msg.ChatID},
	}

	// Converte attachments para data URIs (formato da signal-cli-rest-api)
	for _, att := range msg.Attachments {
		dataURI := fmt.Sprintf("data:%s;base64,%s",
			att.MIMEType, base64Encode(att.Data))
		payload.Base64Attachments = append(payload.Base64Attachments, dataURI)
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

	resp, err := s.client.Do(s.ctx, req)
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

// downloadAttachment baixa um attachment da signal-cli-rest-api via GET /v1/attachments/{id}.
func (s *SignalAdapter) downloadAttachment(attachmentID string) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/v1/attachments/%s", s.baseURL, url.PathEscape(attachmentID))
	req, err := http.NewRequestWithContext(s.ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(s.ctx, req)
	if err != nil {
		return nil, fmt.Errorf("erro ao baixar attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API retornou %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler attachment: %w", err)
	}

	return data, nil
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

// healthCheckAndDetectMode verifica se a API está acessível e retorna o modo ("native" ou "json-rpc").
func (s *SignalAdapter) healthCheckAndDetectMode() (string, error) {
	reqURL := s.baseURL + "/v1/about"
	req, err := http.NewRequestWithContext(s.ctx, "GET", reqURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Do(s.ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var info map[string]interface{}
	mode := "native" // default
	if json.Unmarshal(body, &info) == nil {
		if m, ok := info["mode"].(string); ok {
			mode = m
		}
	}

	fmt.Printf("[Signal] Health check OK (%s, mode=%s)\n", reqURL, mode)
	return mode, nil
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

// handleWSMessage processa uma mensagem recebida via WebSocket ou polling HTTP.
func (s *SignalAdapter) handleWSMessage(data []byte) {
	var envelope wsEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		fmt.Printf("[Signal] Erro ao parsear mensagem: %v (dados: %s)\n", err, truncate(string(data), 200))
		return
	}

	if envelope.Envelope == nil {
		return
	}

	env := envelope.Envelope

	// Log dos campos do remetente para debug de allowlist
	fmt.Printf("[Signal] Envelope: source=%q, sourceNumber=%q, sourceUuid=%q, sourceName=%q\n",
		maskIdentifier(env.Source), maskIdentifier(env.SourceNumber), maskIdentifier(env.SourceUUID), env.SourceName)

	// Extrai texto e attachments da mensagem (dataMessage)
	var text string
	var attachments []messaging.Attachment

	if env.DataMessage != nil {
		text = env.DataMessage.Message

		// Baixa attachments (áudio, imagens, documentos, etc.)
		for _, att := range env.DataMessage.Attachments {
			data, err := s.downloadAttachment(att.ID)
			if err != nil {
				fmt.Printf("[Signal] Erro ao baixar attachment %s (%s): %v\n", att.ID, att.ContentType, err)
				continue
			}

			filename := att.Filename
			if filename == "" {
				filename = fmt.Sprintf("attachment_%s", att.ID)
			}

			attachments = append(attachments, messaging.Attachment{
				Filename: filename,
				MIMEType: att.ContentType,
				Data:     data,
				Size:     att.Size,
			})
			fmt.Printf("[Signal] Attachment baixado: %s (%s, %d bytes)\n", filename, att.ContentType, len(data))
		}
	} else if env.SyncMessage != nil && env.SyncMessage.SentMessage != nil {
		// Mensagens de sync são enviadas por nós de outro dispositivo — ignoramos
		return
	}

	if text == "" && len(attachments) == 0 {
		return // Ignora mensagens sem conteúdo (delivery receipts, typing indicators, etc.)
	}

	s.mu.RLock()
	handler := s.handler
	ctx := s.ctx
	s.mu.RUnlock()

	if handler == nil {
		return
	}

	// ID principal: número de telefone (mais intuitivo para o usuário).
	// Fallback: UUID (versões mais novas do Signal podem omitir o número).
	contactID := env.SourceNumber
	if contactID == "" {
		contactID = env.SourceUUID
		if contactID == "" {
			contactID = env.Source
		}
	}

	// Username: guarda o identificador alternativo para matching na allowlist.
	// Se o ID é o número, o username é o UUID (e vice-versa).
	username := env.SourceUUID
	if username == "" {
		username = env.Source
	}
	if username == contactID {
		username = env.SourceNumber // evita duplicar
	}

	msg := messaging.IncomingMessage{
		ID: fmt.Sprintf("%d", env.Timestamp),
		From: messaging.Contact{
			ID:          contactID,
			DisplayName: env.SourceName,
			Username:    username,
		},
		Text:        text,
		Attachments: attachments,
		Timestamp:   timestampToTime(env.Timestamp),
		Channel:     "signal",
	}

	fmt.Printf("[Signal] Mensagem de %s (%s): %s\n", msg.From.DisplayName, maskIdentifier(msg.From.ID), truncate(msg.Text, 100))

	// Processa em goroutine para não bloquear o reader loop
	go handler(ctx, msg)
}

// httpPollLoop faz polling HTTP para receber mensagens (modo native).
func (s *SignalAdapter) httpPollLoop() {
	pollInterval := 3 * time.Second

	fmt.Printf("[Signal] Iniciando HTTP polling a cada %s\n", pollInterval)

	for {
		select {
		case <-s.ctx.Done():
			fmt.Println("[Signal] HTTP poll loop encerrado (contexto cancelado)")
			return
		case <-time.After(pollInterval):
		}

		s.pollMessages()
	}
}

// pollMessages faz GET /v1/receive/{number} e processa mensagens pendentes.
func (s *SignalAdapter) pollMessages() {
	reqURL := s.baseURL + "/v1/receive/" + url.PathEscape(s.account)
	req, err := http.NewRequestWithContext(s.ctx, "GET", reqURL, nil)
	if err != nil {
		return
	}

	resp, err := s.client.Do(s.ctx, req)
	if err != nil {
		if s.ctx.Err() == nil {
			fmt.Printf("[Signal] Erro no polling: %v\n", err)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[Signal] Polling retornou %d: %s\n", resp.StatusCode, truncate(string(body), 200))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return
	}

	// A resposta pode ser um array de envelopes ou vazia
	var envelopes []wsEnvelope
	if err := json.Unmarshal(body, &envelopes); err != nil {
		// Tenta como envelope único
		var single wsEnvelope
		if json.Unmarshal(body, &single) == nil && single.Envelope != nil {
			envelopes = []wsEnvelope{single}
		} else {
			return
		}
	}

	for _, env := range envelopes {
		data, _ := json.Marshal(env)
		s.handleWSMessage(data)
	}
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
