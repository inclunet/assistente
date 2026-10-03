package providers

import (
	"context"

	"assistente/internal/llm"
)

// ProviderStore abstrai operações de persistência de provedores LLM.
// Implementado por database.LLMProviderStore; pode ser mockado em testes.
type ProviderStore interface {
	// Delete removes a generic provider, refusing OAuth consumers.
	Delete(ctx context.Context, id string) error

	// Save persiste um ou mais provedores (upsert por ID).
	Save(ctx context.Context, providers []*llm.ProviderConfig) error

	// Load carrega todos os provedores persistidos.
	Load(ctx context.Context) ([]*llm.ProviderConfig, error)

	// SetDefault marca o provedor com o ID dado como padrão do sistema.
	SetDefault(ctx context.Context, id string) error

	// GetDefault retorna o provedor marcado como padrão, ou nil se nenhum.
	GetDefault(ctx context.Context) (*llm.ProviderConfig, error)

	// Get retorna um provedor pelo ID, ou nil se não encontrado.
	Get(ctx context.Context, id string) (*llm.ProviderConfig, error)

	// Count retorna o total de provedores persistidos.
	Count(ctx context.Context) (int, error)
}

// CredentialPatternRevisionStore invalida todos os provedores que compartilham
// um pattern antes de a credencial armazenada sob ele ser alterada/removida.
type CredentialPatternRevisionStore interface {
	BumpCompatibilityRevisionsForCredentialPattern(ctx context.Context, pattern string) (map[string]int, error)
}

// CredentialPatternRevisionReader lê a revisão efetiva depois que o store do
// cofre confirmou a mutação transacional.
type CredentialPatternRevisionReader interface {
	GetCompatibilityRevisionsForCredentialPattern(ctx context.Context, pattern string) (map[string]int, error)
}

// AtomicCredentialPatternRevisionStore indica que mutações persistidas do
// cofre avançam revisões na mesma transação SQLite.
type AtomicCredentialPatternRevisionStore interface {
	CredentialMutationsBumpCompatibilityRevisionsAtomically() bool
}
