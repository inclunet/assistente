package database

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type userIDContextKey struct{}

type bootstrapContextKey struct{}

var ErrUserScopeRequired = errors.New("usuário autenticado obrigatório")

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, strings.TrimSpace(userID))
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	userID, ok := ctx.Value(userIDContextKey{}).(string)
	userID = strings.TrimSpace(userID)
	return userID, ok && userID != ""
}

func RequireUserID(ctx context.Context) (string, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return "", ErrUserScopeRequired
	}
	return userID, nil
}

// WithBootstrap marca o contexto como pertencente a um fluxo de bootstrap
// pré-AEP-0052 — fluxos legítimos que precisam gravar dados antes de
// existir uma sessão autenticada (wizard de boas-vindas/CLI setup, registro
// de credenciais via env, migrações legadas).
//
// É a única forma suportada de bypassar RequireUserID em camadas que falham
// fechado. Callers DEVEM justificar o uso e o invariante deve permanecer
// excepcional (provedores/credenciais ficam órfãos com user_id="" até
// AdoptLegacyData os atribuir ao primeiro usuário).
//
// NÃO use WithBootstrap para conveniência: prefira propagar um contexto
// autenticado real. Bypassar RequireUserID por engano é exatamente o vetor
// que esta marca tenta tornar visível.
func WithBootstrap(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, bootstrapContextKey{}, true)
}

// IsBootstrap retorna true quando o contexto foi explicitamente marcado por
// WithBootstrap. Camadas fail-closed (como providers.DBStore.Save) usam isso
// como única exceção permitida ao RequireUserID.
func IsBootstrap(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(bootstrapContextKey{}).(bool)
	return v
}

// RequireUserIDOrBootstrap aceita o contexto se carrega userID OU se foi
// explicitamente marcado por WithBootstrap. Usado por escritas que precisam
// suportar tanto o caminho autenticado normal quanto o bootstrap pré-login.
func RequireUserIDOrBootstrap(ctx context.Context) error {
	if _, ok := UserIDFromContext(ctx); ok {
		return nil
	}
	if IsBootstrap(ctx) {
		return nil
	}
	return ErrUserScopeRequired
}

// ScopeByUser aplica filtro por user_id à query.
//
// IMPORTANTE: ScopeByUser sozinho NÃO é suficiente como guard de segurança.
// Quando o context não contém userID, esta função silenciosamente NÃO aplica
// filtro (retorna a query original). Isso era necessário para preservar
// helpers internos / admin ops / fluxo de bootstrap.
//
// A enforcement principal (fail-closed contra acessos sem login) acontece em
// duas camadas obrigatórias:
//
//  1. Repositórios públicos (`internal/chat/db_store.go`,
//     `internal/tasklist/db_store.go`, `internal/providers/db_store.go`)
//     chamam `RequireUserID(ctx)` no início de cada método e retornam
//     `ErrUserScopeRequired` quando ausente.
//
//  2. Bindings Wails / handlers HTTP usam `App.requireAuthenticatedContext()`
//     antes de qualquer chamada que toque dados de usuário.
//
// Operações administrativas/instance-wide que ignoram escopo (ex.:
// ClearAllConversations, AdoptLegacyData, RebuildFTSIndex,
// FindOrCreateChannelConversation) não chamam ScopeByUser — usam
// `db.WithContext(...)` diretamente.
func ScopeByUser(ctx context.Context, query *gorm.DB, column string) *gorm.DB {
	if query == nil {
		return query
	}
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return query
	}
	if strings.TrimSpace(column) == "" {
		column = "user_id"
	}
	return query.Where(column+" = ?", userID)
}
