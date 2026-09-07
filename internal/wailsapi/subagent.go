package wailsapi

import (
	"assistente/internal/subagent"
	"context"
	"sync"
)

// Subagent é o bind Wails do domínio subagent (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
//
// subagentParentDelivery, buildSubagentNotice, sanitizeUntrusted e a criação
// do Manager permanecem no *App — fora do escopo desta migração.
type Subagent struct {
	mu      sync.RWMutex
	session Session
	mgr     *subagent.Manager
}

// NewSubagent cria o bind vazio; AttachSubagent preenche session + manager no startup.
func NewSubagent() *Subagent {
	return &Subagent{}
}

// AttachSubagent associa Session e Manager após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachSubagent(s *Subagent, session Session, mgr *subagent.Manager) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = session
	s.mgr = mgr
}

func (s *Subagent) deps() (Session, *subagent.Manager, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.session == nil || s.mgr == nil {
		return nil, nil, ErrSubagentNotWired
	}
	return s.session, s.mgr, nil
}

// ListSubAgentRuns devolve os runs de sub-agente do usuário autenticado —
// ativos primeiro, depois os mais recentes — junto da ocupação dos tetos de
// concorrência. limit <= 0 usa o padrão do repositório.
func (s *Subagent) ListSubAgentRuns(limit int) (subagent.RunListResult, error) {
	session, mgr, err := s.deps()
	if err != nil {
		return subagent.RunListResult{}, err
	}
	return WithUser(session, func(ctx context.Context) (subagent.RunListResult, error) {
		return mgr.ListRuns(ctx, limit)
	})
}

// CancelSubAgentRun cancela um run de sub-agente a partir da UI. Reusa o mesmo
// Manager.Cancel da tool (sem caminho alternativo de cancelamento).
func (s *Subagent) CancelSubAgentRun(conversationID, runID string) (subagent.CancelResult, error) {
	session, mgr, err := s.deps()
	if err != nil {
		return subagent.CancelResult{}, err
	}
	return WithUser(session, func(ctx context.Context) (subagent.CancelResult, error) {
		return mgr.Cancel(ctx, conversationID, runID)
	})
}
