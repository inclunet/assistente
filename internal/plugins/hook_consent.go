package plugins

import (
	"context"
	"strconv"

	"assistente/internal/authorization"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"gorm.io/gorm"
)

// DecideToolHook vincula a escolha à geração exibida no diálogo de ativação.
func (i *Inventory) DecideToolHook(ctx context.Context, id, digest, key, generation, workspace, choice string, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	var row database.PluginInstallation
	if err := i.db.WithContext(ctx).Where("id=? AND user_id=? AND digest=? AND disabled=?", id, owner, digest, false).Take(&row).Error; err != nil {
		return err
	}
	if (key != cursorPreToolKey && key != cursorPostToolKey) || strconv.FormatUint(row.HookGeneration, 10) != generation {
		return ErrInstallationChanged
	}
	snapshot, err := i.installedSnapshot(ctx, id, digest)
	if err != nil {
		return err
	}
	if _, ok := toolHookScript(snapshot, key); !ok {
		return ErrComponentUnavailable
	}
	scope := authorization.ScopeWorkspace
	effect := "allow"
	switch choice {
	case "workspace":
	case "denyWorkspace":
		effect = "deny"
	case "global":
		scope = authorization.ScopeGlobal
		workspace = ""
	default:
		return ErrComponentUnavailable
	}
	hook := authorization.PluginHook{Origin: row.Origin, Name: row.Name, InstallationID: id, Digest: digest, Generation: row.HookGeneration, Component: key}
	return authorization.NewRepository(func() *gorm.DB { return i.db }).RememberPluginHook(ctx, hook, scope, workspace, effect, admit)
}
