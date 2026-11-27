package authorization

import (
	"context"
	"encoding/json"
	"errors"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/tools/invocationctx"
	"gorm.io/gorm"
)

const PluginHookDomain = "plugin_hook"

// PluginHook identifica um hook na instalação e na revisão apresentadas à pessoa.
type PluginHook struct {
	Origin         string `json:"origin"`
	Name           string `json:"name"`
	Generation     uint64 `json:"generation"`
	InstallationID string `json:"installationId"`
	Digest         string `json:"digest"`
	Component      string `json:"component"`
}

// RememberPluginHook registra o aceite ou a negativa pela mesma admissão usada no inventário.
// A validação de compatibilidade do componente cabe ao adaptador antes de pedir consentimento.
func (r *Repository) RememberPluginHook(ctx context.Context, hook PluginHook, scope Scope, workspaceID, effect string, admit commandsecurity.Admission) error {
	db, owner, err := r.query(ctx)
	if err != nil {
		return err
	}
	if hook.Origin == "" || hook.Name == "" || hook.InstallationID == "" || hook.Digest == "" || hook.Component == "" || (effect != "allow" && effect != "deny") {
		return errors.New("decisão de hook inválida")
	}
	validScope := scope == ScopeGlobal && workspaceID == "" || scope == ScopeWorkspace && workspaceID != ""
	if !validScope {
		return errors.New("escopo de hook inválido")
	}
	selector, err := json.Marshal(hook)
	if err != nil {
		return err
	}
	return database.WithSQLiteAdmittedCommit(ctx, db, "authorization.plugin_hook", func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.PluginInstallation{}).Where("id = ? AND user_id = ? AND digest = ? AND disabled = ? AND hook_generation = ? AND name = ? AND origin = ?", hook.InstallationID, owner, hook.Digest, false, hook.Generation, hook.Name, hook.Origin).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrNotFound
		}
		return upsert(tx, &database.AuthorizationRule{
			UserID: owner, Domain: PluginHookDomain, Operation: "invoke", Scope: string(scope), ScopeKey: workspaceID,
			Selector: string(selector), SelectorKey: string(selector), SelectorVersion: 1, Effect: effect, Origin: "consent",
		})
	}, admit)
}

// PluginHookAllowed verifica a decisão vigente para a origem já validada do turno.
// Uma negativa aplicável prevalece mesmo se uma permissão global também estiver vigente.
func (r *Repository) PluginHookAllowed(ctx context.Context, hook PluginHook) (bool, error) {
	db, owner, err := r.query(ctx)
	if err != nil {
		return false, err
	}
	workspace := invocationctx.VerifiedWorkspaceID(ctx)
	if workspace == "" {
		return false, nil
	}
	selector, err := json.Marshal(hook)
	if err != nil {
		return false, err
	}
	rules, err := listScopes(db, owner, PluginHookDomain, map[string]string{"workspace": workspace, "global": ""}, string(selector))
	if err != nil {
		return false, err
	}
	for _, rule := range rules {
		if rule.Effect != "allow" {
			return false, nil
		}
	}
	return len(rules) > 0, nil
}

// DeleteInstallationHookRules remove apenas decisões pertencentes à instalação, inclusive negativas
// e regras já revogadas. O inventário chama esta função dentro da transação que invalida a revisão.
func DeleteInstallationHookRules(tx *gorm.DB, owner, installationID string) error {
	return tx.Where("user_id = ? AND domain = ? AND json_extract(selector, '$.installationId') = ?", owner, PluginHookDomain, installationID).
		Delete(&database.AuthorizationRule{}).Error
}
