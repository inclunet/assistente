package llmcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"assistente/internal/configdir"
	httpclient "assistente/internal/tools/http"
)

// URL é a única origem aceita para o catálogo público.
const URL = "https://assistente.inclunet.com.br/json/assistente-catalog.json"

const (
	DefaultTTL      = 24 * time.Hour
	requestTimeout  = 15 * time.Second
	maxCatalogBytes = 2 << 20
	maxCacheBytes   = maxCatalogBytes + (16 << 10)
	cacheDirName    = "llm-catalog"
	cacheFileName   = "catalog.json"
)

// Doer permite injetar transporte determinístico em testes. Em produção o
// cliente compartilhado do aplicativo mantém a política de rede e proxy.
type Doer interface {
	Do(context.Context, *http.Request) (*http.Response, error)
}

type Config struct {
	HTTP Doer
	Dir  string
	TTL  time.Duration
}

// Snapshot é o documento servido junto do estado de frescor do cache.
type Snapshot struct {
	Document  Document
	FetchedAt time.Time
	Age       time.Duration
	// FromCache indica que o documento servido veio do payload persistido; uma
	// resposta 304 revalida esse payload, mas não fornece um documento novo.
	FromCache  bool
	Stale      bool
	Refreshing bool
	ReasonCode string
}

type cacheEnvelope struct {
	FetchedAt    time.Time       `json:"fetched_at"`
	ETag         string          `json:"etag,omitempty"`
	LastModified string          `json:"last_modified,omitempty"`
	Document     json.RawMessage `json:"document"`
}

// Service busca o documento de uma origem fixa, valida antes de publicar e
// conserva no disco somente o último documento válido.
type Service struct {
	http Doer
	path string
	ttl  time.Duration
	now  func() time.Time

	mu           sync.RWMutex
	snapshot     Snapshot
	etag         string
	lastModified string
	loaded       bool
	started      bool
	closed       bool
	refreshing   atomic.Bool
	background   sync.WaitGroup
}

func New(cfg Config) *Service {
	client := cfg.HTTP
	if client == nil {
		client = httpclient.New(&httpclient.Config{
			Timeout: requestTimeout,
			// O catálogo usa uma origem HTTPS fixa. Não seguir redirects impede
			// que uma resposta remota transforme o fetch em acesso a outro host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}, map[string]string{})
	}
	dir := cfg.Dir
	if dir == "" {
		if home := configdir.GetHomeDir(); home != "" {
			dir = filepath.Join(home, cacheDirName)
		}
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	path := ""
	if dir != "" {
		path = filepath.Join(dir, cacheFileName)
	}
	return &Service{http: client, path: path, ttl: ttl, now: time.Now}
}

// Start carrega o cache local e agenda uma atualização sem bloquear a abertura
// da interface. Chamadas repetidas são seguras.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started || s.closed {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.background.Add(1)
	s.mu.Unlock()
	defer s.background.Done()

	s.loadCache()
	s.mu.RLock()
	needsRefresh := !s.loaded || s.now().Sub(s.snapshot.FetchedAt) >= s.ttl
	s.mu.RUnlock()
	if needsRefresh {
		s.refreshInBackground(ctx)
	}
}

func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	snapshot := s.snapshot
	snapshot.Document = snapshot.Document.Clone()
	loaded := s.loaded
	s.mu.RUnlock()
	if loaded {
		snapshot.Age = max(time.Duration(0), s.now().Sub(snapshot.FetchedAt))
		snapshot.Stale = snapshot.Age >= s.ttl
	}
	snapshot.Refreshing = s.refreshing.Load()
	return snapshot
}

// Refresh tenta atualizar explicitamente. Chamadas sobrepostas são
// coalescidas e retornam o snapshot corrente do serviço, que pode refletir
// outra atualização já publicada. Falha mantém o último documento validado.
func (s *Service) Refresh(ctx context.Context) (snapshot Snapshot) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return s.Snapshot()
	}
	if !s.refreshing.CompareAndSwap(false, true) {
		s.mu.Unlock()
		return s.Snapshot()
	}
	s.background.Add(1)
	s.mu.Unlock()
	defer func() {
		s.refreshing.Store(false)
		snapshot = s.Snapshot()
		s.background.Done()
	}()
	_ = s.refresh(ctx)
	return
}

func (s *Service) refreshInBackground(ctx context.Context) {
	s.mu.Lock()
	if s.closed || !s.refreshing.CompareAndSwap(false, true) {
		s.mu.Unlock()
		return
	}
	s.background.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.background.Done()
		defer s.refreshing.Store(false)
		_ = s.refresh(ctx)
	}()
}

