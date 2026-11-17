// Package acptrust adapta as classes de ação ACP ao repositório comum.
package acptrust

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"assistente/internal/authorization"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/trustscope"
)

var ErrEntryNotFound = errors.New("autorização permanente não encontrada")

const subdir = "acp-permissions"

type Entry struct {
	AgentID   string           `json:"agent_id,omitempty"`
	Kind      string           `json:"kind"`
	GrantedAt time.Time        `json:"granted_at"`
	Scope     trustscope.Scope `json:"scope,omitempty"`
}
type Store struct {
	core     *trustscope.Manager[Entry, Resource]
	repo     *authorization.Repository
	home     func() string
	profiles func() ([]string, error)
}

func NewStore(options trustscope.Options) *Store {
	if options.Repository == nil {
		options.Repository = authorization.NewRepository(database.DB)
	}
	if options.HomeDir == nil {
		options.HomeDir = configdir.GetHomeDir
	}
	store := &Store{repo: options.Repository, home: options.HomeDir, profiles: options.LegacyProfiles}
	cfg := trustscope.Config[Entry, Resource]{Domain: "acp", Subdir: subdir, NotFoundError: ErrEntryNotFound,
		LegacyScopes: []trustscope.Scope{trustscope.ScopeProfile},
		ValidateLegacyProfile: func(key, legacyKey string) error {
			resolved, err := store.legacyProfile(legacyKey)
			if err != nil {
				return err
			}
			if resolved != key {
				return errors.New("origem legada ACP pertence a outro perfil")
			}
			return nil
		}, Adapter: trustscope.Adapter[Entry, Resource]{
			Scope: func(e Entry) trustscope.Scope { return e.Scope },
			Normalize: func(e Entry) (Entry, error) {
				e.Kind = normalizeKind(e.Kind)
				if e.Kind == "" {
					return e, errors.New("classe de ação vazia")
				}
				if e.Scope == "" {
					e.Scope = trustscope.ScopeProfile
				}
				if e.GrantedAt.IsZero() {
					e.GrantedAt = time.Now().UTC()
				}
				return e, nil
			},
			Key: func(e Entry) Resource { return Resource{Kind: e.Kind, AgentID: e.AgentID} },
			Matches: func(e Entry, q Resource) bool {
				return e.Kind == normalizeKind(q.Kind) && (e.AgentID == "" || e.AgentID == q.AgentID)
			},
			SameKey: func(e Entry, q Resource) bool { return e.Kind == normalizeKind(q.Kind) && e.AgentID == q.AgentID },
			Identity: func(e Entry) string {
				if e.AgentID == "" {
					return e.Kind
				}
				return e.AgentID + "\x00" + e.Kind
			},
			Effect:    func(Entry) string { return "allow" },
			Operation: func(e Entry) string { return e.Kind },
		}}
	store.core = trustscope.NewManager(cfg, options)
	return store
}

// A sanitização antiga não é invertível: somente o catálogo real pode
// identificar um arquivo, e colisões permanecem bloqueadas.
func (s *Store) legacyProfile(legacyKey string) (string, error) {
	if s.profiles == nil {
		return "", errors.New("catálogo de perfis indisponível para migração ACP")
	}
	keys, err := s.profiles()
	if err != nil {
		return "", err
	}
	resolved := ""
	for _, key := range keys {
		key = ProfileKey(key)
		encoded := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
				return r
			}
			return -1
		}, key)
		if encoded != legacyKey {
			continue
		}
		if resolved != "" && resolved != key {
			return "", errors.New("origem legada ACP com identidade de perfil ambígua")
		}
		resolved = key
	}
	if resolved == "" {
		return "", errors.New("origem legada ACP sem perfil identificável")
	}
	return resolved, nil
}
func profileContext(ctx context.Context, profile string) (context.Context, error) {
	profile = ProfileKey(profile)
	if profile == "" {
		return nil, errors.New("perfil inválido para autorização do agente")
	}
	inv, _ := invocationctx.Get(ctx)
	inv.ProfileSlug = profile
	return invocationctx.With(ctx, inv), nil
}
func (s *Store) List(ctx context.Context, profile string) ([]Entry, error) {
	ctx, err := profileContext(ctx, profile)
	if err != nil {
		return nil, err
	}
	return s.core.List(ctx)
}
func (s *Store) Profiles(ctx context.Context) ([]string, error) {
	keys, err := s.repo.ScopeKeys(ctx, "acp", "profile")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key] = true
	}
	if home := s.home(); home != "" {
		entries, err := os.ReadDir(filepath.Join(home, subdir))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasPrefix(name, "profile-") || !strings.HasSuffix(name, ".json") {
				continue
			}
			// Uma cópia já consumida não depende mais do catálogo atual.
			source, err := filepath.Abs(filepath.Join(home, subdir, name))
			if err != nil {
				return nil, err
			}
			if resolved, err := filepath.EvalSymlinks(source); err == nil {
				source = resolved
			}
			if runtime.GOOS == "windows" {
				source = strings.ToLower(source)
			}
			imported, err := s.repo.Imported(ctx, "acp", source)
			if err != nil {
				return nil, err
			}
			if imported {
				continue
			}
			legacyKey := strings.TrimSuffix(strings.TrimPrefix(name, "profile-"), ".json")
			if legacyKey == "" || ProfileKey(legacyKey) != legacyKey {
				return nil, fmt.Errorf("origem legada ACP inválida: %s", name)
			}
			slug, err := s.legacyProfile(legacyKey)
			if err != nil {
				return nil, err
			}
			// O core importa uma vez; depois só consulta o banco, mesmo que o
			// arquivo continue no diretório como recuperação.
			if _, err := s.List(ctx, slug); err != nil {
				return nil, err
			}
			seen[slug] = true
		}
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out, nil
}
func normalizeKind(kind string) string { return strings.ToLower(strings.TrimSpace(kind)) }
func ProfileKey(slug string) string {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" || configdir.ValidateFilename(slug+".json") != nil {
		return ""
	}
	return slug
}

// Resource vem da identidade do provider e da classe do protocolo, nunca do
// título livre enviado pelo agente. AgentID vazio só existe na migração legada.
type Resource struct {
	Kind    string
	AgentID string
}

func (s *Store) Capture(ctx context.Context) context.Context { return s.core.Capture(ctx) }
func (s *Store) AvailableScopes(ctx context.Context) ([]authorization.Scope, error) {
	return s.core.AvailableScopes(ctx)
}
func (s *Store) Match(ctx context.Context, resource Resource) (authorization.Decision, error) {
	match := s.core.Find(ctx, resource)
	if match.Err != nil {
		return authorization.Decision{}, match.Err
	}
	if match.Found {
		return authorization.Decision{Effect: "allow", Scope: match.Scope}, nil
	}
	return authorization.Decision{Effect: "ask"}, nil
}
func (s *Store) Grant(ctx context.Context, entry Entry) error {
	if strings.TrimSpace(entry.AgentID) == "" {
		return errors.New("identidade do agente ausente")
	}
	return s.core.Add(ctx, entry)
}
func (s *Store) ClearSession(conversationID string) { s.core.ClearSession(conversationID) }
