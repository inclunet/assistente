package trustscope

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"assistente/internal/authorization"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/tools/invocationctx"
)

type Adapter[E any, Q any] struct {
	Scope     func(E) Scope
	Normalize func(E) (E, error)
	Key       func(E) Q
	Matches   func(E, Q) bool
	SameKey   func(E, Q) bool
	Identity  func(E) string
	Effect    func(E) string
	Operation func(E) string
}
type Config[E any, Q any] struct {
	Domain        string
	Subdir        string
	NotFoundError error
	Adapter       Adapter[E, Q]
}

// Options permite injetar o repositório e as fontes legadas sem criar stores
// alternativos. HomeDir/WorkDir são usados apenas pela importação.
type Options struct {
	Repository *authorization.Repository
	HomeDir    func() string
	WorkDir    func() string
	MayImport  func(context.Context) bool
}
type Match[E any] struct {
	Found bool
	Scope Scope
	Entry *E
	Err   error
}
type identity struct {
	User, Conversation, Profile, Workspace string
	Generation                             uint64
}
type identityKey struct{ manager any }
type sessionKey struct{ User, Conversation string }
type Manager[E any, Q any] struct {
	mu                sync.RWMutex
	importMu          sync.Mutex
	session           map[sessionKey][]E
	generation        map[sessionKey]uint64
	activeProfileSlug func() string
	options           Options
	config            Config[E, Q]
}

func NewManager[E any, Q any](config Config[E, Q], options Options) *Manager[E, Q] {
	if options.Repository == nil {
		options.Repository = authorization.NewRepository(database.DB)
	}
	if options.HomeDir == nil {
		options.HomeDir = configdir.GetHomeDir
	}
	if options.WorkDir == nil {
		options.WorkDir = configdir.GetWorkDir
	}
	return &Manager[E, Q]{session: make(map[sessionKey][]E), generation: make(map[sessionKey]uint64), config: config, options: options}
}
func (m *Manager[E, Q]) SetActiveProfileSlugFunc(f func() string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeProfileSlug = f
}
func (m *Manager[E, Q]) SetWorkspaceDirFunc(f func() string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.options.WorkDir = f
}

// Capture congela contexto de origem antes de qualquer espera pelo usuário.
func (m *Manager[E, Q]) Capture(ctx context.Context) context.Context {
	if _, ok := ctx.Value(identityKey{m}).(identity); ok {
		return ctx
	}
	user, _ := database.UserIDFromContext(ctx)
	id := identity{User: user}
	if inv, ok := invocationctx.Get(ctx); ok {
		id.Conversation = inv.ConversationID
		id.Profile = inv.ProfileSlug
	}
	m.mu.RLock()
	profile, work := m.activeProfileSlug, m.options.WorkDir
	m.mu.RUnlock()
	if id.Profile == "" && profile != nil {
		id.Profile = profile()
	}
	id.Profile = strings.ToLower(strings.TrimSpace(id.Profile))
	// O perfil resolvido também acompanha outros avaliadores da mesma decisão
	// (por exemplo a política shell), não só o store que capturou a identidade.
	inv, _ := invocationctx.Get(ctx)
	inv.ProfileSlug = id.Profile
	ctx = invocationctx.With(ctx, inv)
	if root, captured := invocationctx.Workspace(ctx); captured {
		if root != "" {
			id.Workspace = canonicalPath(filepath.Join(root, ".assistente"))
		}
	} else if inv, ok := invocationctx.Get(ctx); ok && inv.Source != "" {
		// Invocações sem identidade de workspace não herdam a tela ativa.
		id.Workspace = ""
	} else if work != nil {
		id.Workspace = canonicalPath(work())
	}
	if id.Conversation != "" {
		m.mu.Lock()
		key := sessionKey{id.User, id.Conversation}
		id.Generation = m.generation[key]
		m.generation[key] = id.Generation
		m.mu.Unlock()
	}
	return context.WithValue(ctx, identityKey{m}, id)
}
func canonicalPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs
}
func (m *Manager[E, Q]) identity(ctx context.Context) (identity, error) {
	id := m.Capture(ctx).Value(identityKey{m}).(identity)
	if id.User == "" {
		return id, database.ErrUserScopeRequired
	}
	if id.Conversation != "" {
		m.mu.RLock()
		stale := m.generation[sessionKey{id.User, id.Conversation}] != id.Generation
		m.mu.RUnlock()
		if stale {
			return id, errors.New("conversa alterada durante autorização")
		}
	}
	return id, nil
}
func (m *Manager[E, Q]) ClearSession(conversation string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.generation {
		if key.Conversation == conversation {
			delete(m.session, key)
			m.generation[key]++
		}
	}
}
func scopeKey(id identity, scope Scope) (string, error) {
	switch scope {
	case ScopeGlobal:
		return "", nil
	case ScopeWorkspace:
		if id.Workspace != "" {
			return id.Workspace, nil
		}
	case ScopeProfile:
		if id.Profile != "" {
			return id.Profile, nil
		}
	}
	return "", fmt.Errorf("identidade indisponível para escopo %s", scope)
}

