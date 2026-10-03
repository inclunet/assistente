package database

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ==================== LLM Providers ====================

// ErrProviderCrossUser indica tentativa de sobrescrever, em contexto
// autenticado, um LLMProvider existente cujo dono é OUTRO usuário. Falha
// fechado (AEP-0052) para impedir hijack cross-user via reuso de ID no Save
// (UPSERT por PK).
var ErrProviderCrossUser = errors.New("llm provider pertence a outro usuário")

// ErrLLMProviderAlreadyExists indica que um provider com o mesmo ID já foi
// criado. CreateLLMProvider nunca faz upsert.
var ErrLLMProviderAlreadyExists = errors.New("llm provider já existe")

// ProviderRepository encapsula a persistência de LLMProvider com um *gorm.DB
// injetado, permitindo reuso em transações e testes sem depender da global db.
type ProviderRepository struct {
	db *gorm.DB
}

// NewProviderRepository cria um ProviderRepository com o *gorm.DB injetado.
func NewProviderRepository(database *gorm.DB) *ProviderRepository {
	return &ProviderRepository{db: database}
}

// SaveLLMProviderWithContext é a fachada de transição sobre a global db.
// Delega para ProviderRepository.SaveLLMProvider.
func SaveLLMProviderWithContext(ctx context.Context, provider *LLMProvider) error {
	return NewProviderRepository(db).SaveLLMProvider(ctx, provider)
}

// SaveLLMProvider salva ou atualiza um provedor associado ao usuário do
// contexto.
//
// SECURITY: fail-closed bootstrap-tolerant (AEP-0052 / B11). Aceita ctx com
// userID OU marcado por WithBootstrap (CLI setup, registro de credenciais
// via env). Sem nenhum dos dois, retorna ErrUserScopeRequired — antes era
// fail-open silencioso (provider.UserID ficava em branco e gravava órfão).
// Defesa em camadas: o caller providers.DBStore.Save também valida.
func (r *ProviderRepository) SaveLLMProvider(ctx context.Context, provider *LLMProvider) error {
	db := r.db
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	if provider == nil {
		return errors.New("llm provider inválido")
	}
	if provider != nil && provider.UserID == "" {
		if userID, ok := UserIDFromContext(ctx); ok {
			provider.UserID = userID
		}
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing LLMProvider
		err := tx.First(&existing, "id = ?", provider.ID).Error
		switch {
		case err == nil:
			// SECURITY (AEP-0052 / fail-closed): impedir que o Save (UPSERT por
			// PK) sobrescreva provider de outro usuário ao reutilizar seu ID.
			if userID, ok := UserIDFromContext(ctx); ok && existing.UserID != "" && existing.UserID != userID {
				return ErrProviderCrossUser
			}
			provider.CompatibilityRevision = existing.CompatibilityRevision
			if provider.CompatibilityRevision < 1 {
				provider.CompatibilityRevision = 1
			}
			if providerCompatibilityIdentityChanged(&existing, provider) {
				provider.CompatibilityRevision++
			}
			provider.ConfigRevision = existing.ConfigRevision
			if provider.ConfigRevision < 1 {
				provider.ConfigRevision = 1
			}
			if providerConfigurationChanged(&existing, provider) {
				provider.ConfigRevision++
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if provider.CompatibilityRevision < 1 {
				provider.CompatibilityRevision = 1
			}
			if provider.ConfigRevision < 1 {
				provider.ConfigRevision = 1
			}
		default:
			return err
		}
		return tx.Save(provider).Error
	})
}

// CreateLLMProvider insere um provider sem sobrescrever um ID existente.
// A consulta após conflito evita expor qualquer dado do registro que colidiu.
func (r *ProviderRepository) CreateLLMProvider(ctx context.Context, provider *LLMProvider) error {
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	if provider == nil {
		return errors.New("llm provider inválido")
	}
	if provider.UserID == "" {
		if userID, ok := UserIDFromContext(ctx); ok {
			provider.UserID = userID
		}
	}
	if provider.CompatibilityRevision < 1 {
		provider.CompatibilityRevision = 1
	}
	if provider.ConfigRevision < 1 {
		provider.ConfigRevision = 1
	}
	if err := r.db.WithContext(ctx).Create(provider).Error; err != nil {
		var count int64
		if lookupErr := r.db.WithContext(ctx).Model(&LLMProvider{}).Where("id = ?", provider.ID).Count(&count).Error; lookupErr == nil && count > 0 {
			return ErrLLMProviderAlreadyExists
		}
		return err
	}
	return nil
}

