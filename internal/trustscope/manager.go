package trustscope

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/logging"
	"assistente/internal/tools/invocationctx"
)

const (
	globalFile    = "global.json"
	workspaceFile = "workspace.json"
)

// Adapter mantém no domínio consumidor a validação e a semântica das entradas.
// trustscope só conhece escopo, identidade, armazenamento e ordem de consulta.
type Adapter[E any, Q any] struct {
	Scope     func(E) Scope
	Normalize func(E) (E, error)
	Key       func(E) Q
	Matches   func(E, Q) bool
	SameKey   func(E, Q) bool
}

// Config descreve um armazenamento escopado sem acoplar trustscope ao domínio.
type Config[E any, Q any] struct {
	Subdir        string
	LogComponent  string
	LogPrefix     string
	DomainLabel   string
	NotFoundError error
	Adapter       Adapter[E, Q]
}

// Match é um resultado encontrado em um dos escopos, na ordem canônica.
type Match[E any] struct {
	Found bool
	Scope Scope
	Entry *E
}

type storeFile[E any] struct {
	Version int `json:"version"`
	Entries []E `json:"entries"`
}

// Manager implementa sessão, identidade e persistência dos escopos comuns.
type Manager[E any, Q any] struct {
	mu      sync.RWMutex
	fileMu  sync.RWMutex
	session map[string][]E

	homeDir           func() string
	workDir           func() string
	activeProfileSlug func() string
	config            Config[E, Q]
}

// NewManager cria um Manager com os diretórios canônicos do configdir.
func NewManager[E any, Q any](config Config[E, Q]) *Manager[E, Q] {
	return newManager(config, configdir.GetHomeDir, configdir.GetWorkDir)
}

// NewManagerWithDirs cria um Manager com diretórios fixos, útil em testes.
func NewManagerWithDirs[E any, Q any](config Config[E, Q], homeDir, workDir string) *Manager[E, Q] {
	return newManager(config, func() string { return homeDir }, func() string { return workDir })
}

func newManager[E any, Q any](config Config[E, Q], homeDir, workDir func() string) *Manager[E, Q] {
	return &Manager[E, Q]{
		session: make(map[string][]E),
		homeDir: homeDir,
		workDir: workDir,
		config:  config,
	}
}

// SetActiveProfileSlugFunc injeta o fallback do perfil ativo.
func (m *Manager[E, Q]) SetActiveProfileSlugFunc(f func() string) {
	m.mu.Lock()
	m.activeProfileSlug = f
	m.mu.Unlock()
}

// SetWorkspaceDirFunc injeta o workspace ativo, com fallback para configdir.
func (m *Manager[E, Q]) SetWorkspaceDirFunc(f func() string) {
	if f == nil {
		return
	}
	m.mu.Lock()
	m.workDir = func() string {
		if dir := f(); dir != "" {
			return dir
		}
		return configdir.GetWorkDir()
	}
	m.mu.Unlock()
}

// ClearSession remove as entradas de uma conversa. ID vazio é no-op.
func (m *Manager[E, Q]) ClearSession(conversationID string) {
	if conversationID == "" {
		return
	}
	m.mu.Lock()
	delete(m.session, conversationID)
	m.mu.Unlock()
}

// Find consulta sessão → perfil → workspace → global.
// Arquivo ausente ou inválido nunca concede trust (fail-closed).
func (m *Manager[E, Q]) Find(ctx context.Context, query Q) Match[E] {
	conversationID, profileSlug := m.identity(ctx)

	m.mu.RLock()
	var sessionEntries []E
	if conversationID != "" {
		sessionEntries = append(sessionEntries, m.session[conversationID]...)
	}
	profilePath := m.profilePath(profileSlug)
	workspacePath := m.workspacePath()
	globalPath := m.globalPath()
	m.mu.RUnlock()

	if entry := m.firstMatch(sessionEntries, query); entry != nil {
		return Match[E]{Found: true, Scope: ScopeSession, Entry: entry}
	}

	m.fileMu.RLock()
	defer m.fileMu.RUnlock()
	if profilePath != "" {
		if entry := m.firstMatch(m.loadFileOrEmpty(ctx, profilePath), query); entry != nil {
			return Match[E]{Found: true, Scope: ScopeProfile, Entry: entry}
		}
	}
	if entry := m.firstMatch(m.loadFileOrEmpty(ctx, workspacePath), query); entry != nil {
		return Match[E]{Found: true, Scope: ScopeWorkspace, Entry: entry}
	}
	if entry := m.firstMatch(m.loadFileOrEmpty(ctx, globalPath), query); entry != nil {
		return Match[E]{Found: true, Scope: ScopeGlobal, Entry: entry}
	}
	return Match[E]{}
}

