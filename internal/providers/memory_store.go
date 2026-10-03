package providers

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// MemoryStore é uma implementação em memória de ProviderStore.
// Útil para testes unitários que não precisam de banco de dados.
type MemoryStore struct {
	mu        sync.RWMutex
	providers map[string]*llm.ProviderConfig
	defaultID string
}

// NewMemoryStore cria um MemoryStore vazio.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		providers: make(map[string]*llm.ProviderConfig),
	}
}

func (s *MemoryStore) Save(_ context.Context, providers []*llm.ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range providers {
		clone := *p
		s.providers[p.ID] = &clone
	}
	return nil
}

func (s *MemoryStore) Load(_ context.Context) ([]*llm.ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*llm.ProviderConfig, 0, len(s.providers))
	for _, p := range s.providers {
		clone := *p
		result = append(result, &clone)
	}
	return result, nil
}

func (s *MemoryStore) SetDefault(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[id]; !ok {
		return fmt.Errorf("provider '%s' não encontrado", id)
	}
	for pid, p := range s.providers {
		p.IsDefault = (pid == id)
	}
	s.defaultID = id
	return nil
}

func (s *MemoryStore) GetDefault(_ context.Context) (*llm.ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.defaultID == "" {
		return nil, fmt.Errorf("nenhum provider default definido")
	}
	p, ok := s.providers[s.defaultID]
	if !ok {
		return nil, fmt.Errorf("provider default '%s' não encontrado", s.defaultID)
	}
	clone := *p
	return &clone, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (*llm.ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[id]
	if !ok {
		return nil, fmt.Errorf("provider '%s' não encontrado", id)
	}
	clone := *p
	return &clone, nil
}

func (s *MemoryStore) Count(_ context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.providers), nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.providers, id)
	if s.defaultID == id {
		s.defaultID = ""
	}
	return nil
}

func (s *MemoryStore) Create(_ context.Context, provider *llm.ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if provider == nil {
		return fmt.Errorf("provedor inválido")
	}
	if _, exists := s.providers[provider.ID]; exists {
		return database.ErrLLMProviderAlreadyExists
	}
	clone := *provider
	s.providers[provider.ID] = &clone
	return nil
}

func (s *MemoryStore) Exists(_ context.Context, id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.providers[id]
	return exists, nil
}

func (s *MemoryStore) RollbackCreate(_ context.Context, provider *llm.ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if provider == nil {
		return fmt.Errorf("reserva de criação inválida")
	}
	if !reflect.DeepEqual(s.providers[provider.ID], provider) {
		return fmt.Errorf("configuração do provedor mudou durante a criação")
	}
	delete(s.providers, provider.ID)
	if s.defaultID == provider.ID {
		s.defaultID = ""
	}
	return nil
}
