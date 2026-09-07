package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/database"
	"assistente/internal/memory"
	"context"
	"sync"
)

// Memory é o bind Wails do domínio memory (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Memory struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.MemoryController
}

// NewMemory cria o bind vazio; AttachMemory preenche session + controller no startup.
func NewMemory() *Memory {
	return &Memory{}
}

// AttachMemory associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachMemory(m *Memory, session Session, ctrl *controllers.MemoryController) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = session
	m.ctrl = ctrl
}

func (m *Memory) deps() (Session, *controllers.MemoryController, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil || m.ctrl == nil {
		return nil, nil, ErrMemoryNotWired
	}
	return m.session, m.ctrl, nil
}

// ListMemoryRecords lista registros de memória conforme o filtro.
func (m *Memory) ListMemoryRecords(filter memory.Filter) (memory.ListResult, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return memory.ListResult{}, err
	}
	return WithUser(session, func(ctx context.Context) (memory.ListResult, error) {
		return ctrl.ListMemoryRecords(ctx, filter)
	})
}

// SearchMemoryRecords busca registros de memória por texto, respeitando o filtro.
func (m *Memory) SearchMemoryRecords(query string, filter memory.Filter) (memory.ListResult, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return memory.ListResult{}, err
	}
	return WithUser(session, func(ctx context.Context) (memory.ListResult, error) {
		return ctrl.SearchMemoryRecords(ctx, query, filter)
	})
}

// GetMemoryRecord retorna um registro de memória pelo id.
func (m *Memory) GetMemoryRecord(id string) (*database.MemoryRecord, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.MemoryRecord, error) {
		return ctrl.GetMemoryRecord(ctx, id)
	})
}

// CreateMemoryRecord cria um novo registro de memória.
func (m *Memory) CreateMemoryRecord(input memory.RecordInput) (*database.MemoryRecord, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.MemoryRecord, error) {
		return ctrl.CreateMemoryRecord(ctx, input)
	})
}

// UpdateMemoryRecord atualiza um registro de memória existente.
func (m *Memory) UpdateMemoryRecord(id string, input memory.RecordInput) (*database.MemoryRecord, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.MemoryRecord, error) {
		return ctrl.UpdateMemoryRecord(ctx, id, input)
	})
}

// ArchiveMemoryRecord arquiva um registro de memória.
func (m *Memory) ArchiveMemoryRecord(id string) (*database.MemoryRecord, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.MemoryRecord, error) {
		return ctrl.ArchiveMemoryRecord(ctx, id)
	})
}

// UnarchiveMemoryRecord restaura um registro de memória arquivado para a política informada.
func (m *Memory) UnarchiveMemoryRecord(id string, loadPolicy string) (*database.MemoryRecord, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.MemoryRecord, error) {
		return ctrl.UnarchiveMemoryRecord(ctx, id, loadPolicy)
	})
}

// DeleteMemoryRecord remove um registro de memória pelo id.
func (m *Memory) DeleteMemoryRecord(id string) error {
	session, ctrl, err := m.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteMemoryRecord(ctx, id)
	})
	return err
}

// GetMemoryPolicySummary retorna o resumo da política de memória aplicada.
func (m *Memory) GetMemoryPolicySummary() (memory.PolicySummary, error) {
	session, ctrl, err := m.deps()
	if err != nil {
		return memory.PolicySummary{}, err
	}
	return WithUser(session, func(ctx context.Context) (memory.PolicySummary, error) {
		return ctrl.GetMemoryPolicySummary(ctx)
	})
}