// Add registra uma entrada. ScopeOnce é validado, mas não armazenado.
func (m *Manager[E, Q]) Add(ctx context.Context, entry E) error {
	var err error
	entry, err = m.config.Adapter.Normalize(entry)
	if err != nil {
		return err
	}
	scope := m.config.Adapter.Scope(entry)
	if !ValidScope(scope) {
		return fmt.Errorf("escopo inválido: %q", scope)
	}
	conversationID, profileSlug := m.identity(ctx)

	switch scope {
	case ScopeOnce:
		return nil
	case ScopeSession:
		if conversationID == "" {
			return fmt.Errorf("escopo de sessão requer ConversationID no contexto")
		}
		m.mu.Lock()
		m.session[conversationID] = m.upsert(m.session[conversationID], entry)
		m.mu.Unlock()
		return nil
	case ScopeWorkspace:
		return m.addPersistent(m.workspacePathLocked(), entry, scope)
	case ScopeProfile:
		if profileSlug == "" {
			return fmt.Errorf("escopo de perfil requer ProfileSlug no contexto")
		}
		return m.addPersistent(m.profilePathLocked(profileSlug), entry, scope)
	case ScopeGlobal:
		return m.addPersistent(m.globalPathLocked(), entry, scope)
	default:
		return fmt.Errorf("escopo não suportado: %q", scope)
	}
}

// List devolve entradas na mesma ordem de escopos usada por Find.
func (m *Manager[E, Q]) List(ctx context.Context) []E {
	conversationID, profileSlug := m.identity(ctx)
	var out []E

	m.mu.RLock()
	if conversationID != "" {
		out = append(out, m.session[conversationID]...)
	}
	profilePath := m.profilePath(profileSlug)
	workspacePath := m.workspacePath()
	globalPath := m.globalPath()
	m.mu.RUnlock()

	m.fileMu.RLock()
	defer m.fileMu.RUnlock()
	if profilePath != "" {
		out = append(out, m.loadFileOrEmpty(ctx, profilePath)...)
	}
	out = append(out, m.loadFileOrEmpty(ctx, workspacePath)...)
	out = append(out, m.loadFileOrEmpty(ctx, globalPath)...)
	return out
}

// Remove apaga a primeira entrada com a chave informada no escopo.
func (m *Manager[E, Q]) Remove(ctx context.Context, scope Scope, query Q) error {
	conversationID, profileSlug := m.identity(ctx)
	switch scope {
	case ScopeSession:
		if conversationID == "" {
			return fmt.Errorf("escopo de sessão requer ConversationID no contexto")
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		entries, removed := m.removeMatch(m.session[conversationID], query)
		if !removed {
			return m.notFoundError()
		}
		m.session[conversationID] = entries
		return nil
	case ScopeWorkspace:
		return m.removePersistent(m.workspacePathLocked(), query)
	case ScopeProfile:
		if profileSlug == "" {
			return fmt.Errorf("escopo de perfil requer ProfileSlug no contexto")
		}
		return m.removePersistent(m.profilePathLocked(profileSlug), query)
	case ScopeGlobal:
		return m.removePersistent(m.globalPathLocked(), query)
	default:
		return fmt.Errorf("escopo não removível: %q", scope)
	}
}

func (m *Manager[E, Q]) addPersistent(path string, entry E, scope Scope) error {
	if path == "" {
		return fmt.Errorf("caminho de allowlist indisponível para o escopo %q", scope)
	}
	m.fileMu.Lock()
	defer m.fileMu.Unlock()
	existing, err := m.loadFile(path)
	if err != nil {
		return fmt.Errorf("não foi possível ler a allowlist antes de gravar (evitando perda de dados): %w", err)
	}
	return m.saveFile(path, m.upsert(existing, entry))
}

func (m *Manager[E, Q]) removePersistent(path string, query Q) error {
	if path == "" {
		return fmt.Errorf("caminho de allowlist indisponível")
	}
	m.fileMu.Lock()
	defer m.fileMu.Unlock()
	existing, err := m.loadFile(path)
	if err != nil {
		return fmt.Errorf("não foi possível ler a allowlist antes de remover: %w", err)
	}
	entries, removed := m.removeMatch(existing, query)
	if !removed {
		return m.notFoundError()
	}
	return m.saveFile(path, entries)
}

func (m *Manager[E, Q]) loadFile(path string) ([]E, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao ler allowlist de %s %s: %w", m.config.DomainLabel, path, err)
	}
	var file storeFile[E]
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("allowlist de %s inválida %s: %w", m.config.DomainLabel, path, err)
	}
	return file.Entries, nil
}

