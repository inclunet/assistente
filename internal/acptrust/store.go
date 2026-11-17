// Package acptrust adapta as classes de ação ACP ao repositório comum.
package acptrust

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Kind      string           `json:"kind"`
	GrantedAt time.Time        `json:"granted_at"`
	Scope     trustscope.Scope `json:"scope,omitempty"`
}
type Store struct {
	core *trustscope.Manager[Entry, string]
	repo *authorization.Repository
	home func() string
}

func NewStore(options trustscope.Options) *Store {
	if options.Repository == nil {
		options.Repository = authorization.NewRepository(database.DB)
	}
	if options.HomeDir == nil {
		options.HomeDir = configdir.GetHomeDir
	}
	cfg := trustscope.Config[Entry, string]{Domain: "acp", Subdir: subdir, NotFoundError: ErrEntryNotFound, Adapter: trustscope.Adapter[Entry, string]{
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
		Key:       func(e Entry) string { return e.Kind },
		Matches:   func(e Entry, q string) bool { return e.Kind == normalizeKind(q) },
		SameKey:   func(e Entry, q string) bool { return e.Kind == normalizeKind(q) },
		Identity:  func(e Entry) string { return e.Kind },
		Effect:    func(Entry) string { return "allow" },
		Operation: func(e Entry) string { return e.Kind },
	}}
	return &Store{core: trustscope.NewManager(cfg, options), repo: options.Repository, home: options.HomeDir}
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
func (s *Store) Allows(ctx context.Context, profile, kind string) (bool, error) {
	ctx, err := profileContext(ctx, profile)
	if err != nil {
		return false, err
	}
	match := s.core.Find(ctx, normalizeKind(kind))
	return match.Found, match.Err
}
func (s *Store) Allow(ctx context.Context, profile, kind string) error {
	ctx, err := profileContext(ctx, profile)
	if err != nil {
		return err
	}
	return s.core.Add(ctx, Entry{Kind: kind, Scope: trustscope.ScopeProfile})
}
func (s *Store) List(ctx context.Context, profile string) ([]Entry, error) {
	ctx, err := profileContext(ctx, profile)
	if err != nil {
		return nil, err
	}
	return s.core.List(ctx)
}
func (s *Store) Revoke(ctx context.Context, profile, kind string) error {
	ctx, err := profileContext(ctx, profile)
	if err != nil {
		return err
	}
	return s.core.Remove(ctx, trustscope.ScopeProfile, normalizeKind(kind))
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
			slug := strings.TrimSuffix(strings.TrimPrefix(name, "profile-"), ".json")
			if slug == "" || ProfileKey(slug) != slug {
				return nil, fmt.Errorf("origem legada ACP inválida: %s", name)
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
