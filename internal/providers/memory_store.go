package providers

import (
	"context"
	"fmt"
	"reflect"
	"sync"

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
		if current, exists := s.providers[p.ID]; exists {
			revision := current.CompatibilityRevision
			if revision < 1 {
				revision = 1
			}
			if memoryProviderCompatibilityIdentityChanged(current, p) {
				revision++
			}
			p.CompatibilityRevision = revision
			configRevision := current.ConfigRevision
			if configRevision < 1 {
				configRevision = 1
			}
			if memoryProviderConfigurationChanged(current, p) {
				configRevision++
			}
			p.ConfigRevision = configRevision
		} else if p.CompatibilityRevision < 1 {
			p.CompatibilityRevision = 1
			if p.ConfigRevision < 1 {
				p.ConfigRevision = 1
			}
		} else if p.ConfigRevision < 1 {
			p.ConfigRevision = 1
		}
		clone := *p
		s.providers[p.ID] = &clone
	}
	return nil
}

func memoryProviderConfigurationChanged(current, next *llm.ProviderConfig) bool {
	if current == nil || next == nil {
		return true
	}
	currentSnapshot := *current
	nextSnapshot := *next
	currentSnapshot.CompatibilityRevision = 0
	nextSnapshot.CompatibilityRevision = 0
	currentSnapshot.ConfigRevision = 0
	nextSnapshot.ConfigRevision = 0
	return !reflect.DeepEqual(currentSnapshot, nextSnapshot)
}

func memoryProviderCompatibilityIdentityChanged(current, next *llm.ProviderConfig) bool {
	if current == nil || next == nil {
		return true
	}
	return current.Type != next.Type || current.APIFormat != next.APIFormat || current.BaseURL != next.BaseURL ||
		current.CredentialPattern != next.CredentialPattern || current.AuthMode != next.AuthMode ||
		current.ReasoningContentMode != next.ReasoningContentMode || current.ACPCommand != next.ACPCommand ||
		!reflect.DeepEqual(current.ACPArgs, next.ACPArgs) || !reflect.DeepEqual(current.ACPEnv, next.ACPEnv) ||
		!reflect.DeepEqual(current.ACPCredentialEnv, next.ACPCredentialEnv) || current.ACPAgentID != next.ACPAgentID
}

func (s *MemoryStore) BumpCompatibilityRevisionsForCredentialPattern(_ context.Context, pattern string) (map[string]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	revisions := make(map[string]int)
	for id, provider := range s.providers {
		if provider.CredentialPattern != pattern {
			continue
		}
		provider.CompatibilityRevision++
		revisions[id] = provider.CompatibilityRevision
	}
	return revisions, nil
}

func (s *MemoryStore) GetCompatibilityRevisionsForCredentialPattern(_ context.Context, pattern string) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	revisions := make(map[string]int)
	for id, provider := range s.providers {
		if provider.CredentialPattern == pattern {
			revisions[id] = provider.CompatibilityRevision
		}
	}
	return revisions, nil
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
		isDefault := pid == id
		if p.IsDefault != isDefault {
			if p.ConfigRevision < 1 {
				p.ConfigRevision = 1
			}
			p.ConfigRevision++
			p.IsDefault = isDefault
		}
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
