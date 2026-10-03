package llm

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"sync"
)

var ErrStaleProviderSnapshot = errors.New("provider snapshot is stale")

// ProviderRegistry armazena os provedores LLM disponíveis
// Thread-safe para acesso concorrente.
type ProviderRegistry struct {
	mu                              sync.RWMutex
	generation                      uint64
	providers                       map[string]*ProviderConfig
	revisionWatermarks              map[string]int
	revisionWatermarkPatterns       map[string]map[string]struct{}
	configRevisionWatermarks        map[string]int
	configSnapshots                 map[string]*ProviderConfig
	stale                           map[string]bool
	stalePatterns                   map[string]bool
	credentialPatternSyncGeneration map[string]uint64
	activeCredentialPatternSyncs    map[string]uint64
}

// NewProviderRegistry cria um novo registry vazio
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers:                       make(map[string]*ProviderConfig),
		revisionWatermarks:              make(map[string]int),
		revisionWatermarkPatterns:       make(map[string]map[string]struct{}),
		configRevisionWatermarks:        make(map[string]int),
		configSnapshots:                 make(map[string]*ProviderConfig),
		stale:                           make(map[string]bool),
		stalePatterns:                   make(map[string]bool),
		credentialPatternSyncGeneration: make(map[string]uint64),
		activeCredentialPatternSyncs:    make(map[string]uint64),
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
	snapshot := cloneProviderConfiguration(provider)
	if err := snapshot.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.registerLocked(snapshot)
}

// Generation identifies the current registry lifetime. Clear invalidates snapshots.
func (r *ProviderRegistry) Generation() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.generation
}

// RegisterGeneration publishes only into the lifetime where work started.
func (r *ProviderRegistry) RegisterGeneration(provider *ProviderConfig, generation uint64) error {
	if provider == nil {
		return fmt.Errorf("provider nil")
	}
	snapshot := cloneProviderConfiguration(provider)
	if err := snapshot.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if generation != r.generation {
		return fmt.Errorf("provider registry session changed")
	}
	return r.registerLocked(snapshot)
}

// registerLocked publica um snapshot sem regredir uma revisão que já chegou
// ao registry por outra operação concorrente. Watermarks também cobrem IDs
// vistos antes do primeiro registro. Um provider que migra para outro pattern
// deixa de herdar stale do pattern anterior; uma falha no novo pattern ainda
// o mantém oculto.
func (r *ProviderRegistry) registerLocked(provider *ProviderConfig) error {
	provider = cloneProviderConfiguration(provider)
	previous := r.providers[provider.ID]
	watermark := r.revisionWatermarks[provider.ID]
	if current := r.providers[provider.ID]; current != nil && current.CompatibilityRevision > watermark {
		watermark = current.CompatibilityRevision
	}
	if provider.CompatibilityRevision < watermark {
		return fmt.Errorf("%w: provider %q has %d, watermark is %d", ErrStaleProviderSnapshot, provider.ID, provider.CompatibilityRevision, watermark)
	}
	if provider.CompatibilityRevision > watermark {
		watermark = provider.CompatibilityRevision
	}
	r.revisionWatermarks[provider.ID] = watermark
	r.recordRevisionWatermarkPatternLocked(provider.ID, provider.CredentialPattern)
	configWatermark := r.configRevisionWatermarks[provider.ID]
	if current := r.providers[provider.ID]; current != nil && current.ConfigRevision > configWatermark {
		configWatermark = current.ConfigRevision
	}
	if provider.ConfigRevision < configWatermark {
		return fmt.Errorf("%w: provider %q has config revision %d, watermark is %d", ErrStaleProviderSnapshot, provider.ID, provider.ConfigRevision, configWatermark)
	}
	if snapshot := r.configSnapshots[provider.ID]; snapshot != nil && provider.ConfigRevision == configWatermark && !sameProviderConfiguration(snapshot, provider) {
		return fmt.Errorf("%w: provider %q changed configuration without advancing config revision %d", ErrStaleProviderSnapshot, provider.ID, provider.ConfigRevision)
	}
	if provider.ConfigRevision > configWatermark {
		configWatermark = provider.ConfigRevision
	}
	r.configRevisionWatermarks[provider.ID] = configWatermark
	r.providers[provider.ID] = provider
	r.configSnapshots[provider.ID] = cloneProviderConfiguration(provider)
	if r.stalePatterns[provider.CredentialPattern] {
		r.stale[provider.ID] = true
	} else {
		delete(r.stale, provider.ID)
	}
	if previous != nil && previous.CredentialPattern != provider.CredentialPattern {
		r.clearOrphanedStalePatternLocked(previous.CredentialPattern)
	}
	return nil
}

func (r *ProviderRegistry) recordRevisionWatermarkPatternLocked(id, pattern string) {
	if id == "" || pattern == "" {
		return
	}
	patterns := r.revisionWatermarkPatterns[id]
	if patterns == nil {
		patterns = make(map[string]struct{})
		r.revisionWatermarkPatterns[id] = patterns
	}
	patterns[pattern] = struct{}{}
}

