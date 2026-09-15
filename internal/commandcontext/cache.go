package commandcontext

import (
	"container/list"
	"errors"
	"sync"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

var (
	ErrInvalidCacheKey      = errors.New("chave de cache contextual inválida")
	ErrInvalidCacheCapacity = errors.New("capacidade de cache contextual inválida")
)

// CacheKey é exatamente a tupla de D12. WorkspaceID permanece ponteiro para
// distinguir nil global de um workspace concreto sem usar string vazia.
type CacheKey struct {
	UserID                    string
	WorkspaceID               *string
	TriggerIdentity           string
	SourceType                commandcatalog.Source
	ContextVersion            string
	RegistryVersion           string
	GlobalConfigGeneration    string
	WorkspaceConfigGeneration *string
	ActiveLayersGeneration    string
}

type cacheKey struct {
	userID                    string
	workspaceID               string
	hasWorkspace              bool
	triggerIdentity           string
	sourceType                commandcatalog.Source
	contextVersion            string
	registryVersion           string
	globalConfigGeneration    string
	workspaceConfigGeneration string
	hasWorkspaceConfig        bool
	activeLayersGeneration    string
}

func (k CacheKey) validate() error {
	if k.UserID == "" || k.TriggerIdentity == "" || k.SourceType == "" || k.RegistryVersion == "" || k.GlobalConfigGeneration == "" || k.ActiveLayersGeneration == "" {
		return ErrInvalidCacheKey
	}
	if k.WorkspaceID != nil && *k.WorkspaceID == "" {
		return ErrInvalidCacheKey
	}
	if k.WorkspaceID == nil && k.WorkspaceConfigGeneration != nil {
		return ErrInvalidCacheKey
	}
	if k.WorkspaceID != nil && (k.WorkspaceConfigGeneration == nil || *k.WorkspaceConfigGeneration == "") {
		return ErrInvalidCacheKey
	}
	return nil
}

func (k CacheKey) internal() (cacheKey, error) {
	if err := k.validate(); err != nil {
		return cacheKey{}, err
	}
	out := cacheKey{
		userID: k.UserID, triggerIdentity: k.TriggerIdentity, sourceType: k.SourceType,
		contextVersion: k.ContextVersion, registryVersion: k.RegistryVersion,
		globalConfigGeneration: k.GlobalConfigGeneration,
		activeLayersGeneration: k.ActiveLayersGeneration,
	}
	if k.WorkspaceConfigGeneration != nil {
		out.workspaceConfigGeneration, out.hasWorkspaceConfig = *k.WorkspaceConfigGeneration, true
	}
	if k.WorkspaceID != nil {
		out.workspaceID, out.hasWorkspace = *k.WorkspaceID, true
	}
	return out, nil
}

// NewCacheKey compõe a chave D12 sem perder o nil tipado do workspace.
func NewCacheKey(scope Scope, triggerIdentity string, sourceType commandcatalog.Source, contextVersion, registryVersion string, generations GenerationSnapshot) (CacheKey, error) {
	if err := scope.validate(); err != nil {
		return CacheKey{}, err
	}
	key := CacheKey{
		UserID: scope.UserID, WorkspaceID: cloneStringPointer(scope.WorkspaceID),
		TriggerIdentity: triggerIdentity, SourceType: sourceType,
		ContextVersion: contextVersion, RegistryVersion: registryVersion,
		GlobalConfigGeneration:    generations.GlobalConfigGeneration,
		WorkspaceConfigGeneration: cloneStringPointer(generations.WorkspaceConfigGeneration),
		ActiveLayersGeneration:    generations.ActiveLayersGeneration,
	}
	if err := key.validate(); err != nil {
		return CacheKey{}, err
	}
	return key, nil
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type cacheEntry struct {
	key    cacheKey
	result commandbindings.Result
}

// ResolutionCache é um LRU bounded de resultados detached. Tanto Selected
// quanto NoMatch/Blocked/Conflict podem ser armazenados; erros de resolução
// não são resultados e não entram no cache.
type ResolutionCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[cacheKey]*list.Element
	order    *list.List
}

// ResultCache é um alias compatível com o nome usado pelo resolvedor.
type ResultCache = ResolutionCache

func NewResolutionCache(capacity int) (*ResolutionCache, error) {
	if capacity <= 0 {
		return nil, ErrInvalidCacheCapacity
	}
	return &ResolutionCache{capacity: capacity, entries: make(map[cacheKey]*list.Element, capacity), order: list.New()}, nil
}

func MustNewResolutionCache(capacity int) *ResolutionCache {
	cache, err := NewResolutionCache(capacity)
	if err != nil {
		panic(err)
	}
	return cache
}

func NewResultCache(capacity int) (*ResolutionCache, error) {
	return NewResolutionCache(capacity)
}

func cloneResult(result commandbindings.Result) commandbindings.Result {
	result.BindingIDs = append([]string(nil), result.BindingIDs...)
	return result
}

// Get retorna uma cópia do resultado e nunca entrega o slice interno.
func (c *ResolutionCache) Get(key CacheKey) (commandbindings.Result, bool) {
	if c == nil {
		return commandbindings.Result{}, false
	}
	internal, err := key.internal()
	if err != nil {
		return commandbindings.Result{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[internal]
	if !ok {
		return commandbindings.Result{}, false
	}
	c.order.MoveToFront(element)
	return cloneResult(element.Value.(cacheEntry).result), true
}

// Put publica um resultado detached. O cache é limitado por capacity e
// remove o item menos recentemente usado quando necessário.
func (c *ResolutionCache) Put(key CacheKey, result commandbindings.Result) error {
	if c == nil {
		return ErrInvalidCacheKey
	}
	internal, err := key.internal()
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[internal]; ok {
		element.Value = cacheEntry{key: internal, result: cloneResult(result)}
		c.order.MoveToFront(element)
		return nil
	}
	element := c.order.PushFront(cacheEntry{key: internal, result: cloneResult(result)})
	c.entries[internal] = element
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		delete(c.entries, oldest.Value.(cacheEntry).key)
		c.order.Remove(oldest)
	}
	return nil
}

func (c *ResolutionCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// InvalidateScope remove só as entradas do usuário e workspace informado.
// Chaves de outro usuário ou de outro workspace permanecem válidas.
func (c *ResolutionCache) InvalidateScope(scope Scope) error {
	if c == nil {
		return ErrInvalidCacheKey
	}
	if err := scope.validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, element := range c.entries {
		if key.userID != scope.UserID || key.hasWorkspace != (scope.WorkspaceID != nil) {
			continue
		}
		if key.hasWorkspace && key.workspaceID != *scope.WorkspaceID {
			continue
		}
		delete(c.entries, key)
		c.order.Remove(element)
	}
	return nil
}

func (c *ResolutionCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[cacheKey]*list.Element, c.capacity)
	c.order.Init()
}