// AvailableScopes é calculado sobre a origem capturada, nunca sobre rótulos
// ou escolhas vindos da interface. A superfície ainda pode restringir a lista.
func (m *Manager[E, Q]) AvailableScopes(ctx context.Context) ([]Scope, error) {
	id, err := m.identity(ctx)
	if err != nil {
		return nil, err
	}
	out := []Scope{ScopeOnce}
	if id.Conversation != "" {
		out = append(out, ScopeSession)
	}
	if id.Workspace != "" {
		out = append(out, ScopeWorkspace)
	}
	if id.Profile != "" {
		out = append(out, ScopeProfile)
	}
	return append(out, ScopeGlobal), nil
}
func (m *Manager[E, Q]) Find(ctx context.Context, query Q) Match[E] {
	entries, err := m.List(ctx)
	if err != nil {
		return Match[E]{Err: err}
	}
	for _, entry := range entries {
		if m.config.Adapter.Matches(entry, query) {
			return Match[E]{Found: true, Scope: m.config.Adapter.Scope(entry), Entry: &entry}
		}
	}
	return Match[E]{}
}
func (m *Manager[E, Q]) List(ctx context.Context) ([]E, error) {
	ctx = m.Capture(ctx)
	id, err := m.identity(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	out := append([]E(nil), m.session[sessionKey{id.User, id.Conversation}]...)
	m.mu.RUnlock()
	keys := make(map[string]string)
	for _, scope := range []Scope{ScopeProfile, ScopeWorkspace, ScopeGlobal} {
		key, err := scopeKey(id, scope)
		if err != nil {
			continue
		}
		if err := m.importLegacy(ctx, scope, key); err != nil {
			return nil, err
		}
		keys[string(scope)] = key
	}
	rows, err := m.options.Repository.ListScopes(ctx, m.config.Domain, keys)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		var entry E
		if row.SelectorVersion != 1 {
			return nil, errors.New("versão de autorização desconhecida")
		}
		if err := json.Unmarshal([]byte(row.Selector), &entry); err != nil {
			return nil, err
		}
		entry, err = m.config.Adapter.Normalize(entry)
		if err != nil {
			return nil, err
		}
		if string(m.config.Adapter.Scope(entry)) != row.Scope {
			return nil, errors.New("escopo divergente na autorização")
		}
		if m.config.Adapter.Effect(entry) != row.Effect || m.config.Adapter.Operation(entry) != row.Operation || m.config.Adapter.Identity(entry) != row.SelectorKey {
			return nil, errors.New("seletor divergente na autorização")
		}
		out = append(out, entry)
	}
	// Uma limpeza pode ocorrer enquanto o banco é consultado. Não publique
	// concessões copiadas de uma geração encerrada durante esse I/O.
	if _, err := m.identity(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
func (m *Manager[E, Q]) rule(entry E, key string) (database.AuthorizationRule, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return database.AuthorizationRule{}, err
	}
	return database.AuthorizationRule{Domain: m.config.Domain, Scope: string(m.config.Adapter.Scope(entry)), ScopeKey: key, SelectorKey: m.config.Adapter.Identity(entry), Selector: string(data), SelectorVersion: 1, Effect: m.config.Adapter.Effect(entry), Operation: m.config.Adapter.Operation(entry), Origin: "consent"}, nil
}
func (m *Manager[E, Q]) Add(ctx context.Context, entry E) error {
	ctx = m.Capture(ctx)
	if _, err := m.List(ctx); err != nil {
		return err
	}
	var err error
	entry, err = m.config.Adapter.Normalize(entry)
	if err != nil {
		return err
	}
	scope := m.config.Adapter.Scope(entry)
	if !ValidScope(scope) {
		return errors.New("escopo inválido")
	}
	id, err := m.identity(ctx)
	if err != nil {
		return err
	}
	if scope == ScopeOnce {
		return nil
	}
	if scope == ScopeSession {
		if id.Conversation == "" {
			return errors.New("escopo sessão requer conversa")
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		key := sessionKey{id.User, id.Conversation}
		if m.generation[key] != id.Generation {
			return errors.New("conversa alterada durante autorização")
		}
		entries := m.session[key]
		for i, old := range entries {
			if m.config.Adapter.SameKey(old, m.config.Adapter.Key(entry)) {
				entries[i] = entry
				m.session[key] = entries
				return nil
			}
		}
		m.session[key] = append(entries, entry)
		return nil
	}
	key, err := scopeKey(id, scope)
	if err != nil {
		return err
	}
	row, err := m.rule(entry, key)
	if err != nil {
		return err
	}
	// Serializa só a gravação curta com a invalidação de sessão. Nunca segura
	// este lock durante leitura de arquivo ou espera pela resposta humana.
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id.Conversation != "" && m.generation[sessionKey{id.User, id.Conversation}] != id.Generation {
		return errors.New("conversa alterada durante autorização")
	}
	return m.options.Repository.Upsert(ctx, row)
}
func (m *Manager[E, Q]) Remove(ctx context.Context, scope Scope, query Q) error {
	ctx = m.Capture(ctx)
	id, err := m.identity(ctx)
	if err != nil {
		return err
	}
	if scope == ScopeSession {
		m.mu.Lock()
		defer m.mu.Unlock()
		key := sessionKey{id.User, id.Conversation}
		if m.generation[key] != id.Generation {
			return errors.New("conversa alterada durante autorização")
		}
		entries := m.session[key]
		for i, entry := range entries {
			if m.config.Adapter.SameKey(entry, query) {
				m.session[key] = append(entries[:i:i], entries[i+1:]...)
				m.generation[key]++
				return nil
			}
		}
		return m.config.NotFoundError
	}
	key, err := scopeKey(id, scope)
	if err != nil {
		return err
	}
	entries, err := m.List(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if m.config.Adapter.Scope(entry) == scope && m.config.Adapter.SameKey(entry, query) {
			err := m.options.Repository.Revoke(ctx, m.config.Domain, string(scope), key, m.config.Adapter.Identity(entry))
			if errors.Is(err, authorization.ErrNotFound) {
				return m.config.NotFoundError
			}
			return err
		}
	}
	return m.config.NotFoundError
}

// importLegacy é a única leitura de arquivo: o marcador transacional impede
// fallback/reimportação depois que o banco passa a ser a fonte canônica.
func (m *Manager[E, Q]) importLegacy(ctx context.Context, scope Scope, key string) error {
	if m.config.Subdir == "" {
		return nil
	}
	m.importMu.Lock()
	defer m.importMu.Unlock()
	m.mu.RLock()
	homeFn := m.options.HomeDir
	permit := m.options.MayImport
	m.mu.RUnlock()
	home := ""
	if homeFn != nil {
		home = homeFn()
	}
	source := ""
	switch scope {
	case ScopeWorkspace:
		source = filepath.Join(key, m.config.Subdir, "workspace.json")
	case ScopeGlobal:
		if home != "" {
			source = filepath.Join(home, m.config.Subdir, "global.json")
		}
	case ScopeProfile:
		if home != "" {
			if err := configdir.ValidateFilename(key + ".json"); err != nil {
				return err
			}
			// Os escritores anteriores removiam pontuação do nome do arquivo.
			// Essa compatibilidade fica só na importação; a identidade no banco
			// continua sendo a do perfil efetivo, sem criar aliases novos.
			legacyKey := strings.Map(func(r rune) rune {
				if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
					return r
				}
				return -1
			}, key)
			if legacyKey == "" {
				return errors.New("identidade de perfil inválida")
			}
			source = filepath.Join(home, m.config.Subdir, "profile-"+legacyKey+".json")
		}
	}
	if source == "" {
		return nil
	}
	source = canonicalPath(source)
	imported, err := m.options.Repository.Imported(ctx, m.config.Domain, source)
	if err != nil || imported {
		return err
	}
	data, err := os.ReadFile(source)
	missing := errors.Is(err, os.ErrNotExist)
	if missing {
		data = nil
	} else if err != nil {
		return fmt.Errorf("ler autorizações legadas: %w", err)
	}
	var legacy struct {
		Version int `json:"version"`
		Entries []E `json:"entries"`
	}
	if !missing {
		allowed, err := m.options.Repository.AuthorizeLegacyImport(ctx, m.config.Domain, source, permit != nil && permit(ctx))
		if err != nil {
			return err
		}
		if !allowed {
			return nil
		}
		if err := json.Unmarshal(data, &legacy); err != nil {
			return fmt.Errorf("autorizações legadas inválidas: %w", err)
		}
		if legacy.Version != 1 {
			return errors.New("versão legada de autorização não suportada")
		}
	}
	rules := make([]database.AuthorizationRule, 0, len(legacy.Entries))
	for _, entry := range legacy.Entries {
		entry, err = m.config.Adapter.Normalize(entry)
		if err != nil {
			return err
		}
		if m.config.Adapter.Scope(entry) != scope {
			return errors.New("escopo legado diverge da origem")
		}
		row, err := m.rule(entry, key)
		if err != nil {
			return err
		}
		rules = append(rules, row)
	}
	digest := sha256.Sum256(data)
	return m.options.Repository.Import(ctx, m.config.Domain, source, fmt.Sprintf("%x", digest), rules, func() error {
		current, err := os.ReadFile(source)
		if errors.Is(err, os.ErrNotExist) != missing {
			return errors.New("origem alterada durante importação de autorizações")
		}
		if errors.Is(err, os.ErrNotExist) {
			current = nil
		} else if err != nil {
			return err
		}
		if sha256.Sum256(current) != digest {
			return errors.New("origem alterada durante importação de autorizações")
		}
		return nil
	})
}