func sameProviderConfiguration(a, b *ProviderConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	left := *a
	right := *b
	left.CompatibilityRevision = 0
	right.CompatibilityRevision = 0
	left.ConfigRevision = 0
	right.ConfigRevision = 0
	return reflect.DeepEqual(left, right)
}

func cloneProviderConfiguration(provider *ProviderConfig) *ProviderConfig {
	if provider == nil {
		return nil
	}
	clone := *provider
	clone.Headers = maps.Clone(provider.Headers)
	clone.ACPArgs = slices.Clone(provider.ACPArgs)
	clone.ACPEnv = maps.Clone(provider.ACPEnv)
	clone.ACPCredentialEnv = maps.Clone(provider.ACPCredentialEnv)
	return &clone
}

func (r *ProviderRegistry) clearOrphanedStalePatternLocked(pattern string) {
	if pattern == "" {
		return
	}
	for _, provider := range r.providers {
		if provider.CredentialPattern == pattern {
			return
		}
	}
	delete(r.stalePatterns, pattern)
	// Invalidar publicações de refresh iniciadas antes da migração. Elas já
	// não descrevem nenhum provider registrado para esse pattern.
	r.credentialPatternSyncGeneration[pattern]++
}

// UpdateCompatibilityRevisions stores monotonic revision watermarks and updates
// registered snapshots without changing their other fields. It deliberately
// does not clear stale; use PublishCredentialPatternRevisions for that.
func (r *ProviderRegistry) UpdateCompatibilityRevisions(revisions map[string]int) error {
	if r == nil {
		return fmt.Errorf("registry nil")
	}
	for id, revision := range revisions {
		if id == "" || revision < 1 {
			return fmt.Errorf("invalid compatibility revision for provider %q", id)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, revision := range revisions {
		if revision > r.revisionWatermarks[id] {
			r.revisionWatermarks[id] = revision
		}
		current, exists := r.providers[id]
		if !exists || revision < current.CompatibilityRevision {
			continue
		}
		if revision > current.CompatibilityRevision {
			updated := *current
			updated.CompatibilityRevision = revision
			r.providers[id] = &updated
		}
	}
	return nil
}

// BeginCredentialPatternRevisionSync marca os snapshots do pattern como
// indisponíveis até a leitura autoritativa terminar e retorna sua geração.
func (r *ProviderRegistry) BeginCredentialPatternRevisionSync(pattern string) uint64 {
	if r == nil || pattern == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	generation, _ := r.markCredentialPatternStaleLocked(pattern)
	r.activeCredentialPatternSyncs[pattern] = generation
	return generation
}

// AbortCredentialPatternRevisionSync encerra uma leitura que falhou sem
// liberar snapshots stale. Uma sincronização futura ainda precisa recuperá-los.
func (r *ProviderRegistry) AbortCredentialPatternRevisionSync(pattern string, generation uint64) {
	if r == nil || pattern == "" || generation == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.credentialPatternSyncGeneration[pattern] == generation && r.activeCredentialPatternSyncs[pattern] == generation {
		delete(r.activeCredentialPatternSyncs, pattern)
	}
}

// PublishCredentialPatternRevisions publica a leitura autoritativa do pattern.
// Watermarks são guardados mesmo se o provider ainda não foi registrado. Uma
// leitura antiga pode atualizar a revisão monotônica, mas só a geração mais
// recente pode remover as marcas stale.
func (r *ProviderRegistry) PublishCredentialPatternRevisions(pattern string, generation uint64, revisions map[string]int) error {
	if r == nil {
		return fmt.Errorf("registry nil")
	}
	if pattern == "" || generation == 0 {
		return fmt.Errorf("invalid credential pattern synchronization")
	}
	for id, revision := range revisions {
		if id == "" || revision < 1 {
			return fmt.Errorf("invalid compatibility revision for provider %q", id)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.credentialPatternSyncGeneration[pattern] != generation {
		return nil
	}
	for id, revision := range revisions {
		if revision > r.revisionWatermarks[id] {
			r.revisionWatermarks[id] = revision
		}
		r.recordRevisionWatermarkPatternLocked(id, pattern)
		current, exists := r.providers[id]
		if !exists || revision < current.CompatibilityRevision {
			continue
		}
		if revision > current.CompatibilityRevision {
			updated := *current
			updated.CompatibilityRevision = revision
			r.providers[id] = &updated
		}
	}
	allCurrentProvidersCovered := true
	for id, provider := range r.providers {
		if provider.CredentialPattern != pattern {
			continue
		}
		if _, exists := revisions[id]; !exists {
			allCurrentProvidersCovered = false
			continue
		}
		delete(r.stale, id)
	}
	if allCurrentProvidersCovered {
		delete(r.stalePatterns, pattern)
	}
	if r.activeCredentialPatternSyncs[pattern] == generation {
		delete(r.activeCredentialPatternSyncs, pattern)
	}
	return nil
}

// MarkCredentialPatternStale bloqueia snapshots cujas credenciais mudaram mas
// cuja nova revisão ainda não foi confirmada no registry.
func (r *ProviderRegistry) MarkCredentialPatternStale(pattern string) int {
	if r == nil || pattern == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, marked := r.markCredentialPatternStaleLocked(pattern)
	return marked
}

func (r *ProviderRegistry) markCredentialPatternStaleLocked(pattern string) (uint64, int) {
	r.credentialPatternSyncGeneration[pattern]++
	delete(r.activeCredentialPatternSyncs, pattern)
	r.stalePatterns[pattern] = true
	marked := 0
	for id, provider := range r.providers {
		if provider.CredentialPattern == pattern {
			r.stale[id] = true
			marked++
		}
	}
	return r.credentialPatternSyncGeneration[pattern], marked
}

// Get retorna um provider pelo ID
func (r *ProviderRegistry) Get(id string) *ProviderConfig {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stale[id] {
		return nil
	}
	return cloneProviderConfiguration(r.providers[id])
}

// List retorna todos os providers (ordenados por ID)
func (r *ProviderRegistry) List() []*ProviderConfig {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*ProviderConfig, 0, len(r.providers))
	for id, provider := range r.providers {
		if r.stale[id] {
			continue
		}
		list = append(list, cloneProviderConfiguration(provider))
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generation++
	r.providers = make(map[string]*ProviderConfig)
	r.revisionWatermarks = make(map[string]int)
	r.revisionWatermarkPatterns = make(map[string]map[string]struct{})
	r.configRevisionWatermarks = make(map[string]int)
	r.configSnapshots = make(map[string]*ProviderConfig)
	r.stale = make(map[string]bool)
	r.stalePatterns = make(map[string]bool)
	r.credentialPatternSyncGeneration = make(map[string]uint64)
	r.activeCredentialPatternSyncs = make(map[string]uint64)
}

// Remove remove um provider pelo ID
func (r *ProviderRegistry) Remove(id string) error {
	if r == nil {
		return fmt.Errorf("registry nil")
	}
	if !r.RemoveIfPresent(id) {
		return fmt.Errorf("provider not found: %s", id)
	}
	return nil
}

// RemoveIfPresent remove um provider caso ele exista, mesmo quando seu
// snapshot está oculto por estar stale. Retorna se havia estado para remover.
func (r *ProviderRegistry) RemoveIfPresent(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[id]; !exists {
		return false
	}
	removed, _ := r.removeProviderLocked(id, "", false)
	return removed
}

// RemoveProvider remove o ID e invalida leituras de credenciais em andamento
// para o pattern autoritativo, mesmo quando só há watermarks e nenhum snapshot.
// Retorna os patterns que ainda precisam de uma leitura para recuperar snapshots.
func (r *ProviderRegistry) RemoveProvider(id, credentialPattern string) (bool, []string) {
	if r == nil {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.removeProviderLocked(id, credentialPattern, true)
}

func (r *ProviderRegistry) removeProviderLocked(id, credentialPattern string, invalidateActivePatterns bool) (bool, []string) {
	provider, exists := r.providers[id]
	patterns := make(map[string]struct{}, 2+len(r.revisionWatermarkPatterns[id])+len(r.activeCredentialPatternSyncs))
	patternsToRefresh := make(map[string]struct{})
	if credentialPattern != "" {
		patterns[credentialPattern] = struct{}{}
	}
	for pattern := range r.revisionWatermarkPatterns[id] {
		patterns[pattern] = struct{}{}
	}
	if invalidateActivePatterns {
		for pattern := range r.activeCredentialPatternSyncs {
			patterns[pattern] = struct{}{}
		}
	} else {
		for pattern := range patterns {
			if _, active := r.activeCredentialPatternSyncs[pattern]; active {
				patternsToRefresh[pattern] = struct{}{}
			}
		}
	}
	if exists {
		delete(r.providers, id)
		delete(r.stale, id)
		if provider.CredentialPattern != "" {
			patterns[provider.CredentialPattern] = struct{}{}
		}
	}
	delete(r.stale, id)
	delete(r.revisionWatermarks, id)
	delete(r.revisionWatermarkPatterns, id)
	delete(r.configRevisionWatermarks, id)
	delete(r.configSnapshots, id)
	for pattern := range patterns {
		// Invalidate a refresh that may have read this provider before its
		// deletion. Otherwise its late publication could restore a watermark
		// for this ID and reject a later provider created with the same ID.
		r.credentialPatternSyncGeneration[pattern]++
		delete(r.activeCredentialPatternSyncs, pattern)
	}
	for pattern := range patterns {
		r.clearOrphanedStalePatternLocked(pattern)
		if r.stalePatterns[pattern] {
			patternsToRefresh[pattern] = struct{}{}
			continue
		}
		for currentID, current := range r.providers {
			if current.CredentialPattern == pattern && r.stale[currentID] {
				patternsToRefresh[pattern] = struct{}{}
				break
			}
		}
	}
	invalidatedPatterns := make([]string, 0, len(patternsToRefresh))
	for pattern := range patternsToRefresh {
		invalidatedPatterns = append(invalidatedPatterns, pattern)
	}
	sort.Strings(invalidatedPatterns)
	return exists, invalidatedPatterns
}
