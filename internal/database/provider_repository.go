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
	if provider != nil && provider.UserID == "" {
		if userID, ok := UserIDFromContext(ctx); ok {
			provider.UserID = userID
		}
	}
	// SECURITY (AEP-0052 / fail-closed): em contexto autenticado, impedir que o
	// Save (UPSERT por PK) sobrescreva um provider existente pertencente a OUTRO
	// usuário ao reutilizar seu ID — vetor de hijack cross-user. Registros órfãos
	// (user_id="") permanecem adotáveis (AdoptLegacyData). O caminho bootstrap
	// (sem userID no ctx) não dispara esta checagem.
	if userID, ok := UserIDFromContext(ctx); ok && provider != nil && provider.ID != "" {
		var existing LLMProvider
		err := db.WithContext(ctx).Select("id", "user_id").First(&existing, "id = ?", provider.ID).Error
		switch {
		case err == nil:
			if existing.UserID != "" && existing.UserID != userID {
				return ErrProviderCrossUser
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			// Sem registro pré-existente: criação normal.
		default:
			return err
		}
	}
	return db.WithContext(ctx).Save(provider).Error
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
		scoped := ScopeByUser(ctx, tx.Model(&LLMProvider{}), "user_id")
		if err := scoped.Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		return ScopeByUser(ctx, tx.Model(&LLMProvider{}), "user_id").Where("id = ?", id).Update("is_default", true).Error
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
