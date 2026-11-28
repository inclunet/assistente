package plugins

import (
	"assistente/internal/authorization"
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
	return i.changeInstallation(ctx, id, digest, action, "", nil, admit)
}

func (i *Inventory) UninstallReviewed(ctx context.Context, id, digest, choice string, reviewed map[string]ProviderRemovalItem, admit commandsecurity.Admission) error {
	if choice != "detach" && choice != "remove" {
		return ErrInstallationChanged
	}
	return i.changeInstallation(ctx, id, digest, "uninstall", choice, reviewed, admit)
}

func (i *Inventory) changeInstallation(ctx context.Context, id, digest, action, choice string, reviewed map[string]ProviderRemovalItem, admit commandsecurity.Admission) error {
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
	var removedProviders []string
	var revokedJobs []string
	var syncNeeded bool
	commit := func(refs map[string][]string, complete bool) error {
		return database.WithSQLiteAdmittedCommit(publishCtx, i.db, "plugins.lifecycle", func(tx *gorm.DB) error {
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
			if err := authorization.DeleteInstallationHookRules(tx, owner, id); err != nil {
				return err
			}
			var bindings int64
			if err := tx.Model(&database.PluginMCPBinding{}).Where("installation_id=?", id).Count(&bindings).Error; err != nil {
				return err
			}
			revokedJobs, err = revokeJobs(tx, owner, id)
			if err != nil {
				return err
			}
			syncNeeded = bindings > 0
			if err := removeMCPBindings(publishCtx, tx, id, "", false); err != nil {
				return err
			}
			if err := tx.Where("installation_id=?", id).Delete(&database.PluginProfileActivation{}).Error; err != nil {
				return err
			}
			if err := tx.Where("installation_id=?", id).Delete(&database.PluginSkillActivation{}).Error; err != nil {
				return err
			}
			if err := tx.Where("installation_id=?", id).Delete(&database.PluginAgentActivation{}).Error; err != nil {
				return err
			}
			if action == "disable" {
				return tx.Model(&row).Where("disabled = ?", false).Updates(map[string]any{"disabled": true, "hook_generation": gorm.Expr("hook_generation + 1")}).Error
			}
			removedProviders, err = i.applyProviderRemoval(tx, publishCtx, owner, id, choice, reviewed, refs, complete)
			if err != nil {
				return err
			}

			if err := tx.Where("installation_id=?", id).Delete(&database.PluginRevision{}).Error; err != nil {
				return err
			}
			return tx.Delete(&row).Error
		}, admit)
	}
	if action == "uninstall" && choice == "remove" {
		if i.removeProviders == nil {
			return ErrComponentUnavailable
		}
		err = i.withProviderConsumers(func(refs map[string][]string, complete bool) error {
			return i.removeProviders(func() ([]string, error) { err := commit(refs, complete); return removedProviders, err })
		})
	} else if action == "uninstall" && len(reviewed) > 0 {
		err = i.withProviderConsumers(commit)
	} else {
		err = commit(map[string][]string{}, false)
	}
	if err != nil {
		return err
	}
	return i.refreshRuntime(ctx, revokedJobs, syncNeeded, admit)
}
