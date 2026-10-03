package providers

import (
	"context"

	"assistente/internal/llm"
)

// ProviderStore abstrai operações de persistência de provedores LLM.
// Implementado por database.LLMProviderStore; pode ser mockado em testes.
type ProviderStore interface {
	// Create insere um provedor e recusa qualquer ID já persistido.
	Create(ctx context.Context, provider *llm.ProviderConfig) error

	// RollbackCreate remove uma reserva não publicada, se a configuração não mudou.
	RollbackCreate(ctx context.Context, provider *llm.ProviderConfig) error

	// Exists consulta a identidade persistida antes dos efeitos da criação.
	Exists(ctx context.Context, id string) (bool, error)

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
