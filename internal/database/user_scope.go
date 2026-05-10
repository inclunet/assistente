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

// ScopeByUser aplica filtro por user_id à query quando o ctx carrega
// userID. Por razões de compatibilidade histórica é fail-open: sem userID
// no ctx a query original é devolvida sem filtro. ScopeByUser sozinho NÃO
// fecha o invariante — ele é apenas a cláusula WHERE.
//
// O fail-closed real do AEP-0052 vive em três camadas que se reforçam:
//
//  1. Pontos de entrada públicos (bindings Wails, handlers HTTP) usam
//     App.requireAuthenticatedContext, que falha fechado com
//     ErrUserScopeRequired antes de qualquer chamada que toque dados do
//     usuário.
//
//  2. Repositórios em internal/chat, internal/providers, internal/tasklist
//     chamam RequireUserID (ou RequireUserIDOrBootstrap nas escritas que
//     toleram bootstrap explícito) no início de cada método. Sem userID
//     o método retorna ErrUserScopeRequired antes de tocar o banco.
//
//  3. ScopeByUser anexa user_id = ? na query. Se chegou aqui, a primeira
//     camada já validou; este filtro garante isolamento mesmo em casos
//     de bug nas camadas acima (defesa em profundidade).
//
// Para funções novas que querem opt-in fail-closed direto na query (sem
// depender da camada 1/2), use ScopeByUserStrict — ele retorna erro
// quando o ctx não tem userID.
//
// Funções instance-wide deliberadas (AdoptLegacyData, RebuildFTSIndex,
// FindOrCreateChannelConversationWithContext no caminho de bootstrap)
// NÃO chamam ScopeByUser — usam db.WithContext(...) diretamente e estão
// marcadas com `// SECURITY: instance-wide`.
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

// ScopeByUserStrict é a variante fail-closed de ScopeByUser. Aplica o
// mesmo filtro user_id = ? quando o ctx carrega userID, e retorna a query
// envenenada com ErrUserScopeRequired (via gorm.AddError) quando NÃO
// carrega — qualquer .Find/.First/.Save/.Update/.Delete subsequente
// devolverá o erro automaticamente, sem precisar de check explícito no
// caller.
//
// Use ScopeByUserStrict em código novo ou em funções *WithContext que
// queiram fechar o invariante diretamente na camada de query, em vez de
// depender da chamada a RequireUserID em uma camada anterior. Combina bem
// com tests que injetam context.Background() para validar fail-closed.
func ScopeByUserStrict(ctx context.Context, query *gorm.DB, column string) *gorm.DB {
	if query == nil {
		return query
	}
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		poisoned := query.Session(&gorm.Session{})
		_ = poisoned.AddError(ErrUserScopeRequired)
		return poisoned
	}
	if strings.TrimSpace(column) == "" {
		column = "user_id"
	}
	return query.Where(column+" = ?", userID)
}