func (m *Manager[E, Q]) loadFileOrEmpty(ctx context.Context, path string) []E {
	entries, err := m.loadFile(path)
	if err != nil {
		logging.Errorf(ctx, m.config.LogComponent, "%s %v", m.config.LogPrefix, err)
		return nil
	}
	return entries
}

func (m *Manager[E, Q]) saveFile(path string, entries []E) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("erro ao criar diretório de allowlist de %s: %w", m.config.DomainLabel, err)
	}
	data, err := json.MarshalIndent(storeFile[E]{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar allowlist de %s: %w", m.config.DomainLabel, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".allowlist-*.tmp")
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo temporário de allowlist de %s: %w", m.config.DomainLabel, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao gravar allowlist de %s: %w", m.config.DomainLabel, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao finalizar allowlist de %s: %w", m.config.DomainLabel, err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao substituir allowlist de %s: %w", m.config.DomainLabel, err)
	}
	return nil
}

func replaceFile(tmpName, path string) error {
	const attempts = 5
	var err error
	for i := 0; i < attempts; i++ {
		if err = os.Rename(tmpName, path); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
}

func (m *Manager[E, Q]) firstMatch(entries []E, query Q) *E {
	for i := range entries {
		if m.config.Adapter.Matches(entries[i], query) {
			entry := entries[i]
			return &entry
		}
	}
	return nil
}

func (m *Manager[E, Q]) upsert(entries []E, entry E) []E {
	query := m.config.Adapter.Key(entry)
	for i := range entries {
		if m.config.Adapter.SameKey(entries[i], query) {
			entries[i] = entry
			return entries
		}
	}
	return append(entries, entry)
}

func (m *Manager[E, Q]) removeMatch(entries []E, query Q) ([]E, bool) {
	out := make([]E, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if !removed && m.config.Adapter.SameKey(entry, query) {
			removed = true
			continue
		}
		out = append(out, entry)
	}
	return out, removed
}

func (m *Manager[E, Q]) identity(ctx context.Context) (conversationID, profileSlug string) {
	if invocation, ok := invocationctx.Get(ctx); ok {
		conversationID, profileSlug = invocation.ConversationID, invocation.ProfileSlug
	}
	if profileSlug != "" {
		return conversationID, profileSlug
	}
	m.mu.RLock()
	fallback := m.activeProfileSlug
	m.mu.RUnlock()
	if fallback != nil {
		profileSlug = fallback()
	}
	return conversationID, profileSlug
}

func (m *Manager[E, Q]) globalPath() string {
	base := m.homeDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, m.config.Subdir, globalFile)
}

func (m *Manager[E, Q]) workspacePath() string {
	base := m.workDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, m.config.Subdir, workspaceFile)
}

func (m *Manager[E, Q]) profilePath(slug string) string {
	slug = sanitizeSlug(slug)
	base := m.homeDir()
	if slug == "" || base == "" {
		return ""
	}
	return filepath.Join(base, m.config.Subdir, "profile-"+slug+".json")
}

// WorkspacePath devolve o arquivo persistente do workspace resolvido agora.
func (m *Manager[E, Q]) WorkspacePath() string {
	return m.workspacePathLocked()
}

func (m *Manager[E, Q]) globalPathLocked() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.globalPath()
}

func (m *Manager[E, Q]) workspacePathLocked() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.workspacePath()
}

func (m *Manager[E, Q]) profilePathLocked(slug string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.profilePath(slug)
}

func (m *Manager[E, Q]) notFoundError() error {
	if m.config.NotFoundError != nil {
		return m.config.NotFoundError
	}
	return errors.New("entrada de trust não encontrada")
}

func sanitizeSlug(slug string) string {
	slug = strings.TrimSpace(strings.ToLower(slug))
	slug = strings.ReplaceAll(slug, "..", "")
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, slug)
}
