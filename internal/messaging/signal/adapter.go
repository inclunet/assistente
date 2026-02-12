package signal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"assistente/internal/messaging"
)

// SignalAdapter implementa messaging.Messenger para o Signal via signal-cli JSON-RPC.
// Lança signal-cli como subprocesso e comunica via stdin/stdout (JSON-RPC 2.0).
//
// Pré-requisitos:
//   - Java 21+ instalado
//   - signal-cli binário acessível (PATH ou caminho absoluto)
//   - Conta Signal vinculada (via signal-cli link ou signal-cli register)
type SignalAdapter struct {
	bin     string // caminho do binário signal-cli
	account string // número de telefone da conta Signal (ex: "+5511999999999")

	handler messaging.IncomingMessageHandler
	status  messaging.ConnectionStatus

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	// Controle de respostas pendentes: id -> canal para receber resposta
	pending   map[string]chan jsonRPCResponse
	pendingMu sync.Mutex

	// Serializa escritas no stdin
	writeMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// NewAdapter cria um novo adapter para o Signal.
// bin é o caminho do binário signal-cli (vazio = "signal-cli" no PATH).
// account é o número de telefone vinculado (ex: "+5511999999999").
func NewAdapter(bin, account string) *SignalAdapter {
	if bin == "" {
		bin = "signal-cli"
	}
	return &SignalAdapter{
		bin:     bin,
		account: account,
		status:  messaging.StatusDisconnected,
		pending: make(map[string]chan jsonRPCResponse),
	}
}

// Name retorna o identificador da plataforma.
func (s *SignalAdapter) Name() string {
	return "signal"
}

// Connect inicia o subprocesso signal-cli em modo JSON-RPC e começa a ler respostas.
func (s *SignalAdapter) Connect(ctx context.Context) error {
	s.mu.Lock()
	s.status = messaging.StatusConnecting
	s.mu.Unlock()

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Monta o comando: signal-cli -a ACCOUNT jsonRpc
	args := []string{}
	if s.account != "" {
		args = append(args, "-a", s.account)
	}
	args = append(args, "jsonRpc")

	s.cmd = exec.CommandContext(s.ctx, s.bin, args...)

	var err error
	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("erro ao obter stdin do signal-cli: %w", err)
	}

	s.stdout, err = s.cmd.StdoutPipe()
	if err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("erro ao obter stdout do signal-cli: %w", err)
	}

	if err := s.cmd.Start(); err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("erro ao iniciar signal-cli: %w", err)
	}

	s.setStatus(messaging.StatusConnected)
	fmt.Printf("[Signal] Conectado (pid=%d, account=%s)\n", s.cmd.Process.Pid, s.account)

	// Inicia goroutine que lê respostas/notificações do stdout
	go s.readLoop()

	// Inicia goroutine que espera o processo terminar
	go s.waitProcess()

	return nil
}

// Disconnect encerra o subprocesso signal-cli.
func (s *SignalAdapter) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	s.status = messaging.StatusDisconnected
	fmt.Println("[Signal] Desconectado")
	return nil
}

// Send envia uma mensagem de texto para um contato via Signal.
func (s *SignalAdapter) Send(ctx context.Context, msg messaging.OutgoingMessage) error {
	req := newSendRequest(msg.ChatID, msg.Text)

	resp, err := s.sendRequest(req)
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem Signal para %s: %w", msg.ChatID, err)
	}

	if resp.Error != nil {
		return fmt.Errorf("signal-cli retornou erro: %v", resp.Error)
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

// sendRequest envia uma requisição JSON-RPC e aguarda a resposta correspondente.
func (s *SignalAdapter) sendRequest(req jsonRPCRequest) (jsonRPCResponse, error) {
	// Cria canal para receber a resposta
	ch := make(chan jsonRPCResponse, 1)
	s.pendingMu.Lock()
	s.pending[req.ID] = ch
	s.pendingMu.Unlock()

	// Cleanup em caso de erro ou timeout
	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, req.ID)
		s.pendingMu.Unlock()
	}()

	// Serializa e envia
	data, err := json.Marshal(req)
	if err != nil {
		return jsonRPCResponse{}, fmt.Errorf("erro ao serializar request: %w", err)
	}
	data = append(data, '\n') // signal-cli espera uma linha por request

	s.writeMu.Lock()
	_, err = s.stdin.Write(data)
	s.writeMu.Unlock()

	if err != nil {
		return jsonRPCResponse{}, fmt.Errorf("erro ao escrever no stdin: %w", err)
	}

	// Aguarda resposta ou cancelamento
	select {
	case resp := <-ch:
		return resp, nil
	case <-s.ctx.Done():
		return jsonRPCResponse{}, fmt.Errorf("contexto cancelado enquanto aguardava resposta")
	}
}

// readLoop lê continuamente o stdout do signal-cli, processando respostas e notificações.
func (s *SignalAdapter) readLoop() {
	scanner := bufio.NewScanner(s.stdout)
	// Aumenta o buffer para mensagens grandes
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			fmt.Printf("[Signal] Erro ao parsear resposta: %v (linha: %s)\n", err, truncate(string(line), 200))
			continue
		}

		if resp.IsNotification() {
			// Notificação de mensagem recebida
			s.handleNotification(resp)
		} else if resp.ID != nil {
			// Resposta a uma requisição pendente
			s.pendingMu.Lock()
			ch, ok := s.pending[*resp.ID]
			s.pendingMu.Unlock()

			if ok {
				ch <- resp
			} else {
				fmt.Printf("[Signal] Resposta para request desconhecido: id=%s\n", *resp.ID)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("[Signal] Erro no reader: %v\n", err)
	}
	fmt.Println("[Signal] Read loop encerrado")
}

// handleNotification processa uma notificação de mensagem recebida do signal-cli.
func (s *SignalAdapter) handleNotification(resp jsonRPCResponse) {
	if resp.Method != "receive" {
		return
	}

	var notification receiveNotification
	if err := json.Unmarshal(resp.Params, &notification); err != nil {
		fmt.Printf("[Signal] Erro ao parsear notificação: %v\n", err)
		return
	}

	env := notification.Envelope

	// Extrai texto da mensagem (dataMessage ou syncMessage)
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

	// Usa sourceNumber como ID do contato (número de telefone)
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

// waitProcess aguarda o processo signal-cli terminar e atualiza o status.
func (s *SignalAdapter) waitProcess() {
	if s.cmd == nil {
		return
	}

	err := s.cmd.Wait()
	if err != nil {
		// Verifica se foi cancelamento normal (ctx.Done)
		if s.ctx.Err() != nil {
			fmt.Println("[Signal] Processo encerrado (contexto cancelado)")
		} else {
			fmt.Printf("[Signal] Processo encerrado com erro: %v\n", err)
			s.setStatus(messaging.StatusError)
		}
	} else {
		fmt.Println("[Signal] Processo encerrado normalmente")
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
