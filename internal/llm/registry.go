package llm

import (
	"fmt"
	"sort"
	"sync"
)

// ProviderRegistry armazena os provedores LLM disponíveis
// Thread-safe para acesso concorrente.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]*ProviderConfig
}

// NewProviderRegistry cria um novo registry vazio
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]*ProviderConfig),
	}
}

// Register registra um provider
func (r *ProviderRegistry) Register(provider *ProviderConfig) error {
	if r == nil {
		return fmt.Errorf("registry nil")
	}
	if provider == nil {
		return fmt.Errorf("provider nil")
	}
	if err := provider.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Sobrescreve se existir (para permitir update)
	r.providers[provider.ID] = provider
	return nil
}

// Get retorna um provider pelo ID
func (r *ProviderRegistry) Get(id string) *ProviderConfig {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.providers[id]
}

// List retorna todos os providers (ordenados por ID)
func (r *ProviderRegistry) List() []*ProviderConfig {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*ProviderConfig, 0, len(r.providers))
	for _, provider := range r.providers {
		list = append(list, provider)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})

	return list
}

// Remove remove um provider pelo ID
func (r *ProviderRegistry) Remove(id string) error {
	if r == nil {
		return fmt.Errorf("registry nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[id]; !exists {
		return fmt.Errorf("provider not found: %s", id)
	}

	delete(r.providers, id)
	return nil
}