func providerConfigurationChanged(current, next *LLMProvider) bool {
	if current == nil || next == nil {
		return true
	}
	return current.UserID != next.UserID || current.Name != next.Name || current.Type != next.Type || current.IsDefault != next.IsDefault ||
		current.APIFormat != next.APIFormat || current.BaseURL != next.BaseURL || current.Model != next.Model ||
		current.DefaultModel != next.DefaultModel || current.Timeout != next.Timeout ||
		current.StreamIdleTimeoutSeconds != next.StreamIdleTimeoutSeconds ||
		current.CredentialPattern != next.CredentialPattern || current.AuthMode != next.AuthMode ||
		current.ReasoningContentMode != next.ReasoningContentMode || current.ACPCommand != next.ACPCommand ||
		current.ACPArgs != next.ACPArgs || current.ACPEnv != next.ACPEnv ||
		current.ACPCredentialEnv != next.ACPCredentialEnv || current.ACPAgentID != next.ACPAgentID
}

func providerCompatibilityIdentityChanged(current, next *LLMProvider) bool {
	if current == nil || next == nil {
		return true
	}
	return current.UserID != next.UserID || current.Type != next.Type || current.APIFormat != next.APIFormat || current.BaseURL != next.BaseURL ||
		current.CredentialPattern != next.CredentialPattern || current.AuthMode != next.AuthMode ||
		current.ReasoningContentMode != next.ReasoningContentMode ||
		current.ACPCommand != next.ACPCommand || current.ACPArgs != next.ACPArgs || current.ACPEnv != next.ACPEnv ||
		current.ACPCredentialEnv != next.ACPCredentialEnv || current.ACPAgentID != next.ACPAgentID
}

