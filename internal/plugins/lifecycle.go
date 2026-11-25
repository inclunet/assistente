package plugins

import (
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
)

var ErrInstallationDisabled = errors.New("PLUGIN_DISABLED")

// ChangeInstallation applies one user decision to one reviewed revision.
// Re-enabling permits new grants. It never replays an earlier authorization.
func (i *Inventory) ChangeInstallation(ctx context.Context, id, digest, action string, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if action != "disable" && action != "enable" && action != "uninstall" {
		return errors.New("unknown plugin action")
	}
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var syncNeeded bool
	err = database.WithSQLiteAdmittedCommit(publishCtx, i.db, "plugins.lifecycle", func(tx *gorm.DB) error {
		var row database.PluginInstallation
		err := tx.Where("user_id=? AND id=? AND digest=?", owner, id, digest).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && action == "uninstall" {
			// Absence is idempotent only if another revision didn't replace it.
			var count int64
			if err := tx.Model(&row).Where("user_id=? AND id=?", owner, id).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return nil
			}
			return ErrInstallationChanged
		}
		if err != nil {
			return err
		}
		if action == "enable" {
			return tx.Model(&row).Update("disabled", false).Error
		}
		var bindings int64
		if err := tx.Model(&database.PluginMCPBinding{}).Where("installation_id=?", id).Count(&bindings).Error; err != nil {
			return err
		}
		syncNeeded = bindings > 0
		if err := removeMCPBindings(publishCtx, tx, id, "", false); err != nil {
			return err
		}
		if err := tx.Where("installation_id=?", id).Delete(&database.PluginSkillActivation{}).Error; err != nil {
			return err
		}
		if action == "disable" {
			return tx.Model(&row).Update("disabled", true).Error
		}
		if err := tx.Where("installation_id=?", id).Delete(&database.PluginRevision{}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	}, admit)
	if err == nil && syncNeeded {
		return i.SyncMCP(ctx, admit)
	}
	return err
}
