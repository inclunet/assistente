package docextract

import (
	"container/list"
	"context"
	"fmt"
	"sync"
)

const (
	defaultCacheEntries = 64
	defaultCacheBytes   = 64 << 20 // 64 MiB de Markdown derivado
)

// FileIdentity identifica a versão do arquivo sem guardar seu conteúdo. Mtime e
// tamanho são suficientes no caminho normal; quem observa uma fonte em que essa
// identidade não é confiável pode preencher Digest com um hash.
type FileIdentity struct {
	Size            int64
	ModTimeUnixNano int64
	Digest          string
}

// CacheConfig limita quanto texto derivado sensível pode permanecer em memória.
type CacheConfig struct {
	MaxEntries int
	MaxBytes   int64
}

// DefaultCacheConfig devolve os limites usados pelas tools do aplicativo.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{MaxEntries: defaultCacheEntries, MaxBytes: defaultCacheBytes}
}

type cacheKey struct {
	path     string
	identity FileIdentity
}

type cacheEntry struct {
	key    cacheKey
	result *Result
	bytes  int64
}

type cacheFlight struct {
	done      chan struct{}
	cancel    context.CancelFunc
	waiters   int
	completed bool
	result    *Result
	err       error
}

// ProjectionCache guarda somente projeções em memória. Não há persistência,
// índice no boot nem cópia plaintext fora do processo (AEP-0093, D6).
type ProjectionCache struct {
	mu         sync.Mutex
	maxEntries int
	maxBytes   int64
	bytes      int64
	entries    map[cacheKey]*list.Element
	byPath     map[string]cacheKey
	lru        *list.List
	flights    map[cacheKey]*cacheFlight
}

// NewProjectionCache cria um cache LRU limitado por entradas e bytes.
func NewProjectionCache(config CacheConfig) *ProjectionCache {
	if config.MaxEntries <= 0 {
		config.MaxEntries = defaultCacheEntries
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = defaultCacheBytes
	}
	return &ProjectionCache{
		maxEntries: config.MaxEntries,
		maxBytes:   config.MaxBytes,
		entries:    make(map[cacheKey]*list.Element),
		byPath:     make(map[string]cacheKey),
		lru:        list.New(),
		flights:    make(map[cacheKey]*cacheFlight),
	}
}

// LoadOrigin diz de onde veio a projeção devolvida. Quem mede custo precisa
// distinguir a chamada que pagou a extração da que pegou carona em outra em
// andamento: as duas são "miss" no cache, mas só a primeira extraiu.
type LoadOrigin int

const (
	// OriginLoaded: esta chamada executou a extração.
	OriginLoaded LoadOrigin = iota
	// OriginCached: veio de uma entrada já presente no cache.
	OriginCached
	// OriginCoalesced: pegou carona em uma extração já em andamento.
	OriginCoalesced
)

func (o LoadOrigin) String() string {
	switch o {
	case OriginCached:
		return "cached"
	case OriginCoalesced:
		return "coalesced"
	default:
		return "loaded"
	}
}

// GetOrLoad devolve a projeção da identidade atual ou chama load uma vez. Cargas
// concorrentes do mesmo arquivo/identidade são coalescidas. A carga tem contexto
// próprio e só é cancelada quando todos os interessados desistem: o contexto do
// primeiro chamador não governa os demais.
func (c *ProjectionCache) GetOrLoad(
	ctx context.Context,
	path string,
	identity FileIdentity,
	load func(context.Context) (*Result, error),
) (*Result, LoadOrigin, error) {
	if c == nil {
		result, err := load(ctx)
		return result, OriginLoaded, err
	}
	key := cacheKey{path: path, identity: identity}

	c.mu.Lock()
	if elem, ok := c.entries[key]; ok {
		c.lru.MoveToFront(elem)
		result := cloneResult(elem.Value.(*cacheEntry).result)
		c.mu.Unlock()
		return result, OriginCached, nil
	}
	if flight, ok := c.flights[key]; ok {
		flight.waiters++
		c.mu.Unlock()
		return c.waitForFlight(ctx, key, flight, OriginCoalesced)
	}
	loadCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	flight := &cacheFlight{done: make(chan struct{}), cancel: cancel, waiters: 1}
	c.flights[key] = flight
	c.mu.Unlock()

	go c.runFlight(key, flight, loadCtx, load)
	return c.waitForFlight(ctx, key, flight, OriginLoaded)
}

