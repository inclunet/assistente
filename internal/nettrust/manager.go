package nettrust

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/trustscope"
)

// ErrEntryNotFound é devolvido quando nenhuma entrada casa com host e porta.
var ErrEntryNotFound = errors.New("entrada de allowlist de rede não encontrada")

const subdir = "network-allowlist"

type entryQuery struct {
	host string
	port string
}

// Manager mantém a API de nettrust e delega o contrato escopado a trustscope.
type Manager struct {
	core *trustscope.Manager[AllowlistEntry, entryQuery]
}

func managerConfig() trustscope.Config[AllowlistEntry, entryQuery] {
	return trustscope.Config[AllowlistEntry, entryQuery]{
		Subdir:        subdir,
		LogComponent:  "nettrust.manager",
		LogPrefix:     "[NetTrust]",
		DomainLabel:   "rede",
		NotFoundError: ErrEntryNotFound,
		Adapter: trustscope.Adapter[AllowlistEntry, entryQuery]{
			Scope: func(entry AllowlistEntry) trustscope.Scope { return entry.Scope },
			Normalize: func(entry AllowlistEntry) (AllowlistEntry, error) {
				entry.Host = normalizeHost(entry.Host)
				if entry.Host == "" {
					return entry, fmt.Errorf("host vazio não pode ser autorizado")
				}
				if entry.CreatedAt.IsZero() {
					entry.CreatedAt = time.Now().UTC()
				}
				return entry, nil
			},
			Key: func(entry AllowlistEntry) entryQuery {
				return queryFor(entry.Host, entry.Port)
			},
			Matches: func(entry AllowlistEntry, query entryQuery) bool {
				return entry.Matches(query.host, query.port)
			},
			SameKey: func(entry AllowlistEntry, query entryQuery) bool {
				return normalizeHost(entry.Host) == query.host &&
					strings.EqualFold(entry.Port, query.port)
			},
		},
	}
}

func queryFor(host, port string) entryQuery {
	return entryQuery{host: normalizeHost(host), port: port}
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

func (m *Manager) Match(ctx context.Context, host, port string) NetworkTrustDecision {
	match := m.core.Find(ctx, queryFor(host, port))
	return NetworkTrustDecision{Allowed: match.Found, Scope: match.Scope, Entry: match.Entry}
}

func (m *Manager) Add(ctx context.Context, entry AllowlistEntry) error {
	return m.core.Add(ctx, entry)
}

func (m *Manager) List(ctx context.Context) []AllowlistEntry {
	return m.core.List(ctx)
}

func (m *Manager) Remove(ctx context.Context, scope Scope, host, port string) error {
	return m.core.Remove(ctx, scope, queryFor(host, port))
}

// workspacePath é mantido para os testes de compatibilidade do consumidor.
func (m *Manager) workspacePath() string {
	return m.core.WorkspacePath()
}
