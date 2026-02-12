package confirmation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultTimeout é o tempo máximo para aguardar a resposta do usuário.
	DefaultTimeout = 60 * time.Second
)

// Request representa uma solicitação de confirmação pendente.
type Request struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	WorkDir   string `json:"workDir"`
	CreatedAt string `json:"createdAt"`
	response  chan bool
}

// Manager gerencia solicitações de confirmação entre backend (tool execution) e frontend.
//
// Fluxo:
//  1. Tool chama RequestConfirmation(ctx, command, workDir)
//  2. Manager cria um Request e emite evento Wails "tool:confirm_command"
//  3. Frontend mostra ConfirmDialog e chama RespondCommandConfirmation via binding
//  4. Respond() envia a resposta pelo channel e desbloqueia RequestConfirmation()
type Manager struct {
	pending   map[string]*Request
	mu        sync.Mutex
	emitEvent func(event string, data any) // callback para emitir eventos Wails
}

// NewManager cria um novo gerenciador de confirmações.
func NewManager(emitEvent func(event string, data any)) *Manager {
	if emitEvent == nil {
		emitEvent = func(string, any) {}
	}

	return &Manager{
		pending:   make(map[string]*Request),
		emitEvent: emitEvent,
	}
}

// RequestConfirmation solicita confirmação ao usuário para executar um comando.
// Bloqueia até que o usuário responda, o contexto seja cancelado, ou o timeout expire.
// Retorna true se o usuário aprovou, false caso contrário.
func (m *Manager) RequestConfirmation(ctx context.Context, command, workDir string) (bool, error) {
	req := &Request{
		ID:        uuid.New().String()[:8],
		Command:   command,
		WorkDir:   workDir,
		CreatedAt: time.Now().Format(time.RFC3339),
		response:  make(chan bool, 1),
	}

	// Registra request pendente
	m.mu.Lock()
	m.pending[req.ID] = req
	m.mu.Unlock()

	// Garante cleanup ao sair
	defer func() {
		m.mu.Lock()
		delete(m.pending, req.ID)
		m.mu.Unlock()
	}()

	// Emite evento para o frontend
	m.emitEvent("tool:confirm_command", map[string]string{
		"id":      req.ID,
		"command": req.Command,
		"workDir": req.WorkDir,
	})

	// Cria timeout para a resposta do usuário
	timeoutCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// Aguarda resposta, timeout ou cancelamento
	select {
	case approved := <-req.response:
		return approved, nil
	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			return false, fmt.Errorf("solicitação cancelada")
		}
		return false, fmt.Errorf("timeout aguardando confirmação do usuário (%s)", DefaultTimeout)
	}
}

// Respond envia a resposta do usuário para uma solicitação de confirmação pendente.
// Chamado pelo binding Wails quando o frontend responde ao dialog.
func (m *Manager) Respond(requestID string, approved bool) error {
	m.mu.Lock()
	req, ok := m.pending[requestID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("solicitação de confirmação '%s' não encontrada ou expirada", requestID)
	}

	// Envia resposta pelo channel (non-blocking pois tem buffer 1)
	select {
	case req.response <- approved:
		return nil
	default:
		return fmt.Errorf("resposta já enviada para solicitação '%s'", requestID)
	}
}

// PendingCount retorna o número de solicitações pendentes.
func (m *Manager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending)
}