func (c *ProjectionCache) runFlight(
	key cacheKey,
	flight *cacheFlight,
	ctx context.Context,
	load func(context.Context) (*Result, error),
) {
	result, err := load(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		result, err = nil, ctxErr
	}
	c.mu.Lock()
	current, stillCurrent := c.flights[key]
	if stillCurrent && current == flight && err == nil && result != nil {
		c.putLocked(key, result)
		flight.result = cloneResult(result)
	}
	flight.err = err
	flight.completed = true
	flight.cancel()
	if stillCurrent && current == flight {
		delete(c.flights, key)
	}
	close(flight.done)
	c.mu.Unlock()
}

func (c *ProjectionCache) waitForFlight(
	ctx context.Context,
	key cacheKey,
	flight *cacheFlight,
	origin LoadOrigin,
) (*Result, LoadOrigin, error) {
	select {
	case <-flight.done:
		return cloneResult(flight.result), origin, flight.err
	case <-ctx.Done():
		c.mu.Lock()
		if current, ok := c.flights[key]; ok && current == flight && !flight.completed {
			flight.waiters--
			if flight.waiters == 0 {
				flight.cancel()
				// Uma chamada posterior não deve herdar um voo que já não tem
				// interessados e cujo contexto foi cancelado.
				delete(c.flights, key)
			}
		}
		c.mu.Unlock()
		return nil, origin, ctx.Err()
	}
}

func (c *ProjectionCache) putLocked(key cacheKey, result *Result) {
	// Uma identidade nova invalida imediatamente a projeção anterior do path.
	// A invalidação vem antes do corte por tamanho: se a projeção nova não cabe,
	// a antiga ainda assim está obsoleta e não pode continuar em memória.
	if oldKey, ok := c.byPath[key.path]; ok && oldKey != key {
		c.removeLocked(oldKey)
	}

	bytes := resultBytes(result)
	if bytes > c.maxBytes {
		return
	}
	if elem, ok := c.entries[key]; ok {
		entry := elem.Value.(*cacheEntry)
		c.bytes -= entry.bytes
		entry.result = cloneResult(result)
		entry.bytes = bytes
		c.bytes += bytes
		c.lru.MoveToFront(elem)
	} else {
		entry := &cacheEntry{key: key, result: cloneResult(result), bytes: bytes}
		c.entries[key] = c.lru.PushFront(entry)
		c.byPath[key.path] = key
		c.bytes += bytes
	}
	for len(c.entries) > c.maxEntries || c.bytes > c.maxBytes {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		c.removeLocked(oldest.Value.(*cacheEntry).key)
	}
}

func (c *ProjectionCache) removeLocked(key cacheKey) {
	elem, ok := c.entries[key]
	if !ok {
		return
	}
	entry := elem.Value.(*cacheEntry)
	delete(c.entries, key)
	if current, ok := c.byPath[key.path]; ok && current == key {
		delete(c.byPath, key.path)
	}
	c.lru.Remove(elem)
	c.bytes -= entry.bytes
}

func resultBytes(result *Result) int64 {
	if result == nil {
		return 0
	}
	total := len(result.Markdown) + len(result.Source) + len(result.Kind)
	for _, warning := range result.Warnings {
		total += len(warning)
	}
	return int64(total)
}

func cloneResult(result *Result) *Result {
	if result == nil {
		return nil
	}
	clone := *result
	clone.Warnings = append([]string(nil), result.Warnings...)
	return &clone
}

// FileIdentityFromStat monta a identidade usada pelas tools sem acoplar este
// pacote a os.FileInfo.
func FileIdentityFromStat(size int64, modTimeUnixNano int64) FileIdentity {
	return FileIdentity{Size: size, ModTimeUnixNano: modTimeUnixNano}
}

func (i FileIdentity) String() string {
	if i.Digest != "" {
		return fmt.Sprintf("%d:%d:%s", i.Size, i.ModTimeUnixNano, i.Digest)
	}
	return fmt.Sprintf("%d:%d", i.Size, i.ModTimeUnixNano)
}
