package database

import (
	"context"

	"gorm.io/gorm"
)

// ==================== LLM Providers ====================

// SaveLLMProviderWithContext salva ou atualiza um provedor associado ao
// usuário do contexto.
//
// SECURITY: fail-closed bootstrap-tolerant (AEP-0052 / B11). Aceita ctx com
// userID OU marcado por WithBootstrap (CLI setup, registro de credenciais
// via env). Sem nenhum dos dois, retorna ErrUserScopeRequired — antes era
// fail-open silencioso (provider.UserID ficava em branco e gravava órfão).
// Defesa em camadas: o caller providers.DBStore.Save também valida.
func SaveLLMProviderWithContext(ctx context.Context, provider *LLMProvider) error {
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	if provider != nil && provider.UserID == "" {
		if userID, ok := UserIDFromContext(ctx); ok {
			provider.UserID = userID
		}
	}
	return db.WithContext(ctx).Save(provider).Error
}

// GetLLMProvidersWithContext retorna todos os provedores do usuário do
// contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Retornar lista global expõe IDs/credenciais (mesmo cifradas/refs) de
// todos os usuários da instância.
func GetLLMProvidersWithContext(ctx context.Context) ([]*LLMProvider, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var providers []*LLMProvider
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").Order("created_at ASC").Find(&providers).Error
	return providers, err
}

// GetLLMProviderWithContext busca um provedor por ID no escopo do usuário do
// contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Antes, ScopeByUser fail-open + First por ID = leitura cross-user de
// provedor alheio com todos os metadados.
func GetLLMProviderWithContext(ctx context.Context, id string) (*LLMProvider, error) {
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

// DeleteLLMProviderWithContext remove um provedor do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Sem isso, DELETE por ID puro apaga provedor de qualquer usuário.
func DeleteLLMProviderWithContext(ctx context.Context, id string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Delete(&LLMProvider{}, "id = ?", id).Error
}

// CountLLMProvidersWithContext retorna o número total de provedores do
// usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retornaria contagem
// global — vetor de inferência sobre uso/dimensão da instância.
func CountLLMProvidersWithContext(ctx context.Context) (int64, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return 0, err
	}
	var count int64
	err := ScopeByUser(ctx, db.WithContext(ctx).Model(&LLMProvider{}), "user_id").Count(&count).Error
	return count, err
}

// SetDefaultProviderWithContext marca um provedor como default (e desmarca os
// demais) no escopo do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID, o reset is_default=false
// limparia o default de TODOS os usuários — operação destrutiva cross-user.
func SetDefaultProviderWithContext(ctx context.Context, id string) error {
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

// GetDefaultProviderWithContext retorna o provedor marcado como default no
// escopo do usuário do contexto, ou nil se nenhum.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retornaria o primeiro
// default que aparecer no banco — vetor de leak de provider alheio.
func GetDefaultProviderWithContext(ctx context.Context) (*LLMProvider, error) {
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

// ensureTaskNoteExternalUniqueIndex aplica índice único parcial em (external_source, external_id).
//
// Escolha de modelagem: chave única global por origem (sem task_id na unicidade), alinhada à
// preferência de produto e ao padrão “ID estável no sistema remoto”. O mesmo comentário Jira
// (por exemplo) deve mapear a no máximo uma TaskNote no app, impedindo duplicatas em re-syncs.
// Notas manuais permanecem fora do índice (WHERE ambos os campos não vazios).
//
// Se a mesma referência externa for associada a outra task local, UpsertTaskNoteByExternal
// retorna erro explícito em vez de duplicar linhas.