// Close aguarda inicialização e atualizações em segundo plano e explícitas;
// o contexto de startup deve ser cancelado antes para interromper a busca.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.background.Wait()
}

func (s *Service) refresh(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Assistente/1.0 (LLM Catalog)")
	s.mu.RLock()
	etag, modified := s.etag, s.lastModified
	loaded := s.loaded
	s.mu.RUnlock()
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if modified != "" {
		req.Header.Set("If-Modified-Since", modified)
	}
	resp, err := s.http.Do(ctx, req)
	if err != nil {
		s.setReason("unreachable")
		return err
	}
	// A resposta já foi obtida; falha de limpeza do body não invalida um
	// catálogo que foi lido, validado e persistido com sucesso.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified {
		if !loaded {
			s.setReason("malformed_catalog")
			return errors.New("resposta 304 sem catálogo em cache")
		}
		s.mu.RLock()
		document := s.snapshot.Document.Clone()
		etag := firstNonEmpty(resp.Header.Get("ETag"), s.etag)
		modified := firstNonEmpty(resp.Header.Get("Last-Modified"), s.lastModified)
		s.mu.RUnlock()
		fetchedAt := s.now()
		envelope, err := makeCacheEnvelope(document, fetchedAt, etag, modified)
		if err != nil {
			s.setReason("cache_write_failed")
			return err
		}
		if err := s.writeCache(envelope); err != nil {
			s.setReason("cache_write_failed")
			return err
		}
		s.mu.Lock()
		s.snapshot.FetchedAt = fetchedAt
		s.snapshot.FromCache = true
		s.snapshot.ReasonCode = ""
		s.etag = etag
		s.lastModified = modified
		s.mu.Unlock()
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		s.setReason("bad_status")
		return fmt.Errorf("catálogo respondeu HTTP %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		s.setReason("malformed_catalog")
		return fmt.Errorf("content-type inesperado: %s", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogBytes+1))
	if err != nil {
		s.setReason("unreachable")
		return err
	}
	if len(body) > maxCatalogBytes {
		s.setReason("malformed_catalog")
		return errors.New("catálogo excede o limite de tamanho")
	}
	document, err := Decode(body)
	if err != nil {
		s.setReason("malformed_catalog")
		return err
	}
	fetchedAt := s.now()
	etag = resp.Header.Get("ETag")
	modified = resp.Header.Get("Last-Modified")
	envelope, err := makeCacheEnvelope(document, fetchedAt, etag, modified)
	if err != nil {
		s.setReason("cache_write_failed")
		return err
	}
	if err := s.writeCache(envelope); err != nil {
		s.setReason("cache_write_failed")
		return err
	}
	s.mu.Lock()
	s.snapshot = Snapshot{Document: document, FetchedAt: fetchedAt, FromCache: false}
	s.loaded = true
	s.etag = etag
	s.lastModified = modified
	s.mu.Unlock()
	return nil
}

func (s *Service) setReason(reason string) {
	s.mu.Lock()
	s.snapshot.ReasonCode = reason
	s.mu.Unlock()
}

func (s *Service) loadCache() {
	if s.path == "" {
		return
	}
	file, err := os.Open(s.path)
	if err != nil {
		return
	}
	body, err := io.ReadAll(io.LimitReader(file, maxCacheBytes+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || len(body) > maxCacheBytes {
		return
	}
	var envelope cacheEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.FetchedAt.IsZero() || len(envelope.Document) == 0 {
		return
	}
	document, err := Decode(envelope.Document)
	if err != nil {
		return
	}
	now := s.now()
	if envelope.FetchedAt.After(now) {
		// Mantém o último documento válido, mas força revalidação após desvio
		// do relógio ou adulteração do timestamp, sem tratá-lo como fresco.
		envelope.FetchedAt = now.Add(-s.ttl)
	}
	s.mu.Lock()
	s.snapshot = Snapshot{Document: document, FetchedAt: envelope.FetchedAt, FromCache: true}
	s.etag = envelope.ETag
	s.lastModified = envelope.LastModified
	s.loaded = true
	s.mu.Unlock()
}

func makeCacheEnvelope(document Document, fetchedAt time.Time, etag, lastModified string) (cacheEnvelope, error) {
	body, err := json.Marshal(document)
	if err != nil {
		return cacheEnvelope{}, err
	}
	return cacheEnvelope{FetchedAt: fetchedAt, ETag: etag, LastModified: lastModified, Document: body}, nil
}

func (s *Service) writeCache(envelope cacheEnvelope) (retErr error) {
	if s.path == "" {
		return nil
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".catalog-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err := os.Remove(tmpName); err != nil && !errors.Is(err, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("remover arquivo temporário do catálogo: %w", err))
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if _, err := tmp.Write(body); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Sync(); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
