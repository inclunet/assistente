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
	DefaultTimeout = 20 * time.Minute
)

// Eventos que o backend emite sobre um questionário.
const (
	// EventQuestionnaire abre o diálogo na tela.
	EventQuestionnaire = "tool:questionnaire"
	// EventQuestionnaireClosed diz que a pergunta perdeu o dono: quem
	// esperava a resposta desistiu ou o prazo estourou. Sem ele o diálogo
	// ficaria aberto pedindo decisão sobre algo que já não existe, e a
	// resposta só descobriria isso ao ser recusada.
	EventQuestionnaireClosed = "tool:questionnaire:closed"
)

// Motivos pelos quais uma pergunta se encerra sem resposta.
const (
	// ClosedCancelled é quem perguntou tendo desistido — turno cancelado,
	// conversa excluída, app encerrando.
	ClosedCancelled = "cancelled"
	// ClosedTimeout é o prazo da pergunta estourado.
	ClosedTimeout = "timeout"
)

// Question define um item do questionário.
type Question struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Prompt      string   `json:"prompt"`
	Description string   `json:"description,omitempty"`
	Content     string   `json:"content,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Options     []string `json:"options,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Step        *float64 `json:"step,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Default     any      `json:"default,omitempty"`
	// AutoFocus indica que este item deve receber o foco inicial quando o
	// diálogo abre, sobrepondo a heurística padrão do frontend (primeiro
	// campo editável). Apenas o primeiro item marcado é considerado.
	AutoFocus bool `json:"autoFocus,omitempty"`
}

// RejectReasonConfig descreve um campo opcional de texto livre exibido junto
// ao botão de cancelar, permitindo ao usuário justificar a rejeição. A resposta
// volta em Response.Answers sob a chave ID mesmo quando Cancelled=true.
type RejectReasonConfig struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	// MaxLen limita o tamanho do texto no frontend (em caracteres); 0 = sem limite explícito.
	MaxLen int `json:"maxLen,omitempty"`
}

// RequestPayload representa uma solicitação de questionário pendente.
type RequestPayload struct {
	ID           string              `json:"id"`
	Title        string              `json:"title,omitempty"`
	Description  string              `json:"description,omitempty"`
	Questions    []Question          `json:"questions"`
	AllowCancel  bool                `json:"allowCancel,omitempty"`
	SubmitLabel  string              `json:"submitLabel,omitempty"`
	CancelLabel  string              `json:"cancelLabel,omitempty"`
	RejectReason *RejectReasonConfig `json:"rejectReason,omitempty"`
	Timeout      time.Duration       `json:"-"` // 0 = DefaultTimeout
	CreatedAt    string              `json:"createdAt"`
	response     chan Response
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
		ID:           uuid.New().String()[:8],
		Title:        payload.Title,
		Description:  payload.Description,
		Questions:    payload.Questions,
		AllowCancel:  payload.AllowCancel,
		SubmitLabel:  payload.SubmitLabel,
		CancelLabel:  payload.CancelLabel,
		RejectReason: payload.RejectReason,
		CreatedAt:    time.Now().Format(time.RFC3339),
		response:     make(chan Response, 1),
	}

	m.mu.Lock()
	m.pending[req.ID] = req
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, req.ID)
		m.mu.Unlock()
	}()

	eventData := map[string]any{
		"id":          req.ID,
		"title":       req.Title,
		"description": req.Description,
		"questions":   req.Questions,
		"allowCancel": req.AllowCancel,
		"submitLabel": req.SubmitLabel,
		"cancelLabel": req.CancelLabel,
		"createdAt":   req.CreatedAt,
	}
	if req.RejectReason != nil {
		eventData["rejectReason"] = req.RejectReason
	}
	m.emitEvent(EventQuestionnaire, eventData)

	timeout := payload.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case resp := <-req.response:
		return resp, nil
	case <-timeoutCtx.Done():
		// A pergunta acabou sem dono, mas o diálogo continua na tela pedindo
		// uma decisão que não chega a lugar nenhum. Quem está lendo precisa
		// saber disso — ainda mais quem lê por leitor de telas, que teria de
		// percorrer o diálogo inteiro para descobrir que ele não vale mais.
		if ctx.Err() != nil {
			m.emitClosed(req.ID, ClosedCancelled)
			return Response{}, fmt.Errorf("solicitação cancelada")
		}
		m.emitClosed(req.ID, ClosedTimeout)
		return Response{}, fmt.Errorf("timeout aguardando respostas do usuário (%s)", timeout)
	}
}

func (m *Manager) emitClosed(requestID, reason string) {
	m.emitEvent(EventQuestionnaireClosed, map[string]any{
		"id":     requestID,
		"reason": reason,
	})
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
