package providers

import (
	"context"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// DBStore implementa ProviderStore usando o banco de dados SQLite via GORM.
type DBStore struct{}

// NewDBStore cria um DBStore pronto para uso.
func NewDBStore() *DBStore { return &DBStore{} }

// Save persiste todos os provedores fornecidos no banco.
// Usa GORM Save (upsert por primary key).
//
// Save é fail-closed: exige que o contexto carregue um userID OU esteja
// explicitamente marcado por database.WithBootstrap. Sem nenhum dos dois
// retorna ErrUserScopeRequired. Essa garantia fecha o vetor levantado no
// review do AEP-0052 onde Save sem proteção podia gravar provedores órfãos
// se chamado por engano fora do caminho de bootstrap.
//
// Caminho normal (post-login): o ctx carrega o userID via WithUserID.
// Caminho bootstrap (CLI setup, wizard pré-login): o caller marca o ctx
// com WithBootstrap deliberadamente. Os registros nascem órfãos
// (user_id="") e são adotados pelo primeiro usuário em AdoptLegacyData.
func (s *DBStore) Save(ctx context.Context, providers []*llm.ProviderConfig) error {
	if err := database.RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	for _, p := range providers {
		dbP := toDBModel(p)
		if err := database.SaveLLMProviderWithContext(ctx, dbP); err != nil {
			return err
		}
	}
	return nil
}

// Load retorna todos os provedores do banco convertidos para ProviderConfig.
func (s *DBStore) Load(ctx context.Context) ([]*llm.ProviderConfig, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	dbProviders, err := database.GetLLMProvidersWithContext(ctx)
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
func (s *DBStore) SetDefault(ctx context.Context, id string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.SetDefaultProviderWithContext(ctx, id)
}

// GetDefault retorna o provedor marcado como padrão, ou nil + erro se nenhum.
func (s *DBStore) GetDefault(ctx context.Context) (*llm.ProviderConfig, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	dbP, err := database.GetDefaultProviderWithContext(ctx)
	if err != nil {
		return nil, err
	}
	return fromDBModel(dbP), nil
}

// Get retorna um provedor por ID.
func (s *DBStore) Get(ctx context.Context, id string) (*llm.ProviderConfig, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	dbP, err := database.GetLLMProviderWithContext(ctx, id)
	if err != nil {
		return nil, err
	}
	return fromDBModel(dbP), nil
}

// Count retorna a contagem total de provedores no banco.
func (s *DBStore) Count(ctx context.Context) (int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, err
	}
	n, err := database.CountLLMProvidersWithContext(ctx)
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