// BumpCompatibilityRevision registra a troca da credencial efetiva sem
// persistir ou derivar qualquer dado do segredo. É usado antes de substituir
// uma chave no cofre quando a referência da credencial continua igual.
func (r *ProviderRepository) BumpCompatibilityRevision(ctx context.Context, id string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	result := ScopeByUser(ctx, r.db.WithContext(ctx).Model(&LLMProvider{}), "user_id").
		Where("id = ?", id).
		UpdateColumn("compatibility_revision", gorm.Expr("compatibility_revision + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BumpCompatibilityRevisionsForCredentialPattern invalida todos os provedores
// do usuário que consomem a credencial compartilhada. A atualização e a
// leitura das novas revisões pertencem ao mesmo snapshot transacional.
func (r *ProviderRepository) BumpCompatibilityRevisionsForCredentialPattern(ctx context.Context, pattern string) (map[string]int, error) {
	if pattern == "" {
		return nil, errors.New("credential pattern vazio")
	}
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return nil, err
	}
	userID, hasUser := UserIDFromContext(ctx)
	scope := func(query *gorm.DB) *gorm.DB {
		if hasUser {
			return query.Where("user_id = ?", userID)
		}
		// O único escopo sem usuário aceito é o bootstrap explícito: providers
		// órfãos ainda não adotados pelo primeiro usuário.
		return query.Where("user_id = ?", "")
	}
	type providerRevision struct {
		ID                    string
		CompatibilityRevision int
	}
	var rows []providerRevision
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := scope(tx.Model(&LLMProvider{})).
			Where("credential_pattern = ?", pattern).
			UpdateColumn("compatibility_revision", gorm.Expr("compatibility_revision + 1")).Error; err != nil {
			return err
		}
		return scope(tx.Model(&LLMProvider{})).
			Select("id", "compatibility_revision").
			Where("credential_pattern = ?", pattern).
			Order("id COLLATE BINARY").
			Find(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	revisions := make(map[string]int, len(rows))
	for _, row := range rows {
		revisions[row.ID] = row.CompatibilityRevision
	}
	return revisions, nil
}

// GetCompatibilityRevisionsForCredentialPattern lê as revisões atuais dos
// provedores que compartilham o pattern, sem alterá-las. Usado após o commit
// do cofre para publicar snapshots no registry.
func (r *ProviderRepository) GetCompatibilityRevisionsForCredentialPattern(ctx context.Context, pattern string) (map[string]int, error) {
	if pattern == "" {
		return nil, errors.New("credential pattern vazio")
	}
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return nil, err
	}
	userID, hasUser := UserIDFromContext(ctx)
	query := r.db.WithContext(ctx).Model(&LLMProvider{})
	if hasUser {
		query = query.Where("user_id = ?", userID)
	} else {
		query = query.Where("user_id = ?", "")
	}
	type providerRevision struct {
		ID                    string
		CompatibilityRevision int
	}
	var rows []providerRevision
	if err := query.Select("id", "compatibility_revision").
		Where("credential_pattern = ?", pattern).
		Order("id COLLATE BINARY").Find(&rows).Error; err != nil {
		return nil, err
	}
	revisions := make(map[string]int, len(rows))
	for _, row := range rows {
		revisions[row.ID] = row.CompatibilityRevision
	}
	return revisions, nil
}

// GetLLMProvidersWithContext é a fachada de transição sobre a global db.
func GetLLMProvidersWithContext(ctx context.Context) ([]*LLMProvider, error) {
	return NewProviderRepository(db).GetLLMProviders(ctx)
}

// GetLLMProviders retorna todos os provedores do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Retornar lista global expõe IDs/credenciais (mesmo cifradas/refs) de
// todos os usuários da instância.
func (r *ProviderRepository) GetLLMProviders(ctx context.Context) ([]*LLMProvider, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var providers []*LLMProvider
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").Order("created_at ASC").Find(&providers).Error
	return providers, err
}

// GetLLMProviderWithContext é a fachada de transição sobre a global db.
func GetLLMProviderWithContext(ctx context.Context, id string) (*LLMProvider, error) {
	return NewProviderRepository(db).GetLLMProvider(ctx, id)
}

// GetLLMProvider busca um provedor por ID no escopo do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Antes, ScopeByUser fail-open + First por ID = leitura cross-user de
// provedor alheio com todos os metadados.
func (r *ProviderRepository) GetLLMProvider(ctx context.Context, id string) (*LLMProvider, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var provider LLMProvider
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&provider, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// DeleteLLMProviderWithContext é a fachada de transição sobre a global db.
func DeleteLLMProviderWithContext(ctx context.Context, id string) error {
	return NewProviderRepository(db).DeleteLLMProvider(ctx, id)
}

// DeleteLLMProvider remove um provedor do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Sem isso, DELETE por ID puro apaga provedor de qualquer usuário.
func (r *ProviderRepository) DeleteLLMProvider(ctx context.Context, id string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Delete(&LLMProvider{}, "id = ?", id).Error
}

// CountLLMProvidersWithContext é a fachada de transição sobre a global db.
func CountLLMProvidersWithContext(ctx context.Context) (int64, error) {
	return NewProviderRepository(db).CountLLMProviders(ctx)
}

// CountLLMProviders retorna o número total de provedores do usuário do
// contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retornaria contagem
// global — vetor de inferência sobre uso/dimensão da instância.
func (r *ProviderRepository) CountLLMProviders(ctx context.Context) (int64, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return 0, err
	}
	var count int64
	err := ScopeByUser(ctx, db.WithContext(ctx).Model(&LLMProvider{}), "user_id").Count(&count).Error
	return count, err
}

// HasAnyLLMProviderForACPAgent informa se algum usuário desta instalação ainda
// referencia o agente gerenciado pela máquina.
//
// A consulta é global de propósito: a instalação ACP é compartilhada pela
// máquina (AEP-0086 D5), então olhar apenas o usuário autenticado permitiria
// quebrar provedores de outra conta ao apagar o diretório comum. Só um booleano
// sai daqui; nenhum identificador ou configuração de outro usuário é exposto.
func HasAnyLLMProviderForACPAgent(ctx context.Context, agentID string) (bool, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return false, err
	}
	var count int64
	err := db.WithContext(ctx).
		Model(&LLMProvider{}).
		Where("acp_agent_id = ?", agentID).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}

// SetDefaultProviderWithContext é a fachada de transição sobre a global db.
func SetDefaultProviderWithContext(ctx context.Context, id string) error {
	return NewProviderRepository(db).SetDefaultProvider(ctx, id)
}

// SetDefaultProvider marca um provedor como default (e desmarca os demais) no
// escopo do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID, o reset is_default=false
// limparia o default de TODOS os usuários — operação destrutiva cross-user.
func (r *ProviderRepository) SetDefaultProvider(ctx context.Context, id string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope := func(query *gorm.DB) *gorm.DB {
			return ScopeByUser(ctx, query, "user_id")
		}
		var target LLMProvider
		if err := scope(tx).Where("id = ?", id).Take(&target).Error; err != nil {
			return err
		}
		if err := scope(tx.Model(&LLMProvider{})).Where("is_default = ? AND id <> ?", true, id).Updates(map[string]any{
			"is_default":      false,
			"config_revision": gorm.Expr("config_revision + 1"),
		}).Error; err != nil {
			return err
		}
		if target.IsDefault {
			return nil
		}
		return scope(tx.Model(&LLMProvider{})).Where("id = ? AND is_default = ?", id, false).Updates(map[string]any{
			"is_default":      true,
			"config_revision": gorm.Expr("config_revision + 1"),
		}).Error
	})
}

// GetDefaultProviderWithContext é a fachada de transição sobre a global db.
func GetDefaultProviderWithContext(ctx context.Context) (*LLMProvider, error) {
	return NewProviderRepository(db).GetDefaultProvider(ctx)
}

// GetDefaultProvider retorna o provedor marcado como default no escopo do
// usuário do contexto, ou nil se nenhum.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retornaria o primeiro
// default que aparecer no banco — vetor de leak de provider alheio.
func (r *ProviderRepository) GetDefaultProvider(ctx context.Context) (*LLMProvider, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var provider LLMProvider
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&provider, "is_default = ?", true).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}
