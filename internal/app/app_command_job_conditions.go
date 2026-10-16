package app

import (
	"context"
	"errors"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/credentials"
	"assistente/internal/workspace"
	"gorm.io/gorm"
)

type commandJobWorkspaceKey struct{}

type commandJobWorkspace struct {
	manager     *workspace.Manager
	snapshot    workspace.CommandSnapshot
	principal   auth.LocalSessionPrincipal
	sessions    *auth.SessionService
	credentials *credentials.Manager
	host        *commandexecution.HostState
	available   bool
}

// O Consumer chama esta fronteira sob o gate e antes de abrir a transação.
// Os read locks seguem authMu -> workspace até commit/rollback; as portas
// usam o snapshot carregado sem readquirir nenhum desses locks (inclusive
// quando há um escritor aguardando).
func (a *App) withCommandJobContext(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil || fn == nil {
		return commandjobactivation.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	manager := a.workspaceMgr
	state := commandJobWorkspace{manager: manager, sessions: a.sessionSvc, credentials: a.credMgr, host: a.commandHost}
	if user := a.currentAuthUser; user != nil && user.UserID == a.currentUserID {
		state.principal = auth.LocalSessionPrincipal{UserID: user.UserID, SessionID: user.SessionID}
	}
	// Uma fonte de UI ausente proíbe ativar/renovar; não pode impedir que a
	// manutenção da instância retire claims sem fonte de outros usuários.
	if manager == nil {
		return fn(context.WithValue(ctx, commandJobWorkspaceKey{}, state))
	}
	invoked := false
	err := manager.WithCommandSnapshot(ctx, func(snapshot workspace.CommandSnapshot) error {
		invoked = true
		state.available = true
		state.snapshot = snapshot
		return fn(context.WithValue(ctx, commandJobWorkspaceKey{}, state))
	})
	if invoked || err == nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Erro do snapshot local (não SQL) significa contexto indisponível. Erros
	// vindos da transação nunca entram neste caminho e são propagados intactos.
	return fn(context.WithValue(ctx, commandJobWorkspaceKey{}, state))
}

func commandJobSameWorkspace(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (a *App) commandJobLayer(ctx context.Context, tx *gorm.DB, owner commandactivation.Owner, rule commandactivation.Rule) (bool, error) {
	if ctx == nil || tx == nil {
		return false, commandjobactivation.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !rule.Enabled || rule.ReviewStatus != "active" || rule.UserID != owner.UserID || !commandJobSameWorkspace(rule.WorkspaceID, owner.WorkspaceID) {
		return false, nil
	}
	switch rule.LayerRefKind {
	case commandactivation.UserRef:
		var layer commandconfig.Layer
		err := tx.WithContext(ctx).Where("id = ? AND user_id = ?", rule.LayerRef, owner.UserID).Take(&layer).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return layer.Enabled && commandJobSameWorkspace(layer.WorkspaceID, rule.WorkspaceID), nil
	case commandactivation.BuiltinRef:
		// O catálogo publicado é imutável. Não reconstruí-lo aqui: sua factory
		// consulta authMu, já mantido pela fronteira transacional do Consumer.
		product := a.commandProduct.Load()
		if product == nil || product.registry == nil || product.principal.UserID != owner.UserID || product.principal.SessionID != owner.AuthContextID {
			return false, commandjobactivation.ErrUnavailable
		}
		projection, err := commandProductProjection(product.registry, nil)
		if err != nil {
			return false, err
		}
		for _, layer := range projection.BuiltinLayers {
			if layer.ID == rule.LayerRef {
				return layer.Active, nil
			}
		}
	}
	return false, nil
}

// O backend conhece o perfil efetivo do workspace. Foco, surface, dispositivo
// e foreground só poderão ser usados quando fornecidos por sua fonte autoritativa.
// Ausência permanece desconhecida, nunca um valor inferido do payload do job.
func (a *App) commandJobCondition(ctx context.Context, tx *gorm.DB, owner commandactivation.Owner, rule commandactivation.Rule, _ commandjobevents.Fact) (bool, error) {
	if ctx == nil || tx == nil {
		return false, commandjobactivation.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if rule.UserID != owner.UserID || !commandJobSameWorkspace(rule.WorkspaceID, owner.WorkspaceID) {
		return false, nil
	}
	state, ok := ctx.Value(commandJobWorkspaceKey{}).(commandJobWorkspace)
	if !ok || !state.available || state.principal.UserID == "" || state.principal.UserID != owner.UserID || state.principal.SessionID != owner.AuthContextID {
		return false, commandjobactivation.ErrUnavailable
	}
	snapshot := state.snapshot
	if owner.WorkspaceID != nil && snapshot.WorkspaceID != *owner.WorkspaceID {
		return false, nil
	}
	facts := commandbindings.Facts{}
	profile := snapshot.Tab.ProfileOverrideSlug
	if profile == "" {
		profile = snapshot.WorkspaceProfile
	}
	if profile != "" {
		facts[commandbindings.Profile] = profile
	}
	return commandconfig.MatchCondition(rule.Condition, facts)
}
