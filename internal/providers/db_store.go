package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/logging"
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
		p, err := fromDBModel(dbP)
		if err != nil {
			// Uma linha ilegível não pode derrubar a lista inteira, e também
			// não pode virar um provedor pela metade: subir um agente de
			// código sem os argumentos que definem o modo dele é pior do que
			// ele não aparecer.
			logging.Errorf(ctx, "providers.store", "[providers] provedor %q ignorado: %v", dbP.ID, err)
			continue
		}
		result = append(result, p)
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
	return fromDBModel(dbP)
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
	return fromDBModel(dbP)
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
		AuthMode:          string(p.AuthMode),
		ACPCommand:        p.ACPCommand,
		ACPArgs:           encodeACPList(p.ACPArgs),
		ACPEnv:            encodeACPMap(p.ACPEnv),
		ACPCredentialEnv:  encodeACPMap(p.ACPCredentialEnv),
		ACPAgentID:        p.ACPAgentID,
	}
}

func fromDBModel(dbP *database.LLMProvider) (*llm.ProviderConfig, error) {
	args, err := decodeACPList(dbP.ACPArgs)
	if err != nil {
		return nil, fmt.Errorf("argumentos do agente ilegíveis: %w", err)
	}
	env, err := decodeACPMap(dbP.ACPEnv)
	if err != nil {
		return nil, fmt.Errorf("variáveis de ambiente do agente ilegíveis: %w", err)
	}
	credentialEnv, err := decodeACPMap(dbP.ACPCredentialEnv)
	if err != nil {
		return nil, fmt.Errorf("credenciais do cofre do agente ilegíveis: %w", err)
	}
	p := &llm.ProviderConfig{
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
		AuthMode:          llm.AuthMode(dbP.AuthMode),
		ACPCommand:        dbP.ACPCommand,
		ACPArgs:           args,
		ACPEnv:            env,
		ACPCredentialEnv:  credentialEnv,
		ACPAgentID:        dbP.ACPAgentID,
	}
	normalizeProviderRuntimeDefaults(p)
	return p, nil
}

// A lista de argumentos e o mapa de ambiente viajam como JSON em coluna de
// texto — mesmo arranjo do servidor MCP stdio, porque o SQLite não guarda
// lista nem mapa. Vazio vira coluna vazia em vez de "null" ou "[]": é o que
// mantém legível a linha de um provedor HTTP, que nunca terá isso.

func encodeACPList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(raw)
}

func encodeACPMap(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(raw)
}

func decodeACPList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func decodeACPMap(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}
