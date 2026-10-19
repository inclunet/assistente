package llm

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ProviderRegistry armazena os provedores LLM disponíveis
// Thread-safe para acesso concorrente.
type ProviderRegistry struct {
	lifetimeMu sync.RWMutex
	mu         sync.RWMutex
	generation uint64
	providers  map[string]*ProviderConfig
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

// RegisterPendingAuthorization mantém conexão existente acessível para edição
// quando ainda não recebeu o aceite HTTP remoto. A validação estrutural é
// obrigatória; o uso normal continua bloqueado por ProviderConfig.Validate.
func (r *ProviderRegistry) RegisterPendingAuthorization(provider *ProviderConfig) error {
	if r == nil {
		return fmt.Errorf("registry nil")
	}
	if provider == nil {
		return fmt.Errorf("provider nil")
	}
	if provider.EffectiveAuthMode() == AuthModeNone || !RequiresInsecureHTTPAuthorization(provider.BaseURL) ||
		provider.InsecureHTTPAuthorizedURL == provider.BaseURL {
		return fmt.Errorf("provider %s não está pendente de autorização HTTP remota", provider.ID)
	}
	if err := provider.validate(true, false); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.ID] = provider
	return nil
}

// RegisterPendingInvalidBaseURL preserva uma conexão existente cuja URL precisa
// ser corrigida. A validação continua bloqueando qualquer uso normal.
func (r *ProviderRegistry) RegisterPendingInvalidBaseURL(provider *ProviderConfig) error {
	if r == nil {
		return fmt.Errorf("registry nil")
	}
	if provider == nil {
		return fmt.Errorf("provider nil")
	}
	if err := provider.Validate(); !errors.Is(err, ErrInvalidProviderBaseURL) {
		return fmt.Errorf("provider %s não está pendente de correção de base_url", provider.ID)
	}
	if err := provider.validate(false, true); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.ID] = provider
	return nil
}

// Generation identifies the current registry lifetime. Clear invalidates snapshots.
func (r *ProviderRegistry) Generation() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.generation
}

// WithGeneration reserves the registry lifetime through a short commit/publication.
// The callback may use registry methods, but must not call Clear or WithGeneration.
func (r *ProviderRegistry) WithGeneration(generation uint64, commit func() error) error {
	r.lifetimeMu.RLock()
	defer r.lifetimeMu.RUnlock()
	if r.Generation() != generation {
		return fmt.Errorf("provider registry session changed")
	}
	return commit()
}

// RegisterGeneration publishes only into the lifetime where work started.
func (r *ProviderRegistry) RegisterGeneration(provider *ProviderConfig, generation uint64) error {
	if provider == nil {
		return fmt.Errorf("provider nil")
	}
	if err := provider.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if generation != r.generation {
		return fmt.Errorf("provider registry session changed")
	}
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

// GetSnapshot returns an isolated provider configuration copied while the
// registry read lock is held. Streaming requests use it to pin the exact
// compatibility revision that created their adapter.
func (r *ProviderRegistry) GetSnapshot(id string) *ProviderConfig {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	registered := r.providers[id]
	if registered == nil {
		return nil
	}
	snapshot := *registered
	snapshot.Headers = cloneProviderStringMap(registered.Headers)
	snapshot.ACPArgs = append([]string(nil), registered.ACPArgs...)
	snapshot.ACPEnv = cloneProviderStringMap(registered.ACPEnv)
	snapshot.ACPCredentialEnv = cloneProviderStringMap(registered.ACPCredentialEnv)
	return &snapshot
}

func cloneProviderStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
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

// Clear remove todos os provedores registrados em memória.
func (r *ProviderRegistry) Clear() {
	if r == nil {
		return
	}
	r.lifetimeMu.Lock()
	defer r.lifetimeMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generation++
	r.providers = make(map[string]*ProviderConfig)
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

// SetDefaultGeneration replaces snapshots instead of mutating pointers held by readers.
func (r *ProviderRegistry) SetDefaultGeneration(id string, generation uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if generation != r.generation {
		return fmt.Errorf("provider registry session changed")
	}
	if r.providers[id] == nil {
		return fmt.Errorf("provider not found: %s", id)
	}
	for key, provider := range r.providers {
		updated := *provider
		updated.IsDefault = key == id
		r.providers[key] = &updated
	}
	return nil
}
