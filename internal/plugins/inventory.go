package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrMCPSyncPending = errors.New("PLUGIN_MCP_SYNC_PENDING")

var ErrPackageChanged = errors.New("o pacote mudou desde a prévia")
var ErrInstallationChanged = errors.New("a instalação mudou desde a prévia")

type Inventory struct {
	db      *gorm.DB
	syncMCP func(context.Context) error
}

func NewInventory(db *gorm.DB) *Inventory { return &Inventory{db: db} }

func (i *Inventory) WithMCPSync(sync func(context.Context) error) *Inventory {
	return &Inventory{db: i.db, syncMCP: sync}
}
func (i *Inventory) SyncMCP(ctx context.Context, admit commandsecurity.Admission) error {
	if err := admit.Check(); err != nil {
		return errors.Join(ErrMCPSyncPending, err)
	}
	if i.syncMCP != nil {
		if err := i.syncMCP(ctx); err != nil {
			return errors.Join(ErrMCPSyncPending, err)
		}
	}
	return nil
}

func (i *Inventory) List(ctx context.Context) ([]database.PluginInstallation, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []database.PluginInstallation
	err = i.db.WithContext(ctx).Where("user_id = ?", owner).Order("name, origin").Find(&rows).Error
	return rows, err
}

// InstallLocal binds publication to both the inspected revision and the
// previously installed digest. The complete captured revision and its pointer
// commit together. Repeating an already published installation is idempotent.
func (i *Inventory) InstallLocal(ctx context.Context, reviewedOrigin, reviewedDigest, previousDigest string) (database.PluginInstallation, error) {
	return i.InstallReviewed(ctx, reviewedOrigin, reviewedDigest, previousDigest, nil, func(_ *gorm.DB, publish func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return publish()
	})
}

func (i *Inventory) InstallReviewed(ctx context.Context, reviewedOrigin, reviewedDigest, previousDigest string, reviewedServers []BoundMCPServer, admit commandsecurity.Admission) (database.PluginInstallation, error) {
	return i.InstallSource(ctx, reviewedOrigin, reviewedDigest, previousDigest, reviewedServers, admit, func() (*pluginpackage.LocalSnapshot, error) { return pluginpackage.InspectLocal(reviewedOrigin) })
}

func (i *Inventory) InstallSource(ctx context.Context, reviewedOrigin, reviewedDigest, previousDigest string, reviewedServers []BoundMCPServer, admit commandsecurity.Admission, inspect func() (*pluginpackage.LocalSnapshot, error)) (database.PluginInstallation, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return database.PluginInstallation{}, err
	}
	snapshot, err := inspect()
	if err != nil {
		return database.PluginInstallation{}, err
	}
	if snapshot.Origin != reviewedOrigin || snapshot.Digest != reviewedDigest {
		return database.PluginInstallation{}, ErrPackageChanged
	}
	files, err := json.Marshal(snapshot.Files)
	if err != nil {
		return database.PluginInstallation{}, err
	}
	var installed database.PluginInstallation
	if admit == nil {
		return database.PluginInstallation{}, database.ErrUserScopeRequired
	}
	mcpChanged := false
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = database.WithSQLiteAdmittedCommit(publishCtx, i.db, "plugins.install", func(tx *gorm.DB) error {
		if snapshot.DiscoveryRepositoryID != "" {
			var repo database.PluginRepository
			if err := tx.Select("id").Where("user_id=? AND id=? AND revision=?", owner, snapshot.DiscoveryRepositoryID, snapshot.DiscoveryRepositoryRevision).Take(&repo).Error; err != nil {
				return err
			}
		}
		installed = database.PluginInstallation{}
		identity := database.PluginInstallation{UserID: owner, Name: snapshot.Manifest.Name, Origin: snapshot.Origin}
		lookup := tx.Where(&identity).First(&installed).Error
		switch {
		case errors.Is(lookup, gorm.ErrRecordNotFound):
			if previousDigest != "" {
				return ErrInstallationChanged
			}
			installed = identity
			installed.DiscoveryRepositoryID = snapshot.DiscoveryRepositoryID
			installed.Digest = snapshot.Digest
			installed.Version = snapshot.Manifest.Version
			if err := tx.Create(&installed).Error; err != nil {
				return err
			}
		case lookup != nil:
			return lookup
		case installed.Digest == snapshot.Digest:
			return nil
		case installed.Digest != previousDigest:
			return ErrInstallationChanged
		default:
			actual, err := boundMCPServers(publishCtx, tx, owner, installed.ID, previousDigest)
			if err != nil {
				return err
			}
			if !slices.Equal(actual, reviewedServers) {
				return ErrInstallationChanged
			}
			update := tx.Model(&installed).Where("user_id = ? AND digest = ?", owner, previousDigest).Updates(map[string]any{"digest": snapshot.Digest, "version": snapshot.Manifest.Version})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return ErrInstallationChanged
			}
			var count int64
			if err := tx.Model(&database.PluginMCPBinding{}).Where("installation_id=?", installed.ID).Count(&count).Error; err != nil {
				return err
			}
			mcpChanged = count > 0
			if err := removeMCPBindings(publishCtx, tx, installed.ID, "", false); err != nil {
				return err
			}
			if err := tx.Where("installation_id = ?", installed.ID).Delete(&database.PluginSkillActivation{}).Error; err != nil {
				return err
			}
		}
		revision := database.PluginRevision{InstallationID: installed.ID, Digest: snapshot.Digest, Files: files}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&revision).Error
	}, admit)
	if err != nil {
		return database.PluginInstallation{}, err
	}
	if mcpChanged {
		return installed, i.SyncMCP(ctx, admit)
	}
	return installed, nil
}
