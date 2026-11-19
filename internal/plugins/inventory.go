package plugins

import (
	"context"
	"encoding/json"
	"errors"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPackageChanged = errors.New("o pacote mudou desde a prévia")
var ErrInstallationChanged = errors.New("a instalação mudou desde a prévia")

type Inventory struct{ db *gorm.DB }

func NewInventory(db *gorm.DB) *Inventory { return &Inventory{db: db} }

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
	return i.InstallReviewed(ctx, reviewedOrigin, reviewedDigest, previousDigest, func(publish func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return publish()
	})
}

func (i *Inventory) InstallReviewed(ctx context.Context, reviewedOrigin, reviewedDigest, previousDigest string, admit commandsecurity.Admission) (database.PluginInstallation, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return database.PluginInstallation{}, err
	}
	snapshot, err := pluginpackage.InspectLocal(reviewedOrigin)
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
	err = admit(func() error {
		return database.WithSQLiteImmediateTransaction(ctx, i.db, "plugins.install", func(tx *gorm.DB) error {
			installed = database.PluginInstallation{}
			identity := database.PluginInstallation{UserID: owner, Name: snapshot.Manifest.Name, Origin: snapshot.Origin}
			lookup := tx.Where(&identity).First(&installed).Error
			switch {
			case errors.Is(lookup, gorm.ErrRecordNotFound):
				if previousDigest != "" {
					return ErrInstallationChanged
				}
				installed = identity
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
				update := tx.Model(&installed).Where("user_id = ? AND digest = ?", owner, previousDigest).Updates(map[string]any{"digest": snapshot.Digest, "version": snapshot.Manifest.Version})
				if update.Error != nil {
					return update.Error
				}
				if update.RowsAffected != 1 {
					return ErrInstallationChanged
				}
			}
			revision := database.PluginRevision{InstallationID: installed.ID, Digest: snapshot.Digest, Files: files}
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&revision).Error
		})
	})
	if err != nil {
		return database.PluginInstallation{}, err
	}
	return installed, nil
}
