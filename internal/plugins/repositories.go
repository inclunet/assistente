package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/plugincatalog"
	"assistente/internal/pluginpackage"
	"gorm.io/gorm"
)

type Repository struct {
	ID       string                `json:"id"`
	Origin   string                `json:"origin"`
	Revision string                `json:"revision"`
	Updated  time.Time             `json:"updated"`
	Indices  []plugincatalog.Index `json:"indices"`
}

func (i *Inventory) Repositories(ctx context.Context) ([]Repository, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []database.PluginRepository
	if err := i.db.WithContext(ctx).Where("user_id=?", owner).Order("origin").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Repository, 0, len(rows))
	for _, row := range rows {
		item := Repository{ID: row.ID, Origin: row.Origin, Revision: row.Revision, Updated: row.UpdatedAt}
		if err := json.Unmarshal(row.Indices, &item.Indices); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// absolute comes only from the native picker. Refresh uses the stored origin.
// Compare-and-swap prevents a refresh from recreating a removed repository.
func (i *Inventory) SaveRepository(ctx context.Context, absolute, id, expected string, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	expectedRoot := ""
	if id != "" {
		var row database.PluginRepository
		if err := i.db.WithContext(ctx).Where("id=? AND user_id=? AND revision=?", id, owner, expected).Take(&row).Error; err != nil {
			return err
		}
		absolute = row.Origin
		expectedRoot = row.RootID
	}
	if !filepath.IsAbs(absolute) {
		return errors.New("invalid repository path")
	}
	origin, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	if id != "" && origin != absolute {
		return ErrInstallationChanged
	}
	root, err := os.OpenRoot(origin)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	rootID, err := pluginpackage.RootIdentity(root)
	if err != nil {
		return err
	}
	if id != "" && expectedRoot != rootID {
		return ErrInstallationChanged
	}
	indices, err := plugincatalog.ReadRoot(root)
	if err != nil {
		return err
	}
	data, err := json.Marshal(indices)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	revision := hex.EncodeToString(digest[:])
	publish, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return database.WithSQLiteAdmittedCommit(publish, i.db, "plugins.repository.save", func(tx *gorm.DB) error {
		if id == "" {
			var existing database.PluginRepository
			lookup := tx.Where("user_id=? AND origin=?", owner, origin).Take(&existing).Error
			if lookup == nil {
				return nil
			}
			if !errors.Is(lookup, gorm.ErrRecordNotFound) {
				return lookup
			}
			return tx.Create(&database.PluginRepository{UserID: owner, Origin: origin, RootID: rootID, Indices: data, Revision: revision}).Error
		}
		result := tx.Model(&database.PluginRepository{}).Where("id=? AND user_id=? AND revision=?", id, owner, expected).Updates(map[string]any{"indices": data, "revision": revision})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInstallationChanged
		}
		return nil
	}, admit)
}

func (i *Inventory) RemoveRepository(ctx context.Context, id, revision string, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	publish, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return database.WithSQLiteAdmittedCommit(publish, i.db, "plugins.repository.remove", func(tx *gorm.DB) error {
		result := tx.Where("user_id=? AND id=? AND revision=?", owner, id, revision).Delete(&database.PluginRepository{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInstallationChanged
		}
		return nil
	}, admit)
}

func (i *Inventory) InspectRepositoryEntry(ctx context.Context, id, revision string, format plugincatalog.Format, name string) (*pluginpackage.LocalSnapshot, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var row database.PluginRepository
	if err := i.db.WithContext(ctx).Where("user_id=? AND id=? AND revision=?", owner, id, revision).Take(&row).Error; err != nil {
		return nil, err
	}
	var indices []plugincatalog.Index
	if err := json.Unmarshal(row.Indices, &indices); err != nil {
		return nil, err
	}
	for _, index := range indices {
		if index.Format != format {
			continue
		}
		for _, entry := range index.Entries {
			if entry.Name != name {
				continue
			}
			if entry.Diagnostic != "" {
				return nil, ErrComponentUnavailable
			}
			snap, err := pluginpackage.InspectRepositoryPackage(row.Origin, row.RootID, entry.Path)
			if err != nil {
				return nil, err
			}
			if snap.Manifest.Name != entry.Name {
				return nil, ErrPackageChanged
			}
			if entry.Authentication == "ON_INSTALL" {
				for _, component := range DiscoverComponents(snap) {
					if component.Kind == "mcp" {
						return nil, errors.New("PLUGIN_AUTH_ON_INSTALL")
					}
				}
			}
			snap.DiscoveryAuthentication = entry.Authentication
			snap.DiscoveryRepositoryID = row.ID
			snap.DiscoveryRepositoryRevision = row.Revision
			return snap, nil
		}
	}
	return nil, ErrComponentUnavailable
}
