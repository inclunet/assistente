package fstrust

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

// ErrEntryNotFound é devolvido por Remove quando nenhuma entrada casa com o
// (path, kind, operation) informado no escopo.
var ErrEntryNotFound = errors.New("entrada de allowlist de path não encontrada")

// subdir é o subdiretório dentro de .assistente/ onde as allowlists de path
// persistentes ficam.
const subdir = "path-allowlist"

const (
	globalFile    = "global.json"
	workspaceFile = "workspace.json"
)

// storeFile é o formato em disco de uma allowlist de path persistida.
type storeFile struct {
	Version int              `json:"version"`
	Entries []AllowlistEntry `json:"entries"`
}

// Manager guarda as allowlists de path nos 5 escopos. Sessão fica em memória
// (chaveada por ConversationID); workspace/perfil/global são arquivos JSON sob
// .assistente/path-allowlist/.
type Manager struct {
	mu      sync.RWMutex                // sessão e funções/configuração em memória
	fileMu  sync.RWMutex                // transações dos arquivos persistidos
	session map[string][]AllowlistEntry // conversationID -> entradas de sessão

	homeDir           func() string
	workDir           func() string
	activeProfileSlug func() string
}

// NewManager cria um Manager usando os diretórios canônicos do configdir.
func NewManager() *Manager {
	return &Manager{
		session: make(map[string][]AllowlistEntry),
		homeDir: configdir.GetHomeDir,
		workDir: configdir.GetWorkDir,
	}
}

// NewManagerWithDirs cria um Manager com diretórios fixos (para testes).
func NewManagerWithDirs(homeDir, workDir string) *Manager {
	return &Manager{
		session: make(map[string][]AllowlistEntry),
		homeDir: func() string { return homeDir },
		workDir: func() string { return workDir },
	}
}

// SetActiveProfileSlugFunc injeta o resolvedor do slug do perfil ativo.
func (m *Manager) SetActiveProfileSlugFunc(f func() string) {
	m.mu.Lock()
	m.activeProfileSlug = f
	m.mu.Unlock()
}

