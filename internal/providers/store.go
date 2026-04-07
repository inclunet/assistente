package providers

import "assistente/internal/llm"

// ProviderStore abstrai operações de persistência de provedores LLM.
// Implementado por database.LLMProviderStore; pode ser mockado em testes.
type ProviderStore interface {
	// Save persiste um ou mais provedores (upsert por ID).
	Save(providers []*llm.ProviderConfig) error

	// Load carrega todos os provedores persistidos.
	Load() ([]*llm.ProviderConfig, error)

	// SetDefault marca o provedor com o ID dado como padrão do sistema.
	SetDefault(id string) error

	// GetDefault retorna o provedor marcado como padrão, ou nil se nenhum.
	GetDefault() (*llm.ProviderConfig, error)

	// Get retorna um provedor pelo ID, ou nil se não encontrado.
	Get(id string) (*llm.ProviderConfig, error)

	// Count retorna o total de provedores persistidos.
	Count() (int, error)
}
