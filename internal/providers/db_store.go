package providers

import (
	"assistente/internal/database"
	"assistente/internal/llm"
)

// DBStore implementa ProviderStore usando o banco de dados SQLite via GORM.
type DBStore struct{}

// NewDBStore cria um DBStore pronto para uso.
func NewDBStore() *DBStore { return &DBStore{} }

// Save persiste todos os provedores fornecidos no banco.
// Usa GORM Save (upsert por primary key).
func (s *DBStore) Save(providers []*llm.ProviderConfig) error {
	for _, p := range providers {
		dbP := toDBModel(p)
		if err := database.SaveLLMProvider(dbP); err != nil {
			return err
		}
	}
	return nil
}

// Load retorna todos os provedores do banco convertidos para ProviderConfig.
func (s *DBStore) Load() ([]*llm.ProviderConfig, error) {
	dbProviders, err := database.GetLLMProviders()
	if err != nil {
		return nil, err
	}
	result := make([]*llm.ProviderConfig, 0, len(dbProviders))
	for _, dbP := range dbProviders {
		result = append(result, fromDBModel(dbP))
	}
	return result, nil
}

// SetDefault marca o provedor com o ID fornecido como padrão no banco.
func (s *DBStore) SetDefault(id string) error {
	return database.SetDefaultProvider(id)
}

// GetDefault retorna o provedor marcado como padrão, ou nil + erro se nenhum.
func (s *DBStore) GetDefault() (*llm.ProviderConfig, error) {
	dbP, err := database.GetDefaultProvider()
	if err != nil {
		return nil, err
	}
	return fromDBModel(dbP), nil
}

// Get retorna um provedor por ID.
func (s *DBStore) Get(id string) (*llm.ProviderConfig, error) {
	dbP, err := database.GetLLMProvider(id)
	if err != nil {
		return nil, err
	}
	return fromDBModel(dbP), nil
}

// Count retorna a contagem total de provedores no banco.
func (s *DBStore) Count() (int, error) {
	n, err := database.CountLLMProviders()
	return int(n), err
}

// ============================================================================
// Conversões entre llm.ProviderConfig e database.LLMProvider
// ============================================================================

func toDBModel(p *llm.ProviderConfig) *database.LLMProvider {
	return &database.LLMProvider{
		ID:                p.ID,
		Name:              p.Name,
		Type:              string(p.Type),
		APIFormat:         string(p.APIFormat),
		BaseURL:           p.BaseURL,
		Model:             p.Model,
		DefaultModel:      p.DefaultModel,
		IsDefault:         p.IsDefault,
		Timeout:           p.Timeout,
		CredentialPattern: p.CredentialPattern,
	}
}

func fromDBModel(dbP *database.LLMProvider) *llm.ProviderConfig {
	return &llm.ProviderConfig{
		ID:                dbP.ID,
		Name:              dbP.Name,
		Type:              llm.ProviderType(dbP.Type),
		APIFormat:         llm.APIFormat(dbP.APIFormat),
		BaseURL:           dbP.BaseURL,
		Model:             dbP.Model,
		DefaultModel:      dbP.DefaultModel,
		IsDefault:         dbP.IsDefault,
		Timeout:           dbP.Timeout,
		CredentialPattern: dbP.CredentialPattern,
	}
}