// SetWorkspaceDirFunc injeta um resolvedor dinâmico do diretório .assistente do
// workspace ATIVO. Se f devolver "", cai no configdir.
func (m *Manager) SetWorkspaceDirFunc(f func() string) {
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

// ClearSession remove todas as entradas do escopo de sessão de uma conversa.
func (m *Manager) ClearSession(conversationID string) {
	if conversationID == "" {
		return
	}
	m.mu.Lock()
	delete(m.session, conversationID)
	m.mu.Unlock()
}

// Match procura uma autorização para absPath+operation em todos os escopos, na
// ordem sessão → perfil → workspace → global.
func (m *Manager) Match(ctx context.Context, absPath, operation string) Decision {
	absPath = NormalizePath(absPath)
	convID, profileSlug := m.identity(ctx)

	m.mu.RLock()
	var sessionEntries []AllowlistEntry
	if convID != "" {
		sessionEntries = append(sessionEntries, m.session[convID]...)
	}
	profilePath := m.profilePath(profileSlug)
	workspacePath := m.workspacePath()
	globalPath := m.globalPath()
	m.mu.RUnlock()

	if e := firstMatch(sessionEntries, absPath, operation); e != nil {
		return Decision{Allowed: true, Scope: ScopeSession, Entry: e}
	}

	m.fileMu.RLock()
	defer m.fileMu.RUnlock()

	if profilePath != "" {
		if e := firstMatch(m.loadFileOrEmpty(ctx, profilePath), absPath, operation); e != nil {
			return Decision{Allowed: true, Scope: ScopeProfile, Entry: e}
		}
	}

	if e := firstMatch(m.loadFileOrEmpty(ctx, workspacePath), absPath, operation); e != nil {
		return Decision{Allowed: true, Scope: ScopeWorkspace, Entry: e}
	}

	if e := firstMatch(m.loadFileOrEmpty(ctx, globalPath), absPath, operation); e != nil {
		return Decision{Allowed: true, Scope: ScopeGlobal, Entry: e}
	}

	return Decision{Allowed: false}
}

// Add registra uma entrada no escopo indicado. ScopeOnce é no-op.
func (m *Manager) Add(ctx context.Context, entry AllowlistEntry) error {
	entry.Path = NormalizePath(entry.Path)
	if entry.Path == "" {
		return fmt.Errorf("path vazio não pode ser autorizado")
	}
	if !ValidKind(entry.Kind) {
		return fmt.Errorf("kind inválido: %q", entry.Kind)
	}
	if strings.TrimSpace(entry.Operation) == "" {
		return fmt.Errorf("operation vazia não pode ser autorizada")
	}
	if !ValidScope(entry.Scope) {
		return fmt.Errorf("escopo inválido: %q", entry.Scope)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	convID, profileSlug := m.identity(ctx)

	switch entry.Scope {
	case ScopeOnce:
		return nil
	case ScopeSession:
		if convID == "" {
			return fmt.Errorf("escopo de sessão requer ConversationID no contexto")
		}
		m.mu.Lock()
		m.session[convID] = upsert(m.session[convID], entry)
		m.mu.Unlock()
		return nil
	case ScopeWorkspace:
		m.mu.RLock()
		path := m.workspacePath()
		m.mu.RUnlock()
		m.fileMu.Lock()
		defer m.fileMu.Unlock()
		return m.addToFile(path, entry)
	case ScopeProfile:
		if profileSlug == "" {
			return fmt.Errorf("escopo de perfil requer ProfileSlug no contexto")
		}
		m.mu.RLock()
		path := m.profilePath(profileSlug)
		m.mu.RUnlock()
		m.fileMu.Lock()
		defer m.fileMu.Unlock()
		return m.addToFile(path, entry)
	case ScopeGlobal:
		m.mu.RLock()
		path := m.globalPath()
		m.mu.RUnlock()
		m.fileMu.Lock()
		defer m.fileMu.Unlock()
		return m.addToFile(path, entry)
	default:
		return fmt.Errorf("escopo não suportado: %q", entry.Scope)
	}
}

// List devolve entradas de sessão + perfil + workspace + global.
func (m *Manager) List(ctx context.Context) []AllowlistEntry {
	var out []AllowlistEntry
	convID, profileSlug := m.identity(ctx)

	m.mu.RLock()
	if convID != "" {
		out = append(out, m.session[convID]...)
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

// Remove apaga a primeira entrada que casar path+kind+operation no escopo.
func (m *Manager) Remove(ctx context.Context, scope Scope, path string, kind Kind, operation string) error {
	path = NormalizePath(path)

	convID, profileSlug := m.identity(ctx)

	switch scope {
	case ScopeSession:
		if convID == "" {
			return fmt.Errorf("escopo de sessão requer ConversationID no contexto")
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		entries, removed := removeMatch(m.session[convID], path, kind, operation)
		if !removed {
			return ErrEntryNotFound
		}
		m.session[convID] = entries
		return nil
	case ScopeWorkspace:
		m.mu.RLock()
		storePath := m.workspacePath()
		m.mu.RUnlock()
		m.fileMu.Lock()
		defer m.fileMu.Unlock()
		return m.removeFromFile(storePath, path, kind, operation)
	case ScopeProfile:
		if profileSlug == "" {
			return fmt.Errorf("escopo de perfil requer ProfileSlug no contexto")
		}
		m.mu.RLock()
		storePath := m.profilePath(profileSlug)
		m.mu.RUnlock()
		m.fileMu.Lock()
		defer m.fileMu.Unlock()
		return m.removeFromFile(storePath, path, kind, operation)
	case ScopeGlobal:
		m.mu.RLock()
		storePath := m.globalPath()
		m.mu.RUnlock()
		m.fileMu.Lock()
		defer m.fileMu.Unlock()
		return m.removeFromFile(storePath, path, kind, operation)
	default:
		return fmt.Errorf("escopo não removível: %q", scope)
	}
}

// ---- paths ----

func (m *Manager) globalPath() string {
	return filepath.Join(m.homeDir(), subdir, globalFile)
}

func (m *Manager) workspacePath() string {
	base := m.workDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, subdir, workspaceFile)
}

func (m *Manager) profilePath(slug string) string {
	slug = sanitizeSlug(slug)
	if slug == "" {
		return ""
	}
	return filepath.Join(m.homeDir(), subdir, "profile-"+slug+".json")
}

// ---- file I/O ----

func (m *Manager) loadFile(path string) ([]AllowlistEntry, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao ler allowlist de path %s: %w", path, err)
	}
	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("allowlist de path inválida %s: %w", path, err)
	}
	return sf.Entries, nil
}

func (m *Manager) loadFileOrEmpty(ctx context.Context, path string) []AllowlistEntry {
	entries, err := m.loadFile(path)
	if err != nil {
		logging.Errorf(ctx, "fstrust.manager", "[FsTrust] %v", err)
		return nil
	}
	return entries
}

func (m *Manager) addToFile(path string, entry AllowlistEntry) error {
	if path == "" {
		return fmt.Errorf("caminho de allowlist indisponível para o escopo %q", entry.Scope)
	}
	existing, err := m.loadFile(path)
	if err != nil {
		return fmt.Errorf("não foi possível ler a allowlist antes de gravar (evitando perda de dados): %w", err)
	}
	return m.saveFile(path, upsert(existing, entry))
}

func (m *Manager) removeFromFile(path string, entryPath string, kind Kind, operation string) error {
	if path == "" {
		return fmt.Errorf("caminho de allowlist indisponível")
	}
	existing, err := m.loadFile(path)
	if err != nil {
		return fmt.Errorf("não foi possível ler a allowlist antes de remover: %w", err)
	}
	entries, removed := removeMatch(existing, entryPath, kind, operation)
	if !removed {
		return ErrEntryNotFound
	}
	return m.saveFile(path, entries)
}

func (m *Manager) saveFile(path string, entries []AllowlistEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("erro ao criar diretório de allowlist de path: %w", err)
	}
	data, err := json.MarshalIndent(storeFile{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar allowlist de path: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".allowlist-*.tmp")
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo temporário de allowlist de path: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao gravar allowlist de path: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao finalizar allowlist de path: %w", err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao substituir allowlist de path: %w", err)
	}
	return nil
}

