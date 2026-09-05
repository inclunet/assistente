package fstrust

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/trustscope"
)

// ErrEntryNotFound é devolvido quando nenhuma entrada casa com a chave.
var ErrEntryNotFound = errors.New("entrada de allowlist de path não encontrada")

const subdir = "path-allowlist"

type entryQuery struct {
	path      string
	kind      Kind
	operation string
	effect    Effect
}

// Manager mantém a API de fstrust e delega o contrato escopado a trustscope.
type Manager struct {
	core *trustscope.Manager[AllowlistEntry, entryQuery]
}

func managerConfig() trustscope.Config[AllowlistEntry, entryQuery] {
	return trustscope.Config[AllowlistEntry, entryQuery]{
		Subdir:        subdir,
		LogComponent:  "fstrust.manager",
		LogPrefix:     "[FsTrust]",
		DomainLabel:   "path",
		NotFoundError: ErrEntryNotFound,
		Adapter: trustscope.Adapter[AllowlistEntry, entryQuery]{
			Scope: func(entry AllowlistEntry) trustscope.Scope { return entry.Scope },
			Normalize: func(entry AllowlistEntry) (AllowlistEntry, error) {
				entry.Path = NormalizePath(entry.Path)
				if entry.Path == "" {
					return entry, fmt.Errorf("path vazio não pode ser autorizado")
				}
				if !ValidKind(entry.Kind) {
					return entry, fmt.Errorf("kind inválido: %q", entry.Kind)
				}
				if strings.TrimSpace(entry.Operation) == "" {
					return entry, fmt.Errorf("operation vazia não pode ser autorizada")
				}
				if !ValidEffect(entry.Effect) {
					return entry, fmt.Errorf("effect inválido: %q", entry.Effect)
				}
				entry.Effect = NormalizedEffect(entry.Effect)
				if entry.CreatedAt.IsZero() {
					entry.CreatedAt = time.Now().UTC()
				}
				return entry, nil
			},
			Key: func(entry AllowlistEntry) entryQuery {
				return queryFor(entry.Path, entry.Kind, entry.Operation, entry.Effect)
			},
			Matches: func(entry AllowlistEntry, query entryQuery) bool {
				return NormalizedEffect(entry.Effect) == query.effect &&
					entry.Matches(query.path, query.operation)
			},
			SameKey: func(entry AllowlistEntry, query entryQuery) bool {
				return NormalizePath(entry.Path) == query.path &&
					entry.Kind == query.kind &&
					entry.Operation == query.operation &&
					NormalizedEffect(entry.Effect) == query.effect
			},
		},
	}
}

func queryFor(path string, kind Kind, operation string, effect Effect) entryQuery {
	return entryQuery{
		path:      NormalizePath(path),
		kind:      kind,
		operation: operation,
		effect:    NormalizedEffect(effect),
	}
}

// NewManager cria um Manager usando os diretórios canônicos.
func NewManager() *Manager {
	return &Manager{core: trustscope.NewManager(managerConfig())}
}

// NewManagerWithDirs cria um Manager com diretórios fixos para testes.
func NewManagerWithDirs(homeDir, workDir string) *Manager {
	return &Manager{core: trustscope.NewManagerWithDirs(managerConfig(), homeDir, workDir)}
}

func (m *Manager) SetActiveProfileSlugFunc(f func() string) {
	m.core.SetActiveProfileSlugFunc(f)
}

func (m *Manager) SetWorkspaceDirFunc(f func() string) {
	m.core.SetWorkspaceDirFunc(f)
}

func (m *Manager) ClearSession(conversationID string) {
	m.core.ClearSession(conversationID)
}

// Match procura uma autorização na ordem canônica dos escopos.
func (m *Manager) Match(ctx context.Context, absPath, operation string) Decision {
	match := m.core.Find(ctx, queryFor(absPath, "", operation, EffectAllow))
	return Decision{Allowed: match.Found, Scope: match.Scope, Entry: match.Entry}
}

// MatchDeny procura uma proibição com precedência sobre qualquer allow.
func (m *Manager) MatchDeny(ctx context.Context, absPath, operation string) DenyMatch {
	match := m.core.Find(ctx, queryFor(absPath, "", operation, EffectDeny))
	return DenyMatch{Matched: match.Found, Scope: match.Scope, Entry: match.Entry}
}

func (m *Manager) Add(ctx context.Context, entry AllowlistEntry) error {
	return m.core.Add(ctx, entry)
}

func (m *Manager) List(ctx context.Context) []AllowlistEntry {
	return m.core.List(ctx)
}

func (m *Manager) Remove(ctx context.Context, scope Scope, path string, kind Kind, operation string, effect Effect) error {
	if !ValidEffect(effect) {
		return fmt.Errorf("effect inválido: %q", effect)
	}
	return m.core.Remove(ctx, scope, queryFor(path, kind, operation, effect))
}
