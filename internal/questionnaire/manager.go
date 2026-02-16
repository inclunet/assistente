package questionnaire

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultTimeout é o tempo máximo para aguardar respostas do usuário.
	DefaultTimeout = 5 * time.Minute
)

// Question define um item do questionário.
type Question struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Prompt      string  `json:"prompt"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Options     []string `json:"options,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Step        *float64 `json:"step,omitempty"`
	Placeholder string  `json:"placeholder,omitempty"`
	Default     any     `json:"default,omitempty"`
}

// RequestPayload representa uma solicitação de questionário pendente.
type RequestPayload struct {
	ID          string     `json:"id"`
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	Questions   []Question `json:"questions"`
	AllowCancel bool       `json:"allowCancel,omitempty"`
	SubmitLabel string     `json:"submitLabel,omitempty"`
	CancelLabel string     `json:"cancelLabel,omitempty"`
	CreatedAt   string     `json:"createdAt"`
	response    chan Response
}

// Response representa a resposta do usuário.
type Response struct {
	ID        string         `json:"id"`
	Answers   map[string]any `json:"answers"`
	Cancelled bool           `json:"cancelled,omitempty"`
}

// Manager gerencia solicitações de questionários entre backend e frontend.
type Manager struct {
	pending   map[string]*RequestPayload
	mu        sync.Mutex
	emitEvent func(event string, data any)
}

// NewManager cria um novo gerenciador de questionários.
func NewManager(emitEvent func(event string, data any)) *Manager {
	if emitEvent == nil {
		emitEvent = func(string, any) {}
	}

	return &Manager{
		pending:   make(map[string]*RequestPayload),
		emitEvent: emitEvent,
	}
}

// RequestQuestionnaire solicita respostas do usuário para um questionário.
// Bloqueia até resposta, cancelamento ou timeout.
func (m *Manager) RequestQuestionnaire(ctx context.Context, payload RequestPayload) (Response, error) {
	req := &RequestPayload{
		ID:          uuid.New().String()[:8],
		Title:       payload.Title,
		Description: payload.Description,
		Questions:   payload.Questions,
		AllowCancel: payload.AllowCancel,
		SubmitLabel: payload.SubmitLabel,
		CancelLabel: payload.CancelLabel,
		CreatedAt:   time.Now().Format(time.RFC3339),
		response:    make(chan Response, 1),
	}

	m.mu.Lock()
	m.pending[req.ID] = req
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, req.ID)
		m.mu.Unlock()
	}()

	m.emitEvent("tool:questionnaire", map[string]any{
		"id":          req.ID,
		"title":       req.Title,
		"description": req.Description,
		"questions":   req.Questions,
		"allowCancel": req.AllowCancel,
		"submitLabel": req.SubmitLabel,
		"cancelLabel": req.CancelLabel,
		"createdAt":   req.CreatedAt,
	})

	timeoutCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	select {
	case resp := <-req.response:
		return resp, nil
	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			return Response{}, fmt.Errorf("solicitação cancelada")
		}
		return Response{}, fmt.Errorf("timeout aguardando respostas do usuário (%s)", DefaultTimeout)
	}
}

// Respond envia a resposta do usuário para um questionário pendente.
func (m *Manager) Respond(requestID string, answers map[string]any, cancelled bool) error {
	m.mu.Lock()
	req, ok := m.pending[requestID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("solicitação de questionário '%s' não encontrada ou expirada", requestID)
	}

	resp := Response{
		ID:        requestID,
		Answers:   answers,
		Cancelled: cancelled,
	}

	select {
	case req.response <- resp:
		return nil
	default:
		return fmt.Errorf("resposta já enviada para questionário '%s'", requestID)
	}
}

// PendingCount retorna o número de solicitações pendentes.
func (m *Manager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending)
}