// replaceFile substitui path pelo conteúdo de tmpName via rename atômico, com
// retries para sharing violations transitórias no Windows.
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

// ---- helpers ----

func firstMatch(entries []AllowlistEntry, absPath, operation string) *AllowlistEntry {
	for i := range entries {
		if entries[i].Matches(absPath, operation) {
			e := entries[i]
			return &e
		}
	}
	return nil
}

// upsert substitui uma entrada com mesmo Path+Kind+Operation (normalizado) ou anexa.
func upsert(entries []AllowlistEntry, entry AllowlistEntry) []AllowlistEntry {
	keyPath := NormalizePath(entry.Path)
	for i := range entries {
		if NormalizePath(entries[i].Path) == keyPath &&
			entries[i].Kind == entry.Kind &&
			entries[i].Operation == entry.Operation {
			entries[i] = entry
			return entries
		}
	}
	return append(entries, entry)
}

func removeMatch(entries []AllowlistEntry, path string, kind Kind, operation string) ([]AllowlistEntry, bool) {
	keyPath := NormalizePath(path)
	out := make([]AllowlistEntry, 0, len(entries))
	removed := false
	for _, e := range entries {
		if !removed && NormalizePath(e.Path) == keyPath && e.Kind == kind && e.Operation == operation {
			removed = true
			continue
		}
		out = append(out, e)
	}
	return out, removed
}

func identityFromContext(ctx context.Context) (conversationID, profileSlug string) {
	if inv, ok := invocationctx.Get(ctx); ok {
		return inv.ConversationID, inv.ProfileSlug
	}
	return "", ""
}

func (m *Manager) identity(ctx context.Context) (conversationID, profileSlug string) {
	conversationID, profileSlug = identityFromContext(ctx)
	if profileSlug != "" {
		return conversationID, profileSlug
	}
	m.mu.RLock()
	f := m.activeProfileSlug
	m.mu.RUnlock()
	if f != nil {
		profileSlug = f()
	}
	return conversationID, profileSlug
}

func sanitizeSlug(slug string) string {
	slug = strings.TrimSpace(strings.ToLower(slug))
	slug = strings.ReplaceAll(slug, "..", "")
	slug = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, slug)
	return slug
}
